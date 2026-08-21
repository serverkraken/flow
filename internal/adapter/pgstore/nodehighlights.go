package pgstore

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// NodeHighlightStore is the Postgres-backed ports.NodeHighlightStore.
type NodeHighlightStore struct{ pool *pgxpool.Pool }

// NewNodeHighlightStore wires the store to a pgx pool.
func NewNodeHighlightStore(pool *pgxpool.Pool) *NodeHighlightStore {
	return &NodeHighlightStore{pool: pool}
}

func (s *NodeHighlightStore) Create(ctx context.Context, h domain.NodeHighlight) (domain.NodeHighlight, error) {
	const q = `INSERT INTO node_highlights (id, owner_id, document_id, node_id, quote, created_at)
VALUES ($1,$2,$3,$4,$5,$6)`
	if _, err := s.pool.Exec(ctx, q, h.ID, h.OwnerID, h.DocumentID, h.NodeID, h.Quote, h.CreatedAt); err != nil {
		return domain.NodeHighlight{}, fmt.Errorf("pgstore: create node highlight: %w", err)
	}
	return h, nil
}

func (s *NodeHighlightStore) Delete(ctx context.Context, ownerID, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM node_highlights WHERE owner_id=$1 AND id=$2`, ownerID, id)
	if err != nil {
		return fmt.Errorf("pgstore: delete node highlight: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ErrNodeHighlightNotFound
	}
	return nil
}

func (s *NodeHighlightStore) ListForDocument(ctx context.Context, ownerID, documentID string) ([]domain.NodeHighlight, error) {
	const q = `SELECT id, owner_id, document_id, node_id, quote, created_at
FROM node_highlights WHERE owner_id=$1 AND document_id=$2 ORDER BY created_at ASC`
	rows, err := s.pool.Query(ctx, q, ownerID, documentID)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list node highlights for document: %w", err)
	}
	defer rows.Close()
	return scanNodeHighlights(rows)
}

func (s *NodeHighlightStore) ListSince(ctx context.Context, ownerID string, since time.Time) ([]domain.NodeHighlight, error) {
	const q = `SELECT id, owner_id, document_id, node_id, quote, created_at
FROM node_highlights WHERE owner_id=$1 AND created_at >= $2 ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, q, ownerID, since)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list node highlights since: %w", err)
	}
	defer rows.Close()
	return scanNodeHighlights(rows)
}

func (s *NodeHighlightStore) ListRecent(ctx context.Context, ownerID string, limit int) ([]domain.NodeHighlight, error) {
	const q = `SELECT id, owner_id, document_id, node_id, quote, created_at
FROM node_highlights WHERE owner_id=$1 ORDER BY created_at DESC, id DESC LIMIT $2`
	rows, err := s.pool.Query(ctx, q, ownerID, limit)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list recent node highlights: %w", err)
	}
	defer rows.Close()
	return scanNodeHighlights(rows)
}

func scanNodeHighlights(rows pgx.Rows) ([]domain.NodeHighlight, error) {
	var out []domain.NodeHighlight
	for rows.Next() {
		var h domain.NodeHighlight
		if err := rows.Scan(&h.ID, &h.OwnerID, &h.DocumentID, &h.NodeID, &h.Quote, &h.CreatedAt); err != nil {
			return nil, fmt.Errorf("pgstore: scan node highlight: %w", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
