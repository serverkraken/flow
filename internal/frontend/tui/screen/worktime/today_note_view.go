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
	uistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
	"github.com/serverkraken/flow/internal/frontend/tui/components/titlebox"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// noteViewWidth liefert die Viewport-Breite für den Note-View aus der
// aktuellen Terminalbreite. Mirror der inneren Box (`h.width - 4`) mit
// einem Floor von 60 für den Pre-WindowSizeMsg-Pfad und schmale Panes.
func noteViewWidth(termW int) int {
	w := termW - 4
	if w < 1 {
		w = 60
	}
	return w
}

// noteViewHeight liefert die Viewport-Höhe für den Note-View. Chrome
// = titlebox (3) + Footer-Hint (1) + Buffer (1). Floor 8 verhindert
// einen unbrauchbar dünnen Viewport bei sehr kleinen Terminals.
func noteViewHeight(termH int) int {
	h := termH - 5
	if h < 8 {
		h = 8
	}
	return h
}

// openNoteViewDialog aktiviert den integrierten Note-Viewer für die
// erste angehängte Note des Tages. Zwei Degenerationspfade:
//   - keine Anhänge: Info-Toast, Dialog bleibt zu.
//   - Read-Fehler: Dialog öffnet, zeigt Fehler inline, viewer-fail-soft.
//
// NoteReader-Wiring ist im Composition-Root garantiert (cmd/flow/main.go);
// ein nil-Reader wäre ein Programmierfehler und surface't als klare
// Toast statt sich an einem externen Tool vorbeizuschmuggeln.
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
	h.dialog = heuteDialogNoteView
	h.noteViewID = id
	h.noteViewErr = nil
	h.noteViewBody = ""
	body, err := h.deps.NoteReader.Read(id)
	if err != nil {
		h.noteViewErr = err
		h.noteViewReady = false
		return h, nil
	}
	h.noteViewBody = body
	rendered := renderNoteViewBody(body, h.width, h.deps)
	vp := viewport.New(noteViewWidth(h.width), noteViewHeight(h.height))
	vp.SetContent(rendered)
	h.noteViewVP = vp
	h.noteViewReady = true
	return h, nil
}

// renderNoteViewBody führt den Body durch den MarkdownRenderer auf der
// passenden inner-Box-Breite (mirror cheatsheet/model.go). Bei width == 0
// (vor erstem WindowSizeMsg) liefert der Renderer trotzdem sinnvollen
// Output. Reine Funktion, damit der Resize-Pfad in today.go denselben
// Code nimmt — sonst zerlaufen Tabellen / Code-Blöcke nach tmux-Pane-
// Resize, weil nur die Viewport-Maße aktualisiert würden.
func renderNoteViewBody(body string, termW int, deps Deps) string {
	if deps.MarkdownRenderer == nil {
		return body
	}
	w := termW - 6
	if w < 20 {
		w = 60
	}
	rendered, err := deps.MarkdownRenderer.Render(body, w)
	if err != nil {
		return body
	}
	return rendered
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
	hint := theme.Dim("  "+uistrings.HintScroll+"  ·  q/Esc/b → schließen", h.pal)
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
		h.noteViewBody = ""
		return h, nil
	}
	if h.noteViewReady {
		var cmd tea.Cmd
		h.noteViewVP, cmd = h.noteViewVP.Update(msg)
		return h, cmd
	}
	return h, nil
}
