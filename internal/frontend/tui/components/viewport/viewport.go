// Package viewport wraps charmbracelet/bubbles/viewport and adds live-tail
// support for streaming log output.
package viewport

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
)

// Model is a scrollable viewport with optional auto-scroll (tail mode).
// When tail is enabled and the user is at the bottom, AppendContent
// automatically scrolls to show the new lines.
type Model struct {
	vp      viewport.Model
	content string
	tail    bool
	theme   theme.Palette
}

// New creates a viewport sized to width x height with tail mode enabled.
func New(width, height int, p theme.Palette) Model {
	return Model{vp: viewport.New(width, height), tail: true, theme: p}
}

// SetSize updates the viewport dimensions.
func (m *Model) SetSize(w, h int) {
	m.vp.Width = w
	m.vp.Height = h
}

// SetContent replaces the viewport content and optionally scrolls to bottom.
func (m *Model) SetContent(s string) {
	m.content = s
	m.vp.SetContent(s)
	if m.tail {
		m.vp.GotoBottom()
	}
}

// AppendContent adds text to the existing content. If the viewport was at the
// bottom before the append it stays there (live-tail behavior).
func (m *Model) AppendContent(s string) {
	wasAtBottom := m.vp.AtBottom()
	m.content += s
	m.vp.SetContent(m.content)
	if wasAtBottom {
		m.vp.GotoBottom()
	}
}

// Content returns the raw content string.
func (m Model) Content() string { return m.content }

// ScrollPercent returns 0.0–1.0 indicating scroll position.
func (m Model) ScrollPercent() float64 { return m.vp.ScrollPercent() }

// AtBottom reports whether the viewport is scrolled to the end.
func (m Model) AtBottom() bool { return m.vp.AtBottom() }

// SetTail enables or disables auto-scroll on append.
func (m *Model) SetTail(on bool) { m.tail = on }

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update forwards messages to the inner viewport. User key scrolling
// automatically disables tail mode when scrolling away from the bottom.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	if _, ok := msg.(tea.KeyMsg); ok && !m.vp.AtBottom() {
		m.tail = false
	}
	return m, cmd
}

// View renders the viewport content.
func (m Model) View() string { return m.vp.View() }
