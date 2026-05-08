// Package cheatsheet implements the cheatsheet screen: a scrollable,
// MarkdownRenderer-styled view of the user's cheatsheet source.
//
// The screen takes a CheatsheetReader (loads the raw Markdown) and a
// MarkdownRenderer (turns it into a styled, terminal-ready string)
// via constructor injection. No filesystem or rendering library
// calls happen in this package.
package cheatsheet

import (
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/frontend/tui/components/statusbar"
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
	vpReady    bool
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

// Update handles window resize, the async loadedMsg, the standalone-quit
// keys (q / Ctrl+C), and viewport scroll. q/Ctrl+C only fire when the
// cheatsheet is the program's root model — when hosted as a flow sidekick
// tab, the parent sidekick consumes those keys first (sidekick/model.go's
// tea.KeyMsg handler), so the local handler is a harmless no-op there.
// Without this, `flow cheatsheet` launched as a standalone tmux popup
// has no way to exit short of Ctrl+C SIGINT.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		w, h := msg.Width-4, msg.Height-4
		// Resize in place so the user's scroll position survives a tmux
		// pane resize. Allocating a new viewport.Model on every resize
		// reset YOffset to 0, which was very visible while the user was
		// mid-scroll and tmux re-laid out the panes.
		if !m.vpReady {
			m.vp = viewport.New(w, h)
			m.vpReady = true
		} else {
			m.vp.Width = w
			m.vp.Height = h
		}
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

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
		// Forward only key messages the local switch didn't claim.
		// Non-Key messages (toast.DismissedMsg, palette switches, etc.)
		// have no business reaching the viewport — bubbles' viewport
		// would just ignore them, but the unbounded forwarding surface
		// is a footgun whenever bubbletea adds a new event type.
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	}
	return m, nil
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
		content = theme.Err("\n  Fehler: "+m.err.Error(), m.pal)
	case !m.rendered:
		content = theme.Dim("\n  Cheatsheet lädt…", m.pal)
	default:
		content = m.vp.View()
	}

	title := "Cheatsheet"
	if m.rendered {
		title = fmt.Sprintf("Cheatsheet · %.0f%%", m.vp.ScrollPercent()*100)
	}
	box := titlebox.Render(title, content, m.width, m.pal)
	// Skill §Hint format ≤4: scrollen, Palette-Sprung (b ist sidekick-router),
	// schließen. „b → Palette" statt „b → zurück" — letzteres impliziert
	// Browser-History, was im Sidekick nicht stimmt.
	footer := statusbar.Hints("↑/↓ · PgUp/PgDn → scrollen  ·  b → Palette  ·  q → schließen", m.pal)
	return box + "\n" + footer
}
