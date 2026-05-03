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
		{got.Green, p.Green, "Green"},
		{got.Yellow, p.Yellow, "Yellow"},
		{got.Red, p.Red, "Red"},
		{got.Cyan, p.Cyan, "Cyan"},
		{got.Dim, p.FgMuted, "Dim → FgMuted"},
	}
	for _, tt := range cases {
		if tt.got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}
