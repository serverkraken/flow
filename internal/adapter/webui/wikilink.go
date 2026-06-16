package webui

import (
	"bytes"
	"html"
	"html/template"
	"sync"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/serverkraken/flow/internal/domain"
)

// WikilinkResolver maps a wikilink target string to an href, an
// optional display title, and whether the target is known.
type WikilinkResolver func(target string) (href, title string, ok bool)

// docPolicy is the bluemonday policy used by RenderDocument. It
// extends UGCPolicy to preserve the `class` attribute (for
// wikilink/wikilink-broken markers) and relative hrefs (e.g.
// /docs/d-arch).
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
		docPolicy = p
	})
	return docPolicy
}

// RenderDocument converts Markdown src to sanitised HTML, extending
// CommonMark with [[target]] and [[target|display]] wikilink syntax.
// The resolve function maps a target string to its href + title; when
// resolve returns ok=false the link renders as a broken-wikilink span
// instead.
func RenderDocument(src string, resolve WikilinkResolver) template.HTML {
	if _, start := domain.ParseFrontmatter(src); start > 0 {
		src = src[start:]
	}
	gm := goldmark.New(
		goldmark.WithParserOptions(
			parser.WithInlineParsers(
				util.Prioritized(&wikiLinkParser{}, 100),
			),
		),
	)
	gm.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(&wikiLinkHTMLRenderer{resolve: resolve}, 100),
		),
	)
	var buf bytes.Buffer
	if err := gm.Convert([]byte(src), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(src))
	}
	clean := getDocPolicy().SanitizeBytes(buf.Bytes())
	return template.HTML(clean)
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

func (wikiLinkParser) Trigger() []byte { return []byte{'['} }

func (wikiLinkParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, _ := block.PeekLine()
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
