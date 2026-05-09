// Package help renders a key-binding overlay for use as a ? screen.
package help

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/frontend/tui/components/titlebox"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// Section groups related key bindings under a heading.
type Section struct {
	Title string
	Keys  [][2]string // [0] = key combo, [1] = description
}

// Render draws a themed help overlay inside a titlebox.
// keyWidth controls the fixed column width for the key labels.
func Render(title string, sections []Section, keyWidth, boxWidth int, p theme.Palette) string {
	sem := p.Sem()
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color(sem.Accent)).Bold(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color(p.FgMuted))
	fg := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Fg))

	var rows []string
	for i, sec := range sections {
		if i > 0 {
			// Blank-Spacer zwischen Sections, nicht NACH der letzten —
			// sonst hängt eine leere Zeile direkt am unteren Border, was die
			// Box vertikal unbalanciert wirken lässt.
			rows = append(rows, "")
		}
		rows = append(rows, accent.Render("  "+sec.Title))
		for _, kv := range sec.Keys {
			// 4-Char-Indent außerhalb des Width-Budgets, damit lange Key-
			// Combos (z.B. "Ctrl+D / Ctrl+U") nicht den gesamten Slot
			// auffressen und damit die Description ohne führendes Space
			// rechts daneben klebt.
			key := "    " + fg.Width(keyWidth).Render(kv[0])
			rows = append(rows, key+dim.Render(kv[1]))
		}
	}

	body := strings.Join(rows, "\n")
	return titlebox.Render(title, body, boxWidth, p)
}
