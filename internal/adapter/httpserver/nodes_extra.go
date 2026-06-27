package httpserver

import (
	"errors"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// handleNodeAncestors handles GET /api/v1/nodes/{id}/ancestors (leaf→root).
func (s *Server) handleNodeAncestors(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	chain, err := s.NodeAncestors.Execute(r.Context(), u.ID, r.PathValue("id"))
	switch {
	case errors.Is(err, ports.ErrNodeNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		if chain == nil {
			chain = []domain.Node{}
		}
		writeJSON(w, http.StatusOK, chain)
	}
}

// handleResolveEngagement handles GET /api/v1/nodes/resolve-engagement.
// Query: ?slug=&machine=&path= (same as /resolve). Returns 200 engagement | 404.
func (s *Server) handleResolveEngagement(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	q := r.URL.Query()
	n, ok, err := s.ResolveEngagement.Execute(r.Context(), u.ID, q.Get("slug"), q.Get("machine"), q.Get("path"))
	switch {
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	case !ok:
		http.Error(w, "not found", http.StatusNotFound)
	default:
		writeJSON(w, http.StatusOK, n)
	}
}
