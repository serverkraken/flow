package webui

import (
	"strings"
	"testing"
)

// TestWocheDayDotClass covers all branches of wocheDayDotClass:
// Weekend, running, hit/over, and default (under/unknown).
func TestWocheDayDotClass(t *testing.T) {
	cases := []struct {
		name    string
		vm      WocheDayVM
		wantSub string // must contain this substring
	}{
		{"weekend", WocheDayVM{Weekend: true}, "text-faint"},
		{"running", WocheDayVM{Variant: "running"}, "text-cyan"},
		{"hit", WocheDayVM{Variant: "hit"}, "text-green"},
		{"over", WocheDayVM{Variant: "over"}, "text-green"},
		{"under", WocheDayVM{Variant: "under"}, "text-faint"},
	}
	for _, tc := range cases {
		got := wocheDayDotClass(tc.vm)
		if !strings.Contains(got, tc.wantSub) {
			t.Errorf("%s: wocheDayDotClass = %q, want to contain %q", tc.name, got, tc.wantSub)
		}
	}
}

// TestWocheLabelClass covers all branches of wocheLabelClass:
// IsToday, Weekend, and the default.
func TestWocheLabelClass(t *testing.T) {
	cases := []struct {
		name    string
		vm      WocheDayVM
		wantSub string
	}{
		{"today", WocheDayVM{IsToday: true}, "text-blue"},
		{"weekend", WocheDayVM{Weekend: true}, "text-muted"},
		{"normal", WocheDayVM{}, "font-semibold"},
	}
	for _, tc := range cases {
		got := wocheLabelClass(tc.vm)
		if !strings.Contains(got, tc.wantSub) {
			t.Errorf("%s: wocheLabelClass = %q, want to contain %q", tc.name, got, tc.wantSub)
		}
	}
}
