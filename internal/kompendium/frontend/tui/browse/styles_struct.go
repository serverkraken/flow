package browse

// browseStyles caches all lipgloss styles used by the browse render
// pipeline, built once per Model from the palette passed into New().
// Mirrors palette/projects/worktime's per-Model style cache: avoids
// package-level globals + a SetPalette() composition-root bridge, and
// keeps the render hot-path allocation-free.
//
// Each cluster maps 1:1 to a section in the legacy package-level vars
// (Layout / chrome, list rows, type badges + counts, search, modal,
// misc, status bar, cursor). The struct also carries the source palette
// so the per-tag chip styler can resolve TagPalette + Bg dynamically.

import (
	"charm.land/lipgloss/v2"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// browseStyles holds every palette-derived style consumed by the
// renderers. Built once at New() via newBrowseStyles(p).
type browseStyles struct {
	// Chrome.
	frame, headline, headerSeparator   lipgloss.Style
	repoChip                           lipgloss.Style
	statusLine, statusKey, statusValue lipgloss.Style
	panel, panelTitle, panelTitleFocus lipgloss.Style
	footer                             lipgloss.Style

	// List rows.
	cursor, cursorStripe, selectedTitle, title lipgloss.Style
	date, todayDate, todayMarker               lipgloss.Style
	excerpt, match                             lipgloss.Style

	// Type badges + counts.
	badgeDaily, badgeProject, badgeFree, badgeUnknown lipgloss.Style
	countDaily, countProject, countFree               lipgloss.Style

	// Search.
	searchActiveLabel, searchPassiveLabel, searchValue lipgloss.Style

	// Modal.
	modalDanger, modalQuestion, modalHint lipgloss.Style

	// Misc.
	dim, errorPara                                       lipgloss.Style
	emptyTitle, emptyHint                                lipgloss.Style
	spinner                                              lipgloss.Style
	paginatorActive, paginatorInactive, paginatorCounter lipgloss.Style

	// Status bar.
	statusBar, statusBarModeSearch, statusBarModeDelete lipgloss.Style
	statusBarPath, statusBarMeta                        lipgloss.Style

	// Palette source. Needed by tagChipStyle (TagPalette hash rotation),
	// the help/modal renderers (components/help.Render + components/modal.
	// Render take a palette directly), and the overlay backdrop
	// (lipgloss.WithWhitespaceStyle uses pal.BgChip).
	pal theme.Palette
}

// newBrowseStyles builds the per-Model style cache from the palette
// passed into New(). Copies the rebuildStyles() body 1:1 — the builder
// expressions are byte-for-byte identical, only the assignment targets
// move from package vars to struct fields.
func newBrowseStyles(p theme.Palette) browseStyles {
	sem := p.Sem()
	return browseStyles{
		// Layout / chrome. Outer App-Frame ist tragende Chrome —
		// BorderStrong (load-bearing) statt sem.Accent.
		frame: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(sem.BorderStrong).
			Padding(0, 1),
		headline: lipgloss.NewStyle().
			Foreground(sem.Accent).
			Bold(true),
		headerSeparator: lipgloss.NewStyle().
			Foreground(sem.Border),
		repoChip: lipgloss.NewStyle().
			Foreground(p.Bg).
			Background(p.Teal).
			Bold(true).
			Padding(0, 1),
		statusLine: lipgloss.NewStyle().
			Foreground(p.FgMuted),
		statusKey: lipgloss.NewStyle().
			Foreground(p.FgMuted).
			Bold(true),
		statusValue: lipgloss.NewStyle().
			Foreground(p.FgDim),
		panel: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(sem.BorderSubtle),
		panelTitle: lipgloss.NewStyle().
			Foreground(p.FgMuted),
		// Fokussierte Pane (Liste) — Titel in Fg+Bold gegen die
		// passiv-mute betitelte Vorschau (UX-Review L3).
		panelTitleFocus: lipgloss.NewStyle().
			Foreground(p.Fg).
			Bold(true),
		footer: lipgloss.NewStyle().
			Foreground(p.FgMuted),

		// List items.
		cursor: lipgloss.NewStyle().
			Foreground(sem.Accent).
			Bold(true),
		cursorStripe: lipgloss.NewStyle().
			Foreground(sem.Active).
			Bold(true),
		selectedTitle: lipgloss.NewStyle().
			Foreground(p.Fg).
			Bold(true),
		title: lipgloss.NewStyle().
			Foreground(p.Fg),
		date: lipgloss.NewStyle().
			Foreground(p.FgDim),
		todayDate: lipgloss.NewStyle().
			Foreground(sem.Warning).
			Bold(true),
		todayMarker: lipgloss.NewStyle().
			Foreground(sem.Warning).
			Bold(true),
		excerpt: lipgloss.NewStyle().
			Foreground(p.FgMuted),
		match: lipgloss.NewStyle().
			Foreground(p.Bg).
			Background(sem.Warning).
			Bold(true),

		// Badges.
		badgeDaily: lipgloss.NewStyle().
			Foreground(p.Bg).
			Background(sem.Accent).
			Bold(true).
			Padding(0, 1),
		badgeProject: lipgloss.NewStyle().
			Foreground(p.Bg).
			Background(sem.Success).
			Bold(true).
			Padding(0, 1),
		badgeFree: lipgloss.NewStyle().
			Foreground(p.Bg).
			Background(sem.Highlight).
			Bold(true).
			Padding(0, 1),
		badgeUnknown: lipgloss.NewStyle().
			Foreground(p.Fg).
			Background(p.FgMuted).
			Bold(true).
			Padding(0, 1),

		// Counts.
		countDaily: lipgloss.NewStyle().
			Foreground(sem.Accent).
			Bold(true),
		countProject: lipgloss.NewStyle().
			Foreground(sem.Success).
			Bold(true),
		countFree: lipgloss.NewStyle().
			Foreground(sem.Highlight).
			Bold(true),

		// Search.
		searchActiveLabel: lipgloss.NewStyle().
			Foreground(sem.Warning).
			Bold(true),
		searchPassiveLabel: lipgloss.NewStyle().
			Foreground(p.FgMuted),
		searchValue: lipgloss.NewStyle().
			Foreground(p.Fg),

		// Modal.
		modalDanger: lipgloss.NewStyle().
			Foreground(sem.Danger).
			Bold(true),
		modalQuestion: lipgloss.NewStyle().
			Foreground(p.Fg).
			Bold(true),
		modalHint: lipgloss.NewStyle().
			Foreground(p.FgMuted),

		// Misc.
		dim: lipgloss.NewStyle().
			Foreground(p.FgMuted),
		// errorPara is paragraph-surface (Skill §Builder catalog) —
		// no Bold, just Sem.Danger fg.
		errorPara: lipgloss.NewStyle().
			Foreground(sem.Danger),
		emptyTitle: lipgloss.NewStyle().
			Foreground(p.FgDim).
			Bold(true),
		emptyHint: lipgloss.NewStyle().
			Foreground(p.FgMuted),
		spinner: lipgloss.NewStyle().
			Foreground(sem.Active),
		paginatorActive: lipgloss.NewStyle().
			Foreground(sem.Active),
		paginatorInactive: lipgloss.NewStyle().
			Foreground(p.BgChip),
		paginatorCounter: lipgloss.NewStyle().
			Foreground(p.FgMuted),

		// Status bar.
		statusBar: lipgloss.NewStyle().
			Background(p.BgChip).
			Foreground(p.FgDim),
		statusBarModeSearch: lipgloss.NewStyle().
			Background(sem.Warning).
			Foreground(p.Bg).
			Bold(true).
			Padding(0, 1),
		statusBarModeDelete: lipgloss.NewStyle().
			Background(sem.Danger).
			Foreground(p.Bg).
			Bold(true).
			Padding(0, 1),
		statusBarPath: lipgloss.NewStyle().
			Background(p.BgChip).
			Foreground(p.Fg),
		statusBarMeta: lipgloss.NewStyle().
			Background(p.BgChip).
			Foreground(p.FgDim),

		pal: p,
	}
}

// tagChipStyle returns a chip styled with a palette color picked from
// a stable hash of the tag text. Two notes share the same chip color
// for `#go` while `#tmux` gets a different one — the tag-bar reads
// less monotone without inventing meaning. Hash uses the palette's
// TagPalette length.
func (s browseStyles) tagChipStyle(tag string) lipgloss.Style {
	bg := s.pal.TagPalette[tagColorIdx(tag, len(s.pal.TagPalette))]
	return lipgloss.NewStyle().
		Foreground(s.pal.Bg).
		Background(bg).
		Bold(true).
		Padding(0, 1)
}

// tagColorIdx is a tiny FNV-1a hash. Importing hash/fnv just for chips
// would be overkill; this fits in five lines and is deterministic.
// modulus is the TagPalette length, passed in so the caller's palette
// (not a package-level var) drives the rotation.
func tagColorIdx(s string, modulus int) int {
	const offset, prime uint32 = 2166136261, 16777619
	h := offset
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= prime
	}
	return int(h % uint32(modulus))
}
