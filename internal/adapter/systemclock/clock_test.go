package systemclock

import (
	"testing"
	"time"
)

func TestNowIsCloseToRealTime(t *testing.T) {
	before := time.Now()
	got := Clock{}.Now()
	after := time.Now()
	if got.Before(before) || got.After(after) {
		t.Fatalf("Clock.Now() = %v, want between %v and %v", got, before, after)
	}
}
