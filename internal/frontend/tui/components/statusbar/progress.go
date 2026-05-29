// Package statusbar provides bottom-bar and progress rendering primitives.
package statusbar

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// Bar renders a horizontal progress bar using glyphs.BarFilled (▰) and
// glyphs.BarEmpty (▱). pct is clamped to [0, 100]; cells is the total
// character width of the bar. Filled cells render in Sem.Accent — the
// neutral progress hue. Callers wanting threshold-aware colouring
// (running cyan / done green / over-target red) pass an explicit colour
// via BarColored.
func Bar(pct, cells int, p theme.Palette) string {
	return BarColored(pct, cells, p.Sem().Accent, p)
}

// BarColored renders the same progress bar as Bar but with the caller-
// supplied colour for the filled segment. Heute threads
// totalThresholdColor (cyan while running / green at target / red far
// over) into the bar so it carries the same state signal as the total
// next to it — without this the bar stayed Accent-blue regardless of
// state and only the numeric total carried colour.
func BarColored(pct, cells int, filled color.Color, p theme.Palette) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filledCells := pct * cells / 100
	empty := cells - filledCells

	f := lipgloss.NewStyle().Foreground(filled).Render(strings.Repeat(glyphs.BarFilled, filledCells))
	e := lipgloss.NewStyle().Foreground(p.Sem().Border).Render(strings.Repeat(glyphs.BarEmpty, empty))
	return f + e
}
