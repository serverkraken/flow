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

// ArtifactStore persists node-scoped artifacts as Postgres blobs (N per node,
// FK ON DELETE CASCADE cleans up when the node goes).
type ArtifactStore struct{ pool *pgxpool.Pool }

// NewArtifactStore wires the store to a pgx pool.
func NewArtifactStore(pool *pgxpool.Pool) *ArtifactStore { return &ArtifactStore{pool: pool} }

const artifactCols = `id, owner_id, node_id, slug, name, mime, size_bytes, ref, bytes, width, height, created_by_kind, created_by_ref, created_at, updated_at`

const artifactMetaCols = `id, owner_id, node_id, slug, name, mime, size_bytes, ref, width, height, created_by_kind, created_by_ref, created_at, updated_at`

func (s *ArtifactStore) Put(ctx context.Context, a domain.Artifact) error {
	const base = `
INSERT INTO artifacts (` + artifactCols + `)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
ON CONFLICT `
	const setClause = ` DO UPDATE SET
    name=$5, mime=$6, size_bytes=$7, ref=$8, bytes=$9, width=$10, height=$11, updated_at=$15`
	target := `(owner_id, node_id, slug)`
	if a.NodeID == "" {
		target = `(owner_id, slug) WHERE node_id IS NULL` // Partial-Index-Arbiter
	}
	_, err := s.pool.Exec(ctx, base+target+setClause,
		a.ID, a.OwnerID, nullableStr(a.NodeID), a.Slug, a.Name, a.Mime, a.SizeBytes, a.Ref, a.Bytes,
		nullableInt(a.Width), nullableInt(a.Height), a.CreatedByKind, a.CreatedByRef, a.CreatedAt, a.UpdatedAt)
	if err != nil {
		return fmt.Errorf("pgstore: put artifact: %w", err)
	}
	return nil
}

// nullableInt maps a zero pixel dimension to NULL (width/height are only
// meaningful for images) — mirrors the node_logos width/height columns.
func nullableInt(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}

// nullableStr maps a node-less artifact ("" NodeID) to SQL NULL — the free
// (owner-global) library sentinel. Node-bound artifacts pass their real ID
// through unchanged.
func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// derefStr maps a NULL node_id back to "" (free artifact).
func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (s *ArtifactStore) Get(ctx context.Context, ownerID, nodeID, slug string) (domain.Artifact, error) {
	const q = `SELECT ` + artifactCols + ` FROM artifacts WHERE owner_id=$1 AND node_id IS NOT DISTINCT FROM $2 AND slug=$3`
	var a domain.Artifact
	var node *string
	var width, height *int
	err := s.pool.QueryRow(ctx, q, ownerID, nullableStr(nodeID), slug).Scan(
		&a.ID, &a.OwnerID, &node, &a.Slug, &a.Name, &a.Mime, &a.SizeBytes, &a.Ref, &a.Bytes,
		&width, &height, &a.CreatedByKind, &a.CreatedByRef, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Artifact{}, ports.ErrArtifactNotFound
	}
	if err != nil {
		return domain.Artifact{}, fmt.Errorf("pgstore: get artifact: %w", err)
	}
	a.NodeID = derefStr(node)
	a.Width, a.Height = derefInt(width), derefInt(height)
	return a, nil
}

func (s *ArtifactStore) GetMeta(ctx context.Context, ownerID, nodeID, slug string) (domain.Artifact, error) {
	const q = `SELECT ` + artifactMetaCols + ` FROM artifacts WHERE owner_id=$1 AND node_id IS NOT DISTINCT FROM $2 AND slug=$3`
	var a domain.Artifact
	var node *string
	var width, height *int
	err := s.pool.QueryRow(ctx, q, ownerID, nullableStr(nodeID), slug).Scan(
		&a.ID, &a.OwnerID, &node, &a.Slug, &a.Name, &a.Mime, &a.SizeBytes, &a.Ref,
		&width, &height, &a.CreatedByKind, &a.CreatedByRef, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Artifact{}, ports.ErrArtifactNotFound
	}
	if err != nil {
		return domain.Artifact{}, fmt.Errorf("pgstore: get artifact meta: %w", err)
	}
	a.NodeID = derefStr(node)
	a.Width, a.Height = derefInt(width), derefInt(height)
	return a, nil
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func (s *ArtifactStore) List(ctx context.Context, ownerID string, nodeIDs ...string) ([]domain.Artifact, error) {
	const q = `SELECT ` + artifactMetaCols + ` FROM artifacts WHERE owner_id=$1 AND node_id = ANY($2) ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, q, ownerID, nodeIDs)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list artifacts: %w", err)
	}
	defer rows.Close()
	var out []domain.Artifact
	for rows.Next() {
		var a domain.Artifact
		var width, height *int
		if err := rows.Scan(&a.ID, &a.OwnerID, &a.NodeID, &a.Slug, &a.Name, &a.Mime, &a.SizeBytes, &a.Ref,
			&width, &height, &a.CreatedByKind, &a.CreatedByRef, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("pgstore: scan artifact: %w", err)
		}
		a.Width, a.Height = derefInt(width), derefInt(height)
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgstore: list artifacts rows: %w", err)
	}
	return out, nil
}

// Rename changes only the display name + updated_at. Slug/ref/bytes stay
// untouched so wikilink-style ![[slug]] references and cached ?v={ref} embed
// URLs keep working after a rename.
func (s *ArtifactStore) Rename(ctx context.Context, ownerID, nodeID, slug, name string) error {
	const q = `UPDATE artifacts SET name=$1, updated_at=now() WHERE owner_id=$2 AND node_id IS NOT DISTINCT FROM $3 AND slug=$4`
	tag, err := s.pool.Exec(ctx, q, name, ownerID, nullableStr(nodeID), slug)
	if err != nil {
		return fmt.Errorf("pgstore: rename artifact: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ErrArtifactNotFound
	}
	return nil
}

// ListFree returns owner-global (node-less) artifact META (no bytes), newest first.
func (s *ArtifactStore) ListFree(ctx context.Context, ownerID string) ([]domain.Artifact, error) {
	const q = `SELECT ` + artifactMetaCols + ` FROM artifacts WHERE owner_id=$1 AND node_id IS NULL ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, q, ownerID)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list free artifacts: %w", err)
	}
	defer rows.Close()
	var out []domain.Artifact
	for rows.Next() {
		var a domain.Artifact
		var node *string
		var width, height *int
		if err := rows.Scan(&a.ID, &a.OwnerID, &node, &a.Slug, &a.Name, &a.Mime, &a.SizeBytes, &a.Ref,
			&width, &height, &a.CreatedByKind, &a.CreatedByRef, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("pgstore: scan free artifact: %w", err)
		}
		a.NodeID = derefStr(node)
		a.Width, a.Height = derefInt(width), derefInt(height)
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgstore: list free artifacts rows: %w", err)
	}
	return out, nil
}

func (s *ArtifactStore) ExistingSlugs(ctx context.Context, ownerID, nodeID string) ([]string, error) {
	const q = `SELECT slug FROM artifacts WHERE owner_id=$1 AND node_id IS NOT DISTINCT FROM $2`
	rows, err := s.pool.Query(ctx, q, ownerID, nullableStr(nodeID))
	if err != nil {
		return nil, fmt.Errorf("pgstore: existing artifact slugs: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, fmt.Errorf("pgstore: scan artifact slug: %w", err)
		}
		out = append(out, slug)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgstore: existing artifact slugs rows: %w", err)
	}
	return out, nil
}

func (s *ArtifactStore) TotalBytes(ctx context.Context, ownerID string) (int64, error) {
	const q = `SELECT coalesce(sum(size_bytes),0) FROM artifacts WHERE owner_id=$1`
	var total int64
	if err := s.pool.QueryRow(ctx, q, ownerID).Scan(&total); err != nil {
		return 0, fmt.Errorf("pgstore: total artifact bytes: %w", err)
	}
	return total, nil
}

func (s *ArtifactStore) Delete(ctx context.Context, ownerID, nodeID, slug string) error {
	const q = `DELETE FROM artifacts WHERE owner_id=$1 AND node_id IS NOT DISTINCT FROM $2 AND slug=$3`
	tag, err := s.pool.Exec(ctx, q, ownerID, nullableStr(nodeID), slug)
	if err != nil {
		return fmt.Errorf("pgstore: delete artifact: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ErrArtifactNotFound
	}
	return nil
}
