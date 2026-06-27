package httpserver

// SSE event constants used for WebUI worktime mutations:
//   domain.EventSessionUpdated ("session.updated") — add / edit completed sessions
//   domain.EventSessionDeleted ("session.deleted") — delete a session
// These match the truthful set in internal/domain/event.go and the existing
// handleEditSession / handleDeleteSession in worktime.go.

import (
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// dayLayout is the yyyy-mm-dd form used by the date-nav query param and forms.
const dayLayout = "2006-01-02"

// parseDayParam resolves a yyyy-mm-dd query value to a local start-of-day,
// falling back to today on empty/invalid input.
func parseDayParam(s *Server, q string) time.Time {
	if t, err := time.ParseInLocation(dayLayout, q, time.Local); err == nil {
		return startOfDay(t)
	}
	return startOfDay(s.Clock.Now())
}

// dayTime combines a start-of-day date with an HH:MM clock time in local tz.
func dayTime(day time.Time, hhmm string) (time.Time, error) {
	clock, err := time.ParseInLocation("15:04", hhmm, time.Local)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(day.Year(), day.Month(), day.Day(),
		clock.Hour(), clock.Minute(), 0, 0, time.Local), nil
}

// renderDay re-renders the Heute fragment (today) after a mutating action,
// optionally with an inline error banner. The day argument is retained for the
// add/edit/delete call signatures but the Heute page is always today-scoped.
func (s *Server) renderDay(w http.ResponseWriter, r *http.Request, u domain.User, _ time.Time, errMsg string) {
	s.renderHeuteFragment(w, r, u, errMsg)
}

// resolveWebProject returns the chosen project id from the form, creating a
// new project when "newProject" is filled. Returns nil when neither is set.
func (s *Server) resolveWebProject(r *http.Request, u domain.User) *string {
	nodeID := r.FormValue("projectId")
	if name := r.FormValue("newProject"); name != "" {
		if p, err := s.CreateNode.Execute(r.Context(), u.ID, name, "", "", ""); err == nil {
			nodeID = p.ID
			s.Bus.Publish(domain.Event{Type: domain.EventNodeCreated, UserID: u.ID})
		}
	}
	if nodeID == "" {
		return nil
	}
	return &nodeID
}

func (s *Server) handleWebAdd(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	day := parseDayParam(s, r.FormValue("date"))
	start, err1 := dayTime(day, r.FormValue("from"))
	stop, err2 := dayTime(day, r.FormValue("to"))
	if err1 != nil || err2 != nil {
		s.renderDay(w, r, u, day, "invalid time — use HH:MM")
		return
	}
	if !stop.After(start) {
		s.renderDay(w, r, u, day, "to must be after from")
		return
	}
	nodeID := s.resolveWebProject(r, u)
	if _, err := s.AddSession.Execute(r.Context(), u.ID, nodeID, start, stop,
		r.FormValue("tag"), r.FormValue("note")); err != nil {
		s.renderDay(w, r, u, day, "could not add: "+err.Error()) // err includes "overlap"
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventSessionUpdated, UserID: u.ID})
	s.renderDay(w, r, u, day, "")
}

func (s *Server) handleWebDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	day := parseDayParam(s, r.FormValue("date"))
	if err := s.DeleteSession.Execute(r.Context(), u.ID, r.FormValue("sessionId")); err != nil {
		s.renderDay(w, r, u, day, "could not delete: "+err.Error())
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventSessionDeleted, UserID: u.ID})
	s.renderDay(w, r, u, day, "")
}

func (s *Server) handleWebEdit(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	day := parseDayParam(s, r.FormValue("date"))
	start, err1 := dayTime(day, r.FormValue("from"))
	stop, err2 := dayTime(day, r.FormValue("to"))
	if err1 != nil || err2 != nil || !stop.After(start) {
		s.renderDay(w, r, u, day, "invalid time range")
		return
	}
	nodeID := s.resolveWebProject(r, u)
	if _, err := s.EditSession.Execute(r.Context(), u.ID, r.FormValue("sessionId"),
		usecase.EditSessionInput{
			NodeID: nodeID,
			Tag:       r.FormValue("tag"),
			Note:      r.FormValue("note"),
			Start:     start,
			Stop:      &stop,
		}); err != nil {
		s.renderDay(w, r, u, day, "could not edit: "+err.Error())
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventSessionUpdated, UserID: u.ID})
	s.renderDay(w, r, u, day, "")
}
