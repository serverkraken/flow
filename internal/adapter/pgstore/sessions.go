package pgstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type SessionStore struct{ pool *pgxpool.Pool }

func NewSessionStore(pool *pgxpool.Pool) *SessionStore { return &SessionStore{pool: pool} }

func (s *SessionStore) Create(ctx context.Context, ws domain.WorkSession) (domain.WorkSession, error) {
	const q = `
INSERT INTO work_sessions (id, owner_id, project_id, tag, note, start_at, stop_at, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING id, owner_id, project_id, tag, note, start_at, stop_at, created_at`
	return scanSession(s.pool.QueryRow(ctx, q,
		ws.ID, ws.OwnerID, ws.ProjectID, ws.Tag, ws.Note, ws.Start, ws.Stop, ws.CreatedAt))
}

func (s *SessionStore) Running(ctx context.Context, ownerID string) (domain.WorkSession, bool, error) {
	const q = `
SELECT id, owner_id, project_id, tag, note, start_at, stop_at, created_at
FROM work_sessions WHERE owner_id=$1 AND stop_at IS NULL`
	ws, err := scanSession(s.pool.QueryRow(ctx, q, ownerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.WorkSession{}, false, nil
	}
	if err != nil {
		return domain.WorkSession{}, false, err
	}
	return ws, true, nil
}

func (s *SessionStore) Stop(ctx context.Context, ownerID, id string, projectID *string, stop time.Time) (domain.WorkSession, error) {
	const q = `
UPDATE work_sessions SET stop_at=$1, project_id=$2
WHERE owner_id=$3 AND id=$4 AND stop_at IS NULL
RETURNING id, owner_id, project_id, tag, note, start_at, stop_at, created_at`
	ws, err := scanSession(s.pool.QueryRow(ctx, q, stop, projectID, ownerID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.WorkSession{}, ports.ErrSessionNotFound
	}
	return ws, err
}

func (s *SessionStore) Update(ctx context.Context, ownerID, id string, projectID *string, tag, note string, start time.Time, stop *time.Time) (domain.WorkSession, error) {
	const q = `
UPDATE work_sessions SET project_id=$1, tag=$2, note=$3, start_at=$4, stop_at=$5
WHERE owner_id=$6 AND id=$7
RETURNING id, owner_id, project_id, tag, note, start_at, stop_at, created_at`
	ws, err := scanSession(s.pool.QueryRow(ctx, q, projectID, tag, note, start, stop, ownerID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.WorkSession{}, ports.ErrSessionNotFound
	}
	return ws, err
}

func (s *SessionStore) Delete(ctx context.Context, ownerID, id string) error {
	const q = `DELETE FROM work_sessions WHERE owner_id=$1 AND id=$2`
	ct, err := s.pool.Exec(ctx, q, ownerID, id)
	if err != nil {
		return fmt.Errorf("pgstore: delete session: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ports.ErrSessionNotFound
	}
	return nil
}

func (s *SessionStore) List(ctx context.Context, ownerID string, since time.Time) ([]domain.WorkSession, error) {
	const q = `
SELECT id, owner_id, project_id, tag, note, start_at, stop_at, created_at
FROM work_sessions WHERE owner_id=$1 AND start_at >= $2
ORDER BY start_at DESC`
	rows, err := s.pool.Query(ctx, q, ownerID, since)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list sessions: %w", err)
	}
	defer rows.Close()
	var out []domain.WorkSession
	for rows.Next() {
		ws, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ws)
	}
	return out, rows.Err()
}

func scanSession(r rowScanner) (domain.WorkSession, error) {
	var ws domain.WorkSession
	if err := r.Scan(&ws.ID, &ws.OwnerID, &ws.ProjectID, &ws.Tag, &ws.Note, &ws.Start, &ws.Stop, &ws.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.WorkSession{}, err
		}
		return domain.WorkSession{}, fmt.Errorf("pgstore: scan session: %w", err)
	}
	return ws, nil
}
