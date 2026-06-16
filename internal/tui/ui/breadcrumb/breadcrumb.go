// Package breadcrumb renders the drill-down path of the active nav-stack,
// e.g. "Docs › Note › Backlink". Returns "" at depth 1 (no drill-down).
package breadcrumb

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// sep is the whitelisted info glyph "›" used between crumbs.
const sep = "›"

// Render joins crumbs with "›"; the last (current) crumb is emphasized.
func Render(crumbs []string, p theme.Palette) string {
	if len(crumbs) <= 1 {
		return ""
	}
	parts := make([]string, len(crumbs))
	for i, c := range crumbs {
		if i == len(crumbs)-1 {
			parts[i] = theme.Strong(c, p)
		} else {
			parts[i] = theme.Dim(c, p)
		}
	}
	joined := strings.Join(parts, theme.Dim(" "+sep+" ", p))
	return lipgloss.NewStyle().Padding(0, theme.PadXS).Render(joined)
}
