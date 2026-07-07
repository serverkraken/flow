package webui

import (
	"strings"
	"testing"
)

// TestWocheDayOffTypeChip covers all branches of wocheDayOffTypeChip: each
// known hue's fixed tc-* token (Farb-Gesetz §7 — semantic, not per-project),
// the yellow/orange tc-o overlap (only 6 tc- tokens exist for 7 day-off
// hues), and the unknown-hue fallback.
func TestWocheDayOffTypeChip(t *testing.T) {
	cases := []struct {
		hue  string
		want string
	}{
		{"purple", "tc-v"},
		{"orange", "tc-o"},
		{"yellow", "tc-o"},
		{"green", "tc-g"},
		{"red", "tc-r"},
		{"cyan", "tc-t"},
		{"blue", "tc-b"},
		{"unknown-hue", "tc-b"},
	}
	for _, tc := range cases {
		if got := wocheDayOffTypeChip(tc.hue); got != tc.want {
			t.Errorf("wocheDayOffTypeChip(%q) = %q, want %q", tc.hue, got, tc.want)
		}
	}
}

// TestWocheVariantHue covers all branches of wocheVariantHue: hit/over share
// the "hit" hue, running gets its own, and under/weekend/unknown fall back to
// the neutral muted hue.
func TestWocheVariantHue(t *testing.T) {
	cases := []struct {
		variant string
		wantSub string
	}{
		{"hit", "text-green"},
		{"over", "text-green"},
		{"running", "text-cyan"},
		{"under", "text-muted"},
		{"weekend", "text-muted"},
		{"", "text-muted"},
	}
	for _, tc := range cases {
		if got := wocheVariantHue(tc.variant); !strings.Contains(got, tc.wantSub) {
			t.Errorf("wocheVariantHue(%q) = %q, want to contain %q", tc.variant, got, tc.wantSub)
		}
	}
}

// TestWocheStatsHue covers both branches of wocheStatsHue: on-track green,
// behind orange.
func TestWocheStatsHue(t *testing.T) {
	if got := wocheStatsHue(true); got != "text-green" {
		t.Errorf("wocheStatsHue(true) = %q, want text-green", got)
	}
	if got := wocheStatsHue(false); got != "text-orange" {
		t.Errorf("wocheStatsHue(false) = %q, want text-orange", got)
	}
}
