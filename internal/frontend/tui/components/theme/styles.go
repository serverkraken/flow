package theme

import "github.com/charmbracelet/lipgloss"

// Pill renders a fixed-width (4 visible characters) colored status indicator.
//
// Known states and their colors:
//   - "OK"   → green
//   - "FAIL" → red
//   - "RUN"  → cyan
//   - "..."  → orange
//   - "skip" → dim
//
// All other values render in dim.
func Pill(state string, p Palette) string {
	var c lipgloss.Color
	switch state {
	case "OK":
		c = p.Green
	case "FAIL":
		c = p.Red
	case "RUN":
		c = p.Cyan
	case "...":
		c = p.Orange
	case "skip":
		c = p.Dim
	default:
		c = p.Dim
	}
	return lipgloss.NewStyle().Foreground(c).Bold(true).Width(4).Render(state)
}
