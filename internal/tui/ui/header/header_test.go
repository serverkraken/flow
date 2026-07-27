package header_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/header"
)

func TestRender_containsTitleAndUser(t *testing.T) {
	got := header.Render("flow", "alice", 40, theme.Default)
	if !strings.Contains(got, "flow") || !strings.Contains(got, "alice") {
		t.Fatalf("header %q missing title or user", got)
	}
	if lipgloss.Width(got) > 40 {
		t.Fatalf("header width %d exceeds 40", lipgloss.Width(got))
	}
}

func TestRender_narrowDoesNotPanic(t *testing.T) {
	_ = header.Render("flow", "alice", 3, theme.Default)
}
