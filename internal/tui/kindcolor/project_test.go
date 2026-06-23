package kindcolor_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/kindcolor"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// Drift guard: every domain palette name must map to a real hue (not the muted
// fallback), else a project could carry a color the TUI renders as grey.
func TestProjectColorCoversWholePalette(t *testing.T) {
	p := theme.Default
	for _, name := range domain.ProjectColors {
		if got := kindcolor.ProjectColor(name, p); got == p.FgMuted {
			t.Errorf("color %q resolved to the muted fallback", name)
		}
	}
	if kindcolor.ProjectColor("", p) != p.FgMuted {
		t.Error("empty name → FgMuted")
	}
	if kindcolor.ProjectColor("chartreuse", p) != p.FgMuted {
		t.Error("unknown name → FgMuted")
	}
}

func TestProjectColorMapsKnown(t *testing.T) {
	p := theme.Default
	if kindcolor.ProjectColor("blue", p) != p.Blue {
		t.Error("blue must map to p.Blue")
	}
	if kindcolor.ProjectColor("teal", p) != p.Teal {
		t.Error("teal must map to p.Teal")
	}
}
