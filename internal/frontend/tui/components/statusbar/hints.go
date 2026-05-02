package statusbar

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
)

// Hints renders a dim footer line with horizontal padding, suitable for
// the bottom of a sidekick screen (e.g. "enter -> run  ·  q -> quit").
func Hints(text string, p theme.Palette) string {
	return lipgloss.NewStyle().Foreground(p.Dim).Padding(0, 1).Render(text)
}
