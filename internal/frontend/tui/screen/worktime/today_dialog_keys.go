package worktime

// Heute dialog key dispatch — Top-Level handleDialogKey verteilt nach
// h.dialog auf die spezifischen Eingabepfade (Simple-Input für Tag/
// Notiz, geteilter noteAttachPicker für NoteAttach, Form-Navigation
// für Edit, Confirm-Forward für Delete, Help-Dismiss). Split aus
// today_dialog.go (Skill §No-Monoliths).

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

func (h heute) handleDialogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch h.dialog {
	case heuteDialogDelete:
		// Confirm-Dialog forwarded an die kanonische confirm.Model. Der
		// resultierende confirm.ResultMsg landet im Outer-Update und löst
		// h.deleteCmd aus (oder schließt den Dialog ohne Aktion).
		if h.confirmModel == nil {
			h.dialog = heuteDialogNone
			return h, nil
		}
		updated, cmd := h.confirmModel.Update(msg)
		h.confirmModel = &updated
		return h, cmd
	case heuteDialogEdit:
		return h.handleFormKey(msg)
	case heuteDialogNoteAttach:
		return h.handleNoteAttachKey(msg)
	case heuteDialogTag, heuteDialogNote:
		return h.handleSimpleInputKey(msg)
	case heuteDialogHelp:
		// Help-Overlay schließt explizit auf Esc oder ?. Andere Tasten
		// laufen normal weiter, damit der User direkt nach dem
		// Erinnern-an-die-Tastenbelegung nicht nochmal drücken muss
		// (z.B. `?` öffnet Help, dann `s` zum Starten — ohne diese
		// Logik wurde `s` als Dialog-Dismiss verschluckt).
		switch msg.String() {
		case "esc", "?", "q":
			h.dialog = heuteDialogNone
			return h, nil
		}
		h.dialog = heuteDialogNone
		return h.handleNormalKey(msg)
	case heuteDialogNoteView:
		if h.noteView == nil {
			h.dialog = heuteDialogNone
			return h, nil
		}
		upd, cmd := h.noteView.Update(msg)
		h.noteView = &upd
		return h, cmd
	}
	return h, nil
}

func (h heute) handleSimpleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		h.dialog = heuteDialogNone
		h.input.Blur()
		h.input.SetValue("")
		h.errMsg = ""
		return h, nil
	case tea.KeyEnter:
		return h.submitDialog()
	case tea.KeyTab, tea.KeyShiftTab:
		// Single-input dialogs (Tag/Note) have nowhere to tab to —
		// swallow the key instead of letting bubbles textinput insert
		// a literal tab character that would survive into the stored
		// field. The tsvsessions writer now sanitises tab/CR/LF at
		// write time too, but rejecting at the input boundary is less
		// surprising for the user.
		return h, nil
	}
	h.errMsg = ""
	var cmd tea.Cmd
	h.input, cmd = h.input.Update(msg)
	return h, cmd
}

// handleNoteAttachKey delegiert an das geteilte noteAttachPicker-
// Widget. Submit fuehrt durch submitDialog (das die Picker-Daten
// ueber SelectedID konsumiert); Cancel schliesst den Dialog ohne
// weitere Aktion. Idle uebernimmt nur den neuen Picker-State
// (Suggestion-Cursor bewegt, Input getippt, …).
func (h heute) handleNoteAttachKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	picker, cmd, action := h.notePicker.Update(msg)
	h.notePicker = picker
	switch action {
	case noteAttachActionCancel:
		h.dialog = heuteDialogNone
		h.errMsg = ""
		return h, nil
	case noteAttachActionSubmit:
		return h.submitDialog()
	}
	return h, cmd
}

func (h heute) handleFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	maxCur := len(h.form) - 1
	switch msg.Type {
	case tea.KeyEsc:
		h.dialog = heuteDialogNone
		h.form = nil
		h.formCur = 0
		h.errMsg = ""
		return h, nil
	case tea.KeyTab, tea.KeyDown:
		next := h.formCur + 1
		if next > maxCur {
			next = 0
		}
		h.focusForm(next)
		return h, textinput.Blink
	case tea.KeyShiftTab, tea.KeyUp:
		next := h.formCur - 1
		if next < 0 {
			next = maxCur
		}
		h.focusForm(next)
		return h, textinput.Blink
	case tea.KeyEnter:
		if h.formCur < maxCur {
			h.focusForm(h.formCur + 1)
			return h, textinput.Blink
		}
		return h.submitDialog()
	}
	h.errMsg = ""
	if h.formCur >= 0 && h.formCur < len(h.form) {
		var cmd tea.Cmd
		h.form[h.formCur], cmd = h.form[h.formCur].Update(msg)
		return h, cmd
	}
	return h, nil
}

func (h *heute) focusForm(i int) {
	for j := range h.form {
		if j == i {
			h.form[j].Focus()
		} else {
			h.form[j].Blur()
		}
	}
	h.formCur = i
}
