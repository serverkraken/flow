package markdown_overlay

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderChrome assembles the frame around the body: title row,
// separator, body (viewport view), footer hint row, and status bar.
// The rounded border is applied on the outside via frameStyle.
func (m Model) renderChrome() string {
	lineW := m.width - contentLineBudget
	if lineW < 1 {
		lineW = 1
	}
	title := titleStyle.Render(m.cfg.title)
	sep := separatorStyle.Render(strings.Repeat("─", lineW))
	body := m.viewport.View()
	footer := m.renderFooter()
	statusBar := m.renderStatusBar()

	content := lipgloss.JoinVertical(lipgloss.Left, title, sep, body, footer, statusBar)
	// Match the historical browse.frameContent padding behaviour: the
	// rounded border otherwise stops at the status bar and leaves a
	// half-blank pane on tall windows.
	target := m.height - 2
	if got := strings.Count(content, "\n") + 1; target > 0 && got < target {
		content += strings.Repeat("\n", target-got)
	}
	return frameStyle.Width(m.width - frameWidthOffset).Render(content)
}

// renderFooter assembles the footer hint row. Only the close-key hint
// is universal; search / code-copy hints are added by later tasks when
// those features become opt-in.
func (m Model) renderFooter() string {
	parts := []string{footerStyle.Render("j/k → scrollen")}
	parts = append(parts,
		footerKeyStyle.Render(strings.Join(m.cfg.closeKeys, "/"))+
			footerStyle.Render(" → zurück"))
	return strings.Join(parts, "  ·  ")
}

// renderStatusBar produces the bottom row: title path on the left,
// scroll-percent (or, in later tasks, match-counter / copy-status) on
// the right. Filler space between is rendered with the status-bar
// background so the bar reads as a solid bar even when truncated.
func (m Model) renderStatusBar() string {
	innerW := m.width - contentLineBudget
	if innerW <= 0 {
		return ""
	}
	title := m.cfg.title
	if title == "" {
		title = "—"
	}
	pathSegment := statusBarPathStyle.Render(" " + title + " ")
	right := statusBarStyle.Render(" " + formatPercent(m.viewport.ScrollPercent()) + " ")
	gap := innerW - lipgloss.Width(pathSegment) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	return pathSegment + statusBarStyle.Render(strings.Repeat(" ", gap)) + right
}

// formatPercent rounds the scroll percent to an integer and appends "%".
// Result fits in 4 cells (e.g. "0%", "42%", "100%").
func formatPercent(p float64) string {
	pct := int(p*100 + 0.5)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return strconv.Itoa(pct) + "%"
}
