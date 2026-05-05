package theme

// Markdown role mapping. The renderer in internal/frontend/tui/markdown
// looks up its block / inline styles here; the source of truth for the
// colours involved is the canonical Palette, passed through as a
// parameter so a NO_COLOR test or a per-screen palette override stays
// parallel-safe.

import (
	"github.com/charmbracelet/lipgloss"
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
	WikilinkBroken lipgloss.Style // unresolved wikilink (red marker, no link)
	ImageChip      lipgloss.Style // textual image placeholder until P1.13

	// Frontmatter card
	CardBadgeDaily   lipgloss.Style // [DAILY] type badge
	CardBadgeProject lipgloss.Style // [PROJECT] type badge
	CardBadgeFree    lipgloss.Style // [FREE] type badge
	CardTitle        lipgloss.Style // big bold note title
	CardMeta         lipgloss.Style // dim metadata (date, id, etc.)
	CardProjectChip  lipgloss.Style // project URL chip
	CardSeparator    lipgloss.Style // ─ rule below the card

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
	color := lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.Bg)).
		Bold(true).
		Padding(0, 1)
	switch kind {
	case CalloutTip, CalloutNote:
		return color.Background(lipgloss.Color(p.Cyan))
	case CalloutInfo:
		return color.Background(lipgloss.Color(p.Blue))
	case CalloutWarning:
		return color.Background(lipgloss.Color(p.Yellow))
	case CalloutDanger:
		return color.Background(lipgloss.Color(p.Red))
	case CalloutImportant:
		return color.Background(lipgloss.Color(p.Purple))
	case CalloutSuccess:
		return color.Background(lipgloss.Color(p.Green))
	}
	return color.Background(lipgloss.Color(p.FgMuted))
}

// CalloutBar returns the leading │ bar style matched to a callout
// kind so the bar colour reinforces the badge.
func CalloutBar(kind CalloutKind, p canonical.Palette) lipgloss.Style {
	switch kind {
	case CalloutTip, CalloutNote:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Cyan))
	case CalloutInfo:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Blue))
	case CalloutWarning:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Yellow))
	case CalloutDanger:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Red))
	case CalloutImportant:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Purple))
	case CalloutSuccess:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Green))
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.FgMuted))
}

// MarkdownRolesFor returns a MarkdownRoles built against the given
// lipgloss renderer + canonical palette. Pass r =
// lipgloss.DefaultRenderer() for normal stdout, or a renderer with
// WithColorProfile(termenv.Ascii) for the NO_COLOR path. The palette
// argument is a parameter (no Active/SetActive global) so concurrent
// renders with different palettes — including the NO_COLOR contrast
// test — never race.
func MarkdownRolesFor(r *lipgloss.Renderer, p canonical.Palette) MarkdownRoles {
	color := r.NewStyle
	return MarkdownRoles{
		H1Bar: color().
			Background(lipgloss.Color(p.BgBar)).
			Foreground(lipgloss.Color(p.Purple)).
			Bold(true),
		H1Text: color().
			Background(lipgloss.Color(p.BgBar)).
			Foreground(lipgloss.Color(p.Purple)).
			Bold(true),
		// H2 wears a subtle BG chip behind the text (BgChip) and a
		// Purple bold foreground so it visually shares the H1 family
		// without claiming the full-width banner. Padding keeps the
		// chip from sitting flush against the prose around it.
		H2: color().
			Foreground(lipgloss.Color(p.Purple)).
			Background(lipgloss.Color(p.BgChip)).
			Bold(true).
			Padding(0, 1),
		// H3 wears bold + cyan + double-bar — distinct family head
		// (purple H1+H2 → cyan H3+H4) with the loudest bar treatment.
		H3: color().
			Foreground(lipgloss.Color(p.Cyan)).
			Bold(true),
		// H4 drops the bold and shifts to Blue so the cyan/blue pair
		// reads as "same family, lower rung". Without this delta H3
		// and H4 collapsed onto each other (both bold cyan) — only the
		// `▌▌` vs `▌` glyph distinguished them, which forced the eye
		// to count.
		H4: color().
			Foreground(lipgloss.Color(p.Blue)),
		H5: color().
			Foreground(lipgloss.Color(p.FgDim)),
		// A11y-3 (audit §2.5): no Faint() on prose. Faint dims a
		// terminal foreground 30–50%, which on top of an already-Muted
		// FgMuted drops the pair below WCAG AA. Italic alone carries
		// the "lowest level heading" semantics — but italic in
		// monospace fonts is unreliable, so the prefix glyph (·)
		// carries the level even if the font has no italic face.
		H6: color().
			Foreground(lipgloss.Color(p.FgMuted)).
			Italic(true),
		Paragraph: color().
			Foreground(lipgloss.Color(p.Fg)),
		// HR is structural punctuation, not content — it should
		// recede, not announce. FgMuted made the rule almost as
		// bright as body text. BgChip is the same colour the
		// frontmatter / footnote separators already use, so all
		// horizontal-rule-like elements share one visual language.
		HRule: color().
			Foreground(lipgloss.Color(p.BgChip)),

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
			Background(lipgloss.Color(p.BgCode)),
		LinkText: color().
			Foreground(lipgloss.Color(p.Blue)),

		CodeFenceBg: color().
			Background(lipgloss.Color(p.BgCode)).
			Foreground(lipgloss.Color(p.FgDim)),
		// Codeband uses BgChipSoft (a touch lighter than BgCode) so
		// the band reads as a frame edge rather than a continuation
		// of the H1 / H2 banners (those use BgBar + BgChip). Without
		// this delta a code block at the top of a section visually
		// merged into the H1 banner above it.
		CodeFenceBand: color().
			Background(lipgloss.Color(p.BgChipSoft)).
			Foreground(lipgloss.Color(p.FgMuted)),
		CodeFenceLabel: color().
			Background(lipgloss.Color(p.BgChipSoft)).
			Foreground(lipgloss.Color(p.Cyan)).
			Bold(true),
		CodeFencePlain: color().
			Background(lipgloss.Color(p.BgCode)).
			Foreground(lipgloss.Color(p.FgDim)),

		Bullet1:      color().Foreground(lipgloss.Color(p.Blue)).Bold(true),
		Bullet2:      color().Foreground(lipgloss.Color(p.Cyan)),
		Bullet3:      color().Foreground(lipgloss.Color(p.Purple)),
		Bullet4:      color().Foreground(lipgloss.Color(p.FgMuted)),
		NumberMarker: color().Foreground(lipgloss.Color(p.FgMuted)).Bold(true),
		TaskOpen:     color().Foreground(lipgloss.Color(p.Yellow)).Bold(true),
		TaskDone:     color().Foreground(lipgloss.Color(p.Green)).Bold(true),
		TaskDoneText: color().Foreground(lipgloss.Color(p.FgMuted)).Strikethrough(true),

		TableBorder: color().Foreground(lipgloss.Color(p.FgMuted)),
		TableHeader: color().Foreground(lipgloss.Color(p.Cyan)).Bold(true),
		TableCell:   color().Foreground(lipgloss.Color(p.Fg)),
		TableRowAlt: color().Background(lipgloss.Color(p.BgChipSoft)).Foreground(lipgloss.Color(p.Fg)),

		WikilinkValid:  color().Foreground(lipgloss.Color(p.Cyan)),
		WikilinkBroken: color().Foreground(lipgloss.Color(p.Red)).Faint(true),
		ImageChip:      color().Foreground(lipgloss.Color(p.FgMuted)).Background(lipgloss.Color(p.BgChipSoft)),

		CardBadgeDaily:   color().Foreground(lipgloss.Color(p.Bg)).Background(lipgloss.Color(p.Yellow)).Bold(true).Padding(0, 1),
		CardBadgeProject: color().Foreground(lipgloss.Color(p.Bg)).Background(lipgloss.Color(p.Purple)).Bold(true).Padding(0, 1),
		CardBadgeFree:    color().Foreground(lipgloss.Color(p.Bg)).Background(lipgloss.Color(p.Cyan)).Bold(true).Padding(0, 1),
		CardTitle:        color().Foreground(lipgloss.Color(p.Fg)).Bold(true),
		CardMeta:         color().Foreground(lipgloss.Color(p.FgMuted)).Italic(true),
		CardProjectChip:  color().Foreground(lipgloss.Color(p.Green)).Background(lipgloss.Color(p.BgChipSoft)),
		CardSeparator:    color().Foreground(lipgloss.Color(p.BgChip)),
		BlockquoteBar:    color().Foreground(lipgloss.Color(p.FgMuted)),
		BlockquoteText:   color().Foreground(lipgloss.Color(p.FgDim)).Italic(true),

		FootnoteRef:       color().Foreground(lipgloss.Color(p.Purple)).Bold(true),
		FootnoteListTitle: color().Foreground(lipgloss.Color(p.Cyan)).Bold(true),
		FootnoteDef:       color().Foreground(lipgloss.Color(p.FgDim)),
	}
}
