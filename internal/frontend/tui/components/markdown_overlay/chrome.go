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

// renderFooter assembles the footer hint row. In ModeSearch the
// row hosts the live textinput + Enter/Esc hints. In ModeNormal it
// lists the available scroll + close keys, plus a search hint when
// the feature is enabled.
func (m Model) renderFooter() string {
	if m.mode == ModeSearch {
		view := m.search.View()
		if view == "" {
			view = "▎"
		}
		return searchActiveLabelStyle.Render("Suche:") + " " + view +
			"   " + footerStyle.Render("Enter → übernehmen  ·  Esc → abbrechen")
	}
	parts := []string{footerStyle.Render("j/k → scrollen")}
	if m.cfg.enableSearch {
		parts = append(parts, footerKeyStyle.Render("/")+footerStyle.Render(" → suchen"))
	}
	parts = append(parts,
		footerKeyStyle.Render(strings.Join(m.cfg.closeKeys, "/"))+
			footerStyle.Render(" → zurück"))
	return strings.Join(parts, "  ·  ")
}

// renderStatusBar produces the bottom row: optional mode badge on the
// left, title path next, match-counter / copy-status / scroll-percent
// on the right. Filler space is rendered with the status-bar
// background so the bar reads as a solid bar even when truncated.
func (m Model) renderStatusBar() string {
	innerW := m.width - contentLineBudget
	if innerW <= 0 {
		return ""
	}
	mode := ""
	if m.mode == ModeSearch {
		mode = statusBarModeSearchStyle.Render("SEARCH")
	}
	title := m.cfg.title
	if title == "" {
		title = "—"
	}
	pathSegment := statusBarPathStyle.Render(" " + title + " ")
	right := m.statusBarRight()
	gap := innerW - lipgloss.Width(mode) - lipgloss.Width(pathSegment) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	return mode + pathSegment + statusBarStyle.Render(strings.Repeat(" ", gap)) + right
}

// statusBarRight selects the right-aligned label: match counter when a
// query is active, otherwise the scroll percent.
func (m Model) statusBarRight() string {
	if m.query != "" {
		var label string
		if len(m.matches) == 0 {
			label = " Keine Treffer "
		} else {
			label = " " + strconv.Itoa(m.matchIdx+1) + "/" + strconv.Itoa(len(m.matches)) + " "
		}
		return statusBarStyle.Render(label)
	}
	return statusBarStyle.Render(" " + formatPercent(m.viewport.ScrollPercent()) + " ")
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
