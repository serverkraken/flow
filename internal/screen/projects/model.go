// Package projects implements the project-switcher screen.
package projects

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
	"github.com/serverkraken/tui-kit/components/picker"
	"github.com/serverkraken/tui-kit/components/titlebox"
	tk "github.com/serverkraken/tui-kit/theme"
)

type dirsLoadedMsg struct {
	dirs     []string
	sessions map[string]bool
	err      error
}

type switchedMsg struct{}

// Model is the bubbletea model for the project-switcher screen.
type Model struct {
	all      []string
	visible  []string
	sessions map[string]bool
	cursor   int
	offset   int
	filter   textinput.Model
	root     string
	theme    tk.Palette
	width    int
	height   int
	err      error
	loading  bool
}

// New creates a new projects Model.
func New(p tk.Palette) Model {
	ti := textinput.New()
	ti.Placeholder = "filter…"
	ti.CharLimit = 80
	root := os.Getenv("SOURCECODE_ROOT")
	if root == "" {
		home, _ := os.UserHomeDir()
		root = filepath.Join(home, "Sourcecode")
	}
	return Model{
		theme:   p,
		filter:  ti,
		root:    root,
		loading: true,
	}
}

// FilterActive reports whether the filter input is focused.
func (m Model) FilterActive() bool { return m.filter.Focused() }

// StateFilter returns the current filter for state persistence.
func (m Model) StateFilter() string { return m.filter.Value() }

// StateCursor returns the cursor for state persistence.
func (m Model) StateCursor() int { return m.cursor }

// WithState restores filter and cursor from persisted state.
func (m Model) WithState(filter string, cursor int) Model {
	m.filter.SetValue(filter)
	m.cursor = cursor
	return m
}

// Init loads the project directory list asynchronously.
func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		dirs, err := loadDirs(m.root)
		sessions := tmuxSessions()
		return dirsLoadedMsg{dirs: dirs, sessions: sessions, err: err}
	}
}

// Update handles messages for the projects screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case dirsLoadedMsg:
		m.loading = false
		m.err = msg.err
		m.all = msg.dirs
		m.sessions = msg.sessions
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
		matches := fuzzy.Find(q, m.all)
		m.visible = make([]string, len(matches))
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

var nameReplacer = strings.NewReplacer(".", "_", " ", "_", "/", "_")

func sessionName(dir string) string { return nameReplacer.Replace(dir) }

func (m Model) switchToProject(dir string) tea.Cmd {
	root := m.root
	return func() tea.Msg {
		name := sessionName(dir)
		target := filepath.Join(root, dir)
		if err := exec.Command("tmux", "has-session", "-t", name).Run(); err != nil {
			_ = exec.Command("tmux", "new-session", "-d", "-s", name, "-c", target).Run()
		}
		_ = exec.Command("tmux", "switch-client", "-t", name).Run()
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
	prompt := lipgloss.NewStyle().Foreground(m.theme.Accent).Render("> ")
	rows = append(rows, prompt+m.filter.View(), "")

	switch {
	case m.loading:
		rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Dim).Render("  lade Projekte…"))
	case !isDirAccessible(m.root):
		rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Red).Render(
			"  $SOURCECODE_ROOT nicht gesetzt oder nicht vorhanden"))
	case m.err != nil:
		rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Red).Render("  "+m.err.Error()))
	case len(m.visible) == 0:
		rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Dim).Render("  keine Treffer"))
	default:
		vis := m.maxVisible()
		end := min(m.offset+vis, len(m.visible))
		if m.offset > 0 {
			rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Dim).
				Render(fmt.Sprintf("  ↑ %d vorherige…", m.offset)))
		}
		for i := m.offset; i < end; i++ {
			dir := m.visible[i]
			name := sessionName(dir)
			label := dir
			if m.sessions[name] {
				label = dir + "  " + lipgloss.NewStyle().Foreground(m.theme.Green).Render("●")
			}
			rows = append(rows, picker.Row(i == m.cursor, label, "", inner, m.theme))
		}
		if end < len(m.visible) {
			rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Dim).
				Render(fmt.Sprintf("  ↓ %d weitere…", len(m.visible)-end)))
		}
	}

	body := strings.Join(rows, "\n")
	var title string
	label := filepath.Base(m.root)
	if m.filter.Value() != "" {
		title = fmt.Sprintf("Projekte · %s · %d/%d", label, len(m.visible), len(m.all))
	} else {
		title = fmt.Sprintf("Projekte · %s · %d", label, len(m.all))
	}
	box := titlebox.Render(title, body, m.width, m.theme)
	footer := lipgloss.NewStyle().Foreground(m.theme.Dim).Padding(0, 1).
		Render("enter → wechseln  ·  j/k → bewegen  ·  / → filter  ·  esc → löschen  ·  b → zurück  ·  q → schließen")
	return box + "\n" + footer
}

func isDirAccessible(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func loadDirs(root string) ([]string, error) {
	if !isDirAccessible(root) {
		return nil, nil
	}
	// Locate git repos by finding .git directories (max depth 5 = repos 4 levels deep).
	out, err := exec.Command("fd", "-H", "--type", "d", "--max-depth", "5",
		"--color", "never", `^\.git$`, root).Output()
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSuffix(line, "/")
		// Strip the /.git suffix to get the repo root.
		line = strings.TrimSuffix(line, "/.git")
		// Make relative to root.
		line = strings.TrimPrefix(line, root+"/")
		if line != "" && line != root {
			dirs = append(dirs, line)
		}
	}
	return dirs, nil
}

func tmuxSessions() map[string]bool {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return nil
	}
	sessions := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			sessions[name] = true
		}
	}
	return sessions
}
