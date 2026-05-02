package systemclock

import "time"

// Clock returns the wall-clock time. The zero value is usable; New is
// kept for API symmetry with the other adapters.
type Clock struct{}

// New constructs a Clock. The zero value works equally well.
func New() Clock { return Clock{} }

// Now returns time.Now in the local timezone.
func (Clock) Now() time.Time { return time.Now() }
