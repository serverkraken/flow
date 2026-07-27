// Package countbar renders a counts line: "n/m <noun> · <glyph> <N> <label> …".
// Domain-free: the caller supplies segments (glyph, label, count, color).
package countbar

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// Seg is one colored count segment.
type Seg struct {
	Glyph string
	Label string
	N     int
	Color theme.Color
}

// Render returns the counts line. visible/total render as "visible/total noun".
func Render(visible, total int, noun string, segs []Seg, p theme.Palette) string {
	head := theme.Strong(fmt.Sprintf("%d/%d %s", visible, total, noun), p)
	parts := make([]string, 0, len(segs))
	for _, s := range segs {
		g := lipgloss.NewStyle().Foreground(s.Color).Render(fmt.Sprintf("%s %d", s.Glyph, s.N))
		parts = append(parts, g+theme.Dim(" "+s.Label, p))
	}
	return head + theme.Dim("   ·   ", p) + strings.Join(parts, "  ")
}
