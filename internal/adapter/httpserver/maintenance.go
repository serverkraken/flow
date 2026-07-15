package httpserver

import (
	"net/http"
)

func (s *Server) handleStripFrontmatter(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	dryRun := r.URL.Query().Get("dry_run") == "true"
	rep, err := s.StripFrontmatter.Execute(r.Context(), u.ID, dryRun)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *Server) handleAuditDocuments(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	report, err := s.AuditDocuments.Execute(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
