package httpserver

import "net/http"

func (s *Server) handleRedesignDocTypes(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	dryRun := r.URL.Query().Get("dry_run") == "true"
	rep, err := s.RedesignDocTypes.Execute(r.Context(), u.ID, dryRun)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
