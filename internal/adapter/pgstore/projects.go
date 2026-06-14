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

type ProjectStore struct{ pool *pgxpool.Pool }

func NewProjectStore(pool *pgxpool.Pool) *ProjectStore { return &ProjectStore{pool: pool} }

func (s *ProjectStore) Create(ctx context.Context, p domain.Project) (domain.Project, error) {
	const q = `
INSERT INTO projects (id, owner_id, name, slug, color, glyph, status, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
RETURNING id, owner_id, name, slug, color, glyph, status, created_at, updated_at`
	return scanProject(s.pool.QueryRow(ctx, q,
		p.ID, p.OwnerID, p.Name, p.Slug, p.Color, p.Glyph, string(p.Status), p.CreatedAt, p.UpdatedAt))
}

func (s *ProjectStore) List(ctx context.Context, ownerID string) ([]domain.Project, error) {
	const q = `
SELECT id, owner_id, name, slug, color, glyph, status, created_at, updated_at
FROM projects WHERE owner_id=$1 ORDER BY name`
	rows, err := s.pool.Query(ctx, q, ownerID)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list projects: %w", err)
	}
	defer rows.Close()
	var out []domain.Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *ProjectStore) Get(ctx context.Context, ownerID, id string) (domain.Project, error) {
	const q = `
SELECT id, owner_id, name, slug, color, glyph, status, created_at, updated_at
FROM projects WHERE owner_id=$1 AND id=$2`
	p, err := scanProject(s.pool.QueryRow(ctx, q, ownerID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Project{}, ports.ErrProjectNotFound
	}
	return p, err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanProject(r rowScanner) (domain.Project, error) {
	var p domain.Project
	var status string
	if err := r.Scan(&p.ID, &p.OwnerID, &p.Name, &p.Slug, &p.Color, &p.Glyph, &status, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Project{}, err
		}
		return domain.Project{}, fmt.Errorf("pgstore: scan project: %w", err)
	}
	p.Status = domain.ProjectStatus(status)
	return p, nil
}
