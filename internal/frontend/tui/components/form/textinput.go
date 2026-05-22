// Package form bundles the themed bubbles widgets the worktime / kompendium
// TUIs share (text inputs, in particular). Centralising the styling keeps
// every dialog input visually consistent across screens.
package form

import (
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// NewTextInput creates a themed text input with the given placeholder.
func NewTextInput(placeholder string, p theme.Palette) textinput.Model {
	sem := p.Sem()
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(p.FgMuted)
	ti.TextStyle = lipgloss.NewStyle().Foreground(p.Fg)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(sem.Accent)
	ti.CharLimit = 80
	return ti
}
