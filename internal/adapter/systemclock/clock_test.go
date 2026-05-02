package systemclock_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/systemclock"
	"github.com/serverkraken/flow/internal/ports"
)

var _ ports.Clock = systemclock.Clock{}

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
