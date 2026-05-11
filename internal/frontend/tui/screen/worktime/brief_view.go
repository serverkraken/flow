package worktime

// Brief-Viewer-Overlay — rendert den Worktime-Brief (Markdown) inline
// via den ports.MarkdownRenderer in einem viewport.Model, statt das
// content an einen externen tmux-Split-Pager wie glow zu reichen. Der
// User-Flow ist: Menu → Brief Week/Month → Target tmux-Split → Overlay
// hier öffnet sich → q/Esc/b schließt zurück zum vorigen Worktime-Tab.
//
// Pattern parallelisiert today_note_view.go: viewport + MarkdownRenderer
// + titlebox + Footer-Hint, plus Scroll-Keys ans viewport. Beim Schließen
// wird der Overlay-State verworfen; das vorige Worktime-View nimmt
// nahtlos zurück, weil der Worktime-Root bei aktivem Overlay den Body
// einfach ersetzt (kein State-Verlust in den Sub-Models).

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	uistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
	"github.com/serverkraken/flow/internal/frontend/tui/components/titlebox"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// briefViewMsg signalisiert dem Worktime-Root, dass ein Brief gerendert
// und im Overlay angezeigt werden soll. Der Body ist der rohe Markdown
// (vom Reporter); der MarkdownRenderer-Aufruf passiert im Overlay-
// Konstruktor, damit Render-Breite die Terminal-Größe respektiert.
type briefViewMsg struct {
	title string
	body  string
}

// briefView ist der Overlay-State auf Worktime-Root-Ebene. Aktiv, wenn
// briefView != nil im Model. Eigenes Lifecycle: konstruiert in
// handleBriefViewMsg, verworfen beim Close-Key.
type briefView struct {
	title    string
	rawBody  string
	rendered string
	vp       viewport.Model
	ready    bool
}

// newBriefView baut den Overlay mit gerendertem Markdown. Sub-Models
// haben über die Worktime-Deps Zugriff auf MarkdownRenderer; wenn nil,
// fällt der Overlay auf raw Markdown zurück (read-fail-soft).
func newBriefView(title, body string, width, height int, deps Deps, pal theme.Palette) briefView {
	bv := briefView{title: title, rawBody: body}
	rendered := body
	if deps.MarkdownRenderer != nil {
		renderW := width - 6
		if renderW < 20 {
			renderW = 60
		}
		if r, rerr := deps.MarkdownRenderer.Render(body, renderW); rerr == nil {
			rendered = r
		}
	}
	bv.rendered = rendered
	vp := viewport.New(briefViewWidth(width), briefViewHeight(height))
	vp.SetContent(rendered)
	bv.vp = vp
	bv.ready = true
	_ = pal // palette is unused for the body; titlebox owns its own colours
	return bv
}

func briefViewWidth(termW int) int {
	w := termW - 4
	if w < 1 {
		w = 60
	}
	return w
}

func briefViewHeight(termH int) int {
	h := termH - 5
	if h < 8 {
		h = 8
	}
	return h
}

// updateKey routet Tasten im Overlay. q/Esc/b schließen — der Worktime-
// Root erkennt das Signal über das ok-Flag und verwirft den Overlay.
// Alle anderen Keys gehen ans viewport (scrolling).
func (b briefView) updateKey(msg tea.KeyMsg) (briefView, tea.Cmd, bool) {
	switch msg.String() {
	case "q", "esc", "b":
		return b, nil, true
	}
	if b.ready {
		var cmd tea.Cmd
		b.vp, cmd = b.vp.Update(msg)
		return b, cmd, false
	}
	return b, nil, false
}

// resize aktualisiert viewport-Dimensionen bei WindowSizeMsg. Re-rendert
// den Markdown bei der neuen Breite, damit Tabellen / Code-Blöcke nicht
// abgeschnitten werden.
func (b briefView) resize(width, height int, deps Deps) briefView {
	renderW := width - 6
	if renderW < 20 {
		renderW = 60
	}
	rendered := b.rawBody
	if deps.MarkdownRenderer != nil {
		if r, rerr := deps.MarkdownRenderer.Render(b.rawBody, renderW); rerr == nil {
			rendered = r
		}
	}
	b.rendered = rendered
	b.vp.Width = briefViewWidth(width)
	b.vp.Height = briefViewHeight(height)
	b.vp.SetContent(rendered)
	return b
}

// view rendert den Overlay als titlebox + Footer-Hint. Der Title führt
// den briefView.title + Scroll-Percent — mirror today_note_view.
func (b briefView) view(inner int, pal theme.Palette) string {
	title := b.title
	if b.ready {
		title = fmt.Sprintf("%s · %.0f%%", b.title, b.vp.ScrollPercent()*100)
	}
	box := titlebox.Render(title, b.vp.View(), inner+2, pal)
	hint := theme.Dim("  "+uistrings.HintScroll+"  ·  q/Esc/b → schließen", pal)
	return strings.Join([]string{box, hint}, "\n")
}
