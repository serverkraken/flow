// Footnote renderers. Inline `[^id]` references render as Unicode
// superscript numbers (¹²³…); the matching definitions render as a
// titled list at the very end of the document — both anchored on
// goldmark's footnote extension AST nodes.

package markdown

import (
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/util"
)

// renderFootnoteLink emits the inline `[^id]` reference as a styled
// superscript number. goldmark assigns a 1-based Index per footnote
// in document order, so consecutive refs read as `¹`, `²`, `³` …
func (r *nodeRenderer) renderFootnoteLink(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	link, ok := n.(*extast.FootnoteLink)
	if !ok {
		return ast.WalkContinue, nil
	}
	_, _ = w.WriteString(r.roles.FootnoteRef.Render(superscript(link.Index)))
	return ast.WalkSkipChildren, nil
}

// renderFootnoteList emits the entire footnote section: a separator
// line, the "Footnotes" heading, then each definition prefixed with
// its superscript index.
func (r *nodeRenderer) renderFootnoteList(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	width := r.width
	if width <= 0 {
		width = 40
	}
	_, _ = w.WriteString("\n")
	_, _ = w.WriteString(r.roles.CardSeparator.Render(strings.Repeat("─", width)))
	_, _ = w.WriteString("\n\n")
	_, _ = w.WriteString(r.roles.FootnoteListTitle.Render("Footnotes"))
	_, _ = w.WriteString("\n\n")
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		fn, ok := c.(*extast.Footnote)
		if !ok {
			continue
		}
		body, err := r.renderChildrenToString(source, fn)
		if err != nil {
			return ast.WalkStop, err
		}
		body = strings.TrimRight(body, "\n")
		marker := r.roles.FootnoteRef.Render(superscript(fn.Index)) + " "
		_, _ = w.WriteString(prefixFirstLine(r.roles.FootnoteDef.Render(body), marker, "  "))
		_, _ = w.WriteString("\n")
	}
	_, _ = w.WriteString("\n")
	return ast.WalkSkipChildren, nil
}

// renderFootnote is a no-op — footnote definitions are walked
// explicitly by renderFootnoteList. Without registering this kind
// goldmark would dispatch to the standard html renderer for it.
func (r *nodeRenderer) renderFootnote(_ util.BufWriter, _ []byte, _ ast.Node, _ bool) (ast.WalkStatus, error) {
	return ast.WalkSkipChildren, nil
}

// renderFootnoteBacklink swallows the back-arrow goldmark inserts
// next to each definition. It only makes sense in HTML where the
// reader can click to jump back; in the terminal it's noise.
func (r *nodeRenderer) renderFootnoteBacklink(_ util.BufWriter, _ []byte, _ ast.Node, _ bool) (ast.WalkStatus, error) {
	return ast.WalkSkipChildren, nil
}

// superscript converts an int (1..) to its Unicode superscript form
// (`¹²³…`). Falls back to `^N` for negative or zero inputs that
// shouldn't reach the renderer in practice.
func superscript(n int) string {
	if n <= 0 {
		return "^" + strconv.Itoa(n)
	}
	digits := []rune("⁰¹²³⁴⁵⁶⁷⁸⁹")
	s := strconv.Itoa(n)
	var b strings.Builder
	for _, c := range s {
		b.WriteRune(digits[c-'0'])
	}
	return b.String()
}
