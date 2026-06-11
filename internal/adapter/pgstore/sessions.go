package pgstore

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Sessions mirrors the sqliteserver.Sessions surface (minus PullSince) on PG.
type Sessions struct{ store *Store }

// NewSessions creates a new Sessions adapter.
func NewSessions(s *Store) *Sessions { return &Sessions{store: s} }

const sessionCols = `id, user_id, project_id, day, started_at, stopped_at, tag, note, version, updated_at`

// BookingDay maps a wall-clock instant to the user's booking day
// (Spec §6: day is computed from started_at in the user's timezone).
func BookingDay(startedAt time.Time, loc *time.Location) time.Time {
	local := startedAt.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

// ListByUserDateRange returns sessions whose day lies in [from, to]
// (both inclusive — the WebUI queries single days as from==to).
func (s *Sessions) ListByUserDateRange(userID string, from, to time.Time) ([]domain.Session, error) {
	rows, err := s.store.Pool().Query(context.Background(),
		`SELECT `+sessionCols+` FROM sessions
		 WHERE user_id = $1 AND day >= $2 AND day <= $3
		 ORDER BY day ASC, started_at ASC`,
		userID, dateOnly(from), dateOnly(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// GetByID retrieves a session by ID for a user.
func (s *Sessions) GetByID(userID, id string) (domain.Session, error) {
	row := s.store.Pool().QueryRow(context.Background(),
		`SELECT `+sessionCols+` FROM sessions WHERE user_id = $1 AND id = $2`, userID, id)
	sess, err := scanSession(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Session{}, ports.ErrSessionNotFound
	}
	return sess, err
}

// Upsert writes with OCC (expectedVersion 0 = insert-only) and returns the
// saved row. The day column comes from in.Date — callers (stop handler,
// manual create) have already computed the booking day via BookingDay.
func (s *Sessions) Upsert(in domain.Session, expectedVersion int64) (domain.Session, error) {
	ctx := context.Background()
	if expectedVersion == 0 {
		row := s.store.Pool().QueryRow(ctx, `
			INSERT INTO sessions (id, user_id, project_id, day, started_at, stopped_at, tag, note, version, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1, now())
			ON CONFLICT (id) DO NOTHING
			RETURNING `+sessionCols,
			in.ID, in.UserID, in.ProjectID, dateOnly(in.Date), in.Start, in.Stop, in.Tag, in.Note)
		out, err := scanSession(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Session{}, ports.ErrSessionVersionConflict
		}
		return out, err
	}
	row := s.store.Pool().QueryRow(ctx, `
		UPDATE sessions
		SET project_id = $3, day = $4, started_at = $5, stopped_at = $6,
		    tag = $7, note = $8, version = version + 1, updated_at = now()
		WHERE user_id = $1 AND id = $2 AND version = $9
		RETURNING `+sessionCols,
		in.UserID, in.ID, in.ProjectID, dateOnly(in.Date), in.Start, in.Stop,
		in.Tag, in.Note, expectedVersion)
	out, err := scanSession(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Session{}, ports.ErrSessionVersionConflict
	}
	return out, err
}

// BulkUpsert is the idempotent import path (Spec §7 sessions:bulk): rows
// whose ID already exists are skipped, never overwritten — re-running an
// import must not clobber server-side edits.
func (s *Sessions) BulkUpsert(sessions []domain.Session) error {
	ctx := context.Background()
	tx, err := s.store.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, in := range sessions {
		if _, err := tx.Exec(ctx, `
			INSERT INTO sessions (id, user_id, project_id, day, started_at, stopped_at, tag, note, version, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1, now())
			ON CONFLICT (id) DO NOTHING`,
			in.ID, in.UserID, in.ProjectID, dateOnly(in.Date), in.Start, in.Stop, in.Tag, in.Note); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Delete removes a session with OCC. Matches sqliteserver semantics:
// missing row → ErrSessionNotFound, version mismatch → conflict.
func (s *Sessions) Delete(userID, id string, expectedVersion int64) error {
	ctx := context.Background()
	tag, err := s.store.Pool().Exec(ctx,
		`DELETE FROM sessions WHERE user_id = $1 AND id = $2 AND version = $3`,
		userID, id, expectedVersion)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, gerr := s.GetByID(userID, id); errors.Is(gerr, ports.ErrSessionNotFound) {
			return ports.ErrSessionNotFound
		}
		return ports.ErrSessionVersionConflict
	}
	return nil
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func scanSession(r rowScanner) (domain.Session, error) {
	var out domain.Session
	if err := r.Scan(&out.ID, &out.UserID, &out.ProjectID, &out.Date,
		&out.Start, &out.Stop, &out.Tag, &out.Note, &out.Version, &out.UpdatedAt); err != nil {
		return domain.Session{}, err
	}
	out.Elapsed = out.Stop.Sub(out.Start)
	return out, nil
}
