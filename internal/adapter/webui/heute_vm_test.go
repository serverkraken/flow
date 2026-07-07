package webui

import (
	"testing"
	"time"
)

// TestFmtClockShort is the unit-level RED→GREEN guard for Review Fix 1: the
// Zeit-Hub's Wochenskala day values and ledger duration column render in the
// Mockup's colon clock format ("6:10"), never zero-padded on the hour digit
// (unlike format.go's unexported fmtDur, "06:10") and never the codebase-wide
// FmtVerbose ("6h 10m") — that stays untouched everywhere else.
func TestFmtClockShort(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{6*time.Hour + 10*time.Minute, "6:10"},
		{40 * time.Hour, "40:00"},
		{304*time.Hour + 46*time.Minute, "304:46"},
		{0, "0:00"},
		{-time.Minute, "0:00"},
		{5 * time.Minute, "0:05"},
	}
	for _, tc := range cases {
		if got := FmtClockShort(tc.d); got != tc.want {
			t.Errorf("FmtClockShort(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}
