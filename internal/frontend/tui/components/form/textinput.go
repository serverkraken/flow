// Package form bundles the themed bubbles widgets the worktime / kompendium
// TUIs share (text inputs, in particular). Centralising the styling keeps
// every dialog input visually consistent across screens.
package form

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// NewTextInput creates a themed text input with the given placeholder.
//
// Under bubbles v2 textinput exposes styling exclusively through the
// Styles struct (Focused/Blurred state pair plus a Cursor sub-struct);
// the v1 top-level fields PlaceholderStyle, TextStyle and Cursor.Style
// are gone. We populate both focus states with the same colours so the
// blurred render of a dialog input stays legible — Fg foreground on the
// text, FgMuted on the placeholder, accent on the cursor.
//
// Cursor.Shape is set to tea.CursorBar so the insertion point reads
// as a thin vertical bar at the gap between cells, matching the
// expectation users carry over from browsers and modern editors. The
// v2 default (CursorBlock) painted a full inverse cell that obscured
// the character behind the cursor and felt out of place in a prose
// dialog input — a real regression in the user's UX when typing slugs,
// search queries, and Heute edit-dialog fields.
func NewTextInput(placeholder string, p theme.Palette) textinput.Model {
	sem := p.Sem()
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 80
	styles := ti.Styles()
	body := lipgloss.NewStyle().Foreground(p.Fg)
	muted := lipgloss.NewStyle().Foreground(p.FgMuted)
	styles.Focused.Text = body
	styles.Focused.Placeholder = muted
	styles.Blurred.Text = body
	styles.Blurred.Placeholder = muted
	styles.Cursor.Color = sem.Accent
	styles.Cursor.Shape = tea.CursorBar
	ti.SetStyles(styles)
	return ti
}
