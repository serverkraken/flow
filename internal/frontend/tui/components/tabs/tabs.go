// Package tabs renders a horizontal tab strip — the abstraction
// behind the worktime "Heute · Woche · History · Frei" header that's
// hand-built today. Two visual variants:
//
//   - Underline (default) — minimal: labels in a row, accent-coloured
//     `─` underline below the active tab, FgDim labels for the rest.
//     Best when the strip sits inside a box that already carries the
//     visual weight.
//   - Pill — each tab is a rounded pill, active fills with Accent.
//     Best when the strip stands alone (e.g. a screen-level switcher).
//
// Active tab carries Bold + Accent foreground in addition to the
// variant-specific marker, so colourless / NO_COLOR rendering still
// distinguishes it (audit A11y-2).
//
// docs/design-system-audit.md §2.3.5.
package tabs

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// Item is one tab. Glyph is optional (0 = none); Badge appears as a
// trailing parenthesised count when non-empty (e.g. "PRs (3)").
type Item struct {
	Label    string
	Glyph    rune
	Badge    string
	Disabled bool
}

// Variant selects the visual style.
type Variant int

const (
	// Underline is the default minimal strip.
	Underline Variant = iota
	// Pill puts each tab in a rounded fill.
	Pill
)

// Render returns the styled tab strip. items defines the order and
// content; active is the index of the currently-active tab (out-of-
// range collapses to "no active tab"). width caps the strip length;
// width <= 0 means "natural width".
func Render(items []Item, active int, width int, variant Variant, p theme.Palette) string {
	if len(items) == 0 {
		return ""
	}
	switch variant {
	case Pill:
		return renderPill(items, active, p)
	default:
		return renderUnderline(items, active, width, p)
	}
}

func renderUnderline(items []Item, active int, width int, p theme.Palette) string {
	sem := p.Sem()
	rendered := make([]string, len(items))
	for i, it := range items {
		rendered[i] = formatLabel(it, i == active, p)
	}
	row := strings.Join(rendered, "  ")

	// Build the underline row: spaces under inactive tabs, `─`
	// segments under the active. Aligned by exact cell width of each
	// rendered label so the underline always sits beneath its tab.
	var ruler strings.Builder
	gap := strings.Repeat(" ", 2)
	for i, lbl := range rendered {
		if i > 0 {
			ruler.WriteString(gap)
		}
		w := lipgloss.Width(lbl)
		if i == active {
			ruler.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color(sem.Accent)).
				Render(strings.Repeat(glyphs.BoxHorizontal, w)))
		} else {
			ruler.WriteString(strings.Repeat(" ", w))
		}
	}
	out := row + "\n" + ruler.String()
	if width > 0 && lipgloss.Width(row) > width {
		// Caller's width budget exceeded — let the caller decide what
		// to truncate. We don't silently drop tabs; that would hide
		// state from the user.
		return out
	}
	return out
}

func renderPill(items []Item, active int, p theme.Palette) string {
	sem := p.Sem()
	rendered := make([]string, len(items))
	for i, it := range items {
		base := lipgloss.NewStyle().Padding(0, 2)
		if it.Disabled {
			rendered[i] = base.
				Foreground(lipgloss.Color(p.FgMuted)).
				Render(formatBare(it))
			continue
		}
		if i == active {
			rendered[i] = base.
				Foreground(lipgloss.Color(p.Bg)).
				Background(lipgloss.Color(sem.Accent)).
				Bold(true).
				Render(formatBare(it))
			continue
		}
		rendered[i] = base.
			Foreground(lipgloss.Color(p.FgDim)).
			Render(formatBare(it))
	}
	return strings.Join(rendered, " ")
}

// formatLabel produces "▶ Label (3)" for an active tab, "Label (3)"
// otherwise. Bold is applied to the active tab's label so the
// terminal shows the active tab even with all colour stripped.
func formatLabel(it Item, isActive bool, p theme.Palette) string {
	bare := formatBare(it)
	if it.Disabled {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(p.FgMuted)).Render(bare)
	}
	if isActive {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Sem().Accent)).
			Bold(true).
			Render(bare)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.FgDim)).Render(bare)
}

// formatBare assembles the raw label (no styling). Glyph and badge
// expansion is shared between the underline and pill variants.
func formatBare(it Item) string {
	var b strings.Builder
	if it.Glyph != 0 {
		b.WriteRune(it.Glyph)
		b.WriteRune(' ')
	}
	b.WriteString(it.Label)
	if it.Badge != "" {
		b.WriteString(" (")
		b.WriteString(it.Badge)
		b.WriteString(")")
	}
	return b.String()
}
