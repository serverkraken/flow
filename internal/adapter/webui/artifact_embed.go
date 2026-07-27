package webui

import (
	"fmt"
	"html"
	"strconv"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// ArtifactRef is what an ArtifactResolver returns for a resolved ![[slug]]
// embed — everything the renderer needs to emit either an inline <img>
// figure (IsImage) or a downloadable file chip. Href points at the
// artifact's OWN node ("/nodes/{artifact.NodeID}/artifacts/{slug}"), or at
// the owner-global free library ("/artefakte/{slug}") when the artifact is
// node-less. Either can differ from the document's node — the artifact may
// live on an ancestor or in the free library — and pointing at the
// document's node instead would 404 on the serve route. Ref (the content
// hash) is mandatory: the image renderer appends it
// as "?v={Ref}" so the <img src> is content-addressed and thus
// immutable-cacheable; the file chip deliberately uses the bare Href (no
// "?v=") so a rename is reflected on the very next download instead of
// serving a stale cached filename.
type ArtifactRef struct {
	Href    string
	Ref     string
	Name    string
	Mime    string
	SizeStr string
	IsImage bool
	Width   int
	Height  int
}

// ArtifactResolver maps a ![[slug]] embed's slug to its ArtifactRef. A nil
// resolver (or one that never resolves the slug) treats every embed as
// unresolved — RenderDocument callers pass nil when no artifact library is
// available for the current scope (node chain or free library).
type ArtifactResolver func(slug string) (ArtifactRef, bool)

// FormatArtifactSize renders a byte count as a short human string ("512 B",
// "12.3 KB", "4.0 MB") for the file-chip's SizeStr field.
func FormatArtifactSize(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(1024), 0
	for n/div >= 1024 && exp < 3 {
		div *= 1024
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

// --- AST node -----------------------------------------------------------

var kindArtifactEmbed = ast.NewNodeKind("ArtifactEmbed")

// artifactEmbedNode is the ![[slug]] inline node. N is filled in by
// figureTransformer's numbering walk (mermaid.go) — it stays 0 for an
// unresolved embed, which the renderer reads as "don't print a figure
// number" (unresolved embeds never consume one, see figureTransformer).
type artifactEmbedNode struct {
	ast.BaseInline
	Slug string
	N    int
}

func (n *artifactEmbedNode) Kind() ast.NodeKind       { return kindArtifactEmbed }
func (n *artifactEmbedNode) Dump(src []byte, lvl int) { ast.DumpHelper(n, src, lvl, nil, nil) }

// --- HTML renderer -------------------------------------------------------

// artifactEmbedHTMLRenderer renders kindArtifactEmbed nodes. It calls
// resolve again at render time (the numbering transformer already called it
// once to decide whether the embed earns a figure number) — resolve is a
// cheap map lookup, so the duplicate call trades a few nanoseconds for
// keeping the transformer and the renderer independently simple.
type artifactEmbedHTMLRenderer struct {
	resolve         ArtifactResolver
	FigLabel        string
	DownloadLabel   string
	UnresolvedLabel string
}

func (r *artifactEmbedHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindArtifactEmbed, r.render)
}

func (r *artifactEmbedHTMLRenderer) render(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	n, ok := node.(*artifactEmbedNode)
	if !ok {
		return ast.WalkContinue, nil
	}

	var ref ArtifactRef
	var resolved bool
	if r.resolve != nil {
		ref, resolved = r.resolve(n.Slug)
	}
	if !resolved {
		_, _ = w.WriteString(`<span class="wikilink-broken" title="`)
		_, _ = w.WriteString(html.EscapeString(r.UnresolvedLabel))
		_, _ = w.WriteString(`">`)
		_, _ = w.WriteString(html.EscapeString(n.Slug))
		_, _ = w.WriteString(`</span>`)
		return ast.WalkSkipChildren, nil
	}

	_, _ = w.WriteString(`<figure class="figure">`)
	if ref.IsImage {
		_, _ = w.WriteString(`<div class="frame"><img loading="lazy" src="`)
		_, _ = w.WriteString(html.EscapeString(ref.Href + "?v=" + ref.Ref))
		_, _ = w.WriteString(`" alt="`)
		_, _ = w.WriteString(html.EscapeString(ref.Name))
		_, _ = w.WriteString(`"`)
		if ref.Width > 0 {
			_, _ = w.WriteString(` width="` + strconv.Itoa(ref.Width) + `"`)
		}
		if ref.Height > 0 {
			_, _ = w.WriteString(` height="` + strconv.Itoa(ref.Height) + `"`)
		}
		_, _ = w.WriteString(`></div>`)
	} else {
		_, _ = w.WriteString(`<a class="filechip" href="`)
		_, _ = w.WriteString(html.EscapeString(ref.Href))
		_, _ = w.WriteString(`" download aria-label="`)
		_, _ = w.WriteString(html.EscapeString(r.DownloadLabel + ": " + ref.Name))
		_, _ = w.WriteString(`">■ `)
		_, _ = w.WriteString(html.EscapeString(ref.Name))
		_, _ = w.WriteString(` · `)
		_, _ = w.WriteString(html.EscapeString(ref.SizeStr))
		_, _ = w.WriteString(`</a>`)
	}
	_, _ = w.WriteString(`<figcaption><b>`)
	_, _ = w.WriteString(html.EscapeString(r.FigLabel))
	_, _ = w.WriteString(` `)
	_, _ = w.WriteString(strconv.Itoa(n.N))
	_, _ = w.WriteString(`</b> · `)
	_, _ = w.WriteString(html.EscapeString(ref.Name))
	_, _ = w.WriteString(`</figcaption></figure>`)
	return ast.WalkSkipChildren, nil
}
