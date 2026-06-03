package conflict_overlay

import (
	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// NewGenericFallback builds a minimal overlay for conflicts whose
// Local/Server payloads could not be decoded. It uses the VariantSessionEdit
// shape so the chrome renders the same border + title, but the body says
// "Details fehlen" and the only meaningful choice is [esc] abbrechen
// (VariantSessionEdit already exposes [s] and [l]; the callbacks here emit
// CancelMsg so both keys are safe to press — they all close the overlay
// without making a state change).
func NewGenericFallback(p theme.Palette) Model {
	return Model{
		variant: VariantSessionEdit,
		title:   "Sync-Konflikt",
		body:    "Sync-Konflikt — Details fehlen\n(Payload konnte nicht dekodiert werden)",
		palette: p,
		choices: []choice{
			// Map both session-edit keys to a no-op cancel so the user can
			// dismiss with any of [s], [l], or [esc].
			{
				key:      "s",
				label:    "(Abbrechen — keine Daten verfügbar)",
				callback: func() tea.Msg { return CancelMsg{} },
			},
			{
				key:      "l",
				label:    "(Abbrechen — keine Daten verfügbar)",
				callback: func() tea.Msg { return CancelMsg{} },
			},
		},
	}
}
