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

const nodeCols = `id, owner_id, parent_id, kind, name, slug, color, glyph, description, upstream_git, origin_slug, status, rate_amount, rate_currency, extra, created_at, updated_at`

func (s *NodeStore) Create(ctx context.Context, n domain.Node) (domain.Node, error) {
	const q = `
INSERT INTO nodes (` + nodeCols + `)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
RETURNING ` + nodeCols
	ra, rc := rateCols(n.Rate)
	os := nullStr(n.OriginSlug)
	ex := n.Extra
	if ex == nil {
		ex = map[string]any{}
	}
	return scanNode(s.pool.QueryRow(ctx, q,
		n.ID, n.OwnerID, n.ParentID, string(n.Kind), n.Name, n.Slug, n.Color, n.Glyph,
		n.Description, n.UpstreamGit, os, string(n.Status), ra, rc, ex, n.CreatedAt, n.UpdatedAt))
}

func (s *NodeStore) List(ctx context.Context, ownerID string) ([]domain.Node, error) {
	const q = `SELECT ` + nodeCols + ` FROM nodes WHERE owner_id=$1 ORDER BY name`
	rows, err := s.pool.Query(ctx, q, ownerID)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list nodes: %w", err)
	}
	defer rows.Close()
	var out []domain.Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *NodeStore) Get(ctx context.Context, ownerID, id string) (domain.Node, error) {
	const q = `SELECT ` + nodeCols + ` FROM nodes WHERE owner_id=$1 AND id=$2`
	n, err := scanNode(s.pool.QueryRow(ctx, q, ownerID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Node{}, ports.ErrNodeNotFound
	}
	return n, err
}

// Update overwrites mutable metadata (name, slug, color, glyph, description,
// upstream_git, origin_slug, status, extra). It does NOT touch rate or parent_id.
func (s *NodeStore) Update(ctx context.Context, ownerID string, n domain.Node) (domain.Node, error) {
	const q = `
UPDATE nodes SET name=$1, slug=$2, color=$3, glyph=$4, description=$5,
                 upstream_git=$6, origin_slug=$7, status=$8, extra=$9, updated_at=$10
WHERE owner_id=$11 AND id=$12
RETURNING ` + nodeCols
	ex := n.Extra
	if ex == nil {
		ex = map[string]any{}
	}
	got, err := scanNode(s.pool.QueryRow(ctx, q,
		n.Name, n.Slug, n.Color, n.Glyph, n.Description, n.UpstreamGit, nullStr(n.OriginSlug),
		string(n.Status), ex, n.UpdatedAt, ownerID, n.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Node{}, ports.ErrNodeNotFound
	}
	return got, err
}

type rowScanner interface {
	Scan(dest ...any) error
}

// nullStr maps "" → SQL NULL so partial CHECKs (origin_slug only on repo) hold.
func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
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
	var n domain.Node
	var kind, status string
	var parentID, originSlug *string
	var ra *int64
	var rc *string
	var extra map[string]any
	if err := r.Scan(
		&n.ID, &n.OwnerID, &parentID, &kind, &n.Name, &n.Slug, &n.Color, &n.Glyph,
		&n.Description, &n.UpstreamGit, &originSlug, &status, &ra, &rc, &extra,
		&n.CreatedAt, &n.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Node{}, err
		}
		return domain.Node{}, fmt.Errorf("pgstore: scan node: %w", err)
	}
	n.ParentID = parentID
	n.Kind = domain.NodeKind(kind)
	if originSlug != nil {
		n.OriginSlug = *originSlug
	}
	n.Status = domain.NodeStatus(status)
	if (ra == nil) != (rc == nil) {
		return domain.Node{}, fmt.Errorf("pgstore: scan node: inconsistent rate columns (amount set=%v currency set=%v)", ra != nil, rc != nil)
	}
	if ra != nil && rc != nil {
		n.Rate = &domain.Money{Amount: *ra, Currency: *rc}
	}
	n.Extra = extra
	return n, nil
}
