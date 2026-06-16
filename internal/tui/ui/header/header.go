// Package header renders the shell's top line: app title on the left, the
// current user on the right, separated by flexible space.
package header

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// Render draws "title ............ user" padded to width.
func Render(title, user string, width int, p theme.Palette) string {
	left := theme.Heading(title, p)
	right := theme.Dim(user, p)
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}
