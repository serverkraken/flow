// Package spinner wraps charmbracelet/bubbles/spinner with theme-aware styling.
package spinner

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
)

// Model is a themed spinner with an adjacent label.
type Model struct {
	label   string
	spinner spinner.Model
	theme   theme.Palette
}

// New creates a spinner with the given label text.
func New(label string, p theme.Palette) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(p.Accent)
	return Model{label: label, spinner: s, theme: p}
}

// Init starts the spinner animation.
func (m Model) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update forwards tick messages to the inner spinner.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

// View renders the spinner followed by the label.
func (m Model) View() string {
	label := lipgloss.NewStyle().Foreground(m.theme.Dim).Render(m.label)
	return m.spinner.View() + " " + label
}
