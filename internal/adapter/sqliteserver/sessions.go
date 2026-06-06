package sqliteserver

import (
	"database/sql"
	"errors"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Sessions is the server-side session store. It provides version-based
// PullSince and optimistic-concurrency Upsert (takes expectedVersion,
// bumps version via NextLamport).
//
// Note: the server Upsert signature differs from ports.SessionStore.Upsert
// (which takes no expectedVersion), so *Sessions does NOT satisfy
// ports.SessionStore. HTTP handlers call the concrete type directly.
type Sessions struct{ store *Store }

// NewSessions constructs a Sessions sub-adapter backed by store.
func NewSessions(s *Store) *Sessions { return &Sessions{store: s} }

// PullSince returns rows with version > since for userID, ordered by version ASC.
// hasMore is true when the result was truncated to limit rows.
func (s *Sessions) PullSince(userID string, since int64, limit int) ([]domain.Session, int64, bool, error) {
	rows, err := s.store.DB().Query(`
		SELECT id, user_id, project_id, date, start, stop, elapsed_ns, tag, note, version, updated_at
		FROM sessions WHERE user_id = ? AND version > ?
		ORDER BY version ASC LIMIT ?`, userID, since, limit+1)
	if err != nil {
		return nil, since, false, err
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Session
	for rows.Next() {
		ss, err := scanServerSession(rows)
		if err != nil {
			return nil, since, false, err
		}
		out = append(out, ss)
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

// Upsert applies the row with optimistic concurrency: if a row with the
// same id exists and stored.version != expectedVersion → 409. Else inserts
// or updates, bumping version via NextLamport.
func (s *Sessions) Upsert(in domain.Session, expectedVersion int64) (domain.Session, error) {
	tx, err := s.store.DB().Begin()
	if err != nil {
		return domain.Session{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var curVersion int64
	row := tx.QueryRow(`SELECT version FROM sessions WHERE id = ?`, in.ID)
	switch err := row.Scan(&curVersion); {
	case errors.Is(err, sql.ErrNoRows):
		if expectedVersion != 0 {
			return domain.Session{}, ports.ErrSessionVersionConflict
		}
	case err != nil:
		return domain.Session{}, err
	default:
		if curVersion != expectedVersion {
			return domain.Session{}, ports.ErrSessionVersionConflict
		}
	}

	v, err := NextLamport(tx)
	if err != nil {
		return domain.Session{}, err
	}
	now := time.Now().UTC()
	if _, err := tx.Exec(
		`
		INSERT INTO sessions (id, user_id, project_id, date, start, stop, elapsed_ns, tag, note, version, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			project_id = excluded.project_id,
			date = excluded.date, start = excluded.start, stop = excluded.stop,
			elapsed_ns = excluded.elapsed_ns, tag = excluded.tag, note = excluded.note,
			version = excluded.version, updated_at = excluded.updated_at`,
		in.ID, in.UserID, in.ProjectID,
		in.Date.Format("2006-01-02"),
		in.Start.Format(time.RFC3339), in.Stop.Format(time.RFC3339),
		int64(in.Elapsed), in.Tag, in.Note, v, now.Format(time.RFC3339),
	); err != nil {
		return domain.Session{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Session{}, err
	}
	in.Version = v
	in.UpdatedAt = now
	return in, nil
}

// ListByUserDateRange returns sessions for userID whose date column falls in
// [from, to] (inclusive), ordered by start ASC. The date column is stored as
// the lexicographically sortable "YYYY-MM-DD" text, so a string range scan is
// safe and avoids parsing every row.
func (s *Sessions) ListByUserDateRange(userID string, from, to time.Time) ([]domain.Session, error) {
	fromStr := from.Format("2006-01-02")
	toStr := to.Format("2006-01-02")
	rows, err := s.store.DB().Query(`
		SELECT id, user_id, project_id, date, start, stop, elapsed_ns, tag, note, version, updated_at
		FROM sessions
		WHERE user_id = ? AND date >= ? AND date <= ?
		ORDER BY start ASC`, userID, fromStr, toStr)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Session
	for rows.Next() {
		ss, err := scanServerSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ss)
	}
	return out, rows.Err()
}

// Delete removes the session with the given ID for userID, with optimistic
// concurrency: if the stored row's version differs from expectedVersion,
// returns ports.ErrSessionVersionConflict. If no row exists for the
// (userID, id) pair, returns ports.ErrSessionNotFound.
//
// User-isolation: a row owned by a different user behaves as "not found"
// for the caller — the cross-tenant existence is not leaked.
func (s *Sessions) Delete(userID, id string, expectedVersion int64) error {
	tx, err := s.store.DB().Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var curVersion int64
	row := tx.QueryRow(`SELECT version FROM sessions WHERE user_id = ? AND id = ?`, userID, id)
	switch err := row.Scan(&curVersion); {
	case errors.Is(err, sql.ErrNoRows):
		return ports.ErrSessionNotFound
	case err != nil:
		return err
	}
	if curVersion != expectedVersion {
		return ports.ErrSessionVersionConflict
	}
	if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ? AND id = ?`, userID, id); err != nil {
		return err
	}
	return tx.Commit()
}

// GetByID returns the session with the given ID scoped to userID.
// Returns ports.ErrSessionNotFound when no row exists.
func (s *Sessions) GetByID(userID, id string) (domain.Session, error) {
	row := s.store.DB().QueryRow(`
		SELECT id, user_id, project_id, date, start, stop, elapsed_ns, tag, note, version, updated_at
		FROM sessions WHERE user_id = ? AND id = ?`, userID, id)
	var sess domain.Session
	var dateStr, startStr, stopStr, updStr string
	var elapsedNs int64
	err := row.Scan(&sess.ID, &sess.UserID, &sess.ProjectID, &dateStr, &startStr, &stopStr, &elapsedNs, &sess.Tag, &sess.Note, &sess.Version, &updStr)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Session{}, ports.ErrSessionNotFound
	}
	if err != nil {
		return domain.Session{}, err
	}
	sess.Date, _ = time.Parse("2006-01-02", dateStr)
	sess.Start, _ = time.Parse(time.RFC3339, startStr)
	sess.Stop, _ = time.Parse(time.RFC3339, stopStr)
	sess.UpdatedAt, _ = time.Parse(time.RFC3339, updStr)
	sess.Elapsed = time.Duration(elapsedNs)
	return sess, nil
}

func scanServerSession(r interface{ Scan(...any) error }) (domain.Session, error) {
	var s domain.Session
	var dateStr, startStr, stopStr, updStr string
	var elapsedNs int64
	if err := r.Scan(&s.ID, &s.UserID, &s.ProjectID, &dateStr, &startStr, &stopStr, &elapsedNs, &s.Tag, &s.Note, &s.Version, &updStr); err != nil {
		return domain.Session{}, err
	}
	s.Date, _ = time.Parse("2006-01-02", dateStr)
	s.Start, _ = time.Parse(time.RFC3339, startStr)
	s.Stop, _ = time.Parse(time.RFC3339, stopStr)
	s.UpdatedAt, _ = time.Parse(time.RFC3339, updStr)
	s.Elapsed = time.Duration(elapsedNs)
	return s, nil
}
