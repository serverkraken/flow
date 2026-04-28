// Package app is the root bubbletea model that owns all four screen models
// and routes messages between them.
package app

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/screen/cheatsheet"
	"github.com/serverkraken/flow/internal/screen/palette"
	"github.com/serverkraken/flow/internal/screen/projects"
	"github.com/serverkraken/flow/internal/screen/worktime"
	"github.com/serverkraken/flow/internal/state"
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

// Model is the root bubbletea model.
type Model struct {
	screens [4]tea.Model
	current screenID
	width   int
	height  int
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
		case "b":
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
	return m.screens[m.current].View()
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
