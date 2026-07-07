package webui

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/i18n"
)

// BuildWeekBars maps a raw 7-day domain.WeekDay slice into the vertical
// Wochenskala's per-day bars (ZeitWeekDay) — the single builder shared by
// Zeit (/zeit, heuteDataFor) and Woche (/woche, wocheDataFor) so their
// .weekbar skylines can never drift apart again (L4 Final-Review Finding 1:
// Woche used to reuse the lossy WocheDayVM.Pct for its weekbar, which zeroed
// a logged Saturday out whenever its Target was 0 — Codex-Fund #3 already
// flagged that Pct as unusable here). scale is computed per day as
// max(that day's own Target, the week's own max logged) so a zero-target
// weekend day never divides by zero (while workdays still share a
// roughly-comparable scale across the week). off reports which local dates
// carry a day-off so a weekend/holiday day shows "frei" instead of a bare
// "—". Values use FmtClockShort ("6:10", Mockup Z.871–879) — NOT
// FmtVerbose, which the Woche detail rows/Kennzahlen panels keep unchanged
// (page prose stays "6h 10m"; only the weekbar skyline is now identical).
func BuildWeekBars(ctx context.Context, week []domain.WeekDay, now time.Time, off map[string]domain.DayOff) []ZeitWeekDay {
	loc := now.Location()
	var maxLogged time.Duration
	logged := make([]time.Duration, len(week))
	for i, wd := range week {
		logged[i] = wd.Total(now)
		if logged[i] > maxLogged {
			maxLogged = logged[i]
		}
	}
	out := make([]ZeitWeekDay, 0, len(week))
	for i, wd := range week {
		l := logged[i]
		scale := wd.Target
		if maxLogged > scale {
			scale = maxLogged
		}
		pct := 0
		if scale > 0 {
			pct = ClampPct(int(l * 100 / scale))
		}
		key := wd.Date.In(loc).Format("2006-01-02")
		_, isOff := off[key]
		valueStr := FmtClockShort(l)
		if l == 0 {
			if isOff || isWeekendDate(wd.Date) {
				valueStr = i18n.T(ctx, "zeit.weekDayOff")
			} else {
				valueStr = "—"
			}
		}
		label := weekdayShortDE(wd.Date.Weekday())
		if wd.IsToday {
			label += " · heute"
		}
		out = append(out, ZeitWeekDay{
			Label:    label,
			ValueStr: valueStr,
			Pct:      pct,
			Has:      l > 0,
			Today:    wd.IsToday,
		})
	}
	return out
}

// isWeekendDate reports whether t falls on Sat/Sun (in t's own location).
// Mirrors httpserver's isWeekendTime (woche_summary.go) — kept as a small
// local twin here since webui cannot import the httpserver package.
func isWeekendDate(t time.Time) bool {
	wd := t.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}

// weekdayShortDE maps a weekday to its short German label ("Mo".."So").
// Mirrors httpserver's wocheWeekdayLabel (webui_woche.go) — kept as a small
// local twin here since webui cannot import the httpserver package.
func weekdayShortDE(wd time.Weekday) string {
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
