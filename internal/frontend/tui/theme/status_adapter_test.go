package theme_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func TestStatusPaletteFor_TokyonightNight(t *testing.T) {
	p := theme.TokyonightNight
	got := theme.StatusPaletteFor(p)

	cases := []struct {
		got, want, name string
	}{
		{got.Green, string(p.Green), "Green"},
		{got.Yellow, string(p.Yellow), "Yellow"},
		{got.Red, string(p.Red), "Red"},
		{got.Cyan, string(p.Cyan), "Cyan"},
		{got.Dim, string(p.FgMuted), "Dim → FgMuted"},
	}
	for _, tt := range cases {
		if tt.got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}
