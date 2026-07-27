package httpserver

import (
	"net/http"
	"time"
)

type nodeMRUDTO struct {
	NodeID       string `json:"nodeId"`
	LastBookedAt string `json:"lastBookedAt"`
}

func (s *Server) handleNodeMRU(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	entries, err := s.NodeMRU.Execute(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	out := make([]nodeMRUDTO, 0, len(entries))
	for _, e := range entries {
		out = append(out, nodeMRUDTO{NodeID: e.NodeID, LastBookedAt: e.LastBookedAt.Format(time.RFC3339)})
	}
	writeJSON(w, http.StatusOK, out)
}
