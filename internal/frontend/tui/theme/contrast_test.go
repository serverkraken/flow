package theme_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// TestPalettesPassWCAG_AA enforces A11y-1 from docs/design-system-audit.md:
// a palette doesn't ship unless every text/glyph-on-surface pair meets the
// 4.5:1 normal-text threshold (or 3:1 for cells used as glyph-only signals).
//
// Fail mode: the offending pair is named so the regen-fix is obvious
// ("FgMuted on Bg = 2.43, want ≥ 4.5" → bump FgMuted toward Fg).
func TestPalettesPassWCAG_AA(t *testing.T) {
	for _, p := range []theme.Palette{theme.TokyonightNight, theme.CatppuccinMocha} {
		t.Run(p.Name, func(t *testing.T) {
			sem := p.Sem()

			// Text-on-surface — full AA (4.5:1).
			textPairs := []struct{ fg, bg, name string }{
				{p.Fg, p.Bg, "Fg on Bg"},
				{p.FgDim, p.Bg, "FgDim on Bg"},
				{p.FgMuted, p.Bg, "FgMuted on Bg"},
				{p.Fg, p.BgPanel, "Fg on BgPanel"},
				{p.FgDim, p.BgPanel, "FgDim on BgPanel"},
				{p.Fg, p.BgChip, "Fg on BgChip (selected row)"},
				{p.Fg, p.BgChipSoft, "Fg on BgChipSoft"},
				{sem.Accent, p.Bg, "Accent on Bg"},
				{sem.Success, p.Bg, "Success on Bg"},
				{sem.Warning, p.Bg, "Warning on Bg"},
				{sem.Danger, p.Bg, "Danger on Bg"},
				{sem.Info, p.Bg, "Info on Bg"},
				{sem.Highlight, p.Bg, "Highlight on Bg"},
				{sem.Active, p.Bg, "Active on Bg"},
			}
			for _, tt := range textPairs {
				t.Run(tt.name, func(t *testing.T) {
					r, err := theme.ContrastRatio(tt.fg, tt.bg)
					if err != nil {
						t.Fatalf("contrast(%q, %q): %v", tt.fg, tt.bg, err)
					}
					if r < theme.WCAGNormalAA {
						t.Errorf("%s: %.2f, want ≥ %.2f (fg=%s bg=%s)",
							tt.name, r, theme.WCAGNormalAA, tt.fg, tt.bg)
					}
				})
			}

			// Bg-as-fg on accent fills (pill, callout) — also AA, since
			// the pill label is body-sized.
			pillPairs := []struct{ fg, bg, name string }{
				{p.Bg, sem.Success, "Bg on Success (pill)"},
				{p.Bg, sem.Warning, "Bg on Warning (pill)"},
				{p.Bg, sem.Danger, "Bg on Danger (pill)"},
				{p.Bg, sem.Info, "Bg on Info (pill)"},
				{p.Bg, sem.Accent, "Bg on Accent (pill)"},
			}
			for _, tt := range pillPairs {
				t.Run(tt.name, func(t *testing.T) {
					r, err := theme.ContrastRatio(tt.fg, tt.bg)
					if err != nil {
						t.Fatalf("contrast(%q, %q): %v", tt.fg, tt.bg, err)
					}
					if r < theme.WCAGNormalAA {
						t.Errorf("%s: %.2f, want ≥ %.2f (fg=%s bg=%s)",
							tt.name, r, theme.WCAGNormalAA, tt.fg, tt.bg)
					}
				})
			}
		})
	}
}

// TestContrastRatio_KnownValues anchors the formula to a few WCAG-published
// reference pairs. If a refactor breaks the math, this test catches it
// before the palette test starts producing wrong-looking numbers.
func TestContrastRatio_KnownValues(t *testing.T) {
	cases := []struct {
		a, b string
		want float64 // WCAG-published reference, ±0.1
	}{
		{"#000000", "#ffffff", 21.00}, // pure black on white
		{"#777777", "#ffffff", 4.48},  // grey 50%
		{"#ffffff", "#000000", 21.00}, // symmetric
	}
	for _, tt := range cases {
		got, err := theme.ContrastRatio(tt.a, tt.b)
		if err != nil {
			t.Fatalf("contrast(%q,%q): %v", tt.a, tt.b, err)
		}
		if diff := got - tt.want; diff > 0.1 || diff < -0.1 {
			t.Errorf("contrast(%q,%q) = %.3f, want %.2f ± 0.1", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestContrastRatio_BadHex(t *testing.T) {
	if _, err := theme.ContrastRatio("not-a-color", "#ffffff"); err == nil {
		t.Error("expected error for invalid hex, got nil")
	}
	if _, err := theme.ContrastRatio("#fff", "#000000"); err == nil {
		t.Error("expected error for short hex, got nil")
	}
}
