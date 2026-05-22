package statusbar

import (
	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// Hints renders a dim footer line with horizontal padding, suitable for
// the bottom of a sidekick screen (e.g. "enter -> run  ·  q -> quit").
func Hints(text string, p theme.Palette) string {
	return lipgloss.NewStyle().Foreground(p.FgMuted).Padding(0, 1).Render(text)
}
