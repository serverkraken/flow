package pgstore

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serverkraken/flow/internal/domain"
)

type DayOffStore struct{ pool *pgxpool.Pool }

func NewDayOffStore(pool *pgxpool.Pool) *DayOffStore { return &DayOffStore{pool: pool} }

// Add upserts on (owner_id, day): a second entry for the same day overwrites
// kind/label/target. id is derived from owner+day so re-adds are stable.
func (s *DayOffStore) Add(ctx context.Context, ownerID string, d domain.DayOff) error {
	const q = `
INSERT INTO day_offs (id, owner_id, day, kind, label, target_min)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (owner_id, day) DO UPDATE
SET kind = EXCLUDED.kind, label = EXCLUDED.label, target_min = EXCLUDED.target_min`
	id := ownerID + ":" + d.Date.Format("2006-01-02")
	_, err := s.pool.Exec(ctx, q, id, ownerID, d.Date, string(d.Kind), d.Label, int(d.Target/time.Minute))
	if err != nil {
		return fmt.Errorf("pgstore: add dayoff: %w", err)
	}
	return nil
}

func (s *DayOffStore) Delete(ctx context.Context, ownerID string, day time.Time) error {
	const q = `DELETE FROM day_offs WHERE owner_id=$1 AND day=$2`
	_, err := s.pool.Exec(ctx, q, ownerID, day)
	if err != nil {
		return fmt.Errorf("pgstore: delete dayoff: %w", err)
	}
	return nil
}

func (s *DayOffStore) ListRange(ctx context.Context, ownerID string, from, to time.Time) ([]domain.DayOff, error) {
	const q = `
SELECT day, kind, label, target_min FROM day_offs
WHERE owner_id=$1 AND day >= $2 AND day <= $3
ORDER BY day`
	rows, err := s.pool.Query(ctx, q, ownerID, from, to)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list dayoffs: %w", err)
	}
	defer rows.Close()
	var out []domain.DayOff
	for rows.Next() {
		var (
			day       time.Time
			kind      string
			label     string
			targetMin int
		)
		if err := rows.Scan(&day, &kind, &label, &targetMin); err != nil {
			return nil, fmt.Errorf("pgstore: scan dayoff: %w", err)
		}
		out = append(out, domain.DayOff{
			Date: day, Kind: domain.Kind(kind), Label: label,
			Target: time.Duration(targetMin) * time.Minute,
		})
	}
	return out, rows.Err()
}
