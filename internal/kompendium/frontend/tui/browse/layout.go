package browse

// Browse layout math — alle Pure-Helper, die aus m.width / m.height
// einzelne Pane-Dimensionen ableiten. Split aus model.go (Skill
// §No-Monoliths): layout-Berechnung formt einen klaren Cluster
// neben Update-Routing, Rendering und Preview-State.

// listRows returns how many terminal lines the list pane has to work
// with. Chrome budget: outer rounded frame (2), three header lines
// (headline + separator + status) plus a blank, list panel title +
// blank, blank + footer + status bar, plus one reserved line for the
// paginator dots (always reserved so layout doesn't shift when the
// list crosses the dot threshold). Conservative — undercounting just
// trims a row, overcounting clips chrome, which is louder.
func (m Model) listRows() int {
	rows := m.height - 12
	if rows < 5 {
		return 5
	}
	return rows
}

// pageJump is the cursor delta for PageUp/PageDown. Entries can be 1–4
// rendered lines tall, so a fixed jump-by-N-entries either flies past
// the screen on dense lists or barely scrolls on sparse ones. Halving
// listRows() and clamping to a sane minimum keeps the jump close to "a
// screen of content" without re-rendering all rows just to count.
func (m Model) pageJump() int {
	jump := m.listRows() / 2
	if jump < 3 {
		return 3
	}
	return jump
}

// layoutViewport sets the preview viewport's dimensions from the current
// window size.
func (m *Model) layoutViewport() {
	w, h := m.previewSize()
	m.preview.SetWidth(max(0, w))
	m.preview.SetHeight(max(0, h))
}

// previewPaneWidth is the OUTER width of the preview pane — including
// the panel's NormalBorder. Zero when there's no preview pane.
func (m Model) previewPaneWidth() int {
	if !m.twoPane() {
		return 0
	}
	w := m.contentWidth() - m.listPaneWidth() - 2 // 2 = gap between panes
	if w < 0 {
		w = 0
	}
	return w
}

// previewSize is the INNER content area Glamour wraps to and the
// viewport renders into. We subtract two each from width and from
// height to reserve the panel's NormalBorder (left+right, top+bottom)
// — without that the Glamour-rendered lines were exactly two cells
// wider than the panel interior, lipgloss soft-wrapped each line into
// two, and a long Markdown body blew the body up to twice its planned
// height. The vertical budget then mirrors the list pane so
// JoinHorizontal stacks both at the same height.
func (m Model) previewSize() (int, int) {
	paneW := m.previewPaneWidth()
	if paneW <= 0 {
		return 0, 0
	}
	innerW := paneW - 2
	if innerW < 1 {
		innerW = 1
	}
	innerH := m.contentHeight() - 8
	if innerH < 1 {
		innerH = 1
	}
	return innerW, innerH
}

func (m Model) twoPane() bool {
	return m.width >= twoPaneMinWidth && m.height >= 18
}

// contentWidth is the width inside the outer rounded frame.
func (m Model) contentWidth() int {
	if m.width <= 4 {
		return 0
	}
	return m.width - 4 // 2 border + 2 padding
}

func (m Model) contentHeight() int {
	if m.height <= 4 {
		return 0
	}
	return m.height - 4
}

func (m Model) listPaneWidth() int {
	if !m.twoPane() {
		return m.contentWidth()
	}
	// 1/3 für die Liste statt vormals 4/10 (UX-Review L3): die Vorschau
	// ist der eigentliche Lese-Inhalt und profitiert von Breite, während
	// die Liste mit Titeln + Badges schmaler auskommt. min 30 hält die
	// Titel nahe der Schwelle lesbar, ohne die Preview zu strangulieren.
	w := m.contentWidth() / 3
	if w < 30 {
		w = 30
	}
	return w
}
