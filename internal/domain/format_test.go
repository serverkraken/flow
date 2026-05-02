package domain_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestFmtDuration(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{0, "0h 00m"},
		{45 * time.Minute, "0h 45m"},
		{time.Hour, "1h 00m"},
		{2*time.Hour + 15*time.Minute, "2h 15m"},
		{8 * time.Hour, "8h 00m"},
		{-time.Hour, "0h 00m"}, // negative clamped
	}
	for _, tc := range tests {
		t.Run(tc.in.String(), func(t *testing.T) {
			if got := domain.FmtDuration(tc.in); got != tc.want {
				t.Errorf("FmtDuration(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFmtSignedDuration(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{0, "+0h 00m"},
		{30 * time.Minute, "+0h 30m"},
		{2*time.Hour + 5*time.Minute, "+2h 05m"},
		{-90 * time.Minute, "-1h 30m"},
		{-time.Hour, "-1h 00m"},
	}
	for _, tc := range tests {
		t.Run(tc.in.String(), func(t *testing.T) {
			if got := domain.FmtSignedDuration(tc.in); got != tc.want {
				t.Errorf("FmtSignedDuration(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestWeekdayShortDe(t *testing.T) {
	want := map[time.Weekday]string{
		time.Sunday:    "So",
		time.Monday:    "Mo",
		time.Tuesday:   "Di",
		time.Wednesday: "Mi",
		time.Thursday:  "Do",
		time.Friday:    "Fr",
		time.Saturday:  "Sa",
	}
	for wd, w := range want {
		if got := domain.WeekdayShortDe(wd); got != w {
			t.Errorf("WeekdayShortDe(%v) = %q, want %q", wd, got, w)
		}
	}
}

func TestMonthShortDe(t *testing.T) {
	want := []string{"", "Jan", "Feb", "Mär", "Apr", "Mai", "Jun", "Jul", "Aug", "Sep", "Okt", "Nov", "Dez"}
	for m := 0; m < 13; m++ {
		if got := domain.MonthShortDe(time.Month(m)); got != want[m] {
			t.Errorf("MonthShortDe(%d) = %q, want %q", m, got, want[m])
		}
	}
}

func TestIcalEscape(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "Foo bar", "Foo bar"},
		{"backslash", `a\b`, `a\\b`},
		{"semicolon", "a;b", `a\;b`},
		{"comma", "a,b", `a\,b`},
		{"newline", "a\nb", `a\nb`},
		{"carriage return dropped", "a\rb", "ab"},
		{"all together", "a\\;,\nb\r", `a\\\;\,\nb`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := domain.IcalEscape(tc.in); got != tc.want {
				t.Errorf("IcalEscape(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestDailyNoteID(t *testing.T) {
	d := time.Date(2026, time.April, 30, 12, 0, 0, 0, time.Local)
	if got := domain.DailyNoteID(d); got != "daily/2026-04-30" {
		t.Errorf("DailyNoteID = %q", got)
	}
}

func TestHumanizeNoteID(t *testing.T) {
	tests := []struct{ in, want string }{
		{"daily/2026-04-30", "Daily 2026-04-30"},
		{"projects/serverkraken/flow", "Projekt serverkraken/flow"},
		{"notes/scratchpad", "Notiz scratchpad"},
		{"someotherthing", "someotherthing"}, // unknown prefix → passthrough
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := domain.HumanizeNoteID(tc.in); got != tc.want {
				t.Errorf("HumanizeNoteID(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
