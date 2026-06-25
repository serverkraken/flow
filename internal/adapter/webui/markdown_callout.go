package webui

import (
	"regexp"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

var calloutKinds = map[string]bool{
	"note":      true,
	"tip":       true,
	"warning":   true,
	"important": true,
	"danger":    true,
}

var calloutRe = regexp.MustCompile(`^\[!([A-Za-z]+)\]\s*(.*)$`)

var kindCallout = ast.NewNodeKind("Callout")

type calloutNode struct {
	ast.BaseBlock
	CalloutKind string
	Title       string
}

func (n *calloutNode) Kind() ast.NodeKind       { return kindCallout }
func (n *calloutNode) Dump(src []byte, lvl int) { ast.DumpHelper(n, src, lvl, nil, nil) }

type calloutTransformer struct{}

func (calloutTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	src := reader.Source()
	var targets []*ast.Blockquote
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if bq, ok := n.(*ast.Blockquote); ok {
				targets = append(targets, bq)
			}
		}
		return ast.WalkContinue, nil
	})

	for _, bq := range targets {
		first := bq.FirstChild()
		if first == nil || first.Type() != ast.TypeBlock {
			continue
		}
		m := calloutRe.FindStringSubmatch(strings.TrimSpace(firstLineText(first, src)))
		if m == nil {
			continue
		}
		kind := strings.ToLower(m[1])
		if !calloutKinds[kind] {
			continue
		}

		cn := &calloutNode{CalloutKind: kind, Title: strings.TrimSpace(m[2])}
		stripFirstLine(first)
		for c := bq.FirstChild(); c != nil; {
			next := c.NextSibling()
			bq.RemoveChild(bq, c)
			cn.AppendChild(cn, c)
			c = next
		}
		bq.Parent().ReplaceChild(bq.Parent(), bq, cn)
	}
}

func firstLineText(n ast.Node, src []byte) string {
	if n.Lines() != nil && n.Lines().Len() > 0 {
		seg := n.Lines().At(0)
		return string(seg.Value(src))
	}
	return ""
}

func stripFirstLine(n ast.Node) {
	if n.Lines() == nil || n.Lines().Len() == 0 {
		return
	}
	lines := n.Lines()
	rebuilt := text.NewSegments()
	for i := 1; i < lines.Len(); i++ {
		rebuilt.Append(lines.At(i))
	}
	n.SetLines(rebuilt)
}

type calloutHTMLRenderer struct{}

func (r *calloutHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindCallout, r.render)
}

var calloutGlyph = map[string]string{
	"note":      "●",
	"tip":       "✓",
	"warning":   "▲",
	"important": "★",
	"danger":    "✗",
}

func (r *calloutHTMLRenderer) render(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*calloutNode)
	if !entering {
		_, _ = w.WriteString(`</div>`)
		return ast.WalkContinue, nil
	}

	_, _ = w.WriteString(`<div class="callout callout-` + n.CalloutKind + `">`)
	_, _ = w.WriteString(`<p class="callout-title"><span class="callout-glyph" aria-hidden="true">`)
	_, _ = w.WriteString(calloutGlyph[n.CalloutKind])
	_, _ = w.WriteString(`</span> `)
	title := n.Title
	if title == "" {
		title = strings.ToUpper(n.CalloutKind[:1]) + n.CalloutKind[1:]
	}
	_, _ = w.Write(util.EscapeHTML([]byte(title)))
	_, _ = w.WriteString(`</p>`)
	return ast.WalkContinue, nil
}
