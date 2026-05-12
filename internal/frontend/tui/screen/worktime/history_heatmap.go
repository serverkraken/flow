package worktime

// History heatmap mode — 26-Wochen-Raster (oder weniger bei schmaler
// Pane), pro Tag eine Heat-Cell mit Glyph + Farbe nach %-Erreichung.
// Plus die Bounds- und Cursor-Helper, die der Mode-Cycle und die Key-
// Handler aufrufen.

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func (h history) renderHeatmap(records []domain.DayRecord, inner int) string {
	if len(records) == 0 {
		return stDim(h.pal, "  Keine Treffer.")
	}
	byKey := make(map[string]domain.DayRecord, len(records))
	for _, r := range records {
		byKey[r.Date.Format("2006-01-02")] = r
	}
	startMon, weeks := h.heatmapBoundsFrom(records)
	if weeks == 0 {
		return stDim(h.pal, "  Pane zu schmal für Heatmap — `v` schaltet Ansicht um.")
	}
	lines := []string{h.renderHeatmapWeekHeader(startMon, weeks)}
	lines = append(lines, h.renderHeatmapRows(byKey, startMon, weeks)...)
	lines = append(lines, "")
	if status := h.renderHeatmapStatus(byKey); status != "" {
		lines = append(lines, status)
	}
	lines = append(lines, h.renderHeatmapLegend(inner))
	return strings.Join(lines, "\n")
}

func (h history) renderHeatmapWeekHeader(startMon time.Time, weeks int) string {
	header := "       "
	prevYear := -1
	for w := 0; w < weeks; w++ {
		mon := startMon.AddDate(0, 0, 7*w)
		yr, wn := mon.ISOWeek()
		col := h.pal.FgMuted
		if prevYear != -1 && yr != prevYear {
			col = h.pal.Sem().Active
		}
		header += lipgloss.NewStyle().Foreground(col).Render(fmt.Sprintf("%2d ", wn%100))
		prevYear = yr
	}
	return header
}

func (h history) renderHeatmapRows(byKey map[string]domain.DayRecord, startMon time.Time, weeks int) []string {
	dayLabels := []string{"Mo", "Di", "Mi", "Do", "Fr", "Sa", "So"}
	now := h.deps.Clock.Now()
	out := make([]string, 0, 7)
	for d := 0; d < 7; d++ {
		row := "   " + lipgloss.NewStyle().Foreground(h.pal.Fg).Width(3).Render(dayLabels[d]) + "  "
		for w := 0; w < weeks; w++ {
			day := startMon.AddDate(0, 0, 7*w+d)
			row += h.renderHeatmapCell(day, byKey, w, d, now)
		}
		out = append(out, row)
	}
	return out
}

func (h history) renderHeatmapCell(day time.Time, byKey map[string]domain.DayRecord, w, d int, now time.Time) string {
	rec, hasRec := byKey[day.Format("2006-01-02")]
	// Einheitlicher Empty-Glyph (· middle-dot) für Werktag und Wochenende —
	// vorher mischte `.` (baseline) für Werktag mit `·` (middle-dot) für
	// Wochenende, was die Spalten optisch unruhig wirken ließ.
	cell := " · "
	var color lipgloss.TerminalColor = h.pal.BgCode
	if hasRec && rec.Target > 0 {
		cell, color = heatmapCellGlyph(h.pal, rec)
	}
	if dayOff, isOff := h.deps.DayOffStore.Lookup(day); isOff {
		if !hasRec || rec.Target == 0 {
			cell = dayOffHeatmapGlyph(dayOff.Kind)
		}
		color = h.pal.Sem().Info
	}
	cellStyle := lipgloss.NewStyle().Foreground(color)
	isCursor := w == h.heatCol && d == h.heatRow
	isToday := sameDay(day, now)
	switch {
	case isCursor:
		// Cursor cell: invert with the accent. Combine the today-underline
		// when the cursor sits on today so the user still gets the
		// "this is today" reinforcement instead of an exclusive switch.
		cellStyle = lipgloss.NewStyle().Foreground(h.pal.Bg).Background(h.pal.Sem().Accent).Bold(true)
		if isToday {
			cellStyle = cellStyle.Underline(true)
		}
	case isToday:
		cellStyle = cellStyle.Underline(true).Bold(true)
	}
	return cellStyle.Render(cell)
}

func (h history) renderHeatmapStatus(byKey map[string]domain.DayRecord) string {
	d, ok := h.heatmapDateAt(h.heatCol, h.heatRow)
	if !ok {
		return ""
	}
	var status string
	if rec, hit := byKey[d.Format("2006-01-02")]; hit {
		status = fmt.Sprintf("   %s  %s  %s / %s",
			domain.WeekdayShortDe(d.Weekday()), d.Format("2006-01-02"),
			formatDur(rec.Total), formatDur(rec.Target))
	} else {
		status = fmt.Sprintf("   %s  %s  —",
			domain.WeekdayShortDe(d.Weekday()), d.Format("2006-01-02"))
	}
	if dayOff, doh := h.deps.DayOffStore.Lookup(d); doh {
		status += "  ·  " + dayOff.Kind.LabelDe()
		if dayOff.Label != "" {
			status += " " + dayOff.Label
		}
	}
	rendered := lipgloss.NewStyle().Foreground(h.pal.Sem().Accent).Render(status)
	if chip := h.attachedChip(d); chip != "" {
		rendered += chip
	}
	return rendered
}

func (h history) renderHeatmapLegend(inner int) string {
	sem := h.pal.Sem()
	legend := []string{
		stDim(h.pal, "· leer"),
		stDim(h.pal, "░ <50%"),
		stDim(h.pal, "▒ <75%"),
		stDim(h.pal, "▓ <100%"),
		lipgloss.NewStyle().Foreground(sem.Success).Render("█ Ziel"),
		lipgloss.NewStyle().Foreground(sem.Danger).Render("▲ ≥150%"),
		lipgloss.NewStyle().Foreground(sem.Info).Render(glyphs.Holiday + "/" + glyphs.Vacation + "/" + glyphs.Extra + " frei"),
		// Heute-Marker erklärt: Underline auf der Heatmap-Zelle = aktueller
		// Tag. Statt eine zusätzliche unterstrichene Demo-Zelle (kostete einen
		// Inline-Style über das §2.6-Budget hinaus) reicht der Text-Hint —
		// die Heatmap selbst trägt die Underline-Semantik.
		stDim(h.pal, "_ heute (unterstrichen)"),
	}
	return joinWrapped(legend, "  ", "   ", "   ", inner)
}

func heatmapCellGlyph(pal theme.Palette, rec domain.DayRecord) (string, lipgloss.TerminalColor) {
	sem := pal.Sem()
	pct := float64(rec.Total) / float64(rec.Target)
	switch {
	case pct >= 1.5:
		// ≥150% bekommt einen distinkten Glyph (▲ = "Up" aus dem Whitelist),
		// damit die A11y-2-Regel hält: Glyph + Farbe, niemals nur Farbe.
		return " ▲ ", sem.Danger
	case pct >= 1.0:
		return " █ ", sem.Success
	case pct >= 0.75:
		return " ▓ ", sem.Success
	case pct >= 0.5:
		return " ▒ ", sem.Warning
	case pct > 0:
		return " ░ ", sem.Warning
	}
	return " · ", pal.BgCode
}

// dayOffHeatmapGlyph wraps dayOffGlyph mit den Heatmap-Cell-Spaces
// (jede Heat-Zelle ist 3 Char breit). Zentralisierter Single-Cell-
// Glyph kommt aus helpers.dayOffGlyph; die Spaces bleiben hier, weil
// nur die Heatmap pro Cell-Position drei Chars verlangt.
func dayOffHeatmapGlyph(k domain.Kind) string {
	return " " + dayOffGlyph(k) + " "
}

// — heatmap bounds + cursor helpers —

func (h history) heatmapBoundsFrom(records []domain.DayRecord) (time.Time, int) {
	if len(records) == 0 {
		return time.Time{}, 0
	}
	newest := records[0].Date
	oldest := records[len(records)-1].Date
	if h.heatOffsetWeeks != 0 {
		shifted := newest.AddDate(0, 0, 7*h.heatOffsetWeeks)
		if shifted.After(newest) {
			shifted = newest
		}
		minEdge := isoMonday(oldest)
		if shifted.Before(minEdge) {
			shifted = minEdge
		}
		newest = shifted
	}
	endMon := isoMonday(newest)
	startMon := isoMonday(oldest)
	if startMon.After(endMon) {
		startMon = endMon
	}
	weeks := int(endMon.Sub(startMon).Hours()/24/7) + 1
	maxByWidth := h.heatmapMaxWeeks()
	if maxByWidth > 0 && weeks > maxByWidth {
		weeks = maxByWidth
		startMon = endMon.AddDate(0, 0, -7*(weeks-1))
	}
	if weeks > 26 {
		weeks = 26
		startMon = endMon.AddDate(0, 0, -7*(weeks-1))
	}
	return startMon, weeks
}

// heatmapMaxWeeks gibt zurück, wie viele Wochen-Spalten in der
// aktuellen Pane-Breite tatsächlich nebeneinander passen. Vorher hatte
// die Heatmap einen Hard-Cap auf 26 Wochen, was bei Sidekick-Panes mit
// < 85 Spalten den Header über die Titlebox-Border drückte.
func (h history) heatmapMaxWeeks() int {
	if h.width == 0 {
		return 26
	}
	inner := h.width - 4
	avail := inner - 7
	if avail < 3 {
		return 0
	}
	n := avail / 3
	if n > 26 {
		n = 26
	}
	return n
}

func (h history) heatmapWeeks() int {
	records := filteredHistory(h.records, h.histQuery, h.deps.Clock.Now())
	_, weeks := h.heatmapBoundsFrom(records)
	return weeks
}

func (h history) heatmapDateAt(col, row int) (time.Time, bool) {
	records := filteredHistory(h.records, h.histQuery, h.deps.Clock.Now())
	startMon, weeks := h.heatmapBoundsFrom(records)
	if weeks == 0 || col < 0 || col >= weeks || row < 0 || row > 6 {
		return time.Time{}, false
	}
	return startMon.AddDate(0, 0, 7*col+row), true
}

func (h history) heatmapTodayCell() (int, int) {
	now := h.deps.Clock.Now()
	records := filteredHistory(h.records, h.histQuery, now)
	startMon, weeks := h.heatmapBoundsFrom(records)
	if weeks == 0 {
		return 0, 0
	}
	row := int(now.Weekday()) - 1
	if row < 0 {
		row = 6
	}
	mon := isoMonday(now)
	col := int(mon.Sub(startMon).Hours() / 24 / 7)
	if col < 0 {
		col = 0
	}
	if col >= weeks {
		col = weeks - 1
	}
	return col, row
}

func (h history) heatmapCellFor(d time.Time) (int, int) {
	records := filteredHistory(h.records, h.histQuery, h.deps.Clock.Now())
	startMon, weeks := h.heatmapBoundsFrom(records)
	if weeks == 0 {
		return 0, 0
	}
	row := int(d.Weekday()) - 1
	if row < 0 {
		row = 6
	}
	mon := isoMonday(d)
	col := int(mon.Sub(startMon).Hours() / 24 / 7)
	if col < 0 {
		col = 0
	}
	if col >= weeks {
		col = weeks - 1
	}
	return col, row
}
