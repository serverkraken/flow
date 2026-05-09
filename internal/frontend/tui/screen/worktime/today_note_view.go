package worktime

// Heute integrierter Note-Viewer — der `o`-Key öffnet die erste
// angehängte Kompendium-Note inline statt einen externen Viewer in
// einem tmux-Split zu starten. Dieselbe Markdown-Pipeline wie der
// Cheatsheet-Tab (ports.MarkdownRenderer + bubbles/viewport).
//
// Der Viewer ist ein Sub-Dialog (heuteDialogNoteView): er nutzt einen
// viewport.Model für Scrolling, blendet Tab-Bar und Footer-Hints aus
// und schließt mit `q`/`Esc`/`b`. Read-Fehler werden inline gerendert
// (statt als Toast), damit der User die Fehlermeldung lange genug sieht.

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/frontend/tui/components/titlebox"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// openNoteViewDialog aktiviert den integrierten Note-Viewer für die
// erste angehängte Note des Tages. Drei Degenerationspfade:
//   - keine Anhänge: Info-Toast, Dialog bleibt zu.
//   - kein NoteReader gewired: Fallback auf den externen Viewer-Pfad.
//   - Read-Fehler: Dialog öffnet, zeigt Fehler inline, viewer-fail-soft.
func (h heute) openNoteViewDialog() (tea.Model, tea.Cmd) {
	if len(h.attachedNotes) == 0 {
		return h, func() tea.Msg {
			return heuteActionDoneMsg{toast: "Keine Notiz angehängt — `n` hängt eine an", info: true}
		}
	}
	if h.deps.NoteReader == nil {
		// Composition-Root hat den NoteReader nicht gewired — degradiere
		// auf den externen Viewer-Pfad statt eines harten Fehlers.
		return h, h.viewAttachedNoteCmd()
	}
	id := h.attachedNotes[0]
	h.dialog = heuteDialogNoteView
	h.noteViewID = id
	h.noteViewErr = nil
	body, err := h.deps.NoteReader.Read(id)
	if err != nil {
		h.noteViewErr = err
		h.noteViewReady = false
		return h, nil
	}
	rendered := body
	if h.deps.MarkdownRenderer != nil {
		// Render-Width = inner-Box (h.width - 4) - 2 Pad innen, mirror
		// cheatsheet/model.go renderContent. Bei width == 0 (vor erstem
		// WindowSizeMsg) liefert der Renderer trotzdem sinnvollen Output.
		w := h.width - 6
		if w < 20 {
			w = 60
		}
		if r, rerr := h.deps.MarkdownRenderer.Render(body, w); rerr == nil {
			rendered = r
		}
	}
	vpW := h.width - 4
	if vpW < 1 {
		vpW = 60
	}
	// Höhe konservativ: titlebox (3) + footer (1) = 4 Zeilen Chrome —
	// real height kommt erst über WindowSizeMsg (heute kennt keine
	// height-Komponente direkt; viewport reflowt bei Update).
	vp := viewport.New(vpW, 20)
	vp.SetContent(rendered)
	h.noteViewVP = vp
	h.noteViewReady = true
	return h, nil
}

// renderNoteViewDialog rendert den Sub-Dialog. Title führt die Note-ID
// + Scroll-Percent (mirror Cheatsheet); Body ist der Viewport oder eine
// Fehlerzeile; Footer listet Scroll- + Close-Keys.
func (h heute) renderNoteViewDialog(inner int) string {
	title := fmt.Sprintf("Note · %s", h.noteViewID)
	if h.noteViewReady {
		title = fmt.Sprintf("Note · %s · %.0f%%", h.noteViewID, h.noteViewVP.ScrollPercent()*100)
	}
	if h.noteViewErr != nil {
		title = fmt.Sprintf("Note · %s · Fehler", h.noteViewID)
	}
	var body string
	switch {
	case h.noteViewErr != nil:
		body = "\n" + theme.Err("  Fehler: "+h.noteViewErr.Error(), h.pal)
	case !h.noteViewReady:
		body = "\n" + theme.Dim("  ○ Note lädt…", h.pal)
	default:
		body = h.noteViewVP.View()
	}
	box := titlebox.Render(title, body, inner+2, h.pal)
	hint := theme.Dim("  ↑/↓ · PgUp/PgDn → scrollen  ·  q/Esc/b → schließen", h.pal)
	return strings.Join([]string{box, hint}, "\n")
}

// updateNoteViewKey routet Tasten innerhalb des Note-Viewers. q/Esc/b
// schließen, alle anderen Tasten gehen an den Viewport für Scrolling.
func (h heute) updateNoteViewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "b":
		h.dialog = heuteDialogNone
		h.noteViewReady = false
		h.noteViewErr = nil
		h.noteViewID = ""
		return h, nil
	}
	if h.noteViewReady {
		var cmd tea.Cmd
		h.noteViewVP, cmd = h.noteViewVP.Update(msg)
		return h, cmd
	}
	return h, nil
}
