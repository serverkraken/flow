package markdown_overlay

import tea "github.com/charmbracelet/bubbletea"

// Model is the markdown overlay's bubbletea model. Construct via New;
// configure dimensions via SetSize after WindowSizeMsg; route messages
// via Update; render via View. Emits ExitMsg when the user hits a
// configured close key.
type Model struct {
	cfg    config
	render RenderFunc
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

// Init satisfies the tea.Model contract; the overlay has no startup
// work.
func (m Model) Init() tea.Cmd { return nil }

// Update returns the model unchanged at this skeleton stage. Subsequent
// tasks extend this with WindowSizeMsg/KeyMsg/search/code-copy routing.
func (m Model) Update(_ tea.Msg) (Model, tea.Cmd) { return m, nil }

// View returns "" at this skeleton stage. Subsequent tasks add the
// viewport + chrome rendering.
func (m Model) View() string { return "" }
