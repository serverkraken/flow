// Package form provides themed input components: a choice picker and a
// pre-styled text input constructor.
package form

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
)

// SelectedMsg is sent when the user confirms a choice with Enter.
type SelectedMsg struct {
	Index int
	Value string
}

// CancelledMsg is sent when the user presses Esc.
type CancelledMsg struct{}

// Choice is a single option in the picker.
type Choice struct {
	Label string
	Value string
}

// ChoiceModel is a bubbletea sub-model for picking one item from a list.
type ChoiceModel struct {
	choices []Choice
	cursor  int
	theme   theme.Palette
	width   int
}

// NewChoice creates a choice picker with the given options.
func NewChoice(choices []Choice, width int, p theme.Palette) ChoiceModel {
	return ChoiceModel{choices: choices, width: width, theme: p}
}

// Cursor returns the current selection index.
func (m ChoiceModel) Cursor() int { return m.cursor }

// Init implements tea.Model.
func (m ChoiceModel) Init() tea.Cmd { return nil }

// Update handles j/k navigation, Enter to select, Esc to cancel.
func (m ChoiceModel) Update(msg tea.Msg) (ChoiceModel, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "j", "down":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "enter":
			if len(m.choices) > 0 {
				c := m.choices[m.cursor]
				return m, func() tea.Msg {
					return SelectedMsg{Index: m.cursor, Value: c.Value}
				}
			}
		case "esc":
			return m, func() tea.Msg { return CancelledMsg{} }
		}
	}
	return m, nil
}

// View renders the choice list with accent-bar cursor.
func (m ChoiceModel) View() string {
	var b strings.Builder
	for i, c := range m.choices {
		b.WriteString(picker.Row(i == m.cursor, c.Label, "", m.width, m.theme))
		if i < len(m.choices)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
