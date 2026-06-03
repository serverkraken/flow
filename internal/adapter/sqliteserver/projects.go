package sqliteserver

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Projects is the server-side project store. It extends the client's
// ProjectStore pattern with version-based PullSince and optimistic-concurrency
// Upsert (takes an expectedVersion parameter and bumps version via NextLamport).
//
// Note: the server Upsert signature differs from ports.ProjectStore.Upsert
// (which takes no expectedVersion), so *Projects does NOT satisfy
// ports.ProjectStore. HTTP handlers call the concrete type directly.
type Projects struct{ store *Store }

// NewProjects constructs a Projects sub-adapter backed by store.
func NewProjects(s *Store) *Projects { return &Projects{store: s} }

// PullSince returns rows with version > since for userID, ordered by version ASC.
// hasMore is true when the result was truncated to limit rows.
func (p *Projects) PullSince(userID string, since int64, limit int) ([]domain.Project, int64, bool, error) {
	rows, err := p.store.DB().Query(`
		SELECT id, user_id, name, slug, created_at, last_used_at, archived_at, version
		FROM projects WHERE user_id = ? AND version > ?
		ORDER BY version ASC LIMIT ?`, userID, since, limit+1)
	if err != nil {
		return nil, since, false, fmt.Errorf("sqliteserver.Projects.PullSince: query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Project
	for rows.Next() {
		proj, err := scanServerProjectRow(rows)
		if err != nil {
			return nil, since, false, err
		}
		out = append(out, proj)
	}
	if err := rows.Err(); err != nil {
		return nil, since, false, fmt.Errorf("sqliteserver.Projects.PullSince: rows: %w", err)
	}

	hasMore := false
	if len(out) > limit {
		out = out[:limit]
		hasMore = true
	}
	high := since
	if len(out) > 0 {
		high = out[len(out)-1].Version
	}
	return out, high, hasMore, nil
}

// Upsert applies the project row with optimistic concurrency. If a row with the
// same id exists and stored.version != expectedVersion → ports.ErrProjectVersionConflict.
// On success inserts or updates, bumping version via NextLamport.
func (p *Projects) Upsert(in domain.Project, expectedVersion int64) (domain.Project, error) {
	tx, err := p.store.DB().Begin()
	if err != nil {
		return domain.Project{}, fmt.Errorf("sqliteserver.Projects.Upsert: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var curVersion int64
	row := tx.QueryRow(`SELECT version FROM projects WHERE id = ?`, in.ID)
	switch err := row.Scan(&curVersion); {
	case errors.Is(err, sql.ErrNoRows):
		if expectedVersion != 0 {
			return domain.Project{}, ports.ErrProjectVersionConflict
		}
	case err != nil:
		return domain.Project{}, fmt.Errorf("sqliteserver.Projects.Upsert: scan version: %w", err)
	default:
		if curVersion != expectedVersion {
			return domain.Project{}, ports.ErrProjectVersionConflict
		}
	}

	v, err := NextLamport(tx)
	if err != nil {
		return domain.Project{}, fmt.Errorf("sqliteserver.Projects.Upsert: lamport: %w", err)
	}

	createdAt := in.CreatedAt.UTC().Format(time.RFC3339)
	lastUsedAt := ""
	if !in.LastUsedAt.IsZero() {
		lastUsedAt = in.LastUsedAt.UTC().Format(time.RFC3339)
	}
	var archivedAt interface{}
	if in.ArchivedAt != nil {
		archivedAt = in.ArchivedAt.UTC().Format(time.RFC3339)
	}

	if _, err := tx.Exec(`
		INSERT INTO projects (id, user_id, name, slug, created_at, last_used_at, archived_at, version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name         = excluded.name,
			slug         = excluded.slug,
			last_used_at = excluded.last_used_at,
			archived_at  = excluded.archived_at,
			version      = excluded.version`,
		in.ID, in.UserID, in.Name, in.Slug, createdAt, lastUsedAt, archivedAt, v,
	); err != nil {
		return domain.Project{}, fmt.Errorf("sqliteserver.Projects.Upsert: exec: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.Project{}, fmt.Errorf("sqliteserver.Projects.Upsert: commit: %w", err)
	}
	in.Version = v
	return in, nil
}

// EnsureBySlug returns the existing project with the given slug or inserts a
// new row. Name and slug are only applied on creation. Used by ingress code.
func (p *Projects) EnsureBySlug(userID, name, slug string) (domain.Project, error) {
	existing, err := p.GetBySlug(userID, slug)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ports.ErrProjectNotFound) {
		return domain.Project{}, fmt.Errorf("sqliteserver.Projects.EnsureBySlug: lookup: %w", err)
	}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = p.store.DB().Exec(
		`INSERT INTO projects (id, user_id, name, slug, created_at, last_used_at, version)
		 VALUES (?, ?, ?, ?, ?, '', 0)`,
		id, userID, name, slug, now,
	)
	if err != nil {
		return domain.Project{}, fmt.Errorf("sqliteserver.Projects.EnsureBySlug: insert: %w", err)
	}
	return p.GetByID(userID, id)
}

// ListActive returns non-archived projects for the user, ordered MRU-first.
func (p *Projects) ListActive(userID string) ([]domain.Project, error) {
	rows, err := p.store.DB().Query(`
		SELECT id, user_id, name, slug, created_at, last_used_at, archived_at, version
		FROM projects
		WHERE user_id = ? AND archived_at IS NULL
		ORDER BY last_used_at DESC, created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("sqliteserver.Projects.ListActive: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanServerProjects(rows)
}

// ListAll returns all projects for the user including archived ones.
func (p *Projects) ListAll(userID string) ([]domain.Project, error) {
	rows, err := p.store.DB().Query(`
		SELECT id, user_id, name, slug, created_at, last_used_at, archived_at, version
		FROM projects
		WHERE user_id = ?
		ORDER BY last_used_at DESC, created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("sqliteserver.Projects.ListAll: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanServerProjects(rows)
}

// GetByID returns the project with the given ID scoped to userID.
func (p *Projects) GetByID(userID, id string) (domain.Project, error) {
	row := p.store.DB().QueryRow(`
		SELECT id, user_id, name, slug, created_at, last_used_at, archived_at, version
		FROM projects WHERE user_id = ? AND id = ?`, userID, id)
	proj, err := scanServerProject(row)
	if errors.Is(err, ports.ErrProjectNotFound) {
		return domain.Project{}, ports.ErrProjectNotFound
	}
	if err != nil {
		return domain.Project{}, fmt.Errorf("sqliteserver.Projects.GetByID: %w", err)
	}
	return proj, nil
}

// GetBySlug returns the project with the given slug scoped to userID.
func (p *Projects) GetBySlug(userID, slug string) (domain.Project, error) {
	row := p.store.DB().QueryRow(`
		SELECT id, user_id, name, slug, created_at, last_used_at, archived_at, version
		FROM projects WHERE user_id = ? AND slug = ?`, userID, slug)
	proj, err := scanServerProject(row)
	if errors.Is(err, ports.ErrProjectNotFound) {
		return domain.Project{}, ports.ErrProjectNotFound
	}
	if err != nil {
		return domain.Project{}, fmt.Errorf("sqliteserver.Projects.GetBySlug: %w", err)
	}
	return proj, nil
}

// TouchLastUsed updates last_used_at to now for the given project.
func (p *Projects) TouchLastUsed(userID, id string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := p.store.DB().Exec(
		`UPDATE projects SET last_used_at = ? WHERE user_id = ? AND id = ?`,
		now, userID, id,
	)
	if err != nil {
		return fmt.Errorf("sqliteserver.Projects.TouchLastUsed: %w", err)
	}
	return nil
}

// Archive soft-deletes the project by setting archived_at to now.
func (p *Projects) Archive(userID, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := p.store.DB().Exec(
		`UPDATE projects SET archived_at = ? WHERE user_id = ? AND id = ?`,
		now, userID, id,
	)
	if err != nil {
		return fmt.Errorf("sqliteserver.Projects.Archive: %w", err)
	}
	return nil
}

func scanServerProjects(rows *sql.Rows) ([]domain.Project, error) {
	var result []domain.Project
	for rows.Next() {
		proj, err := scanServerProjectRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, proj)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqliteserver.Projects: rows: %w", err)
	}
	return result, nil
}

func scanServerProject(row *sql.Row) (domain.Project, error) {
	var (
		proj       domain.Project
		createdAt  string
		lastUsedAt string
		archivedAt sql.NullString
		version    int64
	)
	err := row.Scan(&proj.ID, &proj.UserID, &proj.Name, &proj.Slug,
		&createdAt, &lastUsedAt, &archivedAt, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Project{}, ports.ErrProjectNotFound
	}
	if err != nil {
		return domain.Project{}, fmt.Errorf("sqliteserver.Projects: scan: %w", err)
	}
	return buildServerProject(proj, createdAt, lastUsedAt, archivedAt, version)
}

func scanServerProjectRow(rows *sql.Rows) (domain.Project, error) {
	var (
		proj       domain.Project
		createdAt  string
		lastUsedAt string
		archivedAt sql.NullString
		version    int64
	)
	err := rows.Scan(&proj.ID, &proj.UserID, &proj.Name, &proj.Slug,
		&createdAt, &lastUsedAt, &archivedAt, &version)
	if err != nil {
		return domain.Project{}, fmt.Errorf("sqliteserver.Projects: scan row: %w", err)
	}
	return buildServerProject(proj, createdAt, lastUsedAt, archivedAt, version)
}

func buildServerProject(proj domain.Project, createdAt, lastUsedAt string, archivedAt sql.NullString, version int64) (domain.Project, error) {
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return domain.Project{}, fmt.Errorf("sqliteserver.Projects: parse created_at %q: %w", createdAt, err)
	}
	proj.CreatedAt = t
	proj.Version = version

	if lastUsedAt != "" {
		lu, err := parseServerTimestamp(lastUsedAt)
		if err != nil {
			return domain.Project{}, fmt.Errorf("sqliteserver.Projects: parse last_used_at %q: %w", lastUsedAt, err)
		}
		proj.LastUsedAt = lu
	}

	if archivedAt.Valid {
		at, err := parseServerTimestamp(archivedAt.String)
		if err != nil {
			return domain.Project{}, fmt.Errorf("sqliteserver.Projects: parse archived_at %q: %w", archivedAt.String, err)
		}
		proj.ArchivedAt = &at
	}

	return proj, nil
}

// parseServerTimestamp parses a stored timestamp that may be RFC3339 or RFC3339Nano.
func parseServerTimestamp(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}
