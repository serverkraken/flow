// Package sidekick is the root bubbletea model for the `flow sidekick`
// TUI. It owns the five top-level screens (Palette, Projects, Worktime,
// Cheatsheet, Notes) as opaque tea.Models, routes messages between
// them, and snapshots the active screen's filter / cursor for
// persistence via ports.FlowStateStore.
//
// Screen models are constructed by the composition root and handed in
// via Deps; the sidekick has no knowledge of use cases, adapters, or any
// concrete screen package. Sibling-screen wiring lives in cmd/flow/main.go.
package sidekick

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/help"
	"github.com/serverkraken/flow/internal/frontend/tui/components/statusbar"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/palette"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

type screenID int

const (
	screenPalette    screenID = 0
	screenProjects   screenID = 1
	screenWorktime   screenID = 2
	screenCheatsheet screenID = 3
	screenNotes      screenID = 4
)

// screener is the extra interface every screen model satisfies on top
// of tea.Model. Used to forward keys when a screen has its own filter
// active and to snapshot UI state for persistence.
type screener interface {
	FilterActive() bool
	StateFilter() string
	StateCursor() int
}

// backHandler is implemented by screens that want to consume the global
// `b` key under specific conditions (e.g. cycling tabs inside the screen
// instead of jumping to the palette).
type backHandler interface {
	HandlesBack() bool
}

// keyConsumer is implemented by screens that want to claim specific
// letter keys away from the sidekick's global navigation. The sidekick
// asks each Update which keys the active screen wants right now —
// returning {"n","p"} from Worktime/Heute means `n` (kompendium-attach)
// and `p` (pause) reach the screen instead of switching to Notes /
// Palette. Without this mech, screen-internal bindings advertised in
// `?`-overlays are silently dead in sidekick mode.
type keyConsumer interface {
	ConsumesKeys() []string
}

// stateRestorer is satisfied by screens that persist a filter + cursor
// across runs. The active screen receives WithState(filter, cursor) so
// the snapshot from FlowStateStore is applied before its first render.
type stateRestorer interface {
	WithState(filter string, cursor int) tea.Model
}

// Deps bundles the five pre-built screen models. All fields are required
// — the composition root in cmd/flow/main.go is responsible for wiring
// every slot. nil is not a supported configuration; the sidekick does not
// fall back to legacy in-package constructors.
type Deps struct {
	Palette    tea.Model
	Projects   tea.Model
	Worktime   tea.Model
	Cheatsheet tea.Model
	Notes      tea.Model
}

// Model is the root bubbletea model.
type Model struct {
	screens  [5]tea.Model
	current  screenID
	showHelp bool
	pal      theme.Palette
	width    int
	height   int
}

// New creates the root model with the active screen restored from s.
// When s.Screen names a stateful screen and that screen implements
// stateRestorer, its filter / cursor are reapplied before first render.
func New(p theme.Palette, s domain.FlowState, deps Deps) Model {
	screens := [5]tea.Model{deps.Palette, deps.Projects, deps.Worktime, deps.Cheatsheet, deps.Notes}

	var cur screenID
	switch s.Screen {
	case domain.ScreenProjects:
		cur = screenProjects
	case domain.ScreenWorktime:
		cur = screenWorktime
	case domain.ScreenCheatsheet:
		cur = screenCheatsheet
	case domain.ScreenNotes:
		cur = screenNotes
	default:
		cur = screenPalette
	}
	if sr, ok := screens[cur].(stateRestorer); ok {
		screens[cur] = sr.WithState(s.Filter, s.Cursor)
	}

	return Model{screens: screens, current: cur, pal: p}
}

// Init starts all four screens concurrently so they load in the background.
func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	for _, s := range m.screens {
		if cmd := s.Init(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

// Update routes messages to the active screen or handles global navigation keys.
//
// The function is a flat dispatch table over a fixed set of message
// types. Key handling lives in handleKeyMsg (and its helpers) so the
// gocognit / gocyclo budgets stay green; the high-level shape of
// Update — message-type switch, fan-out to sub-models on size /
// async — stays visible here.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.fanOutToAll(msg)
	case palette.SwitchScreenMsg:
		// In-process screen switch — the palette emits this when a picked
		// entry's action matches the goto.sh deep-link pattern. No subshell,
		// no flow restart, no flicker.
		if id, ok := screenIDForName(msg.Screen); ok {
			m.current = id
		}
		return m, nil
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	}
	return m.fanOutToAll(msg)
}

// fanOutToAll forwards msg to every screen and batches the resulting
// tea.Cmds. Used by WindowSizeMsg and the default-async branch so any
// screen that listens for those gets them.
//
// WindowSizeMsg is forwarded with one row reserved for the global tab
// strip rendered above the active screen, so child screens that
// allocate viewports against the message height (cheatsheet) don't
// extend past the bottom and scroll their footer off.
func (m Model) fanOutToAll(msg tea.Msg) (tea.Model, tea.Cmd) {
	childMsg := msg
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
		reserved := size.Height - 1
		if reserved < 0 {
			reserved = 0
		}
		childMsg = tea.WindowSizeMsg{Width: size.Width, Height: reserved}
	}
	var cmds []tea.Cmd
	for i, s := range m.screens {
		updated, cmd := s.Update(childMsg)
		m.screens[i] = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return m, tea.Batch(cmds...)
}

// handleKeyMsg routes a key. Order: help-overlay-dismiss → forward
// to the active screen if it owns input → forward if the screen
// claimed the key → global key dispatch → fall through to the active
// screen.
func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.showHelp {
		// Help schließt explizit auf Esc/?/q. Jede andere Taste
		// schließt zwar auch, aber wird dann normal verarbeitet —
		// sonst muss man nach `?` zur Erinnerung an einen Shortcut
		// die Taste zweimal drücken.
		m.showHelp = false
		switch msg.String() {
		case "esc", "?", "q":
			return m, nil
		}
	}
	if si, ok := m.screens[m.current].(screener); ok && si.FilterActive() {
		return m.forwardToCurrent(msg)
	}
	if m.screenClaimsKey(msg.String()) {
		return m.forwardToCurrent(msg)
	}
	if next, cmd, ok := m.handleGlobalKey(msg); ok {
		return next, cmd
	}
	return m.forwardToCurrent(msg)
}

// screenClaimsKey reports whether the active screen's keyConsumer
// claim list contains the given key. Lets the screen suppress the
// sidekick's global tab-switch keys (e.g. worktime claims `:` and `n`).
func (m Model) screenClaimsKey(key string) bool {
	kc, ok := m.screens[m.current].(keyConsumer)
	if !ok {
		return false
	}
	for _, claimed := range kc.ConsumesKeys() {
		if claimed == key {
			return true
		}
	}
	return false
}

// handleGlobalKey dispatches the sidekick's own key map (q / ? / b /
// p / f / w / c / n). ok=false means the key isn't a global; the
// caller forwards to the active screen.
func (m Model) handleGlobalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit, true
	case "?":
		m.showHelp = true
		return m, nil, true
	case "b":
		if bh, ok := m.screens[m.current].(backHandler); ok && bh.HandlesBack() {
			next, cmd := m.forwardToCurrent(msg)
			return next, cmd, true
		}
		m.current = screenPalette
		return m, nil, true
	case "p":
		m.current = screenPalette
		return m, nil, true
	case "f":
		m.current = screenProjects
		return m, nil, true
	case "w":
		m.current = screenWorktime
		return m, nil, true
	case "c":
		m.current = screenCheatsheet
		return m, nil, true
	case "n":
		m.current = screenNotes
		return m, nil, true
	}
	return m, nil, false
}

// forwardToCurrent forwards msg to the active screen and returns the
// updated sidekick model.
func (m Model) forwardToCurrent(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.screens[m.current].Update(msg)
	m.screens[m.current] = updated
	return m, cmd
}

// View delegates rendering to the active screen, prefixed by a one-line
// global tab strip that surfaces which sidekick screen is active. The
// strip is suppressed when the `?`-overlay owns the surface — help is
// modal and the strip would compete with the section titles inside it.
func (m Model) View() string {
	if m.showHelp {
		return m.renderHelp()
	}
	return m.renderTabStrip() + "\n" + m.screens[m.current].View()
}

// tabStripEntry is one cell of the global strip. Key is the global
// switch letter (p / f / w / c / n) the user types; Label is the
// human-readable screen name; ID identifies the active match.
type tabStripEntry struct {
	key   string
	label string
	id    screenID
}

// renderTabStrip draws the global five-tab navigation bar at the top of
// every sidekick render. The active tab is bold + Accent (Heading
// style); inactive tabs are dim. The leading letter doubles as the
// global switch key so the strip self-documents the keybinds.
//
// Width-adaptive degradation mirrors worktime/model.go's tabStrip:
// full labels first, then key-only fallback when the pane is too
// narrow for the long form. NO_COLOR readers see brackets around the
// active key in the compact form so the marker survives without
// colour.
func (m Model) renderTabStrip() string {
	entries := []tabStripEntry{
		{"p", "Palette", screenPalette},
		{"f", "Projekte", screenProjects},
		{"w", "Worktime", screenWorktime},
		{"c", "Cheatsheet", screenCheatsheet},
		{"n", "Notes", screenNotes},
	}
	full := m.renderTabStripFull(entries)
	if m.width == 0 || lipgloss.Width(full) <= m.width {
		return full
	}
	return m.renderTabStripCompact(entries)
}

// activeTabStyle is Bold + Accent + Underline — A11y-2 (Skill §Tabs):
// der Underline-SGR sorgt dafür, dass der aktive Tab in NO_COLOR / Color-
// Blind-Settings ohne reine Farbabhängigkeit erkennbar bleibt.
func activeTabStyle(p theme.Palette) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(p.Sem().Accent).Bold(true).Underline(true)
}

func (m Model) renderTabStripFull(entries []tabStripEntry) string {
	sep := theme.Dim("  ·  ", m.pal)
	active := activeTabStyle(m.pal)
	parts := make([]string, len(entries))
	for i, e := range entries {
		text := e.key + " " + e.label
		if e.id == m.current {
			parts[i] = active.Render(text)
		} else {
			parts[i] = theme.Dim(text, m.pal)
		}
	}
	return " " + strings.Join(parts, sep)
}

func (m Model) renderTabStripCompact(entries []tabStripEntry) string {
	sep := theme.Dim(" · ", m.pal)
	active := activeTabStyle(m.pal)
	parts := make([]string, len(entries))
	for i, e := range entries {
		if e.id == m.current {
			parts[i] = active.Render("[" + e.key + "]")
		} else {
			parts[i] = theme.Dim(e.key, m.pal)
		}
	}
	return " " + strings.Join(parts, sep)
}

// helpProvider is the contract a screen implements to feed the
// sidekick `?`-overlay. Each screen owns its key bindings as data and
// returns them; sidekick concatenates with the global section, so a
// new binding lives in exactly one place.
type helpProvider interface {
	HelpSections() []help.Section
}

// helpSectionsGlobal lists the bindings the sidekick root itself owns
// (screen switches + quit + help-toggle). Kept as a data constant so a
// new sidekick-level binding extends this slice and nothing else.
func helpSectionsGlobal() help.Section {
	return help.Section{
		Title: "Global",
		Keys: [][2]string{
			{"p / b", "Palette"},
			{"f", "Projekte"},
			{"w", "Worktime"},
			{"c", "Cheatsheet"},
			{"n", "Notes (Kompendium)"},
			{"?", "Hilfe (diese Ansicht)"},
			{"q / Ctrl+C", "Beenden"},
		},
	}
}

// renderHelp draws the `?`-overlay. Aggregates the global section and
// each screen's HelpSections() so a new binding lands in exactly one
// place — the screen that owns it. Screens that do not implement
// helpProvider are skipped silently; the sidekick has no "stub" data
// to fall back on (which is the point: the overlay never claims a
// binding the screen no longer offers).
func (m Model) renderHelp() string {
	sections := []help.Section{helpSectionsGlobal()}
	for _, s := range m.screens {
		if hp, ok := s.(helpProvider); ok {
			sections = append(sections, hp.HelpSections()...)
		}
	}
	box := help.Render("Hilfe · Tastenbelegung", sections, 22, m.width, m.pal)
	footer := statusbar.Hints("beliebige Taste → zurück", m.pal)
	return box + "\n" + footer
}

// CurrentState returns a snapshot of the active screen's UI state for
// persistence via ports.FlowStateStore.
func (m Model) CurrentState() domain.FlowState {
	s := domain.FlowState{Screen: idToName(m.current)}
	if si, ok := m.screens[m.current].(screener); ok {
		s.Filter = si.StateFilter()
		s.Cursor = si.StateCursor()
	}
	return s
}

func idToName(id screenID) string {
	switch id {
	case screenProjects:
		return domain.ScreenProjects
	case screenWorktime:
		return domain.ScreenWorktime
	case screenCheatsheet:
		return domain.ScreenCheatsheet
	case screenNotes:
		return domain.ScreenNotes
	default:
		return domain.ScreenPalette
	}
}

// screenIDForName resolves a domain screen identifier to its internal
// screenID. Returns false when name does not match any known screen.
func screenIDForName(name string) (screenID, bool) {
	switch name {
	case domain.ScreenPalette:
		return screenPalette, true
	case domain.ScreenProjects:
		return screenProjects, true
	case domain.ScreenWorktime:
		return screenWorktime, true
	case domain.ScreenCheatsheet:
		return screenCheatsheet, true
	case domain.ScreenNotes:
		return screenNotes, true
	}
	return 0, false
}
