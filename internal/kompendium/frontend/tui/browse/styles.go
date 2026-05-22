// Package browse renders the kompendium read view as a Bubble Tea TUI: a
// tier-sorted note list with navigation, search, and a live Markdown
// preview pane. All colors come from internal/frontend/tui/theme so a
// palette swap stays a one-file edit.
package browse

import (
	"charm.land/lipgloss/v2"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// pal ist die canonical Palette dieses Packages. Init beim Package-
// Load aus theme.Default. Per Composition-Root-Bridge wird sie über
// SetPalette(pal) auf den Live-Wert (= tk.Load() in cli/sidekick.go
// und cmd/flow/main.go) ge-swappt; rebuildStyles() baut alle vars
// danach neu, sodass ein @tn_*-tmux-Overlay durchschlägt.
//
// sem ist die Sem()-Sicht — Components konsumieren laut
// docs/design-system.md den semantischen Alias (`sem.Accent`,
// `sem.Danger` …), nicht die rohe Hue. Hue-Direct-Zugriffe bleiben
// nur dort, wo es kein passendes Sem gibt (Teal als Repo-Chip-Hue,
// TagPalette für Tag-Hash-Rotation).
var (
	pal = theme.Default
	sem = pal.Sem()
)

// SetPalette swappt die Package-Palette und rebuildet alle Styles. In
// einem Composition-Root vor browse.New(...)-Aufruf ausführen, sobald
// der Live-Pal-Wert (tk.Load()) verfügbar ist. Tests, die ohne tmux-
// Overlay laufen, können den Default belassen.
func SetPalette(p theme.Palette) {
	pal = p
	sem = p.Sem()
	rebuildStyles()
}

// Layout / chrome.
var (
	frameStyle           lipgloss.Style
	headlineStyle        lipgloss.Style
	headerSeparatorStyle lipgloss.Style
	repoChipStyle        lipgloss.Style
	statusLineStyle      lipgloss.Style
	statusKeyStyle       lipgloss.Style
	statusValueStyle     lipgloss.Style
	panelStyle           lipgloss.Style
	panelTitleStyle      lipgloss.Style
	footerStyle          lipgloss.Style
	footerKeyStyle       lipgloss.Style
)

// List items.
var (
	cursorStyle        lipgloss.Style
	cursorStripeStyle  lipgloss.Style
	selectedTitleStyle lipgloss.Style
	titleStyle         lipgloss.Style
	dateStyle          lipgloss.Style
	todayDateStyle     lipgloss.Style
	todayMarkerStyle   lipgloss.Style
	excerptStyle       lipgloss.Style
	matchStyle         lipgloss.Style
)

// Type badges.
var (
	badgeDailyStyle   lipgloss.Style
	badgeProjectStyle lipgloss.Style
	badgeFreeStyle    lipgloss.Style
	badgeUnknownStyle lipgloss.Style
)

// Header type-count pills.
var (
	countDailyStyle   lipgloss.Style
	countProjectStyle lipgloss.Style
	countFreeStyle    lipgloss.Style
)

// Search bar.
var (
	searchActiveLabelStyle  lipgloss.Style
	searchPassiveLabelStyle lipgloss.Style
	searchValueStyle        lipgloss.Style
)

// Modal-internal content (Headline/Question/Hint). Der Frame selbst
// (DoubleBorder + BgPanel + Padding) kommt aus components/modal.
var (
	modalDangerStyle   lipgloss.Style
	modalQuestionStyle lipgloss.Style
	modalHintStyle     lipgloss.Style
)

// Misc.
var (
	dimStyle                  lipgloss.Style
	errorStyle                lipgloss.Style
	emptyGlyphStyle           lipgloss.Style
	emptyTitleStyle           lipgloss.Style
	emptyHintStyle            lipgloss.Style
	spinnerStyle              lipgloss.Style
	paginatorActiveDotStyle   lipgloss.Style
	paginatorInactiveDotStyle lipgloss.Style
	paginatorCounterStyle     lipgloss.Style
)

// Status bar.
var (
	statusBarStyle           lipgloss.Style
	statusBarModeSearchStyle lipgloss.Style
	statusBarModeDeleteStyle lipgloss.Style
	statusBarPathStyle       lipgloss.Style
	statusBarMetaStyle       lipgloss.Style
)

func init() { rebuildStyles() }

// rebuildStyles weist alle Style-vars neu zu auf Basis der aktuellen
// pal/sem. Aufgerufen aus init() (Default-Palette) und aus
// SetPalette() (Runtime-Swap). Ein Aufruf reicht — die vars werden
// dann von den Render-Sites direkt konsumiert (kein Per-Render-Build).
func rebuildStyles() {
	// Layout / chrome.
	frameStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(sem.Accent).
		Padding(0, 1)
	headlineStyle = lipgloss.NewStyle().
		Foreground(sem.Accent).
		Bold(true)
	headerSeparatorStyle = lipgloss.NewStyle().
		Foreground(pal.BgChip)
	repoChipStyle = lipgloss.NewStyle().
		Foreground(pal.Bg).
		Background(pal.Teal).
		Bold(true).
		Padding(0, 1)
	statusLineStyle = lipgloss.NewStyle().
		Foreground(pal.FgMuted)
	statusKeyStyle = lipgloss.NewStyle().
		Foreground(pal.FgMuted).
		Bold(true)
	statusValueStyle = lipgloss.NewStyle().
		Foreground(pal.FgDim)
	panelStyle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(pal.BgChip)
	panelTitleStyle = lipgloss.NewStyle().
		Foreground(pal.FgMuted)
	footerStyle = lipgloss.NewStyle().
		Foreground(pal.FgMuted)
	footerKeyStyle = lipgloss.NewStyle().
		Foreground(sem.Active).
		Bold(true)

	// List items.
	cursorStyle = lipgloss.NewStyle().
		Foreground(sem.Accent).
		Bold(true)
	cursorStripeStyle = lipgloss.NewStyle().
		Foreground(sem.Active).
		Bold(true)
	selectedTitleStyle = lipgloss.NewStyle().
		Foreground(pal.Fg).
		Bold(true)
	titleStyle = lipgloss.NewStyle().
		Foreground(pal.Fg)
	dateStyle = lipgloss.NewStyle().
		Foreground(pal.FgDim)
	todayDateStyle = lipgloss.NewStyle().
		Foreground(sem.Warning).
		Bold(true)
	todayMarkerStyle = lipgloss.NewStyle().
		Foreground(sem.Warning).
		Bold(true)
	excerptStyle = lipgloss.NewStyle().
		Foreground(pal.FgMuted)
	matchStyle = lipgloss.NewStyle().
		Foreground(pal.Bg).
		Background(sem.Warning).
		Bold(true)

	// Badges.
	badgeDailyStyle = lipgloss.NewStyle().
		Foreground(pal.Bg).
		Background(sem.Accent).
		Bold(true).
		Padding(0, 1)
	badgeProjectStyle = lipgloss.NewStyle().
		Foreground(pal.Bg).
		Background(sem.Success).
		Bold(true).
		Padding(0, 1)
	badgeFreeStyle = lipgloss.NewStyle().
		Foreground(pal.Bg).
		Background(sem.Highlight).
		Bold(true).
		Padding(0, 1)
	badgeUnknownStyle = lipgloss.NewStyle().
		Foreground(pal.Fg).
		Background(pal.FgMuted).
		Bold(true).
		Padding(0, 1)

	// Counts.
	countDailyStyle = lipgloss.NewStyle().
		Foreground(sem.Accent).
		Bold(true)
	countProjectStyle = lipgloss.NewStyle().
		Foreground(sem.Success).
		Bold(true)
	countFreeStyle = lipgloss.NewStyle().
		Foreground(sem.Highlight).
		Bold(true)

	// Search.
	searchActiveLabelStyle = lipgloss.NewStyle().
		Foreground(sem.Warning).
		Bold(true)
	searchPassiveLabelStyle = lipgloss.NewStyle().
		Foreground(pal.FgMuted)
	searchValueStyle = lipgloss.NewStyle().
		Foreground(pal.Fg)

	// Modal.
	modalDangerStyle = lipgloss.NewStyle().
		Foreground(sem.Danger).
		Bold(true)
	modalQuestionStyle = lipgloss.NewStyle().
		Foreground(pal.Fg).
		Bold(true)
	modalHintStyle = lipgloss.NewStyle().
		Foreground(pal.FgMuted)

	// Misc.
	dimStyle = lipgloss.NewStyle().
		Foreground(pal.FgMuted)
	errorStyle = lipgloss.NewStyle().
		Foreground(sem.Danger).
		Bold(true)
	emptyGlyphStyle = lipgloss.NewStyle().
		Foreground(sem.Accent).
		Bold(true)
	emptyTitleStyle = lipgloss.NewStyle().
		Foreground(pal.FgDim).
		Bold(true)
	emptyHintStyle = lipgloss.NewStyle().
		Foreground(pal.FgMuted)
	spinnerStyle = lipgloss.NewStyle().
		Foreground(sem.Active)
	paginatorActiveDotStyle = lipgloss.NewStyle().
		Foreground(sem.Active)
	paginatorInactiveDotStyle = lipgloss.NewStyle().
		Foreground(pal.BgChip)
	paginatorCounterStyle = lipgloss.NewStyle().
		Foreground(pal.FgMuted)

	// Status bar.
	statusBarStyle = lipgloss.NewStyle().
		Background(pal.BgChip).
		Foreground(pal.FgDim)
	statusBarModeSearchStyle = lipgloss.NewStyle().
		Background(sem.Warning).
		Foreground(pal.Bg).
		Bold(true).
		Padding(0, 1)
	statusBarModeDeleteStyle = lipgloss.NewStyle().
		Background(sem.Danger).
		Foreground(pal.Bg).
		Bold(true).
		Padding(0, 1)
	statusBarPathStyle = lipgloss.NewStyle().
		Background(pal.BgChip).
		Foreground(pal.Fg)
	statusBarMetaStyle = lipgloss.NewStyle().
		Background(pal.BgChip).
		Foreground(pal.FgDim)
}

// tagChipStyle returns a chip styled with a palette color picked from a
// stable hash of the tag text. That way two notes share the same chip
// color for `#go` while `#tmux` gets a different one — the tag-bar reads
// less monotone without inventing meaning.
func tagChipStyle(tag string) lipgloss.Style {
	bg := pal.TagPalette[tagColorIdx(tag)]
	return lipgloss.NewStyle().
		Foreground(pal.Bg).
		Background(bg).
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
