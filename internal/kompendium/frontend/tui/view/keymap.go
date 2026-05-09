package view

import "github.com/charmbracelet/bubbles/key"

// keyMap is the central registry of bindings the in-process viewer
// answers to. Mirrors the browse view's pattern so help/`?` overlays
// stay consistent across the TUI.
type keyMap struct {
	Up        key.Binding
	Down      key.Binding
	PageUp    key.Binding
	PageDown  key.Binding
	Top       key.Binding
	Bottom    key.Binding
	Search    key.Binding
	NextMatch key.Binding
	PrevMatch key.Binding
	CopyCode  key.Binding
	Quit      key.Binding
}

// defaultKeys returns the default keymap. less/vim conventions: j/k for
// scroll, /n/N for search, q/Esc to leave.
func defaultKeys() keyMap {
	return keyMap{
		Up: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k/↑", "hoch"),
		),
		Down: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j/↓", "runter"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("ctrl+u", "pgup"),
			key.WithHelp("ctrl+u", "Seite hoch"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("ctrl+d", "pgdown", " "),
			key.WithHelp("ctrl+d", "Seite runter"),
		),
		Top: key.NewBinding(
			key.WithKeys("g", "home"),
			key.WithHelp("g", "Anfang"),
		),
		Bottom: key.NewBinding(
			key.WithKeys("G", "end"),
			key.WithHelp("G", "Ende"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "suchen"),
		),
		NextMatch: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "nächster Treffer"),
		),
		PrevMatch: key.NewBinding(
			key.WithKeys("N"),
			key.WithHelp("N", "vorheriger Treffer"),
		),
		CopyCode: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "Code kopieren"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "esc", "ctrl+c"),
			key.WithHelp("q/Esc", "zurück"),
		),
	}
}
