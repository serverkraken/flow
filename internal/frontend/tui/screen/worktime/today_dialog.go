package worktime

// Heute dialog surfaces — open*Dialog konstruktoren, handleX-Key-
// Dispatch, submitX-Action, plus renderDialog und das `?`-Overlay.
// Split aus today.go (Skill §No-Monoliths): Dialog-Code formt einen
// klaren funktionalen Cluster und gehört nicht ins Model-Routing.

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/confirm"
	"github.com/serverkraken/flow/internal/frontend/tui/components/form"
	"github.com/serverkraken/flow/internal/frontend/tui/components/help"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	uistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// — dialog open —

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

// — dialog dispatch —

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
		return h.updateNoteViewKey(msg)
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

// — dialog submit —

func (h heute) submitDialog() (tea.Model, tea.Cmd) {
	sw := h.deps.SessionWriter
	switch h.dialog {
	case heuteDialogTag:
		tag := strings.TrimSpace(h.input.Value())
		date, idx := h.editDate, h.editIdx
		return h, func() tea.Msg {
			if err := sw.SetTag(date, idx, tag); err != nil {
				return heuteActionDoneMsg{err: err}
			}
			if tag == "" {
				return heuteActionDoneMsg{toast: fmt.Sprintf("✓ Tag entfernt (Session %d)", idx+1)}
			}
			return heuteActionDoneMsg{toast: fmt.Sprintf("✓ Tag »%s« gesetzt (Session %d)", tag, idx+1)}
		}

	case heuteDialogNote:
		note := strings.TrimSpace(h.input.Value())
		date, idx := h.editDate, h.editIdx
		return h, func() tea.Msg {
			if err := sw.SetNote(date, idx, note); err != nil {
				return heuteActionDoneMsg{err: err}
			}
			if note == "" {
				return heuteActionDoneMsg{toast: fmt.Sprintf("✓ Notiz entfernt (Session %d)", idx+1)}
			}
			return heuteActionDoneMsg{toast: fmt.Sprintf("✓ Notiz gespeichert (Session %d)", idx+1)}
		}

	case heuteDialogEdit:
		return h.submitEdit()

	case heuteDialogNoteAttach:
		// Picker-Vorrang: wenn die gefilterte Suggestion-Liste min. einen
		// Eintrag hat und der Cursor in Range, nimm dessen ID. Andernfalls
		// fällt es auf die getippte Raw-ID zurück (Pre-Picker-Verhalten).
		var id string
		if filt := h.filteredNoteSuggestions(); len(filt) > 0 &&
			h.noteSuggCur >= 0 && h.noteSuggCur < len(filt) {
			id = filt[h.noteSuggCur].ID
		} else {
			id = strings.TrimSpace(h.input.Value())
		}
		if id == "" {
			h.errMsg = "Note-ID darf nicht leer sein"
			return h, nil
		}
		date := h.editDate
		writer := h.deps.LinkWriter
		return h, func() tea.Msg {
			if err := writer.Add(date, id); err != nil {
				return heuteActionDoneMsg{err: err}
			}
			return heuteActionDoneMsg{toast: fmt.Sprintf("✓ Note %s angehängt", id)}
		}
	}
	return h, nil
}

func (h heute) submitEdit() (tea.Model, tea.Cmd) {
	if len(h.form) < 2 {
		return h, nil
	}
	startStr := strings.TrimSpace(h.form[0].Value())
	stopStr := strings.TrimSpace(h.form[1].Value())
	tag, note := "", ""
	if len(h.form) >= 3 {
		tag = strings.TrimSpace(h.form[2].Value())
	}
	if len(h.form) >= 4 {
		note = strings.TrimSpace(h.form[3].Value())
	}

	startD, err := domain.ParseHM(startStr)
	if err != nil {
		h.errMsg = err.Error()
		return h, nil
	}
	base := time.Date(h.editDate.Year(), h.editDate.Month(), h.editDate.Day(),
		0, 0, 0, 0, h.editDate.Location())
	startTime := base.Add(startD)
	stopTime, err := domain.ParseStop(normalizeDurationArg(stopStr), startTime, h.deps.Clock.Now())
	if err != nil {
		h.errMsg = err.Error()
		return h, nil
	}
	// HH:MM stop on a non-today date: rebase to the edit's date so we don't
	// pick up "today + HH:MM" from ParseStartArg's now-anchored logic.
	if stopStr != "" && stopStr[0] != '+' {
		if stopHM, perr := domain.ParseHM(stopStr); perr == nil {
			stopTime = base.Add(stopHM)
		}
	}

	sw := h.deps.SessionWriter
	date, idx := h.editDate, h.editIdx
	return h, func() tea.Msg {
		if err := sw.Edit(date, idx, startTime, stopTime); err != nil {
			return heuteActionDoneMsg{err: err}
		}
		if err := sw.SetTag(date, idx, tag); err != nil {
			return heuteActionDoneMsg{err: err}
		}
		if err := sw.SetNote(date, idx, note); err != nil {
			return heuteActionDoneMsg{err: err}
		}
		return heuteActionDoneMsg{toast: fmt.Sprintf("✓ Session %d aktualisiert", idx+1)}
	}
}

// renderDialog rendert die fünf Dialog-Modi (Tag, Notiz, NoteAttach,
// Edit, Delete, Help). Title sitzt am oberen Rand des Dialogs — der
// User soll beim Öffnen sofort wissen *wo* er ist, nicht erst zur
// letzten Zeile scrollen.
//
// Skill §Component vocabulary: Dialog-Title bekommt purple-bold (wie
// titlebox/help-Header) statt dim, sonst ist er im Body nicht mehr von
// dem Hint-String unterscheidbar. Hint-Format folgt §Hint format mit
// `key → action  ·  …`-Separatoren.
//
// Delete-Modus delegiert komplett an confirm.Model, das selbst
// Yellow-Question, Detail-Zeile und kanonisches y/Enter-→-ja-Hint mitbringt.
func (h heute) renderDialog() string {
	inner := h.width - 4
	if inner <= 0 {
		inner = 80
	}

	var rows []string
	var title, hint string

	switch h.dialog {
	case heuteDialogTag:
		title = "Tag setzen"
		hint = uistrings.HintInputSave
		rows = append(rows, picker.SectionHeader("tag", inner, h.pal), "  "+h.input.View())

	case heuteDialogNote:
		title = "Session-Notiz"
		hint = uistrings.HintInputSave
		rows = append(rows, picker.SectionHeader("notiz", inner, h.pal), "  "+h.input.View())

	case heuteDialogNoteAttach:
		title = "Kompendium-Note anhängen"
		// Hint hängt davon ab, ob ein Picker verfügbar ist — sonst
		// liest der User von der nicht-existenten Up/Down-Funktion ab.
		if len(h.noteSuggestions) > 0 {
			hint = "↑/↓ → wählen  ·  tippen → filter  ·  Enter → anhängen  ·  Esc → abbrechen"
		} else {
			hint = "Enter → anhängen  ·  Esc → abbrechen"
		}
		rows = append(rows, picker.SectionHeader("note id", inner, h.pal), "  "+h.input.View())
		rows = append(rows, h.renderNoteSuggestions(inner)...)
		if len(h.attachedNotes) > 0 {
			rows = append(rows, "", stDim(h.pal,
				"  bereits angehängt:  "+strings.Join(h.attachedNotes, "  ·  ")))
		}

	case heuteDialogEdit:
		title = "Session bearbeiten"
		hint = uistrings.HintFormNav
		if h.editIdx >= 0 && h.editIdx < len(h.day.Sessions) {
			s := h.day.Sessions[h.editIdx]
			rows = append(rows, stDim(h.pal, fmt.Sprintf("  Session %d:  %s → %s",
				h.editIdx+1, s.Start.Format("15:04"), s.Stop.Format("15:04"))), "")
		}
		labels := []string{"Start", "Stop", "Tag", "Notiz"}
		for i, ti := range h.form {
			rows = append(rows, picker.SectionHeader(labels[i], inner, h.pal))
			if i == h.formCur {
				rows = append(rows, "  "+ti.View())
			} else {
				v := ti.Value()
				if v == "" {
					v = stDim(h.pal, ti.Placeholder)
				}
				rows = append(rows, "    "+v)
			}
		}

	case heuteDialogDelete:
		title = "Session löschen"
		// confirm.Model rendert bereits seinen eigenen y/Enter-→-ja-Hint;
		// hier nur der gemeinsame Title-Strip, kein doppelter Hint nötig.
		hint = ""
		if h.confirmModel != nil {
			rows = append(rows, "  "+h.confirmModel.View())
		}

	case heuteDialogHelp:
		title = "Heute · Hilfe"
		hint = "beliebige Taste schließt"
		rows = append(rows, h.renderHelpRows(inner)...)
	}

	if h.errMsg != "" {
		rows = append(rows, "", theme.Err("  "+h.errMsg, h.pal))
	}
	// Title + Hint auf eigenen Zeilen: bei schmalen Sidekick-Panes (~70 Cols)
	// schluckte titlebox.Truncate vorher den Hint, weil er auf der gleichen
	// Zeile wie der Title hing und mit ` · `-Separator angehängt wurde.
	header := []string{"  " + theme.Highlight(title, h.pal)}
	if hint != "" {
		header = append(header, "  "+theme.Dim(hint, h.pal))
	}
	header = append(header, "")
	return strings.Join(append(header, rows...), "\n")
}

// helpSectionsHeute is the canonical Heute key-binding inventory. Both
// the standalone `?`-overlay (heute.renderHelpRows) and the sidekick's
// aggregated `?`-overlay (sidekick.renderHelp via worktime.HelpSections)
// read from this single source so the two surfaces cannot drift.
func helpSectionsHeute() []help.Section {
	return []help.Section{
		{Title: "Worktime — Heute · Cursor & Action", Keys: [][2]string{
			{"j/k · g/G", "bewegen · oben/unten"},
			{"s", "starten / stoppen / fortsetzen"},
			{"p", "pause (im laufenden Zustand)"},
		}},
		{Title: "Worktime — Heute · Session-Edit (auf fokussierter Zeile)", Keys: [][2]string{
			{"E / Enter", "Session bearbeiten"},
			{"D", "Session löschen (y/Enter bestätigt)"},
			{"t", "Session-Tag setzen"},
			{"N", "Session-Notiz setzen (großgeschrieben)"},
		}},
		{Title: "Worktime — Heute · Kompendium (für heute)", Keys: [][2]string{
			{"n", "Note anhängen (ID eingeben)"},
			{"o", "erste Note inline ansehen (integrierter Markdown-Viewer)"},
			{"O", "erste Note im Editor öffnen"},
			{"R", "erste Note entfernen"},
		}},
	}
}

// renderHelpRows enumerates Heute's keybinds for the standalone `?`
// overlay. Reads from helpSectionsHeute so the standalone overlay and
// the sidekick aggregator stay in lockstep.
func (h heute) renderHelpRows(inner int) []string {
	sections := helpSectionsHeute()
	rows := []string{}
	for i, sec := range sections {
		if i > 0 {
			rows = append(rows, "")
		}
		// Strip the "Worktime — Heute · " prefix in standalone mode —
		// the parent context is implicit when the user opened heute's
		// own help overlay, and the prefix wastes horizontal real
		// estate inside the cramped dialog frame.
		title := strings.TrimPrefix(sec.Title, "Worktime — Heute · ")
		rows = append(rows, picker.SectionHeader(title, inner, h.pal))
		for _, kv := range sec.Keys {
			keyCell := lipgloss.NewStyle().Width(theme.KeyHintWidth).Render(theme.Highlight(kv[0], h.pal))
			rows = append(rows, "  "+keyCell+stDim(h.pal, kv[1]))
		}
	}
	return rows
}
