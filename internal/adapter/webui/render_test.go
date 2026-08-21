package webui

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestWeekDay_Total_ActivePath(t *testing.T) {
	now := time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC)
	// An active session that started today at 12:00 → 2h elapsed.
	activeStart := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	wd := domain.WeekDay{
		Date:    time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
		Logged:  30 * time.Minute,
		Active:  &activeStart,
		Target:  8 * time.Hour,
		IsToday: true,
	}
	got := wd.Total(now)
	// Logged (30m) + active elapsed (14:00 - 12:00 = 2h) = 2h30m.
	want := 2*time.Hour + 30*time.Minute
	if got != want {
		t.Errorf("WeekDay.Total(active): want %v, got %v", want, got)
	}
}

func TestWeekDay_Total_ActiveBeforeMidnight(t *testing.T) {
	now := time.Date(2026, 6, 15, 1, 0, 0, 0, time.UTC)
	// Active session started yesterday before midnight: should be clamped to midnight.
	activeStart := time.Date(2026, 6, 14, 23, 0, 0, 0, time.UTC)
	wd := domain.WeekDay{
		Date:    time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
		Logged:  0,
		Active:  &activeStart,
		Target:  8 * time.Hour,
		IsToday: true,
	}
	got := wd.Total(now)
	// Clamped to midnight; elapsed = 01:00 - 00:00 = 1h.
	want := time.Hour
	if got != want {
		t.Errorf("WeekDay.Total(active before midnight): want %v, got %v", want, got)
	}
}

// TestFmtClock pins the running clock's shape against the mockups: hours
// unpadded, minutes padded, no unit. The draft branch and this one both had a
// helper called fmtDur with DIFFERENT padding — a ported caller silently got
// the wrong one, which is why this format now has its own name and its own
// test.
func TestFmtClock(t *testing.T) {
	for _, c := range []struct {
		d    time.Duration
		want string
	}{
		{2*time.Hour + 41*time.Minute, "2:41"},
		{9 * time.Minute, "0:09"},
		{12 * time.Hour, "12:00"},
		{-time.Hour, "0:00"},
	} {
		if got := fmtClock(c.d); got != c.want {
			t.Errorf("fmtClock(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}
