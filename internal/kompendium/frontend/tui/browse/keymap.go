package browse

import "charm.land/bubbles/v2/key"

// keyMap is the central registry of bindings the browse view answers to.
// Every binding carries its own help label so the `?` overlay (bubbles/help)
// renders without a second source of truth.
type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Top      key.Binding
	Bottom   key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Filter   key.Binding
	Search   key.Binding
	Edit     key.Binding
	View     key.Binding
	New      key.Binding
	Delete   key.Binding
	Help     key.Binding
	Quit     key.Binding
}

// defaultKeys returns the default keymap. Skill §German UI: Help-Labels auf
// Deutsch. The keymap can be swapped individually without touching the
// rest of the model.
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
		Top: key.NewBinding(
			key.WithKeys("g", "home"),
			key.WithHelp("g", "Anfang"),
		),
		Bottom: key.NewBinding(
			key.WithKeys("G", "end"),
			key.WithHelp("G", "Ende"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("ctrl+u", "pgup"),
			key.WithHelp("ctrl+u", "Seite hoch"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("ctrl+d", "pgdown"),
			key.WithHelp("ctrl+d", "Seite runter"),
		),
		Filter: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "Filter wechseln"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "Suche"),
		),
		Edit: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "bearbeiten"),
		),
		View: key.NewBinding(
			key.WithKeys("v"),
			key.WithHelp("v", "Vorschau"),
		),
		New: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "neu"),
		),
		Delete: key.NewBinding(
			key.WithKeys("D"),
			key.WithHelp("D", "löschen"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "Hilfe"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "schließen"),
		),
	}
}

// ShortHelp implements bubbles/help.KeyMap. Renders on a single status line.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Edit, k.View, k.New, k.Delete, k.Filter, k.Search, k.Help, k.Quit}
}

// FullHelp implements bubbles/help.KeyMap. Renders as a multi-column table
// when the user opens the help overlay.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom, k.PageUp, k.PageDown},
		{k.Edit, k.View, k.New, k.Delete},
		{k.Filter, k.Search, k.Help, k.Quit},
	}
}
