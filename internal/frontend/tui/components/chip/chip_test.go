package chip_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/components/chip"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func TestRender_Solid(t *testing.T) {
	t.Parallel()
	out := chip.Render(chip.Opts{Label: "go", Color: "#9ece6a"}, theme.TokyonightNight)
	if !strings.Contains(out, "go") {
		t.Errorf("solid chip output %q does not contain label", out)
	}
}

func TestRender_Outline(t *testing.T) {
	t.Parallel()
	out := chip.Render(chip.Opts{Label: "infra", Color: "#7aa2f7", Variant: chip.Outline}, theme.TokyonightNight)
	if !strings.Contains(out, "infra") {
		t.Errorf("outline chip output %q does not contain label", out)
	}
}

func TestHash_Deterministic(t *testing.T) {
	t.Parallel()
	pal := []string{"#7dcfff", "#73daca", "#bb9af7", "#9ece6a"}
	first := chip.Hash("go", pal)
	second := chip.Hash("go", pal)
	if first != second {
		t.Errorf("Hash not deterministic: %q vs %q", first, second)
	}
}

func TestHash_StaysInPalette(t *testing.T) {
	t.Parallel()
	pal := []string{"#aaaaaa", "#bbbbbb"}
	for _, in := range []string{"a", "b", "c", "d", "tag-with-dash", "öäü"} {
		got := chip.Hash(in, pal)
		found := false
		for _, c := range pal {
			if got == c {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Hash(%q) = %q, not in palette %v", in, got, pal)
		}
	}
}

func TestHash_EmptyPalette(t *testing.T) {
	t.Parallel()
	if got := chip.Hash("x", nil); got != "" {
		t.Errorf("Hash with empty palette = %q, want empty", got)
	}
}
