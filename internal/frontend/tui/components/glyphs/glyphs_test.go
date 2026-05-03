package glyphs_test

import (
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
)

// TestAllGlyphsSingleWidth: every whitelisted glyph must occupy
// exactly one terminal cell. A glyph rendered at emoji-width (some
// emoji-presentation pictographs, for instance) would push the next
// cell out of column and break tmux status segments and nvim
// sidebars. ansi.StringWidth uses charmbracelet/x/cellbuf's wcwidth
// implementation — same one tmux uses — so what passes here also
// passes in tmux.
func TestAllGlyphsSingleWidth(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"Active": glyphs.Active, "Stopped": glyphs.Stopped, "Paused": glyphs.Paused,
		"Done": glyphs.Done, "Failed": glyphs.Failed,
		"Up": glyphs.Up, "Down": glyphs.Down,
		"Filled": glyphs.Filled, "Empty": glyphs.Empty,
		"Holiday": glyphs.Holiday, "Vacation": glyphs.Vacation, "Extra": glyphs.Extra,
		"AccentBar": glyphs.AccentBar,
		"BarFilled": glyphs.BarFilled, "BarEmpty": glyphs.BarEmpty,
		"BoxRoundedTL": glyphs.BoxRoundedTL, "BoxRoundedTR": glyphs.BoxRoundedTR,
		"BoxRoundedBL": glyphs.BoxRoundedBL, "BoxRoundedBR": glyphs.BoxRoundedBR,
		"BoxNormalTL": glyphs.BoxNormalTL, "BoxNormalTR": glyphs.BoxNormalTR,
		"BoxNormalBL": glyphs.BoxNormalBL, "BoxNormalBR": glyphs.BoxNormalBR,
		"BoxDoubleTL": glyphs.BoxDoubleTL, "BoxDoubleTR": glyphs.BoxDoubleTR,
		"BoxDoubleBL": glyphs.BoxDoubleBL, "BoxDoubleBR": glyphs.BoxDoubleBR,
		"BoxHorizontal": glyphs.BoxHorizontal, "BoxVertical": glyphs.BoxVertical,
		"BoxHorizontalDouble": glyphs.BoxHorizontalDouble, "BoxVerticalDouble": glyphs.BoxVerticalDouble,
	}
	for name, g := range cases {
		t.Run(name, func(t *testing.T) {
			if g == "" {
				t.Fatalf("%s is empty", name)
			}
			if utf8.RuneCountInString(g) != 1 {
				t.Errorf("%s = %q is not exactly one rune (was %d)",
					name, g, utf8.RuneCountInString(g))
			}
			if w := ansi.StringWidth(g); w != 1 {
				t.Errorf("%s = %q has cell-width %d, want 1", name, g, w)
			}
		})
	}
}
