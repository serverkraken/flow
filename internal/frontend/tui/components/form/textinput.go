package form

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
)

// NewTextInput creates a themed text input with the given placeholder.
func NewTextInput(placeholder string, p theme.Palette) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(p.Dim)
	ti.TextStyle = lipgloss.NewStyle().Foreground(p.Fg)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(p.Accent)
	ti.CharLimit = 80
	return ti
}
