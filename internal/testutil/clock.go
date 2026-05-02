package testutil

import "time"

// FixedClock implements ports.Clock with a settable, deterministic time.
// Use Advance to step forward without recreating the clock.
type FixedClock struct {
	T time.Time
}

// Now returns the current value of T.
func (c *FixedClock) Now() time.Time { return c.T }

// Advance steps T forward by d. Negative values move backward.
func (c *FixedClock) Advance(d time.Duration) { c.T = c.T.Add(d) }
