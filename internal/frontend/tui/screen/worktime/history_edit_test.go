package worktime

import (
	"testing"
	"time"
)

// TestParseDrillStop_HHMMOnPastDay: HH:MM stop on a past-day edit
// must NOT fall through ParseStop's now-anchored "Zeit liegt in der
// Zukunft" guard. The drill view always knows the date — anchor on
// it, not on time.Now().
func TestParseDrillStop_HHMMOnPastDay(t *testing.T) {
	t.Parallel()
	yesterday := time.Date(2026, 5, 4, 0, 0, 0, 0, time.Local)
	start := yesterday.Add(8 * time.Hour)
	got, err := parseDrillStop("23:30", start, yesterday)
	if err != nil {
		t.Fatalf("parseDrillStop(23:30) on past day: %v", err)
	}
	want := yesterday.Add(23*time.Hour + 30*time.Minute)
	if !got.Equal(want) {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestParseDrillStop_DurationCaseInsensitive: `+8H02M` must parse the
// same as `+8h02m` — the user reported the uppercase form failing
// with "dauer: ungültig: 8H02M". domain.parseHumanDuration is
// strict-lower; the drill wraps it in strings.ToLower for input.
func TestParseDrillStop_DurationCaseInsensitive(t *testing.T) {
	t.Parallel()
	day := time.Date(2026, 5, 4, 0, 0, 0, 0, time.Local)
	start := day.Add(8 * time.Hour)
	cases := []string{"+8h02m", "+8H02M", "+8H02m", "+1H30M"}
	want := []time.Duration{
		8*time.Hour + 2*time.Minute,
		8*time.Hour + 2*time.Minute,
		8*time.Hour + 2*time.Minute,
		1*time.Hour + 30*time.Minute,
	}
	for i, in := range cases {
		got, err := parseDrillStop(in, start, day)
		if err != nil {
			t.Errorf("parseDrillStop(%q): %v", in, err)
			continue
		}
		if got.Sub(start) != want[i] {
			t.Errorf("parseDrillStop(%q) = %s (Δ=%s), want Δ=%s",
				in, got, got.Sub(start), want[i])
		}
	}
}

// TestParseDrillStop_EmptyRejected: empty stop input is an error
// (cannot mean "now" in the drill — past day editing requires an
// explicit stop).
func TestParseDrillStop_EmptyRejected(t *testing.T) {
	t.Parallel()
	day := time.Date(2026, 5, 4, 0, 0, 0, 0, time.Local)
	if _, err := parseDrillStop("", day, day); err == nil {
		t.Errorf("empty stop should error")
	}
}
