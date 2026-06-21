package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// bindingReq is the JSON body for PUT /api/v1/projects/{id}/bindings.
type bindingReq struct {
	Kind         string `json:"kind"`
	RemoteSlug   string `json:"remoteSlug"`
	MachineID    string `json:"machineId"`
	MachineLabel string `json:"machineLabel"`
	Path         string `json:"path"`
}

// handleResolveProject handles GET /api/v1/projects/resolve.
// Query: ?slug=<remoteSlug>&machine=<machineID>&path=<cwd>
// Returns 200 Project | 404.
func (s *Server) handleResolveProject(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	q := r.URL.Query()
	slug := q.Get("slug")
	machine := q.Get("machine")
	cwd := q.Get("path")
	p, ok, err := s.ResolveProject.Execute(r.Context(), u.ID, slug, machine, cwd)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// handleBindProject handles PUT /api/v1/projects/{id}/bindings.
// Body: {kind, remoteSlug, machineId, machineLabel, path}
// Returns 200 ProjectBinding (upsert).
func (s *Server) handleBindProject(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	projectID := r.PathValue("id")
	var req bindingReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	key := usecase.BindKey{
		Kind:         domain.BindingKind(req.Kind),
		RemoteSlug:   req.RemoteSlug,
		MachineID:    req.MachineID,
		MachineLabel: req.MachineLabel,
		Path:         req.Path,
	}
	b, err := s.BindProject.Execute(r.Context(), u.ID, projectID, key)
	switch {
	case errors.Is(err, ports.ErrProjectNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		writeJSON(w, http.StatusOK, b)
	}
}

// handleUnbindProject handles DELETE /api/v1/projects/bindings.
// Query: ?kind=remote&slug=<remoteSlug>  |  ?kind=path&machine=<machineID>&path=<cwd>
// Returns 204 on success.
func (s *Server) handleUnbindProject(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	q := r.URL.Query()
	kind := domain.BindingKind(q.Get("kind"))
	key := usecase.BindKey{
		Kind:       kind,
		RemoteSlug: q.Get("slug"),
		MachineID:  q.Get("machine"),
		Path:       q.Get("path"),
	}
	if err := s.UnbindProject.Execute(r.Context(), u.ID, key); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListProjectBindingsByProject handles GET /api/v1/projects/{id}/bindings.
// Returns 200 [ProjectBinding…] for one project (WebUI panel).
func (s *Server) handleListProjectBindingsByProject(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	projectID := r.PathValue("id")
	bs, err := s.ListProjectBindings.ExecuteByProject(r.Context(), u.ID, projectID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if bs == nil {
		bs = []domain.ProjectBinding{}
	}
	writeJSON(w, http.StatusOK, bs)
}

// handleListAllProjectBindings handles GET /api/v1/projects/bindings.
// Returns 200 [ProjectBinding…] for the caller (CLI overview).
func (s *Server) handleListAllProjectBindings(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	bs, err := s.ListProjectBindings.Execute(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if bs == nil {
		bs = []domain.ProjectBinding{}
	}
	writeJSON(w, http.StatusOK, bs)
}
