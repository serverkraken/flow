// Package cheatsheet implements the cheatsheet screen: a scrollable,
// MarkdownRenderer-styled view of the user's cheatsheet source.
//
// The screen takes a CheatsheetReader (loads the raw Markdown) and a
// MarkdownRenderer (turns it into a styled, terminal-ready string)
// via constructor injection. No filesystem or rendering library
// calls happen in this package.
//
// Round4: migrated from a hand-rolled viewport + titlebox to the
// shared markdown_overlay component (fifth caller after brief_view,
// today_note_view, kompendium browse, kompendium full-view). The
// overlay carries the unified chrome (rounded frame + title + status
// bar) plus opt-in search and code-copy that the old cheatsheet
// didn't have.
package cheatsheet

import (
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/frontend/tui/components/help"
	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown_overlay"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/ports"
)

type loadedMsg struct {
	content string
	err     error
}

// Model is the bubbletea model for the cheatsheet screen.
type Model struct {
	overlay markdown_overlay.Model
	err     error
	pal     theme.Palette
	width   int
	height  int

	cheatsheet ports.CheatsheetReader
	renderer   ports.MarkdownRenderer
}

// New constructs a cheatsheet Model from the given palette, cheatsheet
// source reader, and Markdown renderer.
func New(p theme.Palette, cs ports.CheatsheetReader, r ports.MarkdownRenderer) Model {
	render := func(src string, w int) string {
		if r == nil {
			return src
		}
		out, err := r.Render(src, w)
		if err != nil {
			return src
		}
		return out
	}
	// Close keys: q + esc. `b` is the sidekick global for "switch to
	// Palette tab" — must NOT be claimed here, otherwise the user
	// can't navigate out of the cheatsheet via b while sidekick is
	// the host. In standalone mode (flow cheatsheet popup), q closes
	// via the ExitMsg → tea.Quit path in Update.
	overlay := markdown_overlay.New(render,
		markdown_overlay.WithTitle("Cheatsheet"),
		markdown_overlay.WithSearch(),
		markdown_overlay.WithCodeCopy(),
		markdown_overlay.WithCloseKeys("q", "esc"),
	)
	return Model{
		overlay:    overlay,
		pal:        p,
		cheatsheet: cs,
		renderer:   r,
	}
}

// HelpSections exposes the cheatsheet-screen key bindings for the
// sidekick `?`-overlay aggregation.
func (Model) HelpSections() []help.Section {
	return []help.Section{{
		Title: "Cheatsheet",
		Keys: [][2]string{
			{"↑ / ↓", "Eine Zeile scrollen"},
			{"PgUp / PgDn", "Eine Seite scrollen"},
			{"g / G", "Anfang / Ende"},
			{"/", "Im Cheatsheet suchen"},
			{"c", "Code-Block kopieren"},
			{"q", "Schließen (im Standalone)"},
		},
	}}
}

// FilterActive reports whether the embedded markdown_overlay is in
// search mode. Sidekick consults this to suppress its global key
// routing while the user is typing in the overlay's search input.
func (m Model) FilterActive() bool {
	return m.overlay.CurrentMode() == markdown_overlay.ModeSearch
}

// StateFilter returns "" — no filter state is persisted across
// sessions; the overlay's transient search is intentional.
func (m Model) StateFilter() string { return "" }

// StateCursor returns 0 — overlay scroll position resets on every
// load; we don't restore it across sessions.
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

// Update handles window resize, the async loadedMsg, ExitMsg from the
// overlay (close → tea.Quit in standalone), and forwards everything
// else to the embedded overlay.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.overlay = m.overlay.SetSize(msg.Width, msg.Height)
		return m, nil

	case loadedMsg:
		m.err = msg.err
		if msg.err != nil {
			m.overlay = m.overlay.SetError(msg.err)
			return m, nil
		}
		m.overlay = m.overlay.SetSource(msg.content)
		return m, nil

	case markdown_overlay.ExitMsg:
		// Standalone-Modus: q in der Overlay-Close-Key-Liste → ExitMsg
		// → wir terminieren. In Sidekick-Modus erreicht q die Cheatsheet
		// nie (sidekick globals fangen das davor) — der Pfad bleibt aber
		// als Safety-Net, falls der Host das Routing aendert.
		return m, tea.Quit
	}

	// Forward everything else (key messages, etc.) to the overlay.
	next, cmd := m.overlay.Update(msg)
	m.overlay = next
	return m, cmd
}

// View renders the embedded markdown_overlay. The overlay produces the
// full screen — rounded frame, title, body, footer, status bar — so
// the cheatsheet wrapper is just a thin host.
func (m Model) View() tea.View {
	v := tea.NewView(m.viewContent())
	v.AltScreen = true
	return v
}

func (m Model) viewContent() string {
	if m.width == 0 {
		return ""
	}
	return m.overlay.View()
}
