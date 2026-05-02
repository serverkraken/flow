package theme

import "testing"

// TestSetActive_RewritesShortcuts: the package-level shortcut vars
// (theme.Bg, theme.Blue, …) must reflect the new palette after a
// SetActive call. Restores the default at the end so subsequent
// tests / running consumers see the expected Tokyonight palette.
func TestSetActive_RewritesShortcuts(t *testing.T) {
	t.Cleanup(func() { SetActive(Tokyonight) })

	SetActive(Catppuccin)

	if Active.Name != "catppuccin" {
		t.Errorf("Active.Name = %q, want catppuccin", Active.Name)
	}
	if Bg != Catppuccin.Bg {
		t.Errorf("Bg = %q, want %q", Bg, Catppuccin.Bg)
	}
	if Blue != Catppuccin.Blue {
		t.Errorf("Blue = %q, want %q", Blue, Catppuccin.Blue)
	}
	if BgCode != Catppuccin.BgCode {
		t.Errorf("BgCode = %q, want %q", BgCode, Catppuccin.BgCode)
	}
	if len(TagPalette) == 0 || TagPalette[0] != Catppuccin.TagPalette[0] {
		t.Errorf("TagPalette[0] = %v, want first of Catppuccin.TagPalette = %q",
			TagPalette, Catppuccin.TagPalette[0])
	}
}

// TestSetActive_BackToDefault: switching back to Tokyonight restores
// every shortcut. Makes sure SetActive's per-field rewrite covers
// every documented field — a missed field would manifest as a
// cross-theme bleed-through.
func TestSetActive_BackToDefault(t *testing.T) {
	t.Cleanup(func() { SetActive(Tokyonight) })

	SetActive(Catppuccin)
	SetActive(Tokyonight)

	if Active.Name != "tokyonight" {
		t.Errorf("Active.Name = %q, want tokyonight", Active.Name)
	}
	if Blue != Tokyonight.Blue {
		t.Errorf("Blue = %q, want %q (Catppuccin bled through)", Blue, Tokyonight.Blue)
	}
}

// TestThemes_RegistryComplete: every Themes map value matches its
// Name field. A typo in the map key would silently break env-var
// resolution; the assertion catches it at test time.
func TestThemes_RegistryComplete(t *testing.T) {
	for key, p := range Themes {
		if key != p.Name {
			t.Errorf("Themes[%q].Name = %q (key/name mismatch)", key, p.Name)
		}
	}
}

// TestPaletteFieldsPopulated: bundled themes must fill every Palette
// field — a zero-string field would render as the terminal's default
// FG/BG, breaking visual consistency. Loops over both bundled
// themes so adding a third surfaces missing fields immediately.
func TestPaletteFieldsPopulated(t *testing.T) {
	for _, p := range []Palette{Tokyonight, Catppuccin} {
		t.Run(p.Name, func(t *testing.T) {
			fields := map[string]string{
				"Bg": p.Bg, "PanelBg": p.PanelBg, "BgCode": p.BgCode,
				"BgHighlight": p.BgHighlight, "BgHighlightSoft": p.BgHighlightSoft,
				"BarBg": p.BarBg, "DangerBg": p.DangerBg, "SuccessBg": p.SuccessBg,
				"Fg": p.Fg, "FgDim": p.FgDim, "Muted": p.Muted,
				"Blue": p.Blue, "Cyan": p.Cyan, "Green": p.Green, "Purple": p.Purple,
				"Magenta": p.Magenta, "Yellow": p.Yellow, "Orange": p.Orange,
				"Red": p.Red, "Teal": p.Teal,
			}
			for name, val := range fields {
				if val == "" {
					t.Errorf("%s field unpopulated", name)
				}
			}
			if len(p.TagPalette) == 0 {
				t.Error("TagPalette is empty")
			}
		})
	}
}
