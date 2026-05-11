package worktime

// Heute dialog key dispatch — Top-Level handleDialogKey verteilt nach
// h.dialog auf die spezifischen Eingabepfade (Simple-Input für Tag/
// Notiz/NoteAttach, Form-Navigation für Edit, Confirm-Forward für
// Delete, Help-Dismiss). Split aus today_dialog.go (Skill §No-Monoliths).

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
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
	case heuteDialogTag, heuteDialogNote, heuteDialogNoteAttach:
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
	// NoteAttach im Picker-Modus claimt Up/Down für die Suggestion-
	// Liste. Tag/Note haben keine Liste, ihre Up/Down fallen auf den
	// Default-Pfad (Textinput-Cursor — nicht relevant für single-line).
	if h.dialog == heuteDialogNoteAttach && len(h.noteSuggestions) > 0 {
		switch msg.String() {
		case "up", "ctrl+p":
			filt := h.filteredNoteSuggestions()
			if n := len(filt); n > 0 {
				h.noteSuggCur = (h.noteSuggCur + n - 1) % n
			}
			return h, nil
		case "down", "ctrl+n":
			filt := h.filteredNoteSuggestions()
			if n := len(filt); n > 0 {
				h.noteSuggCur = (h.noteSuggCur + 1) % n
			}
			return h, nil
		}
	}
	switch msg.Type {
	case tea.KeyEsc:
		h.dialog = heuteDialogNone
		h.input.Blur()
		h.input.SetValue("")
		h.errMsg = ""
		h.noteSuggestions = nil
		h.noteSuggCur = 0
		return h, nil
	case tea.KeyEnter:
		return h.submitDialog()
	case tea.KeyTab, tea.KeyShiftTab:
		// Single-input dialogs (Tag/Note/NoteAttach) have nowhere to
		// tab to — swallow the key instead of letting bubbles textinput
		// insert a literal tab character that would survive into the
		// stored field. The tsvsessions writer now sanitises tab/CR/LF
		// at write time too, but rejecting at the input boundary is
		// less surprising for the user.
		return h, nil
	}
	h.errMsg = ""
	var cmd tea.Cmd
	h.input, cmd = h.input.Update(msg)
	// Filter-Reset des Cursors: bei Tippen auf NoteAttach kann der
	// vorherige cursor-Index out-of-range geraten, wenn das Filter
	// die Liste verkürzt.
	if h.dialog == heuteDialogNoteAttach {
		filt := h.filteredNoteSuggestions()
		if h.noteSuggCur >= len(filt) {
			h.noteSuggCur = 0
		}
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
