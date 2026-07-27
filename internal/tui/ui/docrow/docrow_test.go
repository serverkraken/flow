package docrow_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/docrow"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
)

func TestRender_unselectedVsSelected(t *testing.T) {
	t.Parallel()
	row := docrow.Row{Date: "2026-06-20", Badge: "[F]", Label: "serverkraken/flow", Excerpt: []string{"first", "second"}}

	un := docrow.Render(row, theme.Default)
	if strings.Contains(un, glyphs.AccentBar) {
		t.Errorf("unselected row must not show the accent bar:\n%q", un)
	}
	if !strings.HasPrefix(un, "  ") {
		t.Errorf("unselected row should start with two spaces:\n%q", un)
	}

	row.Selected = true
	sel := docrow.Render(row, theme.Default)
	if !strings.Contains(sel, glyphs.AccentBar) {
		t.Errorf("selected row should show the accent bar:\n%q", sel)
	}
}

func TestRender_labelExcerptAndNoBox(t *testing.T) {
	t.Parallel()
	out := docrow.Render(docrow.Row{
		Date:    "2026-06-20",
		Label:   "my-note",
		Excerpt: []string{"excerpt A", "excerpt B"},
	}, theme.Default)

	for _, want := range []string{"2026-06-20", "my-note", "excerpt A", "excerpt B"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "   excerpt A") {
		t.Errorf("excerpt lines should be indented by three spaces:\n%q", out)
	}
	if strings.ContainsAny(out, "╭╰│") {
		t.Errorf("a doc row must not draw a box:\n%q", out)
	}
}
