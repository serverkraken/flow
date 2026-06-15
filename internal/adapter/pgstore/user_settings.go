package pgstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serverkraken/flow/internal/domain"
)

type UserSettingsStore struct{ pool *pgxpool.Pool }

func NewUserSettingsStore(pool *pgxpool.Pool) *UserSettingsStore {
	return &UserSettingsStore{pool: pool}
}

func (s *UserSettingsStore) Get(ctx context.Context, userID string) (domain.Settings, error) {
	const q = `
SELECT bundesland, default_target_min,
       target_sun_min, target_mon_min, target_tue_min, target_wed_min,
       target_thu_min, target_fri_min, target_sat_min
FROM user_settings WHERE user_id=$1`
	var land string
	var def int
	var wd [7]*int
	err := s.pool.QueryRow(ctx, q, userID).Scan(&land, &def,
		&wd[0], &wd[1], &wd[2], &wd[3], &wd[4], &wd[5], &wd[6])
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Settings{UserID: userID, Bundesland: "NW",
			DefaultTargetMin: domain.DefaultDailyTargetMin,
			WeekdayTargetMin: map[time.Weekday]int{}}, nil
	}
	if err != nil {
		return domain.Settings{}, fmt.Errorf("pgstore: get settings: %w", err)
	}
	overrides := map[time.Weekday]int{}
	for i, p := range wd {
		if p != nil {
			overrides[time.Weekday(i)] = *p
		}
	}
	return domain.Settings{UserID: userID, Bundesland: land,
		DefaultTargetMin: def, WeekdayTargetMin: overrides}, nil
}

func (s *UserSettingsStore) SetBundesland(ctx context.Context, userID, land string) error {
	const q = `
INSERT INTO user_settings (user_id, bundesland, updated_at)
VALUES ($1,$2, now())
ON CONFLICT (user_id) DO UPDATE SET bundesland = EXCLUDED.bundesland, updated_at = now()`
	if _, err := s.pool.Exec(ctx, q, userID, land); err != nil {
		return fmt.Errorf("pgstore: set bundesland: %w", err)
	}
	return nil
}

func (s *UserSettingsStore) SetTargetConfig(ctx context.Context, userID string, defaultMin int, weekday map[time.Weekday]int) error {
	var wd [7]*int
	for d, v := range weekday {
		if d > time.Saturday {
			continue
		}
		vv := v
		wd[int(d)] = &vv
	}
	const q = `
INSERT INTO user_settings
  (user_id, default_target_min,
   target_sun_min, target_mon_min, target_tue_min, target_wed_min,
   target_thu_min, target_fri_min, target_sat_min, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9, now())
ON CONFLICT (user_id) DO UPDATE SET
  default_target_min = EXCLUDED.default_target_min,
  target_sun_min = EXCLUDED.target_sun_min,
  target_mon_min = EXCLUDED.target_mon_min,
  target_tue_min = EXCLUDED.target_tue_min,
  target_wed_min = EXCLUDED.target_wed_min,
  target_thu_min = EXCLUDED.target_thu_min,
  target_fri_min = EXCLUDED.target_fri_min,
  target_sat_min = EXCLUDED.target_sat_min,
  updated_at = now()`
	if _, err := s.pool.Exec(ctx, q, userID, defaultMin,
		wd[0], wd[1], wd[2], wd[3], wd[4], wd[5], wd[6]); err != nil {
		return fmt.Errorf("pgstore: set target config: %w", err)
	}
	return nil
}
