package httpserver

import (
	"encoding/json"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
)

type setTagsReq struct {
	Tags []string `json:"tags"`
}

func (s *Server) handleGetNodeTags(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	tags, err := s.GetTags.Execute(r.Context(), u.ID, domain.TaggableNode, r.PathValue("id"))
	if err != nil {
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")
	tags, err := s.SetTags.Execute(r.Context(), u.ID, domain.TaggableNode, id, req.Tags)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if tags == nil {
		tags = []domain.Tag{}
	}
	s.Bus.Publish(domain.Event{Type: domain.EventNodeUpdated, UserID: u.ID, Data: map[string]any{"id": id}})
	writeJSON(w, http.StatusOK, tags)
}
