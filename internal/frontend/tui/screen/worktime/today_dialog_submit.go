package worktime

// Heute dialog submit — submitDialog dispatcht auf den jeweiligen
// Writer-Pfad (SetTag/SetNote/LinkWriter.Add) und liefert die
// resultierende heuteActionDoneMsg-Closure als tea.Cmd. submitEdit
// trägt die ParseHM/ParseStop-Logik für den Edit-Dialog. Split aus
// today_dialog.go (Skill §No-Monoliths).

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/domain"
)

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
		// Picker bestimmt die ID via SelectedID (Cursor-Pick gewinnt,
		// sonst getipptes raw input). Bei leerer Eingabe: errMsg auf
		// den Picker (nicht auf h.errMsg) — der Picker rendert ihn in
		// seinem eigenen Body, damit der Dialog-Title-Strip nicht
		// doppelt blinkt.
		id := h.notePicker.SelectedID()
		if id == "" {
			h.notePicker = h.notePicker.SetError("Note-ID darf nicht leer sein")
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
