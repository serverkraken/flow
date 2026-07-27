package tabstrip_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/tabstrip"
)

func TestRender_showsAllWhenWide(t *testing.T) {
	got := tabstrip.Render([]string{"Home", "Worktime", "Docs"}, 1, 200, theme.Default)
	for _, w := range []string{"Home", "Worktime", "Docs"} {
		if !strings.Contains(got, w) {
			t.Fatalf("wide strip %q missing %q", got, w)
		}
	}
}

func TestRender_empty(t *testing.T) {
	if tabstrip.Render(nil, 0, 80, theme.Default) != "" {
		t.Fatal("nil titles -> empty")
	}
}

func TestRender_overflowFitsWidthAndKeepsActive(t *testing.T) {
	titles := []string{"Home", "Worktime", "Docs", "Stats", "DayOffs", "Export", "Projects"}
	const width = 24
	got := tabstrip.Render(titles, 5, width, theme.Default) // active = "Export"
	if lipgloss.Width(got) > width {
		t.Fatalf("overflow strip width %d exceeds %d: %q", lipgloss.Width(got), width, got)
	}
	if !strings.Contains(got, "Export") {
		t.Fatalf("overflow strip must keep active tab, got %q", got)
	}
}
