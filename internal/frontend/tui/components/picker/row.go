// Package picker provides rendering primitives for filterable sectioned lists.
package picker

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	tuistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// AccentBarRune is the left-edge marker for the selected row in any
// picker-shaped list. Exported so other screens (palette etc.) can
// reuse the same glyph instead of redeclaring it locally. Sourced from
// the canonical glyph whitelist — the bar lives in exactly one place.
const AccentBarRune = glyphs.AccentBar

// Row renders a single list entry: an accent bar on the left, label on
// the left side, and a hint string right-aligned within width.
//
// width is the available inner width (excluding any outer box border).
// Selected rows show the accent bar (▎) and bold foreground; unselected
// rows show a plain space.
//
// Label and hint are truncated with "…" when their combined width would
// overflow the row (Bubbletea Golden Rule #2). The hint keeps full
// priority; the label loses width first because it is more often the
// long-tail field (note titles, action labels) while the hint is
// usually a short tag like "[deep]" or a key-bind preview. When the
// row is so narrow that even the hint wouldn't fit, the hint is
// dropped and the label gets all remaining space.
func Row(selected bool, label, hint string, width int, p theme.Palette) string {
	sem := p.Sem()
	bar := " "
	labelStyle := lipgloss.NewStyle().Foreground(p.Fg)
	if selected {
		bar = lipgloss.NewStyle().Foreground(sem.Accent).Render(AccentBarRune)
		labelStyle = lipgloss.NewStyle().Foreground(p.Fg).Bold(true)
	}
	hintStyle := lipgloss.NewStyle().Foreground(p.FgMuted)

	// Reserved cells: bar(1) + space(1) + min-gap(1) = 3.
	const reserved = 3
	hintW := lipgloss.Width(hint)
	maxLabel := width - reserved - hintW
	if maxLabel < 1 {
		// No room for label + hint: drop the hint to give the label
		// space. A hint-only row would lose the primary content.
		hint = ""
		hintW = 0
		maxLabel = width - reserved
		if maxLabel < 1 {
			maxLabel = 1
		}
	}
	label = tuistrings.Truncate(label, maxLabel)

	gap := width - 1 - lipgloss.Width(label) - hintW - 1
	if gap < 1 {
		gap = 1
	}
	return bar + " " + labelStyle.Render(label) + strings.Repeat(" ", gap) + hintStyle.Render(hint)
}

// RowWithMatchOpts holds options for RowWithMatch — adds Match (matched
// rune indices in Label) on top of the Row contract. A struct, not
// positional args, because the param list would cluster: Selected /
// Label / Hint / Width / Match is exactly the threshold where a named
// struct reads better at the call site.
type RowWithMatchOpts struct {
	Selected bool
	Label    string
	Hint     string
	Width    int
	Match    []int // rune indices in Label to render in match style
}

// RowWithMatch renders a picker row identical to Row, but applies a
// per-rune match emphasis (Sem.Accent + Bold) on the rune indices in
// Match. When Match is empty, the output is byte-identical to
// Row(opts.Selected, opts.Label, opts.Hint, opts.Width, p).
//
// Plain Row applies a single foreground style across the whole label,
// which would overwrite any inline accent codes — that's why palette
// and projects each carried their own per-rune implementation before
// this component existed.
func RowWithMatch(opts RowWithMatchOpts, p theme.Palette) string {
	if len(opts.Match) == 0 {
		return Row(opts.Selected, opts.Label, opts.Hint, opts.Width, p)
	}
	sem := p.Sem()
	bar := " "
	labelStyle := lipgloss.NewStyle().Foreground(p.Fg)
	matchStyle := lipgloss.NewStyle().Foreground(sem.Accent).Bold(true)
	if opts.Selected {
		bar = lipgloss.NewStyle().Foreground(sem.Accent).Render(AccentBarRune)
		labelStyle = labelStyle.Bold(true).Underline(true)
		matchStyle = matchStyle.Underline(true)
	}
	hi := make(map[int]bool, len(opts.Match))
	for _, idx := range opts.Match {
		hi[idx] = true
	}
	var b strings.Builder
	for i, r := range []rune(opts.Label) {
		if hi[i] {
			b.WriteString(matchStyle.Render(string(r)))
		} else {
			b.WriteString(labelStyle.Render(string(r)))
		}
	}
	rendered := b.String()
	hintStyle := lipgloss.NewStyle().Foreground(p.FgMuted)
	gap := opts.Width - 1 - lipgloss.Width(opts.Label) - lipgloss.Width(opts.Hint) - 1
	if gap < 1 {
		gap = 1
	}
	return bar + " " + rendered + strings.Repeat(" ", gap) + hintStyle.Render(opts.Hint)
}

// SectionHeader renders an uppercased section name with trailing dash fill.
// width is the available inner width.
func SectionHeader(name string, width int, p theme.Palette) string {
	style := lipgloss.NewStyle().Foreground(p.FgMuted).Bold(true).Padding(0, 0, 0, 1)
	rendered := style.Render(strings.ToUpper(name))
	dashStyle := lipgloss.NewStyle().Foreground(p.Sem().Border)

	gap := width - lipgloss.Width(rendered) - 1
	if gap < 0 {
		gap = 0
	}
	return rendered + " " + dashStyle.Render(strings.Repeat("─", gap))
}
