package worktime

// Heute integrierter Note-Viewer — der `o`-Key öffnet die erste
// angehängte Kompendium-Note inline statt einen externen Viewer in
// einem tmux-Split zu starten. Markdown-Pipeline (Glamour) + scroll +
// search + code-copy kommen aus dem markdown_overlay-Component.
//
// Lifecycle: openNoteViewDialog konstruiert das Overlay, lädt den
// Body via deps.NoteReader und übergibt entweder SetSource (Erfolg)
// oder SetError (Read-Fail) an die Komponente. Schließen via
// markdown_overlay.ExitMsg im heute-Update-Switch.

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown_overlay"
)

// openNoteViewDialog aktiviert den integrierten Note-Viewer für die
// erste angehängte Note des Tages. Drei Degenerationspfade:
//   - keine Anhänge: Info-Toast, Dialog bleibt zu.
//   - kein NoteReader gewired: Toast (Programmierfehler), Dialog bleibt zu.
//   - Read-Fehler: Dialog öffnet, zeigt Fehler inline via SetError.
func (h heute) openNoteViewDialog() (tea.Model, tea.Cmd) {
	if len(h.attachedNotes) == 0 {
		return h, func() tea.Msg {
			return heuteActionDoneMsg{toast: "Keine Notiz angehängt — `n` hängt eine an", info: true}
		}
	}
	if h.deps.NoteReader == nil {
		return h, func() tea.Msg {
			return heuteActionDoneMsg{err: fmt.Errorf("note-reader nicht verdrahtet")}
		}
	}
	id := h.attachedNotes[0]
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
	h.dialog = heuteDialogNoteView
	h.noteView = &overlay
	return h, nil
}
