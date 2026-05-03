// nodeRenderer is the goldmark NodeRenderer that turns CommonMark+GFM
// AST into ANSI-styled text using the bundled theme palette. It owns the
// shared per-render state (palette roles, width budget, OSC 8 id
// counter) and delegates per-node-type rendering to the helpers in
// blocks.go / inline.go / code.go / etc.
//
// One nodeRenderer is built per Render call so per-call options
// (NoColor, NerdFont, …) propagate cleanly without globals.

package markdown

import (
	"bufio"
	"bytes"
	"strings"

	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"

	"github.com/serverkraken/flow/internal/frontend/tui/markdown/theme"
	canonical "github.com/serverkraken/flow/internal/frontend/tui/theme"
)

type nodeRenderer struct {
	roles   theme.MarkdownRoles
	palette canonical.Palette
	width   int
	opts    options

	// indent is the per-cell left-padding the parent block reserves
	// for its own marker / bar / continuation prefix. Children
	// subtract it from r.width when computing wrap budgets so a
	// paragraph inside a list item doesn't overflow once the
	// outer list-item prefix is applied.
	indent int

	// osc8ID is the running counter stamped onto OSC 8 hyperlink
	// open sequences. lipgloss / cellbuf re-emit the same id around
	// every wrap boundary so the terminal joins multi-line links
	// into one click target.
	osc8ID int

	// handlersCache memoises the per-kind renderer-func map. Populated
	// by handlers() on first call; consumed by both dispatch (for
	// sub-buffer rendering of children) and RegisterFuncs (for
	// goldmark-driven top-level walk).
	handlersCache map[ast.NodeKind]renderer.NodeRendererFunc
}

// newNodeRenderer constructs a renderer ready to register against a
// goldmark renderer.Renderer. The lipgloss renderer that builds the
// palette comes from the options (NoColor swaps in an Ascii-profile
// renderer); the canonical palette is also pulled from options so
// per-call overrides (WithPalette) reach every helper.
func newNodeRenderer(width int, opts options) *nodeRenderer {
	return &nodeRenderer{
		roles:   theme.MarkdownRolesFor(opts.lip, opts.palette),
		palette: opts.palette,
		width:   width,
		opts:    opts,
	}
}

// RegisterFuncs implements renderer.NodeRenderer. Pulls per-kind
// handlers from the same handlers() table dispatch consumes, plus
// extra walk-only registrations for container kinds (Document,
// TextBlock, Blockquote) so goldmark descends into them without
// inventing styling we haven't decided on yet.
func (r *nodeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	for kind, fn := range r.handlers() {
		reg.Register(kind, fn)
	}
	// Walk-only kind — pure structural container. Skipping
	// registration would leave it rendered as nothing; renderWalk
	// descends and surfaces inner content.
	reg.Register(ast.KindDocument, r.renderWalk)
}

// effectiveWidth returns the column budget for rendering inner
// content — r.width reduced by the parent block's reserved indent.
// Always >=1 so callers don't have to special-case overflow.
func (r *nodeRenderer) effectiveWidth() int {
	w := r.width - r.indent
	if w < 1 {
		w = 1
	}
	return w
}

// renderWalk is the no-op handler — descend into children but emit
// nothing of our own. Used for structural container nodes (Document)
// and for block kinds we haven't styled yet so their inline content
// still surfaces as plain text.
func (r *nodeRenderer) renderWalk(_ util.BufWriter, _ []byte, _ ast.Node, _ bool) (ast.WalkStatus, error) {
	return ast.WalkContinue, nil
}

// renderTextBlock walks children and appends a single newline on
// exit so a tight-list ListItem's inline text doesn't bleed into a
// following sibling block (e.g. a nested List). goldmark uses
// TextBlock specifically inside tight lists; loose-list items use
// Paragraph (which carries its own trailing blank line).
func (r *nodeRenderer) renderTextBlock(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		return ast.WalkContinue, nil
	}
	_, _ = w.WriteString("\n")
	return ast.WalkContinue, nil
}

// renderRawBlock emits the literal source bytes of a leaf block node
// surrounded by blank lines. Used as the placeholder for fenced/
// indented code and HTML blocks until the proper renderers land —
// without it the reader would see no content for those blocks at all.
func (r *nodeRenderer) renderRawBlock(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	_, _ = w.WriteString("\n")
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		_, _ = w.Write(seg.Value(source))
	}
	_, _ = w.WriteString("\n")
	return ast.WalkSkipChildren, nil
}

// renderRawInline emits the source bytes of an inline leaf so raw
// HTML round-trips through the renderer instead of going silent.
// goldmark stores raw inline HTML as one or more segments on the
// node — we walk all of them.
func (r *nodeRenderer) renderRawInline(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	if html, ok := n.(*ast.RawHTML); ok {
		for i := 0; i < html.Segments.Len(); i++ {
			seg := html.Segments.At(i)
			_, _ = w.Write(seg.Value(source))
		}
		return ast.WalkSkipChildren, nil
	}
	return ast.WalkContinue, nil
}

// renderChildrenToString walks node's children through the same node
// renderer into a sub-buffer and returns the resulting ANSI string.
// Used by block renderers that need their inner text up-front (the
// H1 bar must size to the heading text; a ListItem must capture
// nested blocks so it can indent them under the bullet).
func (r *nodeRenderer) renderChildrenToString(source []byte, node ast.Node) (string, error) {
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	for c := node.FirstChild(); c != nil; c = c.NextSibling() {
		if err := ast.Walk(c, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			return r.dispatch(bw, source, n, entering)
		}); err != nil {
			return "", err
		}
	}
	if err := bw.Flush(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// renderInlineToString is the legacy alias kept so existing callers
// (renderHeading, renderEmphasis, renderImage, …) read with intent —
// "give me the inline run" — even though the helper now traverses
// any kind a goldmark walk can reach.
func (r *nodeRenderer) renderInlineToString(source []byte, node ast.Node) (string, error) {
	return r.renderChildrenToString(source, node)
}

// dispatch routes a node to the matching renderer-func via the
// handlersTable. Re-uses the handlers registered against goldmark so
// the sub-buffer rendering from renderChildrenToString matches the
// top-level pipeline. Unknown kinds default to WalkContinue so the
// walker descends into their children (preserves text content for
// kinds we haven't styled yet).
func (r *nodeRenderer) dispatch(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if fn, ok := r.handlers()[n.Kind()]; ok {
		return fn(w, source, n, entering)
	}
	return ast.WalkContinue, nil
}

// handlers returns the per-kind renderer-func map. Built lazily and
// cached on the receiver. Single source of truth — RegisterFuncs and
// dispatch both pull from here so adding a new kind is a one-line
// edit instead of two.
func (r *nodeRenderer) handlers() map[ast.NodeKind]renderer.NodeRendererFunc {
	if r.handlersCache != nil {
		return r.handlersCache
	}
	r.handlersCache = map[ast.NodeKind]renderer.NodeRendererFunc{
		// Block — styled
		ast.KindHeading:             r.renderHeading,
		ast.KindParagraph:           r.renderParagraph,
		ast.KindTextBlock:           r.renderTextBlock,
		ast.KindThematicBreak:       r.renderThematicBreak,
		ast.KindFencedCodeBlock:     r.renderFencedCodeBlock,
		ast.KindCodeBlock:           r.renderIndentedCodeBlock,
		ast.KindList:                r.renderList,
		ast.KindListItem:            r.renderListItem,
		ast.KindBlockquote:          r.renderBlockquote,
		ast.KindHTMLBlock:           r.renderRawBlock,
		extast.KindTable:            r.renderTable,
		extast.KindFootnoteList:     r.renderFootnoteList,
		extast.KindFootnote:         r.renderFootnote,
		extast.KindFootnoteLink:     r.renderFootnoteLink,
		extast.KindFootnoteBacklink: r.renderFootnoteBacklink,
		// Inline — styled
		ast.KindText:     r.renderText,
		ast.KindString:   r.renderString,
		ast.KindEmphasis: r.renderEmphasis,
		ast.KindCodeSpan: r.renderCodeSpan,
		ast.KindLink:     r.renderLink,
		ast.KindAutoLink: r.renderAutoLink,
		ast.KindImage:    r.renderImage,
		ast.KindRawHTML:  r.renderRawInline,
		wikiLinkKind:     r.renderWikiLink,
	}
	return r.handlersCache
}

// trimTrailingBlankLines collapses runs of blank lines at the very end
// of a render so the wrapping viewport doesn't waste rows. Glamour did
// this implicitly; we do it explicitly because every block renderer
// emits its own trailing blank line.
func trimTrailingBlankLines(s string) string {
	return strings.TrimRight(s, "\n") + "\n"
}
