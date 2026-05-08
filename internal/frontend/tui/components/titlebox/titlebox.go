// Package titlebox renders a rounded Unicode box with a title embedded in the top border.
package titlebox

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	tuistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
)

// Render draws a rounded box with title in the top border.
// width is the total outer width including the border characters.
// body may contain newlines; each line is truncated and padded to fill
// the inner width exactly.
//
// Every line of the returned string is exactly width visible characters
// wide, which makes the box safe to use inside fixed-width layout
// containers. A title or body line wider than the inner space is clipped
// with "…" instead of breaking the right border (Bubbletea Golden Rule
// #2 — never auto-wrap in bordered panels).
func Render(title, body string, width int, p theme.Palette) string {
	border := lipgloss.NewStyle().Foreground(p.Border)
	titleStyle := lipgloss.NewStyle().Foreground(p.Purple).Bold(true)
	inner := width - 2

	// Title budget: width − "╭─ " (3) − " " (1) − "╮" (1) = width − 5.
	// Reserve at least 1 dash on the right so the corner glyph never
	// sits flush against the title text.
	titleBudget := width - 5 - 1
	if titleBudget < 1 {
		titleBudget = 1
	}
	titleR := titleStyle.Render(tuistrings.Truncate(title, titleBudget))

	used := 3 + lipgloss.Width(titleR) + 2
	dashes := width - used
	if dashes < 1 {
		dashes = 1
	}
	top := border.Render("╭─ ") + titleR + " " + border.Render(strings.Repeat("─", dashes)+"╮")

	// Body lines: │ <truncated and padded content> │. Truncate first so
	// a single overlong line cannot push past the right border; pad
	// after to fill the inner width exactly.
	pipe := border.Render("│")
	lines := strings.Split(body, "\n")
	rendered := make([]string, len(lines))
	for i, line := range lines {
		clipped := tuistrings.Truncate(line, inner)
		pad := inner - lipgloss.Width(clipped)
		if pad < 0 {
			pad = 0
		}
		rendered[i] = pipe + clipped + strings.Repeat(" ", pad) + pipe
	}

	// Bottom border: ╰──...──╯
	bottom := border.Render("╰" + strings.Repeat("─", inner) + "╯")

	return top + "\n" + strings.Join(rendered, "\n") + "\n" + bottom
}
