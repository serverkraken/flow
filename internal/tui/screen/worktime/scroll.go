package worktime

import (
	"fmt"
	"strings"

	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
)

// bodyBudget is the row budget for the route body. The RouteHost already
// reserves its own 1-line keyhint footer, so the full Frame.Height is ours.
func bodyBudget(height int) int { return height }

// fitHeight lays out fixed header + scrollable mid + fixed footer within budget,
// windowing the mid region around `focus`. Ported from main screen/worktime/scroll.go,
// adapted to the rebuild (footer here = toast slot only).
func fitHeight(header, mid, footer []string, focus, budget int, pal theme.Palette) string {
	if budget <= 0 {
		return ""
	}
	if len(header) >= budget {
		return strings.Join(header[:budget], "\n")
	}
	rows := append([]string(nil), header...)
	remaining := budget - len(header)
	foot := footer
	if len(foot) >= remaining {
		foot = nil
	}
	midBudget := remaining - len(foot)
	rows = append(rows, windowRows(mid, focus, midBudget, pal)...)
	rows = append(rows, foot...)
	return strings.Join(rows, "\n")
}

// windowRows returns at most `budget` rows from mid, scrolled so `focus` is
// visible, with dim "▲ N darüber" / "▼ N darunter" overflow markers.
func windowRows(mid []string, focus, budget int, pal theme.Palette) []string {
	if budget <= 0 {
		return nil
	}
	if len(mid) <= budget {
		return mid
	}
	start := focus - budget/2
	if start < 0 {
		start = 0
	}
	if start+budget > len(mid) {
		start = len(mid) - budget
	}
	end := start + budget
	out := make([]string, 0, budget)
	if start > 0 {
		out = append(out, theme.Dim(fmt.Sprintf("  %s %d darüber", glyphs.Up, start), pal))
		out = append(out, mid[start+1:end]...)
	} else {
		out = append(out, mid[start:end]...)
	}
	if end < len(mid) {
		out[len(out)-1] = theme.Dim(fmt.Sprintf("  %s %d darunter", glyphs.Down, len(mid)-end+1), pal)
	}
	return out
}

// joinHints renders a "  ·  "-joined dim hint line (main renderFooterHints).
func joinHints(parts []string, pal theme.Palette) string {
	return theme.Dim("  "+strings.Join(parts, "  ·  "), pal)
}
