package markdown_overlay

// SetSize updates the overlay's outer dimensions and re-flows the body
// through the RenderFunc at the new inner width. Call from the host's
// tea.WindowSizeMsg handler.
func (m Model) SetSize(w, h int) Model {
	m.width = w
	m.height = h
	return m.rerender()
}

// SetTitle replaces the title shown in the chrome.
func (m Model) SetTitle(title string) Model {
	m.cfg.title = title
	return m
}

// SetSource replaces the markdown body and re-renders. Clears any
// prior SetError surface: a successful body load wipes the failure
// banner. Hosts use this when the underlying document changes (e.g.
// another note loaded into the same overlay instance).
func (m Model) SetSource(src string) Model {
	m.cfg.source = src
	m.err = nil
	return m.rerender()
}

// SetError displaces the body with a tinted error banner until the
// next SetSource. Hosts use this to surface an initial-load failure
// (e.g. NoteReader.Read errored) inside the overlay frame instead of
// as a parent-level toast.
func (m Model) SetError(err error) Model {
	m.err = err
	return m
}
