// Fenced + indented code-block renderer. Tokenises the source via
// chroma (~250 language lexers), styles each token with a lipgloss
// style anchored in the active palette, then frames the result as a
// panel: top band carrying the language label, BG-filled content rows,
// bottom band. Replaces the post-process panel logic from the previous
// pipeline with first-party rendering.

package markdown

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/util"

	canonical "github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// codeBlockRightPad is the breathing room appended past the longest
// line of a code block. Without it the right edge sits flush against
// the last code char and reads as an abrupt cut.
const codeBlockRightPad = 4

// renderFencedCodeBlock renders a ` ```lang ` block as a panel.
// The language is taken from the fence info string; chroma picks
// the lexer (~250 languages, falls back to plain on unknown).
func (r *nodeRenderer) renderFencedCodeBlock(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	fenced := n.(*ast.FencedCodeBlock)
	lang := strings.ToLower(string(fenced.Language(source)))
	src := collectLeafLines(n, source)
	panel := r.renderCodePanel(src, lang)
	_, _ = w.WriteString("\n" + panel + "\n\n")
	return ast.WalkSkipChildren, nil
}

// renderIndentedCodeBlock renders a 4-space-indented block as the
// same panel, without a language label (Markdown indented blocks
// have no info string).
func (r *nodeRenderer) renderIndentedCodeBlock(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	src := collectLeafLines(n, source)
	panel := r.renderCodePanel(src, "")
	_, _ = w.WriteString("\n" + panel + "\n\n")
	return ast.WalkSkipChildren, nil
}

// collectLeafLines concatenates the source bytes covered by every
// line segment of a leaf-block node and trims the trailing newline so
// the panel doesn't gain an empty bottom row.
func collectLeafLines(n ast.Node, source []byte) string {
	var b strings.Builder
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write(seg.Value(source))
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderCodePanel highlights src for lang and frames it as a panel
// sized to the longest content line + codeBlockRightPad, capped at
// effectiveWidth. Lines wider than the budget are HARD-wrapped (the
// previous behaviour truncated with `…`, which silently dropped
// long identifiers / strings off the right edge — reported in
// review as „Text wrappt nicht in Code-Blöcken").
//
// Empty src yields a 1-row panel so the band wrap still reads as
// a code block.
func (r *nodeRenderer) renderCodePanel(src, lang string) string {
	styledLines := r.highlightLines(src, lang)
	if len(styledLines) == 0 {
		styledLines = []string{r.roles.CodeFencePlain.Render("")}
	}
	// Wrap any over-wide line at the panel's content budget
	// (effectiveWidth − 1 lead space − 1 trailing margin so a wrapped
	// line still has room for the right BG-fill). `cellbuf.Wrap`
	// preserves SGR + link state across the wrap boundary, so chroma
	// token colours (string orange, keyword cyan, …) survive when a
	// long token gets broken across rows. `ansi.Hardwrap` did not —
	// it left the second half of a wrapped chroma token unstyled.
	avail := r.effectiveWidth() - 2
	if avail < 1 {
		avail = 1
	}
	var wrapped []string
	for _, line := range styledLines {
		if lipgloss.Width(line) <= avail {
			wrapped = append(wrapped, line)
			continue
		}
		wrapped = append(wrapped, strings.Split(cellbuf.Wrap(line, avail, ""), "\n")...)
	}
	styledLines = wrapped

	maxW := 0
	for _, line := range styledLines {
		if w := lipgloss.Width(line); w > maxW {
			maxW = w
		}
	}
	target := maxW + codeBlockRightPad
	if budget := r.effectiveWidth(); target > budget {
		target = budget
	}
	if target < 1 {
		target = 1
	}
	// Layout: top band (with optional language label) → blank BG-only
	// row in the code background → content rows → blank BG-only row →
	// bottom band. The two BG-only rows give the code visual breathing
	// room so the first/last code line doesn't slam into the band.
	emptyContent := r.codeRow("", target)
	out := make([]string, 0, len(styledLines)+4)
	out = append(out, r.codeBand(target, lang, true))
	out = append(out, emptyContent)
	for _, line := range styledLines {
		out = append(out, r.codeRow(line, target))
	}
	out = append(out, emptyContent)
	out = append(out, r.codeBand(target, "", false))
	return strings.Join(out, "\n")
}

// highlightLines tokenises src with the chroma lexer for lang and
// returns one ANSI-styled string per source line. Falls back to the
// plain CodeFence style when chroma has no lexer or fails to tokenise.
func (r *nodeRenderer) highlightLines(src, lang string) []string {
	lex := lexers.Get(lang)
	if lex == nil {
		return r.plainCodeLines(src)
	}
	iter, err := chroma.Coalesce(lex).Tokenise(nil, src+"\n")
	if err != nil {
		return r.plainCodeLines(src)
	}
	var (
		lines []string
		cur   strings.Builder
	)
	for token := iter(); token != chroma.EOF; token = iter() {
		style := r.chromaStyle(token.Type)
		parts := strings.Split(token.Value, "\n")
		for i, p := range parts {
			if i > 0 {
				lines = append(lines, cur.String())
				cur.Reset()
			}
			if p != "" {
				cur.WriteString(style.Render(p))
			}
		}
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	for len(lines) > 0 && strings.TrimSpace(stripANSI(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// plainCodeLines styles every src line with the plain CodeFencePlain
// role. Used when chroma has no lexer for the requested language.
func (r *nodeRenderer) plainCodeLines(src string) []string {
	raw := strings.Split(src, "\n")
	out := make([]string, len(raw))
	for i, l := range raw {
		out[i] = r.roles.CodeFencePlain.Render(l)
	}
	return out
}

// codeRow frames one styled line: a leading BG-only space (so the
// first column never reads as a tinted hole between SGR resets), the
// (possibly truncated) content, and a trailing BG-only filler that
// pads the visible width to target. Wrapping happens upstream via
// cellbuf.Wrap (see renderCodePanel) which preserves SGR/link state
// across breaks; codeRow only truncates as a last-ditch defence when
// even the wrapped line still exceeds the budget — e.g. a single
// unbreakable token longer than the panel.
func (r *nodeRenderer) codeRow(line string, target int) string {
	leadSpace := r.roles.CodeFenceBg.Render(" ")
	leadW := lipgloss.Width(leadSpace)
	contentBudget := target - leadW
	if contentBudget < 1 {
		return r.roles.CodeFenceBg.Render(strings.Repeat(" ", target))
	}
	if lipgloss.Width(line) > contentBudget {
		line = ansi.Truncate(line, contentBudget, "…")
	}
	pad := target - leadW - lipgloss.Width(line)
	if pad < 0 {
		pad = 0
	}
	return leadSpace + line + r.roles.CodeFenceBg.Render(strings.Repeat(" ", pad))
}

// codeBand emits a band row. The top variant carries the language
// label LEFT-aligned (so the eye lands on it before the code body —
// matches GitHub / Obsidian / VSCode conventions). The bottom is
// BG-only (no label). Empty lang yields a labelless band on the top
// too.
func (r *nodeRenderer) codeBand(target int, lang string, top bool) string {
	if !top || lang == "" {
		return r.roles.CodeFenceBand.Render(strings.Repeat(" ", target))
	}
	label := " " + lang + " "
	labelW := lipgloss.Width(label)
	if labelW > target {
		label = label[:target]
		labelW = target
	}
	fill := target - labelW
	return r.roles.CodeFenceLabel.Render(label) + r.roles.CodeFenceBand.Render(strings.Repeat(" ", fill))
}

// chromaStyle returns the lipgloss style for a chroma token type.
// Every style carries the panel BG so the row stays uniformly tinted
// regardless of which token sits in a given cell. Token-type → colour
// mapping mirrors the upstream tokyo-night chroma style; the active
// palette is the one this nodeRenderer was constructed against, so a
// per-call WithPalette override flows down naturally.
func (r *nodeRenderer) chromaStyle(t chroma.TokenType) lipgloss.Style {
	base := r.roles.CodeFenceBg
	fg := chromaTokenColor(t, r.palette)
	if fg == "" {
		return base
	}
	return base.Foreground(fg)
}

// chromaTokenColor maps a chroma TokenType to a palette colour. Returns
// the zero lipgloss.Color ("") for tokens that should inherit the
// panel's default text colour (most whitespace + unmapped categories).
//
// The cases are ordered most-specific-first so InCategory checks at
// the bottom don't shadow the precise types above.
func chromaTokenColor(t chroma.TokenType, p canonical.Palette) canonical.Color {
	switch t {
	case chroma.KeywordType:
		return p.Blue
	case chroma.NameFunction, chroma.NameDecorator, chroma.NameAttribute:
		return p.Green
	case chroma.NameConstant:
		return p.Purple
	case chroma.NameBuiltin, chroma.NameBuiltinPseudo, chroma.NameClass, chroma.NameVariable:
		return p.Blue
	case chroma.NameTag, chroma.NameNamespace:
		return p.Cyan
	}
	switch {
	case t.InCategory(chroma.Comment):
		return p.FgMuted
	case t.InCategory(chroma.Keyword):
		return p.Cyan
	case t.InSubCategory(chroma.LiteralString) || t == chroma.LiteralString:
		return p.Yellow
	case t.InSubCategory(chroma.LiteralNumber) || t == chroma.LiteralNumber:
		return p.Orange
	case t.InCategory(chroma.Operator):
		return p.Cyan
	case t.InCategory(chroma.Punctuation):
		return p.FgDim
	case t.InCategory(chroma.Generic):
		return p.FgDim
	}
	return ""
}

// stripANSI is a tiny re-export of x/ansi.Strip kept local so code.go
// doesn't need its own import block when only used by the trim loop
// in highlightLines.
func stripANSI(s string) string {
	// chroma doesn't emit OSC sequences in code spans — only SGR. A
	// strings.IndexByte fast-path would be a micro-optimisation; the
	// trim loop runs once per panel, so call into the shared helper.
	return visibleString(s)
}

// visibleString drops every SGR sequence. Lives here (not in width.go)
// because only the highlight pipeline needs it; keeping it local
// avoids polluting the width helper API for unrelated callers.
func visibleString(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == 0x1b && s[i+1] == '[' {
			end := i + 2
			for end < len(s) && s[end] != 'm' {
				end++
			}
			if end < len(s) {
				i = end + 1
				continue
			}
			break
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
