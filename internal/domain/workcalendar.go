package domain

import "time"

// IsWorkday reports whether t is neither a weekend nor a configured day-off.
// The isDayOff predicate is injected so the domain stays I/O-free.
func IsWorkday(t time.Time, isDayOff func(time.Time) bool) bool {
	if isWeekend(t) {
		return false
	}
	return !isDayOff(t)
}

func isWeekend(t time.Time) bool {
	wd := t.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}
