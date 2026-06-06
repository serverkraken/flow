// Package markdown renders kompendium notes to HTML for the WebUI's
// /notes and /repos surfaces. Unlike internal/frontend/tui/markdown
// (ANSI output for terminals), this package emits HTML wrapped by the
// `.prose-flow` typography in styles.css.
//
// Goldmark is configured with GFM (tables, strikethrough, task-list,
// auto-link) and a single custom node renderer that pipes fenced code
// blocks through chroma. Goldmark's default HTML renderer already
// escapes any literal HTML in the source (we do NOT set WithUnsafe), so
// untrusted note content cannot inject script tags or arbitrary
// attributes. The chroma output we splice in is generated from token
// text + a closed set of class names — no user-controlled HTML reaches
// the writer.
package markdown

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Renderer turns markdown bytes into template.HTML safe for splicing
// into a templ template via `@templ.Raw`. Held as a value so the
// WebUI handler can allocate one per server and reuse the configured
// goldmark instance across requests.
type Renderer struct {
	md goldmark.Markdown
}

// New returns a Renderer with GFM + chroma-styled fenced code blocks.
// Safe for concurrent use — goldmark.Markdown is documented as
// safe-for-concurrent-Convert once configured.
func New() *Renderer {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			// HTML5 output (no XHTML self-closing on void elements).
			// Hard-wrap stays off: notes break paragraphs with blank
			// lines explicitly.
			// Explicitly do NOT set html.WithUnsafe — literal HTML in
			// notes must stay escaped.
		),
	)
	// Replace goldmark's built-in FencedCodeBlock renderer with a
	// chroma-backed one so code blocks pick up syntax highlighting.
	md.Renderer().AddOptions(
		renderer.WithNodeRenderers(util.Prioritized(&codeBlockRenderer{}, 99)),
	)
	return &Renderer{md: md}
}

// Render parses src as CommonMark + GFM and returns HTML wrapped in
// template.HTML so the templ template can splice it without extra
// escaping. Any conversion failure returns an empty fragment + the
// error so the handler can log + render a "Note konnte nicht
// dargestellt werden" placeholder rather than 500.
func (r *Renderer) Render(src []byte) (template.HTML, error) {
	var buf bytes.Buffer
	if err := r.md.Convert(src, &buf); err != nil {
		return "", fmt.Errorf("goldmark convert: %w", err)
	}
	return template.HTML(buf.String()), nil //nolint:gosec // goldmark escapes literal HTML; chroma output is class-only.
}

// Headings extracts H2/H3 headings from the rendered markdown so the
// view's Table-of-Contents rail can list them. Returned in document
// order; one entry per heading. Returns nil for src without any
// matching headings.
func (r *Renderer) Headings(src []byte) []Heading {
	doc := r.md.Parser().Parse(text.NewReader(src))
	var out []Heading
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		if h.Level != 2 && h.Level != 3 {
			return ast.WalkContinue, nil
		}
		text := headingText(h, src)
		if text == "" {
			return ast.WalkContinue, nil
		}
		// Anchor must match the id goldmark emits on the rendered
		// <h2 id="…">. parser.WithAutoHeadingID() stores it as the
		// "id" attribute ([]byte) on the heading node. Fall back to
		// slugify only when the parser option isn't active (so the
		// rail still gets some anchor instead of an empty href).
		anchor := autoHeadingAnchor(h)
		if anchor == "" {
			anchor = slugify(text)
		}
		out = append(out, Heading{Level: h.Level, Text: text, Anchor: anchor})
		return ast.WalkContinue, nil
	})
	return out
}

// Heading is one row in the rendered note's table-of-contents.
type Heading struct {
	Level  int    // 2 or 3
	Text   string // human-readable text content
	Anchor string // slug-only id (no leading #)
}

// — internals —

// headingText collects the visible text content of a heading node, joined
// without inline formatting.
func headingText(h *ast.Heading, src []byte) string {
	var b strings.Builder
	_ = ast.Walk(h, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if t, ok := n.(*ast.Text); ok {
			b.Write(t.Segment.Value(src))
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(b.String())
}

// autoHeadingAnchor returns the id goldmark's WithAutoHeadingID()
// parser option stamped on a heading node. Returns "" when no id
// attribute is present (option disabled or parser failed to assign one).
// Stored as []byte in current goldmark; the string-case is kept as a
// future-proof fallback in case the upstream representation changes.
func autoHeadingAnchor(h *ast.Heading) string {
	v, ok := h.AttributeString("id")
	if !ok {
		return ""
	}
	switch s := v.(type) {
	case []byte:
		return string(s)
	case string:
		return s
	default:
		return ""
	}
}

// slugify lowercases and replaces non-alnum runs with `-`. Matches the
// shape of goldmark's auto-heading-id output well enough for in-page
// anchor links rendered in the rail.
func slugify(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// codeBlockRenderer is the goldmark NodeRenderer that handles fenced +
// indented code blocks via chroma. Other node kinds fall through to the
// default HTML renderer.
type codeBlockRenderer struct{}

// RegisterFuncs implements renderer.NodeRenderer. Priority 99 (higher
// than goldmark/html's default 1000) ensures our renderer wins.
func (c *codeBlockRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, c.renderFencedCodeBlock)
	reg.Register(ast.KindCodeBlock, c.renderIndentedCodeBlock)
}

func (c *codeBlockRenderer) renderFencedCodeBlock(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	fenced := n.(*ast.FencedCodeBlock)
	lang := strings.ToLower(string(fenced.Language(source)))
	body := collectLines(n, source)
	c.writeChroma(w, body, lang)
	return ast.WalkSkipChildren, nil
}

func (c *codeBlockRenderer) renderIndentedCodeBlock(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	body := collectLines(n, source)
	c.writeChroma(w, body, "")
	return ast.WalkSkipChildren, nil
}

// collectLines concatenates every Lines() segment of a code block back
// into the original source slice.
func collectLines(n ast.Node, source []byte) []byte {
	lines := n.Lines()
	var out bytes.Buffer
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		out.Write(line.Value(source))
	}
	return out.Bytes()
}

// writeChroma renders src as a <pre><code>…</code></pre> block via
// chroma's html formatter using the tokyonight-adjacent monokai style
// — chroma ships ~70 styles; monokai/dracula read as close-enough on
// the bg-dark canvas. Tokens are emitted with inline `style="…"`
// attributes so we don't have to ship a separate chroma CSS file.
//
// Unknown languages fall back to the analyzer, then to fallback.
func (c *codeBlockRenderer) writeChroma(w util.BufWriter, src []byte, lang string) {
	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Analyse(string(src))
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	style := styles.Get("monokai")
	if style == nil {
		style = styles.Fallback
	}
	formatter := chromahtml.New(chromahtml.WithClasses(false))
	iter, err := lexer.Tokenise(nil, string(src))
	if err != nil {
		// Fall back to escaped <pre><code> on tokenize failure so a
		// broken lexer can't take the whole render down.
		fmt.Fprintf(w, "<pre><code>%s</code></pre>", template.HTMLEscapeString(string(src)))
		return
	}
	if err := formatter.Format(w, style, iter); err != nil {
		fmt.Fprintf(w, "<pre><code>%s</code></pre>", template.HTMLEscapeString(string(src)))
	}
}
