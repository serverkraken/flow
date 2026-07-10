package webui

import (
	"bytes"
	"context"
	"html"
	"html/template"
	"regexp"
	"sync"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

// WikilinkResolver maps a wikilink target string to an href, an
// optional display title, and whether the target is known.
type WikilinkResolver func(target string) (href, title string, ok bool)

// docPolicy is the bluemonday policy used by RenderDocument. It
// extends UGCPolicy to preserve the `class` attribute (for
// wikilink/wikilink-broken markers) and relative hrefs (e.g.
// /wissen/d-arch).
var (
	docPolicyOnce sync.Once
	docPolicy     *bluemonday.Policy
)

func getDocPolicy() *bluemonday.Policy {
	docPolicyOnce.Do(func() {
		p := bluemonday.UGCPolicy()
		p.AllowAttrs("class").OnElements("a", "span")
		p.AllowAttrs("href").OnElements("a")
		p.AllowRelativeURLs(true)
		p.RequireParseableURLs(true)
		p.AllowAttrs("align").OnElements("td", "th")
		p.AllowElements("table", "thead", "tbody", "tr", "th", "td")
		p.AllowAttrs("type", "checked", "disabled").OnElements("input")
		p.AllowAttrs("class").OnElements("li", "ul")
		p.AllowElements("sup", "section")
		p.AllowAttrs("id").OnElements("li", "sup", "a", "section")
		p.AllowAttrs("class").OnElements("section", "ol", "li", "sup")
		p.AllowAttrs("role", "aria-label").OnElements("a", "section")
		p.AllowElements("div")
		p.AllowAttrs("class").OnElements("div", "p")
		p.AllowAttrs("aria-hidden").OnElements("span")
		p.AllowAttrs("class").OnElements("pre", "code", "span")
		p.AllowElements("figure", "figcaption", "details", "summary", "b")
		p.AllowAttrs("class").OnElements("figure", "figcaption", "details")
		// Task 3 (artifact embeds): img/a attributes the figure/chip renderers
		// emit. img `src` is deliberately NOT additionally restricted here via
		// a Matching(re) regexp — bluemonday's UGCPolicy already allows `img
		// src` unconditionally (AllowImages(), nil-Matching) and attribute
		// policies for the same element+attr are OR-combined, so a second,
		// stricter entry would never win against the first permissive one.
		// The actual gate is safeImageHTMLRenderer below, which overrides
		// goldmark's core Image renderer and only ever emits a `src` that
		// matches artifactSrcRe in the first place.
		p.AllowAttrs("alt", "loading", "width", "height").OnElements("img")
		p.AllowAttrs("download").OnElements("a")
		p.AllowAttrs("class").OnElements("img", "a")
		p.AllowAttrs("id").OnElements("h1", "h2", "h3", "h4", "h5", "h6")
		docPolicy = p
	})
	return docPolicy
}

// DocMeta carries render-time facts about a document that the caller needs
// but that the sanitised HTML itself no longer safely exposes. HasMermaid is
// the single source of truth for "does this document need mermaid-init.js" —
// there is deliberately no separate string-scan for a ```mermaid fence,
// which would drift from what the fence parser actually accepted.
type DocMeta struct {
	HasMermaid bool
}

// RenderDocument converts Markdown src to sanitised HTML, extending
// CommonMark with [[target]] and [[target|display]] wikilink syntax,
// ![[slug]] artifact embeds, and setting ```mermaid fences as numbered
// figures. resolve maps a wikilink target string to its href + title; when
// resolve returns ok=false the link renders as a broken-wikilink span
// instead. resolveArtifact maps a ![[slug]] embed's slug to its ArtifactRef;
// a nil resolveArtifact (or one that never resolves) treats every embed as
// unresolved. ctx supplies the locale for figure captions ("Abb." / "Fig.").
func RenderDocument(ctx context.Context, src string, resolve WikilinkResolver, resolveArtifact ArtifactResolver) (template.HTML, DocMeta) {
	if _, start := domain.ParseFrontmatter(src); start > 0 {
		src = src[start:]
	}
	figLabel := components.T(ctx, "document.figure.label")
	renderedFrom := components.T(ctx, "document.figure.mermaid")
	sourceLabel := components.T(ctx, "document.figure.source")
	downloadLabel := components.T(ctx, "document.figure.download")
	unresolvedLabel := components.T(ctx, "document.figure.unresolved")
	ft := &figureTransformer{resolveArtifact: resolveArtifact}
	gm := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
			highlightingExtension(),
		),
		goldmark.WithParserOptions(
			parser.WithInlineParsers(
				util.Prioritized(&wikiLinkParser{}, 100),
			),
			parser.WithASTTransformers(
				util.Prioritized(calloutTransformer{}, 0),
				util.Prioritized(ft, 0),
			),
		),
	)
	gm.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(&wikiLinkHTMLRenderer{resolve: resolve}, 100),
			util.Prioritized(&calloutHTMLRenderer{}, 100),
			util.Prioritized(&mermaidHTMLRenderer{FigLabel: figLabel, RenderedFrom: renderedFrom, SourceLabel: sourceLabel}, 100),
			util.Prioritized(&artifactEmbedHTMLRenderer{resolve: resolveArtifact, FigLabel: figLabel, DownloadLabel: downloadLabel, UnresolvedLabel: unresolvedLabel}, 100),
			util.Prioritized(&safeImageHTMLRenderer{}, 100),
		),
	)
	var buf bytes.Buffer
	if err := gm.Convert([]byte(src), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(src)), DocMeta{}
	}
	clean := getDocPolicy().SanitizeBytes(buf.Bytes())
	return template.HTML(clean), DocMeta{HasMermaid: ft.Count > 0}
}

// artifactSrcRe is the ONLY <img src> shape safeImageHTMLRenderer ever
// emits for a core `![alt](url)` image: the artifact serve route, bare or
// with its "?v={ref}" cache-buster (ref = 12 lowercase hex chars, see
// usecase.UploadNodeLogo's sha256[:12] convention). Anything else — an
// external host, a data: URI, a protocol-relative "//host/..." — renders
// with an empty src.
var artifactSrcRe = regexp.MustCompile(`^/nodes/[A-Za-z0-9_-]+/artifacts/[a-z0-9-]+(\?v=[0-9a-f]{12})?$`)

// safeImageHTMLRenderer overrides goldmark's core Image renderer (same
// ast.KindImage, registered at a lower util.Prioritized number so it wins —
// goldmark.NewRenderer's core html.Renderer registers at priority 1000, and
// registration happens in descending-priority order with each Register call
// overwriting the previous one for that NodeKind, so our priority-100
// registration is applied last and wins).
//
// This is the actual security boundary for inline images (verified against
// the real bluemonday policy, not just reasoned about): RenderDocument never
// sets html.WithUnsafe, so raw <img> HTML typed directly into Markdown is
// already dropped by goldmark itself — the only way a Markdown document can
// produce an <img src> at all is through this ast.KindImage renderer via
// `![alt](url)`. bluemonday's UGCPolicy calls AllowImages() internally,
// which allows `img src` with a nil (match-everything) regexp; attribute
// policies for the same element+attr are OR-combined across all registered
// entries, so ANY additional AllowAttrs("src").Matching(re).OnElements("img")
// layered on top of UGCPolicy is a no-op — the permissive entry from
// AllowImages() already lets everything through regardless. Gating the
// src at render time, before bluemonday ever sees it, is therefore not a
// stylistic choice but the only place this actually works.
type safeImageHTMLRenderer struct{}

func (r *safeImageHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindImage, r.render)
}

func (r *safeImageHTMLRenderer) render(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	n, ok := node.(*ast.Image)
	if !ok {
		return ast.WalkContinue, nil
	}
	dst := string(n.Destination)
	if !artifactSrcRe.MatchString(dst) {
		dst = ""
	}
	_, _ = w.WriteString(`<img src="`)
	_, _ = w.WriteString(html.EscapeString(dst))
	_, _ = w.WriteString(`" alt="`)
	_, _ = w.WriteString(html.EscapeString(imageAltText(source, n)))
	_, _ = w.WriteString(`"`)
	if len(n.Title) > 0 {
		_, _ = w.WriteString(` title="`)
		_, _ = w.WriteString(html.EscapeString(string(n.Title)))
		_, _ = w.WriteString(`"`)
	}
	_, _ = w.WriteString(`>`)
	return ast.WalkSkipChildren, nil
}

// imageAltText collects an Image node's inline children as plain text —
// enough for the common `![text](url)` case and simple nested emphasis,
// without depending on goldmark html.Renderer's unexported renderTexts.
func imageAltText(source []byte, n ast.Node) string {
	var out []byte
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			out = append(out, t.Segment.Value(source)...)
			continue
		}
		out = append(out, imageAltText(source, c)...)
	}
	return string(out)
}

// --- AST node ---------------------------------------------------------

var kindWikiLink = ast.NewNodeKind("WikiLink")

type wikiLinkNode struct {
	ast.BaseInline
	Target  string
	Display string
}

func (w *wikiLinkNode) Kind() ast.NodeKind          { return kindWikiLink }
func (w *wikiLinkNode) Dump(source []byte, lvl int) { ast.DumpHelper(w, source, lvl, nil, nil) }

// --- Inline parser ----------------------------------------------------

type wikiLinkParser struct{}

func (wikiLinkParser) Trigger() []byte { return []byte{'!', '['} }

// Parse handles two syntaxes on top of goldmark's core LinkParser, which
// also triggers on '!' and '[' but sits at priority 200 vs. this parser's
// 100 — this parser is tried first and wins both trigger bytes.
//
//   - "![[slug]]" is an artifact embed (Task 3): returns an
//     artifactEmbedNode. A malformed "![[" (no closing "]]" on the line) or
//     an empty slug returns nil so goldmark's core parsers get a turn —
//     core's image/link parser degrades a stray "![[" to plain text, never
//     a panic. A lone '!' that is not the start of "![[" is not ours either
//     (return nil), which lets core render `![alt](url)` images or a
//     literal '!'.
//   - "[[target]]" / "[[target|display]]" is the pre-existing doc-wikilink
//     syntax, unchanged below.
func (wikiLinkParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, _ := block.PeekLine()
	if len(line) >= 5 && line[0] == '!' && line[1] == '[' && line[2] == '[' {
		end := -1
		for i := 3; i+1 < len(line); i++ {
			if line[i] == '\n' {
				break
			}
			if line[i] == ']' && line[i+1] == ']' {
				end = i
				break
			}
		}
		if end < 0 {
			return nil
		}
		slug := string(line[3:end])
		if slug == "" {
			return nil
		}
		block.Advance(end + 2)
		return &artifactEmbedNode{Slug: slug}
	}
	if len(line) > 0 && line[0] == '!' {
		return nil
	}
	if len(line) < 4 || line[0] != '[' || line[1] != '[' {
		return nil
	}
	end := -1
	for i := 2; i+1 < len(line); i++ {
		if line[i] == '\n' {
			break
		}
		if line[i] == ']' && line[i+1] == ']' {
			end = i
			break
		}
	}
	if end < 0 {
		return nil
	}
	inner := string(line[2:end])
	if inner == "" {
		return nil
	}
	target, display := splitWikiLinkInner(inner)
	if target == "" {
		return nil
	}
	block.Advance(end + 2)
	return &wikiLinkNode{Target: target, Display: display}
}

// splitWikiLinkInner splits "target|display" into its two halves.
// Returns ("", "") when a bare `]` or newline appears before any `|`.
func splitWikiLinkInner(s string) (target, display string) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '|':
			return s[:i], s[i+1:]
		case '\n', ']':
			return "", ""
		}
	}
	return s, ""
}

// --- HTML renderer ----------------------------------------------------

type wikiLinkHTMLRenderer struct {
	resolve WikilinkResolver
}

func (r *wikiLinkHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindWikiLink, r.render)
}

func (r *wikiLinkHTMLRenderer) render(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	wl, ok := n.(*wikiLinkNode)
	if !ok {
		return ast.WalkContinue, nil
	}

	href, title, resolved := r.resolve(wl.Target)

	display := wl.Display
	if display == "" {
		if resolved && title != "" {
			display = title
		} else {
			display = wl.Target
		}
	}

	if resolved {
		_, _ = w.WriteString(`<a class="wikilink" href="`)
		_, _ = w.WriteString(html.EscapeString(href))
		_, _ = w.WriteString(`">`)
		_, _ = w.WriteString(html.EscapeString(display))
		_, _ = w.WriteString(`</a>`)
	} else {
		_, _ = w.WriteString(`<span class="wikilink-broken">`)
		_, _ = w.WriteString(html.EscapeString(display))
		_, _ = w.WriteString(`</span>`)
	}
	return ast.WalkSkipChildren, nil
}
