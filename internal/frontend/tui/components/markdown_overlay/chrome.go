package markdown_overlay

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// renderChrome assembles the frame around the body: title row,
// separator, body (viewport view), footer hint row, and status bar.
// The rounded border is applied on the outside via the frame style.
func (m Model) renderChrome() string {
	s := styles()
	lineW := m.width - contentLineBudget
	if lineW < 1 {
		lineW = 1
	}
	// Truncate the title before styling: a 200-cell kompendium-note
	// title blew past the frame width on a narrow tmux pane, breaking
	// the rounded border. ansi.Truncate is grapheme-aware so wide
	// characters / OSC sequences in a title survive correctly.
	title := s.title.Render(ansi.Truncate(m.cfg.title, lineW, "…"))
	sep := s.separator.Render(strings.Repeat("─", lineW))
	body := m.bodyView()
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
	return s.frame.Width(m.width - frameWidthOffset).Render(content)
}

// renderFooter assembles the footer hint row. In ModeSearch the
// row hosts the live textinput + Enter/Esc hints. In ModeNormal it
// lists the available scroll + close keys, plus a search hint when
// the feature is enabled. Below ~30-cell widths the row would wrap
// onto multiple lines and push the status bar past the chrome budget,
// floating the bottom border; renderFooter degrades the separator and
// drops optional hints before that happens.
func (m Model) renderFooter() string {
	s := styles()
	if m.mode == ModeSearch {
		view := m.search.View()
		if view == "" {
			view = "▎"
		}
		return s.searchActiveLabel.Render("Suche:") + " " + view +
			"   " + s.footer.Render("Enter → übernehmen  ·  Esc → abbrechen")
	}
	lineW := m.width - contentLineBudget
	if lineW < 1 {
		lineW = 1
	}
	scrollHint := s.footer.Render("j/k → scrollen")
	closeHint := s.footer.Render(strings.Join(m.cfg.closeKeys, "/") + " → zurück")

	// Optional hints in priority order (search → code-copy → host extras).
	// Dropped from the right when widths tighten so the host-supplied
	// extras (typically context-specific) survive longest.
	var optional []string
	if m.cfg.enableSearch {
		optional = append(optional, s.footer.Render("/ → suchen"))
	}
	if m.cfg.enableCodeCopy {
		optional = append(optional, s.footer.Render("c → Code kopieren"))
	}
	for _, x := range m.cfg.footerExtras {
		optional = append(optional, s.footer.Render(x))
	}

	// Progressive degrade: prefer wider separators (more visual breathing
	// room) before dropping optional hints — at most widths the user is
	// better served by a denser row that keeps every affordance visible.
	// For each separator try the full hint set first, then drop one
	// optional hint at a time from the right until the row fits.
	for _, sep := range []string{"  ·  ", " · ", " "} {
		for n := len(optional); n >= 0; n-- {
			parts := append([]string{scrollHint}, optional[:n]...)
			parts = append(parts, closeHint)
			joined := strings.Join(parts, sep)
			if lipgloss.Width(joined) <= lineW {
				return joined
			}
		}
	}
	// Pane too narrow even for scroll + close on one line — truncate so
	// the row stays a single line and the status bar keeps its slot.
	return ansi.Truncate(scrollHint+" "+closeHint, lineW, "…")
}

// renderStatusBar produces the bottom row: optional mode badge on the
// left, title path next, match-counter / copy-status / scroll-percent
// on the right. Filler space is rendered with the status-bar
// background so the bar reads as a solid bar even when truncated.
func (m Model) renderStatusBar() string {
	s := styles()
	innerW := m.width - contentLineBudget
	if innerW <= 0 {
		return ""
	}
	mode := ""
	if m.mode == ModeSearch {
		mode = s.statusBarModeSearch.Render("SEARCH")
	}
	title := m.cfg.title
	if title == "" {
		title = "—"
	}
	// Title in the bottom status bar shares the line with the mode
	// badge and the right-aligned counter — clamp it to whatever
	// horizontal budget is left so a long title never pushes the
	// counter into the next line.
	right := m.statusBarRight()
	titleBudget := innerW - lipgloss.Width(mode) - lipgloss.Width(right) - 2
	if titleBudget < 1 {
		titleBudget = 1
	}
	pathSegment := s.statusBarPath.Render(" " + ansi.Truncate(title, titleBudget, "…") + " ")
	gap := innerW - lipgloss.Width(mode) - lipgloss.Width(pathSegment) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	return mode + pathSegment + s.statusBar.Render(strings.Repeat(" ", gap)) + right
}

// statusBarRight selects the right-aligned label. Copy-status wins
// over search count (transient feedback for a just-pressed c); search
// count wins over scroll-percent (query is the focused interaction);
// scroll-percent is the default.
func (m Model) statusBarRight() string {
	sb := styles().statusBar
	if m.copyStatus != "" {
		return sb.Render(" " + m.copyStatus + " ")
	}
	if m.query != "" {
		var label string
		if len(m.matches) == 0 {
			label = " Keine Treffer "
		} else {
			label = " " + strconv.Itoa(m.matchIdx+1) + "/" + strconv.Itoa(len(m.matches)) + " "
		}
		return sb.Render(label)
	}
	return sb.Render(" " + formatPercent(m.viewport.ScrollPercent()) + " ")
}

// bodyView returns the body slot: either the viewport's rendered
// content or a tinted error line when SetError has been called. The
// error path uses the same vertical slot as the body so chrome height
// math stays unchanged.
func (m Model) bodyView() string {
	if m.err != nil {
		return "\n  " + styles().err.Render("Fehler: "+m.err.Error())
	}
	return m.viewport.View()
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
