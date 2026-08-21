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

// NodeBannerStore persists uploaded node banners as Postgres blobs (1 per
// node, FK ON DELETE CASCADE cleans up when the node goes). Mirrors
// NodeLogoStore.
type NodeBannerStore struct{ pool *pgxpool.Pool }

// NewNodeBannerStore wires the store to a pgx pool.
func NewNodeBannerStore(pool *pgxpool.Pool) *NodeBannerStore { return &NodeBannerStore{pool: pool} }

func (s *NodeBannerStore) Put(ctx context.Context, b domain.NodeBanner) error {
	const q = `
INSERT INTO node_banners (node_id, owner_id, mime, ref, bytes, updated_at)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (node_id) DO UPDATE SET mime=$3, ref=$4, bytes=$5, updated_at=$6
WHERE node_banners.owner_id=$2`
	if _, err := s.pool.Exec(ctx, q, b.NodeID, b.OwnerID, b.Mime, b.Ref, b.Bytes, b.UpdatedAt); err != nil {
		return fmt.Errorf("pgstore: put node banner: %w", err)
	}
	return nil
}

func (s *NodeBannerStore) Get(ctx context.Context, ownerID, nodeID string) (domain.NodeBanner, error) {
	const q = `SELECT node_id, owner_id, mime, ref, bytes, updated_at
FROM node_banners WHERE owner_id=$1 AND node_id=$2`
	var b domain.NodeBanner
	err := s.pool.QueryRow(ctx, q, ownerID, nodeID).
		Scan(&b.NodeID, &b.OwnerID, &b.Mime, &b.Ref, &b.Bytes, &b.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NodeBanner{}, ports.ErrNodeBannerNotFound
	}
	if err != nil {
		return domain.NodeBanner{}, fmt.Errorf("pgstore: get node banner: %w", err)
	}
	return b, nil
}

func (s *NodeBannerStore) Delete(ctx context.Context, ownerID, nodeID string) error {
	const q = `DELETE FROM node_banners WHERE owner_id=$1 AND node_id=$2`
	if _, err := s.pool.Exec(ctx, q, ownerID, nodeID); err != nil {
		return fmt.Errorf("pgstore: delete node banner: %w", err)
	}
	return nil
}
