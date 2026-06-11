// Package httpserver implements the REST and bearer APIs.
//
// R1 Bearer-API für Projekte (Spec §7: GET/POST /projects, PUT
// /projects/{id} inkl. Archivieren via archived=true).
package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/webui/sse"
)

// ProjectsAPIStore is the surface MountProjectsAPI needs (pgstore.Projects).
type ProjectsAPIStore interface {
	ListActive(userID string) ([]domain.Project, error)
	ListAll(userID string) ([]domain.Project, error)
	GetByID(userID, id string) (domain.Project, error)
	EnsureBySlug(userID, name, slug string) (domain.Project, error)
	Upsert(in domain.Project, expectedVersion int64) (domain.Project, error)
}

// ProjectsAPIDeps bundles the projects API dependencies.
type ProjectsAPIDeps struct {
	Projects ProjectsAPIStore
	Bus      *sse.Broadcaster
}

// MountProjectsAPI registers the §7 project routes on r.
func MountProjectsAPI(r chi.Router, d ProjectsAPIDeps) {
	r.Get("/projects", d.handleList)
	r.Post("/projects", d.handleCreate)
	r.Get("/projects/{id}", d.handleGet)
	r.Put("/projects/{id}", d.handlePut)
}

type projectDTO struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Slug       string     `json:"slug"`
	ArchivedAt *time.Time `json:"archived_at"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt time.Time  `json:"last_used_at"`
	Version    int64      `json:"version"`
}

func toProjectDTO(p domain.Project) projectDTO {
	return projectDTO{
		ID: p.ID, Name: p.Name, Slug: p.Slug, ArchivedAt: p.ArchivedAt,
		CreatedAt: p.CreatedAt, LastUsedAt: p.LastUsedAt, Version: p.Version,
	}
}

func (d ProjectsAPIDeps) changed(userID string) {
	if d.Bus != nil {
		d.Bus.Changed(userID, "projects")
	}
}

func (d ProjectsAPIDeps) handleList(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	list := d.Projects.ListActive
	if r.URL.Query().Get("all") == "1" {
		list = d.Projects.ListAll
	}
	items, err := list(user.ID)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dtos := make([]projectDTO, 0, len(items))
	for _, p := range items {
		dtos = append(dtos, toProjectDTO(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": dtos})
}

func (d ProjectsAPIDeps) handleGet(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	p, err := d.Projects.GetByID(user.ID, chi.URLParam(r, "id"))
	if errors.Is(err, ports.ErrProjectNotFound) {
		apiError(w, http.StatusNotFound, "projekt existiert nicht")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toProjectDTO(p))
}

func (d ProjectsAPIDeps) handleCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	var in struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apiError(w, http.StatusBadRequest, "bad json")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		apiError(w, http.StatusUnprocessableEntity, "name fehlt")
		return
	}
	if in.Slug == "" {
		in.Slug = slugify(in.Name)
	}
	p, err := d.Projects.EnsureBySlug(user.ID, in.Name, in.Slug)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.changed(user.ID)
	writeJSON(w, http.StatusOK, toProjectDTO(p))
}

func (d ProjectsAPIDeps) handlePut(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	id := chi.URLParam(r, "id")
	expected, ok := ifMatchVersion(r)
	if !ok {
		apiError(w, http.StatusUnprocessableEntity, "If-Match-Header (Version) fehlt")
		return
	}
	var in struct {
		Name     string `json:"name"`
		Slug     string `json:"slug"`
		Archived bool   `json:"archived"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apiError(w, http.StatusBadRequest, "bad json")
		return
	}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Slug) == "" {
		apiError(w, http.StatusUnprocessableEntity, "name/slug fehlen")
		return
	}
	cur, err := d.Projects.GetByID(user.ID, id)
	if errors.Is(err, ports.ErrProjectNotFound) {
		apiError(w, http.StatusNotFound, "projekt existiert nicht")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	next := cur
	next.Name, next.Slug = in.Name, in.Slug
	if in.Archived && next.ArchivedAt == nil {
		now := time.Now().UTC()
		next.ArchivedAt = &now
	}
	if !in.Archived {
		next.ArchivedAt = nil
	}
	saved, err := d.Projects.Upsert(next, expected)
	if errors.Is(err, ports.ErrProjectVersionConflict) {
		writeJSON(w, http.StatusPreconditionFailed, map[string]any{"current": toProjectDTO(cur)})
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.changed(user.ID)
	writeJSON(w, http.StatusOK, toProjectDTO(saved))
}

// slugify is intentionally minimal: lowercase, spaces→dashes; alles
// Weitere regelt die UNIQUE(user_id, slug)-Constraint.
func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	return s
}
