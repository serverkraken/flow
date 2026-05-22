package sidekick_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/sidekick"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// At narrow widths the tab strip degrades to the compact `[p]` form
// instead of `p Palette`. The renderTabStripCompact branch wasn't
// reached by the existing suite — covers it directly.

func TestRenderTabStrip_NarrowWidthDegradesToCompact(t *testing.T) {
	t.Parallel()
	deps, _, _, _, _, _ := newDeps()
	m := sidekick.New(theme.Palette{}, domain.DefaultFlowState(), deps)
	// Tight width — full strip would exceed it. View() composes the
	// tab strip into the render.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 12, Height: 24})
	out := updated.View().Content
	// Compact form has bracketed keys: `[p]` for active.
	if !strings.Contains(out, "[p]") && !strings.Contains(out, "(p)") {
		t.Errorf("narrow-width tab strip should use compact bracketed form, got:\n%s", out)
	}
}
