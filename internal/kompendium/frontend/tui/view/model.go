// Package view is kompendium's in-process full-screen Markdown viewer.
// It replaces the previous shell-out to glow on the browse view's `v`
// key. Renders via the shared internal/frontend/tui/markdown pipeline
// (goldmark + chroma + lipgloss), staying in the same Bubble Tea
// program so URLs render as OSC 8 (clickable in tmux + the prefix+u
// plugin) and the chrome matches the rest of the TUI.
package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/serverkraken/flow/internal/frontend/tui/markdown"
	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
	"github.com/serverkraken/flow/internal/ports"
)

// ExitMsg is the signal the viewer sends when the user dismisses it.
// The hosting Bubble Tea model (browse) returns to its normal mode on
// receipt; the viewer never knows what triggered the exit.
type ExitMsg struct{}

func exitCmd() tea.Cmd { return func() tea.Msg { return ExitMsg{} } }

// Mode is the viewer's input mode.
type Mode int

const (
	// ModeNormal hosts scroll + search-launch keys.
	ModeNormal Mode = iota
	// ModeSearch focuses the textinput; Enter applies the query and
	// returns to ModeNormal, Esc cancels without changing matches.
	ModeSearch
)

// Model is the viewer's Bubble Tea state. It is constructed fresh for
// each `v` press so leftover scroll position / search query never leak
// across notes.
type Model struct {
	title  string
	source string

	resolver    ports.WikilinkResolver
	frontmatter *domain.Frontmatter
	backlinks   []usecase.BacklinkRef

	rendered string
	lines    []string
	plain    []string

	viewport viewport.Model
	width    int
	height   int

	mode     Mode
	search   textinput.Model
	query    string
	matches  []int
	matchIdx int

	// Code-snippet copy state. snippets holds the fenced code blocks
	// extracted from the source at New() time. copyIdx is the
	// 0-indexed snippet `c` will copy on the next press; cycles.
	// copyStatus surfaces in the status bar for ~2s after a `c`
	// press, then a clearCopyStatusMsg wipes it.
	snippets   []codeSnippet
	copyIdx    int
	copyStatus string

	keys keyMap
}

// New returns a fresh viewer rendering source as Markdown with the
// kompendium theme. title is shown in the header. resolver may be
// nil — wikilinks then render as broken (red marker, no OSC 8).
// frontmatter may be nil — the renderer skips the card in that case.
// backlinks may be empty — the renderer skips the footer in that
// case. SetSize must be called before View() yields useful output.
func New(title, source string, resolver ports.WikilinkResolver, frontmatter *domain.Frontmatter, backlinks []usecase.BacklinkRef) Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 256
	ti.Cursor.Style = cursorStyle
	return Model{
		title:       title,
		source:      source,
		resolver:    resolver,
		frontmatter: frontmatter,
		backlinks:   backlinks,
		snippets:    extractCodeSnippets(source),
		viewport:    viewport.New(0, 0),
		search:      ti,
		keys:        defaultKeys(),
	}
}

// Init satisfies the tea.Model contract; the viewer has no startup work.
func (m Model) Init() tea.Cmd { return nil }

// SetSize re-flows the rendered Markdown to the new screen size. Call
// on every tea.WindowSizeMsg.
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height
	return m.rerender()
}

// chrome budget. Two horizontal numbers because lipgloss splits padding
// vs. border when sizing a styled block: content lines are budgeted
// against contentLineBudget (border + padding), but Style.Width()
// includes the padding already and only the border has to be added on
// top — so frameWidthOffset is the smaller value passed to .Width(N).
// Mismatched the two and the status bar wrapped onto a second row.
const (
	chromeVertical    = 2 + 4 // border top+bottom + four inner chrome rows
	contentLineBudget = 2 + 2 // border + padding both sides; max width per content line
	frameWidthOffset  = 2     // border only; argument to lipgloss.Style.Width()
	gutterWidth       = 2     // reserved for the search match bar
)

// contentSize returns the inner width Glamour reflows to and the
// vertical lines available to the viewport. Negative results signal
// "too small to render usefully" and the caller skips body building.
func (m Model) contentSize() (int, int) {
	innerW := m.width - contentLineBudget - gutterWidth
	innerH := m.height - chromeVertical
	if innerW < 1 || innerH < 1 {
		return 0, 0
	}
	return innerW, innerH
}

// rerender (re)builds the rendered body from m.source for the current
// width and resets the viewport content. Search state is preserved but
// match indices are recomputed because line numbering changed.
func (m Model) rerender() Model {
	innerW, innerH := m.contentSize()
	if innerW <= 0 || innerH <= 0 {
		m.rendered = ""
		m.lines = nil
		m.plain = nil
		m.viewport.Width = 0
		m.viewport.Height = 0
		return m
	}
	var opts []markdown.Option
	if m.resolver != nil {
		opts = append(opts, markdown.WithWikilinks(m.resolver))
	}
	if m.frontmatter != nil {
		opts = append(opts, markdown.WithFrontmatter(frontmatterToMarkdown(m.frontmatter)))
	}
	if len(m.backlinks) > 0 {
		opts = append(opts, markdown.WithBacklinks(backlinksToMarkdown(m.backlinks)))
	}
	out, _ := markdown.Render(m.source, innerW, opts...)
	m.rendered = out
	m.lines = strings.Split(out, "\n")
	m.plain = make([]string, len(m.lines))
	for i, l := range m.lines {
		m.plain[i] = strings.ToLower(ansi.Strip(l))
	}
	// viewport width spans gutter + content so prepended bars don't
	// re-wrap the line; it knows to ignore the gutter for its own math
	// because every line carries the same 2-cell prefix.
	m.viewport.Width = innerW + gutterWidth
	m.viewport.Height = innerH
	if m.query != "" {
		m = m.recomputeMatches()
	}
	m.viewport.SetContent(m.composeContent())
	return m
}

// composeContent prepends the gutter (and, when a query is active, the
// match bar) to every line so the viewport sees lines of consistent
// shape. The current match gets matchCurrentBarStyle, other matches
// matchBarStyle, non-matches a plain two-space gutter.
func (m Model) composeContent() string {
	if len(m.lines) == 0 {
		return ""
	}
	if len(m.matches) == 0 {
		// Still prepend gutter so the layout doesn't jump when the
		// user runs a no-match search and then reverts to no query.
		out := make([]string, len(m.lines))
		for i, l := range m.lines {
			out[i] = emptyLineMarker + l
		}
		return strings.Join(out, "\n")
	}
	matchSet := make(map[int]struct{}, len(m.matches))
	for _, i := range m.matches {
		matchSet[i] = struct{}{}
	}
	cur := -1
	if m.matchIdx >= 0 && m.matchIdx < len(m.matches) {
		cur = m.matches[m.matchIdx]
	}
	bar := matchBarStyle.Render("▎ ")
	curBar := matchCurrentBarStyle.Render("▎ ")
	out := make([]string, len(m.lines))
	for i, l := range m.lines {
		if i == cur {
			out[i] = curBar + l
			continue
		}
		if _, ok := matchSet[i]; ok {
			out[i] = bar + l
			continue
		}
		out[i] = emptyLineMarker + l
	}
	return strings.Join(out, "\n")
}

// Update is the viewer's reducer. Returns the updated model + any cmd;
// the caller (browse) routes messages here whenever browse is in
// ModeView.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.SetSize(msg.Width, msg.Height), nil
	case clearCopyStatusMsg:
		m.copyStatus = ""
		return m, nil
	case tea.KeyMsg:
		if m.mode == ModeSearch {
			return m.handleSearchKey(msg)
		}
		return m.handleNormalKey(msg)
	}
	if m.mode != ModeSearch {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleNormalKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, exitCmd()
	case key.Matches(msg, m.keys.Search):
		m.mode = ModeSearch
		m.search.SetValue("")
		m.search.Focus()
		return m, textinput.Blink
	case key.Matches(msg, m.keys.NextMatch):
		return m.cycleMatch(+1), nil
	case key.Matches(msg, m.keys.PrevMatch):
		return m.cycleMatch(-1), nil
	case key.Matches(msg, m.keys.Top):
		m.viewport.GotoTop()
		return m, nil
	case key.Matches(msg, m.keys.Bottom):
		m.viewport.GotoBottom()
		return m, nil
	case key.Matches(msg, m.keys.CopyCode):
		updated, payload := m.copyNextSnippet()
		if payload == "" {
			return updated, clearCopyStatusCmd()
		}
		// Write the OSC 52 directly to stdout AND schedule the
		// status-bar clear. The bubbletea diff-renderer otherwise
		// strips the sequence as a non-displayable fragment, so the
		// payload never reaches the terminal.
		return updated, tea.Batch(writeClipboardCmd(payload), clearCopyStatusCmd())
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// copyNextSnippet picks the next snippet in cycle order, sets the
// status-bar message describing what was copied, and returns the
// snippet body for the caller to push to the clipboard via
// writeClipboardCmd. Empty payload signals there's nothing to copy
// (the status bar still says so).
func (m Model) copyNextSnippet() (Model, string) {
	if len(m.snippets) == 0 {
		m.copyStatus = "Keine Code-Blöcke zum Kopieren."
		return m, ""
	}
	if m.copyIdx >= len(m.snippets) {
		m.copyIdx = 0
	}
	snip := m.snippets[m.copyIdx]
	label := snip.lang
	if label == "" {
		label = "Code"
	}
	m.copyStatus = fmt.Sprintf("Kopiert: %s %d/%d", label, m.copyIdx+1, len(m.snippets))
	m.copyIdx++
	return m, snip.body
}

// clearCopyStatusMsg fires ~2s after a `c` press so the status-bar
// confirmation ("copied terraform 2/3") fades on its own instead of
// camping in the bar until the next event.
type clearCopyStatusMsg struct{}

func clearCopyStatusCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return clearCopyStatusMsg{} })
}

func (m Model) handleSearchKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = ModeNormal
		m.search.Blur()
		return m, nil
	case tea.KeyEnter:
		m.mode = ModeNormal
		m.search.Blur()
		return m.applyQuery(strings.TrimSpace(m.search.Value())), nil
	}
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	return m, cmd
}

// applyQuery rebuilds matches for the new lowered query, jumps to the
// first hit, and refreshes the viewport content so the bar gutter is
// up to date. Empty query clears highlights without dropping scroll.
func (m Model) applyQuery(q string) Model {
	m.query = strings.ToLower(q)
	m = m.recomputeMatches()
	m.viewport.SetContent(m.composeContent())
	m.scrollToCurrent()
	return m
}

// recomputeMatches scans m.plain for the current query. Returns a model
// with .matches populated (or empty when the query is empty / has no
// hits) and .matchIdx clamped to a sane index. Caller is responsible
// for refreshing viewport content + scroll.
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

// cycleMatch advances the match cursor by delta (wrapping around) and
// scrolls to the new current match. No-op when there are no matches.
func (m Model) cycleMatch(delta int) Model {
	if len(m.matches) == 0 {
		return m
	}
	m.matchIdx = (m.matchIdx + delta + len(m.matches)) % len(m.matches)
	m.viewport.SetContent(m.composeContent())
	m.scrollToCurrent()
	return m
}

// scrollToCurrent positions the viewport so the current match sits a
// few lines below the top — leaves visible context above the hit
// rather than dropping the user right onto it.
func (m *Model) scrollToCurrent() {
	if len(m.matches) == 0 {
		return
	}
	line := m.matches[m.matchIdx]
	target := line - m.viewport.Height/3
	if target < 0 {
		target = 0
	}
	m.viewport.SetYOffset(target)
}

// View renders the viewer chrome + viewport into a single string sized
// to (m.width, m.height). Returns "" when the screen is too small for
// useful chrome (< chromeHorizontal+1 cells wide / chromeVertical+1
// tall).
func (m Model) View() string {
	if m.width <= contentLineBudget || m.height <= chromeVertical {
		return ""
	}
	lineW := m.width - contentLineBudget
	if lineW < 1 {
		lineW = 1
	}
	title := titleStyle.Render(m.title)
	sep := separatorStyle.Render(strings.Repeat("─", lineW))
	body := m.viewport.View()
	footer := m.renderFooter()
	statusBar := m.renderStatusBar()

	content := lipgloss.JoinVertical(lipgloss.Left, title, sep, body, footer, statusBar)
	// Match browse.frameContent's manual padding: the rounded border
	// otherwise stops right at the status bar and leaves a half-blank
	// pane on tall windows.
	target := m.height - 2 // border top+bottom
	if got := strings.Count(content, "\n") + 1; target > 0 && got < target {
		content += strings.Repeat("\n", target-got)
	}
	return frameStyle.Width(m.width - frameWidthOffset).Render(content)
}

func (m Model) renderFooter() string {
	if m.mode == ModeSearch {
		view := m.search.View()
		if view == "" {
			view = "▎"
		}
		return searchActiveLabelStyle.Render("Suche:") + " " + view +
			"   " + footerStyle.Render("Enter → übernehmen  ·  Esc → abbrechen")
	}
	hint := footerStyle.Render("j/k → scrollen  ·  ") +
		footerKeyStyle.Render("/") + footerStyle.Render(" → suchen  ·  ")
	if len(m.snippets) > 0 {
		hint += footerKeyStyle.Render("c") + footerStyle.Render(" → Code kopieren  ·  ")
	}
	hint += footerKeyStyle.Render("q") + footerStyle.Render(" → zurück")
	return hint
}

// renderStatusBar mirrors browse's status-bar shape: optional mode
// badge on the left, title path in the middle, match counter on the
// right.
func (m Model) renderStatusBar() string {
	innerW := m.width - contentLineBudget
	if innerW <= 0 {
		return ""
	}
	mode := ""
	if m.mode == ModeSearch {
		mode = statusBarModeSearchStyle.Render("SEARCH")
	}
	meta := m.statusBarMeta()
	title := m.title
	if title == "" {
		title = "—"
	}
	consumed := lipgloss.Width(mode) + lipgloss.Width(meta) + 2
	avail := innerW - consumed
	if avail < 5 {
		avail = 5
	}
	if lipgloss.Width(title) > avail {
		title = truncate(title, avail)
	}
	pathSegment := statusBarPathStyle.Render(" " + title)
	gap := innerW - lipgloss.Width(mode) - lipgloss.Width(pathSegment) - lipgloss.Width(meta)
	if gap < 0 {
		gap = 0
	}
	return mode + pathSegment + statusBarStyle.Render(strings.Repeat(" ", gap)) + meta
}

// statusBarMeta is the right-aligned tail. Match counter when a
// query is active; copy-confirmation when a `c` press just landed
// (transient, cleared on the next non-copy event); otherwise empty.
// Copy status wins over search count — the user just initiated it
// and wants the immediate feedback.
func (m Model) statusBarMeta() string {
	if m.copyStatus != "" {
		return statusBarStyle.Render(" " + m.copyStatus + " ")
	}
	if m.query == "" {
		return ""
	}
	var label string
	if len(m.matches) == 0 {
		label = fmt.Sprintf(" Keine Treffer für %q ", m.query)
	} else {
		label = fmt.Sprintf(" %d/%d %q ", m.matchIdx+1, len(m.matches), m.query)
	}
	return statusBarStyle.Render(label)
}

// truncate clips s to width-1 cells and appends "…". ANSI-aware via
// ansi.Strip-then-rebuild would be over-engineered here — the title is
// a plain note ID with no escape sequences.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	if width <= 1 {
		return "…"
	}
	rs := []rune(s)
	if len(rs) <= width-1 {
		return string(rs) + "…"
	}
	return string(rs[:width-1]) + "…"
}

// CurrentMode returns the model's input mode. Exposed for tests +
// status-bar rendering by the host model.
func (m Model) CurrentMode() Mode { return m.mode }

// Query returns the most recently submitted lowered query. Exposed for
// tests.
func (m Model) Query() string { return m.query }

// Matches returns the line indices that matched the current query.
// Exposed for tests.
func (m Model) Matches() []int { return m.matches }

// MatchIndex returns the cursor's position in Matches(). Exposed for
// tests.
func (m Model) MatchIndex() int { return m.matchIdx }
