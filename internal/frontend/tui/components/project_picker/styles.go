package project_picker

import (
	"sync/atomic"

	"charm.land/lipgloss/v2"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// pickerStyles is a coherent snapshot of all lipgloss.Styles the picker
// uses. Held behind an atomic.Pointer so SetPalette (called from the
// composition root) can replace the whole set atomically — no half-rebuilt
// state visible to a concurrent render. Pattern mirrors markdown_overlay.
type pickerStyles struct {
	frame     lipgloss.Style // outer rounded box
	separator lipgloss.Style // ─────── rule below filter
	filterPfx lipgloss.Style // "▶ " when active
	accentBar lipgloss.Style // ▎ selection bar on selected rows
	newRow    lipgloss.Style // "+ Neues Projekt anlegen" label
	newRowSel lipgloss.Style // same, selected (bold)
	footer    lipgloss.Style // hint line
	noMatches lipgloss.Style // "Keine Treffer." empty-state text
}

var stylesPtr atomic.Pointer[pickerStyles]

// styles returns the active style snapshot. Callers must treat the pointer
// as immutable.
func styles() *pickerStyles { return stylesPtr.Load() }

// SetPalette rebuilds all styles from p and stores the new snapshot. Call
// once at program start before constructing the first Model; safe to call
// concurrently with readers via the atomic.Pointer.
func SetPalette(p theme.Palette) {
	stylesPtr.Store(buildStyles(p))
}

// init seeds styles from theme.Default so tests and builds that don't
// wire the composition root still render correctly.
func init() { stylesPtr.Store(buildStyles(theme.Default)) }

func buildStyles(p theme.Palette) *pickerStyles {
	sem := p.Sem()
	return &pickerStyles{
		// Rounded frame: load-bearing border uses BorderStrong (≥3:1 WCAG)
		// per §Color semantics — modal / picker accent bar rule.
		frame: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(sem.BorderStrong).
			Padding(0, theme.PadXS),

		// Separator rule: dim color signals structure without competing with
		// the list content — matches markdown_overlay separator convention.
		separator: lipgloss.NewStyle().Foreground(sem.Border),

		// Filter prefix: Accent when active (one-accent-per-row rule: the
		// filter is the focal point; items are supporting cast).
		filterPfx: lipgloss.NewStyle().Foreground(sem.Accent),

		// accentBar renders ▎ in Sem.Accent for the selected row.
		// Extracted to styles so no raw hue appears in view.go.
		accentBar: lipgloss.NewStyle().Foreground(sem.Accent),

		// "+ Neues Projekt anlegen" label: Success (green) signals a
		// positive action — creating something new. Matches the overall
		// glyph+color A11y rule: the "+" glyph + green color together carry
		// the meaning. Unselected stays slightly dim (FgDim) to not compete
		// with the selection accent.
		newRow: lipgloss.NewStyle().Foreground(sem.Success),

		// Selected "+Neu" row: bold to match selected-item convention.
		newRowSel: lipgloss.NewStyle().Foreground(sem.Success).Bold(true),

		// Footer hints: canonical FgMuted per §Hint format.
		footer: lipgloss.NewStyle().Foreground(p.FgMuted).Padding(0, theme.PadXS),

		// Empty-state "Keine Treffer.": dim — same as all other empty-state
		// renders across the codebase (projects/renderEmptyState, palette).
		noMatches: lipgloss.NewStyle().Foreground(p.FgMuted),
	}
}
