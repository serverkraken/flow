// Stats-Flow für das Worktime-Aktions-Menü. Aggregiert Sessions im
// vom User gewählten Range über StatsComputer und rendert das Ergebnis
// als Plain Text via domain.WriteStats — gleiche Format-Strecke wie
// `flow worktime stats`.

package worktime

import (
	"bytes"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/domain"
)

// statsPager — wie Export-Pager `less -S`. Stats ist Plain-Text mit
// Tabular-Spalten; less -S erhält die Spaltentreue.
const statsPager = "less -S"

// statsCmd computes a Stats summary for the parsed range expression
// and routes the rendered text through the picked output target.
// Empty expr falls back to "month" (matches `flow worktime stats`'s
// CLI default — so the menu's Stats action gives a useful answer
// even when the user just hits Enter on the empty range form).
func statsCmd(deps Deps, target outputTarget, rangeExpr string) tea.Cmd {
	return func() tea.Msg {
		if deps.Stats == nil || deps.Reader == nil {
			return menuActionDoneMsg{err: fmt.Errorf("stats deps nicht verdrahtet")}
		}
		if deps.Output == nil {
			return menuActionDoneMsg{err: fmt.Errorf("output port nicht verdrahtet")}
		}
		expr := rangeExpr
		if expr == "" {
			expr = "month"
		}
		rng, err := domain.ParseRange(deps.Clock.Now(), expr)
		if err != nil {
			return menuActionDoneMsg{err: fmt.Errorf("range »%s«: %w", expr, err)}
		}

		all, err := deps.Reader.History()
		if err != nil {
			return menuActionDoneMsg{err: fmt.Errorf("stats history: %w", err)}
		}
		records, st := aggregateForRange(deps, all, rng)
		_ = records // kept on the books in case the menu later wants a per-day drill

		var buf bytes.Buffer
		if err := domain.WriteStats(&buf, expr, st); err != nil {
			return menuActionDoneMsg{err: fmt.Errorf("stats render: %w", err)}
		}
		basename := fmt.Sprintf("worktime-stats-%s", sanitizeRangeForFilename(expr))
		return dispatchToTarget(deps.Output, target, buf.String(), basename, "txt", statsPager)
	}
}

// aggregateForRange filters the history records by rng (or returns
// everything when rng is the zero range) and computes the appropriate
// Stats. Mirrors the cli/worktime.go stats branch — the Range-aware
// AggregateRange is what makes "Tag-1, Tag-2, …, Tag-30 each missed"
// produce a -240h saldo instead of zero.
func aggregateForRange(deps Deps, all []domain.DayRecord, rng domain.Range) ([]domain.DayRecord, domain.Stats) {
	if rng.From.IsZero() && rng.To.IsZero() {
		return all, deps.Stats.Aggregate(all)
	}
	records := make([]domain.DayRecord, 0, len(all))
	for _, d := range all {
		if rng.ContainsDate(d.Date) {
			records = append(records, d)
		}
	}
	return records, deps.Stats.AggregateRange(records, rng.From, rng.To)
}
