package project_picker

import (
	tea "charm.land/bubbletea/v2"
)

// Update handles all incoming bubbletea messages for the picker.
//
// Key routing:
//   - WindowSizeMsg → SetSize
//   - up            → move cursor up (wraps)
//   - down          → move cursor down (wraps)
//   - tab           → jump to "+ Neues Projekt anlegen"
//   - enter         → pick selected item or create new project
//   - esc           → emit onCancel
//   - backspace     → remove last rune from filter
//   - <rune>        → append to filter (including j, k)
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.SetSize(msg.Width, msg.Height), nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "up":
		m.cursor--
		if m.cursor < 0 {
			m.cursor = m.neuIdx()
		}
		return m, nil

	case "down":
		m.cursor++
		if m.cursor > m.neuIdx() {
			m.cursor = 0
		}
		return m, nil

	case "tab":
		m.cursor = m.neuIdx()
		return m, nil

	case "enter":
		return m.handleEnter()

	case "esc":
		cancel := m.onCancel
		return m, func() tea.Msg { return cancel }

	case "backspace":
		if len([]rune(m.filter)) > 0 {
			runes := []rune(m.filter)
			m.filter = string(runes[:len(runes)-1])
			m.applyFilter()
		}
		return m, nil
	}

	// Printable single-rune: append to filter.
	if isTypeable(msg) {
		m.filter += msg.Text
		m.applyFilter()
		// After filter change, if cursor now exceeds the list (fewer results),
		// it was already clamped by applyFilter.
		return m, nil
	}

	return m, nil
}

// handleEnter picks the currently focused item or fires onCreate.
func (m Model) handleEnter() (Model, tea.Cmd) {
	if m.cursor == m.neuIdx() {
		// "+ Neues Projekt anlegen" row selected.
		name := m.filter
		create := m.onCreate
		return m, func() tea.Msg { return create(name) }
	}
	if m.cursor < len(m.filtered) {
		p := m.filtered[m.cursor]
		pick := m.onPick
		return m, func() tea.Msg { return pick(p) }
	}
	return m, nil
}

// isTypeable returns true when the KeyPressMsg should be appended to the
// filter. A message qualifies when it has a non-empty Text field (the
// bubbletea v2 convention for printable runes) and is not a special control
// key that we handle explicitly above.
func isTypeable(msg tea.KeyPressMsg) bool {
	if msg.Text == "" {
		return false
	}
	// Control characters and modifiers produce Text of length > 1 or
	// have non-zero Mod. Single printable rune with no modifier.
	if msg.Mod != 0 {
		return false
	}
	runes := []rune(msg.Text)
	if len(runes) != 1 {
		return false
	}
	r := runes[0]
	return r >= ' ' && r != 127
}
