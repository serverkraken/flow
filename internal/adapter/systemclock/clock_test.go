package systemclock_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/systemclock"
	kompendiumports "github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/ports"
)

// Compile-time proof that the same adapter satisfies both hexagons'
// Clock contracts. If either Clock interface drifts, this fails to
// build — the dedup decision (K0 §8) breaks loudly instead of silently.
var (
	_ ports.Clock           = systemclock.Clock{}
	_ kompendiumports.Clock = systemclock.Clock{}
)

func TestNow_TracksWallClock(t *testing.T) {
	before := time.Now()
	got := systemclock.New().Now()
	after := time.Now()

	if got.Before(before) || got.After(after) {
		t.Errorf("Now=%v outside [%v, %v]", got, before, after)
	}
}

func TestNow_ZeroValueWorks(t *testing.T) {
	var c systemclock.Clock
	if c.Now().IsZero() {
		t.Error("zero-value clock returned zero time")
	}
}
