// Package titlebox renders a rounded Unicode box with a title embedded in the top border.
package titlebox

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
)

// Render draws a rounded box with title in the top border.
// width is the total outer width including the border characters.
// body may contain newlines; each line is padded to fill the inner width exactly.
//
// Every line of the returned string is exactly width visible characters wide,
// which makes the box safe to use inside fixed-width layout containers.
func Render(title, body string, width int, p theme.Palette) string {
	border := lipgloss.NewStyle().Foreground(p.Border)
	titleStyle := lipgloss.NewStyle().Foreground(p.Purple).Bold(true)
	titleR := titleStyle.Render(title)
	inner := width - 2

	// Top border: ╭─ Title ──...──╮
	// "╭─ " + title + " " + "╮" = 3 + len(title) + 1 + 1
	used := 3 + lipgloss.Width(titleR) + 2
	dashes := width - used
	if dashes < 1 {
		dashes = 1
	}
	top := border.Render("╭─ ") + titleR + " " + border.Render(strings.Repeat("─", dashes)+"╮")

	// Body lines: │ <padded content> │
	pipe := border.Render("│")
	lines := strings.Split(body, "\n")
	rendered := make([]string, len(lines))
	for i, line := range lines {
		pad := inner - lipgloss.Width(line)
		if pad < 0 {
			pad = 0
		}
		rendered[i] = pipe + line + strings.Repeat(" ", pad) + pipe
	}

	// Bottom border: ╰──...──╯
	bottom := border.Render("╰" + strings.Repeat("─", inner) + "╯")

	return top + "\n" + strings.Join(rendered, "\n") + "\n" + bottom
}
