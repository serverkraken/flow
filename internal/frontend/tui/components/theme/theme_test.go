package theme_test

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
)

func TestFallback_MapsCanonicalDefault(t *testing.T) {
	t.Parallel()

	// Canonical default is Tokyonight Night; Storm (#24283b) was dropped
	// per docs/design-system-audit.md §2.1. Asserting against Fallback()
	// (rather than Load()) avoids the host-tmux flakiness — Load reads
	// @tn_* options regardless of $TMUX, so its return value depends on
	// whether the dev machine has a tmux server running.
	p := theme.Fallback()

	if p.Bg != lipgloss.Color("#1a1b26") {
		t.Errorf("Bg: got %q, want %q", p.Bg, "#1a1b26")
	}
	if p.Accent != lipgloss.Color("#7aa2f7") {
		t.Errorf("Accent: got %q, want %q", p.Accent, "#7aa2f7")
	}
	if p.Green != lipgloss.Color("#9ece6a") {
		t.Errorf("Green: got %q, want %q", p.Green, "#9ece6a")
	}
	// Dim now sources from canonical FgMuted (#9aa5ce), bumped from
	// upstream Tokyonight `comment` (#565f89) so it clears WCAG AA.
	if p.Dim != lipgloss.Color("#9aa5ce") {
		t.Errorf("Dim: got %q, want %q", p.Dim, "#9aa5ce")
	}
}

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
		if !contains(out, tc.glyph) {
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
	if !contains(out, "passed") {
		t.Errorf("missing label in %q", out)
	}
	if !contains(out, "✓") {
		t.Errorf("missing glyph in %q", out)
	}
}

// contains is strings.Contains pulled inline so the test file's
// import block stays minimal.
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestInit_NoPanicOutsideTmux(t *testing.T) {
	t.Setenv("TMUX", "")

	// Must not panic when $TMUX is unset.
	theme.Init()
}
