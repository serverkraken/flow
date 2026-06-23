package httpserver

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
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
			// Skip active sessions (no stop time). This diverges from the worktime
			// day-view, which counts running sessions via Elapsed(now): the cockpit
			// Σ is a settled-time summary, so running time is excluded until stopped.
			continue
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

// ---------------------------------------------------------------------------
// Project form helpers
// ---------------------------------------------------------------------------

// parseRate reads the optional rate form fields. Blank amount → (nil, nil) =
// "clear the rate". A malformed amount → error (re-rendered as a form error).
func parseRate(amount, currency string) (*domain.Money, error) {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return nil, nil
	}
	f, err := strconv.ParseFloat(amount, 64)
	if err != nil || f < 0 {
		return nil, fmt.Errorf("ungültiger Satz %q", amount)
	}
	cur := strings.TrimSpace(currency)
	if cur == "" {
		cur = "EUR"
	}
	return &domain.Money{Amount: int64(f*100 + 0.5), Currency: cur}, nil
}

func formValues(r *http.Request) webui.ProjectFormValues {
	return webui.ProjectFormValues{
		Name:         r.FormValue("name"),
		Slug:         r.FormValue("slug"),
		Description:  r.FormValue("description"),
		UpstreamGit:  r.FormValue("upstreamGit"),
		Status:       r.FormValue("status"),
		Color:        r.FormValue("color"),
		Glyph:        r.FormValue("glyph"),
		RateAmount:   r.FormValue("rateAmount"),
		RateCurrency: r.FormValue("rateCurrency"),
	}
}

// orStatus defaults an empty status form value to "active".
func orStatus(s string) string {
	if s == "" {
		return "active"
	}
	return s
}

// ---------------------------------------------------------------------------
// Project form handlers
// ---------------------------------------------------------------------------

func (s *Server) handleWebProjectNew(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.ProjectForm(webui.ProjectFormData{
		User: u.Username,
		Vals: webui.ProjectFormValues{Status: "active"},
	}, nil).Render(r.Context(), w)
}

func (s *Server) handleWebProjectCreate(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	vals := formValues(r)
	rate, rerr := parseRate(vals.RateAmount, vals.RateCurrency)
	reRender := func(msg string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = webui.ProjectForm(webui.ProjectFormData{User: u.Username, Error: msg, Vals: vals}, nil).Render(r.Context(), w)
	}
	if vals.Name == "" {
		reRender("Name erforderlich")
		return
	}
	if rerr != nil {
		reRender(rerr.Error())
		return
	}
	// Reject a bad upstream up front so we never create a half-configured project
	// (mirrors REST handleCreateProject). Bad git input is the common error path;
	// without this guard CreateProject would succeed and the later UpdateProject
	// failure would leave an orphan name-only project behind.
	if vals.UpstreamGit != "" {
		if _, ok := domain.NormalizeRemoteSlug(vals.UpstreamGit); !ok {
			reRender("Ungültige Upstream-Git-URL")
			return
		}
	}
	// create (name/slug/color/glyph) — same compose sequence as REST handleCreateProject
	p, err := s.CreateProject.Execute(r.Context(), u.ID, vals.Name, vals.Slug, vals.Color, vals.Glyph)
	if err != nil {
		reRender("Konnte Projekt nicht anlegen: " + err.Error())
		return
	}
	// compose description/upstream/status (auto-syncs binding; validates upstream)
	p, err = s.UpdateProject.Execute(r.Context(), u.ID, p.ID, usecase.UpdateProjectInput{
		Name:        p.Name,
		Slug:        p.Slug,
		Color:       p.Color,
		Glyph:       p.Glyph,
		Description: vals.Description,
		UpstreamGit: vals.UpstreamGit,
		Status:      domain.ProjectStatus(orStatus(vals.Status)),
	})
	if err != nil {
		reRender(err.Error())
		return
	}
	if rate != nil {
		_ = s.SetProjectRate.Execute(r.Context(), u.ID, p.ID, rate)
	}
	s.Bus.Publish(domain.Event{Type: domain.EventProjectCreated, UserID: u.ID, Data: map[string]any{"id": p.ID}})
	http.Redirect(w, r, "/projects/"+p.ID, http.StatusSeeOther)
}

func (s *Server) handleWebProjectEdit(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	p, err := s.GetProject.Execute(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	vals := webui.ProjectFormValues{
		Name:        p.Name,
		Slug:        p.Slug,
		Description: p.Description,
		UpstreamGit: p.UpstreamGit,
		Status:      string(p.Status),
		Color:       p.Color,
		Glyph:       p.Glyph,
	}
	if p.Rate != nil {
		vals.RateAmount = fmt.Sprintf("%d.%02d", p.Rate.Amount/100, p.Rate.Amount%100)
		vals.RateCurrency = p.Rate.Currency
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.ProjectForm(webui.ProjectFormData{User: u.Username, Vals: vals}, &p).Render(r.Context(), w)
}

func (s *Server) handleWebProjectUpdate(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	vals := formValues(r)
	rate, rerr := parseRate(vals.RateAmount, vals.RateCurrency)
	cur, gerr := s.GetProject.Execute(r.Context(), u.ID, id)
	if gerr != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	reRender := func(msg string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = webui.ProjectForm(webui.ProjectFormData{User: u.Username, Error: msg, Vals: vals}, &cur).Render(r.Context(), w)
	}
	if rerr != nil {
		reRender(rerr.Error())
		return
	}
	p, err := s.UpdateProject.Execute(r.Context(), u.ID, id, usecase.UpdateProjectInput{
		Name:        vals.Name,
		Slug:        vals.Slug,
		Color:       vals.Color,
		Glyph:       vals.Glyph,
		Description: vals.Description,
		UpstreamGit: vals.UpstreamGit,
		Status:      domain.ProjectStatus(orStatus(vals.Status)),
	})
	switch {
	case errors.Is(err, ports.ErrProjectNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case errors.Is(err, domain.ErrInvalidProject) || errors.Is(err, domain.ErrInvalidUpstream):
		reRender(err.Error())
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	// rate==nil clears any existing rate
	_ = s.SetProjectRate.Execute(r.Context(), u.ID, id, rate)
	s.Bus.Publish(domain.Event{Type: domain.EventProjectUpdated, UserID: u.ID, Data: map[string]any{"id": p.ID}})
	http.Redirect(w, r, "/projects/"+id, http.StatusSeeOther)
}

// handleWebProjectStatus applies a single status transition (full-replace
// UpdateProject preserving current fields).
func (s *Server) handleWebProjectStatus(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	cur, err := s.GetProject.Execute(r.Context(), u.ID, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	_, err = s.UpdateProject.Execute(r.Context(), u.ID, id, usecase.UpdateProjectInput{
		Name:        cur.Name,
		Slug:        cur.Slug,
		Color:       cur.Color,
		Glyph:       cur.Glyph,
		Description: cur.Description,
		UpstreamGit: cur.UpstreamGit,
		Status:      domain.ProjectStatus(r.FormValue("status")),
	})
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventProjectUpdated, UserID: u.ID, Data: map[string]any{"id": id}})
	http.Redirect(w, r, "/projects/"+id, http.StatusSeeOther)
}

func (s *Server) handleWebProjectDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	if err := s.DeleteProject.Execute(r.Context(), u.ID, id); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventProjectDeleted, UserID: u.ID, Data: map[string]any{"id": id}})
	http.Redirect(w, r, "/projects", http.StatusSeeOther)
}
