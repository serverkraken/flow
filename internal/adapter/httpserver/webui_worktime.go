package httpserver

// SSE event constants used for WebUI worktime mutations:
//   domain.EventSessionUpdated ("session.updated") — add / edit completed sessions
//   domain.EventSessionDeleted ("session.deleted") — delete a session
// These match the truthful set in internal/domain/event.go and the existing
// handleEditSession / handleDeleteSession in worktime.go.

import (
	"context"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
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

// worktimeDataFor builds the worktime view model for a specific local day.
func (s *Server) worktimeDataFor(ctx context.Context, u domain.User, day time.Time, errMsg string) (webui.WorktimeData, error) {
	day = startOfDay(day)
	sessions, err := s.ListSessionsRange.Execute(ctx, u.ID, day, day.AddDate(0, 0, 1))
	if err != nil {
		return webui.WorktimeData{}, err
	}
	projects, err := s.ListProjects.Execute(ctx, u.ID)
	if err != nil {
		return webui.WorktimeData{}, err
	}
	bindings, err := s.ListProjectBindings.Execute(ctx, u.ID)
	if err != nil {
		return webui.WorktimeData{}, err
	}
	today := startOfDay(s.Clock.Now())
	isToday := day.Equal(today)
	var running *domain.WorkSession
	if isToday {
		for i := range sessions {
			if sessions[i].Running() {
				r := sessions[i]
				running = &r
			}
		}
	}
	next := day.AddDate(0, 0, 1)
	return webui.WorktimeData{
		User:       u.Username,
		Running:    running,
		Now:        s.Clock.Now(),
		Sessions:   sessions,
		Projects:   projects,
		Bindings:   bindings,
		Date:       day,
		IsToday:    isToday,
		PrevDate:   day.AddDate(0, 0, -1).Format(dayLayout),
		NextDate:   next.Format(dayLayout),
		CanForward: !next.After(today), // clamp: never navigate past today
		Err:        errMsg,
	}, nil
}

// renderDay re-renders the worktime fragment for the given local day,
// optionally with an inline error banner.
func (s *Server) renderDay(w http.ResponseWriter, r *http.Request, u domain.User, day time.Time, errMsg string) {
	d, err := s.worktimeDataFor(r.Context(), u, day, errMsg)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.WorktimeFragment(d).Render(r.Context(), w)
}

// resolveWebProject returns the chosen project id from the form, creating a
// new project when "newProject" is filled. Returns nil when neither is set.
func (s *Server) resolveWebProject(r *http.Request, u domain.User) *string {
	projectID := r.FormValue("projectId")
	if name := r.FormValue("newProject"); name != "" {
		if p, err := s.CreateProject.Execute(r.Context(), u.ID, name, "", "", ""); err == nil {
			projectID = p.ID
			s.Bus.Publish(domain.Event{Type: domain.EventProjectCreated, UserID: u.ID})
		}
	}
	if projectID == "" {
		return nil
	}
	return &projectID
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
	projectID := s.resolveWebProject(r, u)
	if _, err := s.AddSession.Execute(r.Context(), u.ID, projectID, start, stop,
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
	projectID := s.resolveWebProject(r, u)
	if _, err := s.EditSession.Execute(r.Context(), u.ID, r.FormValue("sessionId"),
		usecase.EditSessionInput{
			ProjectID: projectID,
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
