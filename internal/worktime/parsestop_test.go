package worktime_test

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/worktime"
)

func TestParseStop_PlusDuration(t *testing.T) {
	start := time.Date(2026, 4, 30, 9, 0, 0, 0, time.Local)
	cases := map[string]time.Duration{
		"+1h":      time.Hour,
		"+1h30m":   time.Hour + 30*time.Minute,
		"+90m":     90 * time.Minute,
		"+45m":     45 * time.Minute,
		"+45":      45 * time.Minute,
		"+2h":      2 * time.Hour,
		"+2h0m":    2 * time.Hour,
		"+0h30m":   30 * time.Minute,
	}
	for input, want := range cases {
		got, err := worktime.ParseStop(input, start)
		if err != nil {
			t.Errorf("ParseStop(%q): %v", input, err)
			continue
		}
		if got.Sub(start) != want {
			t.Errorf("ParseStop(%q) = %v (Δ %v), want Δ %v",
				input, got, got.Sub(start), want)
		}
	}
}

func TestParseStop_AbsoluteAndRelativeFallback(t *testing.T) {
	start := time.Date(2026, 4, 30, 9, 0, 0, 0, time.Local)

	// "" → now
	now := time.Now()
	got, err := worktime.ParseStop("", start)
	if err != nil {
		t.Fatal(err)
	}
	if d := got.Sub(now); d < -time.Second || d > time.Second {
		t.Errorf("empty stop: %v not within 1s of now (%v)", got, now)
	}
}

func TestParseStop_RejectsZeroOrNegative(t *testing.T) {
	start := time.Now()
	for _, input := range []string{"+0", "+0h", "+0m", "+0h0m"} {
		if _, err := worktime.ParseStop(input, start); err == nil {
			t.Errorf("ParseStop(%q) expected error, got nil", input)
		} else if !strings.Contains(err.Error(), "positiv") {
			t.Logf("ParseStop(%q) returned err: %v", input, err)
		}
	}
}

func TestParseStop_GarbageReturnsError(t *testing.T) {
	start := time.Now()
	for _, input := range []string{"+abc", "+h30m", "+1z"} {
		if _, err := worktime.ParseStop(input, start); err == nil {
			t.Errorf("ParseStop(%q) expected error, got nil", input)
		}
	}
}
