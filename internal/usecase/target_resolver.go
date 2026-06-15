package usecase

import (
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// TargetResolver is a pure, per-request value object computing the daily work
// target for a date. Priority: day-off override > per-weekday override >
// default. Build it from a user's Settings and the merged day-off set
// (ListDayOffs) at the use-case boundary so it stays I/O-free.
type TargetResolver struct {
	Default time.Duration
	// Weekday is indexed by int(time.Weekday) (Sunday=0). A nil entry means
	// "no override → use Default"; a non-nil entry (incl. a 0-duration) is an
	// explicit override.
	Weekday [7]*time.Duration
	// DayOffs is keyed by "2006-01-02"; presence marks the date as a day-off
	// (manual or computed holiday) and supplies its target override.
	DayOffs map[string]domain.DayOff
}

func dayKey(t time.Time) string { return t.Format("2006-01-02") }

// For returns the target work duration for date.
func (r TargetResolver) For(date time.Time) time.Duration {
	if d, ok := r.DayOffs[dayKey(date)]; ok {
		return d.Target // 0 = full day off; >0 = half-day override
	}
	if o := r.Weekday[int(date.Weekday())]; o != nil {
		return *o
	}
	return r.Default
}

// IsDayOff reports whether date carries a day-off (manual or holiday).
func (r TargetResolver) IsDayOff(date time.Time) bool {
	_, ok := r.DayOffs[dayKey(date)]
	return ok
}

// IsWorkday reports whether date is neither weekend nor day-off.
func (r TargetResolver) IsWorkday(date time.Time) bool {
	return domain.IsWorkday(date, r.IsDayOff)
}
