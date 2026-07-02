package httpserver

import (
	"errors"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

func (s *Server) renderFragment(w http.ResponseWriter, r *http.Request, u domain.User) {
	s.renderHeuteFragment(w, r, u, "")
}

func (s *Server) handleWebStart(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	if sess, err := s.StartSession.Execute(r.Context(), u.ID, nil, nil, ""); err == nil {
		s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStarted, UserID: u.ID,
			Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID)})
	}
	s.renderFragment(w, r, u)
}

func (s *Server) handleWebStop(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	sessionID := r.FormValue("sessionId")
	nodeID := r.FormValue("projectId")
	if name := r.FormValue("newProject"); name != "" {
		if p, err := s.CreateNode.Execute(r.Context(), u.ID, usecase.CreateNodeInput{Name: name, Kind: domain.KindEngagement}); err == nil {
			nodeID = p.ID
			s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeCreated, UserID: u.ID, Data: map[string]any{"id": p.ID, "name": p.Name}})
		}
	}
	sess, err := s.StopSession.Execute(r.Context(), u.ID, sessionID, &nodeID)
	if err != nil {
		// Booking is mandatory: surface the reason instead of silently leaving the
		// timer running (otherwise Stop appears to "do nothing").
		msg := "Sitzung konnte nicht gestoppt werden."
		if errors.Is(err, domain.ErrProjectRequired) {
			msg = "Bitte ein Projekt wählen, um die Sitzung zu stoppen."
		}
		s.renderHeuteFragment(w, r, u, msg)
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStopped, UserID: u.ID,
		Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID)})
	s.renderFragment(w, r, u)
}
