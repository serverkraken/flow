package webui

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestFmtDur(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{90 * time.Minute, "01:30"},
		{0, "00:00"},
		{-time.Minute, "00:00"},
		{61 * time.Minute, "01:01"},
	}
	for _, tc := range cases {
		got := fmtDur(tc.d)
		if got != tc.want {
			t.Errorf("fmtDur(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

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
