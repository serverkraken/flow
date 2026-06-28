package domain

import "time"

// RecordSession is the per-session view stats aggregation needs: the tags
// (for the by-tag tally) and the already-computed elapsed. Built from a
// WorkSession at the use-case boundary so the pure aggregators stay I/O-free
// and independent of the live/stopped distinction.
type RecordSession struct {
	Tags    []string
	Elapsed time.Duration
}

// DayRecord is one calendar day's history entry used by stats/burndown.
type DayRecord struct {
	Date     time.Time
	Sessions []RecordSession
	Total    time.Duration
	Target   time.Duration
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
