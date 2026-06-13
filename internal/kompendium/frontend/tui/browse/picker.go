package browse

// Browse picker + edit dispatch — openWritePicker schaltet ModeWritePicker
// und initialisiert die writepicker.Model; runOnSelected schreibt die Note
// in ein Tempfile, baut das editor-Cmd via CmdFunc und übergibt an
// prepareEditCmd/runViaExecCapture. Beide Pfade sind dünne Wrapper um die
// jeweiligen Sub-Models und bilden zusammen den Cluster "User-Action
// → Subprocess". Split aus model.go (Skill §No-Monoliths).

import (
	"context"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/kompendium/domain"
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

// runOnSelected writes the cursor's note to a tempfile, builds the edit
// command via CmdFunc, and hands control to tea.ExecProcess. The tempfile
// path (not the store path) is passed to the builder so NoteStore implementations
// without a local filesystem (apistore) work correctly.
func (m Model) runOnSelected(builder CmdFunc) (tea.Model, tea.Cmd) {
	if m.store == nil || builder == nil {
		return m, nil
	}
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return m, nil
	}
	id := m.visible[m.cursor].ID
	return m, prepareEditCmd(m.store, id, builder)
}

// prepareEditCmd writes the note to a tempfile, launches the editor via
// tea.ExecProcess, and returns an editorDoneMsg when the editor exits. The
// tempfile path is embedded in the done message so the Update reducer can
// read back the edited content and call Store.Put.
func prepareEditCmd(store interface {
	Get(ctx context.Context, id domain.ID) (domain.Note, error)
}, id domain.ID, builder CmdFunc,
) tea.Cmd {
	return func() tea.Msg {
		note, err := store.Get(context.Background(), id)
		if err != nil {
			return editorDoneMsg{id: id, err: fmt.Errorf("get note: %w", err)}
		}

		raw := note.Meta.Serialize(note.Body)

		tmp, err := os.CreateTemp("", "flow-note-*.md")
		if err != nil {
			return editorDoneMsg{id: id, err: fmt.Errorf("create tempfile: %w", err)}
		}
		tmpPath := tmp.Name()

		if _, err := tmp.Write(raw); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
			return editorDoneMsg{id: id, err: fmt.Errorf("write tempfile: %w", err)}
		}
		if err := tmp.Close(); err != nil {
			_ = os.Remove(tmpPath)
			return editorDoneMsg{id: id, err: fmt.Errorf("close tempfile: %w", err)}
		}

		cmd := builder(tmpPath)
		// Return a special message that carries the tempfile path so the
		// editorDoneMsg handler can read back the edited content.
		return editorReadyMsg{id: id, tmpPath: tmpPath, cmd: cmd, rawBefore: raw}
	}
}
