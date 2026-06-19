// Wikilink AST node + parser + renderer. Adds `[[id]]` and
// `[[id|display]]` syntax on top of CommonMark via a custom inline
// parser, then resolves the target through the injected
// ports.WikilinkResolver to decide whether the link renders as valid
// (OSC 8 to whatever URI the resolver returns; kompendium uses
// kompendium://note/<id>) or broken (red marker, no link).

package markdown

import (
	"strconv"

	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// wikiLinkKind identifies the WikiLink AST node so dispatch can
// route it to renderWikiLink. The name is intentionally neutral —
// the renderer is shared between the cheatsheet (no kompendium
// context) and the kompendium browse / view screens, so the AST
// discriminator must not carry kompendium semantics.
var wikiLinkKind = ast.NewNodeKind("WikiLink")

// wikiLink is a leaf inline AST node carrying the parsed target +
// optional display override. Its inline children are not used —
// rendering reads Target / Display directly.
type wikiLink struct {
	ast.BaseInline
	Target  string
	Display string
}

func (w *wikiLink) Kind() ast.NodeKind          { return wikiLinkKind }
func (w *wikiLink) Dump(source []byte, lvl int) { ast.DumpHelper(w, source, lvl, nil, nil) }

// wikiLinkParser is the goldmark inline parser. Triggered by `[`,
// it checks for `[[`, then consumes up to the closing `]]`. Falls
// through (returns nil) when the syntax doesn't match so other
// `[…]` constructs (markdown links, references) keep working.
type wikiLinkParser struct{}

func (wikiLinkParser) Trigger() []byte { return []byte{'['} }

// Parse implements parser.InlineParser. Reads up to the closing `]]`
// and emits a wikiLink node when matched. Newlines or unmatched
// closer abort with nil so the bracket falls through to default
// CommonMark handling.
func (wikiLinkParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, _ := block.PeekLine()
	if len(line) < 4 || line[0] != '[' || line[1] != '[' {
		return nil
	}
	// Find the closing `]]`. Stop at newline — a wikilink that spans a
	// line break is bracket noise.
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
	return &wikiLink{Target: target, Display: display}
}

// splitWikiLinkInner splits the captured `id|display` body into its
// two halves. Display is empty when no pipe is present.
func splitWikiLinkInner(s string) (target, display string) {
	for i := 0; i < len(s); i++ {
		if s[i] == '|' {
			return s[:i], s[i+1:]
		}
		if s[i] == '\n' || s[i] == ']' {
			return "", ""
		}
	}
	return s, ""
}

// renderWikiLink writes one wikilink as a styled span. Valid links
// (resolver returns ok=true) carry an OSC 8 hyperlink to the URI
// the resolver supplies; broken links wear the WikilinkBroken style
// without a hyperlink so the user notices the dead reference.
func (r *nodeRenderer) renderWikiLink(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	wl, ok := n.(*wikiLink)
	if !ok {
		return ast.WalkContinue, nil
	}
	display := wl.Display
	if display == "" {
		display = wl.Target
	}
	uri, title, found := "", "", false
	if r.opts.resolver != nil {
		uri, title, found = r.opts.resolver.Resolve(wl.Target)
	}
	if found {
		if title != "" && wl.Display == "" {
			display = title
		}
		_, _ = w.WriteString(r.styleWikiLink(display, uri, true))
	} else {
		_, _ = w.WriteString(r.styleWikiLink(display, "", false))
	}
	return ast.WalkSkipChildren, nil
}

// styleWikiLink builds the visible span: a leading glyph (→ for
// valid, ⊘ for broken), the styled display text, and — when valid —
// an OSC 8 hyperlink wrap with a per-render id so multi-line wraps
// stay one click target. The glyph picks were swapped from ⇲/⌧ to
// →/⊘ — both are common Unicode; → reads as "follows / link to" and
// ⊘ reads as "no entry / not found", much closer to what the eye
// expects from a notebook cross-reference.
func (r *nodeRenderer) styleWikiLink(display, uri string, valid bool) string {
	if !valid {
		return r.roles.WikilinkBroken.Render("⊘ " + display)
	}
	r.osc8ID++
	return osc8Wrap(uri, r.osc8ID, r.roles.WikilinkValid.Render("→ "+display))
}

// osc8Wrap returns text wrapped in an OSC 8 hyperlink with the given
// uri + id. Empty uri short-circuits to the bare text — callers
// that already verified the link is broken don't need to special-
// case here. The id parameter lets terminals join multi-line wraps
// of the same link into one click target.
//
// Implementation routes through charmbracelet/x/ansi (the same
// package lipgloss v2 and bubbletea v2 use internally) so the
// emitted byte sequence stays in lockstep with the rest of the v2
// stack; the previous hand-rolled `\x1b]8;…\x07` string was a single
// point that could drift if the upstream OSC 8 format ever changed.
func osc8Wrap(uri string, id int, text string) string {
	if uri == "" {
		return text
	}
	return ansi.SetHyperlink(uri, "id="+strconv.Itoa(id)) + text + ansi.ResetHyperlink()
}
