package theme_test

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
)

func TestLoad_ReturnsFallbacksOutsideTmux(t *testing.T) {
	// t.Setenv and t.Parallel cannot be combined in Go 1.26+.
	t.Setenv("TMUX", "")

	p := theme.Load()

	if p.Bg != lipgloss.Color("#24283b") {
		t.Errorf("Bg: got %q, want %q", p.Bg, "#24283b")
	}
	if p.Accent != lipgloss.Color("#7aa2f7") {
		t.Errorf("Accent: got %q, want %q", p.Accent, "#7aa2f7")
	}
	if p.Green != lipgloss.Color("#9ece6a") {
		t.Errorf("Green: got %q, want %q", p.Green, "#9ece6a")
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
