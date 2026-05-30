// Package domain — datefmt.go: zentrale German-Date-Format-Helpers
// für TUI + tmux-Surfaces. Skill §German UI: canonical `Mi., 22. Apr.`
// für die status-bar germandate-plugin-Spiegelung. Drei Formen:
package domain

import (
	"fmt"
	"time"
)

// DateFormat selects a renderer.
type DateFormat int

const (
	// DateShort renders as "Mi., 28. Mai" — for status bars and
	// headlines where the year is contextual.
	DateShort DateFormat = iota
	// DateLong renders as "Mi., 28. Mai 2026" — when the year matters
	// (heatmap-status, history drill).
	DateLong
	// DateNumeric renders as "2026-05-28" — sortable, monospace-aligned,
	// for list rows where dates stack.
	DateNumeric
)

// FmtDateDe renders t in the chosen format.
func FmtDateDe(t time.Time, f DateFormat) string {
	switch f {
	case DateLong:
		return fmt.Sprintf("%s., %d. %s %d",
			WeekdayShortDe(t.Weekday()), t.Day(), MonthShortDe(t.Month()), t.Year())
	case DateNumeric:
		return t.Format("2006-01-02")
	}
	return fmt.Sprintf("%s., %d. %s",
		WeekdayShortDe(t.Weekday()), t.Day(), MonthShortDe(t.Month()))
}

// FmtDateRangeDe renders a from–to range. Compact when month matches
// ("1.–7. Mai"), expanded otherwise ("28. Mai – 3. Jun").
func FmtDateRangeDe(from, to time.Time) string {
	if from.Month() == to.Month() && from.Year() == to.Year() {
		return fmt.Sprintf("%d.–%d. %s", from.Day(), to.Day(), MonthShortDe(from.Month()))
	}
	return fmt.Sprintf("%d. %s – %d. %s",
		from.Day(), MonthShortDe(from.Month()),
		to.Day(), MonthShortDe(to.Month()))
}
