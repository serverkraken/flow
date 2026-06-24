package httpserver

import (
	"errors"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
)

func (s *Server) renderFragment(w http.ResponseWriter, r *http.Request, u domain.User) {
	s.renderHeuteFragment(w, r, u, "")
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
	if _, err := s.StopSession.Execute(r.Context(), u.ID, sessionID, &projectID); err != nil {
		// Booking is mandatory: surface the reason instead of silently leaving the
		// timer running (otherwise Stop appears to "do nothing").
		msg := "Sitzung konnte nicht gestoppt werden."
		if errors.Is(err, domain.ErrProjectRequired) {
			msg = "Bitte ein Projekt wählen, um die Sitzung zu stoppen."
		}
		s.renderHeuteFragment(w, r, u, msg)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventSessionStopped, UserID: u.ID})
	s.renderFragment(w, r, u)
}
