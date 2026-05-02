// Package help renders a key-binding overlay for use as a ? screen.
package help

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
	"github.com/serverkraken/flow/internal/frontend/tui/components/titlebox"
)

// Section groups related key bindings under a heading.
type Section struct {
	Title string
	Keys  [][2]string // [0] = key combo, [1] = description
}

// Render draws a themed help overlay inside a titlebox.
// keyWidth controls the fixed column width for the key labels.
func Render(title string, sections []Section, keyWidth, boxWidth int, p theme.Palette) string {
	accent := lipgloss.NewStyle().Foreground(p.Accent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(p.Dim)
	fg := lipgloss.NewStyle().Foreground(p.Fg)

	var rows []string
	for _, sec := range sections {
		rows = append(rows, accent.Render("  "+sec.Title))
		for _, kv := range sec.Keys {
			key := fg.Width(keyWidth).Render("    " + kv[0])
			rows = append(rows, key+dim.Render(kv[1]))
		}
		rows = append(rows, "")
	}

	body := strings.Join(rows, "\n")
	return titlebox.Render(title, body, boxWidth, p)
}
