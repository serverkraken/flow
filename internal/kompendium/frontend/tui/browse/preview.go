package browse

// Browse preview + viewer surfaces — die Right-Pane-Preview (refresh +
// render mit Glamour-Cache) plus die Full-Screen-Viewer-Aufmach-Logik
// (openViewer + buildViewerSource). wikilinkResolver / browseResolver
// liegen hier, weil Preview UND Viewer denselben Resolver nutzen — ein
// gemeinsamer Cluster, der von den restlichen Files entkoppelt ist.
// Split aus model.go (Skill §No-Monoliths).

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown_overlay"
	"github.com/serverkraken/flow/internal/frontend/tui/markdown"
	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
	flowports "github.com/serverkraken/flow/internal/ports"
)

// openViewer constructs a fresh in-process Markdown viewer for the
// cursor's note and switches to ModeView. The viewer renders the
// already-loaded entry's metadata as a Markdown header followed by
// the on-disk body — same pipeline the preview pane uses, just at
// full screen. Body fetch failures land in m.editErr and stay in
// ModeNormal.
func (m Model) openViewer() Model {
	if m.store == nil {
		return m
	}
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return m
	}
	e := m.visible[m.cursor]
	note, err := m.store.Get(context.Background(), e.ID)
	if err != nil {
		m.editErr = fmt.Errorf("open viewer: %w", err)
		return m
	}
	title := e.Meta.Title
	if title == "" {
		title = e.ID.String()
	}
	source := buildViewerSource(e, note.Body)
	meta := e.Meta
	var backlinks []usecase.BacklinkRef
	if m.backlinksFn != nil {
		backlinks = m.backlinksFn(e.ID)
	}
	resolver := m.wikilinkResolver()
	render := func(src string, w int) string {
		var opts []markdown.Option
		if resolver != nil {
			opts = append(opts, markdown.WithWikilinks(resolver))
		}
		opts = append(opts, markdown.WithFrontmatter(frontmatterToMarkdown(&meta)))
		if len(backlinks) > 0 {
			opts = append(opts, markdown.WithBacklinks(backlinksToMarkdown(backlinks)))
		}
		out, _ := markdown.Render(src, w, opts...)
		return out
	}
	v := markdown_overlay.New(render,
		markdown_overlay.WithTitle(title),
		markdown_overlay.WithSource(source),
		markdown_overlay.WithSearch(),
		markdown_overlay.WithCodeCopy(),
	)
	if m.width > 0 && m.height > 0 {
		v = v.SetSize(m.width, m.height)
	}
	m.viewer = v
	m.mode = ModeView
	return m
}

// buildViewerSource returns the body the viewer renders. The header
// (title + metadata) is no longer prepended as Markdown — the
// renderer's frontmatter card handles it via WithFrontmatter, which
// the overlay's RenderFunc closure binds in.
func buildViewerSource(_ ports.NoteEntry, body []byte) string {
	if len(body) == 0 {
		return "*Inhalt noch nicht geladen.*"
	}
	return string(body)
}

// refreshPreview updates the previewed note + viewport content based on
// the current cursor.
func (m *Model) refreshPreview() {
	if !m.twoPane() {
		m.preview.SetContent("")
		m.previewID = ""
		return
	}
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		m.preview.SetContent(dimStyle.Render("(keine Notiz ausgewählt)"))
		m.previewID = ""
		return
	}
	e := m.visible[m.cursor]
	if m.previewID == e.ID {
		// Already rendered; let the viewport keep its scroll position.
		return
	}
	m.previewID = e.ID
	rendered := m.renderPreviewBody(e)
	m.preview.SetContent(rendered)
	m.preview.GotoTop()
}

// renderPreviewBody builds the preview content for the given entry.
// Glamour does the heavy lifting for the body; the title/metadata header
// is rendered with our own styles so it stays visually consistent with
// the list pane regardless of which Markdown style is active.
//
// The body comes from the store — NOT from m.bodies — because that map
// only carries an 8 KB excerpt of each note (the loader caps it to
// avoid OOM-killing kompendium on notebooks with huge Markdown files).
// The render result is memoised in m.previewCached, so re-rendering
// from disk happens at most once per entry per layout.
func (m *Model) renderPreviewBody(e ports.NoteEntry) string {
	width, _ := m.previewSize()
	if width <= 0 {
		return ""
	}
	if cached, ok := m.previewCached[e.ID]; ok {
		return cached
	}

	var body []byte
	hasBody := false
	if m.store != nil {
		if note, err := m.store.Get(context.Background(), e.ID); err == nil {
			body = note.Body
			hasBody = true
		}
	}

	source := ""
	if hasBody {
		source = string(body)
	} else {
		source = "*Noch kein Body gecacht.*"
	}

	meta := e.Meta
	rendered, err := markdown.Render(
		source, width,
		markdown.WithWikilinks(m.wikilinkResolver()),
		markdown.WithFrontmatter(frontmatterToMarkdown(&meta)),
	)
	if err != nil {
		rendered = source
	}
	m.previewCached[e.ID] = rendered
	return rendered
}

// wikilinkResolver builds a flowports.WikilinkResolver that consults
// the browse model's loaded entries. Used by both renderPreviewBody
// and the full-screen viewer (the same resolver is bound into the
// overlay's RenderFunc closure when ModeView starts) so wikilink
// resolution is consistent across surfaces.
func (m Model) wikilinkResolver() flowports.WikilinkResolver {
	idx := make(map[domain.ID]ports.NoteEntry, len(m.all))
	for _, e := range m.all {
		idx[e.ID] = e
	}
	return browseResolver{entries: idx}
}

// browseResolver looks up wikilink targets in the loaded NoteEntry
// map. Returns kompendium://note/<id> for valid hits, ok=false for
// misses (renderer styles those as broken).
type browseResolver struct {
	entries map[domain.ID]ports.NoteEntry
}

func (r browseResolver) Resolve(target string) (uri, title string, ok bool) {
	e, found := r.entries[domain.ID(target)]
	if !found {
		return "", "", false
	}
	return "kompendium://note/" + target, e.Meta.Title, true
}
