// Package tabstrip renders the shell's top tab row. When the rendered tabs
// exceed the available width it shows a window around the active tab with
// "‹"/"›" overflow markers so the active tab is always visible.
package tabstrip

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// Render draws the tab row. active is the selected tab index; width is the
// usable terminal width. Returns "" for no titles.
func Render(titles []string, active, width int, p theme.Palette) string {
	if len(titles) == 0 {
		return ""
	}
	sem := p.Sem()
	cells := make([]string, len(titles))
	for i, t := range titles {
		label := " " + t + " "
		if i == active {
			cells[i] = lipgloss.NewStyle().Foreground(sem.Active).Bold(true).Background(p.BgChipSoft).Render(label)
		} else {
			cells[i] = lipgloss.NewStyle().Foreground(p.FgMuted).Render(label)
		}
	}
	full := strings.Join(cells, " ")
	if width <= 0 || lipgloss.Width(full) <= width {
		return full
	}

	// Overflow: grow a window outward from the active tab while it fits,
	// reserving 2 cols for the "‹ "/" ›" markers.
	if active < 0 || active >= len(cells) {
		active = 0
	}
	lo, hi := active, active
	used := lipgloss.Width(cells[active])
	budget := width - 2
	for {
		grew := false
		if lo > 0 && used+1+lipgloss.Width(cells[lo-1]) <= budget {
			lo--
			used += 1 + lipgloss.Width(cells[lo])
			grew = true
		}
		if hi < len(cells)-1 && used+1+lipgloss.Width(cells[hi+1]) <= budget {
			hi++
			used += 1 + lipgloss.Width(cells[hi])
			grew = true
		}
		if !grew {
			break
		}
	}
	var b strings.Builder
	if lo > 0 {
		b.WriteString(theme.Dim("‹ ", p))
	}
	b.WriteString(strings.Join(cells[lo:hi+1], " "))
	if hi < len(cells)-1 {
		b.WriteString(theme.Dim(" ›", p))
	}
	return b.String()
}
