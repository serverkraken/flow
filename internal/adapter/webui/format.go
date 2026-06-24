// Package webui holds the templ view components for the server-rendered UI.
package webui

import (
	"fmt"
	"time"
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
