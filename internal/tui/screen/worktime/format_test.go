package worktime

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/timefmt"
)

func TestFormatDur(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0h 00m"},
		{-time.Hour, "0h 00m"},
		{90 * time.Minute, "1h 30m"},
		{2*time.Hour + 5*time.Minute, "2h 05m"},
	}
	for _, c := range cases {
		if got := formatDur(c.d); got != c.want {
			t.Errorf("formatDur(%v) = %q, want %q", c.d, got, c.want)
		}
	}
	if got := formatDurLive(2*time.Hour + 5*time.Minute + 9*time.Second); got != "2h 05m 09s" {
		t.Errorf("formatDurLive = %q", got)
	}
}

func TestPctOfTarget(t *testing.T) {
	if got := pctOfTarget(time.Hour, 2*time.Hour); got != 50 {
		t.Errorf("50%% = %d", got)
	}
	if got := pctOfTarget(3*time.Hour, 2*time.Hour); got != 100 {
		t.Errorf("clamp = %d", got)
	}
	if got := pctOfTarget(time.Hour, 0); got != 0 {
		t.Errorf("zero target = %d", got)
	}
}

func TestParseHM(t *testing.T) {
	d, err := timefmt.ParseHM("09:30")
	if err != nil || d != 9*time.Hour+30*time.Minute {
		t.Fatalf("ParseHM(09:30) = %v, %v", d, err)
	}
	if _, err := timefmt.ParseHM("nonsense"); err == nil {
		t.Fatal("ParseHM(nonsense) should error")
	}
}

func TestParseStopRelativeAndAbsolute(t *testing.T) {
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	now := time.Date(2026, 6, 14, 18, 0, 0, 0, time.UTC)
	got, err := timefmt.ParseStop(timefmt.NormalizeDurationArg("+1h30m"), start, now)
	if err != nil || !got.Equal(start.Add(90*time.Minute)) {
		t.Fatalf("ParseStop(+1h30m) = %v, %v", got, err)
	}
}

func TestFmtDateDe(t *testing.T) {
	d := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC) // a Sunday
	if got := fmtDateDe(d); got != "So · 14.06.2026" {
		t.Errorf("fmtDateDe = %q", got)
	}
}
