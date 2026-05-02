package testutil

import (
	"time"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// FixedClock is a ports.Clock whose Now always returns Time. Use it in
// use-case tests where wall-clock dependence would make assertions flaky.
type FixedClock struct {
	Time time.Time
}

// Now implements ports.Clock.
func (c FixedClock) Now() time.Time { return c.Time }

var _ ports.Clock = FixedClock{}
