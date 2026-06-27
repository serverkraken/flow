package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// bindingReq is the JSON body for PUT /api/v1/nodes/{id}/bindings.
type bindingReq struct {
	Kind         string `json:"kind"`
	RemoteSlug   string `json:"remoteSlug"`
	MachineID    string `json:"machineId"`
	MachineLabel string `json:"machineLabel"`
	Path         string `json:"path"`
}

// handleResolveNode handles GET /api/v1/nodes/resolve.
// Query: ?slug=<remoteSlug>&machine=<machineID>&path=<cwd>
// Returns 200 Project | 404.
func (s *Server) handleResolveNode(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	q := r.URL.Query()
	slug := q.Get("slug")
	machine := q.Get("machine")
	cwd := q.Get("path")
	p, ok, err := s.ResolveNode.Execute(r.Context(), u.ID, slug, machine, cwd)
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

// handleBindNode handles PUT /api/v1/nodes/{id}/bindings.
// Body: {kind, remoteSlug, machineId, machineLabel, path}
// Returns 200 ProjectBinding (upsert).
func (s *Server) handleBindNode(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	nodeID := r.PathValue("id")
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
	b, err := s.BindNode.Execute(r.Context(), u.ID, nodeID, key)
	switch {
	case errors.Is(err, ports.ErrNodeNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, usecase.ErrInvalidBindTarget):
		http.Error(w, "binding target has the wrong kind (remote→repo, path→repo or leaf vorhaben)", http.StatusBadRequest)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		writeJSON(w, http.StatusOK, b)
	}
}

// handleUnbindNode handles DELETE /api/v1/nodes/bindings.
// Query: ?kind=remote&slug=<remoteSlug>  |  ?kind=path&machine=<machineID>&path=<cwd>
// Returns 204 on success.
func (s *Server) handleUnbindNode(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	q := r.URL.Query()
	kind := domain.BindingKind(q.Get("kind"))
	key := usecase.BindKey{
		Kind:       kind,
		RemoteSlug: q.Get("slug"),
		MachineID:  q.Get("machine"),
		Path:       q.Get("path"),
	}
	if err := s.UnbindNode.Execute(r.Context(), u.ID, key); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListNodeBindingsByNode handles GET /api/v1/nodes/{id}/bindings.
// Returns 200 [ProjectBinding…] for one project (WebUI panel).
func (s *Server) handleListNodeBindingsByNode(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	nodeID := r.PathValue("id")
	bs, err := s.ListNodeBindings.ExecuteByProject(r.Context(), u.ID, nodeID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if bs == nil {
		bs = []domain.ProjectBinding{}
	}
	writeJSON(w, http.StatusOK, bs)
}

// handleListAllNodeBindings handles GET /api/v1/nodes/bindings.
// Returns 200 [ProjectBinding…] for the caller (CLI overview).
func (s *Server) handleListAllNodeBindings(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	bs, err := s.ListNodeBindings.Execute(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if bs == nil {
		bs = []domain.ProjectBinding{}
	}
	writeJSON(w, http.StatusOK, bs)
}
