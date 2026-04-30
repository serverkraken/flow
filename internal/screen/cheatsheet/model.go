// Package cheatsheet implements the cheatsheet screen: a scrollable glamour-
// rendered view of ~/.tmux/cheatsheet.md.
package cheatsheet

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/tui-kit/components/titlebox"
	tk "github.com/serverkraken/tui-kit/theme"
)

type loadedMsg struct {
	content string
	err     error
}

// Model is the bubbletea model for the cheatsheet screen.
type Model struct {
	vp         viewport.Model
	rawContent string
	rendered   bool
	err        error
	theme      tk.Palette
	width      int
	height     int
}

// New creates a new cheatsheet Model.
func New(p tk.Palette) Model {
	return Model{theme: p}
}

// FilterActive always returns false — cheatsheet has no text filter.
func (m Model) FilterActive() bool { return false }

// StateFilter returns "" — no filter to persist.
func (m Model) StateFilter() string { return "" }

// StateCursor returns 0 — cursor not persisted for cheatsheet.
func (m Model) StateCursor() int { return 0 }

// Init loads ~/.tmux/cheatsheet.md asynchronously.
func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		home, err := os.UserHomeDir()
		if err != nil {
			return loadedMsg{err: err}
		}
		data, err := os.ReadFile(filepath.Join(home, ".tmux", "cheatsheet.md"))
		if err != nil {
			return loadedMsg{err: err}
		}
		return loadedMsg{content: string(data)}
	}
}

// Update handles messages for the cheatsheet screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.vp = viewport.New(msg.Width-4, msg.Height-4)
		if m.rawContent != "" {
			m.rendered = false
			m.renderContent()
		}
		return m, nil

	case loadedMsg:
		m.err = msg.err
		m.rawContent = msg.content
		if m.width > 0 {
			m.renderContent()
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m *Model) renderContent() {
	if m.rawContent == "" || m.width == 0 {
		return
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(m.width-2),
	)
	if err != nil {
		m.vp.SetContent(m.rawContent)
		m.rendered = true
		return
	}
	rendered, err := r.Render(m.rawContent)
	if err != nil {
		m.vp.SetContent(m.rawContent)
	} else {
		m.vp.SetContent(rendered)
	}
	m.rendered = true
}

// View renders the cheatsheet screen.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	var content string
	if m.err != nil {
		content = lipgloss.NewStyle().Foreground(m.theme.Red).Render(
			"\n  Fehler: " + m.err.Error())
	} else if !m.rendered {
		content = lipgloss.NewStyle().Foreground(m.theme.Dim).Render("\n  lade…")
	} else {
		content = m.vp.View()
	}

	title := "Cheatsheet"
	if m.rendered {
		title = fmt.Sprintf("Cheatsheet · %.0f%%", m.vp.ScrollPercent()*100)
	}
	box := titlebox.Render(title, content, m.width, m.theme)
	footer := lipgloss.NewStyle().Foreground(m.theme.Dim).Padding(0, 1).
		Render("↑/↓ PgUp/PgDn → scrollen  ·  b → zurück  ·  q → schließen")
	return box + "\n" + footer
}
