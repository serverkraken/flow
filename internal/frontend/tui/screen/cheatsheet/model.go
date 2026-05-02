// Package cheatsheet implements the cheatsheet screen: a scrollable,
// MarkdownRenderer-styled view of the user's cheatsheet source.
//
// The screen takes a CheatsheetReader (loads the raw Markdown) and a
// MarkdownRenderer (turns it into a styled, terminal-ready string)
// via constructor injection. No filesystem or glamour calls happen
// in this package.
package cheatsheet

import (
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
	"github.com/serverkraken/flow/internal/frontend/tui/components/titlebox"
	"github.com/serverkraken/flow/internal/ports"
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
	pal        theme.Palette
	width      int
	height     int

	cheatsheet ports.CheatsheetReader
	renderer   ports.MarkdownRenderer
}

// New constructs a cheatsheet Model from the given palette, cheatsheet
// source reader, and Markdown renderer.
func New(p theme.Palette, cs ports.CheatsheetReader, r ports.MarkdownRenderer) Model {
	return Model{pal: p, cheatsheet: cs, renderer: r}
}

// FilterActive always returns false — cheatsheet has no text filter.
func (m Model) FilterActive() bool { return false }

// StateFilter returns "" — no filter to persist.
func (m Model) StateFilter() string { return "" }

// StateCursor returns 0 — cursor not persisted for cheatsheet.
func (m Model) StateCursor() int { return 0 }

// Init loads the cheatsheet source asynchronously through the injected
// CheatsheetReader.
func (m Model) Init() tea.Cmd {
	cs := m.cheatsheet
	return func() tea.Msg {
		s, err := cs.Load()
		return loadedMsg{content: s, err: err}
	}
}

// Update handles window resize, the async loadedMsg, and viewport scroll
// keys.
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
	rendered, err := m.renderer.Render(m.rawContent, m.width-2)
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
	switch {
	case m.err != nil:
		content = lipgloss.NewStyle().Foreground(m.pal.Red).Render(
			"\n  Fehler: " + m.err.Error())
	case !m.rendered:
		content = lipgloss.NewStyle().Foreground(m.pal.Dim).Render("\n  lade…")
	default:
		content = m.vp.View()
	}

	title := "Cheatsheet"
	if m.rendered {
		title = fmt.Sprintf("Cheatsheet · %.0f%%", m.vp.ScrollPercent()*100)
	}
	box := titlebox.Render(title, content, m.width, m.pal)
	footer := lipgloss.NewStyle().Foreground(m.pal.Dim).Padding(0, 1).
		Render("↑/↓ PgUp/PgDn → scrollen  ·  b → zurück  ·  q → schließen")
	return box + "\n" + footer
}
