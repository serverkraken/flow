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

type NodeStore struct{ pool *pgxpool.Pool }

func NewNodeStore(pool *pgxpool.Pool) *NodeStore { return &NodeStore{pool: pool} }

func (s *NodeStore) Create(ctx context.Context, p domain.Node) (domain.Node, error) {
	const q = `
INSERT INTO nodes (id, owner_id, name, slug, color, glyph, description, upstream_git, status, created_at, updated_at, rate_amount, rate_currency)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
RETURNING id, owner_id, name, slug, color, glyph, description, upstream_git, status, created_at, updated_at, rate_amount, rate_currency`
	ra, rc := rateCols(p.Rate)
	return scanNode(s.pool.QueryRow(ctx, q,
		p.ID, p.OwnerID, p.Name, p.Slug, p.Color, p.Glyph, p.Description, p.UpstreamGit, string(p.Status), p.CreatedAt, p.UpdatedAt, ra, rc))
}

func (s *NodeStore) List(ctx context.Context, ownerID string) ([]domain.Node, error) {
	const q = `
SELECT id, owner_id, name, slug, color, glyph, description, upstream_git, status, created_at, updated_at, rate_amount, rate_currency
FROM nodes WHERE owner_id=$1 ORDER BY name`
	rows, err := s.pool.Query(ctx, q, ownerID)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list projects: %w", err)
	}
	defer rows.Close()
	var out []domain.Node
	for rows.Next() {
		p, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *NodeStore) Get(ctx context.Context, ownerID, id string) (domain.Node, error) {
	const q = `
SELECT id, owner_id, name, slug, color, glyph, description, upstream_git, status, created_at, updated_at, rate_amount, rate_currency
FROM nodes WHERE owner_id=$1 AND id=$2`
	p, err := scanNode(s.pool.QueryRow(ctx, q, ownerID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Node{}, ports.ErrNodeNotFound
	}
	return p, err
}

func (s *NodeStore) Update(ctx context.Context, ownerID string, p domain.Node) (domain.Node, error) {
	const q = `
UPDATE nodes SET name=$1, slug=$2, color=$3, glyph=$4, description=$5, upstream_git=$6, status=$7, updated_at=$8
WHERE owner_id=$9 AND id=$10
RETURNING id, owner_id, name, slug, color, glyph, description, upstream_git, status, created_at, updated_at, rate_amount, rate_currency`
	got, err := scanNode(s.pool.QueryRow(ctx, q,
		p.Name, p.Slug, p.Color, p.Glyph, p.Description, p.UpstreamGit, string(p.Status), p.UpdatedAt, ownerID, p.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Node{}, ports.ErrNodeNotFound
	}
	return got, err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func (s *NodeStore) SetRate(ctx context.Context, ownerID, id string, rate *domain.Money) error {
	ra, rc := rateCols(rate)
	const q = `UPDATE nodes SET rate_amount=$1, rate_currency=$2, updated_at=now() WHERE owner_id=$3 AND id=$4`
	tag, err := s.pool.Exec(ctx, q, ra, rc, ownerID, id)
	if err != nil {
		return fmt.Errorf("pgstore: set rate: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ErrNodeNotFound
	}
	return nil
}

func (s *NodeStore) Delete(ctx context.Context, ownerID, id string) error {
	const q = `DELETE FROM nodes WHERE owner_id=$1 AND id=$2`
	tag, err := s.pool.Exec(ctx, q, ownerID, id)
	if err != nil {
		return fmt.Errorf("pgstore: delete project: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ErrNodeNotFound
	}
	return nil
}

// rateCols maps an optional Money to the two nullable columns (both-or-neither).
func rateCols(m *domain.Money) (*int64, *string) {
	if m == nil {
		return nil, nil
	}
	a, c := m.Amount, m.Currency
	return &a, &c
}

func scanNode(r rowScanner) (domain.Node, error) {
	var p domain.Node
	var status string
	var ra *int64
	var rc *string
	if err := r.Scan(&p.ID, &p.OwnerID, &p.Name, &p.Slug, &p.Color, &p.Glyph, &p.Description, &p.UpstreamGit, &status, &p.CreatedAt, &p.UpdatedAt, &ra, &rc); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Node{}, err
		}
		return domain.Node{}, fmt.Errorf("pgstore: scan project: %w", err)
	}
	p.Status = domain.NodeStatus(status)
	if (ra == nil) != (rc == nil) {
		return domain.Node{}, fmt.Errorf("pgstore: scan project: inconsistent rate columns (amount set=%v currency set=%v)", ra != nil, rc != nil)
	}
	if ra != nil && rc != nil {
		p.Rate = &domain.Money{Amount: *ra, Currency: *rc}
	}
	return p, nil
}
