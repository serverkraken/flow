package theme

// Markdown role mapping. The renderer in internal/frontend/tui/markdown
// looks up its block / inline styles here; the source of truth for the
// colours involved is the canonical Palette, passed through as a
// parameter so a NO_COLOR test or a per-screen palette override stays
// parallel-safe.

import (
	"charm.land/lipgloss/v2"
	canonical "github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// MarkdownRoles bundles every pre-built lipgloss style the renderer
// needs. Built once per Render call (the constructor takes a
// lipgloss.Renderer so NoColor / TrueColor profiles can be picked
// per call without polluting global state).
type MarkdownRoles struct {
	// Block-level
	H1Bar     lipgloss.Style
	H1Text    lipgloss.Style
	H2        lipgloss.Style
	H3        lipgloss.Style
	H4        lipgloss.Style
	H5        lipgloss.Style
	H6        lipgloss.Style
	Paragraph lipgloss.Style
	HRule     lipgloss.Style

	// Inline
	Strong   lipgloss.Style
	Emph     lipgloss.Style
	Strike   lipgloss.Style
	CodeSpan lipgloss.Style
	LinkText lipgloss.Style

	// Code-block panel
	CodeFenceBg    lipgloss.Style // base BG-only style applied to every line of a fenced/indented block
	CodeFenceBand  lipgloss.Style // top/bottom band carrying the language label
	CodeFenceLabel lipgloss.Style // language label text inside the band
	CodeFencePlain lipgloss.Style // unstyled fallback when chroma has no lexer for a fence

	// Lists
	Bullet1      lipgloss.Style // L1 bullet glyph (●)
	Bullet2      lipgloss.Style // L2 bullet glyph (○)
	Bullet3      lipgloss.Style // L3 bullet glyph (◆)
	Bullet4      lipgloss.Style // L4+ bullet glyph (▪)
	NumberMarker lipgloss.Style // ordered-list "1." marker
	TaskOpen     lipgloss.Style // ☐
	TaskDone     lipgloss.Style // ☑
	TaskDoneText lipgloss.Style // strike-through prose for completed tasks

	// Tables
	TableBorder lipgloss.Style // ┌ ─ ┐ │ etc. — box-drawing
	TableHeader lipgloss.Style // header cell text (bold + colored)
	TableCell   lipgloss.Style // body cell text
	TableRowAlt lipgloss.Style // alternating row tint background

	// Wikilinks
	WikilinkValid  lipgloss.Style // resolved wikilink (cyan, underlined, OSC 8 wrapped)
	WikilinkBroken lipgloss.Style // unresolved wikilink (red + strike, no link)
	ImageChip      lipgloss.Style // textual image placeholder until P1.13

	// Frontmatter card
	CardBadgeDaily   lipgloss.Style // [DAILY] type badge
	CardBadgeProject lipgloss.Style // [PROJECT] type badge
	CardBadgeFree    lipgloss.Style // [FREE] type badge
	CardTitle        lipgloss.Style // big bold note title
	CardMeta         lipgloss.Style // dim metadata (date, id, etc.)
	CardProjectChip  lipgloss.Style // project URL chip
	CardSeparator    lipgloss.Style // ─ rule below the card
	// TagChips is the per-palette pre-built slice of styles for the
	// frontmatter-card tag chips. The renderer indexes into this via a
	// stable hash of the tag string so `#go` keeps the same colour
	// across notes. Before this slot was lifted out, frontmatter.tagChip
	// constructed an inline `lipgloss.NewStyle()` that bypassed the
	// per-Renderer NO_COLOR / Ascii profile — A11y-4 regression.
	TagChips []lipgloss.Style

	// Block quotes + GitHub-style callouts
	BlockquoteBar  lipgloss.Style // leading │ bar in front of quoted lines
	BlockquoteText lipgloss.Style // body text style for quoted prose

	// Footnotes
	FootnoteRef       lipgloss.Style // superscript marker inline
	FootnoteListTitle lipgloss.Style // "Footnotes" heading at end of document
	FootnoteDef       lipgloss.Style // definition body text
}

// CalloutKind enumerates the GitHub-style callout types.
type CalloutKind string

// Recognised callout kinds. Lowercase to match what users typically
// type after the bang (`> [!note]`, `> [!warning]`, …).
const (
	CalloutNote      CalloutKind = "note"
	CalloutTip       CalloutKind = "tip"
	CalloutInfo      CalloutKind = "info"
	CalloutWarning   CalloutKind = "warning"
	CalloutDanger    CalloutKind = "danger"
	CalloutImportant CalloutKind = "important"
	CalloutSuccess   CalloutKind = "success"
)

// CalloutBadge returns the styled badge chip ("NOTE", "WARNING", …)
// for a callout kind, painted onto the given palette.
func CalloutBadge(kind CalloutKind, p canonical.Palette) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(p.Bg).
		Bold(true).
		Padding(0, 1).
		Background(calloutColor(kind, p))
}

// CalloutBar returns the leading │ bar style matched to a callout
// kind so the bar colour reinforces the badge.
func CalloutBar(kind CalloutKind, p canonical.Palette) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(calloutColor(kind, p))
}

// calloutColor maps a callout kind to its semantic color (Skill §Color
// semantics: consume p.Sem(), nicht die rohe Hue). note/tip/info teilen
// sich Sem().Info — alle drei sind „informativ ohne Aktion", die Badge-
// Beschriftung (NOTE/TIP/INFO) trägt die Differenzierung statt der Farbe;
// info wechselt damit von Blau auf das Info-Cyan. important = Highlight
// (Identität), Rest auf die passende Sem-Rolle.
func calloutColor(kind CalloutKind, p canonical.Palette) canonical.Color {
	sem := p.Sem()
	switch kind {
	case CalloutTip, CalloutNote, CalloutInfo:
		return sem.Info
	case CalloutWarning:
		return sem.Warning
	case CalloutDanger:
		return sem.Danger
	case CalloutImportant:
		return sem.Highlight
	case CalloutSuccess:
		return sem.Success
	}
	return p.FgMuted
}

// MarkdownRolesFor returns a MarkdownRoles built from a canonical
// palette. Under lipgloss v2 styles are deterministic and emit
// TrueColor unconditionally; NO_COLOR is handled by post-processing
// the final rendered string with ansi.Strip rather than by swapping
// a renderer profile. The palette argument stays parametric so
// concurrent renders with different palettes — including the
// contrast test — never race on a global.
func MarkdownRolesFor(p canonical.Palette) MarkdownRoles {
	color := lipgloss.NewStyle
	return MarkdownRoles{
		H1Bar: color().
			Background(p.BgBar).
			Foreground(p.Purple).
			Bold(true),
		H1Text: color().
			Background(p.BgBar).
			Foreground(p.Purple).
			Bold(true),
		// H2 wears a subtle BG chip behind the text (BgChip) and a
		// Purple bold foreground so it visually shares the H1 family
		// without claiming the full-width banner. Padding keeps the
		// chip from sitting flush against the prose around it.
		H2: color().
			Foreground(p.Purple).
			Background(p.BgChip).
			Bold(true).
			Padding(0, 1),
		// H3 wears bold + cyan + double-bar — distinct family head
		// (purple H1+H2 → cyan H3+H4) with the loudest bar treatment.
		H3: color().
			Foreground(p.Cyan).
			Bold(true),
		// H4 drops the bold and shifts to Blue so the cyan/blue pair
		// reads as "same family, lower rung". Without this delta H3
		// and H4 collapsed onto each other (both bold cyan) — only the
		// `▌▌` vs `▌` glyph distinguished them, which forced the eye
		// to count.
		H4: color().
			Foreground(p.Blue),
		H5: color().
			Foreground(p.FgDim),
		// A11y-3 (audit §2.5): no Faint() on prose. Faint dims a
		// terminal foreground 30–50%, which on top of an already-Muted
		// FgMuted drops the pair below WCAG AA. Italic alone carries
		// the "lowest level heading" semantics — but italic in
		// monospace fonts is unreliable, so the prefix glyph (·)
		// carries the level even if the font has no italic face.
		H6: color().
			Foreground(p.FgMuted).
			Italic(true),
		Paragraph: color().
			Foreground(p.Fg),
		// HR is structural punctuation, not content — it should
		// recede, not announce. FgMuted made the rule almost as
		// bright as body text. BgChip is the same colour the
		// frontmatter / footnote separators already use, so all
		// horizontal-rule-like elements share one visual language.
		HRule: color().
			Foreground(p.BgChip),

		Strong: color().Bold(true),
		Emph:   color().Italic(true),
		Strike: color().Strikethrough(true),
		// Inline code reads on a tinted chip; the foreground inherits
		// the surrounding prose colour so inline `code` stays in the
		// same visual register as fenced ``` ``` ``` blocks (which
		// run plain text on the panel BG plus chroma highlighting).
		// The previous green foreground made inline code feel like a
		// different language than the same identifier in a block.
		CodeSpan: color().
			Background(p.BgCode),
		LinkText: color().
			Foreground(p.Blue),

		CodeFenceBg: color().
			Background(p.BgCode).
			Foreground(p.FgDim),
		// Codeband uses BgChipSoft (a touch lighter than BgCode) so
		// the band reads as a frame edge rather than a continuation
		// of the H1 / H2 banners (those use BgBar + BgChip). Without
		// this delta a code block at the top of a section visually
		// merged into the H1 banner above it.
		CodeFenceBand: color().
			Background(p.BgChipSoft).
			Foreground(p.FgMuted),
		CodeFenceLabel: color().
			Background(p.BgChipSoft).
			Foreground(p.Cyan).
			Bold(true),
		CodeFencePlain: color().
			Background(p.BgCode).
			Foreground(p.FgDim),

		Bullet1:      color().Foreground(p.Blue).Bold(true),
		Bullet2:      color().Foreground(p.Cyan),
		Bullet3:      color().Foreground(p.Purple),
		Bullet4:      color().Foreground(p.FgMuted),
		NumberMarker: color().Foreground(p.FgMuted).Bold(true),
		TaskOpen:     color().Foreground(p.Yellow).Bold(true),
		TaskDone:     color().Foreground(p.Green).Bold(true),
		TaskDoneText: color().Foreground(p.FgMuted).Strikethrough(true),

		TableBorder: color().Foreground(p.FgMuted),
		TableHeader: color().Foreground(p.Cyan).Bold(true),
		TableCell:   color().Foreground(p.Fg),
		TableRowAlt: color().Background(p.BgChipSoft).Foreground(p.Fg),

		WikilinkValid: color().Foreground(p.Cyan),
		// A11y-3 (audit §2.5): kein Faint() auf Prose. Faint reduziert
		// die terminal-Foreground-Helligkeit um 30–50 %; auf einem schon
		// schwachen Red-on-BgBar fällt das unter WCAG AA. Die Brokenness
		// signalisiert weiterhin der `⊘`-Glyph (in backlinkLine +
		// renderWikiLink), unterscheidbar von `→` bei WikilinkValid auch
		// im NO_COLOR-Profil. Kein zusätzlicher SGR-Modifier — Lipgloss
		// segmentiert Strikethrough per-Zelle und reißt den Span in
		// individuelle Zeichen-SGR-Sequenzen auseinander.
		WikilinkBroken: color().Foreground(p.Red),
		ImageChip:      color().Foreground(p.FgMuted).Background(p.BgChipSoft),

		CardBadgeDaily:   color().Foreground(p.Bg).Background(p.Yellow).Bold(true).Padding(0, 1),
		CardBadgeProject: color().Foreground(p.Bg).Background(p.Purple).Bold(true).Padding(0, 1),
		CardBadgeFree:    color().Foreground(p.Bg).Background(p.Cyan).Bold(true).Padding(0, 1),
		CardTitle:        color().Foreground(p.Fg).Bold(true),
		CardMeta:         color().Foreground(p.FgMuted).Italic(true),
		CardProjectChip:  color().Foreground(p.Green).Background(p.BgChipSoft),
		CardSeparator:    color().Foreground(p.BgChip),
		BlockquoteBar:    color().Foreground(p.FgMuted),
		BlockquoteText:   color().Foreground(p.FgDim).Italic(true),

		FootnoteRef:       color().Foreground(p.Purple).Bold(true),
		FootnoteListTitle: color().Foreground(p.Cyan).Bold(true),
		FootnoteDef:       color().Foreground(p.FgDim),

		TagChips: tagChipStyles(color, p),
	}
}

// tagChipStyles baut die per-Tag-Slot-Styles aus der Palette-TagPalette.
// Die Styles laufen durch denselben lipgloss.Renderer wie alle anderen
// Roles, daher reicht der Ascii-Profile-Pfad (NO_COLOR) durch und der
// Test TestMarkdownRolesFor_NoColorPath deckt die Tag-Chips
// genauso ab wie alle anderen Slots.
func tagChipStyles(color func() lipgloss.Style, p canonical.Palette) []lipgloss.Style {
	out := make([]lipgloss.Style, len(p.TagPalette))
	for i, c := range p.TagPalette {
		out[i] = color().
			Foreground(p.Bg).
			Background(c).
			Bold(true).
			Padding(0, 1)
	}
	return out
}
