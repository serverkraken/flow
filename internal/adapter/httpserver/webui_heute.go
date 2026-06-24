package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// heuteDataFor builds the Heute (today) view model from the same data sources as
// worktimeDataFor (sessions/projects) plus the stats target math, shaped for the
// Slice-0 AppShell components.
// handleHeuteHome renders the full Heute page on the AppShell at GET /.
func (s *Server) handleHeuteHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	vm, err := s.heuteDataFor(r.Context(), u, "")
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.HeutePage(vm).Render(r.Context(), w)
}

// handleHeuteFragment renders the inner content fragment at GET /ui/worktime
// (the SSE-swap target).
func (s *Server) handleHeuteFragment(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	s.renderHeuteFragment(w, r, u, "")
}

// renderHeuteFragment re-renders the Heute fragment, optionally with an inline
// error banner. POST action handlers (start/stop/add/edit/delete) funnel through
// renderFragment/renderDay which now delegate here.
func (s *Server) renderHeuteFragment(w http.ResponseWriter, r *http.Request, u domain.User, errMsg string) {
	vm, err := s.heuteDataFor(r.Context(), u, errMsg)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.HeuteFragment(vm).Render(r.Context(), w)
}

func (s *Server) heuteDataFor(ctx context.Context, u domain.User, errMsg string) (webui.HeuteVM, error) {
	now := s.Clock.Now()
	day := startOfDay(now)
	sessions, err := s.ListSessionsRange.Execute(ctx, u.ID, day, day.AddDate(0, 0, 1))
	if err != nil {
		return webui.HeuteVM{}, err
	}
	projects, err := s.ListProjects.Execute(ctx, u.ID)
	if err != nil {
		return webui.HeuteVM{}, err
	}

	var running *domain.WorkSession
	for i := range sessions {
		if sessions[i].Running() {
			r := sessions[i]
			running = &r
		}
	}

	vm := webui.HeuteVM{
		User:     u.Username,
		Date:     day,
		Running:  running,
		DayParam: day.Format(dayLayout),
		HasProj:  len(projects) > 0,
		Err:      errMsg,
	}

	// Project pickers.
	vm.Projects = make([]components.FuzzyProjectVM, 0, len(projects))
	for _, p := range projects {
		vm.Projects = append(vm.Projects, components.FuzzyProjectVM{
			ID:    p.ID,
			Name:  p.Name,
			Hue:   p.Color,
			Glyph: glyphOr(p.Glyph),
			Rate:  rateLabel(p.Rate),
		})
	}

	// Today's session rows (newest stay in chronological order from the store).
	vm.Rows = make([]components.SessionRowVM, 0, len(sessions))
	for _, sess := range sessions {
		vm.Rows = append(vm.Rows, sessionRowVM(sess, projects, now))
	}

	if running != nil {
		vm.RunningBase = heuteRunningBase(*running, now)
		vm.StartedAt = running.Start.Local().Format("15:04")
		vm.RunningTag = running.Tag
		name, hue := projectIdentity(projects, running.ProjectID)
		vm.RunningName = name
		vm.RunningHue = hue
	}

	// Daily target + balance (reuse the stats computation; degrade to zero if the
	// stats usecase is not wired, e.g. in narrow tests).
	if s.Stats.Sessions != nil {
		if today, terr := s.Stats.Today(ctx, u.ID); terr == nil {
			vm.LoggedDur = webui.FmtVerbose(today.Logged)
			vm.TargetDur = webui.FmtVerbose(today.Target)
			if today.Target > 0 {
				vm.TargetPct = webui.ClampPct(int(today.Logged * 100 / today.Target))
			}
			vm.TargetVar = heuteTargetVariant(today, running != nil)
			vm.Balance = webui.FmtSaldoVerbose(today.Saldo)
			vm.BalancePos = today.Saldo >= 0
		}
		// Week pace strip (Mon..Fri).
		if week, werr := s.Stats.Week(ctx, u.ID, time.Time{}); werr == nil {
			vm.WeekKW = fmt.Sprintf("KW %d", isoWeek(now))
			vm.WeekRows, vm.WeekTotal, vm.WeekGoal = heuteWeekRows(week, now)
		}
	}

	return vm, nil
}

// sessionRowVM maps a stored session to its list-row view model.
func sessionRowVM(sess domain.WorkSession, projects []domain.Project, now time.Time) components.SessionRowVM {
	name, hue := projectIdentity(projects, sess.ProjectID)
	glyph := projectGlyph(projects, sess.ProjectID)
	return components.SessionRowVM{
		ID:         sess.ID,
		Title:      name,
		Hue:        hue,
		Glyph:      glyph,
		Tag:        sess.Tag,
		TimeRange:  fmtClockRange(sess),
		Duration:   webui.FmtVerbose(sess.Elapsed(now)),
		Unassigned: sess.ProjectID == nil,
		Running:    sess.Running(),
	}
}

// fmtClockRange renders "09:00–11:00" (or "09:00–…" while running).
func fmtClockRange(s domain.WorkSession) string {
	start := s.Start.Local().Format("15:04")
	if s.Stop == nil {
		return start + "–…"
	}
	return start + "–" + s.Stop.Local().Format("15:04")
}

// projectIdentity resolves a session's project id to (display name, hue).
func projectIdentity(projects []domain.Project, id *string) (string, string) {
	if id == nil {
		return "ohne Projekt", ""
	}
	for _, p := range projects {
		if p.ID == *id {
			return p.Name, p.Color
		}
	}
	return "ohne Projekt", ""
}

// projectGlyph resolves a session's project glyph, defaulting to the unassigned
// hollow circle.
func projectGlyph(projects []domain.Project, id *string) string {
	if id == nil {
		return "○"
	}
	for _, p := range projects {
		if p.ID == *id {
			return glyphOr(p.Glyph)
		}
	}
	return "○"
}

// glyphOr returns the project glyph or a default identity glyph when unset.
func glyphOr(g string) string {
	if g == "" {
		return "◆"
	}
	return g
}

// rateLabel formats an optional per-hour rate as "95 €/h" or "—".
func rateLabel(rate *domain.Money) string {
	if rate == nil {
		return "—"
	}
	sym := rate.Currency
	if rate.Currency == "EUR" {
		sym = "€"
	}
	return fmt.Sprintf("%d %s/h", rate.Amount/100, sym)
}

// heuteRunningBase returns the running session's elapsed seconds for the
// live-timer data-base seed.
func heuteRunningBase(running domain.WorkSession, now time.Time) int {
	sec := int(now.Sub(running.Start) / time.Second)
	if sec < 0 {
		sec = 0
	}
	return sec
}

// heuteTargetVariant picks the progress-bar variant for the day's progress.
func heuteTargetVariant(today usecase.TodaySummary, running bool) string {
	switch {
	case running:
		return "running"
	case today.Saldo > 0:
		return "over"
	case today.Saldo == 0 && today.Target > 0:
		return "hit"
	default:
		return "under"
	}
}

// isoWeek returns the ISO-8601 week number for t.
func isoWeek(t time.Time) int {
	_, wk := t.ISOWeek()
	return wk
}

// heuteWeekRows maps the stats week days into Mon..Fri pace rows and returns the
// week total + goal labels (workweek excludes weekends, matching the worktime
// parity rules).
func heuteWeekRows(week []domain.WeekDay, now time.Time) ([]webui.HeuteWeekRow, string, string) {
	labels := map[time.Weekday]string{
		time.Monday: "Mo", time.Tuesday: "Di", time.Wednesday: "Mi",
		time.Thursday: "Do", time.Friday: "Fr",
	}
	rows := make([]webui.HeuteWeekRow, 0, 5)
	var total, goal time.Duration
	for _, wd := range week {
		label, ok := labels[wd.Date.Weekday()]
		if !ok {
			continue // skip weekends
		}
		logged := wd.Total(now)
		total += logged
		goal += wd.Target
		pct := 0
		if wd.Target > 0 {
			pct = webui.ClampPct(int(logged * 100 / wd.Target))
		}
		state := "missed"
		switch {
		case wd.IsToday:
			state = "today"
		case wd.Target > 0 && logged >= wd.Target:
			state = "hit"
		}
		rows = append(rows, webui.HeuteWeekRow{
			Label:   label,
			Logged:  webui.FmtVerbose(logged),
			Pct:     pct,
			State:   state,
			IsToday: wd.IsToday,
		})
	}
	return rows, webui.FmtVerbose(total), webui.FmtVerbose(goal)
}
