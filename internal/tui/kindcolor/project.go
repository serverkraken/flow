package kindcolor

import (
	"github.com/serverkraken/flow/internal/tui/theme"
)

// NodeColor maps a whitelisted project color NAME (domain.NodeColors) to
// the active palette's hue. Unset or unknown names fall back to FgMuted (the
// caller renders a neutral swatch rather than guessing). Single source so the
// TUI cannot drift from the domain whitelist (enforced by a drift-guard test).
func NodeColor(name string, p theme.Palette) theme.Color {
	switch name {
	case "blue":
		return p.Blue
	case "cyan":
		return p.Cyan
	case "green":
		return p.Green
	case "purple":
		return p.Purple
	case "magenta":
		return p.Magenta
	case "yellow":
		return p.Yellow
	case "orange":
		return p.Orange
	case "red":
		return p.Red
	case "teal":
		return p.Teal
	default:
		return p.FgMuted
	}
}
