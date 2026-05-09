// Package browse renders the kompendium read view as a Bubble Tea TUI: a
// tier-sorted note list with navigation, search, and a live Markdown
// preview pane. All colors come from internal/frontend/tui/theme so a
// palette swap stays a one-file edit.
package browse

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// pal ist die canonical Palette dieses Packages. Init beim Package-
// Load aus theme.Default; Runtime-Swap (Stufe-7-Goal) wired pal als
// per-render-Param.
//
// sem ist die Sem()-Sicht auf pal — Components konsumieren laut
// docs/design-system.md den semantischen Alias (`sem.Accent`,
// `sem.Danger` …), nicht die rohe Hue. Hue-Direct-Zugriffe bleiben
// nur dort, wo es kein passendes Sem gibt (Teal als Repo-Chip-Hue,
// TagPalette für Tag-Hash-Rotation).
var (
	pal = theme.Default
	sem = pal.Sem()
)

// Layout / chrome.
var (
	frameStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(sem.Accent).
			Padding(0, 1)

	headlineStyle = lipgloss.NewStyle().
			Foreground(sem.Accent).
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
			Foreground(sem.Active).
			Bold(true)
)

// List items.
var (
	cursorStyle = lipgloss.NewStyle().
			Foreground(sem.Accent).
			Bold(true)

	cursorStripeStyle = lipgloss.NewStyle().
				Foreground(sem.Active).
				Bold(true)

	selectedTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(pal.Fg)).
				Bold(true)

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.Fg))

	dateStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.FgDim))

	todayDateStyle = lipgloss.NewStyle().
			Foreground(sem.Warning).
			Bold(true)

	todayMarkerStyle = lipgloss.NewStyle().
				Foreground(sem.Warning).
				Bold(true)

	excerptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.FgMuted)).
			Italic(true)

	matchStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.Bg)).
			Background(sem.Warning).
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
			Background(sem.Accent).
			Bold(true).
			Padding(0, 1)

	badgeProjectStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(pal.Bg)).
				Background(sem.Success).
				Bold(true).
				Padding(0, 1)

	badgeFreeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.Bg)).
			Background(sem.Highlight).
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
			Foreground(sem.Accent).
			Bold(true)

	countProjectStyle = lipgloss.NewStyle().
				Foreground(sem.Success).
				Bold(true)

	countFreeStyle = lipgloss.NewStyle().
			Foreground(sem.Highlight).
			Bold(true)
)

// Search bar.
var (
	searchActiveLabelStyle = lipgloss.NewStyle().
				Foreground(sem.Warning).
				Bold(true)

	searchPassiveLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(pal.FgMuted))

	searchValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(pal.Fg))
)

// Modal-internal content styles (Headline/Question/Hint). Der Frame
// selbst (DoubleBorder + BgPanel + Padding) kommt seit P4c aus
// components/modal — siehe model.go renderDeleteModal /
// renderHelpOverlay. modalStyle, modalSafeStyle und die Key-Pillen
// (modalKey*Style) wurden entsprechend entfernt; für Key-Highlights
// in zukünftigen Modalen ist components/theme.RenderPill der richtige
// Ort.
var (
	modalDangerStyle = lipgloss.NewStyle().
				Foreground(sem.Danger).
				Bold(true)

	modalQuestionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(pal.Fg)).
				Bold(true)

	modalHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.FgMuted)).
			Italic(true)
)

// Misc.
var (
	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.FgMuted))

	errorStyle = lipgloss.NewStyle().
			Foreground(sem.Danger).
			Bold(true)

	emptyGlyphStyle = lipgloss.NewStyle().
			Foreground(sem.Accent).
			Bold(true)

	emptyTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.FgDim)).
			Bold(true)

	emptyHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(pal.FgMuted)).
			Italic(true)

	spinnerStyle = lipgloss.NewStyle().
			Foreground(sem.Active)

	paginatorActiveDotStyle = lipgloss.NewStyle().
				Foreground(sem.Active)

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
					Background(sem.Warning).
					Foreground(lipgloss.Color(pal.Bg)).
					Bold(true).
					Padding(0, 1)

	statusBarModeDeleteStyle = lipgloss.NewStyle().
					Background(sem.Danger).
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
