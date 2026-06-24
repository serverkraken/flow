package domain

import "time"

// TimeRange is a half-open [Start, Stop) interval.
type TimeRange struct {
	Start time.Time
	Stop  time.Time
}

// SplitDaily splits [start, stop] into contiguous per-calendar-day ranges, cut
// at local midnight in loc. A same-day span returns a single unchanged range.
// Used so a timer running across midnight books a separate session per day,
// keeping each day's totals accurate. start..stop must be ordered; a degenerate
// span returns one range.
func SplitDaily(start, stop time.Time, loc *time.Location) []TimeRange {
	if loc == nil {
		loc = time.Local
	}
	if !stop.After(start) {
		return []TimeRange{{Start: start, Stop: stop}}
	}
	var out []TimeRange
	cur := start
	for {
		lc := cur.In(loc)
		nextMidnight := time.Date(lc.Year(), lc.Month(), lc.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
		if !nextMidnight.Before(stop) {
			out = append(out, TimeRange{Start: cur, Stop: stop})
			return out
		}
		out = append(out, TimeRange{Start: cur, Stop: nextMidnight})
		cur = nextMidnight
	}
}
