// internal/adapter/pgstore/active_sessions.go
package pgstore

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ActiveSessions is the server-side worktime statemachine
// (start/stop/pause/resume — Spec §7). Start/Stop keep the sqliteserver
// signatures so WebUI handlers swap without churn; Pause/Resume are new
// and idempotent (no expectedVersion — the server is the only writer of
// pause state and the endpoints are defined idempotent).
type ActiveSessions struct {
	store    *Store
	sessions *Sessions
	settings *Settings
}

func NewActiveSessions(s *Store, sessions *Sessions, settings *Settings) *ActiveSessions {
	return &ActiveSessions{store: s, sessions: sessions, settings: settings}
}

const activeCols = `user_id, project_id, started_at, paused_at, pause_total_ns, started_on_device, tag, note, version`

// Start creates the active row. expectedVersion 0 = "must not exist";
// any existing row for (user, project) is a conflict (Spec §7 → 409).
// startedAt zero value → server time now (Server-Zeit, nie Client-Zeit).
func (a *ActiveSessions) Start(userID, projectID string, startedAt time.Time, device string, expectedVersion int64, tag, note string) (domain.ActiveSession, error) {
	ctx := context.Background()
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	startedAt = startedAt.UTC()
	if expectedVersion != 0 {
		return domain.ActiveSession{}, ports.ErrActiveSessionConflict
	}
	row := a.store.Pool().QueryRow(ctx, `
		INSERT INTO active_sessions (user_id, project_id, started_at, started_on_device, tag, note, version)
		VALUES ($1, $2, $3, $4, $5, $6, 1)
		ON CONFLICT (user_id, project_id) DO NOTHING
		RETURNING `+activeCols,
		userID, projectID, startedAt, device, tag, note)
	out, err := scanActive(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ActiveSession{}, ports.ErrActiveSessionConflict
	}
	return out, err
}

// Stop atomically converts the active row into a finished session.
// elapsed = now − started_at − pause_total (eine offene Pause endet mit
// dem Stop, Spec §7). Booking day: started_at in the user's timezone.
// Empty tag/note keep the stored values.
func (a *ActiveSessions) Stop(userID, projectID string, expectedVersion int64, tag, note string) (domain.Session, error) {
	ctx := context.Background()
	tx, err := a.store.Pool().Begin(ctx)
	if err != nil {
		return domain.Session{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row := tx.QueryRow(ctx,
		`SELECT `+activeCols+` FROM active_sessions
		 WHERE user_id = $1 AND project_id = $2 FOR UPDATE`, userID, projectID)
	cur, err := scanActive(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Session{}, ports.ErrActiveSessionNotFound
	}
	if err != nil {
		return domain.Session{}, err
	}
	if cur.Version != expectedVersion {
		return domain.Session{}, ports.ErrActiveSessionConflict
	}
	if tag == "" {
		tag = cur.Tag
	}
	if note == "" {
		note = cur.Note
	}

	now := time.Now().UTC()
	elapsed := cur.Elapsed(now)
	stop := cur.StartedAt.Add(cur.PauseTotal).Add(elapsed)
	if cur.PausedAt != nil {
		// Offene Pause zählt als Pause bis zum Stop: stop = jetzt.
		stop = now
	}
	day := BookingDay(cur.StartedAt, a.settings.Location(userID))

	sess := domain.Session{
		ID: uuid.NewString(), UserID: userID, ProjectID: projectID,
		Date: day, Start: cur.StartedAt, Stop: stop, Elapsed: elapsed,
		Tag: tag, Note: note,
	}
	insRow := tx.QueryRow(ctx, `
		INSERT INTO sessions (id, user_id, project_id, day, started_at, stopped_at, tag, note, version, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1, now())
		RETURNING `+sessionCols, sess.ID, sess.UserID, sess.ProjectID, sess.Date, sess.Start, sess.Stop, sess.Tag, sess.Note)
	saved, err := scanSession(insRow)
	if err != nil {
		return domain.Session{}, err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM active_sessions WHERE user_id = $1 AND project_id = $2`,
		userID, projectID); err != nil {
		return domain.Session{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Session{}, err
	}
	saved.Elapsed = elapsed // Wahrheit der Statemachine, nicht stop−start
	return saved, nil
}

// Pause sets paused_at if running; already-paused is a no-op returning the
// current row. Missing row → ErrActiveSessionNotFound.
func (a *ActiveSessions) Pause(userID, projectID string) (domain.ActiveSession, error) {
	ctx := context.Background()
	now := time.Now().UTC()
	_, err := a.store.Pool().Exec(ctx, `
		UPDATE active_sessions SET paused_at = $3, version = version + 1
		WHERE user_id = $1 AND project_id = $2 AND paused_at IS NULL`,
		userID, projectID, now)
	if err != nil {
		return domain.ActiveSession{}, err
	}
	return a.Get(userID, projectID)
}

// Resume folds the open pause into pause_total_ns and clears paused_at;
// not-paused is a no-op returning the current row.
func (a *ActiveSessions) Resume(userID, projectID string) (domain.ActiveSession, error) {
	ctx := context.Background()
	now := time.Now().UTC()
	_, err := a.store.Pool().Exec(ctx, `
		UPDATE active_sessions
		SET pause_total_ns = pause_total_ns
		      + (EXTRACT(EPOCH FROM ($3::timestamptz - paused_at)) * 1e9)::bigint,
		    paused_at = NULL,
		    version = version + 1
		WHERE user_id = $1 AND project_id = $2 AND paused_at IS NOT NULL`,
		userID, projectID, now)
	if err != nil {
		return domain.ActiveSession{}, err
	}
	return a.Get(userID, projectID)
}

func (a *ActiveSessions) ListByUser(userID string) ([]domain.ActiveSession, error) {
	rows, err := a.store.Pool().Query(context.Background(),
		`SELECT `+activeCols+` FROM active_sessions WHERE user_id = $1 ORDER BY started_at ASC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ActiveSession
	for rows.Next() {
		as, err := scanActive(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, as)
	}
	return out, rows.Err()
}

func (a *ActiveSessions) Get(userID, projectID string) (domain.ActiveSession, error) {
	row := a.store.Pool().QueryRow(context.Background(),
		`SELECT `+activeCols+` FROM active_sessions WHERE user_id = $1 AND project_id = $2`,
		userID, projectID)
	out, err := scanActive(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ActiveSession{}, ports.ErrActiveSessionNotFound
	}
	return out, err
}

func scanActive(r rowScanner) (domain.ActiveSession, error) {
	var out domain.ActiveSession
	var pausedAt *time.Time
	var pauseNS int64
	if err := r.Scan(&out.UserID, &out.ProjectID, &out.StartedAt, &pausedAt, &pauseNS,
		&out.StartedOnDevice, &out.Tag, &out.Note, &out.Version); err != nil {
		return domain.ActiveSession{}, err
	}
	out.PausedAt = pausedAt
	out.PauseTotal = time.Duration(pauseNS)
	return out, nil
}
