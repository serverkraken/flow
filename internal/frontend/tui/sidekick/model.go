// Package sidekick is the root bubbletea model for the `flow sidekick`
// TUI. It owns the four top-level screens (Palette, Projects, Worktime,
// Cheatsheet) as opaque tea.Models, routes messages between them, and
// snapshots the active screen's filter / cursor for persistence via
// ports.FlowStateStore.
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
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
	"github.com/serverkraken/flow/internal/frontend/tui/components/titlebox"
)

type screenID int

const (
	screenPalette    screenID = 0
	screenProjects   screenID = 1
	screenWorktime   screenID = 2
	screenCheatsheet screenID = 3
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

// Deps bundles the four pre-built screen models. All fields are required
// — the composition root in cmd/flow/main.go is responsible for wiring
// every slot. nil is not a supported configuration; the sidekick does not
// fall back to legacy in-package constructors.
type Deps struct {
	Palette    tea.Model
	Projects   tea.Model
	Worktime   tea.Model
	Cheatsheet tea.Model
}

// Model is the root bubbletea model.
type Model struct {
	screens  [4]tea.Model
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
	screens := [4]tea.Model{deps.Palette, deps.Projects, deps.Worktime, deps.Cheatsheet}

	var cur screenID
	switch s.Screen {
	case domain.ScreenProjects:
		cur = screenProjects
	case domain.ScreenWorktime:
		cur = screenWorktime
	case domain.ScreenCheatsheet:
		cur = screenCheatsheet
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

func (m Model) renderHelp() string {
	sections := []struct {
		title string
		keys  [][2]string
	}{
		{"Global", [][2]string{
			{"p / b", "Palette"},
			{"f", "Projekte"},
			{"w", "Worktime"},
			{"c", "Cheatsheet"},
			{"?", "Hilfe (diese Ansicht)"},
			{"q / Ctrl+C", "Beenden"},
		}},
		{"Palette", [][2]string{
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
		{"Projekte", [][2]string{
			{"j / k / ↑ / ↓", "Navigieren"},
			{"G / g", "Ende / Anfang"},
			{"Ctrl+D / Ctrl+U", "Seite vor / zurück"},
			{"/", "Filter öffnen"},
			{"Esc", "Filter löschen"},
			{"Enter", "Wechseln"},
		}},
		{"Worktime", [][2]string{
			{"Tab / 1·2·3·4", "Heute · Woche · History · Frei"},
			{"shift+Tab", "Vorherige Tab"},
			{"b", "Vorheriger Tab (oder zurück zur Palette wenn auf Heute)"},
			{"s", "Start / Stopp (mit Zeitangabe)"},
			{"f", "Fokus-Modus: Start + Daily-Note"},
			{"C", "Startzeit der laufenden Session korrigieren"},
			{"e", "Manuellen Eintrag erstellen"},
			{"E / Enter", "Session bearbeiten"},
			{"d", "Session löschen (mit Bestätigung)"},
			{"u", "Letzte Löschung rückgängig"},
			{"t / N", "Tag · Notiz für Session"},
			{"n", "Kompendium-Notiz anhängen / detachen"},
			{"o", "Notiz read-only ansehen (glow)"},
			{"O", "Notiz im Editor öffnen"},
			{"D", "Notiz detachen"},
			{"j/k · g/G · ↑/↓", "Cursor · oben/unten"},
			{"Woche: h/l", "Woche zurück / vor (t = aktuell)"},
			{"Woche: Enter", "Drill-Down auf Tag"},
			{"History: v", "List- ↔ Heatmap-Ansicht"},
			{"History: /", "Filter (KW18, 2026, tag:deep, FROM..TO)"},
			{"History: h/j/k/l", "Heatmap-Cursor (im Heatmap-Mode)"},
			{"Frei: a", "Tag(e) frei eintragen"},
			{"Frei: d/x", "Eintrag entfernen"},
			{"Frei: h/l", "Jahr ± · t aktuell"},
			{"r", "Neu laden"},
		}},
	}

	accent := lipgloss.NewStyle().Foreground(m.pal.Accent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(m.pal.Dim)
	fg := lipgloss.NewStyle().Foreground(m.pal.Fg)

	var rows []string
	for _, sec := range sections {
		rows = append(rows, accent.Render("  "+sec.title))
		for _, kv := range sec.keys {
			key := fg.Width(22).Render("    " + kv[0])
			rows = append(rows, key+dim.Render(kv[1]))
		}
		rows = append(rows, "")
	}

	body := strings.Join(rows, "\n")
	box := titlebox.Render("Hilfe · Tastenbelegung", body, m.width, m.pal)
	footer := lipgloss.NewStyle().Foreground(m.pal.Dim).Padding(0, 1).
		Render("beliebige Taste → zurück")
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
	default:
		return domain.ScreenPalette
	}
}
