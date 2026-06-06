package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/a-h/templ"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/webui/templates/layout"
	"github.com/serverkraken/flow/internal/webui/templates/shared"
	"github.com/serverkraken/flow/internal/webui/templates/worktime"
)

// View-model builders, one per sub-tab. Each consumes the request +
// deps + user + clock and returns the templ component, the spine state,
// or an error to propagate. Kept here (separate from worktime.go) so
// the dispatcher in worktime.go stays small and the per-tab logic is
// easy to scan side-by-side.
//
// Convention: all builders compute the spine state from today's
// sessions / active row so the left-edge status segment stays
// consistent across sub-tabs even when the body shows a historical or
// woche / frei view.

// — Heute —

func renderToday(_ *http.Request, d WorktimeDeps, u domain.User, now time.Time) (templ.Component, layout.SpineState, error) {
	vm := worktime.TodayVM{
		Header: worktime.PageHeader{
			Eyebrow: worktime.FormatGermanDateHeader(now),
			Title:   "Worktime",
		},
		NowHour: now.Hour(),
	}

	today, err := d.View.Today(u.ID)
	if err != nil {
		return nil, layout.SpineState{}, fmt.Errorf("today: %w", err)
	}
	todayTotal := today.Total(now)
	saldoToday := todayTotal - today.Target
	vm.TodayTotal = worktime.FormatHHMM(todayTotal)
	vm.TodayTarget = worktime.FormatHHMM(today.Target)
	vm.TodaySaldo = worktime.FormatSignedHHMM(saldoToday)
	vm.TodaySaldoPos = saldoToday >= 0

	week, err := d.View.Week(u.ID)
	if err != nil {
		return nil, layout.SpineState{}, fmt.Errorf("week: %w", err)
	}
	weekLogged, weekTarget := weekTotals(week, now)
	vm.WeekLogged = worktime.FormatHHMM(weekLogged)
	vm.WeekTarget = worktime.FormatHHMM(weekTarget)

	active, err := firstActiveSession(d, u.ID)
	if err != nil {
		return nil, layout.SpineState{}, err
	}
	if active != nil {
		vm.HasActive = true
		vm.WeekDaysRun = true
		projLabel := active.ProjectID
		if proj, err := d.Projects.GetByID(u.ID, active.ProjectID); err == nil {
			projLabel = proj.Name
		}
		vm.Live = shared.LiveBanner{
			ProjectLabel: projLabel,
			Tag:          active.Tag,
			ElapsedLabel: worktime.FormatElapsedHumane(now.Sub(active.StartedAt)),
			StartedAt:    active.StartedAt.In(now.Location()).Format("15:04"),
			SinceLabel:   "→ läuft",
		}
	}

	// Year-saldo placeholder — Phase 2 will replace this with a real
	// year aggregate. M6 reuses the week-saldo so the third stripe cell
	// renders something deterministic and the layout doesn't shift.
	yearSaldo := weekLogged - weekTarget
	vm.YearSaldo = worktime.FormatSignedHHMM(yearSaldo)
	vm.YearSaldoPos = yearSaldo >= 0
	vm.YearSubLabel = "M6: zeigt Woche-Saldo, Jahres-Aggregat folgt in Phase 2"

	resolveName := newProjectNameResolver(d.Projects, u.ID)
	vm.DayBar = worktime.HourMask(today.Sessions, activeStartPtr(active), now)
	vm.Entries = worktime.BuildSessionRows(today.Sessions, active, now, now.Location(), resolveName)

	// Rail aggregates use today's sessions only.
	vm.ProjectShares, _ = worktime.AggregateProjectShares(today.Sessions, now, active, resolveName, 4)
	vm.TagShares = worktime.AggregateTagShares(today.Sessions, now, active)
	vm.SyncDeviceLabel = d.DeviceLabel

	spine := layout.SpineState{
		AnyActive: vm.HasActive,
		HourMask:  vm.DayBar,
		NowHour:   vm.NowHour,
		SyncState: "ok",
	}
	return worktime.Today(vm), spine, nil
}

// — Woche —

func renderWeek(_ *http.Request, d WorktimeDeps, u domain.User, now time.Time) (templ.Component, layout.SpineState, error) {
	monday := worktime.MondayOf(now)
	vm := worktime.WeekVM{
		Header: worktime.PageHeader{
			Eyebrow: worktime.FormatGermanWeekRange(monday),
			Title:   "Worktime",
		},
	}

	week, err := d.View.Week(u.ID)
	if err != nil {
		return nil, layout.SpineState{}, fmt.Errorf("week: %w", err)
	}
	weekLogged, weekTarget := weekTotals(week, now)
	saldoWeek := weekLogged - weekTarget
	vm.WeekLogged = worktime.FormatHHMM(weekLogged)
	vm.WeekTarget = worktime.FormatHHMM(weekTarget)
	vm.WeekSaldo = worktime.FormatSignedHHMM(saldoWeek)
	vm.WeekSaldoPos = saldoWeek >= 0
	vm.DaysBooked = countBookedDays(week, now)

	_, isoWeek := monday.ISOWeek()
	vm.WeekISOLabel = fmt.Sprintf("KW %d", isoWeek)

	vm.WeekBars = worktime.BuildWeekBars(week, monday, now)
	for _, b := range vm.WeekBars {
		if b.Hours > 0 {
			vm.HasWeekData = true
			break
		}
	}

	// Week sessions for per-project / per-tag aggregates.
	sunday := monday.AddDate(0, 0, 6)
	weekSessions, err := d.Sessions.ListByUserDateRange(u.ID, monday, sunday)
	if err != nil {
		return nil, layout.SpineState{}, fmt.Errorf("week sessions: %w", err)
	}

	active, err := firstActiveSession(d, u.ID)
	if err != nil {
		return nil, layout.SpineState{}, err
	}
	if active != nil {
		vm.HasRunning = true
	}

	resolveName := newProjectNameResolver(d.Projects, u.ID)
	var grand time.Duration
	vm.ProjectShares, grand = worktime.AggregateProjectShares(weekSessions, now, active, resolveName, 5)
	vm.TagShares = worktime.AggregateTagShares(weekSessions, now, active)
	vm.ProjectsTotal = worktime.FormatHHMM(grand)

	vm.SessionsCount = len(weekSessions)
	if active != nil {
		vm.SessionsCount++
	}
	vm.LongestLabel, vm.ShortestLabel = sessionLengthExtremes(weekSessions, now.Location())

	// 12-week saldo sparkline: aggregate last 12 weeks by ISO week. We
	// query an 84-day window to cover 12 full weeks plus the current
	// (partial) one.
	from := monday.AddDate(0, 0, -7*11) // 11 weeks back + current = 12 buckets
	to := sunday
	rangeSessions, err := d.View.Range(u.ID, from, to)
	if err != nil {
		return nil, layout.SpineState{}, fmt.Errorf("range: %w", err)
	}
	vm.SaldoSeries = worktime.BuildWeekSaldoSeries(rangeSessions, from, to, d.View.DefaultTarget, now)

	spine, err := todaySpine(d, u.ID, active, now)
	if err != nil {
		return nil, layout.SpineState{}, err
	}
	return worktime.Week(vm), spine, nil
}

// — Verlauf —

const historyDateFormat = "2006-01-02"

func renderHistory(r *http.Request, d WorktimeDeps, u domain.User, now time.Time) (templ.Component, layout.SpineState, error) {
	date := parseHistoryDate(r.URL.Query().Get("date"), now)
	dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, now.Location())
	isToday := domain.SameDay(date, now)

	vm := worktime.HistoryVM{
		Header: worktime.PageHeader{
			Eyebrow: worktime.FormatGermanDateHeader(date),
			Title:   "Worktime",
		},
		DayLabel: worktime.FormatGermanDayLabel(date),
		IsToday:  isToday,
	}
	yesterday := date.AddDate(0, 0, -1)
	tomorrow := date.AddDate(0, 0, 1)
	vm.PrevHref = fmt.Sprintf("/worktime?tab=verlauf&date=%s", yesterday.Format(historyDateFormat))
	vm.TodayHref = fmt.Sprintf("/worktime?tab=verlauf&date=%s",
		time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Format(historyDateFormat))
	if !isToday {
		// Don't allow navigation into the future.
		todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		if !tomorrow.After(todayStart) {
			vm.NextHref = fmt.Sprintf("/worktime?tab=verlauf&date=%s", tomorrow.Format(historyDateFormat))
		}
	}
	// Relative-day vocabulary for the jump-header — "gestern · heute ·
	// morgen" reads better than "vorheriger / heute / nächster" for
	// dates near today, and falls back to a short German date further
	// out.
	vm.PrevLabel = worktime.RelativeDayLabel(yesterday, now)
	vm.SelectedLabel = worktime.RelativeDayLabel(date, now)
	vm.NextLabel = worktime.RelativeDayLabel(tomorrow, now)

	sessions, err := d.Sessions.ListByUserDateRange(u.ID, dayStart, dayStart)
	if err != nil {
		return nil, layout.SpineState{}, fmt.Errorf("history sessions: %w", err)
	}
	target := defaultTargetFor(date, d.View.DefaultTarget)
	total := worktime.SumDurations(sessions)
	saldo := total - target
	vm.DayTotal = worktime.FormatHHMM(total)
	vm.DayTarget = worktime.FormatHHMM(target)
	vm.DaySaldo = worktime.FormatSignedHHMM(saldo)
	vm.DaySaldoPos = saldo >= 0
	vm.DaySessions = len(sessions)

	resolveName := newProjectNameResolver(d.Projects, u.ID)
	// Verlauf never shows the active session; pass nil so the table
	// doesn't get a "läuft" row even on today.
	vm.Entries = worktime.BuildSessionRows(sessions, nil, now, now.Location(), resolveName)
	vm.DayBar = worktime.HourMask(sessions, nil, date)

	active, err := firstActiveSession(d, u.ID)
	if err != nil {
		return nil, layout.SpineState{}, err
	}
	spine, err := todaySpine(d, u.ID, active, now)
	if err != nil {
		return nil, layout.SpineState{}, err
	}
	return worktime.History(vm), spine, nil
}

// — Frei —

func renderFrei(_ *http.Request, d WorktimeDeps, u domain.User, now time.Time) (templ.Component, layout.SpineState, error) {
	// TODO Phase 2: wire DayOffStore on server side, replace placeholder with list.
	vm := worktime.FreiVM{
		Header: worktime.PageHeader{
			Eyebrow: worktime.FormatGermanDateHeader(now),
			Title:   "Worktime",
		},
	}
	active, err := firstActiveSession(d, u.ID)
	if err != nil {
		return nil, layout.SpineState{}, err
	}
	spine, err := todaySpine(d, u.ID, active, now)
	if err != nil {
		return nil, layout.SpineState{}, err
	}
	return worktime.Frei(vm), spine, nil
}
