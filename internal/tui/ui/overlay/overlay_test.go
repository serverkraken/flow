package overlay_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/tui/ui/overlay"
)

func TestRender_centersBoxWithinField(t *testing.T) {
	box := "PALETTE"
	got := overlay.Render(box, 40, 10)
	if !strings.Contains(got, "PALETTE") {
		t.Fatalf("overlay must contain the box content, got %q", got)
	}
	if lipgloss.Width(got) != 40 {
		t.Fatalf("overlay width = %d, want 40", lipgloss.Width(got))
	}
	if lipgloss.Height(got) != 10 {
		t.Fatalf("overlay height = %d, want 10", lipgloss.Height(got))
	}
}

func TestRender_zeroDimsFallBackToBox(t *testing.T) {
	got := overlay.Render("X", 0, 0)
	if !strings.Contains(got, "X") {
		t.Fatalf("zero dims should still render the box, got %q", got)
	}
}
