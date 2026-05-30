package worktime

// History heatmap mode — 26-Wochen-Raster (oder weniger bei schmaler
// Pane), pro Tag eine Heat-Cell mit Glyph + Farbe nach %-Erreichung.
// Plus die Bounds- und Cursor-Helper, die der Mode-Cycle und die Key-
// Handler aufrufen.

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
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
		// Jahres-Grenze ist Identity/Highlight, kein Active —
		// Skill §Color semantics: Cyan ist „live/running", nicht
		// „diese Spalte markiert was Besonderes". Purple/Highlight
		// trägt die Kalender-Boundary ohne mit den Running-Dots
		// zu konkurrieren. Beide Styles sind gecacht in h.styles.
		st := h.styles.headerWeekNum
		if prevYear != -1 && yr != prevYear {
			st = h.styles.headerYearChange
		}
		header += st.Render(fmt.Sprintf("%2d ", wn%100))
		prevYear = yr
	}
	return header
}

func (h history) renderHeatmapRows(byKey map[string]domain.DayRecord, startMon time.Time, weeks int) []string {
	dayLabels := []string{"Mo", "Di", "Mi", "Do", "Fr", "Sa", "So"}
	now := h.deps.Clock.Now()
	out := make([]string, 0, 7)
	for d := 0; d < 7; d++ {
		row := "   " + h.styles.dayLabelFg.Render(dayLabels[d]) + "  "
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
	cell := " " + glyphs.BulletDot + " "
	var color color.Color = h.pal.Sem().Border
	if hasRec && rec.Target > 0 {
		cell, color = heatmapCellGlyph(h.pal, rec)
	} else if hasRec && rec.Total > 0 {
		// Erfasste Zeit ohne Tagesziel (z.B. target_sat=0) fiel sonst auf
		// die leere ·-Zelle und versteckte die Arbeit — Info-Punkt (Cyan ●)
		// wie im Monatsraster, ohne eine Ziel-Erfüllung zu behaupten.
		cell, color = " "+glyphs.Filled+" ", h.pal.Sem().Info
	}
	if dayOff, isOff := h.deps.DayOffStore.Lookup(day); isOff {
		if !hasRec || rec.Target == 0 {
			// Spec 2026-05-13: ● für day-off (cross-surface mit tmux + week).
			cell = " " + glyphs.Filled + " "
		}
		color = theme.KindColor(h.pal, dayOff.Kind)
	}
	cellStyle := lipgloss.NewStyle().Foreground(color)
	isCursor := w == h.heatCol && d == h.heatRow
	isToday := sameDay(day, now)
	switch {
	case isCursor:
		// Cursor cell: invert with the accent (gecachte cursorCell).
		// Combine the today-underline when the cursor sits on today so
		// the user still gets the "this is today" reinforcement instead
		// of an exclusive switch.
		cellStyle = h.styles.cursorCell
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
	// Focused-cell readout: Strong (Fg + bold), not Accent. The grid
	// already marks the selection with cursorCell; painting the whole
	// detail line in the interactive Accent diluted that token (UX-Review
	// M7 Accent-overuse). Bold keeps it legible as the focused readout.
	rendered := theme.Strong(status, h.pal)
	if chip := h.attachedChip(d); chip != "" {
		rendered += chip
	}
	return rendered
}

func (h history) renderHeatmapLegend(inner int) string {
	sem := h.pal.Sem()
	// Pace-Chips aus heatScale (low→high) generiert, damit Legende und
	// Zelle denselben Glyph + dieselbe Farbe je Bucket tragen. Der frühere
	// Text-Chip „_ heute (unterstrichen)" ist raus — er war der einzige
	// Glyph-lose Eintrag und brach den Rhythmus (Skill §Visual hierarchy).
	legend := []string{stDim(h.pal, glyphs.BulletDot+" leer")}
	for i := len(heatScale) - 1; i >= 0; i-- {
		s := heatScale[i]
		legend = append(legend,
			lipgloss.NewStyle().Foreground(s.color(h.pal)).Render(s.glyph+" "+s.label))
	}
	// Spec 2026-05-13-filled-dayoff-dots-supersede: Day-off legend uses
	// ● + Sem.Schedule/Highlight/Notice. Glyph + Farbe matchen die
	// heatmap-Zelle UND den tmux-Pace-Dot — cross-surface identity.
	legend = append(legend,
		lipgloss.NewStyle().Foreground(sem.Schedule).Render(glyphs.Filled+" Feiertag"),
		lipgloss.NewStyle().Foreground(sem.Highlight).Render(glyphs.Filled+" Urlaub"),
		lipgloss.NewStyle().Foreground(sem.Notice).Render(glyphs.Filled+" Krank"),
	)
	return joinWrapped(legend, "  ", "   ", "   ", inner)
}

// heatStep ties one density bucket to its glyph, legend label and
// semantic color. The cell renderer and the legend both iterate
// heatScale, so a bucket can never drift between the two surfaces —
// vorher waren ░/▒ in der Zelle Warning-gefärbt, in der Legende aber
// dim (A11y-1 Glyph↔Legende-Mismatch). Reihenfolge high→low.
// ▲ (≥150%) trägt Glyph + Farbe (A11y-2: nie nur Farbe); ▓ statt █ ab
// 75% hält Grün exklusiv für den Hit ≥100% (Skill §Color semantics).
type heatStep struct {
	minPct float64
	glyph  string
	label  string
	color  func(theme.Palette) color.Color
}

var heatScale = []heatStep{
	{1.5, glyphs.Up, "≥150%", func(p theme.Palette) color.Color { return p.Sem().Danger }},
	{1.0, glyphs.HeatFull, "Ziel", func(p theme.Palette) color.Color { return p.Sem().Success }},
	{0.75, glyphs.HeatDark, "<100%", func(p theme.Palette) color.Color { return p.Sem().Active }},
	// Behind-Pace eskaliert mit der Dichte: ▒ <75% mild (Warning/Gelb),
	// ░ <50% deutlich darunter (Notice/Orange — laut Sem „firmer than
	// Warning"). Vorher trugen beide Buckets Warning, der Gradient kollabierte
	// (UX-Review H5). Glyph ░ hält sie vom ●-Krank-Chip (auch Notice) getrennt.
	{0.5, glyphs.HeatMedium, "<75%", func(p theme.Palette) color.Color { return p.Sem().Warning }},
	{0, glyphs.HeatLight, "<50%", func(p theme.Palette) color.Color { return p.Sem().Notice }},
}

func heatmapCellGlyph(pal theme.Palette, rec domain.DayRecord) (string, color.Color) {
	pct := float64(rec.Total) / float64(rec.Target)
	for _, s := range heatScale {
		if pct > 0 && pct >= s.minPct {
			return " " + s.glyph + " ", s.color(pal)
		}
	}
	return " " + glyphs.BulletDot + " ", pal.Sem().Border
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
