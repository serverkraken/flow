package webui

import (
	"html"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// headingNonSlug and headingUmlauts mirror usecase.Slugify's collapsing
// rules exactly (lowercase, German umlauts transliterated, any run of
// non-alphanumerics collapsed to a single "-", trimmed) so a heading's
// auto-slug reads the same way a node's slug would — but are defined
// locally rather than imported: webui (an adapter) rendering Markdown has no
// business depending on usecase for a five-line string helper, and the two
// call sites diverging later is fine since they slug different kinds of
// text.
var (
	headingNonSlug = regexp.MustCompile(`[^a-z0-9]+`)
	headingUmlauts = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss")
)

// headingSlugify turns heading text into a GitHub-style anchor slug.
func headingSlugify(s string) string {
	s = headingUmlauts.Replace(strings.ToLower(s))
	s = headingNonSlug.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// headingText collects a heading's inline children as plain text (Text
// segments, recursing through Emphasis/CodeSpan/Link/etc. wrappers) — the
// same "flatten inline nodes to a string" shape as imageAltText in
// wikilink.go, but kept separate since alt-text and slug-source text serve
// different callers and have no reason to share a signature.
func headingText(source []byte, n ast.Node) string {
	var sb strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			sb.Write(t.Segment.Value(source))
			continue
		}
		sb.WriteString(headingText(source, c))
	}
	return sb.String()
}

// headingSlugTransformer assigns a server-side, GitHub-style slug `id` to
// every heading in the document (h1-h6) and appends a headAnchorNode — a
// hoverable "¶" deep-link, rendered by headAnchorHTMLRenderer below — as the
// heading's last child. Two passes, same shape as figureTransformer
// (mermaid.go): the first walk only collects the *ast.Heading nodes in
// document order; the second loop mutates them. Mutating during the walk
// itself would work too (ast.Walk snapshots a node's next sibling before
// recursing, so appending a new last child to the node currently being
// visited is safe), but collect-then-mutate keeps this transformer legible
// and consistent with the transformer already in this package.
//
// Slugs must be stable and collision-free within one document: two
// headings that slug to the same base text ("Setup" and "Setup") get
// "-1", "-2", ... suffixes on the second and later occurrences — the same
// scheme GitHub itself uses for shared #slug URLs, and required for the
// slug to double as a stable id other documents can wikilink/deep-link to.
type headingSlugTransformer struct{}

func (headingSlugTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	src := reader.Source()
	var headings []*ast.Heading
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if h, ok := n.(*ast.Heading); ok {
				headings = append(headings, h)
			}
		}
		return ast.WalkContinue, nil
	})

	seen := map[string]int{}
	for _, h := range headings {
		base := headingSlugify(headingText(src, h))
		if base == "" {
			base = "section"
		}
		slug := base
		if count, dup := seen[base]; dup {
			slug = base + "-" + strconv.Itoa(count)
		}
		seen[base]++
		h.SetAttribute([]byte("id"), []byte(slug))
		h.AppendChild(h, &headAnchorNode{Slug: slug})
	}
}

// --- head-anchor AST node + renderer -----------------------------------

var kindHeadAnchor = ast.NewNodeKind("HeadAnchor")

// headAnchorNode is the hoverable "¶" deep-link headingSlugTransformer
// appends as a heading's last inline child. It carries only the slug the
// enclosing heading already got as its `id` — the anchor's href is always
// "#" + that same slug, so the two can never drift apart.
type headAnchorNode struct {
	ast.BaseInline
	Slug string
}

func (n *headAnchorNode) Kind() ast.NodeKind       { return kindHeadAnchor }
func (n *headAnchorNode) Dump(src []byte, lvl int) { ast.DumpHelper(n, src, lvl, nil, nil) }

// headAnchorHTMLRenderer renders headAnchorNode as
// `<a class="head-anchor" href="#slug" data-copy="#slug" ...>¶</a>`. The
// `¶` glyph is plain monospace text, not an emoji/icon font (Lesesaal
// design doctrine — glyph whitelist). data-copy/data-copied-label plug into
// the existing static/js/clipboard.js click handler (Bestand, no new popup
// JS) — clipboard.js resolves a "#..." data-copy value against
// location.href at click time so the copied text is a full shareable deep
// link, not just the bare fragment.
type headAnchorHTMLRenderer struct {
	CopyLabel   string
	CopiedLabel string
}

func (r *headAnchorHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindHeadAnchor, r.render)
}

func (r *headAnchorHTMLRenderer) render(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	n, ok := node.(*headAnchorNode)
	if !ok {
		return ast.WalkContinue, nil
	}
	href := "#" + html.EscapeString(n.Slug)
	_, _ = w.WriteString(`<a class="head-anchor" href="`)
	_, _ = w.WriteString(href)
	_, _ = w.WriteString(`" data-copy="`)
	_, _ = w.WriteString(href)
	_, _ = w.WriteString(`" data-copied-label="`)
	_, _ = w.WriteString(html.EscapeString(r.CopiedLabel))
	_, _ = w.WriteString(`" aria-label="`)
	_, _ = w.WriteString(html.EscapeString(r.CopyLabel))
	_, _ = w.WriteString(`" title="`)
	_, _ = w.WriteString(html.EscapeString(r.CopyLabel))
	_, _ = w.WriteString(`">¶</a>`)
	return ast.WalkSkipChildren, nil
}
