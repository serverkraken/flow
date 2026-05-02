package domain_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestParseHM(t *testing.T) {
	tests := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"00:00", 0, false},
		{"08:30", 8*time.Hour + 30*time.Minute, false},
		{"23:59", 23*time.Hour + 59*time.Minute, false},
		{" 09:15 ", 9*time.Hour + 15*time.Minute, false},
		{"0830", 0, true},  // missing colon
		{"ab:cd", 0, true}, // not numeric
		{"08:cd", 0, true}, // minute not numeric
		{"", 0, true},      // empty
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := domain.ParseHM(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseStartArg(t *testing.T) {
	now := time.Date(2026, time.April, 29, 14, 30, 0, 0, time.Local)

	tests := []struct {
		name    string
		arg     string
		want    time.Time
		wantErr bool
	}{
		{"empty returns now", "", now, false},
		{"HH:MM today", "09:00", time.Date(2026, time.April, 29, 9, 0, 0, 0, time.Local), false},
		{"HH:MM equal to now", "14:30", now, false},
		{"HH:MM in future fails", "16:00", time.Time{}, true},
		{"-Nm offset", "-45m", now.Add(-45 * time.Minute), false},
		{"-Nh offset", "-2h", now.Add(-2 * time.Hour), false},
		{"-NhMMm offset", "-1h30m", now.Add(-1*time.Hour - 30*time.Minute), false},
		{"-NhM offset (no trailing m)", "-1h30", now.Add(-1*time.Hour - 30*time.Minute), false},
		{"-Nh with bad minute fails", "-1hxx", time.Time{}, true},
		{"unknown format fails", "tomorrow", time.Time{}, true},
		{"HH:MM with non-numeric falls through to error", "ab:cd", time.Time{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := domain.ParseStartArg(tc.arg, now)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if !tc.wantErr && !got.Equal(tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseStop(t *testing.T) {
	now := time.Date(2026, time.April, 29, 14, 30, 0, 0, time.Local)
	start := time.Date(2026, time.April, 29, 9, 0, 0, 0, time.Local)

	tests := []struct {
		name    string
		arg     string
		want    time.Time
		wantErr bool
	}{
		{"empty returns now", "", now, false},
		{"absolute HH:MM delegates", "12:00", time.Date(2026, time.April, 29, 12, 0, 0, 0, time.Local), false},
		{"+Nh adds to start", "+2h", start.Add(2 * time.Hour), false},
		{"+NhMMm adds to start", "+1h30m", start.Add(1*time.Hour + 30*time.Minute), false},
		{"+90m adds minutes to start", "+90m", start.Add(90 * time.Minute), false},
		{"+45 (bare minutes) adds to start", "+45", start.Add(45 * time.Minute), false},
		{"+0 is rejected (non-positive)", "+0", time.Time{}, true},
		{"+ with garbage fails", "+xx", time.Time{}, true},
		{"+ alone fails (empty duration)", "+", time.Time{}, true},
		{"+ with bad hours fails", "+xh30m", time.Time{}, true},
		{"+ with bad minutes fails", "+1hxx", time.Time{}, true},
		{"-Nm delegates to ParseStartArg", "-15m", now.Add(-15 * time.Minute), false},
		{"unknown delegates and errors", "garbage", time.Time{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := domain.ParseStop(tc.arg, start, now)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if !tc.wantErr && !got.Equal(tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseRange(t *testing.T) {
	now := time.Date(2026, time.April, 29, 14, 30, 0, 0, time.Local) // Wednesday
	loc := time.Local

	day := func(y, m, d int) time.Time {
		return time.Date(y, time.Month(m), d, 0, 0, 0, 0, loc)
	}

	tests := []struct {
		name     string
		expr     string
		wantFrom time.Time
		wantTo   time.Time
		wantErr  bool
	}{
		{"empty is unbounded", "", time.Time{}, time.Time{}, false},
		{"today", "today", day(2026, 4, 29), day(2026, 4, 30), false},
		{"week starts on Monday", "week", day(2026, 4, 27), day(2026, 5, 4), false},
		{"month", "month", day(2026, 4, 1), day(2026, 5, 1), false},
		{"YYYY", "2026", day(2026, 1, 1), day(2027, 1, 1), false},
		{"YYYY-MM", "2026-04", day(2026, 4, 1), day(2026, 5, 1), false},
		{"YYYY-MM-DD..YYYY-MM-DD", "2026-04-01..2026-04-30", day(2026, 4, 1), day(2026, 5, 1), false},
		{"bad From in range", "garbage..2026-04-30", time.Time{}, time.Time{}, true},
		{"bad To in range", "2026-04-01..garbage", time.Time{}, time.Time{}, true},
		{"bad expression", "yesterday", time.Time{}, time.Time{}, true},
		{"YYYY non-numeric falls through", "abcd", time.Time{}, time.Time{}, true},
		{"YYYY-MM bad month", "2026-13", time.Time{}, time.Time{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := domain.ParseRange(now, tc.expr)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if !got.From.Equal(tc.wantFrom) || !got.To.Equal(tc.wantTo) {
				t.Errorf("got [%v, %v), want [%v, %v)", got.From, got.To, tc.wantFrom, tc.wantTo)
			}
		})
	}
}

// Sunday triggers the wd==0 → wd=7 branch in ParseRange("week").
func TestParseRange_WeekFromSunday(t *testing.T) {
	sunday := time.Date(2026, time.May, 3, 12, 0, 0, 0, time.Local)
	got, err := domain.ParseRange(sunday, "week")
	if err != nil {
		t.Fatal(err)
	}
	wantMon := time.Date(2026, time.April, 27, 0, 0, 0, 0, time.Local)
	if !got.From.Equal(wantMon) {
		t.Errorf("Monday from Sunday: got %v, want %v", got.From, wantMon)
	}
}
