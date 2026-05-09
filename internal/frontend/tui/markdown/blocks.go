// Block-level renderers: heading, paragraph, thematic break, lists
// (bullet + ordered + task). Blockquote / table / callout renderers
// land in later phases (see plan §Phasen-Rollout).

package markdown

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/util"

	"github.com/serverkraken/flow/internal/frontend/tui/markdown/theme"
)

// renderHeading styles ATX/Setext headings per level. H1 is a banner
// (full-width BG row above, BG-coloured row carrying the text, BG-only
// row below). H2 / H3 carry a leading thick bar (▍ / ▍▍). H4-H6 use
// dim arrow prefixes so the hierarchy stays monotonic.
//
// Always returns WalkSkipChildren — children are rendered inline into
// a sub-buffer first so the bar layout can size to the heading text.
func (r *nodeRenderer) renderHeading(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	h := n.(*ast.Heading)
	inner, err := r.renderInlineToString(source, n)
	if err != nil {
		return ast.WalkStop, err
	}
	out := r.styleHeading(h.Level, inner)
	_, _ = w.WriteString("\n")
	_, _ = w.WriteString(out)
	_, _ = w.WriteString("\n\n")
	return ast.WalkSkipChildren, nil
}

// styleHeading renders the level-appropriate visual treatment.
// Five distinct shapes — two independent visual signals (decoration
// + colour family) reinforcing each other:
//
//	H1  full-width Purple BG banner (1 row)             — section title
//	H2  Purple-on-BgChip chip + ─ underline             — subsection
//	H3  ▌▌ thick double bar + bold cyan                 — block
//	H4  ▌  single bar + non-bold blue                   — sub-block
//	H5  › chevron + dim                                 — fine-grained
//	H6  · mid-dot + italic muted                        — note
//
// Decoration weight steps down monotonically (banner → chip → ▌▌ →
// ▌ → › → ·) and the colour family shifts H3→H4 (cyan→blue) and
// H5→H6 (dim→muted+italic). The raw `#` markdown sigils are
// stripped from the rendered output — the glyph + colour pair
// already carries the level, and exposing the markdown source noise
// makes the rendered document feel like coloured-raw-markdown rather
// than a typeset document. Over-wide headings truncate with `…`
// rather than wrap (a banner-style heading on two lines reads broken).
func (r *nodeRenderer) styleHeading(level int, inner string) string {
	switch level {
	case 1:
		return r.styleH1(inner)
	case 2:
		return r.styleH2(inner)
	case 3:
		return r.roles.H3.Render("▌▌ " + inner)
	case 4:
		return r.roles.H4.Render("▌ " + inner)
	case 5:
		return r.roles.H5.Render("› " + inner)
	default:
		return r.roles.H6.Render("· " + inner)
	}
}

// styleH2 renders the heading as a chip — text on the BgHighlight
// pad — followed by a full-width `─` rule. The chip ends where the
// title ends; the rule provides the secondary signal across the
// full column. Distinct from H1's full-width banner and from H3's
// plain leading bar.
func (r *nodeRenderer) styleH2(text string) string {
	w := r.effectiveWidth()
	row := text
	if visibleWidth(row) > w {
		row = ansi.Truncate(row, w, "…")
	}
	chip := r.roles.H2.Render(row)
	// HRule ist die heading-rule-Rolle (auch genutzt für thematic breaks).
	// CardSeparator ist das frontmatter-card / backlinks-card Pendant —
	// dieselbe Hue, aber semantisch eine andere Rolle; das Heading sollte
	// nicht an Card-Style gekoppelt sein.
	rule := r.roles.HRule.Render(strings.Repeat("─", w))
	return chip + "\n" + rule
}

// styleH1 emits a single full-width row carrying the heading text
// over the H1 banner background. Width = effectiveWidth() so a H1
// inside a list item still fits the indent budget; over-wide titles
// get truncated with `…`.
func (r *nodeRenderer) styleH1(inner string) string {
	w := r.effectiveWidth()
	// Truncate `inner` selbst auf w-2, damit die umrahmenden Spaces
	// (" inner ") die volle effective-width nicht durch das ansi.Truncate
	// um zwei Zeichen auffressen — vorher kürzte `ansi.Truncate(text, w)`
	// den Inhalt um 1 Cell pro umrahmendem Space + 1 für das "…", also drei
	// Zellen statt der nötigen einen.
	if visibleWidth(inner) > w-2 {
		inner = ansi.Truncate(inner, w-2, "…")
	}
	text := " " + inner + " "
	return r.roles.H1Text.Width(w).Render(text)
}

// renderParagraph captures inline children, reflows to the
// effective-width budget (r.width minus parent indent), and emits
// the result followed by a blank line. Reflow happens here (not at
// the inline level) so wrap boundaries respect the paragraph's
// full text rather than per-fragment widths.
//
// Inside a blockquote, the BlockquoteText role (FgDim + italic) is
// the baseline style; emitting Paragraph's own Foreground here would
// override the dim cue and make quoted prose visually identical to
// the surrounding body. r.inQuote > 0 selects the quoted-body style
// for the wrap baseline so the leading │ bar and the dim italic
// agree.
func (r *nodeRenderer) renderParagraph(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	inner, err := r.renderInlineToString(source, n)
	if err != nil {
		return ast.WalkStop, err
	}
	wrapped := wrapText(inner, r.effectiveWidth())
	style := r.roles.Paragraph
	if r.inQuote > 0 {
		style = r.roles.BlockquoteText
	}
	_, _ = w.WriteString(style.Render(wrapped))
	_, _ = w.WriteString("\n\n")
	return ast.WalkSkipChildren, nil
}

// calloutPattern matches a GFM callout marker: an opening line of
// the form `[!KIND]` (case-insensitive), optionally followed by
// title text on the same line. Captures the kind in group 1.
var calloutPattern = regexp.MustCompile(`(?i)^\s*\[!([A-Z]+)\]\s*(.*)$`)

// renderBlockquote styles a blockquote and detects the GFM callout
// shape. Plain quotes get a muted │ leader; callouts get a coloured
// badge + matching bar so the kind reads at a glance. Always
// returns WalkSkipChildren — children are captured into a sub-buffer
// so the bar can be applied per-line on the rendered output.
func (r *nodeRenderer) renderBlockquote(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	// Both callout and plain-quote variants reserve 2 cells for the
	// leading bar/badge column, so children must wrap to width-2 to
	// leave room for the prefix without overflowing.
	r.indent += 2
	defer func() { r.indent -= 2 }()

	if kind, title, bodyNodes, ok := detectCalloutAST(source, n); ok {
		body, err := r.renderNodeListToString(source, bodyNodes, n)
		if err != nil {
			return ast.WalkStop, err
		}
		_, _ = w.WriteString("\n")
		_, _ = w.WriteString(r.styleCallout(kind, title, strings.TrimRight(body, "\n")))
		_, _ = w.WriteString("\n\n")
		return ast.WalkSkipChildren, nil
	}

	r.inQuote++
	body, err := r.renderChildrenToString(source, n)
	r.inQuote--
	if err != nil {
		return ast.WalkStop, err
	}
	body = strings.TrimRight(body, "\n")
	bar := r.roles.BlockquoteBar.Render("│ ")
	out := prefixFirstLine(body, bar, bar)
	_, _ = w.WriteString("\n")
	_, _ = w.WriteString(out)
	_, _ = w.WriteString("\n\n")
	return ast.WalkSkipChildren, nil
}

// detectCalloutAST inspects the blockquote's first child paragraph
// for a `[!KIND]` marker at the start of its first source line.
// Reading from para.Lines() instead of the first Text node sidesteps
// goldmark's CommonMark link parser, which splits `[...]` into
// several Text segments and would defeat a per-segment regex match.
//
// On hit, returns the kind + title (rest-of-line) + the blockquote's
// remaining block children as the body. bodyNodes may be empty for
// marker-only callouts. ok=false signals a plain blockquote.
func detectCalloutAST(source []byte, bq ast.Node) (kind theme.CalloutKind, title string, bodyNodes []ast.Node, ok bool) {
	first := bq.FirstChild()
	if first == nil {
		return "", "", nil, false
	}
	para, isPara := first.(*ast.Paragraph)
	if !isPara {
		return "", "", nil, false
	}
	lines := para.Lines()
	if lines.Len() == 0 {
		return "", "", nil, false
	}
	seg := lines.At(0)
	firstLine := string(seg.Value(source))
	m := calloutPattern.FindStringSubmatch(strings.TrimRight(firstLine, "\n"))
	if m == nil {
		return "", "", nil, false
	}
	k := theme.CalloutKind(strings.ToLower(m[1]))
	if !knownCallout(k) {
		return "", "", nil, false
	}
	title = strings.TrimSpace(m[2])
	for c := para.NextSibling(); c != nil; c = c.NextSibling() {
		bodyNodes = append(bodyNodes, c)
	}
	return k, title, bodyNodes, true
}

// renderNodeListToString runs ast.Walk over each node through our
// dispatch and returns the concatenated ANSI output. Used by
// renderBlockquote to render the body of a callout (a slice of
// blockquote children minus the marker paragraph) into one string.
func (r *nodeRenderer) renderNodeListToString(source []byte, nodes []ast.Node, _ ast.Node) (string, error) {
	if len(nodes) == 0 {
		return "", nil
	}
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	for _, n := range nodes {
		if err := ast.Walk(n, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
			return r.dispatch(bw, source, node, entering)
		}); err != nil {
			return "", err
		}
	}
	if err := bw.Flush(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// knownCallout returns true for the seven recognised callout kinds.
// Unknown markers (typos, custom kinds) fall back to plain
// blockquote styling so the user sees what they wrote without an
// invented colour.
func knownCallout(k theme.CalloutKind) bool {
	switch k {
	case theme.CalloutNote, theme.CalloutTip, theme.CalloutInfo,
		theme.CalloutWarning, theme.CalloutDanger, theme.CalloutImportant,
		theme.CalloutSuccess:
		return true
	}
	return false
}

// styleCallout builds the rendered callout: header row carrying a
// coloured badge + optional title, then the body indented under a
// matching coloured bar. body may be empty when the callout has no
// content beyond the marker line.
func (r *nodeRenderer) styleCallout(kind theme.CalloutKind, title, body string) string {
	badge := theme.CalloutBadge(kind, r.palette).Render(strings.ToUpper(string(kind)))
	bar := theme.CalloutBar(kind, r.palette).Render("│ ")
	header := badge
	if title != "" {
		header += " " + r.roles.CardTitle.Render(title)
	}
	if body == "" {
		return bar + header
	}
	body = strings.TrimRight(body, "\n")
	indented := prefixFirstLine(body, bar, bar)
	return bar + header + "\n" + indented
}

// renderThematicBreak emits a full-width muted ─ rule. Width caps at
// effectiveWidth so the rule never spills into chrome or out of the
// parent block's indent.
func (r *nodeRenderer) renderThematicBreak(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	width := r.effectiveWidth()
	_, _ = fmt.Fprintln(w)
	_, _ = w.WriteString(r.roles.HRule.Render(strings.Repeat("─", width)))
	_, _ = w.WriteString("\n\n")
	return ast.WalkSkipChildren, nil
}

// renderList walks list children. The per-item rendering happens in
// renderListItem; this handler exists mainly to be the dispatch
// target so children get visited (without registering it goldmark
// would skip the subtree). A trailing blank line is emitted on
// exit so a list followed by prose has breathing room.
func (r *nodeRenderer) renderList(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		return ast.WalkContinue, nil
	}
	_, _ = w.WriteString("\n")
	return ast.WalkContinue, nil
}

// renderListItem captures the item's body, prefixes the bullet (or
// numbered marker, or task checkbox) on the first line, and pads the
// continuation lines so wrapped text + nested-block content sits
// under the bullet's content column.
//
// Per-level indent is NOT added here — when this item lives inside
// another item, the outer renderListItem's restPrefix provides the
// indent for every line of this nested output. Adding indent here
// too would double up. Depth is still computed because it picks the
// bullet glyph + colour.
//
// Tight vs loose: CommonMark distinguishes by whether items contain
// blank-line-separated paragraphs. Both cases work here because
// renderChildrenToString preserves the goldmark output verbatim;
// only the trailing blank-line trim normalises spacing.
func (r *nodeRenderer) renderListItem(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	parent, ok := n.Parent().(*ast.List)
	if !ok {
		return ast.WalkContinue, nil
	}
	depth := listAncestorDepth(parent)
	marker, taskDone, isTask := r.itemMarker(parent, n, source, depth)

	// Push the marker width onto the indent so the item's children
	// (paragraphs, code blocks, nested lists) wrap to width minus
	// the column we'll prepend in prefixFirstLine. Without this the
	// nested content overflows by `len(marker)` cells once the
	// prefix is applied.
	markerW := lipglossWidth(marker)
	r.indent += markerW
	body, err := r.renderChildrenToString(source, n)
	parentIndent := r.indent - markerW
	r.indent -= markerW
	if err != nil {
		return ast.WalkStop, err
	}
	body = strings.TrimRight(body, "\n")

	// Tight list items render via TextBlock (no Paragraph wrap),
	// so explicit wrap is needed here. Paragraph-rendered loose
	// items already fit width-marker so this is a no-op for them.
	//
	// Wrap budget = full width − every parent ListItem's marker
	// (parentIndent) − this item's own marker. The previous
	// `r.width-markerW` only accounted for the immediate marker, so
	// at depth ≥ 2 the wrapped continuation lines sat past the
	// content column and the parent's prefixFirstLine pushed them
	// further right than the bullet content.
	budget := r.width - parentIndent - markerW
	if budget < 1 {
		budget = 1
	}
	body = wrapText(body, budget)

	if isTask && taskDone {
		body = applyDoneStyle(stripANSI(body), r.roles.TaskDoneText)
	}

	out := prefixFirstLine(body, marker, strings.Repeat(" ", markerW))
	_, _ = w.WriteString(out)
	_, _ = w.WriteString("\n")
	return ast.WalkSkipChildren, nil
}

// itemMarker picks the per-item leader: per-depth bullet for
// unordered lists, "N. " for ordered lists, ☐/☑ for task items.
// Returns the rendered (styled) marker, plus the task-done flag so
// renderListItem can dim+strike the body text for completed tasks.
func (r *nodeRenderer) itemMarker(list *ast.List, item ast.Node, _ []byte, depth int) (marker string, taskDone bool, isTask bool) {
	if box := findTaskCheckbox(item); box != nil {
		isTask = true
		taskDone = box.IsChecked
		if box.IsChecked {
			marker = r.roles.TaskDone.Render("☑") + " "
		} else {
			marker = r.roles.TaskOpen.Render("☐") + " "
		}
		return
	}
	if list.IsOrdered() {
		idx := orderedItemIndex(list, item)
		num := list.Start + idx
		marker = r.roles.NumberMarker.Render(fmt.Sprintf("%d.", num)) + " "
		return
	}
	glyph := bulletGlyph(depth)
	style := r.bulletStyle(depth)
	marker = style.Render(glyph) + " "
	return
}

// bulletGlyph picks the bullet glyph for a depth (1-indexed).
// Wraps past 4 because deeply nested lists are rare and the L4+
// style stays the same — picking a different glyph at depth 5 would
// add noise without conveying hierarchy.
func bulletGlyph(depth int) string {
	switch depth {
	case 1:
		return "●"
	case 2:
		return "○"
	case 3:
		return "◆"
	default:
		return "▪"
	}
}

// bulletStyle picks the lipgloss style for a bullet at given depth.
func (r *nodeRenderer) bulletStyle(depth int) lipglossStyleLike {
	switch depth {
	case 1:
		return r.roles.Bullet1
	case 2:
		return r.roles.Bullet2
	case 3:
		return r.roles.Bullet3
	default:
		return r.roles.Bullet4
	}
}

// listAncestorDepth returns the 1-indexed depth of a List node by
// counting List ancestors. The outermost list is depth 1, a list
// nested directly inside an item of that list is depth 2, etc.
func listAncestorDepth(list ast.Node) int {
	d := 1
	for p := list.Parent(); p != nil; p = p.Parent() {
		if p.Kind() == ast.KindList {
			d++
		}
	}
	return d
}

// orderedItemIndex returns the 0-indexed position of item among its
// sibling ListItems. goldmark doesn't expose this directly so we
// walk the parent's children — cheap because list sizes are small.
func orderedItemIndex(list *ast.List, item ast.Node) int {
	idx := 0
	for c := list.FirstChild(); c != nil; c = c.NextSibling() {
		if c == item {
			return idx
		}
		idx++
	}
	return 0
}

// findTaskCheckbox returns the GFM TaskCheckBox node a task-list
// item carries as its first inline child, or nil for non-task
// items. The checkbox sits inside the item's first paragraph /
// text-block child.
func findTaskCheckbox(item ast.Node) *extast.TaskCheckBox {
	first := item.FirstChild()
	if first == nil {
		return nil
	}
	if box, ok := first.FirstChild().(*extast.TaskCheckBox); ok {
		return box
	}
	return nil
}

// prefixFirstLine prepends `firstPrefix` to the first line of body
// and `restPrefix` to every subsequent line. Used to put a bullet
// (or number, or checkbox) on the first row of an item and indent
// the wrapped continuation under it.
func prefixFirstLine(body, firstPrefix, restPrefix string) string {
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		if i == 0 {
			lines[i] = firstPrefix + l
		} else {
			lines[i] = restPrefix + l
		}
	}
	return strings.Join(lines, "\n")
}

// lipglossStyleLike is the subset of lipgloss.Style we use for
// bullets — kept abstract so bulletStyle's switch can return the
// shared interface without importing lipgloss into every call site.
type lipglossStyleLike interface {
	Render(...string) string
}

// lipglossWidth wraps lipgloss.Width so the helper can be swapped
// for tests that inject a different measurement (none today).
func lipglossWidth(s string) int {
	return visibleWidth(s)
}

// applyDoneStyle wraps text in the open-SGR of style + strikethrough
// (SGR 9), and closes with a single reset. Going through lipgloss's
// Render for a Strikethrough+Foreground style emits one SGR run **per
// grapheme** (defensive against terminals that reset strikethrough on
// space), which inflates done-task lines by ~30× the byte count
// without visual benefit. The manual wrap keeps the line down to one
// open + one close.
//
// Input must be plain text (no SGR sequences inside) — the caller
// strips ANSI before invoking.
func applyDoneStyle(plain string, style lipglossStyleLike) string {
	open := openingSGR(toLipglossStyle(style))
	if open == "" {
		// NO_COLOR / Ascii profile — no SGR baseline; just pass
		// through. The leading checkbox glyph already conveys "done"
		// in the plain layout.
		return plain
	}
	return open + plain + "\x1b[0m"
}

// toLipglossStyle narrows the lipglossStyleLike interface back to the
// concrete lipgloss.Style openingSGR expects. The interface exists so
// bulletStyle's switch can return Style values without dragging
// lipgloss into the call site, but applyDoneStyle does need the
// concrete type to extract the open sequence.
func toLipglossStyle(s lipglossStyleLike) lipgloss.Style {
	if ls, ok := s.(lipgloss.Style); ok {
		return ls
	}
	return lipgloss.Style{}
}
