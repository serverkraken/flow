package help_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/components/help"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

var testPalette = theme.Load()

func TestRender_ContainsSectionTitles(t *testing.T) {
	t.Parallel()
	sections := []help.Section{
		{Title: "Global", Keys: [][2]string{{"q", "Quit"}}},
		{Title: "Navigation", Keys: [][2]string{{"j", "Down"}, {"k", "Up"}}},
	}
	out := help.Render("Help", sections, 20, 60, testPalette)
	if !strings.Contains(out, "Global") {
		t.Error("output missing section title 'Global'")
	}
	if !strings.Contains(out, "Navigation") {
		t.Error("output missing section title 'Navigation'")
	}
}

func TestRender_ContainsKeysAndDescriptions(t *testing.T) {
	t.Parallel()
	sections := []help.Section{
		{Title: "Test", Keys: [][2]string{{"ctrl+c", "Exit"}}},
	}
	out := help.Render("Help", sections, 20, 60, testPalette)
	if !strings.Contains(out, "ctrl+c") {
		t.Error("output missing key 'ctrl+c'")
	}
	if !strings.Contains(out, "Exit") {
		t.Error("output missing description 'Exit'")
	}
}

func TestRender_ContainsTitle(t *testing.T) {
	t.Parallel()
	sections := []help.Section{
		{Title: "S", Keys: [][2]string{{"a", "b"}}},
	}
	out := help.Render("My Title", sections, 20, 60, testPalette)
	if !strings.Contains(out, "My Title") {
		t.Error("output missing box title")
	}
}

func TestRender_EmptySections(t *testing.T) {
	t.Parallel()
	out := help.Render("Help", nil, 20, 40, testPalette)
	if out == "" {
		t.Error("expected non-empty output even with no sections")
	}
}
