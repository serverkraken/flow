package sqliteclient

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// RepoNotes implements ports.RepoNoteStore against the SQLite repo_notes table.
type RepoNotes struct {
	store *Store
}

var _ ports.RepoNoteStore = (*RepoNotes)(nil)

// NewRepoNotes constructs a RepoNotes sub-adapter backed by store.
func NewRepoNotes(store *Store) *RepoNotes { return &RepoNotes{store: store} }

// GetByRepo returns the RepoNote for (userID, repoID). Returns
// ports.ErrRepoNoteNotFound when none exists.
func (r *RepoNotes) GetByRepo(userID, repoID string) (domain.RepoNote, error) {
	row := r.store.DB().QueryRow(
		`SELECT id, repo_id, user_id, content, version, updated_at
		   FROM repo_notes WHERE user_id = ? AND repo_id = ?`,
		userID, repoID,
	)
	return scanRepoNoteRow(row)
}

// Upsert inserts or replaces a RepoNote row. Used by both the editing path
// (use-case writes, version=existing.Version) and the sync ingestion path
// (sync worker writes, version=server's).
func (r *RepoNotes) Upsert(n domain.RepoNote) error {
	updatedAt := n.UpdatedAt.UTC().Format(time.RFC3339)
	_, err := r.store.DB().Exec(
		`INSERT INTO repo_notes (id, repo_id, user_id, content, version, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   repo_id    = excluded.repo_id,
		   content    = excluded.content,
		   version    = excluded.version,
		   updated_at = excluded.updated_at`,
		n.ID, n.RepoID, n.UserID, n.Content, n.Version, updatedAt,
	)
	if err != nil {
		return fmt.Errorf("sqliteclient.RepoNotes.Upsert: %w", err)
	}
	return nil
}

// Delete removes the RepoNote row with the given ID for the user.
func (r *RepoNotes) Delete(userID, id string) error {
	_, err := r.store.DB().Exec(
		`DELETE FROM repo_notes WHERE user_id = ? AND id = ?`,
		userID, id,
	)
	if err != nil {
		return fmt.Errorf("sqliteclient.RepoNotes.Delete: %w", err)
	}
	return nil
}

// PullSince returns notes with Version > since for userID, ordered ASC.
func (r *RepoNotes) PullSince(userID string, since int64, limit int) ([]domain.RepoNote, int64, bool, error) {
	rows, err := r.store.DB().Query(
		`SELECT id, repo_id, user_id, content, version, updated_at
		   FROM repo_notes WHERE user_id = ? AND version > ?
		   ORDER BY version ASC LIMIT ?`,
		userID, since, limit+1,
	)
	if err != nil {
		return nil, since, false, fmt.Errorf("sqliteclient.RepoNotes.PullSince: query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.RepoNote
	for rows.Next() {
		n, err := scanRepoNoteRows(rows)
		if err != nil {
			return nil, since, false, err
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, since, false, fmt.Errorf("sqliteclient.RepoNotes.PullSince: rows: %w", err)
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

func scanRepoNoteCommon(s rowScanner) (domain.RepoNote, error) {
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
			return domain.RepoNote{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, perr)
		}
		n.UpdatedAt = t
	}
	return n, nil
}

func scanRepoNoteRow(row *sql.Row) (domain.RepoNote, error)    { return scanRepoNoteCommon(row) }
func scanRepoNoteRows(rows *sql.Rows) (domain.RepoNote, error) { return scanRepoNoteCommon(rows) }
