// Package keyhint renders the contextual footer key-hint line. A Route
// returns []Hint from KeyHints(); the shell shows up to maxFooter of them in
// the footer and the rest in the ?-help overlay.
package keyhint

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// Hint is one footer key-hint: a key token and what it does.
type Hint struct {
	Key  string
	Desc string
}

// maxFooter is how many hints fit on the footer line; the rest live in ?-help.
const maxFooter = 4

// Render formats up to maxFooter hints as "key desc  ·  key desc". Returns ""
// for no hints. Keys are accented, descriptions dimmed.
func Render(hints []Hint, p theme.Palette) string {
	if len(hints) == 0 {
		return ""
	}
	n := len(hints)
	if n > maxFooter {
		n = maxFooter
	}
	parts := make([]string, 0, n)
	for _, h := range hints[:n] {
		seg := theme.Active(h.Key, p)
		if h.Desc != "" {
			seg += " " + theme.Dim(h.Desc, p)
		}
		parts = append(parts, seg)
	}
	line := strings.Join(parts, theme.Dim("  ·  ", p))
	return lipgloss.NewStyle().Padding(0, theme.PadXS).Render(line)
}
