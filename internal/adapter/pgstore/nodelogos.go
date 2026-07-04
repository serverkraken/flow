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

// NodeLogoStore persists uploaded node logos as Postgres blobs (1 per node,
// FK ON DELETE CASCADE cleans up when the node goes).
type NodeLogoStore struct{ pool *pgxpool.Pool }

// NewNodeLogoStore wires the store to a pgx pool.
func NewNodeLogoStore(pool *pgxpool.Pool) *NodeLogoStore { return &NodeLogoStore{pool: pool} }

func (s *NodeLogoStore) Put(ctx context.Context, l domain.NodeLogo) error {
	const q = `
INSERT INTO node_logos (node_id, owner_id, mime, ref, bytes, updated_at, width, height)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (node_id) DO UPDATE SET mime=$3, ref=$4, bytes=$5, updated_at=$6, width=$7, height=$8`
	if _, err := s.pool.Exec(ctx, q, l.NodeID, l.OwnerID, l.Mime, l.Ref, l.Bytes, l.UpdatedAt, l.Width, l.Height); err != nil {
		return fmt.Errorf("pgstore: put node logo: %w", err)
	}
	return nil
}

func (s *NodeLogoStore) Get(ctx context.Context, ownerID, nodeID string) (domain.NodeLogo, error) {
	const q = `SELECT node_id, owner_id, mime, ref, bytes, updated_at, width, height
FROM node_logos WHERE owner_id=$1 AND node_id=$2`
	var l domain.NodeLogo
	err := s.pool.QueryRow(ctx, q, ownerID, nodeID).
		Scan(&l.NodeID, &l.OwnerID, &l.Mime, &l.Ref, &l.Bytes, &l.UpdatedAt, &l.Width, &l.Height)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NodeLogo{}, ports.ErrNodeLogoNotFound
	}
	if err != nil {
		return domain.NodeLogo{}, fmt.Errorf("pgstore: get node logo: %w", err)
	}
	return l, nil
}

func (s *NodeLogoStore) Delete(ctx context.Context, ownerID, nodeID string) error {
	const q = `DELETE FROM node_logos WHERE owner_id=$1 AND node_id=$2`
	if _, err := s.pool.Exec(ctx, q, ownerID, nodeID); err != nil {
		return fmt.Errorf("pgstore: delete node logo: %w", err)
	}
	return nil
}
