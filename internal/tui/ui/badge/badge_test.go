package badge_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/badge"
)

func TestRender_WidthAndContent(t *testing.T) {
	t.Parallel()
	p := theme.Default
	out := badge.Render("PROJ.", p.Sem().Success, p)
	if !strings.Contains(out, "PROJ.") {
		t.Errorf("badge missing label: %q", out)
	}
	if w := lipgloss.Width(out); w != lipgloss.Width("PROJ.")+2 {
		t.Errorf("badge width = %d, want label+2", w)
	}
}
