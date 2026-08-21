package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/i18n"
)

// handleZeitHome renders the full Heute page on the AppShell at GET /zeit.
func (s *Server) handleZeitHome(w http.ResponseWriter, r *http.Request) {
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
// error banner. POST action handlers (add/edit/delete) funnel through
// renderDay, which delegates here; start/stop is owned by the K1 shell timer
// widget (webui_timer.go) since K3 Task 6.
func (s *Server) renderHeuteFragment(w http.ResponseWriter, r *http.Request, u domain.User, errMsg string) {
	vm, err := s.heuteDataFor(r.Context(), u, errMsg)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.HeuteFragment(vm).Render(r.Context(), w)
}

// heuteDataFor builds the Zeit (/zeit) view model — L4 Task 3 replaces the
// Kristall Saldo-Kacheln + Mo–Fr pace strip with the Lesesaal Tages-Ledger,
// the vertical 7-day Wochenskala, and the Werkzeuge menu.
func (s *Server) heuteDataFor(ctx context.Context, u domain.User, errMsg string) (webui.HeuteVM, error) {
	now := s.Clock.Now()
	loc := now.Location()
	day := startOfDay(now)
	sessions, err := s.ListSessionsRange.Execute(ctx, u.ID, day, day.AddDate(0, 0, 1))
	if err != nil {
		return webui.HeuteVM{}, err
	}
	projects, err := s.ListNodes.Execute(ctx, u.ID)
	if err != nil {
		return webui.HeuteVM{}, err
	}

	// The running timer is a singleton and may have been started on an EARLIER
	// day (e.g. left running overnight). Resolve it via GetRunningSession so it
	// stays visible on Zeit regardless of its start day; fall back to scanning
	// today's range for harnesses that don't wire the usecase.
	var running *domain.WorkSession
	if s.GetRunningSession.Sessions != nil {
		if r, ok, rerr := s.GetRunningSession.Execute(ctx, u.ID); rerr == nil && ok {
			rs := r
			running = &rs
		}
	} else {
		for i := range sessions {
			if sessions[i].Running() {
				r := sessions[i]
				running = &r
			}
		}
	}

	vm := webui.HeuteVM{
		User:      u.Username,
		Date:      day,
		Running:   running,
		DayParam:  day.Format(dayLayout),
		DateTitle: webui.FmtDayTitle(now),
		Err:       errMsg,
	}

	// Booking picker — bookable = Engagement/Vorhaben/Repo (Spec #1-Fix; was
	// KindEngagement-only). Status is intentionally left unfiltered, matching
	// the pre-fix behavior (no status check existed here before either) — a
	// broader kind filter must not also newly restrict by status.
	vm.Nodes = make([]components.NodePickerItem, 0, len(projects))
	for _, p := range projects {
		if !domain.IsBookable(p.Kind) {
			continue
		}
		vm.Nodes = append(vm.Nodes, components.NodePickerItem{
			ID:    p.ID,
			Name:  p.Name,
			Hue:   p.Color,
			Glyph: glyphOr(p.Glyph),
			Rate:  rateLabel(p.Rate),
		})
	}
	vm.HasProj = len(vm.Nodes) > 0

	// Today's ledger rows + per-row edit dialog (newest stay in chronological
	// order from the store). A running row's BaseSeconds feeds the LIVE row's
	// ticking data-timer span (heute.templ); it stays zero for completed rows.
	// DurationShort (Mockup clock format, Review Fix 1) and Note (Review Fix
	// 2) are Zeit-Hub-only additions kept off the shared SessionRowVM.
	vm.Ledger = make([]webui.HeuteLedgerRow, 0, len(sessions))
	for _, sess := range sessions {
		row := sessionRowVM(sess, projects, now)
		var base, since int64
		if row.Running {
			base = int64(sess.Elapsed(now) / time.Second)
			since = now.Unix() - base
		}
		vm.Ledger = append(vm.Ledger, webui.HeuteLedgerRow{
			Row:           row,
			Edit:          heuteEditDialogVM(sess, vm.Nodes, vm.DayParam),
			BaseSeconds:   base,
			SinceEpoch:    since,
			DurationShort: webui.FmtClockShort(sess.Elapsed(now)),
			Note:          sess.Note,
		})
	}

	// Diese Woche: the raw 7-day domain.WeekDay slice (Stats.Week, Mon..Sun) —
	// NOT the lossy WocheDayVM — feeds the vertical Wochenskala + the Soll-Zeile
	// (WeekGoalLine), reusing computeWocheSummary/onTrack (woche_summary.go)
	// verbatim rather than a second Rechenpfad.
	if s.Stats.Sessions != nil {
		if week, werr := s.Stats.Week(ctx, u.ID, time.Time{}); werr == nil && len(week) > 0 {
			offs := map[string]domain.DayOff{}
			if s.ListDayOffs.Store != nil {
				weekStart := week[0].Date
				weekEnd := week[len(week)-1].Date
				if offList, oerr := s.ListDayOffs.Execute(ctx, u.ID, weekStart, weekEnd); oerr == nil {
					for _, o := range offList {
						offs[o.Date.In(loc).Format("2006-01-02")] = o
					}
				}
			}
			vm.WeekDays = webui.BuildWeekBars(ctx, week, now, offs)
			sum := computeWocheSummary(week, offs, now)
			vm.WeekTotal = webui.FmtVerbose(sum.totalLogged)
			vm.WeekGoal = webui.FmtVerbose(sum.totalTarget)
			track := i18n.T(ctx, "zeit.onTrack")
			if !sum.onTrack() {
				track = i18n.T(ctx, "zeit.behind")
			}
			vm.WeekGoalLine = fmt.Sprintf(i18n.T(ctx, "zeit.weekGoal"), vm.WeekGoal, vm.WeekTotal, track)
		}
	}

	// Σ-Zeile (AllTimeSub, Mockup Z.851): owner-scoped all-time session scan
	// (s.ListSessions, since=zero) + the merged day-off count over the same
	// window — never a new/ungescoped read path (l4-global-constraints.md).
	if s.ListSessions.Sessions != nil {
		if all, aerr := s.ListSessions.Execute(ctx, u.ID, time.Time{}); aerr == nil {
			var total time.Duration
			var earliest time.Time
			for _, sess := range all {
				total += sess.Elapsed(now)
				if earliest.IsZero() || sess.Start.Before(earliest) {
					earliest = sess.Start
				}
			}
			since := earliest
			if since.IsZero() {
				since = now
			}
			dayoffCount := 0
			if s.ListDayOffs.Store != nil {
				if offs, oerr := s.ListDayOffs.Execute(ctx, u.ID, since, now.AddDate(1, 0, 0)); oerr == nil {
					dayoffCount = len(offs)
				}
			}
			vm.AllTimeSub = fmt.Sprintf(i18n.T(ctx, "zeit.allTimeSub"),
				webui.FmtVerbose(total), len(all), webui.FmtDayMonth(since), dayoffCount)
		}
	}

	vm.Tools = []webui.ZeitTool{
		{TitleKey: "zeit.tool.export", DescKey: "zeit.tool.export.desc", Href: "/export"},
		{TitleKey: "zeit.tool.dayoffs", DescKey: "zeit.tool.dayoffs.desc", Href: "/dayoffs"},
		{TitleKey: "zeit.tool.stats", DescKey: "zeit.tool.stats.desc", Href: "/woche"},
		{TitleKey: "zeit.tool.historie", DescKey: "zeit.tool.historie.desc", Href: "/historie"},
	}

	return vm, nil
}

// sessionRowVM maps a stored session to its list-row view model.
func sessionRowVM(sess domain.WorkSession, projects []domain.Node, now time.Time) components.SessionRowVM {
	name, hue := nodeIdentity(projects, sess.NodeID)
	glyph := nodeGlyph(projects, sess.NodeID)
	return components.SessionRowVM{
		ID:         sess.ID,
		Title:      name,
		Hue:        hue,
		Glyph:      glyph,
		Tags:       sess.Tags,
		TimeRange:  fmtClockRange(sess),
		Duration:   webui.FmtVerbose(sess.Elapsed(now)),
		Unassigned: sess.NodeID == nil,
		Running:    sess.Running(),
	}
}

// heuteEditDialogVM builds the per-row edit SessionDialogVM for the Heute
// ledger. A running session (no Stop) is not editable here — return the zero VM
// (Mode "" → the template skips its dialog). Nodes is the shown reassignment
// picker (preselected to the session's own node); SessionID + Date ride hidden
// so /ui/worktime/edit (form-based) resolves the target.
func heuteEditDialogVM(sess domain.WorkSession, nodes []components.NodePickerItem, dayParam string) components.SessionDialogVM {
	if sess.Stop == nil {
		return components.SessionDialogVM{}
	}
	nodeID := ""
	if sess.NodeID != nil {
		nodeID = *sess.NodeID
	}
	return components.SessionDialogVM{
		DialogID:  "edit-" + sess.ID,
		Mode:      "edit",
		Action:    "/ui/worktime/edit",
		Target:    "#content",
		SessionID: sess.ID,
		Date:      sess.Start.Local().Format("2006-01-02"),
		From:      sess.Start.Local().Format("15:04"),
		To:        sess.Stop.Local().Format("15:04"),
		Tag:       strings.Join(sess.Tags, " "),
		Note:      sess.Note,
		Nodes:     heutePickerNodes(nodes),
		NodeID:    nodeID,
	}
}

// heutePickerNodes converts the Heute booking picker's display items
// ([]components.NodePickerItem) into the []domain.Node shape the shared
// SessionDialog's picker field expects. Shared by heuteEditDialogVM and the
// add-dialog VM so the conversion lives in exactly one place.
func heutePickerNodes(items []components.NodePickerItem) []domain.Node {
	nodes := make([]domain.Node, 0, len(items))
	for _, n := range items {
		nodes = append(nodes, domain.Node{ID: n.ID, Name: n.Name})
	}
	return nodes
}

// fmtClockRange renders "09:00–11:00" (or "09:00–…" while running).
func fmtClockRange(s domain.WorkSession) string {
	start := s.Start.Local().Format("15:04")
	if s.Stop == nil {
		return start + "–…"
	}
	return start + "–" + s.Stop.Local().Format("15:04")
}

// nodeIdentity resolves a session's node id to (display name, hue).
// Returns "ohne Engagement" when the session is unassigned (Slice B: sessions
// carry an engagement id).
func nodeIdentity(nodes []domain.Node, id *string) (string, string) {
	if id == nil {
		return "ohne Engagement", ""
	}
	for _, n := range nodes {
		if n.ID == *id {
			return n.Name, n.Color
		}
	}
	return "ohne Engagement", ""
}

// nodeGlyph resolves a session's node glyph, defaulting to the unassigned
// hollow circle.
func nodeGlyph(nodes []domain.Node, id *string) string {
	if id == nil {
		return "○"
	}
	for _, n := range nodes {
		if n.ID == *id {
			return glyphOr(n.Glyph)
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

// rateLabel formats an optional per-hour rate as "95 €/h" or "—". The rule
// lives in webui.RateLabel — the view models need the same one, and two
// copies of a formatting rule drift.
func rateLabel(rate *domain.Money) string { return webui.RateLabel(rate) }

// isoWeek returns the ISO-8601 week number for t (still used by wocheDataFor,
// webui_woche.go).
func isoWeek(t time.Time) int {
	_, wk := t.ISOWeek()
	return wk
}
