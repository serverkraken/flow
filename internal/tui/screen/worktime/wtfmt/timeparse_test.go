package wtfmt_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtfmt"
)

func TestParseHM(t *testing.T) {
	got, err := wtfmt.ParseHM("09:30")
	if err != nil {
		t.Fatalf("ParseHM: %v", err)
	}
	if got != 9*time.Hour+30*time.Minute {
		t.Fatalf("ParseHM = %v, want 9h30m", got)
	}
	if _, err := wtfmt.ParseHM("nope"); err == nil {
		t.Fatal("ParseHM(nope) should error")
	}
}

func TestParseStop(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	start := day.Add(9 * time.Hour)
	now := day.Add(23 * time.Hour)
	// absolute HH:MM
	got, err := wtfmt.ParseStop("12:00", start, now)
	if err != nil {
		t.Fatalf("ParseStop abs: %v", err)
	}
	if got.Hour() != 12 || got.Day() != 18 {
		t.Fatalf("ParseStop abs = %v, want 12:00 on the start's day", got)
	}
	// relative +1h30m off the start
	got2, err := wtfmt.ParseStop("+1h30m", start, now)
	if err != nil {
		t.Fatalf("ParseStop rel: %v", err)
	}
	if !got2.Equal(start.Add(90 * time.Minute)) {
		t.Fatalf("ParseStop rel = %v, want start+1h30m", got2)
	}
}

func TestNormalizeDurationArg(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"+1h30m", "1h30m"},
		{"1h30m", "1h30m"},
		{" +2h ", "2h"},
		{"12:00", "12:00"},
	}
	for _, c := range cases {
		if got := wtfmt.NormalizeDurationArg(c.in); got != c.want {
			t.Errorf("NormalizeDurationArg(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
