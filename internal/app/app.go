// Package app is the root bubbletea model that owns all four screen models
// and routes messages between them.
package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/screen/cheatsheet"
	"github.com/serverkraken/flow/internal/screen/palette"
	"github.com/serverkraken/flow/internal/screen/projects"
	"github.com/serverkraken/flow/internal/screen/worktime"
	"github.com/serverkraken/flow/internal/state"
	"github.com/serverkraken/tui-kit/components/titlebox"
	tk "github.com/serverkraken/tui-kit/theme"
)

type screenID int

const (
	screenPalette    screenID = 0
	screenProjects   screenID = 1
	screenWorktime   screenID = 2
	screenCheatsheet screenID = 3
)

// screener is the extra interface all screen models satisfy beyond tea.Model.
type screener interface {
	FilterActive() bool
	StateFilter() string
	StateCursor() int
}

// backHandler is implemented by screens that want to consume the global `b`
// key under specific conditions (e.g. switching back to a previous tab inside
// the screen instead of jumping to the palette).
type backHandler interface {
	HandlesBack() bool
}

// Model is the root bubbletea model.
type Model struct {
	screens  [4]tea.Model
	current  screenID
	showHelp bool
	pal      tk.Palette
	width    int
	height   int
}

// New creates the root model, restoring screen/filter/cursor from s.
func New(p tk.Palette, s state.State) Model {
	pm := palette.New(p)
	pr := projects.New(p)
	wm := worktime.New(p)
	cm := cheatsheet.New(p)

	var cur screenID
	switch s.Screen {
	case state.Projects:
		cur = screenProjects
		pr = pr.WithState(s.Filter, s.Cursor)
	case state.Worktime:
		cur = screenWorktime
	case state.Cheatsheet:
		cur = screenCheatsheet
	default:
		cur = screenPalette
		pm = pm.WithState(s.Filter, s.Cursor)
	}

	return Model{
		screens: [4]tea.Model{pm, pr, wm, cm},
		current: cur,
		pal:     p,
	}
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
		// Forward to the screen when its filter is active — screen owns input.
		if si, ok := m.screens[m.current].(screener); ok && si.FilterActive() {
			updated, cmd := m.screens[m.current].Update(msg)
			m.screens[m.current] = updated
			return m, cmd
		}
		// Global navigation keys.
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
		// All other keys go to the current screen.
		updated, cmd := m.screens[m.current].Update(msg)
		m.screens[m.current] = updated
		return m, cmd

	default:
		// Async messages (loaded, tick, …) are fanned out to all screens so
		// each can react to its own private message types and ignore the rest.
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

// View delegates rendering to the active screen.
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

// CurrentState returns a snapshot of the active screen's UI state for persistence.
func (m Model) CurrentState() state.State {
	s := state.State{Screen: idToName(m.current)}
	if si, ok := m.screens[m.current].(screener); ok {
		s.Filter = si.StateFilter()
		s.Cursor = si.StateCursor()
	}
	return s
}

func idToName(id screenID) string {
	switch id {
	case screenProjects:
		return state.Projects
	case screenWorktime:
		return state.Worktime
	case screenCheatsheet:
		return state.Cheatsheet
	default:
		return state.Palette
	}
}
