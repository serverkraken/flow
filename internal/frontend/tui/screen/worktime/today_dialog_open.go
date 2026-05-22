package worktime

// Heute dialog openers — Konstruktoren für Tag-, Notiz-, Edit-,
// NoteAttach- und Delete-Dialog. Split aus today_dialog.go (Skill
// §No-Monoliths): Dialog-Aufmach-Code formt einen klaren Cluster
// neben Key-Dispatch (today_dialog_keys.go) und Submit
// (today_dialog_submit.go); renderDialog bleibt in today_dialog.go.

import (
	"fmt"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/frontend/tui/components/confirm"
	"github.com/serverkraken/flow/internal/frontend/tui/components/form"
)

func (h heute) openTagDialog() (tea.Model, tea.Cmd) {
	s := h.day.Sessions[h.cursor]
	h.editIdx = h.cursor
	h.editDate = s.Date
	h.dialog = heuteDialogTag
	h.input = form.NewTextInput("tag (z.B. deep, meeting)", h.pal)
	h.input.SetValue(s.Tag)
	h.input.Focus()
	h.errMsg = ""
	return h, textinput.Blink
}

func (h heute) openNoteDialog() (tea.Model, tea.Cmd) {
	s := h.day.Sessions[h.cursor]
	h.editIdx = h.cursor
	h.editDate = s.Date
	h.dialog = heuteDialogNote
	h.input = form.NewTextInput("kurzer Text", h.pal)
	h.input.SetValue(s.Note)
	h.input.Focus()
	h.errMsg = ""
	return h, textinput.Blink
}

func (h heute) openEditDialog() (tea.Model, tea.Cmd) {
	s := h.day.Sessions[h.cursor]
	h.editIdx = h.cursor
	h.editDate = s.Date
	h.dialog = heuteDialogEdit

	start := form.NewTextInput("HH:MM", h.pal)
	start.SetValue(s.Start.Format("15:04"))
	stop := form.NewTextInput("HH:MM oder +1h30m", h.pal)
	stop.SetValue(s.Stop.Format("15:04"))
	tag := form.NewTextInput("z.B. deep, meeting", h.pal)
	tag.SetValue(s.Tag)
	note := form.NewTextInput("kurzer Text", h.pal)
	note.SetValue(s.Note)
	start.Focus()
	h.form = []textinput.Model{start, stop, tag, note}
	h.formCur = 0
	h.errMsg = ""
	return h, textinput.Blink
}

// openNoteAttachDialog aktiviert den Kompendium-Note-Attach-Picker
// fuer "heute". Delegiert an das geteilte noteAttachPicker-Widget;
// das Widget liest deps.NoteLister.Recent fuer den Suggestion-Stream
// und degradiert zur Raw-ID-Eingabe wenn NoteLister nil ist.
func (h heute) openNoteAttachDialog() (tea.Model, tea.Cmd) {
	h.editDate = h.deps.Clock.Now()
	h.dialog = heuteDialogNoteAttach
	h.errMsg = ""
	picker, cmd := h.notePicker.Open(h.editDate, h.attachedNotes)
	h.notePicker = picker
	return h, cmd
}

func (h heute) openDeleteDialog() (tea.Model, tea.Cmd) {
	s := h.day.Sessions[h.cursor]
	h.editIdx = h.cursor
	h.editDate = s.Date
	h.dialog = heuteDialogDelete
	h.errMsg = ""
	question := fmt.Sprintf("Session %d löschen?", h.cursor+1)
	detail := fmt.Sprintf("%s → %s   %s",
		s.Start.Format("15:04"), s.Stop.Format("15:04"), formatDur(s.Elapsed))
	cm := confirm.New(question, detail, h.pal)
	h.confirmModel = &cm
	return h, cm.Init()
}
