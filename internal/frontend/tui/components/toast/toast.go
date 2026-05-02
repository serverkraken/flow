// Package toast provides a self-dismissing transient message as a bubbletea sub-model.
package toast

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
)

// DismissedMsg is sent when the toast auto-dismisses.
type DismissedMsg struct{}

// Model is the bubbletea sub-model for a toast notification.
type Model struct {
	text    string
	dur     time.Duration
	visible bool
	theme   theme.Palette
}

// New creates a toast that auto-dismisses after dur.
func New(text string, dur time.Duration, p theme.Palette) Model {
	return Model{text: text, dur: dur, visible: true, theme: p}
}

// Visible reports whether the toast is still showing.
func (m Model) Visible() bool { return m.visible }

// Init starts the dismiss timer.
func (m Model) Init() tea.Cmd {
	return tea.Tick(m.dur, func(time.Time) tea.Msg { return DismissedMsg{} })
}

// Update hides the toast on DismissedMsg.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if _, ok := msg.(DismissedMsg); ok {
		m.visible = false
	}
	return m, nil
}

// View renders the toast or an empty string when dismissed.
func (m Model) View() string {
	if !m.visible {
		return ""
	}
	return lipgloss.NewStyle().Foreground(m.theme.Green).Bold(true).
		Render("✓ " + m.text)
}
