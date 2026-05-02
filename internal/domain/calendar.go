package domain

import "time"

// SplitAtMidnight splits a span [start, stop) into one Session per calendar
// day so each day reflects its own elapsed time. Returns a single-element
// slice when the span doesn't cross midnight.
func SplitAtMidnight(start, stop time.Time) []Session {
	if !SameDay(start, stop) && stop.After(start) {
		var parts []Session
		cur := start
		for {
			midnight := time.Date(cur.Year(), cur.Month(), cur.Day(), 0, 0, 0, 0, cur.Location()).
				AddDate(0, 0, 1)
			end := midnight
			if !end.Before(stop) {
				end = stop
			}
			parts = append(parts, Session{
				Date:    time.Date(cur.Year(), cur.Month(), cur.Day(), 0, 0, 0, 0, cur.Location()),
				Start:   cur,
				Stop:    end,
				Elapsed: end.Sub(cur),
			})
			if !end.Before(stop) {
				break
			}
			cur = end
		}
		return parts
	}
	return []Session{{
		Date:    time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location()),
		Start:   start,
		Stop:    stop,
		Elapsed: stop.Sub(start),
	}}
}

// IsWorkday reports whether t is neither a weekend nor a configured day-off.
// The isDayOff predicate is injected so the domain stays I/O-free; callers
// build it from the dayoff store at the use-case boundary.
func IsWorkday(t time.Time, isDayOff func(time.Time) bool) bool {
	if isWeekend(t) {
		return false
	}
	if isDayOff(t) {
		return false
	}
	return true
}

// isWeekend reports whether t falls on Saturday or Sunday.
func isWeekend(t time.Time) bool {
	wd := t.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}

// SameDay reports whether a and b fall on the same calendar day in their
// own location. Exported because use cases reach for it when filtering
// today's sessions.
func SameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

// isoMonday returns the Monday 00:00 of t's ISO week, in t's location.
func isoMonday(t time.Time) time.Time {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	d := t.AddDate(0, 0, -(wd - 1))
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, t.Location())
}
