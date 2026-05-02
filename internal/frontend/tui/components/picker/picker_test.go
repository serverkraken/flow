package picker_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
)

var testPalette = theme.Load()

func TestRow_SelectedShowsAccentBar(t *testing.T) {
	t.Parallel()
	row := picker.Row(true, "label", "hint", 40, testPalette)
	// The accent bar character must appear in the rendered output.
	if !strings.Contains(row, "▎") {
		t.Errorf("selected row does not contain accent bar ▎: %q", row)
	}
}

func TestRow_UnselectedShowsSpace(t *testing.T) {
	t.Parallel()
	row := picker.Row(false, "label", "hint", 40, testPalette)
	if strings.Contains(row, "▎") {
		t.Errorf("unselected row should not contain accent bar ▎: %q", row)
	}
	// Must start with a space (no bar).
	if len(row) == 0 || row[0] != ' ' {
		t.Errorf("unselected row should start with space, got %q", row)
	}
}

func TestRow_LabelAndHintPresent(t *testing.T) {
	t.Parallel()
	row := picker.Row(false, "my-label", "ctrl+x", 60, testPalette)
	if !strings.Contains(row, "my-label") {
		t.Errorf("row missing label: %q", row)
	}
	if !strings.Contains(row, "ctrl+x") {
		t.Errorf("row missing hint: %q", row)
	}
}

func TestSectionHeader_UppercasesName(t *testing.T) {
	t.Parallel()
	h := picker.SectionHeader("notes", 40, testPalette)
	if !strings.Contains(h, "NOTES") {
		t.Errorf("section header does not contain uppercase name: %q", h)
	}
}

func TestSectionHeader_FitsWidth(t *testing.T) {
	t.Parallel()
	const width = 40
	h := picker.SectionHeader("git", width, testPalette)
	got := lipgloss.Width(h)
	if got > width {
		t.Errorf("SectionHeader width = %d, want ≤ %d", got, width)
	}
}
