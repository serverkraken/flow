// Package browse renders the kompendium read view as a Bubble Tea TUI: a
// tier-sorted note list with navigation, search, and a live Markdown
// preview pane. All colors come from internal/frontend/tui/theme so a
// palette swap stays a one-file edit.
package browse

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// pal is the canonical palette this package's styles render against.
// Initialised once at package-init from theme.Default; a runtime swap
// would need this var rewritten (P3+ component-kit work moves browse
// onto a per-render palette parameter, removing the package var).
var pal = theme.Default

// Layout / chrome.
var (
	frameStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(pal.Blue)).
			Padding(0, 1)

	headlineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.Blue)).
			Bold(true)

	headerSeparatorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(pal.BgChip))

	repoChipStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.Bg)).
			Background(lipgloss.Color(pal.Teal)).
			Bold(true).
			Padding(0, 1)

	statusLineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.FgMuted))

	statusKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.FgMuted)).
			Bold(true)

	statusValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(pal.FgDim))

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(pal.BgChip))

	panelTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.FgMuted)).
			Italic(true)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.FgMuted)).
			Italic(true)

	footerKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.Cyan)).
			Bold(true)
)

// List items.
var (
	cursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.Blue)).
			Bold(true)

	cursorStripeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(pal.Cyan)).
				Bold(true)

	selectedTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(pal.Fg)).
				Bold(true)

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.Fg))

	dateStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.FgDim))

	todayDateStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.Yellow)).
			Bold(true)

	todayMarkerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(pal.Yellow)).
				Bold(true)

	excerptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.FgMuted)).
			Italic(true)

	matchStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.Bg)).
			Background(lipgloss.Color(pal.Yellow)).
			Bold(true)
)

// tagChipStyle returns a chip styled with a palette color picked from a
// stable hash of the tag text. That way two notes share the same chip
// color for `#go` while `#tmux` gets a different one — the tag-bar reads
// less monotone without inventing meaning.
func tagChipStyle(tag string) lipgloss.Style {
	bg := pal.TagPalette[tagColorIdx(tag)]
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(pal.Bg)).
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
	return int(h % uint32(len(pal.TagPalette)))
}

// Type badges.
var (
	badgeDailyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.Bg)).
			Background(lipgloss.Color(pal.Blue)).
			Bold(true).
			Padding(0, 1)

	badgeProjectStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(pal.Bg)).
				Background(lipgloss.Color(pal.Green)).
				Bold(true).
				Padding(0, 1)

	badgeFreeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.Bg)).
			Background(lipgloss.Color(pal.Purple)).
			Bold(true).
			Padding(0, 1)

	badgeUnknownStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(pal.Fg)).
				Background(lipgloss.Color(pal.FgMuted)).
				Bold(true).
				Padding(0, 1)
)

// Header type-count pills (subtler than the row badges — only fg/bold).
var (
	countDailyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.Blue)).
			Bold(true)

	countProjectStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(pal.Green)).
				Bold(true)

	countFreeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.Purple)).
			Bold(true)
)

// Search bar.
var (
	searchActiveLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(pal.Yellow)).
				Bold(true)

	searchPassiveLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(pal.FgMuted))

	searchValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(pal.Fg))
)

// Modals — DoubleBorder lifts them visually above the rounded outer frame.
var (
	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color(pal.Red)).
			Padding(1, 3).
			Background(lipgloss.Color(pal.BgPanel)).
			Foreground(lipgloss.Color(pal.Fg))

	modalSafeStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color(pal.Blue)).
			Padding(1, 3).
			Background(lipgloss.Color(pal.BgPanel)).
			Foreground(lipgloss.Color(pal.Fg))

	modalDangerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(pal.Red)).
				Bold(true)

	modalQuestionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(pal.Fg)).
				Bold(true)

	modalKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.Bg)).
			Background(lipgloss.Color(pal.Blue)).
			Bold(true).
			Padding(0, 1)

	modalKeyDangerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(pal.Bg)).
				Background(lipgloss.Color(pal.Red)).
				Bold(true).
				Padding(0, 1)

	modalHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.FgMuted)).
			Italic(true)
)

// Misc.
var (
	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.FgMuted))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.Red)).
			Bold(true)

	emptyGlyphStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.Blue)).
			Bold(true)

	emptyTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.FgDim)).
			Bold(true)

	emptyHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.FgMuted)).
			Italic(true)

	spinnerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.Cyan))

	paginatorActiveDotStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(pal.Cyan))

	paginatorInactiveDotStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color(pal.BgChip))

	paginatorCounterStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(pal.FgMuted)).
				Italic(true)
)

// Status bar — sits at the bottom of the frame, full-width, vim-style.
var (
	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(pal.BgChip)).
			Foreground(lipgloss.Color(pal.FgDim))

	statusBarModeSearchStyle = lipgloss.NewStyle().
					Background(lipgloss.Color(pal.Yellow)).
					Foreground(lipgloss.Color(pal.Bg)).
					Bold(true).
					Padding(0, 1)

	statusBarModeDeleteStyle = lipgloss.NewStyle().
					Background(lipgloss.Color(pal.Red)).
					Foreground(lipgloss.Color(pal.Bg)).
					Bold(true).
					Padding(0, 1)

	statusBarPathStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(pal.BgChip)).
				Foreground(lipgloss.Color(pal.Fg))

	statusBarMetaStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(pal.BgChip)).
				Foreground(lipgloss.Color(pal.FgDim))
)
