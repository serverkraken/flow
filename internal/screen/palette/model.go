package palette

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
	"github.com/serverkraken/tui-kit/components/picker"
	"github.com/serverkraken/tui-kit/components/titlebox"
	tk "github.com/serverkraken/tui-kit/theme"
)

// Internal messages.
type loadedMsg struct {
	entries []Entry
	err     error
}

type dispatchedMsg struct{}

// Model is the bubbletea model for the palette screen.
type Model struct {
	all     []Entry
	visible []Entry
	cursor  int
	filter  textinput.Model
	theme   tk.Palette
	width   int
	height  int
	err     error
	loading bool
}

// New creates a new palette Model.
func New(p tk.Palette) Model {
	ti := textinput.New()
	ti.Placeholder = "filter…"
	ti.CharLimit = 80
	return Model{
		theme:   p,
		filter:  ti,
		loading: true,
	}
}

// FilterActive reports whether the text input has focus.
func (m Model) FilterActive() bool { return m.filter.Focused() }

// StateFilter returns the current filter value for state persistence.
func (m Model) StateFilter() string { return m.filter.Value() }

// StateCursor returns the cursor position for state persistence.
func (m Model) StateCursor() int { return m.cursor }

// WithState restores filter and cursor from persisted state.
func (m Model) WithState(filter string, cursor int) Model {
	m.filter.SetValue(filter)
	m.cursor = cursor
	return m
}

// Init loads palette entries asynchronously.
func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		entries, err := LoadEntries()
		return loadedMsg{entries: entries, err: err}
	}
}

// Update handles messages for the palette screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case loadedMsg:
		m.loading = false
		m.err = msg.err
		m.all = msg.entries
		m.applyFilter()
		return m, nil

	case dispatchedMsg:
		return m, tea.Quit

	case tea.KeyMsg:
		if m.filter.Focused() {
			return m.handleFilterKey(msg)
		}
		return m.handleNormalKey(msg)
	}
	return m, nil
}

func (m Model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "/":
		m.filter.Focus()
		return m, textinput.Blink
	case "j", "down":
		if m.cursor < len(m.visible)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter":
		if len(m.visible) > 0 {
			return m, m.dispatch(m.visible[m.cursor].Action)
		}
	}
	return m, nil
}

func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.filter.Blur()
		m.filter.SetValue("")
		m.applyFilter()
		return m, nil
	case tea.KeyEnter:
		m.filter.Blur()
		if len(m.visible) > 0 {
			return m, m.dispatch(m.visible[m.cursor].Action)
		}
		return m, nil
	}
	var cmd tea.Cmd
	prev := m.filter.Value()
	m.filter, cmd = m.filter.Update(msg)
	if m.filter.Value() != prev {
		m.applyFilter()
	}
	return m, cmd
}

func (m *Model) applyFilter() {
	q := m.filter.Value()
	if q == "" {
		m.visible = m.all
	} else {
		targets := make([]string, len(m.all))
		for i, e := range m.all {
			targets[i] = e.Section + " " + e.Label
		}
		matches := fuzzy.Find(q, targets)
		m.visible = make([]Entry, len(matches))
		for i, match := range matches {
			m.visible[i] = m.all[match.Index]
		}
	}
	if m.cursor >= len(m.visible) {
		m.cursor = max(0, len(m.visible)-1)
	}
}

func (m Model) dispatch(action string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("tmux", "run-shell", "-b",
			fmt.Sprintf("sleep 0.15; tmux %s", action))
		_ = cmd.Start()
		return dispatchedMsg{}
	}
}

// View renders the palette screen.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	inner := m.width - 4 // box border (2) + padding (2)

	var rows []string

	// Filter bar.
	prompt := lipgloss.NewStyle().Foreground(m.theme.Accent).Render("> ")
	filterLine := prompt + m.filter.View()
	rows = append(rows, filterLine, "")

	if m.loading {
		rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Dim).Render("  lade…"))
	} else if m.err != nil {
		rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Red).Render("  "+m.err.Error()))
	} else if len(m.visible) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Dim).Render("  keine Treffer"))
	} else {
		rows = append(rows, m.renderEntries(inner)...)
	}

	body := strings.Join(rows, "\n")
	title := fmt.Sprintf("Palette · %d Aktionen", len(m.all))
	box := titlebox.Render(title, body, m.width, m.theme)
	footer := lipgloss.NewStyle().Foreground(m.theme.Dim).Padding(0, 1).
		Render("enter → ausführen  ·  j/k → bewegen  ·  / → filter  ·  esc → löschen  ·  q → schließen")
	return box + "\n" + footer
}

func (m Model) renderEntries(innerWidth int) []string {
	var rows []string
	lastSection := ""
	for i, e := range m.visible {
		if e.Section != lastSection {
			if lastSection != "" {
				rows = append(rows, "")
			}
			rows = append(rows, picker.SectionHeader(e.Section, innerWidth, m.theme))
			lastSection = e.Section
		}
		label := e.Label
		if e.Icon != "" {
			label = e.Icon + "  " + e.Label
		}
		rows = append(rows, picker.Row(i == m.cursor, label, "", innerWidth, m.theme))
	}
	return rows
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
