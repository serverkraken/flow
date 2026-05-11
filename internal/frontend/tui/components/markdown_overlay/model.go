package markdown_overlay

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// Chrome budget. Two horizontal numbers because lipgloss splits padding
// vs. border when sizing a styled block. contentLineBudget is the room
// the border + padding take per line; frameWidthOffset is the slimmer
// number passed to lipgloss.Style.Width (border only). gutterWidth
// reserves the two cells the match-bar prefix occupies in search mode.
const (
	chromeVertical    = 2 + 4 // border top+bottom + title + sep + footer + statusBar
	contentLineBudget = 2 + 2 // border + padding both sides
	frameWidthOffset  = 2     // border-only; argument to lipgloss.Style.Width
	gutterWidth       = 2     // reserved for the search match bar
)

// Model is the markdown overlay's bubbletea model. Construct via New;
// configure dimensions via SetSize after WindowSizeMsg; route messages
// via Update; render via View. Emits ExitMsg when the user hits a
// configured close key (added in a later task).
type Model struct {
	cfg    config
	render RenderFunc

	width  int
	height int

	rendered string
	viewport viewport.Model
}

// New constructs a Model. render must not be nil — a nil RenderFunc is
// a wiring bug, not a runtime fallback.
func New(render RenderFunc, opts ...Option) Model {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	return Model{cfg: cfg, render: render}
}

// Init satisfies the bubbletea Model contract; the overlay has no
// startup work.
func (m Model) Init() tea.Cmd { return nil }

// Update returns the model unchanged at this stage. Subsequent tasks
// extend this with WindowSizeMsg/KeyMsg/search/code-copy routing.
func (m Model) Update(_ tea.Msg) (Model, tea.Cmd) { return m, nil }

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
// inner width and pushes the result into the viewport. Called from
// SetSize and SetSource.
func (m Model) rerender() Model {
	innerW, innerH := m.contentSize()
	if innerW <= 0 || innerH <= 0 {
		m.rendered = ""
		m.viewport.Width = 0
		m.viewport.Height = 0
		return m
	}
	m.rendered = m.render(m.cfg.source, innerW)
	m.viewport.Width = innerW + gutterWidth
	m.viewport.Height = innerH
	m.viewport.SetContent(m.rendered)
	return m
}
