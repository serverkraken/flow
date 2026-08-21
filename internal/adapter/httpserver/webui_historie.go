package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

const (
	historieHourPx   = 48 // one hour = 48px (matches the --hour CSS token)
	historiePageSize = 50
)

// gridWindow returns the [floor,ceil] minute-of-day band for the week's grid:
// default 06:00–20:00, expanded down/up to the hour to fit any out-of-band
// session start/stop, clamped into [0,1440]. Running sessions pass `now` as
// their stop minute via the caller.
func gridWindow(mins []int) (int, int) {
	floor, ceil := 360, 1200 // 06:00, 20:00
	for _, m := range mins {
		if m < floor {
			floor = (m / 60) * 60 // snap down to the hour
		}
		if m > ceil {
			ceil = ((m + 59) / 60) * 60 // snap up to the hour
		}
	}
	if floor < 0 {
		floor = 0
	}
	if ceil > 1440 {
		ceil = 1440
	}
	return floor, ceil
}

// splitIDs splits a comma-joined id list, trimming and dropping empties.
func splitIDs(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// handleHistorieHome renders the full Historie page on the AppShell at
// GET /historie?view=cal|list&cal=week|month&week=YYYY-MM-DD&page=N.
func (s *Server) handleHistorieHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	if r.URL.Query().Get("view") == "list" {
		vm, err := s.historieListData(r.Context(), u, r, "")
		if err != nil {
			s.webServerError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = webui.HistorieListPage(vm).Render(r.Context(), w)
		return
	}
	vm, err := s.historieCalData(r.Context(), u, r, "")
	if err != nil {
		s.webServerError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.HistoriePage(vm).Render(r.Context(), w)
}

// handleHistorieCalendarFragment renders the inner calendar fragment (SSE/nav swap).
func (s *Server) handleHistorieCalendarFragment(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	vm, err := s.historieCalData(r.Context(), u, r, "")
	if err != nil {
		s.webServerError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.HistorieCalendarFragment(vm).Render(r.Context(), w)
}

// handleHistorieListFragment renders the inner list fragment (SSE/pagination swap).
func (s *Server) handleHistorieListFragment(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	vm, err := s.historieListData(r.Context(), u, r, "")
	if err != nil {
		s.webServerError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.HistorieListFragment(vm).Render(r.Context(), w)
}

// renderHistorieFragment re-renders the calendar OR list inner fragment per the
// ?view= form/query value, optionally with an inline error banner.
func (s *Server) renderHistorieFragment(w http.ResponseWriter, r *http.Request, u domain.User, errMsg string) {
	if r.FormValue("view") == "list" {
		vm, err := s.historieListData(r.Context(), u, r, errMsg)
		if err != nil {
			s.webServerError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = webui.HistorieListFragment(vm).Render(r.Context(), w)
		return
	}
	vm, err := s.historieCalData(r.Context(), u, r, errMsg)
	if err != nil {
		s.webServerError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.HistorieCalendarFragment(vm).Render(r.Context(), w)
}

// handleHistorieReassign assigns one project to the selected sessions (bulk),
// supporting inline project-create, then re-renders the active fragment.
func (s *Server) handleHistorieReassign(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	ids := splitIDs(r.FormValue("ids"))
	pid := s.resolveWebNode(r, u)
	if pid == nil {
		s.renderHistorieFragment(w, r, u, "kein Projekt gewählt")
		return
	}
	if _, err := s.BulkAssignNode.Execute(r.Context(), u.ID, ids, *pid); err != nil {
		s.renderHistorieFragment(w, r, u, historieBulkErr(err))
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionUpdated, UserID: u.ID})
	s.renderHistorieFragment(w, r, u, "")
}

// handleHistorieBulkDelete deletes the selected sessions, then re-renders.
func (s *Server) handleHistorieBulkDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	ids := splitIDs(r.FormValue("ids"))
	if _, err := s.BulkDeleteSessions.Execute(r.Context(), u.ID, ids); err != nil {
		s.renderHistorieFragment(w, r, u, historieBulkErr(err))
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionDeleted, UserID: u.ID})
	s.renderHistorieFragment(w, r, u, "")
}

// historieBulkErr maps a bulk usecase error to a short German banner.
func historieBulkErr(err error) string {
	switch {
	case errors.Is(err, usecase.ErrNoSessions):
		return "keine Sitzungen ausgewählt"
	case errors.Is(err, ports.ErrNodeNotFound):
		return "Projekt nicht gefunden"
	default:
		return "Aktion fehlgeschlagen: " + err.Error()
	}
}

// ── calendar (week + month) data ─────────────────────────────────────────────

func (s *Server) historieCalData(ctx context.Context, u domain.User, r *http.Request, errMsg string) (webui.HistorieVM, error) {
	now := s.Clock.Now()
	loc := now.Location()
	calView := r.URL.Query().Get("cal")
	if calView != "month" {
		calView = "week"
	}

	projects, err := s.ListNodes.Execute(ctx, u.ID)
	if err != nil {
		return webui.HistorieVM{}, err
	}

	vm := webui.HistorieVM{
		User:     u.Username,
		View:     "cal",
		CalView:  calView,
		HourPx:   historieHourPx,
		Nodes: historieProjectPickers(projects),
		Err:      errMsg,
	}

	// Resolve reference week (ISO Monday, local), clamped to never exceed current.
	ref := now
	if wp := r.URL.Query().Get("week"); wp != "" {
		if parsed, perr := time.ParseInLocation(dayLayout, wp, loc); perr == nil {
			ref = parsed
		}
	}
	weekStart := isoMonday(ref)
	curMonday := isoMonday(now)
	if weekStart.After(curMonday) {
		weekStart = curMonday
	}
	vm.WeekStart = weekStart

	if calView == "month" {
		if err := s.historieBuildMonth(ctx, u, &vm, ref, now, projects); err != nil {
			return webui.HistorieVM{}, err
		}
		return vm, nil
	}
	if err := s.historieBuildWeek(ctx, u, &vm, weekStart, curMonday, now, projects); err != nil {
		return webui.HistorieVM{}, err
	}
	return vm, nil
}

// historieBuildWeek fills the week grid: 7 day columns with positioned blocks +
// mobile agenda rows, the hybrid time window, and the now-line for today.
func (s *Server) historieBuildWeek(ctx context.Context, u domain.User, vm *webui.HistorieVM, weekStart, curMonday, now time.Time, projects []domain.Node) error {
	loc := now.Location()
	weekEnd := weekStart.AddDate(0, 0, 7)
	sessions, err := s.ListSessionsRange.Execute(ctx, u.ID, weekStart, weekEnd)
	if err != nil {
		return err
	}

	// Day-offs (Urlaub/Krank/Feiertag/…) so the calendar is complete, keyed by
	// local date. Reuses the Woche hue/label mapping.
	offByDate := map[string]domain.DayOff{}
	if offs, derr := s.ListDayOffs.Execute(ctx, u.ID, weekStart, weekEnd); derr == nil {
		for _, o := range offs {
			offByDate[o.Date.In(loc).Format(dayLayout)] = o
		}
	}

	// Collect minute-of-day extremes (local) for the hybrid window.
	mins := make([]int, 0, len(sessions)*2)
	for _, sess := range sessions {
		mins = append(mins, minuteOfDay(sess.Start, loc))
		stop := now
		if sess.Stop != nil {
			stop = *sess.Stop
		}
		mins = append(mins, effectiveStopMin(sess.Start, stop, loc))
	}
	floor, ceil := gridWindow(mins)
	vm.WindowFloorMin = floor
	vm.GridHeightPx = (ceil - floor) / 60 * historieHourPx
	vm.HourLabels = historieHourLabels(floor, ceil)

	// Bucket sessions by local day index (0=Mon..6=Sun).
	byDay := make([][]domain.WorkSession, 7)
	for _, sess := range sessions {
		idx := int(dayIndexMon(sess.Start.In(loc)))
		byDay[idx] = append(byDay[idx], sess)
	}

	isCurrent := weekStart.Equal(curMonday)
	vm.RangeLabel = wocheRangeLabel(weekStart)
	vm.PrevHref = historieCalHref("week", weekStart.AddDate(0, 0, -7))
	if !isCurrent {
		nx := weekStart.AddDate(0, 0, 7)
		if nx.Equal(curMonday) {
			vm.NextHref = historieCalHref("week", time.Time{})
		} else {
			vm.NextHref = historieCalHref("week", nx)
		}
		vm.ThisHref = historieCalHref("week", time.Time{})
	}
	vm.CalWeekURL = "/historie?view=cal&cal=week" + historieWeekParam(weekStart, isCurrent)
	vm.CalMonthURL = "/historie?view=cal&cal=month" + historieWeekParam(weekStart, isCurrent)
	vm.ListHref = "/historie?view=list"
	vm.FragmentURL = "/ui/historie/calendar?cal=week" + historieWeekParam(weekStart, isCurrent)

	unassigned := 0
	anyLogged := false
	for i := 0; i < 7; i++ {
		day := weekStart.AddDate(0, 0, i)
		dayVM := webui.HistorieDayVM{
			Key:          historieWeekdayKey(i),
			DateKey:      day.Format(dayLayout),
			Label:        historieWeekdayKey(i),
			DayNum:       day.Format("2"),
			DateLabel:    day.Format("02.01."),
			IsToday:      sameLocalDay(day, now, loc),
			IsWeekend:    i >= 5,
			NowLineTopPx: -1,
		}
		if off, ok := offByDate[day.Format(dayLayout)]; ok {
			dayVM.DayOff = true
			dayVM.DayOffLabel = off.Kind.LabelDe()
			dayVM.DayOffHue = dayOffHue(off.Kind)
		}
		var dayTotal time.Duration
		for _, sess := range byDay[i] {
			blk, row, dur, isUn := historieSessionVMs(sess, projects, now, loc, floor)
			dayVM.Blocks = append(dayVM.Blocks, blk)
			dayVM.Rows = append(dayVM.Rows, row)
			dayTotal += dur
			if isUn {
				unassigned++
			}
		}
		if dayTotal > 0 {
			anyLogged = true
			dayVM.Dur = webui.FmtVerbose(dayTotal)
		}
		if dayVM.IsToday {
			dayVM.NowLineTopPx = (minuteOfDay(now, loc) - floor) * historieHourPx / 60
		}
		vm.Days = append(vm.Days, dayVM)
	}
	vm.UnassignedCount = unassigned
	vm.Empty = !anyLogged
	return nil
}

// historieBuildMonth fills the month grid: leading/trailing padding cells to
// align Mon-first weeks, each in-month cell carrying hours + an unassigned flag.
func (s *Server) historieBuildMonth(ctx context.Context, u domain.User, vm *webui.HistorieVM, ref, now time.Time, projects []domain.Node) error {
	loc := now.Location()
	first := time.Date(ref.Year(), ref.Month(), 1, 0, 0, 0, 0, loc)
	next := first.AddDate(0, 0, 32)
	monthEnd := time.Date(next.Year(), next.Month(), 1, 0, 0, 0, 0, loc)
	sessions, err := s.ListSessionsRange.Execute(ctx, u.ID, first, monthEnd)
	if err != nil {
		return err
	}

	type dayAgg struct {
		dur        time.Duration
		unassigned bool
		hues       []string
	}
	agg := make(map[int]*dayAgg)
	monthUnassigned := 0
	var monthTotal time.Duration
	for _, sess := range sessions {
		d := sess.Start.In(loc).Day()
		a := agg[d]
		if a == nil {
			a = &dayAgg{}
			agg[d] = a
		}
		dur := sess.Elapsed(now)
		a.dur += dur
		monthTotal += dur
		if sess.NodeID == nil {
			a.unassigned = true
			monthUnassigned++
		} else if hue := projectHue(projects, sess.NodeID); hue != "" {
			a.hues = append(a.hues, hue)
		}
	}

	vm.MonthLabel = historieMonthYear(first)
	vm.RangeLabel = historieMonthYear(first)
	vm.MonthTotal = webui.FmtVerbose(monthTotal)
	vm.MonthUnassigned = monthUnassigned
	prevMonth := first.AddDate(0, 0, -1)
	vm.PrevHref = historieCalHref("month", prevMonth)
	curFirst := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	if !first.Equal(curFirst) {
		vm.NextHref = historieCalHref("month", first.AddDate(0, 0, 32))
		vm.ThisHref = historieCalHref("month", time.Time{})
	}
	vm.CalWeekURL = "/historie?view=cal&cal=week"
	vm.CalMonthURL = "/historie?view=cal&cal=month&week=" + first.Format(dayLayout)
	vm.ListHref = "/historie?view=list"
	// SSE live-refresh must target the FRAGMENT endpoint (not the full-page
	// route) — otherwise an `sse:session.*` event swaps a whole HTML document
	// (Base/AppShell + a nested #content) into #content. Mirrors the week path.
	vm.FragmentURL = "/ui/historie/calendar?cal=month&week=" + first.Format(dayLayout)

	// Leading padding (Mon-first): empty cells before day 1.
	lead := int(dayIndexMon(first))
	for i := 0; i < lead; i++ {
		vm.MonthCells = append(vm.MonthCells, webui.HistorieMonthCellVM{Empty: true})
	}
	daysInMonth := monthEnd.AddDate(0, 0, -1).Day()
	for d := 1; d <= daysInMonth; d++ {
		day := time.Date(first.Year(), first.Month(), d, 0, 0, 0, 0, loc)
		cell := webui.HistorieMonthCellVM{
			DayNum:    strconv.Itoa(d),
			IsToday:   sameLocalDay(day, now, loc),
			IsWeekend: dayIndexMon(day) >= 5,
			WeekHref:  historieCalHref("week", isoMonday(day)),
		}
		if a := agg[d]; a != nil {
			cell.Hours = webui.FmtCompact(a.dur)
			cell.HasUnassigned = a.unassigned
			cell.Bars = historieMonthBars(a.hues, a.unassigned)
		}
		vm.MonthCells = append(vm.MonthCells, cell)
	}
	return nil
}

// ── list data ────────────────────────────────────────────────────────────────

func (s *Server) historieListData(ctx context.Context, u domain.User, r *http.Request, errMsg string) (webui.HistorieListVM, error) {
	now := s.Clock.Now()
	page := 1
	if p, perr := strconv.Atoi(r.URL.Query().Get("page")); perr == nil && p > 1 {
		page = p
	}
	offset := (page - 1) * historiePageSize
	sessions, total, err := s.ListSessionsPage.Execute(ctx, u.ID, historiePageSize, offset)
	if err != nil {
		return webui.HistorieListVM{}, err
	}
	projects, err := s.ListNodes.Execute(ctx, u.ID)
	if err != nil {
		return webui.HistorieListVM{}, err
	}

	vm := webui.HistorieListVM{
		User:     u.Username,
		Nodes: historieProjectPickers(projects),
		Empty:    total == 0,
		Err:      errMsg,
		Page: components.PageNav{
			Page:     page,
			Total:    total,
			PageSize: historiePageSize,
			BaseHref: "/historie?view=list",
		},
	}
	vm.Rows = make([]components.SessionRowVM, 0, len(sessions))
	for _, sess := range sessions {
		row := sessionRowVM(sess, projects, now)
		row.Selectable = true
		vm.Rows = append(vm.Rows, row)
	}
	return vm, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

// historieSessionVMs maps a stored session to its block VM (grid) + row VM
// (agenda). Returns the session duration and whether it was unassigned.
func historieSessionVMs(sess domain.WorkSession, projects []domain.Node, now time.Time, loc *time.Location, floor int) (components.SessionBlockVM, components.SessionRowVM, time.Duration, bool) {
	startMin := minuteOfDay(sess.Start, loc)
	stopT := now
	if sess.Stop != nil {
		stopT = *sess.Stop
	}
	stopMin := effectiveStopMin(sess.Start, stopT, loc)
	if stopMin < startMin {
		stopMin = startMin
	}
	topPx := (startMin - floor) * historieHourPx / 60
	heightPx := (stopMin - startMin) * historieHourPx / 60
	if heightPx < 24 {
		heightPx = 24
	}
	dur := sess.Elapsed(now)

	name, hue := nodeIdentity(projects, sess.NodeID)
	glyph := nodeGlyph(projects, sess.NodeID)
	unassigned := sess.NodeID == nil

	editTo := ""
	if sess.Stop != nil {
		editTo = sess.Stop.In(loc).Format("15:04")
	}
	editPID := ""
	if sess.NodeID != nil {
		editPID = *sess.NodeID
	}
	blk := components.SessionBlockVM{
		ID:            sess.ID,
		TopPx:         topPx,
		HeightPx:      heightPx,
		Hue:           hue,
		Glyph:         glyph,
		Title:         name,
		TimeRange:     fmtClockRange(sess) + " · " + webui.FmtCompact(dur),
		Tags:          sess.Tags,
		Unassigned:    unassigned,
		Running:       sess.Running(),
		Size:          historieBlockSize(heightPx),
		EditTo:        editTo,
		EditTag:       strings.Join(sess.Tags, " "),
		EditNote:      sess.Note,
		EditNodeID: editPID,
	}
	row := sessionRowVM(sess, projects, now)
	row.Selectable = true
	return blk, row, dur, unassigned
}

// historieBlockSize maps a block height to the reveal class (sm reveals time, md
// reveals tag/hint) — mirroring the mockup's .block-sm/.block-md thresholds.
func historieBlockSize(heightPx int) string {
	switch {
	case heightPx >= 80:
		return "md"
	case heightPx >= 44:
		return "sm"
	default:
		return ""
	}
}

// historieHourLabels builds the time-axis ticks between floor and ceil (hourly).
func historieHourLabels(floor, ceil int) []webui.HistorieHourLabel {
	var out []webui.HistorieHourLabel
	for m := floor; m <= ceil; m += 60 {
		nudge := -7
		if m == floor {
			nudge = 3 // first label sits just below the top edge
		}
		out = append(out, webui.HistorieHourLabel{
			Label: fmt.Sprintf("%02d", m/60),
			TopPx: (m-floor)*historieHourPx/60 + nudge,
		})
	}
	return out
}

// historieMonthBars renders up to 3 project hue bars + an optional dashed
// unassigned bar (illustrative widths derived from hue count).
func historieMonthBars(hues []string, unassigned bool) []webui.HistorieMonthBar {
	var bars []webui.HistorieMonthBar
	seen := map[string]bool{}
	for _, h := range hues {
		if seen[h] || len(bars) >= 3 {
			continue
		}
		seen[h] = true
		bars = append(bars, webui.HistorieMonthBar{Hue: h, WidthPct: 55 + len(bars)*15})
	}
	if unassigned {
		bars = append(bars, webui.HistorieMonthBar{Dashed: true, WidthPct: 45})
	}
	return bars
}

// historieProjectPickers maps domain projects to picker VMs (name/hue/glyph/rate).
func historieProjectPickers(projects []domain.Node) []components.NodePickerItem {
	out := make([]components.NodePickerItem, 0, len(projects))
	for _, p := range projects {
		out = append(out, components.NodePickerItem{
			ID:    p.ID,
			Name:  p.Name,
			Hue:   p.Color,
			Glyph: glyphOr(p.Glyph),
			Rate:  rateLabel(p.Rate),
		})
	}
	return out
}

// projectHue resolves a session's project hue ("" if unknown/unassigned).
func projectHue(projects []domain.Node, id *string) string {
	if id == nil {
		return ""
	}
	for _, p := range projects {
		if p.ID == *id {
			return p.Color
		}
	}
	return ""
}

// historieCalHref builds a /historie?view=cal calendar URL; a zero week/month
// reference yields the current-period link (no ?week=).
func historieCalHref(cal string, ref time.Time) string {
	base := "/historie?view=cal&cal=" + cal
	if ref.IsZero() {
		return base
	}
	return base + "&week=" + ref.Format(dayLayout)
}

// historieWeekParam returns "&week=YYYY-MM-DD" for a non-current week, else "".
func historieWeekParam(weekStart time.Time, isCurrent bool) string {
	if isCurrent {
		return ""
	}
	return "&week=" + weekStart.Format(dayLayout)
}

// historieWeekdayKey maps a Mon-first index (0..6) to its short German label.
func historieWeekdayKey(i int) string {
	return [...]string{"Mo", "Di", "Mi", "Do", "Fr", "Sa", "So"}[i%7]
}

// historieMonthYear renders "Juni 2026".
func historieMonthYear(t time.Time) string {
	return [...]string{
		"Januar", "Februar", "März", "April", "Mai", "Juni",
		"Juli", "August", "September", "Oktober", "November", "Dezember",
	}[int(t.Month())-1] + " " + strconv.Itoa(t.Year())
}

// minuteOfDay returns the local minute-of-day for t.
func minuteOfDay(t time.Time, loc *time.Location) int {
	lt := t.In(loc)
	return lt.Hour()*60 + lt.Minute()
}

// effectiveStopMin returns the block's stop minute-of-day on its START-day grid
// column. A stop that lands on a later local day (e.g. a split chunk that ends
// at 00:00 the next day, or any cross-midnight span) fills to 24:00 (1440)
// instead of collapsing to minute 0 of the next day.
func effectiveStopMin(start, stop time.Time, loc *time.Location) int {
	if !sameLocalDay(start, stop, loc) {
		return 24 * 60
	}
	return minuteOfDay(stop, loc)
}

// dayIndexMon returns the Mon-first weekday index (0=Mon..6=Sun) of t.
func dayIndexMon(t time.Time) time.Weekday {
	return time.Weekday((int(t.Weekday()) + 6) % 7)
}

// sameLocalDay reports whether a and b fall on the same calendar day in loc.
func sameLocalDay(a, b time.Time, loc *time.Location) bool {
	ay, am, ad := a.In(loc).Date()
	by, bm, bd := b.In(loc).Date()
	return ay == by && am == bm && ad == bd
}
