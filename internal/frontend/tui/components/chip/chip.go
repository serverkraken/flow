// Package chip is a single-line coloured label — the generalisation
// of the per-tag chips that worktime, kompendium-browse, and the
// markdown frontmatter card all build by hand today.
//
// Two visual variants:
//
//   - Solid   — background carries the colour, foreground is the
//                palette's Bg so the label always reads against it.
//                Default; used for tags, ranks, status badges.
//   - Outline — border carries the colour, foreground matches; useful
//                where Solid would be too loud (e.g. a dense list of
//                chips next to prose).
//
// docs/design-system-audit.md §2.3.3.
package chip

import (
	"hash/fnv"

	"github.com/charmbracelet/lipgloss"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// Variant selects the chip presentation.
type Variant int

const (
	// Solid fills the background with Color, foreground = palette Bg.
	Solid Variant = iota
	// Outline draws a rounded border in Color, foreground = Color.
	Outline
)

// Opts is the input for Render. Color is a raw hex string ("#9ece6a")
// — usually pulled from theme.Palette via Hash() so a tag-set
// distributes evenly over the canonical TagPalette.
type Opts struct {
	Label   string
	Color   string
	Variant Variant
}

// Render returns the styled chip ready to drop into a row layout.
// Width is determined by Label + 1 cell of horizontal padding on
// each side; callers wanting a fixed-width pill use the pill
// component instead.
func Render(opts Opts, p theme.Palette) string {
	switch opts.Variant {
	case Outline:
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(opts.Color)).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(opts.Color)).
			Padding(0, 1).
			Render(opts.Label)
	default:
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Bg)).
			Background(lipgloss.Color(opts.Color)).
			Bold(true).
			Padding(0, 1).
			Render(opts.Label)
	}
}

// Hash maps an arbitrary string to a stable colour from palette via
// FNV-1a. A given input always picks the same slot, so the same tag
// reads as the same colour across surfaces (browse list, markdown
// card, status bar). Returns "" when palette is empty.
func Hash(s string, palette []string) string {
	if len(palette) == 0 {
		return ""
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return palette[int(h.Sum32()%uint32(len(palette)))]
}
