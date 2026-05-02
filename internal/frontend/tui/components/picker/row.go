// Package picker provides rendering primitives for filterable sectioned lists.
package picker

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
)

const accentBarRune = "▎"

// Row renders a single list entry: an accent bar on the left, label on the left
// side, and a hint string right-aligned within width.
//
// width is the available inner width (excluding any outer box border).
// Selected rows show the accent bar (▎) and bold foreground; unselected rows
// show a plain space.
func Row(selected bool, label, hint string, width int, p theme.Palette) string {
	bar := " "
	labelStyle := lipgloss.NewStyle().Foreground(p.Fg)
	if selected {
		bar = lipgloss.NewStyle().Foreground(p.Accent).Render(accentBarRune)
		labelStyle = lipgloss.NewStyle().Foreground(p.Fg).Bold(true)
	}
	hintStyle := lipgloss.NewStyle().Foreground(p.Dim)

	// bar(1) + space(1) + label + gap + hint
	gap := width - 1 - lipgloss.Width(label) - lipgloss.Width(hint) - 1
	if gap < 1 {
		gap = 1
	}
	return bar + " " + labelStyle.Render(label) + strings.Repeat(" ", gap) + hintStyle.Render(hint)
}

// SectionHeader renders an uppercased section name with trailing dash fill.
// width is the available inner width.
func SectionHeader(name string, width int, p theme.Palette) string {
	style := lipgloss.NewStyle().Foreground(p.Dim).Bold(true).Padding(0, 0, 0, 1)
	rendered := style.Render(strings.ToUpper(name))
	dashStyle := lipgloss.NewStyle().Foreground(p.Border)

	gap := width - lipgloss.Width(rendered) - 1
	if gap < 0 {
		gap = 0
	}
	return rendered + " " + dashStyle.Render(strings.Repeat("─", gap))
}
