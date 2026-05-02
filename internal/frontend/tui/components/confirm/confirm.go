// Package confirm provides a yes/no dialog as a bubbletea sub-model.
package confirm

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
)

// ResultMsg is sent when the user confirms or denies.
type ResultMsg struct {
	Confirmed bool
}

// Model is the bubbletea sub-model for a yes/no dialog.
type Model struct {
	question string
	detail   string
	theme    theme.Palette
}

// New creates a confirm dialog. question is shown prominently; detail is
// optional context rendered below it.
func New(question, detail string, p theme.Palette) Model {
	return Model{question: question, detail: detail, theme: p}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update handles y/Enter (confirm) and n/Esc (deny).
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "y", "enter":
			return m, confirmed(true)
		case "n", "esc":
			return m, confirmed(false)
		}
	}
	return m, nil
}

// View renders the dialog.
func (m Model) View() string {
	q := lipgloss.NewStyle().Foreground(m.theme.Yellow).Bold(true).Render(m.question)
	var detail string
	if m.detail != "" {
		detail = "\n" + lipgloss.NewStyle().Foreground(m.theme.Fg).Render(m.detail)
	}
	hint := lipgloss.NewStyle().Foreground(m.theme.Dim).Render("y/Enter → ja  ·  n/Esc → nein")
	return q + detail + "\n\n" + hint
}

func confirmed(yes bool) tea.Cmd {
	return func() tea.Msg { return ResultMsg{Confirmed: yes} }
}
