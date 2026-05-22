// Package titlebox renders a rounded Unicode box with a title embedded in the top border.
package titlebox

import (
	"strings"

	"charm.land/lipgloss/v2"
	tuistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
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
//
// Width < 7: not enough room for "╭─ X ─╮" (corners + "─ x ─"); the top
// border degrades to a plain corner line ("╭───╮") so the rendered
// width still equals `width` exactly. Below width 4 nothing reasonable
// fits; the function returns "" so the caller can degrade gracefully.
func Render(title, body string, width int, p theme.Palette) string {
	if width < 4 {
		return ""
	}
	sem := p.Sem()
	border := lipgloss.NewStyle().Foreground(sem.Border)
	titleStyle := lipgloss.NewStyle().Foreground(sem.Highlight).Bold(true)
	inner := width - 2

	// Title budget: width − "╭─ " (3) − " " (1) − "╮" (1) − ≥1 right-dash = width − 6.
	// Below that, fall back to a title-less top border so width stays exact —
	// the previous impl produced a top row 1–2 chars wider than width when
	// titleBudget was clamped to 1, breaking the alignment with the body.
	titleBudget := width - 6
	var top string
	if titleBudget < 1 {
		top = border.Render("╭" + strings.Repeat("─", inner) + "╮")
	} else {
		titleR := titleStyle.Render(tuistrings.Truncate(title, titleBudget))
		used := 3 + lipgloss.Width(titleR) + 2
		dashes := width - used
		if dashes < 1 {
			dashes = 1
		}
		top = border.Render("╭─ ") + titleR + " " + border.Render(strings.Repeat("─", dashes)+"╮")
	}

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
