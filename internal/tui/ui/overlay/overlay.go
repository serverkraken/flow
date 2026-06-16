// Package overlay composites a modal box centered on a field of the given
// size, for the shell's :-palette and ?-help layers.
package overlay

import "charm.land/lipgloss/v2"

// Render centers box within a width×height field. Non-positive dims fall back
// to the box's own measured size.
func Render(box string, width, height int) string {
	if width <= 0 {
		width = lipgloss.Width(box)
	}
	if height <= 0 {
		height = lipgloss.Height(box)
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}
