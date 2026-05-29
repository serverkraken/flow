package worktime

// History drill inline note viewer — `o` in der Drill-Sicht oeffnet
// die erste an drillDate angehaengte Kompendium-Note im integrierten
// markdown_overlay (analog Heute's `o`-Pfad in today_note_view.go).
// Schliessen via markdown_overlay.ExitMsg (siehe history.go Update-
// Switch).
//
// Degenerationspfade:
//   - keine Anhaenge: Info-Toast via historyActionDoneMsg, Dialog
//     bleibt zu — der Drill-Footer haette dem User ja kein `o` als
//     Hint angezeigt, aber falls die Tastenanordnung im Muscle-Memory
//     ist und der Tag nichts hat, geben wir explizites Feedback statt
//     stiller no-op.
//   - kein NoteReader gewired (Compositions-Root-Bug): Error-Toast.
//   - Read-Fehler des konkreten Note-IDs: Overlay oeffnet mit
//     SetError, der User sieht den Fehler inline statt eines
//     toast-flashes der gleich wieder weg ist.

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown_overlay"
)

func (h history) openDrillNoteView() (tea.Model, tea.Cmd) {
	if len(h.drillAttached) == 0 {
		return h, func() tea.Msg {
			return historyActionDoneMsg{
				toast: "Keine Notiz angehängt — `n` hängt eine an",
				date:  h.drillDate,
			}
		}
	}
	if h.deps.NoteReader == nil {
		return h, func() tea.Msg {
			return historyActionDoneMsg{err: fmt.Errorf("note-reader nicht verdrahtet"), date: h.drillDate}
		}
	}
	id := h.drillAttached[0]
	render := func(src string, w int) string {
		if h.deps.MarkdownRenderer == nil {
			return src
		}
		out, err := h.deps.MarkdownRenderer.Render(src, w)
		if err != nil {
			return src
		}
		return out
	}
	overlay := markdown_overlay.New(render,
		markdown_overlay.WithTitle("Note · "+id),
		markdown_overlay.WithSearch(),
		markdown_overlay.WithCodeCopy(),
	).SetSize(h.width, h.height)
	body, err := h.deps.NoteReader.Read(id)
	if err != nil {
		overlay = overlay.SetError(err)
	} else {
		overlay = overlay.SetSource(body)
	}
	h.dialog = historyDialogDrillNoteView
	h.drillNoteView = &overlay
	return h, nil
}

// handleDrillNoteViewKey leitet KeyMsgs an das Overlay weiter. Das
// Overlay konsumiert q/esc/b als Close-Keys und emittiert ExitMsg,
// der vom Outer-Update-Switch in history.go aufgefangen wird —
// dieser Handler muss den ExitMsg-Pfad NICHT selbst behandeln.
func (h history) handleDrillNoteViewKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if h.drillNoteView == nil {
		h.dialog = historyDialogDrill
		return h, nil
	}
	upd, cmd := h.drillNoteView.Update(msg)
	h.drillNoteView = &upd
	return h, cmd
}

// editDrillNoteCmd oeffnet die erste angehaengte Note im externen
// Editor (typischerweise tmux split + nvim) via deps.NoteOpener.
// Spiegelbild von Heute's editAttachedNoteCmd (today_actions.go).
// Drill-Date-Scope: nutzt h.drillDate fuer die Toast-Bestaetigung;
// LinkWriter wird nicht angefasst (Open ist read-only auf dem Store).
func (h history) editDrillNoteCmd() tea.Cmd {
	if len(h.drillAttached) == 0 {
		date := h.drillDate
		return func() tea.Msg {
			return historyActionDoneMsg{
				toast: "Keine Notiz angehängt — `n` hängt eine an",
				date:  date,
			}
		}
	}
	id := h.drillAttached[0]
	date := h.drillDate
	opener := h.deps.NoteOpener
	return func() tea.Msg {
		if opener == nil {
			return historyActionDoneMsg{err: fmt.Errorf("note-opener nicht verdrahtet"), date: date}
		}
		if err := opener.Open(id); err != nil {
			return historyActionDoneMsg{err: err, date: date}
		}
		return historyActionDoneMsg{
			toast: fmt.Sprintf("✓ Note %s zum Bearbeiten geöffnet", id),
			date:  date,
		}
	}
}

// detachDrillNoteCmd entfernt die erste angehaengte Note via
// LinkWriter.Remove(drillDate, id). Spiegelbild von Heute's
// detachAttachedNoteCmd. Kein Confirm-Dialog: die Operation ist
// reversibel (re-attach via `n` mit derselben ID) und der Store ist
// idempotent (siehe LinkStore.Remove no-op-Vertrag).
//
// Nach Erfolg laeuft die uebliche historyActionDoneMsg-Pipeline, die
// drillLoadCmd nachzieht — drillAttached refreshed automatisch, der
// Chip verschwindet wenn die letzte Note weg ist.
func (h history) detachDrillNoteCmd() tea.Cmd {
	if len(h.drillAttached) == 0 {
		date := h.drillDate
		return func() tea.Msg {
			return historyActionDoneMsg{toast: "Keine Notiz angehängt", date: date}
		}
	}
	id := h.drillAttached[0]
	date := h.drillDate
	writer := h.deps.LinkWriter
	return func() tea.Msg {
		if err := writer.Remove(date, id); err != nil {
			return historyActionDoneMsg{err: err, date: date}
		}
		return historyActionDoneMsg{
			toast: fmt.Sprintf("✓ Note %s entfernt", id),
			date:  date,
		}
	}
}
