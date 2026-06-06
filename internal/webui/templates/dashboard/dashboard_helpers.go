// Package dashboard renders the WebUI landing surface at `/` — the
// "Action Stream" layout (Mockup B): live-session banner, today/saldo
// numerics, 24-hour day-bar, activity timeline, three mini-cards.
//
// Pure-Go formatting helpers live here so the templ source stays
// markup-only; tests can exercise the formatters without templ codegen.
package dashboard

import (
	"fmt"
	"sort"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// FormatHHMM renders a duration as "H:MM" (with no leading sign).
// Negative durations are clamped to 0 — callers that want a signed
// readout should call FormatSignedHHMM instead.
func FormatHHMM(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	return fmt.Sprintf("%d:%02d", h, m)
}

// FormatSignedHHMM renders a duration as "+H:MM" / "-H:MM" / "0:00".
// Used for saldo readouts where the sign carries meaning.
func FormatSignedHHMM(d time.Duration) string {
	if d == 0 {
		return "0:00"
	}
	sign := "+"
	if d < 0 {
		sign = "-"
		d = -d
	}
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	return fmt.Sprintf("%s%d:%02d", sign, h, m)
}

// HourMask returns a 24-element array where mask[h]=1 if any session in
// `sessions` overlaps the [h:00, h+1:00) window of `now`'s local day, or
// if `active` started during that hour (or earlier today and the hour is
// ≤ now's hour). Hours outside today's range are 0.
//
// A session that crosses an hour boundary marks every hour it touches.
func HourMask(sessions []domain.Session, active *time.Time, now time.Time) [24]int {
	var mask [24]int
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	dayEnd := dayStart.AddDate(0, 0, 1)

	for _, s := range sessions {
		start := s.Start
		stop := s.Stop
		if !stop.After(dayStart) || !start.Before(dayEnd) {
			continue
		}
		if start.Before(dayStart) {
			start = dayStart
		}
		if stop.After(dayEnd) {
			stop = dayEnd
		}
		for h := start.Hour(); h <= stop.Hour() && h < 24; h++ {
			// A session that ends exactly at hour boundary (stop.Minute=0 and
			// stop.Second=0) should not mark the boundary hour as worked.
			if h == stop.Hour() && stop.Minute() == 0 && stop.Second() == 0 && h != start.Hour() {
				continue
			}
			mask[h] = 1
		}
	}

	if active != nil {
		start := *active
		if start.Before(dayStart) {
			start = dayStart
		}
		if !start.Before(dayEnd) {
			return mask
		}
		for h := start.Hour(); h <= now.Hour() && h < 24; h++ {
			mask[h] = 1
		}
	}
	return mask
}

// HumanRelativeTime renders `t` relative to `now` in a humane German
// format ("heute · 09:28", "gestern · 17:45", "vor 2 Tagen · 14:12",
// "vor 8s"). Sub-minute differences emit "vor Ns" / "vor Nm" so the
// activity stream's "letzte Aktivität" card has a useful readout for
// fresh events.
func HumanRelativeTime(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	delta := now.Sub(t)
	if delta < 0 {
		delta = 0
	}
	if delta < time.Minute {
		secs := int(delta / time.Second)
		if secs < 1 {
			secs = 1
		}
		return fmt.Sprintf("vor %ds", secs)
	}
	if delta < time.Hour {
		mins := int(delta / time.Minute)
		return fmt.Sprintf("vor %dm", mins)
	}

	tDay := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	nowDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	days := int(nowDay.Sub(tDay) / (24 * time.Hour))
	hm := t.Format("15:04")
	switch days {
	case 0:
		return fmt.Sprintf("heute · %s", hm)
	case 1:
		return fmt.Sprintf("gestern · %s", hm)
	default:
		return fmt.Sprintf("vor %d Tagen · %s", days, hm)
	}
}

// ActivityEvent is one row in the activity stream. Kind switches the
// dot color + glyph and the localised verb in the renderer. M6 only
// emits SessionStarted / SessionStopped — Note + Conflict event types
// land later when an EventLog table exists.
type ActivityKind int

const (
	// ActivitySessionStopped — a finished session row.
	ActivitySessionStopped ActivityKind = iota
	// ActivitySessionStarted — currently active session (live).
	ActivitySessionStarted
)

// ActivityEvent is the row payload the dashboard template renders.
type ActivityEvent struct {
	Kind        ActivityKind
	When        time.Time // event timestamp for sorting + relative label
	ProjectName string    // resolved via Projects.GetByID
	Tag         string    // domain.Session.Tag — optional, e.g. "design"
	Duration    time.Duration
}

// BuildActivityStream merges finished sessions + the (single) active
// session into a unified, newest-first activity feed truncated to `max`
// items. ProjectName must be resolved by the caller (we don't want to
// do per-row DB lookups inside the renderer).
func BuildActivityStream(
	sessions []domain.Session,
	active *domain.ActiveSession,
	projectName func(projectID string) string,
	max int,
	now time.Time,
) []ActivityEvent {
	events := make([]ActivityEvent, 0, len(sessions)+1)
	if active != nil {
		events = append(events, ActivityEvent{
			Kind:        ActivitySessionStarted,
			When:        active.StartedAt,
			ProjectName: projectName(active.ProjectID),
			Tag:         active.Tag,
			Duration:    now.Sub(active.StartedAt),
		})
	}
	for _, s := range sessions {
		events = append(events, ActivityEvent{
			Kind:        ActivitySessionStopped,
			When:        s.Stop,
			ProjectName: projectName(s.ProjectID),
			Tag:         s.Tag,
			Duration:    s.Elapsed,
		})
	}
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].When.After(events[j].When)
	})
	if len(events) > max {
		events = events[:max]
	}
	return events
}

// TopProjectOfWeek picks the project with the longest total time across
// `sessions` and returns its (id, total). Returns empty id when sessions
// is empty.
func TopProjectOfWeek(sessions []domain.Session) (projectID string, total time.Duration) {
	if len(sessions) == 0 {
		return "", 0
	}
	totals := make(map[string]time.Duration, len(sessions))
	for _, s := range sessions {
		totals[s.ProjectID] += s.Elapsed
	}
	for id, sum := range totals {
		if sum > total {
			projectID = id
			total = sum
		}
	}
	return projectID, total
}

// WeekTotals sums logged + target across a slice of WeekDays.
func WeekTotals(week []domain.WeekDay, now time.Time) (logged, target time.Duration) {
	for _, wd := range week {
		logged += wd.Total(now)
		target += wd.Target
	}
	return logged, target
}

// MondayOf returns 00:00 of t's ISO Monday in t's location.
// Duplicated here (with internal/usecase) to avoid pulling that package
// into the template helpers.
func MondayOf(t time.Time) time.Time {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	return day.AddDate(0, 0, -(wd - 1))
}

// FormatGermanDateHeader renders "Sa · 06. Juni · KW 23" for the tiny
// header above the two-up numerics. Plain Go date formatting with a
// hand-rolled German weekday + month table — i18n stays out of M6.
func FormatGermanDateHeader(t time.Time) string {
	wd := germanWeekdayShort(t.Weekday())
	month := germanMonth(t.Month())
	_, week := t.ISOWeek()
	return fmt.Sprintf("%s · %02d. %s · KW %d", wd, t.Day(), month, week)
}

func germanWeekdayShort(w time.Weekday) string {
	switch w {
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

func germanMonth(m time.Month) string {
	switch m {
	case time.January:
		return "Januar"
	case time.February:
		return "Februar"
	case time.March:
		return "März"
	case time.April:
		return "April"
	case time.May:
		return "Mai"
	case time.June:
		return "Juni"
	case time.July:
		return "Juli"
	case time.August:
		return "August"
	case time.September:
		return "September"
	case time.October:
		return "Oktober"
	case time.November:
		return "November"
	default:
		return "Dezember"
	}
}
