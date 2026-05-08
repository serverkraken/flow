package titlebox_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/frontend/tui/components/titlebox"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

var testPalette = theme.Load()

func TestRender_WidthInvariant(t *testing.T) {
	t.Parallel()
	const width = 60
	out := titlebox.Render("Test", "line one\nline two", width, testPalette)
	for i, line := range strings.Split(out, "\n") {
		got := lipgloss.Width(line)
		if got != width {
			t.Errorf("line %d: visible width = %d, want %d: %q", i, got, width, line)
		}
	}
}

func TestRender_TitlePresentInTopBorder(t *testing.T) {
	t.Parallel()
	out := titlebox.Render("MyTitle", "body", 50, testPalette)
	topLine := strings.SplitN(out, "\n", 2)[0]
	// Strip ANSI to check the raw text content.
	stripped := lipgloss.NewStyle().Render(topLine)
	if !strings.Contains(stripped, "MyTitle") {
		t.Errorf("top border does not contain title %q: %q", "MyTitle", stripped)
	}
}

func TestRender_MultiLineBody_LineCount(t *testing.T) {
	t.Parallel()
	// 3 body lines → top + 3 body + bottom = 5 output lines.
	body := "line1\nline2\nline3"
	out := titlebox.Render("T", body, 40, testPalette)
	lines := strings.Split(out, "\n")
	if len(lines) != 5 {
		t.Errorf("got %d lines, want 5", len(lines))
	}
}

func TestRender_EmptyBody(t *testing.T) {
	t.Parallel()
	// Empty body = 1 empty line → top + 1 + bottom = 3 lines total.
	out := titlebox.Render("T", "", 30, testPalette)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Errorf("got %d lines, want 3", len(lines))
	}
}
