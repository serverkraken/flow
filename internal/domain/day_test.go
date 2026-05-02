package domain_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// fixedNow returns a deterministic "now" used across the day tests.
// Chosen as a Wednesday afternoon so weekend logic is exercised elsewhere.
func fixedNow() time.Time {
	return time.Date(2026, time.April, 29, 14, 30, 0, 0, time.Local)
}

func TestDay_IsRunning(t *testing.T) {
	now := fixedNow()
	cases := []struct {
		name string
		day  domain.Day
		want bool
	}{
		{"idle", domain.Day{}, false},
		{"running", domain.Day{Active: ptr(now.Add(-time.Hour))}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.day.IsRunning(); got != tc.want {
				t.Errorf("IsRunning() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDay_IsPaused(t *testing.T) {
	now := fixedNow()
	pause := now.Add(-30 * time.Minute)
	active := now.Add(-time.Hour)

	cases := []struct {
		name string
		day  domain.Day
		want bool
	}{
		{"idle no pause", domain.Day{}, false},
		{"paused", domain.Day{PausedAt: &pause}, true},
		{"running ignores pause marker", domain.Day{Active: &active, PausedAt: &pause}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.day.IsPaused(); got != tc.want {
				t.Errorf("IsPaused() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDay_Total(t *testing.T) {
	now := fixedNow()
	logged := 2*time.Hour + 15*time.Minute

	t.Run("idle returns logged", func(t *testing.T) {
		d := domain.Day{Logged: logged}
		if got := d.Total(now); got != logged {
			t.Errorf("Total = %v, want %v", got, logged)
		}
	})

	t.Run("active started today", func(t *testing.T) {
		start := now.Add(-90 * time.Minute)
		d := domain.Day{Logged: logged, Active: &start}
		want := logged + 90*time.Minute
		if got := d.Total(now); got != want {
			t.Errorf("Total = %v, want %v", got, want)
		}
	})

	t.Run("active crossed midnight is capped to midnight", func(t *testing.T) {
		// Active started yesterday at 23:30 — today's slice begins at 00:00.
		start := time.Date(2026, time.April, 28, 23, 30, 0, 0, time.Local)
		d := domain.Day{Logged: logged, Active: &start}
		// now is 14:30 today, so today's active slice is 14h 30m.
		want := logged + 14*time.Hour + 30*time.Minute
		if got := d.Total(now); got != want {
			t.Errorf("Total = %v, want %v", got, want)
		}
	})
}

func TestWeekDay_Total(t *testing.T) {
	now := fixedNow()
	logged := time.Hour

	t.Run("past day uses logged only", func(t *testing.T) {
		w := domain.WeekDay{Logged: logged, IsToday: false}
		if got := w.Total(now); got != logged {
			t.Errorf("Total = %v, want %v", got, logged)
		}
	})

	t.Run("today without active uses logged only", func(t *testing.T) {
		w := domain.WeekDay{Logged: logged, IsToday: true, Active: nil}
		if got := w.Total(now); got != logged {
			t.Errorf("Total = %v, want %v", got, logged)
		}
	})

	t.Run("today with active adds tail", func(t *testing.T) {
		start := now.Add(-45 * time.Minute)
		w := domain.WeekDay{Logged: logged, IsToday: true, Active: &start}
		want := logged + 45*time.Minute
		if got := w.Total(now); got != want {
			t.Errorf("Total = %v, want %v", got, want)
		}
	})

	t.Run("today with active across midnight clamps to midnight", func(t *testing.T) {
		start := time.Date(2026, time.April, 28, 22, 0, 0, 0, time.Local)
		w := domain.WeekDay{Logged: logged, IsToday: true, Active: &start}
		// 14:30 today − 00:00 = 14h 30m, plus logged 1h.
		want := logged + 14*time.Hour + 30*time.Minute
		if got := w.Total(now); got != want {
			t.Errorf("Total = %v, want %v", got, want)
		}
	})
}

func ptr[T any](v T) *T { return &v }
