package pgstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type UserStore struct{ pool *pgxpool.Pool }

func NewUserStore(pool *pgxpool.Pool) *UserStore { return &UserStore{pool: pool} }

func (s *UserStore) UpsertBySub(ctx context.Context, u domain.User) (domain.User, error) {
	const q = `
INSERT INTO users (id, oidc_sub, username, email, display_name)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (oidc_sub) DO UPDATE
  SET username=EXCLUDED.username, email=EXCLUDED.email, display_name=EXCLUDED.display_name
RETURNING id, oidc_sub, username, email, display_name`
	var out domain.User
	err := s.pool.QueryRow(ctx, q, u.ID, u.OIDCSub, u.Username, u.Email, u.DisplayName).
		Scan(&out.ID, &out.OIDCSub, &out.Username, &out.Email, &out.DisplayName)
	if err != nil {
		return domain.User{}, fmt.Errorf("pgstore: upsert user: %w", err)
	}
	return out, nil
}

func (s *UserStore) GetBySub(ctx context.Context, sub string) (domain.User, error) {
	const q = `SELECT id, oidc_sub, username, email, display_name FROM users WHERE oidc_sub=$1`
	var out domain.User
	err := s.pool.QueryRow(ctx, q, sub).Scan(&out.ID, &out.OIDCSub, &out.Username, &out.Email, &out.DisplayName)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, ports.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("pgstore: get user: %w", err)
	}
	return out, nil
}

func (s *UserStore) GetByID(ctx context.Context, id string) (domain.User, error) {
	const q = `SELECT id, oidc_sub, username, email, display_name FROM users WHERE id=$1`
	var out domain.User
	err := s.pool.QueryRow(ctx, q, id).Scan(&out.ID, &out.OIDCSub, &out.Username, &out.Email, &out.DisplayName)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, ports.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("pgstore: get user by id: %w", err)
	}
	return out, nil
}
