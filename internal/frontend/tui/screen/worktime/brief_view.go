package worktime

// Brief-Viewer-Overlay — rendert den Worktime-Brief (Markdown) inline
// im markdown_overlay-Component (gemeinsam mit der today_note-Anzeige
// und dem kompendium-Full-Screen-Viewer). Konstruktion: das Menu
// emittiert briefViewMsg, der Worktime-Root baut einen neuen Overlay
// mit dem Markdown-Renderer aus Deps. Schließen via ExitMsg vom
// Overlay; der Root verwirft den Overlay-Pointer.

import (
	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown_overlay"
)

// briefViewMsg signalisiert dem Worktime-Root, dass ein Brief gerendert
// und im Overlay angezeigt werden soll. Der Body ist der rohe Markdown
// (vom Reporter); der Render-Closure des Overlays übernimmt die
// Glamour-Render-Pipeline bei der jeweils aktuellen Breite.
type briefViewMsg struct {
	title string
	body  string
}

// newBriefView baut den Overlay mit dem deps.MarkdownRenderer als
// RenderFunc-Closure. nil-Renderer wäre Wiring-Bug; in dem Fall
// liefert der Closure raw markdown zurück (fail-soft, sichtbarer Wall
// statt leerer Overlay).
func newBriefView(title, body string, width, height int, deps Deps) markdown_overlay.Model {
	render := func(src string, w int) string {
		if deps.MarkdownRenderer == nil {
			return src
		}
		out, err := deps.MarkdownRenderer.Render(src, w)
		if err != nil {
			return src
		}
		return out
	}
	return markdown_overlay.New(
		render,
		markdown_overlay.WithTitle(title),
		markdown_overlay.WithSource(body),
		markdown_overlay.WithSearch(),
		markdown_overlay.WithCodeCopy(),
	).SetSize(width, height)
}
