// internal/adapter/pgstore/projects.go
package pgstore

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Projects mirrors the sqliteserver.Projects surface (minus PullSince) on PG.
type Projects struct{ store *Store }

func NewProjects(s *Store) *Projects { return &Projects{store: s} }

const projectCols = `id, user_id, name, slug, archived_at, created_at, last_used_at, version, updated_at`

func (p *Projects) ListActive(userID string) ([]domain.Project, error) {
	return p.list(userID, `AND archived_at IS NULL`)
}

func (p *Projects) ListAll(userID string) ([]domain.Project, error) {
	return p.list(userID, ``)
}

func (p *Projects) list(userID, extraCond string) ([]domain.Project, error) {
	rows, err := p.store.Pool().Query(context.Background(),
		`SELECT `+projectCols+` FROM projects WHERE user_id = $1 `+extraCond+` ORDER BY name ASC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Project
	for rows.Next() {
		proj, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, proj)
	}
	return out, rows.Err()
}

func (p *Projects) GetByID(userID, id string) (domain.Project, error) {
	row := p.store.Pool().QueryRow(context.Background(),
		`SELECT `+projectCols+` FROM projects WHERE user_id = $1 AND id = $2`, userID, id)
	return scanProjectNotFound(row)
}

func (p *Projects) GetBySlug(userID, slug string) (domain.Project, error) {
	row := p.store.Pool().QueryRow(context.Background(),
		`SELECT `+projectCols+` FROM projects WHERE user_id = $1 AND slug = $2`, userID, slug)
	return scanProjectNotFound(row)
}

// EnsureBySlug creates the project if missing and returns the existing row
// otherwise — it never renames (matches sqliteserver semantics).
func (p *Projects) EnsureBySlug(userID, name, slug string) (domain.Project, error) {
	row := p.store.Pool().QueryRow(context.Background(), `
		INSERT INTO projects (id, user_id, name, slug)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, slug) DO UPDATE SET slug = EXCLUDED.slug -- no-op, forces RETURNING
		RETURNING `+projectCols,
		uuid.NewString(), userID, name, slug)
	return scanProject(row)
}

// Upsert writes with OCC: the stored version must equal expectedVersion
// (0 = "must not exist yet"). Returns the saved row with bumped version.
func (p *Projects) Upsert(in domain.Project, expectedVersion int64) (domain.Project, error) {
	ctx := context.Background()
	if expectedVersion == 0 {
		if in.ID == "" {
			in.ID = uuid.NewString()
		}
		row := p.store.Pool().QueryRow(ctx, `
			INSERT INTO projects (id, user_id, name, slug, archived_at, version, updated_at)
			VALUES ($1, $2, $3, $4, $5, 1, now())
			ON CONFLICT (id) DO NOTHING
			RETURNING `+projectCols,
			in.ID, in.UserID, in.Name, in.Slug, in.ArchivedAt)
		out, err := scanProject(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Project{}, ports.ErrProjectVersionConflict
		}
		return out, err
	}
	row := p.store.Pool().QueryRow(ctx, `
		UPDATE projects
		SET name = $3, slug = $4, archived_at = $5, version = version + 1, updated_at = now()
		WHERE user_id = $1 AND id = $2 AND version = $6
		RETURNING `+projectCols,
		in.UserID, in.ID, in.Name, in.Slug, in.ArchivedAt, expectedVersion)
	out, err := scanProject(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Project{}, ports.ErrProjectVersionConflict
	}
	return out, err
}

func (p *Projects) TouchLastUsed(userID, id string) error {
	_, err := p.store.Pool().Exec(context.Background(),
		`UPDATE projects SET last_used_at = now() WHERE user_id = $1 AND id = $2`, userID, id)
	return err
}

func (p *Projects) Archive(userID, id string) error {
	_, err := p.store.Pool().Exec(context.Background(),
		`UPDATE projects SET archived_at = now(), version = version + 1, updated_at = now()
		 WHERE user_id = $1 AND id = $2 AND archived_at IS NULL`, userID, id)
	return err
}

func scanProject(r rowScanner) (domain.Project, error) {
	var out domain.Project
	var archivedAt, lastUsedAt *time.Time
	err := r.Scan(&out.ID, &out.UserID, &out.Name, &out.Slug,
		&archivedAt, &out.CreatedAt, &lastUsedAt, &out.Version, new(time.Time))
	if err != nil {
		return domain.Project{}, err
	}
	out.ArchivedAt = archivedAt
	if lastUsedAt != nil {
		out.LastUsedAt = *lastUsedAt
	}
	return out, nil
}

func scanProjectNotFound(r rowScanner) (domain.Project, error) {
	out, err := scanProject(r)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Project{}, ports.ErrProjectNotFound
	}
	return out, err
}
