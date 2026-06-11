// internal/adapter/pgstore/dayoffs.go
package pgstore

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// DayOffs is the server-side day-off store. The client-side
// ports.DayOffStore has no userID in its signatures, so this adapter gets
// its own server shape; the httpserver handlers depend on it directly.
type DayOffs struct{ store *Store }

func NewDayOffs(s *Store) *DayOffs { return &DayOffs{store: s} }

// List returns the user's day-offs within the given year, ordered by day.
func (d *DayOffs) List(userID string, year int) ([]domain.DayOff, error) {
	from := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(1, 0, 0)
	rows, err := d.store.Pool().Query(context.Background(), `
		SELECT day, kind, label, target_ns FROM day_offs
		WHERE user_id = $1 AND day >= $2 AND day < $3 ORDER BY day ASC`,
		userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.DayOff
	for rows.Next() {
		var off domain.DayOff
		var kind string
		var targetNS int64
		if err := rows.Scan(&off.Date, &kind, &off.Label, &targetNS); err != nil {
			return nil, err
		}
		off.Kind = domain.Kind(kind)
		off.Target = time.Duration(targetNS)
		out = append(out, off)
	}
	return out, rows.Err()
}

// Put upserts the day-off for off.Date (date precision).
func (d *DayOffs) Put(userID string, off domain.DayOff) error {
	_, err := d.store.Pool().Exec(context.Background(), `
		INSERT INTO day_offs (user_id, day, kind, label, target_ns)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, day) DO UPDATE
			SET kind = EXCLUDED.kind, label = EXCLUDED.label, target_ns = EXCLUDED.target_ns`,
		userID, off.Date, string(off.Kind), off.Label, int64(off.Target))
	return err
}

// Delete removes the day-off; idempotent.
func (d *DayOffs) Delete(userID string, day time.Time) error {
	_, err := d.store.Pool().Exec(context.Background(),
		`DELETE FROM day_offs WHERE user_id = $1 AND day = $2`, userID, day)
	return err
}
