package domain

import "time"

// HasOverlap reports whether the half-open interval [start, stop) intersects any
// session in existing, skipping the session whose ID == excludeID (pass "" to
// skip nothing). A nil stop means an open-ended running interval [start, +inf).
//
// This is the single source of the "no two sessions of one owner may overlap"
// rule; every path that persists an arbitrary interval (AddSession, EditSession)
// calls it. Touching edges do not overlap (the interval is half-open).
func HasOverlap(existing []WorkSession, start time.Time, stop *time.Time, excludeID string) bool {
	for _, e := range existing {
		if e.ID == excludeID {
			continue
		}
		if intervalsIntersect(start, stop, e.Start, e.Stop) {
			return true
		}
	}
	return false
}

// intervalsIntersect reports whether [aStart, aStop) and [bStart, bStop) overlap,
// where a nil stop denotes +inf. Rule: aStart < bStop && bStart < aStop.
func intervalsIntersect(aStart time.Time, aStop *time.Time, bStart time.Time, bStop *time.Time) bool {
	return beforeEnd(aStart, bStop) && beforeEnd(bStart, aStop)
}

// beforeEnd reports t < end, treating a nil end as +inf (always true).
func beforeEnd(t time.Time, end *time.Time) bool {
	return end == nil || t.Before(*end)
}
