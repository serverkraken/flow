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

type FeedTokenStore struct{ pool *pgxpool.Pool }

func NewFeedTokenStore(pool *pgxpool.Pool) *FeedTokenStore { return &FeedTokenStore{pool: pool} }

func (s *FeedTokenStore) Create(ctx context.Context, ft domain.FeedToken) error {
	const q = `INSERT INTO feed_tokens (token, user_id, kind, created_at) VALUES ($1,$2,$3,$4)`
	if _, err := s.pool.Exec(ctx, q, ft.Token, ft.UserID, ft.Kind, ft.CreatedAt); err != nil {
		return fmt.Errorf("pgstore: create feed token: %w", err)
	}
	return nil
}

// Resolve returns the owner of an active (non-revoked) token, or
// ErrFeedTokenNotFound for unknown/revoked tokens (no existence leak).
func (s *FeedTokenStore) Resolve(ctx context.Context, token string) (string, error) {
	const q = `SELECT user_id FROM feed_tokens WHERE token=$1 AND revoked_at IS NULL`
	var owner string
	err := s.pool.QueryRow(ctx, q, token).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ports.ErrFeedTokenNotFound
	}
	if err != nil {
		return "", fmt.Errorf("pgstore: resolve feed token: %w", err)
	}
	return owner, nil
}

func (s *FeedTokenStore) ListByUser(ctx context.Context, userID string) ([]domain.FeedToken, error) {
	const q = `SELECT token, kind, created_at FROM feed_tokens WHERE user_id=$1 AND revoked_at IS NULL ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list feed tokens: %w", err)
	}
	defer rows.Close()
	var out []domain.FeedToken
	for rows.Next() {
		ft := domain.FeedToken{UserID: userID}
		if err := rows.Scan(&ft.Token, &ft.Kind, &ft.CreatedAt); err != nil {
			return nil, fmt.Errorf("pgstore: scan feed token: %w", err)
		}
		out = append(out, ft)
	}
	return out, rows.Err()
}

// Revoke marks the token revoked. Idempotent and owner-scoped.
func (s *FeedTokenStore) Revoke(ctx context.Context, userID, token string) error {
	const q = `UPDATE feed_tokens SET revoked_at = now() WHERE user_id=$1 AND token=$2 AND revoked_at IS NULL`
	if _, err := s.pool.Exec(ctx, q, userID, token); err != nil {
		return fmt.Errorf("pgstore: revoke feed token: %w", err)
	}
	return nil
}
