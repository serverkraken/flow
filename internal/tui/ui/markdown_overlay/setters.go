package markdown_overlay

// SetSize updates the overlay's outer dimensions and re-flows the body
// through the RenderFunc at the new inner width. Call from the host's
// tea.WindowSizeMsg handler.
func (m Model) SetSize(w, h int) Model {
	m.width = w
	m.height = h
	return m.rerender()
}

// Rerender re-runs the RenderFunc at the current width (e.g. after the
// host changed focus state the render closure reads) and refreshes the
// viewport content, preserving the existing scroll position. SetContent
// does not reset yOffset, so the offset is maintained naturally through
// the rerender() → viewport.SetContent() call chain.
func (m Model) Rerender() Model {
	return m.SetSize(m.width, m.height)
}

// SetTitle replaces the title shown in the chrome. Does NOT rerender
// (asymmetric to SetSource): the title is read live from cfg.title by
// the chrome on every View(), so the next paint picks up the change
// without re-flowing the body.
func (m Model) SetTitle(title string) Model {
	m.cfg.title = title
	return m
}

// SetRender swaps the RenderFunc and re-renders in place, preserving the
// current scroll position (rerender → viewport.SetContent does not reset
// yOffset). Hosts use this when the render closure must be rebound to refreshed
// state (e.g. a same-document SSE reload that updates title / tags / backlinks)
// without rebuilding the overlay — which would snap scroll back to the top.
func (m Model) SetRender(render RenderFunc) Model {
	m.render = render
	return m.rerender()
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
