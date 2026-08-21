package webui

import (
	"testing"
	"time"
)

func TestWeekSollMinutes(t *testing.T) {
	got := WeekSollMinutes(480, map[time.Weekday]int{time.Friday: 240})
	if got != 4*480+240 {
		t.Errorf("WeekSollMinutes = %d", got)
	}
	if FmtMinutesClock(got) != "36:00 h" || FmtMinutesClock(5) != "0:05 h" {
		t.Errorf("FmtMinutesClock: %s / %s", FmtMinutesClock(got), FmtMinutesClock(5))
	}
}
