package httpserver

import (
	"context"
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

// worktimeData builds the today-only view model (used by start/stop which are
// always today operations).
func (s *Server) worktimeData(ctx context.Context, u domain.User) (webui.WorktimeData, error) {
	today := startOfDay(s.Clock.Now())
	return s.worktimeDataFor(ctx, u, today, "")
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
	day := parseDayParam(s, r.URL.Query().Get("date"))
	d, err := s.worktimeDataFor(r.Context(), u, day, "")
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.WorktimePage(d).Render(r.Context(), w)
}

func (s *Server) handleWebFragment(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	day := parseDayParam(s, r.URL.Query().Get("date"))
	d, err := s.worktimeDataFor(r.Context(), u, day, "")
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.WorktimeFragment(d).Render(r.Context(), w)
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
