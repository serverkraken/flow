package domain_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestConfig_TargetForWeekday(t *testing.T) {
	c := domain.Config{
		DefaultTarget: 8 * time.Hour,
		PerWeekday: map[time.Weekday]time.Duration{
			time.Friday: 6 * time.Hour,
		},
	}
	if got := c.TargetForWeekday(time.Monday); got != 8*time.Hour {
		t.Errorf("Monday = %v, want default 8h", got)
	}
	if got := c.TargetForWeekday(time.Friday); got != 6*time.Hour {
		t.Errorf("Friday = %v, want override 6h", got)
	}

	// Empty map falls back to default.
	c2 := domain.Config{DefaultTarget: 4 * time.Hour}
	if got := c2.TargetForWeekday(time.Wednesday); got != 4*time.Hour {
		t.Errorf("nil PerWeekday should fall through, got %v", got)
	}
}

func TestConfig_TagTarget(t *testing.T) {
	c := domain.Config{
		TagTargets: map[string]time.Duration{
			"deep":    4 * time.Hour,
			"Meeting": 2 * time.Hour,
		},
	}

	tests := []struct {
		in   string
		want time.Duration
	}{
		{"deep", 4 * time.Hour},    // exact
		{"Deep", 4 * time.Hour},    // case-insensitive
		{"DEEP", 4 * time.Hour},    // case-insensitive uppercase
		{"meeting", 2 * time.Hour}, // case-insensitive against keyed casing
		{"unknown", 0},             // miss
		{"", 0},                    // empty
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := c.TagTarget(tc.in); got != tc.want {
				t.Errorf("TagTarget(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestConfig_TagTargetNilMap(t *testing.T) {
	c := domain.Config{}
	if got := c.TagTarget("anything"); got != 0 {
		t.Errorf("nil map should return 0, got %v", got)
	}
}
