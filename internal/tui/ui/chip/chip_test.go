package chip_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/chip"
)

func TestRender_WrapsLabelInAngles(t *testing.T) {
	t.Parallel()
	p := theme.Default
	out := chip.Render("serverkraken/flow", p.Sem().Accent, p)
	if !strings.Contains(out, "serverkraken/flow") {
		t.Errorf("chip missing label: %q", out)
	}
	if !strings.Contains(out, "⟨") || !strings.Contains(out, "⟩") {
		t.Errorf("chip missing angle brackets: %q", out)
	}
}
