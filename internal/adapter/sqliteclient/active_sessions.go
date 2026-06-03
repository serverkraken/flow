package sqliteclient

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ActiveSessions implements ports.ActiveSessionStore against the SQLite
// active_sessions table.
type ActiveSessions struct {
	store *Store
}

// compile-time interface assertion
var _ ports.ActiveSessionStore = (*ActiveSessions)(nil)

// NewActiveSessions constructs an ActiveSessions sub-adapter backed by store.
func NewActiveSessions(store *Store) *ActiveSessions { return &ActiveSessions{store: store} }

// ListByUser returns all in-progress sessions for the given user.
func (a *ActiveSessions) ListByUser(userID string) ([]domain.ActiveSession, error) {
	rows, err := a.store.DB().Query(
		`SELECT user_id, project_id, started_at, started_on_device, tag, note, version
		   FROM active_sessions
		  WHERE user_id = ?`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("sqliteclient.ActiveSessions.ListByUser: query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []domain.ActiveSession
	for rows.Next() {
		as, err := scanActiveSession(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, as)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqliteclient.ActiveSessions.ListByUser: rows: %w", err)
	}
	return result, nil
}

// Get returns the active session for the given (userID, projectID) pair.
// Returns ports.ErrActiveSessionNotFound when no such session exists.
func (a *ActiveSessions) Get(userID, projectID string) (domain.ActiveSession, error) {
	row := a.store.DB().QueryRow(
		`SELECT user_id, project_id, started_at, started_on_device, tag, note, version
		   FROM active_sessions
		  WHERE user_id = ? AND project_id = ?`,
		userID, projectID,
	)
	var as domain.ActiveSession
	var startedAt string
	err := row.Scan(&as.UserID, &as.ProjectID, &startedAt, &as.StartedOnDevice, &as.Tag, &as.Note, &as.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ActiveSession{}, ports.ErrActiveSessionNotFound
	}
	if err != nil {
		return domain.ActiveSession{}, fmt.Errorf("sqliteclient.ActiveSessions.Get: scan: %w", err)
	}
	t, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return domain.ActiveSession{}, fmt.Errorf("sqliteclient.ActiveSessions.Get: parse started_at %q: %w", startedAt, err)
	}
	as.StartedAt = t
	return as, nil
}

// Upsert inserts or replaces the active session for (userID, projectID).
func (a *ActiveSessions) Upsert(as domain.ActiveSession) error {
	startedAt := as.StartedAt.UTC().Format(time.RFC3339)
	_, err := a.store.DB().Exec(
		`INSERT INTO active_sessions (user_id, project_id, started_at, started_on_device, tag, note, version)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, project_id) DO UPDATE SET
		   started_at        = excluded.started_at,
		   started_on_device = excluded.started_on_device,
		   tag               = excluded.tag,
		   note              = excluded.note,
		   version           = excluded.version`,
		as.UserID, as.ProjectID, startedAt, as.StartedOnDevice, as.Tag, as.Note, as.Version,
	)
	if err != nil {
		return fmt.Errorf("sqliteclient.ActiveSessions.Upsert: %w", err)
	}
	return nil
}

// Delete removes the active session for the given (userID, projectID) pair.
func (a *ActiveSessions) Delete(userID, projectID string) error {
	_, err := a.store.DB().Exec(
		`DELETE FROM active_sessions WHERE user_id = ? AND project_id = ?`,
		userID, projectID,
	)
	if err != nil {
		return fmt.Errorf("sqliteclient.ActiveSessions.Delete: %w", err)
	}
	return nil
}

func scanActiveSession(rows *sql.Rows) (domain.ActiveSession, error) {
	var as domain.ActiveSession
	var startedAt string
	err := rows.Scan(&as.UserID, &as.ProjectID, &startedAt, &as.StartedOnDevice, &as.Tag, &as.Note, &as.Version)
	if err != nil {
		return domain.ActiveSession{}, fmt.Errorf("sqliteclient.ActiveSessions: scan: %w", err)
	}
	t, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return domain.ActiveSession{}, fmt.Errorf("sqliteclient.ActiveSessions: parse started_at %q: %w", startedAt, err)
	}
	as.StartedAt = t
	return as, nil
}
