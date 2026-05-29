package worktime

// Height-aware body composition shared by all four tab sub-models. Before
// this, every tab joined its full row list with "\n" and ignored its
// forwarded height — on a terminal shorter than the content the altscreen
// scrolled the headline + tab strip off the top (the "Text verschwindet"
// bug). fitHeight pins the header + footer and windows the scrollable
// middle around the cursor, mirroring the palette / projects screens'
// maxVisible-offset pattern generalised to (header, mid, footer) segments.

import (
	"fmt"
	"strings"

	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// titleboxRows is the vertical budget the worktime root's titlebox frame
// consumes around every tab body (top border + bottom border). Each
// sub-model subtracts it from its forwarded height so the rendered body
// never overflows the visible area. Both launch paths agree on this: the
// standalone `flow worktime` program and the sidekick host both wrap the
// active tab body in exactly one titlebox.
const titleboxRows = 2

// budgetUnbounded is the row budget used before the first WindowSizeMsg
// arrives (height 0). It is effectively "no limit" so the first paint
// renders the whole body instead of being clipped to a stale/zero height.
const budgetUnbounded = 1 << 30

// bodyBudget converts a sub-model's forwarded height into the row budget
// its body may occupy: the height minus the titlebox frame the worktime
// root wraps around every tab. All four tab sub-models share this so the
// header-pin / footer-pin / mid-window math agrees across the screen.
func bodyBudget(height int) int {
	if height == 0 {
		return budgetUnbounded
	}
	return height - titleboxRows
}

// fitHeight composes a height-bounded tab body: header rows pinned at the
// top, footer rows pinned at the bottom, and mid rows windowed into the
// remaining space so the row at index focus stays visible (centered when
// mid overflows). Hidden mid rows are signalled by a dim "▲ N darüber" /
// "▼ N darunter" marker in the top/bottom slot.
//
// budget is the total row count the returned block must not exceed —
// callers pass h.height - titleboxRows. When everything already fits, mid
// is returned untouched (no markers, no padding) so tall terminals render
// exactly as before. Priority under extreme squeeze is header > footer >
// mid for the pinned blocks: the header carries the screen's identity
// anchor (date / headline) and is the last thing dropped.
func fitHeight(header, mid, footer []string, focus, budget int, pal theme.Palette) string {
	if budget <= 0 {
		return ""
	}
	// Normalise to one visible line per slice element. Some callers pass
	// pre-joined chips (renderFooterHints / joinWrapped, week KPIs) that
	// wrap to multiple lines on a narrow terminal; counting those as a
	// single row would let the body overshoot the height budget again.
	header = flattenLines(header)
	footer = flattenLines(footer)
	mid, focus = flattenLinesFocus(mid, focus)
	if len(header) >= budget {
		return strings.Join(header[:budget], "\n")
	}
	rows := append([]string(nil), header...)
	remaining := budget - len(header)

	// Pin the footer only while it leaves at least one row for the mid
	// block; on a tiny budget the focal content wins the last rows.
	foot := footer
	if len(foot) >= remaining {
		foot = nil
	}
	midBudget := remaining - len(foot)

	rows = append(rows, windowRows(mid, focus, midBudget, pal)...)
	rows = append(rows, foot...)
	return strings.Join(rows, "\n")
}

// flattenLines splits any element containing newlines into separate
// elements so each element is exactly one visible line — the invariant
// windowRows and the budget math rely on.
func flattenLines(rows []string) []string {
	multiline := false
	for _, r := range rows {
		if strings.IndexByte(r, '\n') >= 0 {
			multiline = true
			break
		}
	}
	if !multiline {
		return rows
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, strings.Split(r, "\n")...)
	}
	return out
}

// flattenLinesFocus is flattenLines for the mid slice, remapping focus
// (an index into the pre-flatten slice) to the flattened slice so the
// cursor row stays the windowing anchor after multi-line elements expand.
func flattenLinesFocus(rows []string, focus int) ([]string, int) {
	multiline := false
	for _, r := range rows {
		if strings.IndexByte(r, '\n') >= 0 {
			multiline = true
			break
		}
	}
	if !multiline {
		return rows, focus
	}
	out := make([]string, 0, len(rows))
	newFocus := 0
	for i, r := range rows {
		if i == focus {
			newFocus = len(out)
		}
		out = append(out, strings.Split(r, "\n")...)
	}
	return out, newFocus
}

// windowRows clamps mid to at most w rows, centered on focus. When mid
// overflows w it replaces the top/bottom slot with a dim ▲/▼ overflow
// marker so hidden rows are signalled. A budget below 3 is too tight for
// markers, so it returns the focus-centered slice raw. The focus row is
// never the slot a marker occupies (the centering keeps focus off both
// edges whenever a marker is present), so the cursor always stays visible.
func windowRows(mid []string, focus, w int, pal theme.Palette) []string {
	n := len(mid)
	if w <= 0 {
		return nil
	}
	if n <= w {
		return mid
	}
	if focus < 0 {
		focus = 0
	}
	if focus >= n {
		focus = n - 1
	}
	off := focus - w/2
	if off < 0 {
		off = 0
	}
	if off > n-w {
		off = n - w
	}
	win := make([]string, w)
	copy(win, mid[off:off+w])
	if w < 3 {
		return win
	}
	if off > 0 {
		win[0] = overflowMarker(glyphs.Up, off+1, "darüber", pal)
	}
	if off+w < n {
		win[w-1] = overflowMarker(glyphs.Down, n-(off+w)+1, "darunter", pal)
	}
	return win
}

// overflowMarker renders the dim "▲ N darüber" / "▼ N darunter" line that
// stands in for clipped mid rows. 2-cell indent matches the tab bodies'
// row indent so the marker lines up with the content it replaces.
func overflowMarker(glyph string, count int, dir string, pal theme.Palette) string {
	return theme.Dim(fmt.Sprintf("  %s %d %s", glyph, count, dir), pal)
}
