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

// ProjectBindingStore persists project_bindings rows. All reads are owner-scoped.
type ProjectBindingStore struct{ pool *pgxpool.Pool }

// NewProjectBindingStore returns a store backed by the given pool.
func NewProjectBindingStore(pool *pgxpool.Pool) *ProjectBindingStore {
	return &ProjectBindingStore{pool: pool}
}

// Upsert inserts or reassigns a binding. For remote bindings the conflict target
// is (owner_id, remote_slug) WHERE kind='remote'; for path bindings it is
// (owner_id, machine_id, path) WHERE kind='path'.
func (s *ProjectBindingStore) Upsert(ctx context.Context, b domain.ProjectBinding) (domain.ProjectBinding, error) {
	switch b.Kind {
	case domain.BindingRemote:
		return s.upsertRemote(ctx, b)
	case domain.BindingPath:
		return s.upsertPath(ctx, b)
	default:
		return domain.ProjectBinding{}, fmt.Errorf("pgstore: unknown binding kind %q", b.Kind)
	}
}

func (s *ProjectBindingStore) upsertRemote(ctx context.Context, b domain.ProjectBinding) (domain.ProjectBinding, error) {
	const q = `
INSERT INTO project_bindings
  (id, owner_id, project_id, kind, remote_slug, machine_id, machine_label, path, created_at, updated_at)
VALUES ($1,$2,$3,'remote',$4,NULL,NULL,NULL,$5,$6)
ON CONFLICT (owner_id, remote_slug) WHERE kind='remote'
DO UPDATE SET project_id=EXCLUDED.project_id, updated_at=EXCLUDED.updated_at
RETURNING id, owner_id, project_id, kind, remote_slug, machine_id, machine_label, path, created_at, updated_at`
	return scanBinding(s.pool.QueryRow(ctx, q,
		b.ID, b.OwnerID, b.ProjectID, b.RemoteSlug, b.CreatedAt, b.UpdatedAt))
}

func (s *ProjectBindingStore) upsertPath(ctx context.Context, b domain.ProjectBinding) (domain.ProjectBinding, error) {
	const q = `
INSERT INTO project_bindings
  (id, owner_id, project_id, kind, remote_slug, machine_id, machine_label, path, created_at, updated_at)
VALUES ($1,$2,$3,'path',NULL,$4,$5,$6,$7,$8)
ON CONFLICT (owner_id, machine_id, path) WHERE kind='path'
DO UPDATE SET project_id=EXCLUDED.project_id, machine_label=EXCLUDED.machine_label, updated_at=EXCLUDED.updated_at
RETURNING id, owner_id, project_id, kind, remote_slug, machine_id, machine_label, path, created_at, updated_at`
	return scanBinding(s.pool.QueryRow(ctx, q,
		b.ID, b.OwnerID, b.ProjectID, b.MachineID, b.MachineLabel, b.Path, b.CreatedAt, b.UpdatedAt))
}

// DeleteRemote removes a remote binding by (owner, remoteSlug).
// Returns ErrBindingNotFound if no row is removed.
func (s *ProjectBindingStore) DeleteRemote(ctx context.Context, ownerID, remoteSlug string) error {
	const q = `DELETE FROM project_bindings WHERE owner_id=$1 AND remote_slug=$2 AND kind='remote'`
	tag, err := s.pool.Exec(ctx, q, ownerID, remoteSlug)
	if err != nil {
		return fmt.Errorf("pgstore: delete remote binding: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ErrBindingNotFound
	}
	return nil
}

// DeletePath removes a path binding by (owner, machineID, path).
// Returns ErrBindingNotFound if no row is removed.
func (s *ProjectBindingStore) DeletePath(ctx context.Context, ownerID, machineID, path string) error {
	const q = `DELETE FROM project_bindings WHERE owner_id=$1 AND machine_id=$2 AND path=$3 AND kind='path'`
	tag, err := s.pool.Exec(ctx, q, ownerID, machineID, path)
	if err != nil {
		return fmt.Errorf("pgstore: delete path binding: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ErrBindingNotFound
	}
	return nil
}

// List returns all bindings for ownerID, ordered by created_at.
func (s *ProjectBindingStore) List(ctx context.Context, ownerID string) ([]domain.ProjectBinding, error) {
	const q = `
SELECT id, owner_id, project_id, kind, remote_slug, machine_id, machine_label, path, created_at, updated_at
FROM project_bindings WHERE owner_id=$1 ORDER BY created_at`
	return s.queryBindings(ctx, q, ownerID)
}

// ListByProject returns all bindings for (ownerID, projectID), ordered by created_at.
func (s *ProjectBindingStore) ListByProject(ctx context.Context, ownerID, projectID string) ([]domain.ProjectBinding, error) {
	const q = `
SELECT id, owner_id, project_id, kind, remote_slug, machine_id, machine_label, path, created_at, updated_at
FROM project_bindings WHERE owner_id=$1 AND project_id=$2 ORDER BY created_at`
	return s.queryBindings(ctx, q, ownerID, projectID)
}

func (s *ProjectBindingStore) queryBindings(ctx context.Context, q string, args ...any) ([]domain.ProjectBinding, error) {
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list bindings: %w", err)
	}
	defer rows.Close()
	var out []domain.ProjectBinding
	for rows.Next() {
		b, err := scanBinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func scanBinding(r rowScanner) (domain.ProjectBinding, error) {
	var b domain.ProjectBinding
	var kind string
	var remoteSlug, machineID, machineLabel, path *string
	if err := r.Scan(
		&b.ID, &b.OwnerID, &b.ProjectID, &kind,
		&remoteSlug, &machineID, &machineLabel, &path,
		&b.CreatedAt, &b.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ProjectBinding{}, err
		}
		return domain.ProjectBinding{}, fmt.Errorf("pgstore: scan binding: %w", err)
	}
	b.Kind = domain.BindingKind(kind)
	if remoteSlug != nil {
		b.RemoteSlug = *remoteSlug
	}
	if machineID != nil {
		b.MachineID = *machineID
	}
	if machineLabel != nil {
		b.MachineLabel = *machineLabel
	}
	if path != nil {
		b.Path = *path
	}
	return b, nil
}
