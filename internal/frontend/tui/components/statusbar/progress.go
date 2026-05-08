// Package statusbar provides bottom-bar and progress rendering primitives.
package statusbar

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// Bar renders a horizontal progress bar using ▰ (filled) and ▱ (empty) block characters.
// pct is clamped to [0, 100]; cells is the total character width of the bar.
func Bar(pct, cells int, p theme.Palette) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := pct * cells / 100
	empty := cells - filled
	sem := p.Sem()

	f := lipgloss.NewStyle().Foreground(lipgloss.Color(sem.Accent)).Render(strings.Repeat("▰", filled))
	e := lipgloss.NewStyle().Foreground(lipgloss.Color(p.BgCode)).Render(strings.Repeat("▱", empty))
	return f + e
}
