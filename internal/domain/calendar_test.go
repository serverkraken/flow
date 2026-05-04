package domain_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestSplitAtMidnight(t *testing.T) {
	loc := time.Local
	at := func(y, m, d, h, min int) time.Time {
		return time.Date(y, time.Month(m), d, h, min, 0, 0, loc)
	}

	t.Run("same day returns single session", func(t *testing.T) {
		start := at(2026, 4, 29, 9, 0)
		stop := at(2026, 4, 29, 17, 30)
		parts := domain.SplitAtMidnight(start, stop)
		if len(parts) != 1 {
			t.Fatalf("want 1 part, got %d", len(parts))
		}
		got := parts[0]
		if !got.Date.Equal(at(2026, 4, 29, 0, 0)) {
			t.Errorf("Date = %v", got.Date)
		}
		if got.Elapsed != 8*time.Hour+30*time.Minute {
			t.Errorf("Elapsed = %v", got.Elapsed)
		}
	})

	t.Run("crosses one midnight", func(t *testing.T) {
		start := at(2026, 4, 29, 22, 0)
		stop := at(2026, 4, 30, 1, 30)
		parts := domain.SplitAtMidnight(start, stop)
		if len(parts) != 2 {
			t.Fatalf("want 2 parts, got %d", len(parts))
		}
		// First part: 22:00 → 00:00 = 2h, dated 4/29.
		if parts[0].Elapsed != 2*time.Hour {
			t.Errorf("part[0].Elapsed = %v", parts[0].Elapsed)
		}
		if !parts[0].Date.Equal(at(2026, 4, 29, 0, 0)) {
			t.Errorf("part[0].Date = %v", parts[0].Date)
		}
		// Second part: 00:00 → 01:30 = 1h30m, dated 4/30.
		if parts[1].Elapsed != 90*time.Minute {
			t.Errorf("part[1].Elapsed = %v", parts[1].Elapsed)
		}
		if !parts[1].Date.Equal(at(2026, 4, 30, 0, 0)) {
			t.Errorf("part[1].Date = %v", parts[1].Date)
		}
	})

	t.Run("crosses two midnights", func(t *testing.T) {
		start := at(2026, 4, 29, 23, 0)
		stop := at(2026, 5, 1, 1, 0) // 1h on 4/29, 24h on 4/30, 1h on 5/1
		parts := domain.SplitAtMidnight(start, stop)
		if len(parts) != 3 {
			t.Fatalf("want 3 parts, got %d", len(parts))
		}
		want := []time.Duration{time.Hour, 24 * time.Hour, time.Hour}
		for i, w := range want {
			if parts[i].Elapsed != w {
				t.Errorf("part[%d].Elapsed = %v, want %v", i, parts[i].Elapsed, w)
			}
		}
	})

	t.Run("stop equals start uses single-session fallback", func(t *testing.T) {
		// Same-day path is also taken when stop == start (sameDay && !After).
		ts := at(2026, 4, 29, 12, 0)
		parts := domain.SplitAtMidnight(ts, ts)
		if len(parts) != 1 {
			t.Fatalf("want 1 part, got %d", len(parts))
		}
		if parts[0].Elapsed != 0 {
			t.Errorf("Elapsed = %v, want 0", parts[0].Elapsed)
		}
	})

	t.Run("stop before start returns nil", func(t *testing.T) {
		// Inverted spans were previously silently accepted as a
		// degenerate single-session record with negative Elapsed —
		// downstream accumulators (WeekDay.Total, MonthBurndown) didn't
		// clamp negatives so the bad input shrank totals. The function
		// now refuses inverted input; callers (session_writer) already
		// validate before calling.
		start := at(2026, 4, 30, 9, 0)
		stop := at(2026, 4, 29, 23, 0)
		parts := domain.SplitAtMidnight(start, stop)
		if parts != nil {
			t.Fatalf("want nil for inverted span, got %d parts", len(parts))
		}
	})
}

func TestIsWorkday(t *testing.T) {
	mon := time.Date(2026, time.April, 27, 9, 0, 0, 0, time.Local) // Mon
	sat := time.Date(2026, time.May, 2, 9, 0, 0, 0, time.Local)
	sun := time.Date(2026, time.May, 3, 9, 0, 0, 0, time.Local)

	noDayOff := func(time.Time) bool { return false }
	always := func(time.Time) bool { return true }

	tests := []struct {
		name     string
		date     time.Time
		isDayOff func(time.Time) bool
		want     bool
	}{
		{"weekday no dayoff", mon, noDayOff, true},
		{"weekday with dayoff", mon, always, false},
		{"saturday is not workday", sat, noDayOff, false},
		{"sunday is not workday", sun, noDayOff, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := domain.IsWorkday(tc.date, tc.isDayOff); got != tc.want {
				t.Errorf("IsWorkday = %v, want %v", got, tc.want)
			}
		})
	}
}
