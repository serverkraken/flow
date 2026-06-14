package httpserver

import (
	"context"
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

func (s *Server) worktimeData(ctx context.Context, u domain.User) (webui.WorktimeData, error) {
	since := startOfDay(s.Clock.Now())
	sessions, err := s.ListSessions.Execute(ctx, u.ID, since)
	if err != nil {
		return webui.WorktimeData{}, err
	}
	projects, err := s.ListProjects.Execute(ctx, u.ID)
	if err != nil {
		return webui.WorktimeData{}, err
	}
	var running *domain.WorkSession
	for i := range sessions {
		if sessions[i].Running() {
			r := sessions[i]
			running = &r
		}
	}
	return webui.WorktimeData{
		User: u.Username, Running: running, Now: s.Clock.Now(),
		Sessions: sessions, Projects: projects,
	}, nil
}

func (s *Server) renderFragment(w http.ResponseWriter, r *http.Request, u domain.User) {
	d, err := s.worktimeData(r.Context(), u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.WorktimeFragment(d).Render(r.Context(), w)
}

func (s *Server) handleWebHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d, err := s.worktimeData(r.Context(), u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.WorktimePage(d).Render(r.Context(), w)
}

func (s *Server) handleWebFragment(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	s.renderFragment(w, r, u)
}

func (s *Server) handleWebStart(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	if _, err := s.StartSession.Execute(r.Context(), u.ID, nil, "", ""); err == nil {
		s.Bus.Publish(domain.Event{Type: domain.EventSessionStarted, UserID: u.ID})
	}
	s.renderFragment(w, r, u)
}

func (s *Server) handleWebStop(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	sessionID := r.FormValue("sessionId")
	projectID := r.FormValue("projectId")
	if name := r.FormValue("newProject"); name != "" {
		if p, err := s.CreateProject.Execute(r.Context(), u.ID, name, "", "", ""); err == nil {
			projectID = p.ID
			s.Bus.Publish(domain.Event{Type: domain.EventProjectCreated, UserID: u.ID})
		}
	}
	if _, err := s.StopSession.Execute(r.Context(), u.ID, sessionID, &projectID); err == nil {
		s.Bus.Publish(domain.Event{Type: domain.EventSessionStopped, UserID: u.ID})
	}
	s.renderFragment(w, r, u)
}
