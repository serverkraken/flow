package markdown_overlay

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
)

// Mode is the overlay's input mode. ModeSearch is reachable only when
// WithSearch is set; otherwise the textinput stays Blurred and `/` is
// forwarded to the viewport.
type Mode int

const (
	// ModeNormal accepts scroll, search-launch, match-cycle, code-copy
	// and close keys.
	ModeNormal Mode = iota
	// ModeSearch focuses the textinput; Enter applies the query and
	// returns to ModeNormal, Esc cancels without changing matches.
	ModeSearch
)

// CurrentMode reports the live input mode. Exposed for host
// status-bar integration and tests.
func (m Model) CurrentMode() Mode { return m.mode }

// Query returns the last applied (lowered) search query. Empty when
// the user has not yet pressed Enter on a search.
func (m Model) Query() string { return m.query }

// Matches returns the rendered-line indices that matched the current
// query. Empty when no query is active or no matches were found.
func (m Model) Matches() []int { return m.matches }

// MatchIndex is the cursor's position in Matches (0-indexed). n / N
// cycle this index with wrap-around.
func (m Model) MatchIndex() int { return m.matchIdx }

func newSearchInput() textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 256
	tiStyles := ti.Styles()
	tiStyles.Cursor.Color = styles().cursor.GetForeground()
	tiStyles.Cursor.Shape = tea.CursorBar
	ti.SetStyles(tiStyles)
	return ti
}

// handleSearchKey runs while m.mode == ModeSearch. Enter applies the
// trimmed query (and returns to ModeNormal); Esc cancels without
// touching the query state; anything else is forwarded to the
// textinput.
func (m Model) handleSearchKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = ModeNormal
		m.search.Blur()
		return m, nil
	case "enter":
		m.mode = ModeNormal
		m.search.Blur()
		return m.applyQuery(strings.TrimSpace(m.search.Value())), nil
	}
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	return m, cmd
}

// maybeEnterSearch attempts to enter search-mode for the given KeyMsg.
// Returns (model, cmd, true) when the key was the search-launcher and
// the model is now in ModeSearch; (model, nil, false) otherwise.
func (m Model) maybeEnterSearch(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.cfg.enableSearch {
		return m, nil, false
	}
	if !key.Matches(msg, m.keys.Search) {
		return m, nil, false
	}
	m.mode = ModeSearch
	m.search.SetValue("")
	m.search.Focus()
	return m, textinput.Blink, true
}

// cycleMatch advances the match cursor by delta (wrapping around) and
// scrolls the viewport so the new current match sits a few lines below
// the top. No-op when there are no matches.
func (m Model) cycleMatch(delta int) Model {
	if len(m.matches) == 0 {
		return m
	}
	m.matchIdx = (m.matchIdx + delta + len(m.matches)) % len(m.matches)
	m.viewport.SetContent(m.composeContent())
	m = m.scrollToCurrent()
	return m
}

// applyQuery rebuilds matches for the new query, jumps to the first
// hit, and refreshes viewport content so the bar gutter is up to date.
// Empty query clears highlights without dropping scroll.
func (m Model) applyQuery(q string) Model {
	m.query = strings.ToLower(q)
	m = m.recomputeMatches()
	m.viewport.SetContent(m.composeContent())
	m = m.scrollToCurrent()
	return m
}

func (m Model) recomputeMatches() Model {
	m.matches = m.matches[:0]
	if m.query == "" {
		m.matchIdx = 0
		return m
	}
	for i, line := range m.plain {
		if strings.Contains(line, m.query) {
			m.matches = append(m.matches, i)
		}
	}
	if m.matchIdx >= len(m.matches) {
		m.matchIdx = 0
	}
	return m
}

// scrollToCurrent positions the viewport so the current match sits a
// few lines below the top. Leaves visible context above the hit rather
// than dropping the user right onto it.
func (m Model) scrollToCurrent() Model {
	if len(m.matches) == 0 {
		return m
	}
	line := m.matches[m.matchIdx]
	target := line - m.viewport.Height()/3
	if target < 0 {
		target = 0
	}
	m.viewport.SetYOffset(target)
	return m
}

// composeContent prepends the match gutter (or an empty gutter) to
// every rendered line so the viewport sees lines of consistent shape.
// Pre-pending two cells to every line keeps the wrap point identical
// whether or not a bar is rendered.
func (m Model) composeContent() string {
	if len(m.lines) == 0 {
		return ""
	}
	const empty = "  "
	if len(m.matches) == 0 {
		out := make([]string, len(m.lines))
		for i, l := range m.lines {
			out[i] = empty + l
		}
		return strings.Join(out, "\n")
	}
	s := styles()
	bar := s.matchBar.Render(glyphs.AccentBar + " ")
	curBar := s.matchCurrentBar.Render(glyphs.AccentBar + " ")
	cur := -1
	if m.matchIdx >= 0 && m.matchIdx < len(m.matches) {
		cur = m.matches[m.matchIdx]
	}
	matchSet := make(map[int]struct{}, len(m.matches))
	for _, i := range m.matches {
		matchSet[i] = struct{}{}
	}
	out := make([]string, len(m.lines))
	for i, l := range m.lines {
		switch i {
		case cur:
			out[i] = curBar + l
		default:
			if _, ok := matchSet[i]; ok {
				out[i] = bar + l
			} else {
				out[i] = empty + l
			}
		}
	}
	return strings.Join(out, "\n")
}

// refreshLineCache populates m.lines + m.plain from m.rendered. Search
// match scan walks m.plain (ANSI-stripped + lowered).
func (m Model) refreshLineCache() Model {
	m.lines = strings.Split(m.rendered, "\n")
	m.plain = make([]string, len(m.lines))
	for i, l := range m.lines {
		m.plain[i] = strings.ToLower(ansi.Strip(l))
	}
	return m
}
