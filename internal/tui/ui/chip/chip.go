// Package chip renders a compact ⟨ label ⟩ chip used for an active filter or
// context indicator. Domain-free: the caller supplies label+color.
package chip

import (
	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// Render returns ⟨ label ⟩ as a bold dark-on-color chip.
func Render(label string, c theme.Color, p theme.Palette) string {
	return lipgloss.NewStyle().
		Foreground(p.Bg).
		Background(c).
		Bold(true).
		Padding(0, 1).
		Render("⟨ " + label + " ⟩")
}
