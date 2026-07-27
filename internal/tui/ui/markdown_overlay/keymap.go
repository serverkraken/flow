package markdown_overlay

import "charm.land/bubbles/v2/key"

// keyMap collects the key bindings for opt-in features. Close keys are
// matched dynamically against config (they're configurable per call
// site); the rest are static bindings activated only when the
// corresponding feature flag is on.
type keyMap struct {
	Search    key.Binding
	NextMatch key.Binding
	PrevMatch key.Binding
	Top       key.Binding
	Bottom    key.Binding
	PageUp    key.Binding
	PageDown  key.Binding
	CopyCode  key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Search:    key.NewBinding(key.WithKeys("/")),
		NextMatch: key.NewBinding(key.WithKeys("n")),
		PrevMatch: key.NewBinding(key.WithKeys("N")),
		Top:       key.NewBinding(key.WithKeys("g", "home")),
		Bottom:    key.NewBinding(key.WithKeys("G", "end")),
		PageUp: key.NewBinding(
			key.WithKeys("pgup", "ctrl+u"),
			key.WithHelp("PgUp / Ctrl+U", "seite zurück"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown", "ctrl+d"),
			key.WithHelp("PgDn / Ctrl+D", "seite weiter"),
		),
		CopyCode: key.NewBinding(key.WithKeys("c")),
	}
}
