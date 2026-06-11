package usecase

import (
	"sort"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ServerSessionsReader is the narrow read surface the WebUI ServerWorktimeView
// needs from a session store. Declared locally so the usecase layer stays
// adapter-free; `*pgstore.Sessions` satisfies it structurally.
type ServerSessionsReader interface {
	ListByUserDateRange(userID string, from, to time.Time) ([]domain.Session, error)
}

// ServerActiveSessionsReader is the narrow read surface ServerWorktimeView
// needs from an active-session store. Same hexagonal rationale as
// ServerSessionsReader.
type ServerActiveSessionsReader interface {
	ListByUser(userID string) ([]domain.ActiveSession, error)
}

// ServerWorktimeView is the multi-tenant, server-side read entry point used by
// the WebUI (M6/M7). It mirrors the four read shapes of the client-side
// WorktimeReader (Today, Week, History, Range) so the WebUI templates can
// reuse the domain.Day / domain.WeekDay / domain.DayRecord shapes the TUI
// already renders.
//
// Differences from the client-side WorktimeReader (intentional, M6 scope):
//   - Per-user targets are not yet implemented server-side. Every day uses
//     DefaultTarget on weekdays and 0 on weekends. Per-user targets land in a
//     later phase together with a server-side TargetResolver.
//   - No Pause semantics: server has no pause state, so Day.PausedAt is
//     always nil and IsPaused() reports false.
//   - DayOff (Frei) is not handled here — the WebUI Frei tab renders a
//     placeholder until a server-side DayOffStore exists.
//
// Dependencies are the pgstore sub-adapters, following the convention
// established by the M2/M3 httpserver handlers. The narrow reader interfaces
// (ServerSessionsReader / ServerActiveSessionsReader) are satisfied
// structurally by the pgstore adapters.
type ServerWorktimeView struct {
	Sessions      ServerSessionsReader
	Active        ServerActiveSessionsReader
	Clock         ports.Clock
	DefaultTarget time.Duration

	// ShowWeekend, when true, keeps Sat/Sun rows in Week views even when
	// they have no sessions and aren't today.
	ShowWeekend bool
}

// historyWindow is how far History looks back. Matches the client-side
// WorktimeReader's "last 60 days" semantics — kept here so the WebUI history
// page doesn't accidentally load years of sessions on every request.
const historyWindow = 60 * 24 * time.Hour

// Today returns the day record for userID's "today" in the server clock's
// timezone — sessions logged so far, active marker (if any), and the
// resolved daily target. Pause state is intentionally absent (see type doc).
func (v *ServerWorktimeView) Today(userID string) (domain.Day, error) {
	now := v.Clock.Now()
	day := domain.Day{Target: v.targetFor(now)}

	active, err := v.firstActiveStart(userID)
	if err != nil {
		return day, err
	}
	if active != nil {
		day.Active = active
	}

	dayStart := startOfDay(now)
	sessions, err := v.Sessions.ListByUserDateRange(userID, dayStart, dayStart)
	if err != nil {
		return day, err
	}
	day.Sessions = sessions
	for _, s := range sessions {
		day.Logged += s.Elapsed
	}
	return day, nil
}

// Week returns Mon-Sun of the current ISO week for userID. Weekend rows
// without sessions are dropped unless ShowWeekend is set or they are today.
func (v *ServerWorktimeView) Week(userID string) ([]domain.WeekDay, error) {
	now := v.Clock.Now()
	active, err := v.firstActiveStart(userID)
	if err != nil {
		return nil, err
	}

	monday := mondayOf(now)
	sunday := monday.AddDate(0, 0, 6)

	sessions, err := v.Sessions.ListByUserDateRange(userID, monday, sunday)
	if err != nil {
		return nil, err
	}

	byDay := make(map[string]time.Duration)
	for _, s := range sessions {
		byDay[s.Date.Format("2006-01-02")] += s.Elapsed
	}

	var week []domain.WeekDay
	for i := 0; i < 7; i++ {
		d := monday.AddDate(0, 0, i)
		isToday := domain.SameDay(d, now)
		logged := byDay[d.Format("2006-01-02")]
		isWeekend := i >= 5

		if isWeekend && logged == 0 && !isToday && !v.ShowWeekend {
			continue
		}

		var dayActive *time.Time
		if isToday {
			dayActive = active
		}
		week = append(week, domain.WeekDay{
			Date:    d,
			Logged:  logged,
			Active:  dayActive,
			Target:  v.targetFor(d),
			IsToday: isToday,
		})
	}
	return week, nil
}

// History returns the last 60 days that have at least one session for userID,
// newest first. Bounded window keeps the WebUI history page snappy even on
// long histories.
func (v *ServerWorktimeView) History(userID string) ([]domain.DayRecord, error) {
	now := v.Clock.Now()
	from := startOfDay(now.Add(-historyWindow))
	to := startOfDay(now)

	sessions, err := v.Sessions.ListByUserDateRange(userID, from, to)
	if err != nil {
		return nil, err
	}

	byDate := make(map[string]*domain.DayRecord)
	var order []string
	for _, s := range sessions {
		key := s.Date.Format("2006-01-02")
		if _, ok := byDate[key]; !ok {
			byDate[key] = &domain.DayRecord{Date: s.Date, Target: v.targetFor(s.Date)}
			order = append(order, key)
		}
		rec := byDate[key]
		rec.Sessions = append(rec.Sessions, s)
		rec.Total += s.Elapsed
	}

	sort.Sort(sort.Reverse(sort.StringSlice(order)))
	out := make([]domain.DayRecord, len(order))
	for i, key := range order {
		out[i] = *byDate[key]
	}
	return out, nil
}

// Range returns userID's sessions whose Date falls inside [from, to] (inclusive).
func (v *ServerWorktimeView) Range(userID string, from, to time.Time) ([]domain.Session, error) {
	return v.Sessions.ListByUserDateRange(userID, startOfDay(from), startOfDay(to))
}

// firstActiveStart returns StartedAt of the first active row for userID, or
// nil if none. Server enforces 1 active per user in practice, so picking the
// first entry is the correct shape for Today/Week.
func (v *ServerWorktimeView) firstActiveStart(userID string) (*time.Time, error) {
	rows, err := v.Active.ListByUser(userID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	t := rows[0].StartedAt
	return &t, nil
}

// targetFor returns the daily target for date. M6 hardcodes weekday=Default,
// weekend=0. A future ServerTargetResolver will replace this with per-user
// targets read from a server-side targets table.
func (v *ServerWorktimeView) targetFor(date time.Time) time.Duration {
	wd := date.Weekday()
	if wd == time.Saturday || wd == time.Sunday {
		return 0
	}
	return v.DefaultTarget
}

// mondayOf returns 00:00 of t's ISO Monday in t's location.
// Reuses startOfDay from session_writer.go.
func mondayOf(t time.Time) time.Time {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	return startOfDay(t).AddDate(0, 0, -(wd - 1))
}
