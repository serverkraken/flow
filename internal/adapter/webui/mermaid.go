package webui

import (
	"html"
	"strconv"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Mermaid diagrams are set as a numbered figure, never rendered through
// goldmark's FencedCodeBlock renderer slot. goldmark-highlighting owns
// ast.KindFencedCodeBlock at priority 200 and that dispatch is exclusive per
// NodeKind — a renderer registered on KindFencedCodeBlock (at any priority)
// would steal every fenced block, silently dropping Chroma highlighting from
// every other language. Instead this follows the calloutTransformer pattern:
// an AST transformer swaps mermaid-language FencedCodeBlock nodes for a
// dedicated mermaidNode, and a renderer is registered for that new kind only
// — Chroma's FencedCodeBlock renderer is left untouched for every other
// fence.

var kindMermaid = ast.NewNodeKind("Mermaid")

type mermaidNode struct {
	ast.BaseBlock
	Source string
	N      int
}

func (n *mermaidNode) Kind() ast.NodeKind       { return kindMermaid }
func (n *mermaidNode) Dump(src []byte, lvl int) { ast.DumpHelper(n, src, lvl, nil, nil) }

// figureTransformer does two passes over the document. (1) It replaces every
// ```mermaid fenced code block with an (unnumbered) mermaidNode — Count
// tracks how many were found, read back by RenderDocument after Convert to
// populate DocMeta.HasMermaid. (2) It then walks the WHOLE document once
// more, in source order, and assigns the shared figure counter: every
// mermaidNode gets the next number, and every artifactEmbedNode (Task 3,
// artifact_embed.go) gets the next number too, but ONLY if resolveArtifact
// resolves its slug — an unresolved ![[slug]] must not consume a number
// (Spec OE #4), or a broken reference would leave a visible gap ("Abb. 1",
// "Abb. 3" with no "Abb. 2" anywhere). Both node kinds' HTML renderers only
// ever read N back; they never number themselves, which is what keeps a
// mixed Mermaid/artifact document numbered in one true document order
// instead of two independent per-kind counters.
type figureTransformer struct {
	resolveArtifact ArtifactResolver
	Count           int
}

func (t *figureTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	src := reader.Source()
	var targets []*ast.FencedCodeBlock
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if fcb, ok := n.(*ast.FencedCodeBlock); ok {
				if string(fcb.Language(src)) == "mermaid" {
					targets = append(targets, fcb)
				}
			}
		}
		return ast.WalkContinue, nil
	})

	for _, fcb := range targets {
		t.Count++
		mn := &mermaidNode{Source: string(fcb.Lines().Value(src))}
		fcb.Parent().ReplaceChild(fcb.Parent(), fcb, mn)
	}

	n := 0
	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := node.(type) {
		case *mermaidNode:
			n++
			v.N = n
		case *artifactEmbedNode:
			if t.resolveArtifact != nil {
				if _, ok := t.resolveArtifact(v.Slug); ok {
					n++
					v.N = n
				}
			}
		}
		return ast.WalkContinue, nil
	})
}

// mermaidHTMLRenderer renders kindMermaid nodes as a progressively enhanced
// figure: the raw source is readable in <pre class="mermaid"> without JS;
// mermaid-init.js (Task 4) replaces it with the rendered SVG. The source is
// also reachable via the collapsed <details> once a diagram has replaced it.
type mermaidHTMLRenderer struct {
	FigLabel     string
	RenderedFrom string
	SourceLabel  string
}

func (r *mermaidHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindMermaid, r.render)
}

func (r *mermaidHTMLRenderer) render(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n, ok := node.(*mermaidNode)
	if !ok {
		return ast.WalkContinue, nil
	}
	escaped := html.EscapeString(n.Source)

	_, _ = w.WriteString(`<figure class="mermaid-figure">`)
	_, _ = w.WriteString(`<div class="frame"><pre class="mermaid">`)
	_, _ = w.WriteString(escaped)
	_, _ = w.WriteString(`</pre></div>`)
	_, _ = w.WriteString(`<figcaption><b>`)
	_, _ = w.WriteString(html.EscapeString(r.FigLabel))
	_, _ = w.WriteString(` `)
	_, _ = w.WriteString(strconv.Itoa(n.N))
	_, _ = w.WriteString(`</b> · <span class="mermaid-cap">`)
	_, _ = w.WriteString(html.EscapeString(r.RenderedFrom))
	_, _ = w.WriteString(`</span></figcaption>`)
	_, _ = w.WriteString(`<details class="mermaid-src"><summary>`)
	_, _ = w.WriteString(html.EscapeString(r.SourceLabel))
	_, _ = w.WriteString(`</summary><pre><code>`)
	_, _ = w.WriteString(escaped)
	_, _ = w.WriteString(`</code></pre></details>`)
	_, _ = w.WriteString(`</figure>`)
	return ast.WalkSkipChildren, nil
}
