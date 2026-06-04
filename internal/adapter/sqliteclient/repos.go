package sqliteclient

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Repos implements ports.RepoStore against the SQLite repos table.
type Repos struct {
	store *Store
}

var _ ports.RepoStore = (*Repos)(nil)

// NewRepos constructs a Repos sub-adapter backed by store.
func NewRepos(store *Store) *Repos { return &Repos{store: store} }

// EnsureByCanonicalKey returns the existing Repo for (userID, key) or
// inserts a fresh row with a new UUID + version=0. Idempotent.
func (r *Repos) EnsureByCanonicalKey(userID, key, displayName string) (domain.Repo, error) {
	row := r.store.DB().QueryRow(
		`SELECT id, user_id, canonical_key, display_name, created_at, version
		   FROM repos WHERE user_id = ? AND canonical_key = ?`,
		userID, key,
	)
	repo, err := scanRepo(row)
	if err == nil {
		return repo, nil
	}
	if !errors.Is(err, ports.ErrRepoNotFound) {
		return domain.Repo{}, fmt.Errorf("sqliteclient.Repos.EnsureByCanonicalKey: lookup: %w", err)
	}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := r.store.DB().Exec(
		`INSERT INTO repos (id, user_id, canonical_key, display_name, created_at, version)
		 VALUES (?, ?, ?, ?, ?, 0)`,
		id, userID, key, displayName, now,
	); err != nil {
		return domain.Repo{}, fmt.Errorf("sqliteclient.Repos.EnsureByCanonicalKey: insert: %w", err)
	}
	return r.GetByID(userID, id)
}

// GetByID returns the Repo for (userID, id). ErrRepoNotFound when missing.
func (r *Repos) GetByID(userID, id string) (domain.Repo, error) {
	row := r.store.DB().QueryRow(
		`SELECT id, user_id, canonical_key, display_name, created_at, version
		   FROM repos WHERE user_id = ? AND id = ?`,
		userID, id,
	)
	repo, err := scanRepo(row)
	if err != nil {
		if errors.Is(err, ports.ErrRepoNotFound) {
			return domain.Repo{}, ports.ErrRepoNotFound
		}
		return domain.Repo{}, fmt.Errorf("sqliteclient.Repos.GetByID: %w", err)
	}
	return repo, nil
}

// Upsert inserts or replaces a Repo row. Used by the sync worker to ingest
// server-side rows; no OCC here — server is authoritative for Version.
func (r *Repos) Upsert(in domain.Repo) error {
	createdAt := in.CreatedAt.UTC().Format(time.RFC3339)
	_, err := r.store.DB().Exec(
		`INSERT INTO repos (id, user_id, canonical_key, display_name, created_at, version)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   canonical_key = excluded.canonical_key,
		   display_name  = excluded.display_name,
		   version       = excluded.version`,
		in.ID, in.UserID, in.CanonicalKey, in.DisplayName, createdAt, in.Version,
	)
	if err != nil {
		return fmt.Errorf("sqliteclient.Repos.Upsert: %w", err)
	}
	return nil
}

// PullSince returns repos with Version > since, ordered by Version ASC.
// hasMore is true when the page filled to limit.
func (r *Repos) PullSince(userID string, since int64, limit int) ([]domain.Repo, int64, bool, error) {
	rows, err := r.store.DB().Query(
		`SELECT id, user_id, canonical_key, display_name, created_at, version
		   FROM repos WHERE user_id = ? AND version > ?
		   ORDER BY version ASC LIMIT ?`,
		userID, since, limit+1,
	)
	if err != nil {
		return nil, since, false, fmt.Errorf("sqliteclient.Repos.PullSince: query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Repo
	for rows.Next() {
		repo, err := scanRepoFromRows(rows)
		if err != nil {
			return nil, since, false, err
		}
		out = append(out, repo)
	}
	if err := rows.Err(); err != nil {
		return nil, since, false, fmt.Errorf("sqliteclient.Repos.PullSince: rows: %w", err)
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

// rowScanner unifies *sql.Row and *sql.Rows for scanRepo / scanRepoFromRows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanRepoCommon(s rowScanner) (domain.Repo, error) {
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
			return domain.Repo{}, fmt.Errorf("parse created_at %q: %w", createdAt, perr)
		}
		repo.CreatedAt = t
	}
	return repo, nil
}

func scanRepo(row *sql.Row) (domain.Repo, error)           { return scanRepoCommon(row) }
func scanRepoFromRows(rows *sql.Rows) (domain.Repo, error) { return scanRepoCommon(rows) }
