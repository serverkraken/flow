// Package picker provides rendering primitives for filterable sectioned lists.
package picker

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	tuistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// AccentBarRune is the left-edge marker for the selected row in any
// picker-shaped list. Exported so other screens (palette etc.) can
// reuse the same glyph instead of redeclaring it locally.
const AccentBarRune = "▎"

// Row renders a single list entry: an accent bar on the left, label on
// the left side, and a hint string right-aligned within width.
//
// width is the available inner width (excluding any outer box border).
// Selected rows show the accent bar (▎) and bold foreground; unselected
// rows show a plain space.
//
// Label and hint are truncated with "…" when their combined width would
// overflow the row (Bubbletea Golden Rule #2). The hint keeps full
// priority; the label loses width first because it is more often the
// long-tail field (note titles, action labels) while the hint is
// usually a short tag like "[deep]" or a key-bind preview. When the
// row is so narrow that even the hint wouldn't fit, the hint is
// dropped and the label gets all remaining space.
func Row(selected bool, label, hint string, width int, p theme.Palette) string {
	sem := p.Sem()
	bar := " "
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Fg))
	if selected {
		bar = lipgloss.NewStyle().Foreground(lipgloss.Color(sem.Accent)).Render(AccentBarRune)
		labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.Fg)).Bold(true)
	}
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.FgMuted))

	// Reserved cells: bar(1) + space(1) + min-gap(1) = 3.
	const reserved = 3
	hintW := lipgloss.Width(hint)
	maxLabel := width - reserved - hintW
	if maxLabel < 1 {
		// No room for label + hint: drop the hint to give the label
		// space. A hint-only row would lose the primary content.
		hint = ""
		hintW = 0
		maxLabel = width - reserved
		if maxLabel < 1 {
			maxLabel = 1
		}
	}
	label = tuistrings.Truncate(label, maxLabel)

	gap := width - 1 - lipgloss.Width(label) - hintW - 1
	if gap < 1 {
		gap = 1
	}
	return bar + " " + labelStyle.Render(label) + strings.Repeat(" ", gap) + hintStyle.Render(hint)
}

// SectionHeader renders an uppercased section name with trailing dash fill.
// width is the available inner width.
func SectionHeader(name string, width int, p theme.Palette) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(p.FgMuted)).Bold(true).Padding(0, 0, 0, 1)
	rendered := style.Render(strings.ToUpper(name))
	dashStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.BgCode))

	gap := width - lipgloss.Width(rendered) - 1
	if gap < 0 {
		gap = 0
	}
	return rendered + " " + dashStyle.Render(strings.Repeat("─", gap))
}
