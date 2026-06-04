package sqliteserver

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// RepoNotes is the server-side repo-note store. Upsert is OCC + Lamport,
// pull-since paged. Does NOT satisfy ports.RepoNoteStore — Upsert takes
// expectedVersion, same divergence pattern as Sessions/Projects/Repos.
type RepoNotes struct{ store *Store }

// NewRepoNotes constructs a RepoNotes sub-adapter backed by store.
func NewRepoNotes(s *Store) *RepoNotes { return &RepoNotes{store: s} }

// Upsert applies the note with OCC. expectedVersion=0 must mean "new row".
// Server validates the repo row exists for the same user — FK protection.
func (n *RepoNotes) Upsert(in domain.RepoNote, expectedVersion int64) (domain.RepoNote, error) {
	tx, err := n.store.DB().Begin()
	if err != nil {
		return domain.RepoNote{}, fmt.Errorf("sqliteserver.RepoNotes.Upsert: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var curVersion int64
	row := tx.QueryRow(`SELECT version FROM repo_notes WHERE id = ?`, in.ID)
	switch err := row.Scan(&curVersion); {
	case errors.Is(err, sql.ErrNoRows):
		if expectedVersion != 0 {
			return domain.RepoNote{}, ports.ErrRepoNoteVersionConflict
		}
	case err != nil:
		return domain.RepoNote{}, fmt.Errorf("scan version: %w", err)
	default:
		if curVersion != expectedVersion {
			return domain.RepoNote{}, ports.ErrRepoNoteVersionConflict
		}
	}

	// FK guard: repo must exist for the same user.
	var ownerID string
	if err := tx.QueryRow(`SELECT user_id FROM repos WHERE id = ?`, in.RepoID).Scan(&ownerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.RepoNote{}, fmt.Errorf("repo %q not found", in.RepoID)
		}
		return domain.RepoNote{}, fmt.Errorf("repo lookup: %w", err)
	}
	if ownerID != in.UserID {
		return domain.RepoNote{}, fmt.Errorf("repo %q belongs to another user", in.RepoID)
	}

	v, err := NextLamport(tx)
	if err != nil {
		return domain.RepoNote{}, fmt.Errorf("lamport: %w", err)
	}
	updatedAt := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.Exec(
		`INSERT INTO repo_notes (id, repo_id, user_id, content, version, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   content    = excluded.content,
		   version    = excluded.version,
		   updated_at = excluded.updated_at`,
		in.ID, in.RepoID, in.UserID, in.Content, v, updatedAt,
	); err != nil {
		return domain.RepoNote{}, fmt.Errorf("exec: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.RepoNote{}, fmt.Errorf("commit: %w", err)
	}
	in.Version = v
	in.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return in, nil
}

// GetByRepo returns the note for (userID, repoID), or ErrRepoNoteNotFound.
func (n *RepoNotes) GetByRepo(userID, repoID string) (domain.RepoNote, error) {
	row := n.store.DB().QueryRow(
		`SELECT id, repo_id, user_id, content, version, updated_at
		   FROM repo_notes WHERE user_id = ? AND repo_id = ?`,
		userID, repoID,
	)
	return scanServerRepoNote(row)
}

// PullSince returns notes with Version > since for the user, ordered ASC.
func (n *RepoNotes) PullSince(userID string, since int64, limit int) ([]domain.RepoNote, int64, bool, error) {
	rows, err := n.store.DB().Query(
		`SELECT id, repo_id, user_id, content, version, updated_at
		   FROM repo_notes WHERE user_id = ? AND version > ?
		   ORDER BY version ASC LIMIT ?`,
		userID, since, limit+1,
	)
	if err != nil {
		return nil, since, false, err
	}
	defer func() { _ = rows.Close() }()

	var out []domain.RepoNote
	for rows.Next() {
		note, err := scanServerRepoNoteRows(rows)
		if err != nil {
			return nil, since, false, err
		}
		out = append(out, note)
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

func scanServerRepoNoteCommon(s serverRowScanner) (domain.RepoNote, error) {
	var (
		n         domain.RepoNote
		updatedAt string
	)
	err := s.Scan(&n.ID, &n.RepoID, &n.UserID, &n.Content, &n.Version, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.RepoNote{}, ports.ErrRepoNoteNotFound
	}
	if err != nil {
		return domain.RepoNote{}, fmt.Errorf("scan: %w", err)
	}
	if updatedAt != "" {
		t, perr := time.Parse(time.RFC3339, updatedAt)
		if perr != nil {
			return domain.RepoNote{}, fmt.Errorf("parse updated_at: %w", perr)
		}
		n.UpdatedAt = t
	}
	return n, nil
}

func scanServerRepoNote(row *sql.Row) (domain.RepoNote, error) { return scanServerRepoNoteCommon(row) }

func scanServerRepoNoteRows(rows *sql.Rows) (domain.RepoNote, error) {
	return scanServerRepoNoteCommon(rows)
}
