// Package systemclock is the production ports.Clock.
package systemclock

import "time"

type Clock struct{}

func (Clock) Now() time.Time { return time.Now() }
