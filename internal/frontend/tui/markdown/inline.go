// Inline-level renderers: text, emphasis, code-span, links,
// images, hard breaks. The full link / wikilink / OSC 8 treatment
// lands in P1.11.4 — for now links emit styled visible text + the
// raw URL in parentheses so notes stay readable.

package markdown

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/util"
)

// renderText emits the segment of source covered by this Text node.
// Soft and hard line breaks are surfaced as a space and a \n
// respectively (CommonMark: a soft break inside a paragraph is a
// space; a hard break — two trailing spaces or a backslash — is a
// real newline).
func (r *nodeRenderer) renderText(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	t := n.(*ast.Text)
	_, _ = w.Write(t.Segment.Value(source))
	if t.HardLineBreak() {
		_, _ = w.WriteString("\n")
	} else if t.SoftLineBreak() {
		_, _ = w.WriteString(" ")
	}
	return ast.WalkContinue, nil
}

// renderString emits a literal string node. Used by extensions whose
// inline content carries auxiliary text (e.g. linkify-injected URLs)
// that doesn't have a Segment in the source.
func (r *nodeRenderer) renderString(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	if s, ok := n.(*ast.String); ok {
		_, _ = w.Write(s.Value)
	}
	return ast.WalkContinue, nil
}

// renderEmphasis wraps inner content with bold (level 2) or italic
// (level 1). goldmark normalises `*x*`/`_x_` to level 1 and `**x**`/
// `__x__` to level 2 — there is no separate Strong node type.
func (r *nodeRenderer) renderEmphasis(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	em := n.(*ast.Emphasis)
	inner, err := r.renderInlineToString(source, n)
	if err != nil {
		return ast.WalkStop, err
	}
	if em.Level >= 2 {
		_, _ = w.WriteString(r.roles.Strong.Render(inner))
	} else {
		_, _ = w.WriteString(r.roles.Emph.Render(inner))
	}
	return ast.WalkSkipChildren, nil
}

// renderStrikethrough wraps inner content with SGR 9 (strikethrough).
// goldmark's GFM extension parses `~~text~~` into an extast.Strikethrough
// node; without an explicit handler the default HTML renderer fires and
// the strike disappears from the ANSI output.
//
// Bypasses lipgloss.Render (the Strike role) because lipgloss emits one
// open/close pair **per grapheme cluster** for Strikethrough — defensive
// against terminals that reset the attribute on space, but the bytes
// balloon ~30× without visual benefit. A manual `\x1b[9m...\x1b[29m`
// wrap is the same effect with one open + one close. Inner SGRs from
// nested inline spans (bold, code, …) round-trip unchanged because
// `[29m` only resets strike, leaving fg/bg intact.
func (r *nodeRenderer) renderStrikethrough(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	inner, err := r.renderInlineToString(source, n)
	if err != nil {
		return ast.WalkStop, err
	}
	if r.opts.noColor || isASCIIProfile(r.opts.lip) {
		_, _ = w.WriteString(inner)
		return ast.WalkSkipChildren, nil
	}
	_, _ = w.WriteString("\x1b[9m" + inner + "\x1b[29m")
	return ast.WalkSkipChildren, nil
}

// isASCIIProfile reports whether the lipgloss renderer sits on the
// Ascii color profile (NO_COLOR path). The renderer's exposed
// ColorProfile method is the authoritative read; nil renderer falls
// back to "non-ascii" so the caller emits SGR by default.
func isASCIIProfile(r *lipgloss.Renderer) bool {
	if r == nil {
		return false
	}
	return r.ColorProfile() == termenv.Ascii
}

// renderCodeSpan emits inline `code` with a coloured BG span and
// hair-space padding so the BG reads as a chip rather than tinted
// letters. Children of a CodeSpan are always Text nodes.
func (r *nodeRenderer) renderCodeSpan(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	var inner []byte
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			inner = append(inner, t.Segment.Value(source)...)
		}
	}
	_, _ = w.WriteString(r.roles.CodeSpan.Render(" " + string(inner) + " "))
	return ast.WalkSkipChildren, nil
}

// renderLink emits the link text styled with LinkText (color, no
// underline — the lipgloss per-cell underline quirk would split the
// span char-by-char) and wraps the whole span in OSC 8 so the link
// is clickable in terminals that support hyperlinks.
func (r *nodeRenderer) renderLink(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	link := n.(*ast.Link)
	inner, err := r.renderInlineToString(source, n)
	if err != nil {
		return ast.WalkStop, err
	}
	dest := string(link.Destination)
	r.osc8ID++
	_, _ = w.WriteString(osc8Wrap(dest, r.osc8ID, r.roles.LinkText.Render(inner)))
	return ast.WalkSkipChildren, nil
}

// renderAutoLink emits the URL as styled text wrapped in OSC 8.
// Both <https://…> autolinks and GFM-linkify-detected bare URLs end
// up here.
func (r *nodeRenderer) renderAutoLink(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	a := n.(*ast.AutoLink)
	url := string(a.URL(source))
	r.osc8ID++
	_, _ = w.WriteString(osc8Wrap(url, r.osc8ID, r.roles.LinkText.Render(url)))
	return ast.WalkSkipChildren, nil
}

// renderImage emits a compact `[image: alt]` chip in the ImageChip
// style and stows the URL in an OSC 8 wrap when present. Including
// the URL in the chip body bloated the chip across multiple lines
// for any non-trivial URL and triggered the WrapURLs post-process to
// double-wrap it; clickable behaviour belongs in OSC 8 where the URL
// is invisible until hover. Real graphics rendering (Kitty / Sixel /
// chafa) is deferred to P1.13.
func (r *nodeRenderer) renderImage(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	img := n.(*ast.Image)
	alt, err := r.renderInlineToString(source, n)
	if err != nil {
		return ast.WalkStop, err
	}
	label := "image"
	if alt != "" {
		label = "image: " + alt
	}
	chip := r.roles.ImageChip.Render(" " + label + " ")
	if dest := string(img.Destination); dest != "" {
		r.osc8ID++
		chip = osc8Wrap(dest, r.osc8ID, chip)
	}
	_, _ = w.WriteString(chip)
	return ast.WalkSkipChildren, nil
}
