package domain

import "time"

// Day holds today's logged sessions plus optional active/pause markers.
type Day struct {
	Sessions []Session
	Active   *time.Time
	// PausedAt is the stop time of the last pause, set when in pause-mode
	// (no active session, but a pause marker exists).
	PausedAt *time.Time
	Logged   time.Duration
	Target   time.Duration
}

// IsPaused reports whether the user paused (Pause()) and hasn't resumed yet.
// Distinct from "fresh idle" — UI shows "in Pause seit HH:MM" instead of
// "noch nicht erfasst", and Resume() picks up where Pause() left off.
func (d Day) IsPaused() bool { return d.Active == nil && d.PausedAt != nil }

// IsRunning reports whether a session is currently active.
func (d Day) IsRunning() bool { return d.Active != nil }

// Total returns logged + active elapsed (capped at midnight for the current
// day so a session that crossed midnight only contributes today's slice).
func (d Day) Total(now time.Time) time.Duration {
	total := d.Logged
	if d.Active != nil {
		midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		start := *d.Active
		if start.Before(midnight) {
			start = midnight
		}
		total += now.Sub(start)
	}
	return total
}

// WeekDay is one day in the week view.
type WeekDay struct {
	Date    time.Time
	Logged  time.Duration
	Active  *time.Time
	Target  time.Duration
	IsToday bool
}

// Total returns logged + active elapsed for this day. The active tail is only
// added when this is today's row — past days never have a live counter.
func (w WeekDay) Total(now time.Time) time.Duration {
	if !w.IsToday || w.Active == nil {
		return w.Logged
	}
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	start := *w.Active
	if start.Before(midnight) {
		start = midnight
	}
	return w.Logged + now.Sub(start)
}

// DayRecord is one calendar day's history entry, used by stats/export views.
type DayRecord struct {
	Date     time.Time
	Sessions []Session
	Total    time.Duration
	Target   time.Duration
}
