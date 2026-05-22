package browse

// Browse update path — Update-Reducer plus die Key-Handler-Kette
// (handleKey → handleNormalKey → handleNavKey / handleActionKey),
// Mode-spezifische Pfade (Search, ConfirmDelete, View, WritePicker),
// Mouse-Handling und startConfirmDelete. Split aus model.go (Skill
// §No-Monoliths): Reducer-Routing trennt sich sauber von Types/
// Konstruktion (model.go), Rendering (view.go) und Side-Effect-Cmds
// (commands.go).

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown_overlay"
	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/frontend/tui/writepicker"
)

// Update is the Bubble Tea reducer.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.mode == ModeView {
		return m.updateViewer(msg)
	}
	if m.mode == ModeWritePicker {
		return m.updatePicker(msg)
	}
	switch msg := msg.(type) {
	case entriesLoadedMsg:
		m.all = msg.entries
		m.loadErr = msg.err
		m.loaded = true
		m.applyFilters()
		m.refreshPreview()
		if m.store != nil && len(m.all) > 0 {
			return m, loadBodiesCmd(m.store, m.all)
		}
		return m, nil
	case bodiesLoadedMsg:
		m.bodies = msg.bodies
		m.applyFilters()
		m.refreshPreview()
		return m, nil
	case editFinishedMsg:
		if msg.err != nil {
			m.editErr = msg.err
			return m, nil
		}
		m.editErr = nil
		// Drop the rendered-preview cache + previewID so the next
		// refreshPreview (triggered by entriesLoadedMsg below) re-
		// reads the just-saved file from the store. Without this,
		// `if m.previewID == e.ID { return }` short-circuits and the
		// user keeps seeing the pre-edit body until they cursor away
		// and back.
		m.previewCached = map[domain.ID]string{}
		m.previewID = ""
		return m, loadEntriesCmd(m.list, m.currentRepo)
	case deleteFinishedMsg:
		if msg.err != nil {
			m.editErr = msg.err
			return m, nil
		}
		m.editErr = nil
		return m, loadEntriesCmd(m.list, m.currentRepo)
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.helpUI.Width = m.width
		m.layoutViewport()
		// Invalidate the cached preview AND drop previewID so
		// refreshPreview's `if previewID == e.ID { return }` short-
		// circuit doesn't keep the old-width rendering on screen
		// after a tmux pane resize / window resize.
		m.previewCached = map[domain.ID]string{}
		m.previewID = ""
		m.refreshPreview()
		if !m.loaded {
			return m, m.spin.Tick
		}
		return m, nil
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case ModeSearch:
		return m.handleSearchKey(msg)
	case ModeConfirmDelete:
		return m.handleConfirmDeleteKey(msg)
	}
	return m.handleNormalKey(msg)
}

// updateViewer routes every message to the active viewer sub-model
// while in ModeView. The window-size message is intercepted so the
// list pane has fresh dimensions when the viewer exits, and ExitMsg
// returns the model to ModeNormal.
func (m Model) updateViewer(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.helpUI.Width = m.width
		m.layoutViewport()
		m.previewCached = map[domain.ID]string{}
		m.previewID = ""
		m.refreshPreview()
		m.viewer = m.viewer.SetSize(m.width, m.height)
		return m, nil
	case markdown_overlay.ExitMsg:
		m.mode = ModeNormal
		m.viewer = markdown_overlay.Model{}
		return m, nil
	}
	var cmd tea.Cmd
	m.viewer, cmd = m.viewer.Update(msg)
	return m, cmd
}

func (m Model) handleNormalKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.showHelp {
		if key.Matches(msg, m.keys.Help) || key.Matches(msg, m.keys.Quit) || msg.String() == "esc" {
			m.showHelp = false
		}
		return m, nil
	}
	if model, cmd, handled := m.handleNavKey(msg); handled {
		return model, cmd
	}
	if model, cmd, handled := m.handleActionKey(msg); handled {
		return model, cmd
	}
	return m, nil
}

// handleNavKey handles cursor movement keys. Returns handled=true when one
// of the nav bindings matched so the caller doesn't fall through.
func (m Model) handleNavKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.visible)-1 {
			m.cursor++
			m.refreshPreview()
		}
	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
			m.refreshPreview()
		}
	case key.Matches(msg, m.keys.Top):
		m.cursor = 0
		m.refreshPreview()
	case key.Matches(msg, m.keys.Bottom):
		if len(m.visible) > 0 {
			m.cursor = len(m.visible) - 1
			m.refreshPreview()
		}
	case key.Matches(msg, m.keys.PageUp):
		m.cursor = max(0, m.cursor-m.pageJump())
		m.refreshPreview()
	case key.Matches(msg, m.keys.PageDown):
		m.cursor = min(len(m.visible)-1, m.cursor+m.pageJump())
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.refreshPreview()
	default:
		return m, nil, false
	}
	return m, nil, true
}

// handleActionKey handles non-navigation bindings (filter, search, edit,
// view, new, delete, help, quit).
func (m Model) handleActionKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		m.quitting = true
		return m, tea.Quit, true
	case key.Matches(msg, m.keys.Filter):
		m.filter = (m.filter + 1) % 4
		m.applyFilters()
		m.refreshPreview()
		return m, nil, true
	case key.Matches(msg, m.keys.Search):
		m.mode = ModeSearch
		m.search.Focus()
		return m, textinput.Blink, true
	case key.Matches(msg, m.keys.Edit):
		model, cmd := m.runOnSelected(m.editCmd)
		return model, cmd, true
	case key.Matches(msg, m.keys.View):
		return m.openViewer(), nil, true
	case key.Matches(msg, m.keys.New):
		model, cmd := m.openWritePicker()
		return model, cmd, true
	case key.Matches(msg, m.keys.Delete):
		return m.startConfirmDelete(), nil, true
	case key.Matches(msg, m.keys.Help):
		m.showHelp = true
		return m, nil, true
	}
	return m, nil, false
}

func (m Model) handleSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = ModeNormal
		m.search.SetValue("")
		m.search.Blur()
		m.applyFilters()
		m.refreshPreview()
		return m, nil
	case "enter":
		m.mode = ModeNormal
		m.search.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	prev := m.search.Value()
	m.search, cmd = m.search.Update(msg)
	if m.search.Value() != prev {
		m.applyFilters()
		m.refreshPreview()
	}
	return m, cmd
}

// startConfirmDelete switches into ModeConfirmDelete and stashes the
// cursor's note ID. No-op when the cursor is on no entry or the delete
// use case wasn't wired (e.g. tests passing nil).
func (m Model) startConfirmDelete() Model {
	if m.delete == nil {
		return m
	}
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return m
	}
	m.deleteTargetID = m.visible[m.cursor].ID
	m.mode = ModeConfirmDelete
	return m
}

// handleConfirmDeleteKey — kanonisches y/Enter → ja, n/Esc → nein
// (Skill §Keybind grammar). Vorher fehlte Enter als Confirm-Variante,
// was die Konvention der restlichen Codebase (confirm.Model) uneinheitlich
// machte.
func (m Model) handleConfirmDeleteKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		id := m.deleteTargetID
		m.mode = ModeNormal
		m.deleteTargetID = ""
		return m, deleteCmd(m.delete, id)
	case "n", "N", "esc", "ctrl+c":
		m.mode = ModeNormal
		m.deleteTargetID = ""
	}
	return m, nil
}

// handleMouse handles wheel scrolling. Under bubbletea v2, wheel
// events arrive as a dedicated tea.MouseWheelMsg (no separate press /
// release flavours — a wheel notch is atomic), so the v1 check
// `msg.Action != MouseActionPress` evaporates.
func (m Model) handleMouse(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	switch msg.Mouse().Button {
	case tea.MouseWheelUp:
		if m.cursor > 0 {
			m.cursor--
			m.refreshPreview()
		}
	case tea.MouseWheelDown:
		if m.cursor < len(m.visible)-1 {
			m.cursor++
			m.refreshPreview()
		}
	}
	return m, nil
}

// updatePicker is the reducer-branch active while ModeWritePicker is
// the input mode. The picker emits writepicker.DoneMsg when the user
// either selects a type (with optional slug) or cancels; we harvest
// the Result, return to ModeNormal, and — when the choice was not
// Cancel — fork the corresponding `flow kompendium new <type>`
// subcommand via tea.ExecProcess. That subcommand is a plain CLI
// (creates the file, opens nvim), not another tea.Program, so the
// nested-tea problem that motivated this whole refactor doesn't
// recur.
func (m Model) updatePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.helpUI.Width = m.width
		next, cmd := m.picker.Update(msg)
		m.picker = next.(writepicker.Model)
		return m, cmd
	case writepicker.DoneMsg:
		m.mode = ModeNormal
		m.picker = writepicker.Model{}
		if msg.Result.Choice == writepicker.ChoiceCancel || m.writeCmd == nil {
			return m, nil
		}
		cmd := m.writeCmd(msg.Result)
		if cmd == nil {
			return m, nil
		}
		return m, runViaExecCapture(cmd)
	}
	next, cmd := m.picker.Update(msg)
	m.picker = next.(writepicker.Model)
	return m, cmd
}
