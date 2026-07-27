// Package badge renders a small colored label pill (dark text on a semantic
// color), e.g. kind badges in lists. Domain-free: the caller supplies label+color.
package badge

import (
	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// Render returns a bold dark-on-color pill with single-column horizontal padding.
func Render(label string, c theme.Color, p theme.Palette) string {
	return lipgloss.NewStyle().
		Foreground(p.Bg).
		Background(c).
		Bold(true).
		Padding(0, 1).
		Render(label)
}
