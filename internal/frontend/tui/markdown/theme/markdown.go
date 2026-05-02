package theme

// Markdown role mapping. The renderer in internal/frontend/tui/markdown
// looks up its block / inline styles here so a palette swap stays a
// one-file edit (the active Palette in palette.go is the only ground
// truth).

import "github.com/charmbracelet/lipgloss"

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
// for a callout kind.
func CalloutBadge(kind CalloutKind) lipgloss.Style {
	color := lipgloss.NewStyle().
		Foreground(lipgloss.Color(Bg)).
		Bold(true).
		Padding(0, 1)
	switch kind {
	case CalloutTip, CalloutNote:
		return color.Background(lipgloss.Color(Cyan))
	case CalloutInfo:
		return color.Background(lipgloss.Color(Blue))
	case CalloutWarning:
		return color.Background(lipgloss.Color(Yellow))
	case CalloutDanger:
		return color.Background(lipgloss.Color(Red))
	case CalloutImportant:
		return color.Background(lipgloss.Color(Purple))
	case CalloutSuccess:
		return color.Background(lipgloss.Color(Green))
	}
	return color.Background(lipgloss.Color(Muted))
}

// CalloutBar returns the leading │ bar style matched to a callout
// kind so the bar colour reinforces the badge.
func CalloutBar(kind CalloutKind) lipgloss.Style {
	switch kind {
	case CalloutTip, CalloutNote:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(Cyan))
	case CalloutInfo:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(Blue))
	case CalloutWarning:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(Yellow))
	case CalloutDanger:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(Red))
	case CalloutImportant:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(Purple))
	case CalloutSuccess:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(Green))
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(Muted))
}

// MarkdownRolesFor returns a MarkdownRoles built against the given
// lipgloss renderer. Pass r = lipgloss.DefaultRenderer() for normal
// stdout, or a renderer with WithColorProfile(termenv.Ascii) for the
// NO_COLOR path.
func MarkdownRolesFor(r *lipgloss.Renderer) MarkdownRoles {
	color := r.NewStyle
	return MarkdownRoles{
		H1Bar: color().
			Background(lipgloss.Color(BarBg)).
			Foreground(lipgloss.Color(Purple)).
			Bold(true),
		H1Text: color().
			Background(lipgloss.Color(BarBg)).
			Foreground(lipgloss.Color(Purple)).
			Bold(true),
		// H2 wears a subtle BG chip behind the text (BgHighlight) and
		// a Purple bold foreground so it visually shares the H1 family
		// without claiming the full-width banner. Padding keeps the
		// chip from sitting flush against the prose around it.
		H2: color().
			Foreground(lipgloss.Color(Purple)).
			Background(lipgloss.Color(BgHighlight)).
			Bold(true).
			Padding(0, 1),
		H3: color().
			Foreground(lipgloss.Color(Cyan)).
			Bold(true),
		H4: color().
			Foreground(lipgloss.Color(Cyan)).
			Bold(true),
		H5: color().
			Foreground(lipgloss.Color(FgDim)),
		H6: color().
			Foreground(lipgloss.Color(Muted)).
			Faint(true).
			Italic(true),
		Paragraph: color().
			Foreground(lipgloss.Color(Fg)),
		HRule: color().
			Foreground(lipgloss.Color(Muted)),

		Strong: color().Bold(true),
		Emph:   color().Italic(true),
		Strike: color().Strikethrough(true),
		CodeSpan: color().
			Foreground(lipgloss.Color(Green)).
			Background(lipgloss.Color(BgCode)),
		LinkText: color().
			Foreground(lipgloss.Color(Blue)),

		CodeFenceBg: color().
			Background(lipgloss.Color(BgCode)).
			Foreground(lipgloss.Color(FgDim)),
		CodeFenceBand: color().
			Background(lipgloss.Color(BarBg)).
			Foreground(lipgloss.Color(Muted)),
		CodeFenceLabel: color().
			Background(lipgloss.Color(BarBg)).
			Foreground(lipgloss.Color(Cyan)).
			Bold(true),
		CodeFencePlain: color().
			Background(lipgloss.Color(BgCode)).
			Foreground(lipgloss.Color(FgDim)),

		Bullet1:      color().Foreground(lipgloss.Color(Blue)).Bold(true),
		Bullet2:      color().Foreground(lipgloss.Color(Cyan)),
		Bullet3:      color().Foreground(lipgloss.Color(Purple)),
		Bullet4:      color().Foreground(lipgloss.Color(Muted)),
		NumberMarker: color().Foreground(lipgloss.Color(Muted)).Bold(true),
		TaskOpen:     color().Foreground(lipgloss.Color(Yellow)).Bold(true),
		TaskDone:     color().Foreground(lipgloss.Color(Green)).Bold(true),
		TaskDoneText: color().Foreground(lipgloss.Color(Muted)).Strikethrough(true),

		TableBorder: color().Foreground(lipgloss.Color(Muted)),
		TableHeader: color().Foreground(lipgloss.Color(Cyan)).Bold(true),
		TableCell:   color().Foreground(lipgloss.Color(Fg)),
		TableRowAlt: color().Background(lipgloss.Color(BgHighlightSoft)).Foreground(lipgloss.Color(Fg)),

		WikilinkValid:  color().Foreground(lipgloss.Color(Cyan)),
		WikilinkBroken: color().Foreground(lipgloss.Color(Red)).Faint(true),
		ImageChip:      color().Foreground(lipgloss.Color(Muted)).Background(lipgloss.Color(BgHighlightSoft)),

		CardBadgeDaily:   color().Foreground(lipgloss.Color(Bg)).Background(lipgloss.Color(Yellow)).Bold(true).Padding(0, 1),
		CardBadgeProject: color().Foreground(lipgloss.Color(Bg)).Background(lipgloss.Color(Purple)).Bold(true).Padding(0, 1),
		CardBadgeFree:    color().Foreground(lipgloss.Color(Bg)).Background(lipgloss.Color(Cyan)).Bold(true).Padding(0, 1),
		CardTitle:        color().Foreground(lipgloss.Color(Fg)).Bold(true),
		CardMeta:         color().Foreground(lipgloss.Color(Muted)).Italic(true),
		CardProjectChip:  color().Foreground(lipgloss.Color(Green)).Background(lipgloss.Color(BgHighlightSoft)),
		CardSeparator:    color().Foreground(lipgloss.Color(BgHighlight)),
		BlockquoteBar:    color().Foreground(lipgloss.Color(Muted)),
		BlockquoteText:   color().Foreground(lipgloss.Color(FgDim)).Italic(true),

		FootnoteRef:       color().Foreground(lipgloss.Color(Purple)).Bold(true),
		FootnoteListTitle: color().Foreground(lipgloss.Color(Cyan)).Bold(true),
		FootnoteDef:       color().Foreground(lipgloss.Color(FgDim)),
	}
}
