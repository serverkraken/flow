package httpserver

import (
	"errors"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
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

// projectWorktime aggregates the project's sessions into total/week/month hour
// counts and computes the earnings string when p.Rate != nil.
// Sessions are fetched for the full project lifetime (year 2000 to now+1d)
// and filtered in-process by ProjectID to avoid a new backend usecase.
func (s *Server) projectWorktime(r *http.Request, u domain.User, p domain.Project) (totalH, weekH, monthH float64, earnings string) {
	ctx := r.Context()
	now := s.Clock.Now()
	// Fetch all sessions for the owner from a wide window and filter by project.
	since := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	until := now.AddDate(0, 0, 1)
	sessions, err := s.ListSessionsRange.Execute(ctx, u.ID, since, until)
	if err != nil {
		return
	}

	// week/month boundaries (local time)
	weekStart := startOfWeek(now.Local())
	monthStart := startOfMonth(now.Local())

	var totalDur, weekDur, monthDur time.Duration
	for _, sess := range sessions {
		if sess.ProjectID == nil || *sess.ProjectID != p.ID {
			continue
		}
		if sess.Running() {
			continue // skip active sessions (no stop time)
		}
		d := sess.Elapsed(now)
		totalDur += d
		if !sess.Start.Before(weekStart) {
			weekDur += d
		}
		if !sess.Start.Before(monthStart) {
			monthDur += d
		}
	}

	totalH = totalDur.Hours()
	weekH = weekDur.Hours()
	monthH = monthDur.Hours()

	if p.Rate != nil {
		amt := p.Rate.Mul(totalDur)
		earnings = amt.String()
	}
	return
}

// startOfWeek returns the Monday 00:00:00 of the week containing t (local).
func startOfWeek(t time.Time) time.Time {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7 // Sunday → 7
	}
	return time.Date(t.Year(), t.Month(), t.Day()-wd+1, 0, 0, 0, 0, t.Location())
}

// startOfMonth returns the first day of the month containing t (local).
func startOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

// projectCockpitData assembles the full cockpit view model: project, rendered
// description, per-project worktime aggregate, scoped docs, and bindings.
// Read-only; aggregation is done here without a new backend usecase.
func (s *Server) projectCockpitData(r *http.Request, u domain.User, id string) (webui.ProjectCockpit, error) {
	p, err := s.GetProject.Execute(r.Context(), u.ID, id)
	if err != nil {
		return webui.ProjectCockpit{}, err
	}
	d := webui.ProjectCockpit{User: u.Username, P: p}
	// Description: render markdown; wikilinks resolve to nothing for the cockpit.
	if p.Description != "" {
		d.DescriptionHTML = webui.RenderDocument(p.Description, func(string) (string, string, bool) { return "", "", false })
	}
	// Worktime aggregate.
	d.TotalHours, d.WeekHours, d.MonthHours, d.Earnings = s.projectWorktime(r, u, p)
	// Project-scoped documents.
	pid := p.ID
	d.Docs, _ = s.ListDocuments.Execute(r.Context(), u.ID, &pid, nil)
	// Bindings (read-only).
	bindings, _ := s.ListProjectBindings.ExecuteByProject(r.Context(), u.ID, p.ID)
	d.Bindings = bindings
	return d, nil
}

func (s *Server) handleWebProjectView(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d, err := s.projectCockpitData(r, u, r.PathValue("id"))
	if errors.Is(err, ports.ErrProjectNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.ProjectView(d).Render(r.Context(), w)
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
