// Package worktime is the multi-tab worktime screen — port-driven
// successor to internal/screen/worktime.
//
// The root Model holds four sub-models (Heute / Woche / History /
// Frei) in a fixed-index array, with tab switching, an adaptive ticker
// (1 s for the first minute of an active session, then 10 s) and a
// lightweight dayRefreshMsg. The four sub-models live in their own
// files: today.go (Heute), week.go (Woche), history.go (History),
// dayoffs.go (Frei).
package worktime

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
	"github.com/serverkraken/flow/internal/frontend/tui/components/titlebox"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// stateRestorer is the optional contract a sub-model implements when
// it can restore its (filter, cursor) state from persistence. Mirrors
// sidekick's stateRestorer shape — duplicated so this package stays
// self-contained.
type stateRestorer interface {
	WithState(filter string, cursor int) tea.Model
}

// Deps bundles every use case the worktime screen and its sub-models
// consume. Wired by the composition root and threaded into all four
// sub-models so they never reach for I/O directly.
type Deps struct {
	Reader        *usecase.WorktimeReader
	Stats         *usecase.StatsComputer
	SessionWriter *usecase.SessionWriter
	Tagger        *usecase.Tagger
	DayOffReader  *usecase.DayOffReader
	DayOffWriter  *usecase.DayOffWriter
	LinkReader    *usecase.LinkReader
	LinkWriter    *usecase.LinkWriter
	Reporter      *usecase.Reporter
	NoteOpener    *usecase.NoteOpener
	Clock         interface{ Now() time.Time }
	// Output is the worktime menu's three-target sink (Clipboard /
	// tmux-Split / Datei in ~/Downloads). Wired in cmd/flow/main.go via
	// internal/adapter/output. Slice B: nil-tolerant (no flow uses it
	// yet); Slice C/D/E start dispatching Brief / Export / Stats output
	// through this port.
	Output ports.Output
}

// tab identifies one of the four worktime sub-screens.
type tab int

const (
	tabHeute   tab = 0
	tabWoche   tab = 1
	tabHistory tab = 2
	tabFrei    tab = 3
)

// Internal messages.

// tickMsg drives the adaptive ticker. The interval is
// fast (1 s) for the first minute of an active session, then slow
// (10 s) so a long-running tracker doesn't burn CPU.
type tickMsg time.Time

// dayRefreshMsg is the lightweight per-tick day reload — only the
// today snapshot is reloaded, not the heavier weekly / history /
// kompendium calls.
type dayRefreshMsg struct{}

const (
	tickFast = 1 * time.Second
	tickSlow = 10 * time.Second
)

// Model is the root bubbletea model for the worktime screen.
type Model struct {
	pal     theme.Palette
	deps    Deps
	width   int
	height  int
	current tab
	subs    [4]tea.Model
	menu    menuModel
}

// New constructs the worktime root model with the four sub-models
// wired against the given Deps.
func New(p theme.Palette, deps Deps) Model {
	return Model{
		pal:  p,
		deps: deps,
		subs: [4]tea.Model{
			newHeute(p, deps),
			newWoche(p, deps),
			newHistory(p, deps),
			newFrei(p, deps),
		},
		menu: newMenuModel(p, deps),
	}
}

// WithState restores the persisted tab selection (parsed from filter,
// shape "tab=NAME[|sub-filter]") and forwards the persisted cursor +
// the sub-filter half to the active sub-model when that sub-model
// supports state restoration. Called by the sidekick root after
// constructing the model.
func (m Model) WithState(filter string, cursor int) tea.Model {
	subFilter := ""
	if filter != "" {
		head, rest, hasRest := strings.Cut(filter, "|")
		if rest != "" || hasRest {
			subFilter = rest
		}
		if name, ok := strings.CutPrefix(head, "tab="); ok {
			if t, ok := parseTabName(name); ok {
				m.current = t
			}
		}
	}
	if sr, ok := m.subs[m.current].(stateRestorer); ok {
		m.subs[m.current] = sr.WithState(subFilter, cursor)
	}
	return m
}

// FilterActive returns whether either the action menu or the active
// sub-model is currently consuming text input. The Worktime root,
// sidekick parent, and tab-switching keys all check this before
// claiming letter keys back.
func (m Model) FilterActive() bool {
	if m.menu.Active() {
		return true
	}
	if fa, ok := m.subs[m.current].(filterActiver); ok {
		return fa.FilterActive()
	}
	return false
}

// StateFilter returns the persisted state for the worktime screen.
// Encodes the active tab as "tab=N" plus, when the active sub-model
// itself carries a filter, the sub-model's own filter via "tab=N|<f>".
// WithState parses this back into (tab, filter) for restoration.
func (m Model) StateFilter() string {
	tabPart := "tab=" + tabName(m.current)
	if fa, ok := m.subs[m.current].(filterActiver); ok {
		if f := fa.StateFilter(); f != "" {
			return tabPart + "|" + f
		}
	}
	return tabPart
}

// tabName returns a stable string identifier for t — used by
// StateFilter so persisted state survives a tab-index renumbering.
func tabName(t tab) string {
	switch t {
	case tabHeute:
		return "heute"
	case tabWoche:
		return "woche"
	case tabHistory:
		return "history"
	case tabFrei:
		return "frei"
	}
	return "heute"
}

// parseTabName is the inverse of tabName.
func parseTabName(s string) (tab, bool) {
	switch s {
	case "heute":
		return tabHeute, true
	case "woche":
		return tabWoche, true
	case "history":
		return tabHistory, true
	case "frei":
		return tabFrei, true
	}
	return tabHeute, false
}

// StateCursor returns the active sub-model's cursor for state
// persistence. Each tab persists its own cursor shape (Heute's row,
// Woche's day, History's drill index, Frei's row). The active tab is
// encoded in StateFilter so WithState can restore both halves.
func (m Model) StateCursor() int {
	if cs, ok := m.subs[m.current].(cursorStater); ok {
		return cs.StateCursor()
	}
	return 0
}

// HandlesBack tells the parent app that this screen consumes the global
// `b` key for tab cycling — only when the user is on a non-default tab.
// On tabHeute we let `b` fall through to the palette switch.
func (m Model) HandlesBack() bool { return m.current != tabHeute }

// ConsumesKeys lists letter / punctuation keys the active sub-model and
// the worktime-root menu claim, so the sidekick's global navigation
// (p / n / f / w / c) doesn't intercept keys the worktime surface itself
// binds. `:` is always claimed because the action menu lives at the
// root and must open from any tab.
func (m Model) ConsumesKeys() []string {
	keys := []string{":"}
	if kc, ok := m.subs[m.current].(interface{ ConsumesKeys() []string }); ok {
		keys = append(keys, kc.ConsumesKeys()...)
	}
	return keys
}

// Init starts every sub-model concurrently and schedules the first
// tick.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.scheduleTick()}
	for _, s := range m.subs {
		if cmd := s.Init(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

// Update routes messages to the active sub-model and handles the
// global tab keys + tick. When the action menu is open, all keys go
// to the menu and tab-switching is suspended.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.menu = m.menu.SetSize(msg.Width, msg.Height)
		var cmds []tea.Cmd
		for i, s := range m.subs {
			updated, cmd := s.Update(msg)
			m.subs[i] = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)

	case tickMsg:
		// Forward the tick to the active sub-model as a dayRefreshMsg —
		// the heavy snapshot reloads stay scoped to the visible tab.
		var cmds []tea.Cmd
		updated, cmd := m.subs[m.current].Update(dayRefreshMsg{})
		m.subs[m.current] = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		cmds = append(cmds, m.scheduleTick())
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	}

	// Async messages (loadedMsg variants from sub-models, toast
	// dismiss ticks for the menu) are dispatched to every sub-screen
	// PLUS the menu so each picks up the ones it owns. Recipients
	// drop messages they don't recognise.
	var cmds []tea.Cmd
	for i, s := range m.subs {
		updated, cmd := s.Update(msg)
		m.subs[i] = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if m.menu.Active() {
		updated, cmd := m.menu.Update(msg)
		m.menu = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return m, tea.Batch(cmds...)
}

// handleKeyMsg dispatches a key when no async / tick / window message
// is in flight. Order:
//  1. q is the universal exit key — returns tea.Quit from any sub-
//     mode (menu, dialog, picker, help overlay) UNLESS a textinput is
//     currently focused; in that case 'q' is a literal letter the user
//     wants in their tag / note / range / HH:MM input.
//  2. Menu owns input while open.
//  3. Tab-router keys (1/2/3/4/Tab/b/`:`) when no dialog/menu blocks.
//  4. Forward everything else to the active sub-model.
//
// Split off Update to keep cyclomatic complexity inside the project
// budget.
func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "q" && !m.textInputActive() {
		return m, tea.Quit
	}
	if m.menu.Active() {
		updated, cmd := m.menu.Update(msg)
		m.menu = updated
		return m, cmd
	}
	if !m.FilterActive() {
		if next, ok := m.handleTabRouterKey(msg); ok {
			return next, nil
		}
	}
	updated, cmd := m.subs[m.current].Update(msg)
	m.subs[m.current] = updated
	return m, cmd
}

// handleTabRouterKey handles the global tab-switching keys plus the
// `:` action-menu trigger. Returns (model, true) when the key was
// claimed; (zero, false) lets the caller forward the key to the active
// sub-model.
func (m Model) handleTabRouterKey(msg tea.KeyMsg) (Model, bool) {
	switch msg.String() {
	case ":":
		m.menu = m.menu.openMenu(m.current)
		return m, true
	case "1":
		m.current = tabHeute
		return m, true
	case "2":
		m.current = tabWoche
		return m, true
	case "3":
		m.current = tabHistory
		return m, true
	case "4":
		m.current = tabFrei
		return m, true
	case "tab":
		m.current = (m.current + 1) % 4
		return m, true
	case "b":
		if m.current != tabHeute {
			m.current = (m.current + 3) % 4 // -1 mod 4
			return m, true
		}
	}
	return Model{}, false
}

// View renders the active sub-model with a tab strip on top. When the
// action menu is open it replaces the tab body — the tab strip stays
// so the user keeps the visual anchor of which tab they came from.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	var body string
	if m.menu.Active() {
		body = m.menu.View()
	} else {
		body = m.subs[m.current].View()
		if body == "" {
			body = theme.Dim("  (lädt …)", m.pal)
		}
	}
	return titlebox.Render(m.tabStrip(m.width), body, m.width, m.pal)
}

// tabStrip renders the four-tab navigation. Three-step degradation keeps
// the strip inside the titlebox budget on narrow tmux panes: full labels
// with "  ·  " spacing → compact "·" separators → single-char fallback
// ("H · W · Hi · F"). titlebox.Render needs at most width-5 chars in the
// title; anything wider pushes the corner past the right edge.
func (m Model) tabStrip(width int) string {
	labels := []string{"Heute", "Woche", "History", "Frei"}
	short := []string{"H", "W", "Hi", "F"}
	budget := width - 5
	if budget < 1 {
		budget = 1
	}
	for _, opt := range []struct {
		labels []string
		sep    string
	}{
		{labels, "  ·  "},
		{labels, " · "},
		{short, " · "},
	} {
		if out := m.renderTabs(opt.labels, opt.sep); lipgloss.Width(out) <= budget {
			return out
		}
	}
	return m.renderTabs(short, " ")
}

func (m Model) renderTabs(labels []string, sep string) string {
	out := ""
	for i, l := range labels {
		if i > 0 {
			out += theme.Dim(sep, m.pal)
		}
		if tab(i) == m.current {
			out += theme.Heading(l, m.pal)
		} else {
			out += theme.Dim(l, m.pal)
		}
	}
	return out
}

// scheduleTick returns a tea.Cmd that fires after the adaptive
// interval. Fast (1 s) when the active sub-model reports it via
// FastTick (e.g. Heute during the first minute of a running session);
// slow (10 s) otherwise.
func (m Model) scheduleTick() tea.Cmd {
	d := tickSlow
	if ft, ok := m.subs[m.current].(fastTicker); ok && ft.FastTick(time.Now()) {
		d = tickFast
	}
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// — interfaces sub-models can opt into —

type filterActiver interface {
	FilterActive() bool
	StateFilter() string
}

type cursorStater interface {
	StateCursor() int
}

// fastTicker lets a sub-model opt into the fast (1 s) tick interval
// while it has a freshly-started session whose seconds-counter is
// visible to the user. Returning false drops to the slow tick.
type fastTicker interface {
	FastTick(now time.Time) bool
}

// textInputActiver lets a sub-model report whether a textinput.Model is
// currently focused — i.e. the user is typing free-form text into a
// field and 'q' should land in the field, not quit the program.
//
// Sub-models that don't implement this default to "no text input
// active" — q in those contexts (Heute idle, Woche, History list,
// menu list / target / land) returns tea.Quit at the worktime root.
type textInputActiver interface {
	TextInputActive() bool
}

// textInputActive aggregates the menu's and the active sub-model's
// text-input state. The worktime root checks this before honouring
// q-as-quit so typing 'q' inside a tag / note / range form / etc.
// edits the field instead of quitting.
func (m Model) textInputActive() bool {
	if m.menu.Active() && m.menu.TextInputActive() {
		return true
	}
	if ti, ok := m.subs[m.current].(textInputActiver); ok {
		return ti.TextInputActive()
	}
	return false
}
