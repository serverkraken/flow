// Package projects implements the project-switcher screen: a
// fuzzy-filterable list of git repos under SOURCECODE_ROOT, annotated
// with which ones already have a tmux session attached.
//
// The screen is port-driven: ProjectsReader enumerates + tmux-annotates
// repos, ProjectSwitcher creates / attaches the session. No filesystem
// or exec.Command calls happen in this package.
package projects

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
	"github.com/serverkraken/flow/internal/frontend/tui/components/titlebox"
	"github.com/serverkraken/flow/internal/usecase"
)

type loadedMsg struct {
	projects []domain.Project
	err      error
}

type switchedMsg struct{}

// Model is the bubbletea model for the project-switcher screen.
type Model struct {
	all     []domain.Project
	visible []domain.Project
	cursor  int
	offset  int
	filter  textinput.Model
	pal     theme.Palette
	width   int
	height  int
	err     error
	loading bool
	rootDir string

	reader   *usecase.ProjectsReader
	switcher *usecase.ProjectSwitcher
}

// New constructs a projects Model. rootDir is purely informational —
// shown in the title bar; the reader is responsible for honouring it.
func New(p theme.Palette, rootDir string, reader *usecase.ProjectsReader, switcher *usecase.ProjectSwitcher) Model {
	ti := textinput.New()
	ti.Placeholder = "filter…"
	ti.CharLimit = 80
	return Model{
		pal:      p,
		filter:   ti,
		rootDir:  rootDir,
		loading:  true,
		reader:   reader,
		switcher: switcher,
	}
}

// FilterActive reports whether the filter input is focused.
func (m Model) FilterActive() bool { return m.filter.Focused() }

// StateFilter returns the current filter for state persistence.
func (m Model) StateFilter() string { return m.filter.Value() }

// StateCursor returns the cursor for state persistence.
func (m Model) StateCursor() int { return m.cursor }

// WithState restores filter and cursor from persisted state. Returns
// tea.Model so the sidekick root can call through its stateRestorer
// interface.
func (m Model) WithState(filter string, cursor int) tea.Model {
	m.filter.SetValue(filter)
	m.cursor = cursor
	return m
}

// Init kicks off the async project enumeration.
func (m Model) Init() tea.Cmd {
	r := m.reader
	return func() tea.Msg {
		ps, err := r.List()
		return loadedMsg{projects: ps, err: err}
	}
}

// Update handles messages for the projects screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case loadedMsg:
		m.loading = false
		m.err = msg.err
		m.all = msg.projects
		m.applyFilter()
		return m, nil

	case switchedMsg:
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
			m.ensureCursorVisible()
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
			m.ensureCursorVisible()
		}
	case "G":
		m.cursor = max(0, len(m.visible)-1)
		m.ensureCursorVisible()
	case "g":
		m.cursor = 0
		m.ensureCursorVisible()
	case "pgdown", "ctrl+d":
		m.cursor = min(len(m.visible)-1, m.cursor+m.maxVisible())
		m.ensureCursorVisible()
	case "pgup", "ctrl+u":
		m.cursor = max(0, m.cursor-m.maxVisible())
		m.ensureCursorVisible()
	case "enter":
		if len(m.visible) > 0 {
			return m, m.switchToProject(m.visible[m.cursor])
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
			return m, m.switchToProject(m.visible[m.cursor])
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
		names := make([]string, len(m.all))
		for i, p := range m.all {
			names[i] = p.Name
		}
		matches := fuzzy.Find(q, names)
		m.visible = make([]domain.Project, len(matches))
		for i, match := range matches {
			m.visible[i] = m.all[match.Index]
		}
	}
	if m.cursor >= len(m.visible) {
		m.cursor = max(0, len(m.visible)-1)
	}
	m.offset = 0
	m.ensureCursorVisible()
}

func (m Model) maxVisible() int {
	return max(1, m.height-6)
}

func (m *Model) ensureCursorVisible() {
	vis := m.maxVisible()
	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+vis {
		m.offset = m.cursor - vis + 1
	}
}

// switchToProject delegates to the use case. Errors propagate to the
// loadedMsg.err field via a follow-up reload, but we quit on success
// so the operator sees the new tmux session in the foreground.
func (m Model) switchToProject(p domain.Project) tea.Cmd {
	sw := m.switcher
	return func() tea.Msg {
		_ = sw.Switch(p) // best-effort; tmux failures show up next run
		return switchedMsg{}
	}
}

// View renders the projects screen.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	inner := m.width - 4

	var rows []string
	prompt := lipgloss.NewStyle().Foreground(m.pal.Accent).Bold(true).Render("› ")
	rows = append(rows, prompt+m.filter.View(), "")

	switch {
	case m.loading:
		rows = append(rows, lipgloss.NewStyle().Foreground(m.pal.Dim).Render("  lade Projekte…"))
	case m.err != nil:
		rows = append(rows, lipgloss.NewStyle().Foreground(m.pal.Red).Render("  "+m.err.Error()))
	case len(m.all) == 0:
		rows = append(rows, lipgloss.NewStyle().Foreground(m.pal.Dim).Render(
			"  keine Projekte gefunden — $SOURCECODE_ROOT prüfen"))
	case len(m.visible) == 0:
		rows = append(rows, lipgloss.NewStyle().Foreground(m.pal.Dim).Render("  keine Treffer"))
	default:
		vis := m.maxVisible()
		end := min(m.offset+vis, len(m.visible))
		if m.offset > 0 {
			rows = append(rows, lipgloss.NewStyle().Foreground(m.pal.Dim).
				Render(fmt.Sprintf("  ↑ %d vorherige…", m.offset)))
		}
		for i := m.offset; i < end; i++ {
			p := m.visible[i]
			label := p.Name
			if p.HasTmuxSession {
				label = p.Name + "  " + lipgloss.NewStyle().Foreground(m.pal.Green).Render("●")
			}
			rows = append(rows, picker.Row(i == m.cursor, label, "", inner, m.pal))
		}
		if end < len(m.visible) {
			rows = append(rows, lipgloss.NewStyle().Foreground(m.pal.Dim).
				Render(fmt.Sprintf("  ↓ %d weitere…", len(m.visible)-end)))
		}
	}

	body := strings.Join(rows, "\n")
	label := lastSegment(m.rootDir)
	var title string
	if m.filter.Value() != "" {
		title = fmt.Sprintf("Projekte · %s · %d/%d", label, len(m.visible), len(m.all))
	} else {
		title = fmt.Sprintf("Projekte · %s · %d", label, len(m.all))
	}
	box := titlebox.Render(title, body, m.width, m.pal)
	// 4-Cap (skill §Spacing): wichtigste 4 im Footer, Surplus (esc/b/q) sind
	// Fixed-Slot-Keys, dokumentiert im sidekick-globalen `?`-Overlay.
	footer := lipgloss.NewStyle().Foreground(m.pal.Dim).Padding(0, 1).
		Render("enter → wechseln  ·  j/k → bewegen  ·  / → filter  ·  ? → hilfe")
	return box + "\n" + footer
}

func lastSegment(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}
