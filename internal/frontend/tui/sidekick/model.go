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
	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/help"
	"github.com/serverkraken/flow/internal/frontend/tui/components/statusbar"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/palette"
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
// types and a fixed set of global keys; cyclomatic complexity sits at
// 22 (just above the linter's 20 threshold). Splitting helpers would
// hide the dispatch structure behind indirection without simplifying
// it. Same rationale palette/model.go's handleNormalKey uses.
//
//nolint:gocyclo
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		for i, s := range m.screens {
			updated, cmd := s.Update(msg)
			m.screens[i] = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)

	case palette.SwitchScreenMsg:
		// In-process screen switch — the palette emits this when a picked
		// entry's action matches the goto.sh deep-link pattern. No subshell,
		// no flow restart, no flicker.
		if id, ok := screenIDForName(msg.Screen); ok {
			m.current = id
		}
		return m, nil

	case tea.KeyMsg:
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		if si, ok := m.screens[m.current].(screener); ok && si.FilterActive() {
			updated, cmd := m.screens[m.current].Update(msg)
			m.screens[m.current] = updated
			return m, cmd
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.showHelp = true
			return m, nil
		case "b":
			if bh, ok := m.screens[m.current].(backHandler); ok && bh.HandlesBack() {
				updated, cmd := m.screens[m.current].Update(msg)
				m.screens[m.current] = updated
				return m, cmd
			}
			m.current = screenPalette
			return m, nil
		case "p":
			m.current = screenPalette
			return m, nil
		case "f":
			m.current = screenProjects
			return m, nil
		case "w":
			m.current = screenWorktime
			return m, nil
		case "c":
			m.current = screenCheatsheet
			return m, nil
		case "n":
			m.current = screenNotes
			return m, nil
		}
		updated, cmd := m.screens[m.current].Update(msg)
		m.screens[m.current] = updated
		return m, cmd

	default:
		for i, s := range m.screens {
			updated, cmd := s.Update(msg)
			m.screens[i] = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)
	}
}

// View delegates rendering to the active screen, or to the help overlay
// when `?` was pressed.
func (m Model) View() string {
	if m.showHelp {
		return m.renderHelp()
	}
	return m.screens[m.current].View()
}

// renderHelp draws the `?`-overlay for the sidekick. Sections are grouped by
// purpose, keys ordered by frequency. Uses the canonical help.Render
// component instead of hand-rolled titlebox styling so visual drift across
// help overlays in the codebase is impossible.
func (m Model) renderHelp() string {
	sections := []help.Section{
		{Title: "Global", Keys: [][2]string{
			{"p / b", "Palette"},
			{"f", "Projekte"},
			{"w", "Worktime"},
			{"c", "Cheatsheet"},
			{"n", "Notes (Kompendium)"},
			{"?", "Hilfe (diese Ansicht)"},
			{"q / Ctrl+C", "Beenden"},
		}},
		{Title: "Palette", Keys: [][2]string{
			{"a–z (außer j/k/g/G)", "tippen → Filter direkt"},
			{"/", "Filter explizit öffnen"},
			{"j / k / ↑ / ↓", "Navigieren"},
			{"G / g", "Ende / Anfang"},
			{"] / [", "Nächste / vorige Section"},
			{"Ctrl+D / Ctrl+U", "Seite vor / zurück"},
			{"1–9", "Direktwahl (n-ter Treffer)"},
			{".", "Pin / Unpin (→ Favoriten)"},
			{"Enter", "Ausführen"},
			{"Esc", "Filter leeren · 2× → schließen"},
		}},
		{Title: "Projekte", Keys: [][2]string{
			{"j / k / ↑ / ↓", "Navigieren"},
			{"G / g", "Ende / Anfang"},
			{"Ctrl+D / Ctrl+U", "Seite vor / zurück"},
			{"/", "Filter öffnen"},
			{"Esc", "Filter löschen"},
			{"Enter", "Wechseln"},
		}},
		{Title: "Worktime — Tabs", Keys: [][2]string{
			{"1 · 2 · 3 · 4", "Heute · Woche · History · Frei"},
			{"Tab", "Nächster Tab"},
			{"b", "Voriger Tab (oder zurück zur Palette wenn auf Heute)"},
		}},
		{Title: "Worktime — Heute", Keys: [][2]string{
			{"j/k · g/G", "Cursor bewegen · oben/unten"},
			{"s", "Starten / Stoppen / Fortsetzen"},
			{"p", "Pause (im laufenden Zustand)"},
			{"E / Enter", "Session bearbeiten"},
			{"D", "Session löschen (y/Enter bestätigt)"},
			{"t · N", "Tag · Notiz für die fokussierte Session"},
			{"n", "Kompendium-Note an heute anhängen"},
			{"o · O", "Erste angehängte Note ansehen · bearbeiten"},
			{"Ctrl+D", "Erste angehängte Note entfernen"},
			{"?", "Diese Hilfe (auch im Standalone-`flow worktime today`)"},
		}},
		{Title: "Worktime — Woche", Keys: [][2]string{
			{"j/k · g/G", "Tag fokussieren · oben/unten"},
		}},
		{Title: "Worktime — History", Keys: [][2]string{
			{"j/k · g/G", "Cursor / Zeile · oben/unten"},
			{"Enter", "Drill-Down auf den Tag"},
			{"v", "Ansicht: Liste → Heatmap → Tag-Clock → Monat"},
			{"/", "Filter (KW18, 2026, 2026-04, tag:deep, note:standup)"},
			{"F", "Filter mit Prefix »tag:« vorbelegen"},
			{"[ / ]", "Filter um eine Einheit zurück / vor"},
			{"T", "Filter zurücksetzen / aktuelles Fenster"},
			{"h · l (Heatmap/Tag-Clock/Monat)", "Cursor horizontal"},
		}},
		{Title: "Worktime — Frei", Keys: [][2]string{
			{"j/k · g/G", "Eintrag fokussieren"},
			{"a", "Tag(e) frei eintragen (Form)"},
			{"A · K", "heute = Urlaub · heute = krank"},
			{"B", "Gesetzliche Feiertage syncen"},
			{"D", "Eintrag löschen (y/Enter bestätigt)"},
			{"h · l · [ · ]", "Jahr zurück / vor"},
			{"T", "Aktuelles Jahr"},
		}},
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
