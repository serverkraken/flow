package markdown_overlay

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

// Chrome budget. Two horizontal numbers because lipgloss splits padding
// vs. border when sizing a styled block. contentLineBudget is the room
// the border + padding take per line; frameWidthOffset is the slimmer
// number passed to lipgloss.Style.Width (border only). gutterWidth
// reserves the two cells the match-bar prefix occupies in search mode.
// chromeVertical = 2 (border top+bottom) + 1 title + 1 separator
// + 1 footer + 1 status bar.
const (
	chromeVertical    = 6
	contentLineBudget = 4 // 2 border + 2 padding
	frameWidthOffset  = 2 // border-only; argument to lipgloss.Style.Width
	gutterWidth       = 2 // reserved for the search match bar
)

// Model is the markdown overlay's bubbletea model. Construct via New;
// configure dimensions via SetSize after WindowSizeMsg; route messages
// via Update; render via View. Emits ExitMsg when the user hits a
// configured close key.
type Model struct {
	cfg    config
	render RenderFunc

	width  int
	height int

	rendered string
	viewport viewport.Model

	// search state (used only when cfg.enableSearch).
	mode     Mode
	search   textinput.Model
	query    string
	matches  []int
	matchIdx int
	lines    []string
	plain    []string

	// code-copy state (used only when cfg.enableCodeCopy).
	snippets   []codeSnippet
	copyIdx    int
	copyStatus string

	// err displaces the body when non-nil. Set via SetError, cleared by
	// any subsequent SetSource (so a successful re-load wipes a prior
	// failure surface).
	err error

	keys keyMap
}

// New constructs a Model. render must not be nil — a nil RenderFunc is
// a wiring bug, not a runtime fallback.
func New(render RenderFunc, opts ...Option) Model {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	return Model{
		cfg:    cfg,
		render: render,
		search: newSearchInput(),
		keys:   defaultKeys(),
	}
}

// Init satisfies the bubbletea Model contract; the overlay has no
// startup work.
func (m Model) Init() tea.Cmd { return nil }

// ExitMsg is emitted when the user hits a configured close key. The
// host model must observe it in its own Update and clear its overlay
// state field; the overlay does not know what triggered its presence
// and therefore cannot un-mount itself.
type ExitMsg struct{}

func exitCmd() tea.Cmd { return func() tea.Msg { return ExitMsg{} } }

// Update routes incoming messages. WindowSizeMsg re-flows the body;
// KeyMsg in ModeSearch routes to the textinput; KeyMsg otherwise
// dispatches search-launch, match-cycle, close-key (emit ExitMsg)
// and finally falls through to the viewport.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.SetSize(msg.Width, msg.Height), nil
	case tea.KeyPressMsg:
		if m.mode == ModeSearch {
			return m.handleSearchKey(msg)
		}
		if updated, cmd, handled := m.maybeEnterSearch(msg); handled {
			return updated, cmd
		}
		switch {
		case m.cfg.enableSearch && key.Matches(msg, m.keys.NextMatch):
			return m.cycleMatch(+1), nil
		case m.cfg.enableSearch && key.Matches(msg, m.keys.PrevMatch):
			return m.cycleMatch(-1), nil
		case key.Matches(msg, m.keys.Top):
			m.viewport.GotoTop()
			return m, nil
		case key.Matches(msg, m.keys.Bottom):
			m.viewport.GotoBottom()
			return m, nil
		case m.cfg.enableCodeCopy && key.Matches(msg, m.keys.CopyCode):
			updated, payload := m.copyNextSnippet()
			if payload == "" {
				return updated, clearCopyStatusCmd()
			}
			return updated, tea.Batch(writeClipboardCmd(payload), clearCopyStatusCmd())
		}
		if m.isCloseKey(msg) {
			return m, exitCmd()
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	case clearCopyStatusMsg:
		m.copyStatus = ""
		return m, nil
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m Model) isCloseKey(msg tea.KeyPressMsg) bool {
	s := msg.String()
	for _, k := range m.cfg.closeKeys {
		if s == k {
			return true
		}
	}
	return false
}

// View renders the chrome (frame + title + separator + body + footer +
// status bar) sized to (m.width, m.height). Returns "" when the screen
// is too small for useful chrome.
func (m Model) View() string {
	if m.width <= contentLineBudget || m.height <= chromeVertical {
		return ""
	}
	return m.renderChrome()
}

// contentSize returns the inner content width (markdown reflow target)
// and the inner content height (viewport vertical capacity). Returns
// (0,0) when the outer size is too small to render anything useful.
func (m Model) contentSize() (int, int) {
	innerW := m.width - contentLineBudget - gutterWidth
	innerH := m.height - chromeVertical
	if innerW < 1 || innerH < 1 {
		return 0, 0
	}
	return innerW, innerH
}

// rerender re-flows the body through the RenderFunc at the current
// inner width, refreshes the search line cache, recomputes match
// positions if a query is active, and pushes the gutter-prepended
// content into the viewport. Called from SetSize and SetSource.
func (m Model) rerender() Model {
	innerW, innerH := m.contentSize()
	if innerW <= 0 || innerH <= 0 {
		m.rendered = ""
		m.viewport.SetWidth(0)
		m.viewport.SetHeight(0)
		return m
	}
	m.rendered = m.render(m.cfg.source, innerW)
	m = m.refreshLineCache()
	if m.cfg.enableCodeCopy {
		m.snippets = extractCodeSnippets(m.cfg.source)
	}
	if m.query != "" {
		m = m.recomputeMatches()
	}
	m.viewport.SetWidth(innerW + gutterWidth)
	m.viewport.SetHeight(innerH)
	m.viewport.SetContent(m.composeContent())
	return m
}
