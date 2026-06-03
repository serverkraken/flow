package sqliteserver

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ActiveSessions is the server-side active-session store. It provides
// optimistic-concurrency Start (insert-or-takeover) and an atomic Stop that,
// in a single transaction, deletes the active_sessions row AND inserts a
// finished sessions row — both bumping the Lamport counter so pull-since
// clients observe both events.
//
// Note: the Start/Stop signature differs from ports.ActiveSessionStore
// (which exposes Upsert/Delete without a version or tag/note parameters),
// so *ActiveSessions does NOT satisfy ports.ActiveSessionStore. HTTP handlers
// in Task 26 call the concrete type directly.
type ActiveSessions struct{ store *Store }

// NewActiveSessions constructs an ActiveSessions sub-adapter backed by store.
func NewActiveSessions(s *Store) *ActiveSessions { return &ActiveSessions{store: s} }

// Start creates or force-takes-over an active_sessions row.
//
// expectedVersion = 0 means "must not exist"; any other value means
// "force-takeover — the caller holds version N, so replace it". Returns
// ErrActiveSessionConflict if the stored version does not match.
func (a *ActiveSessions) Start(userID, projectID, device string, expectedVersion int64) (domain.ActiveSession, error) {
	tx, err := a.store.DB().Begin()
	if err != nil {
		return domain.ActiveSession{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var curVersion int64
	switch err := tx.QueryRow(
		`SELECT version FROM active_sessions WHERE user_id = ? AND project_id = ?`,
		userID, projectID).Scan(&curVersion); {
	case errors.Is(err, sql.ErrNoRows):
		if expectedVersion != 0 {
			return domain.ActiveSession{}, ports.ErrActiveSessionConflict
		}
	case err != nil:
		return domain.ActiveSession{}, err
	default:
		if curVersion != expectedVersion {
			return domain.ActiveSession{}, ports.ErrActiveSessionConflict
		}
	}

	v, err := NextLamport(tx)
	if err != nil {
		return domain.ActiveSession{}, err
	}
	now := time.Now().UTC()
	if _, err := tx.Exec(`
		INSERT INTO active_sessions (user_id, project_id, started_at, started_on_device, version)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, project_id) DO UPDATE SET
			started_at = excluded.started_at,
			started_on_device = excluded.started_on_device,
			version = excluded.version`,
		userID, projectID, now.Format(time.RFC3339), device, v); err != nil {
		return domain.ActiveSession{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.ActiveSession{}, err
	}
	return domain.ActiveSession{
		UserID: userID, ProjectID: projectID,
		StartedAt: now, StartedOnDevice: device, Version: v,
	}, nil
}

// Stop is atomic: in one transaction, deletes the active_sessions row AND
// inserts a finished sessions row spanning [started_at, now). Both operations
// get distinct Lamport versions so pull-since clients see both updates.
//
// Returns ErrActiveSessionConflict if expectedVersion does not match the
// stored row, or ErrActiveSessionNotFound if no row exists.
func (a *ActiveSessions) Stop(userID, projectID string, expectedVersion int64, tag, note string) (domain.Session, error) {
	tx, err := a.store.DB().Begin()
	if err != nil {
		return domain.Session{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var startedAt string
	var curVersion int64
	if err := tx.QueryRow(
		`SELECT started_at, version FROM active_sessions WHERE user_id = ? AND project_id = ?`,
		userID, projectID).Scan(&startedAt, &curVersion); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Session{}, ports.ErrActiveSessionNotFound
		}
		return domain.Session{}, err
	}
	if curVersion != expectedVersion {
		return domain.Session{}, ports.ErrActiveSessionConflict
	}

	start, _ := time.Parse(time.RFC3339, startedAt)
	now := time.Now().UTC()

	sessV, err := NextLamport(tx)
	if err != nil {
		return domain.Session{}, err
	}
	sess := domain.Session{
		ID:        uuid.NewString(),
		UserID:    userID,
		ProjectID: projectID,
		Date:      start.Truncate(24 * time.Hour),
		Start:     start,
		Stop:      now,
		Elapsed:   now.Sub(start),
		Tag:       tag,
		Note:      note,
		Version:   sessV,
		UpdatedAt: now,
	}
	if _, err := tx.Exec(`
		INSERT INTO sessions (id, user_id, project_id, date, start, stop, elapsed_ns, tag, note, version, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.UserID, sess.ProjectID,
		sess.Date.Format("2006-01-02"),
		sess.Start.Format(time.RFC3339),
		sess.Stop.Format(time.RFC3339),
		int64(sess.Elapsed),
		sess.Tag, sess.Note, sess.Version,
		sess.UpdatedAt.Format(time.RFC3339)); err != nil {
		return domain.Session{}, err
	}

	if _, err := tx.Exec(
		`DELETE FROM active_sessions WHERE user_id = ? AND project_id = ?`,
		userID, projectID); err != nil {
		return domain.Session{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Session{}, err
	}
	return sess, nil
}

// ListByUser returns all active sessions for the given user.
func (a *ActiveSessions) ListByUser(userID string) ([]domain.ActiveSession, error) {
	rows, err := a.store.DB().Query(`
		SELECT user_id, project_id, started_at, started_on_device, version
		FROM active_sessions WHERE user_id = ?
		ORDER BY started_at ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []domain.ActiveSession
	for rows.Next() {
		as, err := scanActiveSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, as)
	}
	return out, rows.Err()
}

// Get returns the active session for a specific (user, project) pair.
// Returns ErrActiveSessionNotFound if no row exists.
func (a *ActiveSessions) Get(userID, projectID string) (domain.ActiveSession, error) {
	var as domain.ActiveSession
	var startedAt string
	err := a.store.DB().QueryRow(`
		SELECT user_id, project_id, started_at, started_on_device, version
		FROM active_sessions WHERE user_id = ? AND project_id = ?`,
		userID, projectID).Scan(
		&as.UserID, &as.ProjectID, &startedAt, &as.StartedOnDevice, &as.Version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ActiveSession{}, ports.ErrActiveSessionNotFound
		}
		return domain.ActiveSession{}, err
	}
	as.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	return as, nil
}

// PullSince returns active sessions for userID with version > since, ordered
// by version ASC. The returned int64 is the new high-watermark (highest
// version in the result, or since if no rows).
func (a *ActiveSessions) PullSince(userID string, since int64) ([]domain.ActiveSession, int64, error) {
	rows, err := a.store.DB().Query(`
		SELECT user_id, project_id, started_at, started_on_device, version
		FROM active_sessions WHERE user_id = ? AND version > ?
		ORDER BY version ASC`, userID, since)
	if err != nil {
		return nil, since, err
	}
	defer func() { _ = rows.Close() }()

	var out []domain.ActiveSession
	for rows.Next() {
		as, err := scanActiveSession(rows)
		if err != nil {
			return nil, since, err
		}
		out = append(out, as)
	}
	if err := rows.Err(); err != nil {
		return nil, since, err
	}

	high := since
	if len(out) > 0 {
		high = out[len(out)-1].Version
	}
	return out, high, nil
}

func scanActiveSession(r interface{ Scan(...any) error }) (domain.ActiveSession, error) {
	var as domain.ActiveSession
	var startedAt string
	if err := r.Scan(&as.UserID, &as.ProjectID, &startedAt, &as.StartedOnDevice, &as.Version); err != nil {
		return domain.ActiveSession{}, err
	}
	as.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	return as, nil
}
