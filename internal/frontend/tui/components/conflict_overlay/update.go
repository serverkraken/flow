package conflict_overlay

import (
	tea "charm.land/bubbletea/v2"
)

// Update routes incoming messages.
//
//   - tea.WindowSizeMsg  → SetSize (re-flow layout)
//   - tea.KeyPressMsg    → match against variant choices; Esc → CancelMsg
//   - all others         → model unchanged, no cmd
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.SetSize(msg.Width, msg.Height), nil

	case tea.KeyPressMsg:
		key := msg.String()

		// Esc always cancels.
		if key == "esc" {
			return m, cancelCmd()
		}

		// Match against variant-specific choices.
		for _, c := range m.choices {
			if key == c.key {
				cb := c.callback
				return m, func() tea.Msg { return cb() }
			}
		}
	}
	return m, nil
}

// cancelCmd emits a CancelMsg.
func cancelCmd() tea.Cmd {
	return func() tea.Msg { return CancelMsg{} }
}
