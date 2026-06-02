package sqliteclient

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Projects implements ports.ProjectStore against the SQLite projects table.
type Projects struct {
	store *Store
}

// compile-time interface assertion
var _ ports.ProjectStore = (*Projects)(nil)

// NewProjects constructs a Projects sub-adapter backed by store.
func NewProjects(store *Store) *Projects { return &Projects{store: store} }

// ListActive returns non-archived projects for the user, ordered MRU-first.
func (p *Projects) ListActive(userID string) ([]domain.Project, error) {
	rows, err := p.store.DB().Query(
		`SELECT id, user_id, name, slug, created_at, last_used_at, archived_at, version
		   FROM projects
		  WHERE user_id = ? AND archived_at IS NULL
		  ORDER BY last_used_at DESC, created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("sqliteclient.Projects.ListActive: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanProjects(rows)
}

// ListAll returns all projects for the user including archived ones.
func (p *Projects) ListAll(userID string) ([]domain.Project, error) {
	rows, err := p.store.DB().Query(
		`SELECT id, user_id, name, slug, created_at, last_used_at, archived_at, version
		   FROM projects
		  WHERE user_id = ?
		  ORDER BY last_used_at DESC, created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("sqliteclient.Projects.ListAll: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanProjects(rows)
}

// GetByID returns the project with the given ID scoped to userID.
func (p *Projects) GetByID(userID, id string) (domain.Project, error) {
	row := p.store.DB().QueryRow(
		`SELECT id, user_id, name, slug, created_at, last_used_at, archived_at, version
		   FROM projects WHERE user_id = ? AND id = ?`,
		userID, id,
	)
	proj, err := scanProject(row)
	if errors.Is(err, ports.ErrProjectNotFound) {
		return domain.Project{}, ports.ErrProjectNotFound
	}
	if err != nil {
		return domain.Project{}, fmt.Errorf("sqliteclient.Projects.GetByID: %w", err)
	}
	return proj, nil
}

// GetBySlug returns the project with the given slug scoped to userID.
func (p *Projects) GetBySlug(userID, slug string) (domain.Project, error) {
	row := p.store.DB().QueryRow(
		`SELECT id, user_id, name, slug, created_at, last_used_at, archived_at, version
		   FROM projects WHERE user_id = ? AND slug = ?`,
		userID, slug,
	)
	proj, err := scanProject(row)
	if errors.Is(err, ports.ErrProjectNotFound) {
		return domain.Project{}, ports.ErrProjectNotFound
	}
	if err != nil {
		return domain.Project{}, fmt.Errorf("sqliteclient.Projects.GetBySlug: %w", err)
	}
	return proj, nil
}

// EnsureBySlug returns the existing project with the given slug or inserts a
// new one. Name and slug are only applied on creation.
func (p *Projects) EnsureBySlug(userID, name, slug string) (domain.Project, error) {
	existing, err := p.GetBySlug(userID, slug)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ports.ErrProjectNotFound) {
		return domain.Project{}, fmt.Errorf("sqliteclient.Projects.EnsureBySlug: lookup: %w", err)
	}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = p.store.DB().Exec(
		`INSERT INTO projects (id, user_id, name, slug, created_at, last_used_at, version)
		 VALUES (?, ?, ?, ?, ?, '', 0)`,
		id, userID, name, slug, now,
	)
	if err != nil {
		return domain.Project{}, fmt.Errorf("sqliteclient.Projects.EnsureBySlug: insert: %w", err)
	}
	return p.GetByID(userID, id)
}

// Upsert inserts or fully replaces a project row (used by sync ingestion).
func (p *Projects) Upsert(proj domain.Project) error {
	createdAt := proj.CreatedAt.UTC().Format(time.RFC3339)
	lastUsedAt := ""
	if !proj.LastUsedAt.IsZero() {
		lastUsedAt = proj.LastUsedAt.UTC().Format(time.RFC3339)
	}
	var archivedAt interface{}
	if proj.ArchivedAt != nil {
		archivedAt = proj.ArchivedAt.UTC().Format(time.RFC3339)
	}
	_, err := p.store.DB().Exec(
		`INSERT INTO projects (id, user_id, name, slug, created_at, last_used_at, archived_at, version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   name        = excluded.name,
		   slug        = excluded.slug,
		   last_used_at = excluded.last_used_at,
		   archived_at = excluded.archived_at,
		   version     = excluded.version`,
		proj.ID, proj.UserID, proj.Name, proj.Slug, createdAt, lastUsedAt, archivedAt, proj.Version,
	)
	if err != nil {
		return fmt.Errorf("sqliteclient.Projects.Upsert: %w", err)
	}
	return nil
}

// TouchLastUsed updates last_used_at to now for the given project.
func (p *Projects) TouchLastUsed(userID, id string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := p.store.DB().Exec(
		`UPDATE projects SET last_used_at = ? WHERE user_id = ? AND id = ?`,
		now, userID, id,
	)
	if err != nil {
		return fmt.Errorf("sqliteclient.Projects.TouchLastUsed: %w", err)
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
		return fmt.Errorf("sqliteclient.Projects.Archive: %w", err)
	}
	return nil
}

func scanProjects(rows *sql.Rows) ([]domain.Project, error) {
	var result []domain.Project
	for rows.Next() {
		proj, err := scanProjectRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, proj)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqliteclient.Projects: rows: %w", err)
	}
	return result, nil
}

func scanProject(row *sql.Row) (domain.Project, error) {
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
		return domain.Project{}, fmt.Errorf("sqliteclient.Projects: scan: %w", err)
	}
	return buildProject(proj, createdAt, lastUsedAt, archivedAt, version)
}

func scanProjectRow(rows *sql.Rows) (domain.Project, error) {
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
		return domain.Project{}, fmt.Errorf("sqliteclient.Projects: scan row: %w", err)
	}
	return buildProject(proj, createdAt, lastUsedAt, archivedAt, version)
}

func buildProject(proj domain.Project, createdAt, lastUsedAt string, archivedAt sql.NullString, version int64) (domain.Project, error) {
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return domain.Project{}, fmt.Errorf("sqliteclient.Projects: parse created_at %q: %w", createdAt, err)
	}
	proj.CreatedAt = t
	proj.Version = version

	if lastUsedAt != "" {
		lu, err := parseTimestamp(lastUsedAt)
		if err != nil {
			return domain.Project{}, fmt.Errorf("sqliteclient.Projects: parse last_used_at %q: %w", lastUsedAt, err)
		}
		proj.LastUsedAt = lu
	}

	if archivedAt.Valid {
		at, err := parseTimestamp(archivedAt.String)
		if err != nil {
			return domain.Project{}, fmt.Errorf("sqliteclient.Projects: parse archived_at %q: %w", archivedAt.String, err)
		}
		proj.ArchivedAt = &at
	}

	return proj, nil
}

// parseTimestamp parses a stored timestamp string that may be RFC3339 or
// RFC3339Nano — TouchLastUsed stores nanosecond-precision strings for
// deterministic MRU ordering even within the same second.
func parseTimestamp(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}
