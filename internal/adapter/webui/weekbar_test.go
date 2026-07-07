package webui_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

// TestBuildWeekBars_ScaleAndLabels is the unit-level RED→GREEN guard for the
// pure builder feeding the vertical Wochenskala — shared by both Zeit
// (heuteDataFor) and Woche (wocheDataFor, L4 Final-Review Finding 1) so their
// weekbar skylines can never drift apart again: bar height is proportional
// to the week's own max logged (never a NaN/divide-by-zero on a zero-target
// weekend day), a zero-logged workday shows "—", a zero-logged day covered
// by a day-off (or a weekend with nothing logged) shows "frei", and today's
// label carries the "· heute" suffix.
func TestBuildWeekBars_ScaleAndLabels(t *testing.T) {
	now := time.Date(2026, 6, 18, 15, 0, 0, 0, time.Local) // Thursday
	mon := time.Date(2026, 6, 15, 0, 0, 0, 0, time.Local)
	today := time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	week := make([]domain.WeekDay, 7)
	for i := range week {
		d := mon.AddDate(0, 0, i)
		week[i] = domain.WeekDay{Date: d, Target: 8 * time.Hour, IsToday: d.Equal(today)}
	}
	// Monday: 4h logged (half of the 8h target/scale) → has, ~50%.
	week[0].Logged = 4 * time.Hour
	// Tuesday: nothing logged, no day-off, a workday → "—".
	week[1].Logged = 0
	// Wednesday: nothing logged but covered by a day-off → "frei".
	week[2].Logged = 0
	// Thursday (today): 8h exactly on target/scale → 100%.
	week[3].Logged = 8 * time.Hour
	// Sat/Sun: zero target (weekend), nothing logged → "frei".
	week[5].Target, week[6].Target = 0, 0

	off := map[string]domain.DayOff{
		week[2].Date.Format("2006-01-02"): {Date: week[2].Date, Kind: domain.KindVacation},
	}

	days := webui.BuildWeekBars(context.Background(), week, now, off)
	if len(days) != 7 {
		t.Fatalf("want 7 days, got %d", len(days))
	}
	if !days[0].Has || days[0].Pct <= 0 || days[0].Pct >= 100 {
		t.Errorf("Monday (4h/8h): want Has + partial bar, got %+v", days[0])
	}
	if days[1].ValueStr != "—" {
		t.Errorf("Tuesday (0 logged, no day-off): want %q, got %q", "—", days[1].ValueStr)
	}
	if days[2].ValueStr != "frei" {
		t.Errorf("Wednesday (0 logged, day-off): want %q, got %q", "frei", days[2].ValueStr)
	}
	if !days[3].Today || !days[3].Has || days[3].Pct != 100 {
		t.Errorf("Thursday (today, 8h/8h): want Today+Has+100%%, got %+v", days[3])
	}
	if !strings.Contains(days[3].Label, "· heute") {
		t.Errorf("today's label missing '· heute' suffix, got %q", days[3].Label)
	}
	if days[5].ValueStr != "frei" || days[6].ValueStr != "frei" {
		t.Errorf("weekend with zero target/logged: want %q, got Sa=%q So=%q", "frei", days[5].ValueStr, days[6].ValueStr)
	}
}

// TestBuildWeekBars_ClockShortFormat verifies Finding 1's format fix: a
// Saturday with logged hours and Target==0 must produce a positive bar
// (never the WocheDayVM.Pct division-by-zero collapse the old Woche weekbar
// hit) and its ValueStr must be the "H:MM" clock format (FmtClockShort),
// matching the Zeit-Hub — not the "6h 10m" FmtVerbose prose format the Woche
// detail rows keep.
func TestBuildWeekBars_ClockShortFormat(t *testing.T) {
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.Local) // Saturday
	sat := time.Date(2026, 6, 20, 0, 0, 0, 0, time.Local)
	week := []domain.WeekDay{
		{Date: sat, Target: 0, Logged: 6*time.Hour + 10*time.Minute, IsToday: true},
	}
	days := webui.BuildWeekBars(context.Background(), week, now, map[string]domain.DayOff{})
	if len(days) != 1 {
		t.Fatalf("want 1 day, got %d", len(days))
	}
	if !days[0].Has || days[0].Pct <= 0 {
		t.Errorf("Saturday with logged hours + Target 0: want Has + Pct > 0, got %+v", days[0])
	}
	if days[0].ValueStr != "6:10" {
		t.Errorf("ValueStr = %q, want %q (H:MM, FmtClockShort)", days[0].ValueStr, "6:10")
	}
}
