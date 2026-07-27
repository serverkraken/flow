package httpserver

import (
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// wocheSummary holds the aggregated Mon–Fri totals + goal counters for one week.
// Ported verbatim (arithmetic-wise) from internal/tui/screen/worktime/week/
// summary.go, adapted from the apiclient.WeekDay DTO (minute ints) to the
// domain.WeekDay value (time.Duration). Weekends are excluded from BOTH totals
// — a normal week is 40h = 5×8h and the server still emits a default target for
// Sat/Sun, so summing all seven days would inflate the week target.
type wocheSummary struct {
	totalLogged, totalTarget time.Duration
	workdays, hits, expected int
}

// isWeekendTime reports whether t falls on Sat/Sun (in t's own location).
func isWeekendTime(t time.Time) bool {
	wd := t.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}

// computeWocheSummary aggregates week totals and goal counters. Only Mon–Fri
// count toward the week. Day-off weekdays keep their (server-netted, usually 0)
// target in the totals but are not counted as workdays/hits. "expected" = past
// workdays plus today if already hit, using the IsToday flag to avoid clock
// calls. `now` is used to fold the running tail into today's logged via
// WeekDay.Total — pass the local clock now.
func computeWocheSummary(days []domain.WeekDay, offs map[string]domain.DayOff, now time.Time) wocheSummary {
	var s wocheSummary
	todayIdx := -1
	for i, d := range days {
		if d.IsToday {
			todayIdx = i
		}
	}
	for i, d := range days {
		if isWeekendTime(d.Date) {
			continue // weekends never count toward the Mon–Fri week
		}
		logged := d.Total(now)
		s.totalLogged += logged
		s.totalTarget += d.Target
		key := d.Date.Format("2006-01-02")
		if _, off := offs[key]; off {
			continue
		}
		s.workdays++
		hit := d.Target > 0 && logged >= d.Target
		if hit {
			s.hits++
		}
		past := todayIdx >= 0 && i < todayIdx
		if past || (i == todayIdx && hit) {
			s.expected++
		}
	}
	return s
}

// onTrack reports whether the week is on pace: at least as many goals hit as
// expected by now (with no expectation yet → on track).
func (s wocheSummary) onTrack() bool {
	if s.expected == 0 {
		return true
	}
	return s.hits >= s.expected
}

// paceDotState maps one weekday to its components.PaceDot.State string
// (behind|ontrack|ahead|running|holiday|off), ported from week/pacedot.go's
// classifyPaceDot. off is non-nil when a day-off covers the date.
//
//	day-off          → "off"  (Frei list + Woche dots share kindcolor elsewhere;
//	                          the WebUI PaceDots glyph for "off" is the hollow ○)
//	target hit       → "ahead"
//	today, not hit   → "running"
//	missed (past)    → "behind"
func paceDotState(d domain.WeekDay, off *domain.DayOff, now time.Time) string {
	if off != nil {
		return "off"
	}
	if d.Target > 0 && d.Total(now) >= d.Target {
		return "ahead"
	}
	if d.IsToday {
		return "running"
	}
	return "behind"
}
