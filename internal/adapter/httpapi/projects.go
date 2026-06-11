package httpapi

// Projects implements ports.ProjectStore against the bearer API.
//
// Server scopes all reads/writes to the authenticated user — the userID
// parameter on each method is accepted for interface compliance but ignored.
//
// A single cache is maintained for the ListAll result set; ListActive filters
// client-side from that same cache to avoid two separate cache entries.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Projects implements ports.ProjectStore via the server bearer API.
type Projects struct {
	c     *Client
	cache resourceCache[[]projectDTO] // always caches the full ListAll result
}

// NewProjects constructs a Projects adapter backed by c.
func NewProjects(c *Client) *Projects {
	p := &Projects{c: c}
	if snap, ok := loadSnapshot(); ok {
		p.cache.put(snap.Projects)
	}
	return p
}

var _ ports.ProjectStore = (*Projects)(nil)

// ListActive returns non-archived projects for the authenticated user.
// userID is ignored — the server scopes the response to the bearer token.
func (p *Projects) ListActive(_ string) ([]domain.Project, error) {
	all, err := p.listAll()
	if err != nil {
		return nil, err
	}
	out := make([]domain.Project, 0, len(all))
	for _, proj := range all {
		if proj.ArchivedAt == nil {
			out = append(out, proj)
		}
	}
	return out, nil
}

// ListAll returns all projects including archived ones.
// userID is ignored — the server scopes the response to the bearer token.
func (p *Projects) ListAll(_ string) ([]domain.Project, error) {
	return p.listAll()
}

// GetByID returns a project by its ID.
// userID is ignored — the server scopes the response to the bearer token.
func (p *Projects) GetByID(_, id string) (domain.Project, error) {
	all, err := p.listAll()
	if err != nil {
		return domain.Project{}, err
	}
	for _, proj := range all {
		if proj.ID == id {
			return proj, nil
		}
	}
	// Not in cache — attempt a direct fetch
	var dto projectDTO
	err = p.c.doJSON(context.Background(), http.MethodGet,
		fmt.Sprintf("/api/v1/projects/%s", id),
		nil, -1, &dto)
	if err != nil {
		if statusCode(err) == http.StatusNotFound {
			return domain.Project{}, ports.ErrProjectNotFound
		}
		return domain.Project{}, err
	}
	return projectFromDTO(dto, ""), nil
}

// GetBySlug returns a project by its slug.
// userID is ignored — the server scopes the response to the bearer token.
func (p *Projects) GetBySlug(_, slug string) (domain.Project, error) {
	all, err := p.listAll()
	if err != nil {
		return domain.Project{}, err
	}
	for _, proj := range all {
		if proj.Slug == slug {
			return proj, nil
		}
	}
	return domain.Project{}, ports.ErrProjectNotFound
}

// EnsureBySlug returns an existing project by slug or creates a new one.
// userID is ignored — the server scopes writes to the bearer token.
func (p *Projects) EnsureBySlug(_, name, slug string) (domain.Project, error) {
	proj, err := p.GetBySlug("", slug)
	if err == nil {
		return proj, nil
	}
	if !isProjectNotFound(err) {
		return domain.Project{}, err
	}
	// Create via POST
	body := struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}{Name: name, Slug: slug}
	var dto projectDTO
	err = p.c.doJSON(context.Background(), http.MethodPost,
		"/api/v1/projects",
		body, -1, &dto)
	if err != nil {
		if statusCode(err) == http.StatusConflict {
			// Race: already exists — retry GetBySlug
			p.cache.invalidate()
			return p.GetBySlug("", slug)
		}
		return domain.Project{}, err
	}
	p.cache.invalidate()
	return projectFromDTO(dto, ""), nil
}

// Upsert updates a project on the server (PUT with If-Match).
// userID is ignored — the server scopes writes to the bearer token.
func (p *Projects) Upsert(proj domain.Project) error {
	body := struct {
		Name     string `json:"name"`
		Slug     string `json:"slug"`
		Archived bool   `json:"archived"`
	}{
		Name:     proj.Name,
		Slug:     proj.Slug,
		Archived: proj.ArchivedAt != nil,
	}
	var out projectDTO
	err := p.c.doJSON(context.Background(), http.MethodPut,
		fmt.Sprintf("/api/v1/projects/%s", proj.ID),
		body, proj.Version, &out)
	if err != nil {
		switch statusCode(err) {
		case http.StatusPreconditionFailed:
			return ports.ErrProjectVersionConflict
		case http.StatusNotFound:
			return ports.ErrProjectNotFound
		}
		return err
	}
	p.cache.invalidate()
	return nil
}

// TouchLastUsed is a no-op. The server updates last_used_at on session start
// (Entscheidung 3 — no dedicated endpoint required on the client side).
func (p *Projects) TouchLastUsed(_, _ string) error { return nil }

// Archive soft-deletes the project.
// userID is ignored — the server scopes writes to the bearer token.
func (p *Projects) Archive(_, id string) error {
	proj, err := p.GetByID("", id)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	proj.ArchivedAt = &now
	return p.Upsert(proj)
}

// — helpers -------------------------------------------------------------------

// listAll fetches all projects (including archived) from cache or server.
func (p *Projects) listAll() ([]domain.Project, error) {
	if cached, ok := p.cache.get(); ok {
		return p.toDomain(cached), nil
	}
	var env itemsEnvelope[projectDTO]
	err := p.c.doJSON(context.Background(), http.MethodGet,
		"/api/v1/projects?all=1",
		nil, -1, &env)
	if err != nil {
		if fb, ok := p.cache.fallback(); ok {
			return p.toDomain(fb), nil
		}
		return nil, err
	}
	p.cache.put(env.Items)
	go func() {
		snap, ok := loadSnapshot()
		if !ok {
			snap = Snapshot{}
		}
		snap.Projects = env.Items
		if err := saveSnapshot(snap); err != nil {
			slog.Warn("httpapi: projects snapshot save failed", "err", err)
		}
	}()
	return p.toDomain(env.Items), nil
}

func (p *Projects) toDomain(dtos []projectDTO) []domain.Project {
	out := make([]domain.Project, 0, len(dtos))
	for _, d := range dtos {
		out = append(out, projectFromDTO(d, ""))
	}
	return out
}

func isProjectNotFound(err error) bool {
	return errors.Is(err, ports.ErrProjectNotFound)
}
