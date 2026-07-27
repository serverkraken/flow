package countbar_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/countbar"
)

func TestRender_CountsAndNoun(t *testing.T) {
	t.Parallel()
	p := theme.Default
	out := countbar.Render(9, 24, "Notizen", []countbar.Seg{
		{Glyph: "●", Label: "täglich", N: 10, Color: p.Sem().Accent},
		{Glyph: "◆", Label: "projekt", N: 9, Color: p.Sem().Success},
	}, p)
	for _, want := range []string{"9/24 Notizen", "täglich", "projekt", "● 10", "◆ 9"} {
		if !strings.Contains(out, want) {
			t.Errorf("countbar missing %q in:\n%s", want, out)
		}
	}
}
