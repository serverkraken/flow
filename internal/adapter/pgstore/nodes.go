package pgstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type NodeStore struct{ pool *pgxpool.Pool }

func NewNodeStore(pool *pgxpool.Pool) *NodeStore { return &NodeStore{pool: pool} }

const nodeCols = `id, owner_id, parent_id, kind, name, slug, color, glyph, description, upstream_git, origin_slug, status, rate_amount, rate_currency, extra, created_at, updated_at, counts_toward_target, icon, logo_ref`

func (s *NodeStore) Create(ctx context.Context, n domain.Node) (domain.Node, error) {
	const q = `
INSERT INTO nodes (` + nodeCols + `)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
RETURNING ` + nodeCols
	ra, rc := rateCols(n.Rate)
	os := nullStr(n.OriginSlug)
	ex := n.Extra
	if ex == nil {
		ex = map[string]any{}
	}
	got, err := scanNode(s.pool.QueryRow(ctx, q,
		n.ID, n.OwnerID, n.ParentID, string(n.Kind), n.Name, n.Slug, n.Color, n.Glyph,
		n.Description, n.UpstreamGit, os, string(n.Status), ra, rc, ex, n.CreatedAt, n.UpdatedAt, n.CountsTowardTarget, n.Icon, n.LogoRef))
	if err != nil {
		return domain.Node{}, mapSlugConflict(err)
	}
	return got, nil
}

// mapSlugConflict turns a Postgres unique-violation (23505) on a node slug index
// into the friendly ports.ErrNodeSlugTaken; other errors pass through unchanged.
func mapSlugConflict(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ports.ErrNodeSlugTaken
	}
	return err
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
// upstream_git, origin_slug, status, extra, counts_toward_target, icon,
// logo_ref). It does NOT touch rate or parent_id.
func (s *NodeStore) Update(ctx context.Context, ownerID string, n domain.Node) (domain.Node, error) {
	const q = `
UPDATE nodes SET name=$1, slug=$2, color=$3, glyph=$4, description=$5,
                 upstream_git=$6, origin_slug=$7, status=$8, extra=$9, counts_toward_target=$10,
                 icon=$11, logo_ref=$12, updated_at=$13
WHERE owner_id=$14 AND id=$15
RETURNING ` + nodeCols
	ex := n.Extra
	if ex == nil {
		ex = map[string]any{}
	}
	got, err := scanNode(s.pool.QueryRow(ctx, q,
		n.Name, n.Slug, n.Color, n.Glyph, n.Description, n.UpstreamGit, nullStr(n.OriginSlug),
		string(n.Status), ex, n.CountsTowardTarget, n.Icon, n.LogoRef, n.UpdatedAt, ownerID, n.ID))
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
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pgstore: begin delete node: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the owner-scoped node before checking dependent project documents.
	// A concurrent document FK insert must finish before this lock is acquired,
	// or wait until the delete commits and then fail its FK check. This closes
	// the check/delete race without changing the schema.
	var lockedID string
	if err := tx.QueryRow(ctx, `SELECT id FROM nodes WHERE owner_id=$1 AND id=$2 FOR UPDATE`, ownerID, id).Scan(&lockedID); errors.Is(err, pgx.ErrNoRows) {
		return ports.ErrNodeNotFound
	} else if err != nil {
		return fmt.Errorf("pgstore: lock node for delete: %w", err)
	}

	var hasProjectDocuments bool
	// Deliberately check all owners: the FK targets the globally unique node ID,
	// so even a legacy/corrupt foreign-owner reference would otherwise be
	// mutated by ON DELETE SET NULL. The caller learns only that deletion is
	// blocked, never which owner or document caused it.
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM documents WHERE node_id=$1 AND type=$2)`, id, string(domain.DocProject)).Scan(&hasProjectDocuments); err != nil {
		return fmt.Errorf("pgstore: check node project documents: %w", err)
	}
	if hasProjectDocuments {
		return ports.ErrNodeHasProjectDocuments
	}

	_, err = tx.Exec(ctx, `DELETE FROM nodes WHERE owner_id=$1 AND id=$2`, ownerID, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ports.ErrNodeHasChildren
		}
		return fmt.Errorf("pgstore: delete node: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("pgstore: commit delete node: %w", err)
	}
	return nil
}

func (s *NodeStore) Children(ctx context.Context, ownerID string, parentID *string) ([]domain.Node, error) {
	var rows pgx.Rows
	var err error
	if parentID == nil {
		rows, err = s.pool.Query(ctx, `SELECT `+nodeCols+` FROM nodes WHERE owner_id=$1 AND parent_id IS NULL ORDER BY name`, ownerID)
	} else {
		rows, err = s.pool.Query(ctx, `SELECT `+nodeCols+` FROM nodes WHERE owner_id=$1 AND parent_id=$2 ORDER BY name`, ownerID, *parentID)
	}
	if err != nil {
		return nil, fmt.Errorf("pgstore: children: %w", err)
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

// Ancestors returns the node and its ancestors ordered leaf→root.
func (s *NodeStore) Ancestors(ctx context.Context, ownerID, nodeID string) ([]domain.Node, error) {
	const q = `
WITH RECURSIVE chain AS (
  SELECT id, owner_id, parent_id, kind, name, slug, color, glyph, description, upstream_git, origin_slug, status, rate_amount, rate_currency, extra, created_at, updated_at, counts_toward_target, icon, logo_ref, 0 AS depth, ARRAY[id] AS path
  FROM nodes WHERE owner_id=$1 AND id=$2
  UNION ALL
  SELECT n.id, n.owner_id, n.parent_id, n.kind, n.name, n.slug, n.color, n.glyph, n.description, n.upstream_git, n.origin_slug, n.status, n.rate_amount, n.rate_currency, n.extra, n.created_at, n.updated_at, n.counts_toward_target, n.icon, n.logo_ref, c.depth+1, c.path || n.id
  FROM nodes n JOIN chain c ON n.id = c.parent_id
  WHERE n.owner_id=$1 AND NOT n.id = ANY(c.path)
)
SELECT id, owner_id, parent_id, kind, name, slug, color, glyph, description, upstream_git, origin_slug, status, rate_amount, rate_currency, extra, created_at, updated_at, counts_toward_target, icon, logo_ref
FROM chain ORDER BY depth`
	rows, err := s.pool.Query(ctx, q, ownerID, nodeID)
	if err != nil {
		return nil, fmt.Errorf("pgstore: ancestors: %w", err)
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

// Subtree returns the node itself and all its descendants (root→leaf order).
func (s *NodeStore) Subtree(ctx context.Context, ownerID, nodeID string) ([]domain.Node, error) {
	const q = `
WITH RECURSIVE sub AS (
  SELECT id, owner_id, parent_id, kind, name, slug, color, glyph, description, upstream_git, origin_slug, status, rate_amount, rate_currency, extra, created_at, updated_at, counts_toward_target, icon, logo_ref, 0 AS depth, ARRAY[id] AS path
  FROM nodes WHERE owner_id=$1 AND id=$2
  UNION ALL
  SELECT n.id, n.owner_id, n.parent_id, n.kind, n.name, n.slug, n.color, n.glyph, n.description, n.upstream_git, n.origin_slug, n.status, n.rate_amount, n.rate_currency, n.extra, n.created_at, n.updated_at, n.counts_toward_target, n.icon, n.logo_ref, s.depth+1, s.path || n.id
  FROM nodes n JOIN sub s ON n.parent_id = s.id
  WHERE n.owner_id=$1 AND NOT n.id = ANY(s.path)
)
SELECT id, owner_id, parent_id, kind, name, slug, color, glyph, description, upstream_git, origin_slug, status, rate_amount, rate_currency, extra, created_at, updated_at, counts_toward_target, icon, logo_ref
FROM sub ORDER BY depth`
	rows, err := s.pool.Query(ctx, q, ownerID, nodeID)
	if err != nil {
		return nil, fmt.Errorf("pgstore: subtree: %w", err)
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

func (s *NodeStore) Reparent(ctx context.Context, ownerID, id string, parentID *string) (domain.Node, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Node{}, fmt.Errorf("pgstore: reparent begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Serialize hierarchy mutations per tenant. The usecase's pre-check remains
	// useful for fast feedback, while this lock closes the concurrent-move gap.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, ownerID); err != nil {
		return domain.Node{}, fmt.Errorf("pgstore: lock node hierarchy: %w", err)
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nodes WHERE owner_id=$1 AND id=$2)`, ownerID, id).Scan(&exists); err != nil {
		return domain.Node{}, fmt.Errorf("pgstore: find moved node: %w", err)
	}
	if !exists {
		return domain.Node{}, ports.ErrNodeNotFound
	}
	if parentID != nil {
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nodes WHERE owner_id=$1 AND id=$2)`, ownerID, *parentID).Scan(&exists); err != nil {
			return domain.Node{}, fmt.Errorf("pgstore: find new parent: %w", err)
		}
		if !exists {
			return domain.Node{}, ports.ErrNodeNotFound
		}
		const cycleQ = `
WITH RECURSIVE chain AS (
  SELECT id, parent_id, ARRAY[id] AS path
  FROM nodes WHERE owner_id=$1 AND id=$2
  UNION ALL
  SELECT n.id, n.parent_id, c.path || n.id
  FROM nodes n JOIN chain c ON n.id=c.parent_id
  WHERE n.owner_id=$1 AND NOT n.id = ANY(c.path)
)
SELECT EXISTS(SELECT 1 FROM chain WHERE id=$3)`
		var cycle bool
		if err := tx.QueryRow(ctx, cycleQ, ownerID, *parentID, id).Scan(&cycle); err != nil {
			return domain.Node{}, fmt.Errorf("pgstore: check node cycle: %w", err)
		}
		if cycle {
			return domain.Node{}, ports.ErrNodeCycle
		}
	}
	const q = `UPDATE nodes SET parent_id=$1, updated_at=now() WHERE owner_id=$2 AND id=$3 RETURNING ` + nodeCols
	n, err := scanNode(tx.QueryRow(ctx, q, parentID, ownerID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Node{}, ports.ErrNodeNotFound
	}
	if err != nil {
		return domain.Node{}, mapSlugConflict(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Node{}, fmt.Errorf("pgstore: reparent commit: %w", err)
	}
	return n, nil
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
		&n.CreatedAt, &n.UpdatedAt, &n.CountsTowardTarget, &n.Icon, &n.LogoRef,
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
