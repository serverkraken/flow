// Package webui holds the templ view components for the server-rendered UI.
package webui

import (
	"fmt"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// fmtDur renders a duration as HH:MM (clamped at zero).
func fmtDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%02d:%02d", int(d.Hours()), int(d.Minutes())%60)
}

// fmtInt formats an int as a string (used in templ expressions).
func fmtInt(n int) string { return fmt.Sprintf("%d", n) }

// monthBarStyle returns an inline CSS width percentage for the monthly burndown bar.
// It clamps the result to [0, 100].
func monthBarStyle(d StatsData) string {
	pct := monthBarPct(d)
	return fmt.Sprintf("width: %d%%", pct)
}

// monthBarPct computes the burndown bar percentage from StatsData strings.
// Since StatsData already carries pre-formatted strings we use a simple heuristic:
// MonthOnTrack and the strings don't give us raw minutes — store pct on StatsData instead.
// This function exists for the templ call; it delegates to d.MonthPct.
func monthBarPct(d StatsData) int {
	if d.MonthPct < 0 {
		return 0
	}
	if d.MonthPct > 100 {
		return 100
	}
	return d.MonthPct
}

// weekBarStyle returns an inline CSS width percentage for a week row bar.
func weekBarStyle(row StatsWeekRow) string {
	pct := row.Pct
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return fmt.Sprintf("width: %d%%", pct)
}


// fmtHM renders a timestamp as local HH:MM.
func fmtHM(t time.Time) string { return t.Local().Format("15:04") }

// projName resolves a project id to its name, or "—" when unset/unknown.
func projName(projects []domain.Project, id *string) string {
	if id == nil {
		return "—"
	}
	for _, p := range projects {
		if p.ID == *id {
			return p.Name
		}
	}
	return "—"
}

// stopHM renders a session's stop time as HH:MM, or "…" while running.
func stopHM(s domain.WorkSession) string {
	if s.Stop == nil {
		return "…"
	}
	return fmtHM(*s.Stop)
}

// isCurrentProject reports whether id is the session's current project, used
// to pre-select the edit form's project <option> so a save preserves it.
func isCurrentProject(s domain.WorkSession, id string) bool {
	return s.ProjectID != nil && *s.ProjectID == id
}
