package httpserver

import (
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

// projectsListData loads the owner's projects and applies the status filter.
// "" → active+paused (default view); "archived" → archived only; "all" → every status.
func (s *Server) projectsListData(r *http.Request, u domain.User) webui.ProjectsPageData {
	status := r.URL.Query().Get("status")
	all, _ := s.ListProjects.Execute(r.Context(), u.ID)
	var filtered []domain.Project
	for _, p := range all {
		switch status {
		case "all":
			filtered = append(filtered, p)
		case "archived":
			if p.Status == domain.ProjectArchived {
				filtered = append(filtered, p)
			}
		default: // active + paused
			if p.Status == domain.ProjectActive || p.Status == domain.ProjectPaused {
				filtered = append(filtered, p)
			}
		}
	}
	return webui.ProjectsPageData{User: u.Username, Status: status, Projects: filtered}
}

func (s *Server) handleWebProjectsHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d := s.projectsListData(r, u)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.ProjectsPage(d).Render(r.Context(), w)
}

func (s *Server) handleWebProjectsList(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d := s.projectsListData(r, u)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.ProjectsFragment(d).Render(r.Context(), w)
}
