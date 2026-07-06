package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

// handleWocheHome renders the full Woche page on the AppShell at GET /woche.
func (s *Server) handleWocheHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	vm, err := s.wocheDataFor(r.Context(), u, r.URL.Query().Get("week"))
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.WochePage(vm).Render(r.Context(), w)
}

// handleWocheFragment renders the inner content fragment at GET
// /ui/woche/fragment?week=YYYY-MM-DD (the SSE-swap + KW-nav target).
func (s *Server) handleWocheFragment(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	vm, err := s.wocheDataFor(r.Context(), u, r.URL.Query().Get("week"))
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.WocheFragment(vm).Render(r.Context(), w)
}

// wocheDataFor builds the Woche view model. `weekParam` is the optional
// ?week=YYYY-MM-DD; empty (or unparsable) → the current week. The week is always
// resolved to its ISO-Monday in the local clock location (tz lesson: bucket in
// now.Location(), never UTC).
func (s *Server) wocheDataFor(ctx context.Context, u domain.User, weekParam string) (webui.WocheVM, error) {
	now := s.Clock.Now()
	loc := now.Location()

	// Resolve the reference day → ISO Monday (local).
	ref := now
	if weekParam != "" {
		if parsed, err := time.ParseInLocation("2006-01-02", weekParam, loc); err == nil {
			ref = parsed
		}
	}
	weekStart := isoMonday(ref)
	curMonday := isoMonday(now)
	// Clamp forward: never navigate past the current week. A future ?week= snaps
	// back to the current week.
	if weekStart.After(curMonday) {
		weekStart = curMonday
	}
	isCurrent := weekStart.Equal(curMonday)

	days, err := s.Stats.Week(ctx, u.ID, weekStart)
	if err != nil {
		return webui.WocheVM{}, err
	}

	// Day-offs spanning the displayed week, keyed by yyyy-mm-dd (local).
	weekEnd := weekStart.AddDate(0, 0, 6)
	offList, err := s.ListDayOffs.Execute(ctx, u.ID, weekStart, weekEnd)
	if err != nil {
		return webui.WocheVM{}, err
	}
	offs := make(map[string]domain.DayOff, len(offList))
	for _, o := range offList {
		offs[o.Date.In(loc).Format("2006-01-02")] = o
	}

	vm := webui.WocheVM{
		User:      u.Username,
		WeekStart: weekStart,
		KWLabel:   fmt.Sprintf("KW %d", isoWeek(weekStart)),
		WeekLabel: wocheRangeLabel(weekStart),
		PrevWeek:  weekStart.AddDate(0, 0, -7).Format(dayLayout),
		IsCurrent: isCurrent,
	}
	if !isCurrent {
		// Forward navigation is always toward (never past) the current week.
		nx := weekStart.AddDate(0, 0, 7)
		if nx.Equal(curMonday) {
			vm.NextWeek = "" // next == current → "this week" (no ?week=)
		} else {
			vm.NextWeek = nx.Format(dayLayout)
		}
		vm.CanForward = true
	}

	// Per-weekday target for the header hint (first Mon..Fri target found).
	for _, d := range days {
		if !isWeekendTime(d.Date) && d.Target > 0 {
			vm.WorkdayGoal = webui.FmtVerbose(d.Target)
			break
		}
	}
	if vm.WorkdayGoal == "" {
		vm.WorkdayGoal = webui.FmtVerbose(8 * time.Hour)
	}

	// Build day rows + pace dots.
	var anyLogged bool
	dots := make([]components.PaceDot, 0, 5)
	for _, d := range days {
		key := d.Date.In(loc).Format("2006-01-02")
		var off *domain.DayOff
		if o, ok := offs[key]; ok {
			off = &o
		}
		logged := d.Total(now)
		if logged > 0 {
			anyLogged = true
		}
		row := wocheDayRowVM(d, off, now)
		vm.Days = append(vm.Days, row)
		if !isWeekendTime(d.Date) {
			dots = append(dots, components.PaceDot{State: paceDotState(d, off, now)})
		}
	}

	// Summary → WOCHE GESAMT banner + KENNZAHLEN panel.
	sum := computeWocheSummary(days, offs, now)
	pct := 0
	if sum.totalTarget > 0 {
		pct = int(sum.totalLogged * 100 / sum.totalTarget)
	}
	saldo := sum.totalLogged - sum.totalTarget
	vm.Total = components.WeekTotalVM{
		Total:   webui.FmtVerbose(sum.totalLogged),
		Target:  webui.FmtVerbose(sum.totalTarget),
		Pct:     webui.ClampPct(pct),
		Variant: wocheTotalVariant(sum, isCurrent),
	}
	avg := time.Duration(0)
	if sum.workdays > 0 {
		avg = sum.totalLogged / time.Duration(sum.workdays)
	}
	vm.Kennzahlen = components.KennzahlenVM{
		AvgPerDay:  webui.FmtVerbose(avg),
		GoalsHit:   sum.hits,
		GoalsTotal: sum.workdays,
		Balance:    webui.FmtSaldoVerbose(saldo),
		BalancePos: saldo >= 0,
		Dots:       dots,
		OnTrack:    sum.onTrack(),
	}
	vm.Empty = !anyLogged

	// Statistik panel (Offene Entsch. #4): the monthly Burndown/Saldo glance
	// metric that used to live on Home now renders here (WocheStatsVM,
	// wocheStatsPanel) instead of the retired Kristall BurndownBanner.
	burn, err := s.Stats.Burndown(ctx, u.ID)
	if err != nil {
		return webui.WocheVM{}, err
	}
	vm.Stats = webui.WocheStatsVM{
		Total:   webui.FmtVerbose(burn.Total),
		Target:  webui.FmtVerbose(burn.Target),
		Saldo:   webui.FmtSaldoVerbose(burn.Saldo),
		OnTrack: burn.OnTrack,
	}

	return vm, nil
}

// wocheDayRowVM maps one domain.WeekDay (+ optional day-off) to its row VM.
func wocheDayRowVM(d domain.WeekDay, off *domain.DayOff, now time.Time) webui.WocheDayVM {
	row := webui.WocheDayVM{
		Label:     wocheWeekdayLabel(d.Date.Weekday()),
		DateLabel: d.Date.Format("02.01."),
		IsToday:   d.IsToday,
		Weekend:   isWeekendTime(d.Date),
	}
	if row.Weekend {
		row.Variant = "weekend"
		return row
	}

	logged := d.Total(now)
	row.Dur = webui.FmtVerbose(logged)
	row.TargetDur = webui.FmtVerbose(d.Target)

	if off != nil {
		row.DayOff = true
		row.DayOffLabel = off.Kind.LabelDe()
		row.DayOffHue = dayOffHue(off.Kind)
		row.Variant = "under"
		return row
	}

	if d.Target > 0 {
		row.Pct = webui.ClampPct(int(logged * 100 / d.Target))
	}
	saldo := logged - d.Target
	if d.Target > 0 || logged > 0 {
		row.Saldo = webui.FmtSaldoVerbose(saldo)
		row.SaldoPos = saldo >= 0
	}
	row.Variant = wocheDayVariant(d, logged)
	return row
}

// wocheDayVariant picks the per-day bar variant (hit|over|under|running).
func wocheDayVariant(d domain.WeekDay, logged time.Duration) string {
	hit := d.Target > 0 && logged >= d.Target
	switch {
	case d.IsToday && !hit:
		return "running"
	case d.Target > 0 && logged > d.Target:
		return "over"
	case d.Target > 0 && logged == d.Target:
		return "hit"
	default:
		return "under"
	}
}

// wocheTotalVariant picks the WOCHE GESAMT bar variant.
func wocheTotalVariant(s wocheSummary, isCurrent bool) string {
	switch {
	case s.totalTarget > 0 && s.totalLogged > s.totalTarget:
		return "over"
	case s.totalTarget > 0 && s.totalLogged == s.totalTarget:
		return "hit"
	case isCurrent:
		return "running"
	default:
		return "under"
	}
}

// wocheWeekdayLabel maps a weekday to its short German label.
func wocheWeekdayLabel(wd time.Weekday) string {
	switch wd {
	case time.Monday:
		return "Mo"
	case time.Tuesday:
		return "Di"
	case time.Wednesday:
		return "Mi"
	case time.Thursday:
		return "Do"
	case time.Friday:
		return "Fr"
	case time.Saturday:
		return "Sa"
	default:
		return "So"
	}
}

// wocheRangeLabel renders "15.–21.06.2026" for the Mon..Sun span.
func wocheRangeLabel(monday time.Time) string {
	sun := monday.AddDate(0, 0, 6)
	return fmt.Sprintf("%s–%s", monday.Format("02."), sun.Format("02.01.2006"))
}

// dayOffHue maps a day-off Kind to a web hue token, mirroring kindcolor.DayOffColor
// (TUI) so the Frei list and the Woche day-off chips never drift. Holiday=blue
// (Schedule), Vacation=purple (Highlight), Sick=orange (Notice), Flex=green,
// Special=yellow, ChildSick=red, Training=cyan.
func dayOffHue(k domain.Kind) string {
	switch k {
	case domain.KindHoliday:
		return "blue"
	case domain.KindVacation:
		return "purple"
	case domain.KindSick:
		return "orange"
	case domain.KindFlex:
		return "green"
	case domain.KindSpecial:
		return "yellow"
	case domain.KindChildSick:
		return "red"
	case domain.KindTraining:
		return "cyan"
	default:
		return "blue"
	}
}

// isoMonday returns the ISO-week Monday (00:00 local) containing t.
func isoMonday(t time.Time) time.Time {
	d := startOfDay(t)
	// Go's Weekday: Sunday=0..Saturday=6; ISO weeks start Monday.
	offset := (int(d.Weekday()) + 6) % 7
	return d.AddDate(0, 0, -offset)
}
