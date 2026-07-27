package httpserver

import (
	"errors"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type setTagsReq struct {
	Tags []string `json:"tags"`
}

func (s *Server) handleGetNodeTags(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	tags, err := s.NodeTags.Get(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		if errors.Is(err, ports.ErrNodeNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if tags == nil {
		tags = []domain.Tag{}
	}
	writeJSON(w, http.StatusOK, tags)
}

func (s *Server) handleSetNodeTags(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req setTagsReq
	if !decodeJSONBody(w, r, &req, maxJSONBodyBytes, false) {
		return
	}
	id := r.PathValue("id")
	tags, err := s.NodeTags.Set(r.Context(), u.ID, id, req.Tags)
	if err != nil {
		if errors.Is(err, ports.ErrNodeNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if tags == nil {
		tags = []domain.Tag{}
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeUpdated, UserID: u.ID, Data: map[string]any{"id": id}})
	writeJSON(w, http.StatusOK, tags)
}
