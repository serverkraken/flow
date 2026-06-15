package domain

import "time"

// MonthBurndownReport summarises progress against the implied monthly target
// (sum of TargetFor(d) over all workdays in the month, day-offs subtracted).
//
// Used by the Today header to show "Monat 78h / 160h · vorne 2h" — a glance
// at whether the running balance is healthy. The compute function that
// produces a report from records lives separately (added in F1.1's
// aggregate step).
type MonthBurndownReport struct {
	Total       time.Duration // logged so far this month (incl. today logged + active)
	Target      time.Duration // sum of targets over all workdays of the month
	Saldo       time.Duration // Total - expected-by-now
	OnTrack     bool          // Saldo >= 0
	WorkdaysAll int
	WorkdaysDue int // workdays whose date <= today
}
