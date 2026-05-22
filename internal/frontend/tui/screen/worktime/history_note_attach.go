package worktime

// History note-attach — `n` in der Drill-Sicht hängt eine Kompendium-
// Note an den fokussierten Tag (h.drillDate) an. Spiegelt das Heute-
// Verhalten (today_dialog_*.go) für vergangene Tage; die User-Story
// "Note retrospektiv anhängen" war vorher nicht erreichbar.
//
// Implementierung delegiert an noteAttachPicker (note_attach_picker.go),
// damit der Picker-Code (Input + Suggestion-Liste + Filter + Cursor)
// nicht dupliziert wird. Hier nur Glue: Dialog-State, Key-Routing,
// LinkWriter-Submit, historyActionDoneMsg-Conversion.

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// openNoteAttachDialog aktiviert den Note-Attach-Picker für den
// gerade gedrillten Tag. attached-Liste wird sync via LinkReader
// geladen (kleine TSV-Datei; das Pattern ist konsistent mit
// today_dialog_open.go's sync NoteLister.Recent-Aufruf).
func (h history) openNoteAttachDialog() (history, tea.Cmd) {
	if h.drillDate.IsZero() {
		return h, nil
	}
	var attached []string
	if h.deps.LinkReader != nil {
		if ids, err := h.deps.LinkReader.ListByDate(h.drillDate); err == nil {
			attached = ids
		}
	}
	picker, cmd := h.notePicker.Open(h.drillDate, attached)
	h.notePicker = picker
	h.dialog = historyDialogDrillNoteAttach
	h.errMsg = ""
	return h, cmd
}

// handleDrillNoteAttachKey routet KeyMsgs in den Picker und reagiert
// auf seine Action-Verdikte: Submit ruft LinkWriter.Add via tea.Cmd
// (Resultat landet als historyActionDoneMsg), Cancel schließt zurück
// in den Drill-View, Idle übernimmt nur den neuen Picker-State.
func (h history) handleDrillNoteAttachKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	picker, cmd, action := h.notePicker.Update(msg)
	h.notePicker = picker
	switch action {
	case noteAttachActionCancel:
		h.dialog = historyDialogDrill
		h.errMsg = ""
		return h, nil
	case noteAttachActionSubmit:
		id := h.notePicker.SelectedID()
		if id == "" {
			h.notePicker = h.notePicker.SetError("Note-ID darf nicht leer sein")
			return h, nil
		}
		// Pre-close the dialog so the resulting toast lands on the
		// regular drill view; on error the historyActionDoneMsg handler
		// surfaces h.errMsg next to the session list.
		date := h.drillDate
		writer := h.deps.LinkWriter
		h.dialog = historyDialogDrill
		return h, func() tea.Msg {
			if err := writer.Add(date, id); err != nil {
				return historyActionDoneMsg{err: err, date: date}
			}
			return historyActionDoneMsg{
				date:  date,
				toast: fmt.Sprintf("✓ Note %s angehängt an %s", id, date.Format("2006-01-02")),
			}
		}
	}
	return h, cmd
}
