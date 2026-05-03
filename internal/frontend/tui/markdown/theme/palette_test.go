package theme_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/markdown/theme"
	canonical "github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// TestShortcutShims_TrackCanonicalDefault: the deprecated package-level
// shortcuts (theme.Bg, theme.Blue, …) must mirror canonical.Default at
// init time. P4 will migrate kompendium screens off these shims; until
// then the assertion catches a drift between the two sources of truth
// before it manifests as a colour mismatch on screen.
func TestShortcutShims_TrackCanonicalDefault(t *testing.T) {
	t.Parallel()
	def := canonical.Default
	cases := []struct {
		got, want, name string
	}{
		{theme.Bg, def.Bg, "Bg"},
		{theme.PanelBg, def.BgPanel, "PanelBg → BgPanel"},
		{theme.BgCode, def.BgCode, "BgCode"},
		{theme.BgHighlight, def.BgChip, "BgHighlight → BgChip"},
		{theme.BgHighlightSoft, def.BgChipSoft, "BgHighlightSoft → BgChipSoft"},
		{theme.BarBg, def.BgBar, "BarBg → BgBar"},
		{theme.DangerBg, def.BgDanger, "DangerBg → BgDanger"},
		{theme.SuccessBg, def.BgSuccess, "SuccessBg → BgSuccess"},
		{theme.Fg, def.Fg, "Fg"},
		{theme.FgDim, def.FgDim, "FgDim"},
		{theme.Muted, def.FgMuted, "Muted → FgMuted"},
		{theme.Blue, def.Blue, "Blue"},
		{theme.Cyan, def.Cyan, "Cyan"},
		{theme.Green, def.Green, "Green"},
		{theme.Purple, def.Purple, "Purple"},
		{theme.Magenta, def.Magenta, "Magenta"},
		{theme.Yellow, def.Yellow, "Yellow"},
		{theme.Orange, def.Orange, "Orange"},
		{theme.Red, def.Red, "Red"},
		{theme.Teal, def.Teal, "Teal"},
	}
	for _, tt := range cases {
		if tt.got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, tt.got, tt.want)
		}
	}
	if len(theme.TagPalette) != len(def.TagPalette) {
		t.Errorf("TagPalette length: got %d, want %d", len(theme.TagPalette), len(def.TagPalette))
	}
	for i := range theme.TagPalette {
		if theme.TagPalette[i] != def.TagPalette[i] {
			t.Errorf("TagPalette[%d]: got %q, want %q", i, theme.TagPalette[i], def.TagPalette[i])
		}
	}
}
