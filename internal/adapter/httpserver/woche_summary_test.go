package httpserver

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// TestComputeWocheSummary_MonFriOnly proves weekends are excluded from both the
// logged total and the target total, goals-hit counts only met Mon–Fri workdays,
// and a day-off weekday is removed from the workday/goal counters.
func TestComputeWocheSummary_MonFriOnly(t *testing.T) {
	mon := time.Date(2026, 6, 15, 0, 0, 0, 0, time.Local) // Monday
	day := func(offset int, logged, target time.Duration) domain.WeekDay {
		return domain.WeekDay{
			Date:   mon.AddDate(0, 0, offset),
			Logged: logged,
			Target: target,
		}
	}
	h := func(n int) time.Duration { return time.Duration(n) * time.Hour }

	days := []domain.WeekDay{
		day(0, h(8), h(8)), // Mon hit
		day(1, h(9), h(8)), // Tue hit (over)
		day(2, h(7), h(8)), // Wed miss
		day(3, h(8), h(8)), // Thu hit
		day(4, h(8), h(8)), // Fri hit (but day-off → not counted)
		day(5, h(5), h(8)), // Sat — must be excluded entirely
		day(6, h(5), h(8)), // Sun — must be excluded entirely
	}
	offs := map[string]domain.DayOff{
		mon.AddDate(0, 0, 4).Format("2006-01-02"): {Kind: domain.KindVacation},
	}
	now := mon.AddDate(0, 0, 6).Add(12 * time.Hour) // Sun noon; no today in range

	s := computeWocheSummary(days, offs, now)

	// Logged total = Mon..Fri only = 8+9+7+8+8 = 40h (Sat/Sun's 10h excluded).
	if s.totalLogged != h(40) {
		t.Errorf("totalLogged = %v, want 40h (weekends excluded)", s.totalLogged)
	}
	// Target total = Mon..Fri only = 5×8 = 40h (Sat/Sun's 16h excluded).
	if s.totalTarget != h(40) {
		t.Errorf("totalTarget = %v, want 40h (weekends excluded)", s.totalTarget)
	}
	// Workdays = 4 (Fri is a day-off, not counted; weekends never counted).
	if s.workdays != 4 {
		t.Errorf("workdays = %d, want 4", s.workdays)
	}
	// Hits = Mon, Tue, Thu = 3 (Wed missed; Fri day-off skipped).
	if s.hits != 3 {
		t.Errorf("hits = %d, want 3", s.hits)
	}
}

func TestIsWeekendTime(t *testing.T) {
	cases := []struct {
		date string
		want bool
	}{
		{"2026-06-15", false}, // Mon
		{"2026-06-19", false}, // Fri
		{"2026-06-20", true},  // Sat
		{"2026-06-21", true},  // Sun
	}
	for _, c := range cases {
		d, _ := time.ParseInLocation("2006-01-02", c.date, time.Local)
		if got := isWeekendTime(d); got != c.want {
			t.Errorf("isWeekendTime(%s) = %v, want %v", c.date, got, c.want)
		}
	}
}

// TestPaceDotState covers the four mapped states.
func TestPaceDotState(t *testing.T) {
	mon := time.Date(2026, 6, 15, 0, 0, 0, 0, time.Local)
	now := mon.Add(12 * time.Hour)
	hit := domain.WeekDay{Date: mon, Logged: 8 * time.Hour, Target: 8 * time.Hour}
	miss := domain.WeekDay{Date: mon, Logged: 4 * time.Hour, Target: 8 * time.Hour}
	today := domain.WeekDay{Date: mon, Logged: 2 * time.Hour, Target: 8 * time.Hour, IsToday: true}

	if got := paceDotState(hit, nil, now); got != "ahead" {
		t.Errorf("hit → %q, want ahead", got)
	}
	if got := paceDotState(miss, nil, now); got != "behind" {
		t.Errorf("miss → %q, want behind", got)
	}
	if got := paceDotState(today, nil, now); got != "running" {
		t.Errorf("today → %q, want running", got)
	}
	if got := paceDotState(hit, &domain.DayOff{Kind: domain.KindVacation}, now); got != "off" {
		t.Errorf("dayoff → %q, want off", got)
	}
}
