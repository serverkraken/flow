// Package dashboard renders the WebUI landing surface at `/` — the
// "Action Stream" layout (Mockup B): live-session banner, today/saldo
// numerics, 24-hour day-bar, activity timeline, three mini-cards.
//
// Pure-Go formatting helpers live here so the templ source stays
// markup-only; tests can exercise the formatters without templ codegen.
//
// Shared formatters (HHMM / signed-HHMM / hour-mask / Monday-of /
// German date headers / relative time) live in
// internal/webui/format/. This file keeps only the dashboard-specific
// aggregators that build the activity stream + week mini-cards.
package dashboard

import (
	"sort"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// ActivityKind tags each row in the activity stream. The kind switches
// the dot color + glyph and the localised verb in the renderer. M6 only
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
