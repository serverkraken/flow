package domain

import "time"

// Range is a half-open date interval [From, To). Zero From or To means
// "no bound on that side" — both zero means "all dates".
type Range struct {
	From time.Time // inclusive (00:00 of the day in local TZ)
	To   time.Time // exclusive (00:00 of the day in local TZ)
}

// ContainsDate reports whether d (a date-only value) falls inside r.
func (r Range) ContainsDate(d time.Time) bool {
	if !r.From.IsZero() && d.Before(r.From) {
		return false
	}
	if !r.To.IsZero() && !d.Before(r.To) {
		return false
	}
	return true
}
