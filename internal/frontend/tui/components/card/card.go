// Package card is the compact two-row header used in the markdown
// frontmatter card and the worktime "today" block. Layout:
//
//	[BADGE] Title              meta
//	────────────────────────────────  (optional, when Separator=true)
//	body line 1
//	body line 2
//
// The badge is pre-rendered (callers pass the styled string from a
// pill component or any custom builder), so card stays palette-agnostic
// for the badge's own colours and only owns the surrounding layout.
//
// docs/design-system-audit.md §2.3.6.
package card

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// Opts is the input for Render. Width 0 means "no width pinning"; the
// card will be as wide as the longest line. Body may contain newlines;
// it is rendered verbatim under the heading row.
type Opts struct {
	Badge     string // optional pre-styled badge (pill / chip)
	Title     string
	Meta      string // right-aligned next to the title; empty hides
	Body      string
	Width     int
	Separator bool // ─ rule between heading row and body
}

// Render returns the card as a multi-line string. Empty Opts returns
// "".
func Render(opts Opts, p theme.Palette) string {
	if opts.Title == "" && opts.Body == "" && opts.Badge == "" {
		return ""
	}

	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Sem().Highlight)).Bold(true)
	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.FgMuted)).Italic(true)
	sepStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Sem().BorderSubtle))

	headLeft := joinHorizontal(opts.Badge, titleStyle.Render(opts.Title))
	var head string
	if opts.Meta != "" && opts.Width > 0 {
		head = fillRight(headLeft, metaStyle.Render(opts.Meta), opts.Width)
	} else if opts.Meta != "" {
		head = headLeft + "  " + metaStyle.Render(opts.Meta)
	} else {
		head = headLeft
	}

	parts := []string{head}
	if opts.Separator {
		w := opts.Width
		if w <= 0 {
			w = lipgloss.Width(head)
		}
		parts = append(parts, sepStyle.Render(strings.Repeat("─", w)))
	}
	if opts.Body != "" {
		parts = append(parts, opts.Body)
	}
	return strings.Join(parts, "\n")
}

// joinHorizontal places left and right side by side with one space
// between them, skipping empty operands so a missing badge doesn't
// leave a stray leading space.
func joinHorizontal(left, right string) string {
	switch {
	case left == "":
		return right
	case right == "":
		return left
	default:
		return left + " " + right
	}
}

// fillRight returns left … right padded so the visible width equals
// width. When left+right already exceeds width, returns left+space+
// right untrimmed (clipping is the caller's concern — better to
// overflow visibly than truncate metadata silently).
func fillRight(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return left + " " + right
	}
	return left + strings.Repeat(" ", gap) + right
}
