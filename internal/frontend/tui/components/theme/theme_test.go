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

func TestInit_NoPanicOutsideTmux(t *testing.T) {
	t.Setenv("TMUX", "")

	// Must not panic when $TMUX is unset.
	theme.Init()
}
