package sqliteserver

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Repos is the server-side repo store. Upsert is OCC + Lamport-versioned
// (server is the version authority); EnsureByCanonicalKey is idempotent
// per (user_id, canonical_key).
//
// Note: server Upsert signature includes expectedVersion, so *Repos does
// NOT satisfy ports.RepoStore (which has no version arg). HTTP handlers
// call the concrete type directly.
type Repos struct{ store *Store }

// NewRepos constructs a Repos sub-adapter backed by store.
func NewRepos(s *Store) *Repos { return &Repos{store: s} }

// EnsureByCanonicalKey returns the existing row for (userID, key) or
// inserts a fresh one with a new UUID + version=0.
func (r *Repos) EnsureByCanonicalKey(userID, key, displayName string) (domain.Repo, error) {
	existing, err := r.getByKey(userID, key)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ports.ErrRepoNotFound) {
		return domain.Repo{}, fmt.Errorf("sqliteserver.Repos.EnsureByCanonicalKey: lookup: %w", err)
	}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := r.store.DB().Exec(
		`INSERT INTO repos (id, user_id, canonical_key, display_name, created_at, version)
		 VALUES (?, ?, ?, ?, ?, 0)`,
		id, userID, key, displayName, now,
	); err != nil {
		return domain.Repo{}, fmt.Errorf("sqliteserver.Repos.EnsureByCanonicalKey: insert: %w", err)
	}
	return r.GetByID(userID, id)
}

// Upsert applies the repo with optimistic concurrency. expectedVersion=0
// means "must not exist" for a new row; non-zero requires the stored row
// to match exactly.
func (r *Repos) Upsert(in domain.Repo, expectedVersion int64) (domain.Repo, error) {
	tx, err := r.store.DB().Begin()
	if err != nil {
		return domain.Repo{}, fmt.Errorf("sqliteserver.Repos.Upsert: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var curVersion int64
	row := tx.QueryRow(`SELECT version FROM repos WHERE id = ?`, in.ID)
	switch err := row.Scan(&curVersion); {
	case errors.Is(err, sql.ErrNoRows):
		if expectedVersion != 0 {
			return domain.Repo{}, ports.ErrRepoVersionConflict
		}
	case err != nil:
		return domain.Repo{}, fmt.Errorf("scan version: %w", err)
	default:
		if curVersion != expectedVersion {
			return domain.Repo{}, ports.ErrRepoVersionConflict
		}
	}

	v, err := NextLamport(tx)
	if err != nil {
		return domain.Repo{}, fmt.Errorf("lamport: %w", err)
	}
	createdAt := in.CreatedAt.UTC().Format(time.RFC3339)
	if in.CreatedAt.IsZero() {
		createdAt = time.Now().UTC().Format(time.RFC3339)
	}
	if _, err := tx.Exec(
		`INSERT INTO repos (id, user_id, canonical_key, display_name, created_at, version)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   canonical_key = excluded.canonical_key,
		   display_name  = excluded.display_name,
		   version       = excluded.version`,
		in.ID, in.UserID, in.CanonicalKey, in.DisplayName, createdAt, v,
	); err != nil {
		return domain.Repo{}, fmt.Errorf("exec: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.Repo{}, fmt.Errorf("commit: %w", err)
	}
	in.Version = v
	return in, nil
}

// GetByID returns the Repo for (userID, id). ErrRepoNotFound on miss.
func (r *Repos) GetByID(userID, id string) (domain.Repo, error) {
	row := r.store.DB().QueryRow(
		`SELECT id, user_id, canonical_key, display_name, created_at, version
		   FROM repos WHERE user_id = ? AND id = ?`,
		userID, id,
	)
	return scanServerRepo(row)
}

// getByKey looks up by (userID, canonical_key) — internal helper for
// EnsureByCanonicalKey.
func (r *Repos) getByKey(userID, key string) (domain.Repo, error) {
	row := r.store.DB().QueryRow(
		`SELECT id, user_id, canonical_key, display_name, created_at, version
		   FROM repos WHERE user_id = ? AND canonical_key = ?`,
		userID, key,
	)
	return scanServerRepo(row)
}

// PullSince returns repos with Version > since, ordered ASC.
func (r *Repos) PullSince(userID string, since int64, limit int) ([]domain.Repo, int64, bool, error) {
	rows, err := r.store.DB().Query(
		`SELECT id, user_id, canonical_key, display_name, created_at, version
		   FROM repos WHERE user_id = ? AND version > ?
		   ORDER BY version ASC LIMIT ?`,
		userID, since, limit+1,
	)
	if err != nil {
		return nil, since, false, err
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Repo
	for rows.Next() {
		repo, err := scanServerRepoRows(rows)
		if err != nil {
			return nil, since, false, err
		}
		out = append(out, repo)
	}
	if err := rows.Err(); err != nil {
		return nil, since, false, err
	}

	hasMore := false
	if len(out) > limit {
		out = out[:limit]
		hasMore = true
	}
	high := since
	if len(out) > 0 {
		high = out[len(out)-1].Version
	}
	return out, high, hasMore, nil
}

// serverRowScanner unifies *sql.Row and *sql.Rows for scan helpers across
// the server-side repo + repo_notes adapters.
type serverRowScanner interface {
	Scan(dest ...any) error
}

func scanServerRepoCommon(s serverRowScanner) (domain.Repo, error) {
	var (
		repo      domain.Repo
		createdAt string
	)
	err := s.Scan(&repo.ID, &repo.UserID, &repo.CanonicalKey, &repo.DisplayName, &createdAt, &repo.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Repo{}, ports.ErrRepoNotFound
	}
	if err != nil {
		return domain.Repo{}, fmt.Errorf("scan: %w", err)
	}
	if createdAt != "" {
		t, perr := time.Parse(time.RFC3339, createdAt)
		if perr != nil {
			return domain.Repo{}, fmt.Errorf("parse created_at: %w", perr)
		}
		repo.CreatedAt = t
	}
	return repo, nil
}

func scanServerRepo(row *sql.Row) (domain.Repo, error)       { return scanServerRepoCommon(row) }
func scanServerRepoRows(rows *sql.Rows) (domain.Repo, error) { return scanServerRepoCommon(rows) }
