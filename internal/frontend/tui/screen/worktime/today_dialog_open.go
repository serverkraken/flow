package worktime

// Heute dialog openers — Konstruktoren für Tag-, Notiz-, Edit-,
// NoteAttach- und Delete-Dialog. Split aus today_dialog.go (Skill
// §No-Monoliths): Dialog-Aufmach-Code formt einen klaren Cluster
// neben Key-Dispatch (today_dialog_keys.go) und Submit
// (today_dialog_submit.go); renderDialog bleibt in today_dialog.go.

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
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

// openNoteAttachDialog aktiviert den Kompendium-Note-Attach-Picker.
// Bei vorhandenem deps.NoteLister: zeigt ein Live-gefiltertes Picker-
// Listing der jüngsten Notes (Up/Down navigiert, Enter wählt).
// Ohne NoteLister: degradiert zum Pre-Picker-Verhalten (Raw-ID-Eingabe).
func (h heute) openNoteAttachDialog() (tea.Model, tea.Cmd) {
	h.editDate = h.deps.Clock.Now()
	h.dialog = heuteDialogNoteAttach
	placeholder := "tippen → suchen, oder Note-ID"
	if h.deps.NoteLister == nil {
		placeholder = "Note-ID (z.B. 2026-05-03 oder daily-2026-05-03)"
	}
	h.input = form.NewTextInput(placeholder, h.pal)
	h.input.SetValue("")
	h.input.Focus()
	h.errMsg = ""
	h.noteSuggestions = nil
	h.noteSuggCur = 0
	if h.deps.NoteLister != nil {
		h.noteSuggestions = h.deps.NoteLister.Recent(noteAttachPickerLimit)
	}
	return h, textinput.Blink
}

// filteredNoteSuggestions reduziert h.noteSuggestions auf Einträge, die
// die aktuelle Input-Substring (case-insensitive) in ID oder Title
// haben. Leere Eingabe = ungefilterte Liste. Aufruf nur in
// Render/Key-Handling, damit die Quelle (Recent-Snapshot) unverändert
// bleibt.
func (h heute) filteredNoteSuggestions() []NoteSuggestion {
	q := strings.ToLower(strings.TrimSpace(h.input.Value()))
	if q == "" || len(h.noteSuggestions) == 0 {
		return h.noteSuggestions
	}
	out := make([]NoteSuggestion, 0, len(h.noteSuggestions))
	for _, s := range h.noteSuggestions {
		if strings.Contains(strings.ToLower(s.ID), q) ||
			strings.Contains(strings.ToLower(s.Title), q) {
			out = append(out, s)
		}
	}
	return out
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
