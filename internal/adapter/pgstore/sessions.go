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

// sessCols is the work_sessions column list. Tags are NOT a column: they live
// in the taggings junction and are read-hydrated (see hydrateTags). The legacy
// `tag` column is dropped from all SQL here and DROPped from the schema in F2.
const sessCols = `id, owner_id, node_id, note, start_at, stop_at, created_at`

func (s *SessionStore) Create(ctx context.Context, ws domain.WorkSession) (domain.WorkSession, error) {
	const q = `
INSERT INTO work_sessions (id, owner_id, node_id, note, start_at, stop_at, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7)
RETURNING ` + sessCols
	return scanSession(s.pool.QueryRow(ctx, q,
		ws.ID, ws.OwnerID, ws.NodeID, ws.Note, ws.Start, ws.Stop, ws.CreatedAt))
}

func (s *SessionStore) Running(ctx context.Context, ownerID string) (domain.WorkSession, bool, error) {
	const q = `SELECT ` + sessCols + `
FROM work_sessions WHERE owner_id=$1 AND stop_at IS NULL`
	ws, err := scanSession(s.pool.QueryRow(ctx, q, ownerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.WorkSession{}, false, nil
	}
	if err != nil {
		return domain.WorkSession{}, false, err
	}
	hyd, err := s.hydrateTags(ctx, ownerID, []domain.WorkSession{ws})
	if err != nil {
		return domain.WorkSession{}, false, err
	}
	return hyd[0], true, nil
}

func (s *SessionStore) Get(ctx context.Context, ownerID, id string) (domain.WorkSession, error) {
	const q = `SELECT ` + sessCols + `
FROM work_sessions WHERE owner_id=$1 AND id=$2`
	ws, err := scanSession(s.pool.QueryRow(ctx, q, ownerID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.WorkSession{}, ports.ErrSessionNotFound
	}
	if err != nil {
		return domain.WorkSession{}, err
	}
	hyd, err := s.hydrateTags(ctx, ownerID, []domain.WorkSession{ws})
	if err != nil {
		return domain.WorkSession{}, err
	}
	return hyd[0], nil
}

func (s *SessionStore) Stop(ctx context.Context, ownerID, id string, nodeID *string, stop time.Time) (domain.WorkSession, error) {
	const q = `
UPDATE work_sessions SET stop_at=$1, node_id=$2
WHERE owner_id=$3 AND id=$4 AND stop_at IS NULL
RETURNING ` + sessCols
	ws, err := scanSession(s.pool.QueryRow(ctx, q, stop, nodeID, ownerID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.WorkSession{}, ports.ErrSessionNotFound
	}
	return ws, err
}

func (s *SessionStore) Update(ctx context.Context, ownerID, id string, nodeID *string, note string, start time.Time, stop *time.Time) (domain.WorkSession, error) {
	const q = `
UPDATE work_sessions SET node_id=$1, note=$2, start_at=$3, stop_at=$4
WHERE owner_id=$5 AND id=$6
RETURNING ` + sessCols
	ws, err := scanSession(s.pool.QueryRow(ctx, q, nodeID, note, start, stop, ownerID, id))
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
	const q = `SELECT ` + sessCols + `
FROM work_sessions WHERE owner_id=$1 AND start_at >= $2
ORDER BY start_at DESC`
	rows, err := s.pool.Query(ctx, q, ownerID, since)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list sessions: %w", err)
	}
	out, err := scanSessions(rows)
	if err != nil {
		return nil, err
	}
	return s.hydrateTags(ctx, ownerID, out)
}

func (s *SessionStore) ListRange(ctx context.Context, ownerID string, since, until time.Time) ([]domain.WorkSession, error) {
	const q = `SELECT ` + sessCols + `
FROM work_sessions WHERE owner_id=$1 AND start_at >= $2 AND start_at < $3
ORDER BY start_at DESC`
	rows, err := s.pool.Query(ctx, q, ownerID, since, until)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list sessions range: %w", err)
	}
	out, err := scanSessions(rows)
	if err != nil {
		return nil, err
	}
	return s.hydrateTags(ctx, ownerID, out)
}

func (s *SessionStore) ListPage(ctx context.Context, ownerID string, limit, offset int) ([]domain.WorkSession, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM work_sessions WHERE owner_id=$1`, ownerID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("pgstore: count sessions: %w", err)
	}
	const q = `SELECT ` + sessCols + `
FROM work_sessions WHERE owner_id=$1
ORDER BY start_at DESC
LIMIT $2 OFFSET $3`
	rows, err := s.pool.Query(ctx, q, ownerID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("pgstore: list sessions page: %w", err)
	}
	out, err := scanSessions(rows)
	if err != nil {
		return nil, 0, err
	}
	out, err = s.hydrateTags(ctx, ownerID, out)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// hydrateTags fills each session's Tags from the taggings junction in one query.
// Mirrors documents.go's hydrateTags but scoped to taggable_type='work_session'.
func (s *SessionStore) hydrateTags(ctx context.Context, ownerID string, ws []domain.WorkSession) ([]domain.WorkSession, error) {
	if len(ws) == 0 {
		return ws, nil
	}
	ids := make([]string, len(ws))
	for i, w := range ws {
		ids[i] = w.ID
	}
	const q = `SELECT tg.taggable_id, t.slug FROM taggings tg JOIN tags t ON t.id = tg.tag_id ` +
		`WHERE t.owner_id=$1 AND tg.taggable_type='work_session' AND tg.taggable_id = ANY($2) ORDER BY t.slug`
	rows, err := s.pool.Query(ctx, q, ownerID, ids)
	if err != nil {
		return nil, fmt.Errorf("pgstore: hydrate session tags: %w", err)
	}
	defer rows.Close()
	byID := map[string][]string{}
	for rows.Next() {
		var id, slug string
		if err := rows.Scan(&id, &slug); err != nil {
			return nil, err
		}
		byID[id] = append(byID[id], slug)
	}
	for i := range ws {
		ws[i].Tags = byID[ws[i].ID]
	}
	return ws, rows.Err()
}

// TagTimes returns the total tracked minutes per tag for the owner, optionally
// filtered to sessions whose start_at falls in [from, to). Zero value means
// unbounded on that side.
func (s *SessionStore) TagTimes(ctx context.Context, ownerID string, from, to time.Time) ([]domain.TagTime, error) {
	const q = `SELECT t.slug,
  COALESCE(SUM(EXTRACT(EPOCH FROM (COALESCE(ws.stop_at, now()) - ws.start_at)))/60, 0)::int AS minutes
FROM work_sessions ws
JOIN taggings tg ON tg.taggable_type='work_session' AND tg.taggable_id = ws.id
JOIN tags t ON t.id = tg.tag_id
WHERE ws.owner_id=$1 AND ($2::timestamptz IS NULL OR ws.start_at >= $2)
  AND ($3::timestamptz IS NULL OR ws.start_at < $3)
GROUP BY t.slug ORDER BY minutes DESC, t.slug`
	var fromArg, toArg any
	if !from.IsZero() {
		fromArg = from
	}
	if !to.IsZero() {
		toArg = to
	}
	rows, err := s.pool.Query(ctx, q, ownerID, fromArg, toArg)
	if err != nil {
		return nil, fmt.Errorf("pgstore: tag times: %w", err)
	}
	defer rows.Close()
	var out []domain.TagTime
	for rows.Next() {
		var tt domain.TagTime
		if err := rows.Scan(&tt.Tag, &tt.Minutes); err != nil {
			return nil, err
		}
		out = append(out, tt)
	}
	return out, rows.Err()
}

func scanSessions(rows pgx.Rows) ([]domain.WorkSession, error) {
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
	if err := r.Scan(&ws.ID, &ws.OwnerID, &ws.NodeID, &ws.Note, &ws.Start, &ws.Stop, &ws.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.WorkSession{}, err
		}
		return domain.WorkSession{}, fmt.Errorf("pgstore: scan session: %w", err)
	}
	return ws, nil
}
