package pgstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serverkraken/flow/internal/domain"
)

type UserSettingsStore struct{ pool *pgxpool.Pool }

func NewUserSettingsStore(pool *pgxpool.Pool) *UserSettingsStore {
	return &UserSettingsStore{pool: pool}
}

// Get returns the stored settings or a lazy default (Bundesland "NW") when the
// user never saved any. It does not write the default row — SetBundesland does.
func (s *UserSettingsStore) Get(ctx context.Context, userID string) (domain.Settings, error) {
	const q = `SELECT bundesland FROM user_settings WHERE user_id=$1`
	var land string
	err := s.pool.QueryRow(ctx, q, userID).Scan(&land)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Settings{UserID: userID, Bundesland: "NW"}, nil
	}
	if err != nil {
		return domain.Settings{}, fmt.Errorf("pgstore: get settings: %w", err)
	}
	return domain.Settings{UserID: userID, Bundesland: land}, nil
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
