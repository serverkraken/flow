package theme_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func TestPill_FourWideForAllKnownStates(t *testing.T) {
	t.Parallel()
	p := theme.Load()

	states := []string{"OK", "FAIL", "RUN", "...", "skip", "unknown", ""}
	for _, s := range states {
		got := lipgloss.Width(theme.Pill(s, p))
		if got != 4 {
			t.Errorf("Pill(%q): visible width = %d, want 4", s, got)
		}
	}
}

// TestRenderPill_KindGlyphsAreDistinct: A11y-2 — every PillKind must
// render with its own distinct glyph so a NO_COLOR or colour-blind
// viewer can tell them apart.
func TestRenderPill_KindGlyphsAreDistinct(t *testing.T) {
	t.Parallel()
	p := theme.Load()

	cases := []struct {
		kind  theme.PillKind
		glyph string
	}{
		{theme.PillSuccess, "✓"},
		{theme.PillWarning, "▲"},
		{theme.PillDanger, "✗"},
		{theme.PillActive, "▶"},
		{theme.PillInfo, "›"},
		{theme.PillSkip, "○"},
		{theme.PillNeutral, "·"},
	}
	for _, tc := range cases {
		out := theme.RenderPill(tc.kind, "", p)
		if !strings.Contains(out, tc.glyph) {
			t.Errorf("RenderPill(%d) missing glyph %q in %q", tc.kind, tc.glyph, out)
		}
	}
}

// TestRenderPill_LabelAppears: a label passed to RenderPill must
// surface in the rendered string so the kind glyph carries the
// flavour and the label carries the specifics.
func TestRenderPill_LabelAppears(t *testing.T) {
	t.Parallel()
	p := theme.Load()
	out := theme.RenderPill(theme.PillSuccess, "passed", p)
	if !strings.Contains(out, "passed") {
		t.Errorf("missing label in %q", out)
	}
	if !strings.Contains(out, "✓") {
		t.Errorf("missing glyph in %q", out)
	}
}

func TestInit_NoPanicOutsideTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	// Must not panic when $TMUX is unset.
	theme.Init()
}
