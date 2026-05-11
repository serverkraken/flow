package browse

// Browse picker + edit dispatch — openWritePicker schaltet ModeWritePicker
// und initialisiert die writepicker.Model; runOnSelected baut für die
// fokussierte Note das *exec.Cmd via CmdFunc und reicht es an
// runViaExecCapture weiter. Beide Pfade sind dünne Wrapper um die
// jeweiligen Sub-Models und bilden zusammen den Cluster "User-Action
// → Subprocess". Split aus model.go (Skill §No-Monoliths).

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/serverkraken/flow/internal/kompendium/frontend/tui/writepicker"
)

// openWritePicker enters ModeWritePicker with a freshly built picker.
// Project is always offered — even when currentRepo is empty (cwd is
// not a repo, or is a repo without an `origin` remote). The actual
// `flow kompendium new project` invocation surfaces wrapProjectErr's
// hint (»cd into a repository«, »project notes need an origin
// remote«) via the runViaExecCapture stderr-passthrough — which is
// more discoverable than silently hiding the option from the menu.
//
// Pre-refactor only currentRepo!="" enabled Project, so users in a
// repo without origin (kompendium notebooks under ~/notes don't need
// one) saw a 2-option picker and had no signal why Project was
// missing. The hint-on-attempt UX is the better trade.
func (m Model) openWritePicker() (tea.Model, tea.Cmd) {
	m.picker = writepicker.New(true)
	m.mode = ModeWritePicker
	m.editErr = nil
	return m, m.picker.Init()
}

// runOnSelected resolves the cursor's note ID to a path, builds the
// edit command, and hands control to tea.ExecProcess.
func (m Model) runOnSelected(builder CmdFunc) (tea.Model, tea.Cmd) {
	if m.store == nil || builder == nil {
		return m, nil
	}
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return m, nil
	}
	id := m.visible[m.cursor].ID
	cmd := builder(m.store.Path(id))
	return m, runViaExecCapture(cmd)
}
