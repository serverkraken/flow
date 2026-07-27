// Package docrow renders one document entry in the kompendium "list row" look:
// an accent stripe when selected, a dim date cell, a kind badge, the
// project/title label, then indented excerpt lines. It is presentation-only and
// domain-free — callers resolve the document fields to strings and pass the
// excerpt already styled (the list passes a dimmed body preview; the search view
// passes a highlighted match snippet), so every surface shares one row design.
package docrow

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
)

// Row is one rendered document entry. Date and Badge are pre-formatted cells;
// Excerpt holds already-styled lines (rendered verbatim under the header).
type Row struct {
	Date     string
	Badge    string
	Label    string
	Excerpt  []string
	Selected bool
}

// Render returns the row as a multi-line string ending in a blank separator line.
func Render(r Row, pal theme.Palette) string {
	stripe := "  "
	labelStyle := lipgloss.NewStyle().Foreground(pal.Fg)
	if r.Selected {
		stripe = lipgloss.NewStyle().Foreground(pal.Sem().Active).Render(glyphs.AccentBar) + " "
		labelStyle = labelStyle.Bold(true)
	}

	var b strings.Builder
	b.WriteString(stripe +
		theme.Dim(r.Date, pal) + "  " +
		r.Badge + "  " +
		labelStyle.Render(r.Label) + "\n")
	for _, line := range r.Excerpt {
		b.WriteString("   " + line + "\n")
	}
	b.WriteString("\n")
	return b.String()
}
