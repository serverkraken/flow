package uidemo

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
	"github.com/serverkraken/flow/internal/tui/ui/statusbar"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// TestDesignSystemComposes renders the central progress primitive with the
// loaded palette and checks the canonical bar glyphs appear — a smoke test that
// theme + glyphs + statusbar compose after the port.
func TestDesignSystemComposes(t *testing.T) {
	p := theme.Load()

	full := statusbar.BarColored(100, 10, p.Sem().Success, p)
	if !strings.Contains(full, glyphs.BarFilled) {
		t.Fatalf("100%% bar should contain BarFilled %q: %q", glyphs.BarFilled, full)
	}
	if strings.Contains(full, glyphs.BarEmpty) {
		t.Fatalf("100%% bar should have no empty cells: %q", full)
	}

	half := statusbar.BarColored(50, 10, p.Sem().Active, p)
	if !strings.Contains(half, glyphs.BarFilled) || !strings.Contains(half, glyphs.BarEmpty) {
		t.Fatalf("50%% bar should mix filled+empty: %q", half)
	}

	empty := statusbar.BarColored(0, 10, p.Sem().Active, p)
	if strings.Contains(empty, glyphs.BarFilled) {
		t.Fatalf("0%% bar should have no filled cells: %q", empty)
	}
}
