package usecase

import (
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// StateManager loads, persists, and restores the TUI's screen / filter /
// cursor state. ConsumeNextScreen takes precedence over the persisted
// state when present — that's how `goto.sh` deep-links into a specific
// screen.
type StateManager struct {
	Store ports.FlowStateStore
}

// Restore returns the state to start the TUI in. Order of precedence:
//
//  1. The one-shot next-screen file (if present, overrides screen and
//     resets filter+cursor to defaults).
//  2. The persisted state.json.
//  3. domain.DefaultFlowState() when both are missing.
func (m *StateManager) Restore() (domain.FlowState, error) {
	state, err := m.Store.Load()
	if err != nil {
		return domain.DefaultFlowState(), err
	}
	next, _ := m.Store.ConsumeNextScreen()
	if next != "" {
		state.Screen = next
		state.Filter = ""
		state.Cursor = 0
	}
	return state, nil
}

// Save persists the given state.
func (m *StateManager) Save(s domain.FlowState) error {
	return m.Store.Save(s)
}

// WriteNextScreen writes a one-shot deep-link target. Used by external
// tools (goto.sh) that want flow to come up on a specific screen on next
// launch.
func (m *StateManager) WriteNextScreen(screen string) error {
	return m.Store.WriteNextScreen(screen)
}
