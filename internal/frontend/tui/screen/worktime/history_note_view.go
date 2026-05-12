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

	tea "github.com/charmbracelet/bubbletea"

	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown_overlay"
)

func (h history) openDrillNoteView() (tea.Model, tea.Cmd) {
	if len(h.drillAttached) == 0 {
		return h, func() tea.Msg {
			return historyActionDoneMsg{
				toast: "Keine Notiz angehaengt — `n` haengt eine an",
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
func (h history) handleDrillNoteViewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if h.drillNoteView == nil {
		h.dialog = historyDialogDrill
		return h, nil
	}
	upd, cmd := h.drillNoteView.Update(msg)
	h.drillNoteView = &upd
	return h, cmd
}
