// Width helpers for the markdown renderer. Kept apart from the
// renderer itself because the wrap/indent primitives are reused by
// every block-renderer and tests want to exercise them in isolation.

package markdown

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
)

// wrapText reflows s to a maximum visual width.
// `cellbuf.Wrap` is style-AND-link aware: when it inserts a wrap
// boundary inside a styled span (inline `code` BG, link colour, …)
// it closes the SGR before the newline AND re-opens it on the next
// line so the styling carries across the wrap. The plain
// `ansi.Wrap` we used before only re-applied the FG colour, which
// dropped the BG of inline-code spans on continuation lines.
//
// Empty or non-positive width is a no-op pass-through.
func wrapText(s string, width int) string {
	if width <= 0 || s == "" {
		return s
	}
	return cellbuf.Wrap(s, width, " -")
}

// visibleWidth returns the visual width of s with ANSI sequences
// stripped. Single-line inputs only — for multi-line text take the
// max over lines.
func visibleWidth(s string) int {
	return ansi.StringWidth(s)
}
