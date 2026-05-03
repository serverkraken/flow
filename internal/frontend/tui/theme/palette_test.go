package theme_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func TestThemesRegistry_KeysMatchNames(t *testing.T) {
	if len(theme.Themes) == 0 {
		t.Fatal("Themes registry is empty")
	}
	for key, p := range theme.Themes {
		if key != p.Name {
			t.Errorf("Themes[%q].Name = %q, want %q", key, p.Name, key)
		}
	}
}

func TestDefaultIsTokyonightNight(t *testing.T) {
	if theme.Default.Name != theme.TokyonightNight.Name {
		t.Errorf("Default.Name = %q, want %q", theme.Default.Name, theme.TokyonightNight.Name)
	}
}

func TestSem_MapsToHues(t *testing.T) {
	p := theme.TokyonightNight
	sem := p.Sem()
	cases := []struct {
		got, want, name string
	}{
		{sem.Accent, p.Blue, "Accent → Blue"},
		{sem.Active, p.Cyan, "Active → Cyan"},
		{sem.Success, p.Green, "Success → Green"},
		{sem.Warning, p.Yellow, "Warning → Yellow"},
		{sem.Danger, p.Red, "Danger → Red"},
		{sem.Info, p.Cyan, "Info → Cyan"},
		{sem.Highlight, p.Purple, "Highlight → Purple"},
		{sem.BorderSubtle, p.BgChip, "BorderSubtle → BgChip"},
		{sem.BorderStrong, p.FgMuted, "BorderStrong → FgMuted"},
	}
	for _, tt := range cases {
		if tt.got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestPalette_NoEmptyFields(t *testing.T) {
	for _, p := range []theme.Palette{theme.TokyonightNight, theme.CatppuccinMocha} {
		t.Run(p.Name, func(t *testing.T) {
			fields := map[string]string{
				"Bg": p.Bg, "BgPanel": p.BgPanel, "BgCode": p.BgCode,
				"BgChip": p.BgChip, "BgChipSoft": p.BgChipSoft, "BgBar": p.BgBar,
				"BgDanger": p.BgDanger, "BgSuccess": p.BgSuccess,
				"Fg": p.Fg, "FgDim": p.FgDim, "FgMuted": p.FgMuted,
				"Blue": p.Blue, "Cyan": p.Cyan, "Green": p.Green,
				"Purple": p.Purple, "Magenta": p.Magenta, "Yellow": p.Yellow,
				"Orange": p.Orange, "Red": p.Red, "Teal": p.Teal,
			}
			for name, v := range fields {
				if v == "" {
					t.Errorf("%s.%s is empty", p.Name, name)
				}
			}
			if len(p.TagPalette) == 0 {
				t.Errorf("%s.TagPalette is empty", p.Name)
			}
		})
	}
}
