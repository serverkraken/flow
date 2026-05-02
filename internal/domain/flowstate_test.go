package domain_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestIsValidScreen(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{domain.ScreenPalette, true},
		{domain.ScreenProjects, true},
		{domain.ScreenWorktime, true},
		{domain.ScreenCheatsheet, true},
		{"", false},
		{"unknown", false},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := domain.IsValidScreen(tc.in); got != tc.want {
				t.Errorf("IsValidScreen(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestDefaultFlowState(t *testing.T) {
	if got := domain.DefaultFlowState(); got.Screen != domain.ScreenPalette {
		t.Errorf("default screen = %q, want %q", got.Screen, domain.ScreenPalette)
	}
}
