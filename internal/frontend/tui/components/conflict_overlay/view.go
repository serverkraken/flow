package conflict_overlay

import (
	"strings"

	"charm.land/lipgloss/v2"

	tuistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
)

// View renders the conflict overlay sized to (m.width, m.height).
// Returns "" when the screen is too small for useful chrome.
func (m Model) View() string {
	if m.width < minWidth || m.height < chromeVertical {
		return ""
	}
	return m.renderChrome()
}

// renderChrome assembles the full overlay:
//   - rounded border (outer frame)
//   - title row (bold + Danger)
//   - separator rule
//   - blank line
//   - body (plain text)
//   - blank line
//   - choice lines: "[key] label"
//   - Esc cancel hint (dim)
func (m Model) renderChrome() string {
	s := styles()
	// inner width: outer width minus border (2) minus padding (PadSM*2 = 4)
	innerW := m.width - contentLineBudget - 2 // 2 border + 4 padding (PadSM=2 each side)
	if innerW < 4 {
		innerW = 4
	}

	// ── Title ──────────────────────────────────────────────────────────
	titleText := tuistrings.Truncate(m.title, innerW)
	title := s.title.Render(titleText)

	// ── Separator ──────────────────────────────────────────────────────
	// Dashes fill the inner width exactly.
	sep := s.separator.Render(strings.Repeat("─", innerW))

	// ── Body ───────────────────────────────────────────────────────────
	bodyLines := strings.Split(m.body, "\n")
	renderedBody := make([]string, len(bodyLines))
	for i, line := range bodyLines {
		clipped := tuistrings.Truncate(line, innerW)
		renderedBody[i] = s.body.Render(clipped)
	}
	body := strings.Join(renderedBody, "\n")

	// ── Choices ────────────────────────────────────────────────────────
	choiceLines := make([]string, 0, len(m.choices)+1)
	for _, c := range m.choices {
		bracket := s.choiceKey.Render("[" + c.key + "]")
		// Truncate only the label part when the combined line would overflow.
		fullLine := bracket + s.choiceLabel.Render(" "+c.label)
		if lipgloss.Width(fullLine) > innerW {
			// Reserve 3 cells for "[x]" + 1 for space before label.
			labelBudget := innerW - 4
			if labelBudget < 1 {
				labelBudget = 1
			}
			fullLine = bracket + s.choiceLabel.Render(" "+tuistrings.Truncate(c.label, labelBudget))
		}
		choiceLines = append(choiceLines, fullLine)
	}
	// Esc hint: dim, always last. "[esc] abbrechen" — HintCancel would
	// produce "[esc] Esc → abbrechen" (redundant key mention), so we use
	// the German verb directly.
	escLine := s.escHint.Render("[esc] abbrechen")
	choiceLines = append(choiceLines, escLine)
	choices := strings.Join(choiceLines, "\n")

	// ── Assemble content ───────────────────────────────────────────────
	// title + sep + blank + body + blank + choices
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		sep,
		"",
		body,
		"",
		choices,
	)

	// Apply the outer rounded frame. Under lipgloss v2, Width(n) is
	// outer-total (border + padding + content), so we pass m.width directly.
	return s.frame.Width(m.width).Render(content)
}
