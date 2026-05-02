// Package browse renders the kompendium read view as a Bubble Tea TUI: a
// tier-sorted note list with navigation, search, and a live Markdown
// preview pane. All colors come from internal/frontend/tui/theme so a
// palette swap stays a one-file edit.
package browse

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/serverkraken/flow/internal/frontend/tui/markdown/theme"
)

// Layout / chrome.
var (
	frameStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(theme.Blue)).
			Padding(0, 1)

	headlineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Blue)).
			Bold(true)

	headerSeparatorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.BgHighlight))

	repoChipStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Bg)).
			Background(lipgloss.Color(theme.Teal)).
			Bold(true).
			Padding(0, 1)

	statusLineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Muted))

	statusKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Muted)).
			Bold(true)

	statusValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.FgDim))

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(theme.BgHighlight))

	panelTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Muted)).
			Italic(true)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Muted)).
			Italic(true)

	footerKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Cyan)).
			Bold(true)
)

// List items.
var (
	cursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Blue)).
			Bold(true)

	cursorStripeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.Cyan)).
				Bold(true)

	selectedTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.Fg)).
				Bold(true)

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Fg))

	dateStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.FgDim))

	todayDateStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Yellow)).
			Bold(true)

	todayMarkerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.Yellow)).
				Bold(true)

	excerptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Muted)).
			Italic(true)

	matchStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Bg)).
			Background(lipgloss.Color(theme.Yellow)).
			Bold(true)
)

// tagChipStyle returns a chip styled with a palette color picked from a
// stable hash of the tag text. That way two notes share the same chip
// color for `#go` while `#tmux` gets a different one — the tag-bar reads
// less monotone without inventing meaning.
func tagChipStyle(tag string) lipgloss.Style {
	bg := theme.TagPalette[tagColorIdx(tag)]
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Bg)).
		Background(lipgloss.Color(bg)).
		Bold(true).
		Padding(0, 1)
}

// tagColorIdx is a tiny FNV-1a hash. Importing hash/fnv just for chips
// would be overkill; this fits in five lines and is deterministic.
func tagColorIdx(s string) int {
	const offset, prime uint32 = 2166136261, 16777619
	h := offset
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= prime
	}
	return int(h % uint32(len(theme.TagPalette)))
}

// Type badges.
var (
	badgeDailyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Bg)).
			Background(lipgloss.Color(theme.Blue)).
			Bold(true).
			Padding(0, 1)

	badgeProjectStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.Bg)).
				Background(lipgloss.Color(theme.Green)).
				Bold(true).
				Padding(0, 1)

	badgeFreeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Bg)).
			Background(lipgloss.Color(theme.Purple)).
			Bold(true).
			Padding(0, 1)

	badgeUnknownStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.Fg)).
				Background(lipgloss.Color(theme.Muted)).
				Bold(true).
				Padding(0, 1)
)

// Header type-count pills (subtler than the row badges — only fg/bold).
var (
	countDailyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Blue)).
			Bold(true)

	countProjectStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.Green)).
				Bold(true)

	countFreeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Purple)).
			Bold(true)
)

// Search bar.
var (
	searchActiveLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.Yellow)).
				Bold(true)

	searchPassiveLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.Muted))

	searchValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.Fg))
)

// Modals — DoubleBorder lifts them visually above the rounded outer frame.
var (
	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color(theme.Red)).
			Padding(1, 3).
			Background(lipgloss.Color(theme.PanelBg)).
			Foreground(lipgloss.Color(theme.Fg))

	modalSafeStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color(theme.Blue)).
			Padding(1, 3).
			Background(lipgloss.Color(theme.PanelBg)).
			Foreground(lipgloss.Color(theme.Fg))

	modalDangerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.Red)).
				Bold(true)

	modalQuestionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.Fg)).
				Bold(true)

	modalKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Bg)).
			Background(lipgloss.Color(theme.Blue)).
			Bold(true).
			Padding(0, 1)

	modalKeyDangerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.Bg)).
				Background(lipgloss.Color(theme.Red)).
				Bold(true).
				Padding(0, 1)

	modalHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Muted)).
			Italic(true)
)

// Misc.
var (
	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Muted))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Red)).
			Bold(true)

	emptyGlyphStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Blue)).
			Bold(true)

	emptyTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.FgDim)).
			Bold(true)

	emptyHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Muted)).
			Italic(true)

	spinnerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Cyan))

	paginatorActiveDotStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.Cyan))

	paginatorInactiveDotStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color(theme.BgHighlight))

	paginatorCounterStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.Muted)).
				Italic(true)
)

// Status bar — sits at the bottom of the frame, full-width, vim-style.
var (
	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(theme.BgHighlight)).
			Foreground(lipgloss.Color(theme.FgDim))

	statusBarModeSearchStyle = lipgloss.NewStyle().
					Background(lipgloss.Color(theme.Yellow)).
					Foreground(lipgloss.Color(theme.Bg)).
					Bold(true).
					Padding(0, 1)

	statusBarModeDeleteStyle = lipgloss.NewStyle().
					Background(lipgloss.Color(theme.Red)).
					Foreground(lipgloss.Color(theme.Bg)).
					Bold(true).
					Padding(0, 1)

	statusBarPathStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(theme.BgHighlight)).
				Foreground(lipgloss.Color(theme.Fg))

	statusBarMetaStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(theme.BgHighlight)).
				Foreground(lipgloss.Color(theme.FgDim))
)
