package markdown_overlay

import (
	"sync/atomic"

	"charm.land/lipgloss/v2"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// chromeStyles is a coherent snapshot of every lipgloss.Style this
// component renders with. Held behind an atomic.Pointer so SetPalette
// can replace the whole set with a single Store while readers do a
// single Load — there is no half-rebuilt state visible to render.
//
// Background (round4): the previous package-var layout had 13 globals
// that SetPalette rewrote one-by-one in rebuildStyles. Single-goroutine
// bubbletea event loop tolerated that, but t.Parallel() in tests +
// SetPalette would surface as a -race data race. atomic.Pointer keeps
// reads lock-free, writes coherent.
type chromeStyles struct {
	frame               lipgloss.Style
	title               lipgloss.Style
	separator           lipgloss.Style
	footer              lipgloss.Style
	statusBar           lipgloss.Style
	statusBarPath       lipgloss.Style
	statusBarModeSearch lipgloss.Style
	searchActiveLabel   lipgloss.Style
	cursor              lipgloss.Style
	matchBar            lipgloss.Style
	matchCurrentBar     lipgloss.Style
	err                 lipgloss.Style
}

var stylesPtr atomic.Pointer[chromeStyles]

// styles returns the active style snapshot. Callers MUST treat the
// returned pointer as immutable; mutating any field corrupts the
// shared snapshot.
func styles() *chromeStyles { return stylesPtr.Load() }

// SetPalette swaps the package palette atomically and rebuilds all
// styles. Call once at boot, before any New(...) — see cmd/flow/main.go.
// Safe to call concurrently with reads thanks to the atomic.Pointer.
func SetPalette(p theme.Palette) {
	stylesPtr.Store(buildStyles(p))
}

// init seeds the styles from theme.Default so the component renders
// correctly when imported by tests that don't wire the composition
// root. Production callers (cmd/flow/main.go) override via SetPalette
// before constructing the first Model.
func init() { stylesPtr.Store(buildStyles(theme.Default)) }

func buildStyles(p theme.Palette) *chromeStyles {
	sem := p.Sem()
	return &chromeStyles{
		frame: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(sem.Accent).
			Padding(0, 1),

		title: lipgloss.NewStyle().
			Foreground(sem.Accent).
			Bold(true),

		separator: lipgloss.NewStyle().
			Foreground(p.BgChip),

		// footer renders the whole hint row — keys AND actions — in one
		// dim color, matching the app-wide statusbar.Hints convention
		// (Skill §Hint format: "All-dim (FgMuted)"). Keys carried a
		// cyan-bold footerKey before, which made the footer compete with
		// the cyan title/cursor and broke the single-accent-per-row rule;
		// the search-mode badge (searchActiveLabel) stays the one accent.
		footer: lipgloss.NewStyle().
			Foreground(p.FgMuted),

		statusBar: lipgloss.NewStyle().
			Background(p.BgChip).
			Foreground(p.FgDim),

		statusBarPath: lipgloss.NewStyle().
			Background(p.BgChip).
			Foreground(p.Fg),

		statusBarModeSearch: lipgloss.NewStyle().
			Background(sem.Warning).
			Foreground(p.Bg).
			Bold(true).
			Padding(0, 1),

		searchActiveLabel: lipgloss.NewStyle().
			Foreground(sem.Warning).
			Bold(true),

		cursor: lipgloss.NewStyle().
			Foreground(sem.Accent).
			Bold(true),

		// matchBar renders the left-margin bar prepended to lines that
		// matched the search query. Inline highlight would have to
		// splice SGR codes into glamour output without breaking nested
		// OSC 8 hyperlinks — fragile. A two-cell left bar stays robust.
		matchBar: lipgloss.NewStyle().
			Foreground(sem.Warning),

		// matchCurrentBar marks the cursor's current match. Sem has no
		// Orange alias (Orange is a Markdown-domain hue, not a semantic
		// token); pal.Orange stays direct.
		matchCurrentBar: lipgloss.NewStyle().
			Foreground(p.Orange).
			Bold(true),

		err: lipgloss.NewStyle().
			Foreground(sem.Danger).
			Bold(true),
	}
}
