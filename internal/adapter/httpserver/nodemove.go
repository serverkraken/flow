package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

type moveNodeReq struct {
	ParentID *string `json:"parentId"` // null/absent = make root
}

// handleMoveNode handles POST /api/v1/nodes/{id}/move.
// Body: {"parentId": string|null}. Reparents (cycle-free, kind-checked).
func (s *Server) handleMoveNode(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req moveNodeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	n, err := s.MoveNode.Execute(r.Context(), u.ID, r.PathValue("id"), req.ParentID)
	switch {
	case errors.Is(err, usecase.ErrNodeCycle):
		http.Error(w, "move would create a cycle", http.StatusConflict)
	case errors.Is(err, ports.ErrNodeSlugTaken):
		http.Error(w, "the target parent already has a node with this slug", http.StatusConflict)
	case errors.Is(err, domain.ErrInvalidNode):
		http.Error(w, "invalid parent for this node kind", http.StatusBadRequest)
	case errors.Is(err, ports.ErrNodeNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeMoved, UserID: u.ID, Data: map[string]any{"id": n.ID, "parentId": n.ParentID, "name": n.Name, "node": n.ID}})
		writeJSON(w, http.StatusOK, n)
	}
}
