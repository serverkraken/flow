package worktime

// History month-grid mode — kalendarisches Monatsraster mit Heat-Cell-
// Glyphen pro Tag, plus Cursor-Status und Monatsaggregat. Plus die
// Tag-Cursor-Clamp-Helper, die bei Mode-Switches und Refreshes greifen.

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func (h history) renderMonth(records []domain.DayRecord, inner int) string {
	// Monatsraster ist fix 7 Tage × 5 Char + 4 Char Prefix = 39 Char
	// Mindestbreite. Bei schmaleren Panes wird die rechte Spalte
	// geclippt — fallback auf Hinweis statt halbem Grid.
	if inner < 39 {
		return stDim(h.pal, "  Pane zu schmal für Monatsraster — `v` schaltet Ansicht um.")
	}
	now := h.deps.Clock.Now()
	monthRef := h.monthRef
	if monthRef.IsZero() {
		monthRef = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	}
	first := time.Date(monthRef.Year(), monthRef.Month(), 1, 0, 0, 0, 0, monthRef.Location())
	byKey := make(map[string]domain.DayRecord, len(records))
	for _, r := range records {
		byKey[r.Date.Format("2006-01-02")] = r
	}
	header := theme.Heading(fmt.Sprintf("  %s %d", domain.MonthShortDe(first.Month()), first.Year()), h.pal)
	lines := []string{header, "", h.renderMonthDayLabels()}
	lines = append(lines, h.renderMonthGridRows(first, byKey, monthRef)...)
	lines = append(lines, "", h.renderMonthCursorStatus(first, byKey))
	if extras := h.renderMonthAggregate(monthRef, now); extras != "" {
		lines = append(lines, "", extras)
	}
	return strings.Join(lines, "\n")
}

func (h history) renderMonthDayLabels() string {
	dayLabels := []string{"Mo", "Di", "Mi", "Do", "Fr", "Sa", "So"}
	hdr := "    "
	for _, lbl := range dayLabels {
		hdr += h.styles.dayLabelMuted.Render(fmt.Sprintf(" %-3s ", lbl))
	}
	return hdr
}

func (h history) renderMonthGridRows(first time.Time, byKey map[string]domain.DayRecord, monthRef time.Time) []string {
	wd := int(first.Weekday())
	if wd == 0 {
		wd = 7
	}
	gridStart := first.AddDate(0, 0, -(wd - 1))
	out := make([]string, 0, 6)
	for week := 0; week < 6; week++ {
		row := "    "
		anyInMonth := false
		for d := 0; d < 7; d++ {
			day := gridStart.AddDate(0, 0, week*7+d)
			inMonth := day.Month() == first.Month() && day.Year() == first.Year()
			if inMonth {
				anyInMonth = true
			}
			row += h.renderMonthCell(day, inMonth, byKey, monthRef)
		}
		if !anyInMonth && week > 0 {
			break
		}
		out = append(out, row)
	}
	return out
}

func (h history) renderMonthCursorStatus(first time.Time, byKey map[string]domain.DayRecord) string {
	last := first.AddDate(0, 1, -1)
	cursorDay := h.monthCur
	if cursorDay < 1 || cursorDay > last.Day() {
		cursorDay = 1
	}
	cursorDate := time.Date(first.Year(), first.Month(), cursorDay, 0, 0, 0, 0, first.Location())
	rec, hasRec := byKey[cursorDate.Format("2006-01-02")]
	var status string
	if hasRec {
		pct := 0
		if rec.Target > 0 {
			pct = int(rec.Total * 100 / rec.Target)
		}
		status = fmt.Sprintf("   %s  %s  %s / %s  ·  %d%%",
			domain.WeekdayShortDe(cursorDate.Weekday()), cursorDate.Format("2006-01-02"),
			formatDur(rec.Total), formatDur(rec.Target), pct)
	} else {
		status = fmt.Sprintf("   %s  %s  —",
			domain.WeekdayShortDe(cursorDate.Weekday()), cursorDate.Format("2006-01-02"))
	}
	if dayOff, doh := h.deps.DayOffStore.Lookup(cursorDate); doh {
		status += "  ·  " + dayOff.Kind.LabelDe()
		if dayOff.Label != "" {
			status += " " + dayOff.Label
		}
	}
	rendered := lipgloss.NewStyle().Foreground(h.pal.Sem().Accent).Render(status)
	if chip := h.attachedChip(cursorDate); chip != "" {
		rendered += chip
	}
	return rendered
}

func (h history) renderMonthAggregate(monthRef, now time.Time) string {
	if h.monthStats.Days == 0 || monthRef.Year() != now.Year() || monthRef.Month() != now.Month() {
		return ""
	}
	balStyle := h.styles.balZero
	switch {
	case h.monthStats.Overtime > 0:
		balStyle = h.styles.balPositive
	case h.monthStats.Overtime < 0:
		balStyle = h.styles.balNegative
	}
	bal := balStyle.Render(domain.FmtSignedDuration(h.monthStats.Overtime))
	return "   " + stDim(h.pal, fmt.Sprintf("Monat %s  ·  Ziele %d/%d  ·  Saldo ",
		formatDur(h.monthStats.Total), h.monthStats.Hits, h.monthStats.Workdays)) + bal
}

func (h history) renderMonthCell(day time.Time, inMonth bool, byKey map[string]domain.DayRecord, monthRef time.Time) string {
	if !inMonth {
		return "     "
	}
	rec, hasRec := byKey[day.Format("2006-01-02")]
	dayOff, isOff := h.deps.DayOffStore.Lookup(day)
	isCursor := day.Day() == h.monthCur && day.Month() == monthRef.Month() && day.Year() == monthRef.Year()
	isToday := sameDay(day, h.deps.Clock.Now())
	isWeekend := day.Weekday() == time.Saturday || day.Weekday() == time.Sunday

	sem := h.pal.Sem()
	glyph := glyphs.BulletDot
	var color lipgloss.TerminalColor = h.pal.BgCode
	switch {
	case hasRec && rec.Target > 0:
		pct := float64(rec.Total) / float64(rec.Target)
		switch {
		case pct >= 1.5:
			glyph, color = glyphs.Up, sem.Danger
		case pct >= 1.0:
			glyph, color = glyphs.HeatFull, sem.Success
		case pct >= 0.75:
			// "Auf Kurs, aber noch nicht angekommen" — Sem.Active statt
			// Success (siehe history_heatmap.heatmapCellGlyph für die
			// Begründung; Glyph + Farbe sind die identische Skala über
			// beide Surfaces).
			glyph, color = glyphs.HeatDark, sem.Active
		case pct >= 0.5:
			glyph, color = glyphs.HeatMedium, sem.Warning
		case pct > 0:
			glyph, color = glyphs.HeatLight, sem.Warning
		}
	case isOff:
		// Spec 2026-05-13: ● für day-off (cross-surface mit tmux + week).
		glyph = glyphs.Filled
		color = theme.KindColor(h.pal, dayOff.Kind)
	case isWeekend:
		glyph, color = " ", h.pal.FgMuted
	}
	dayNum := fmt.Sprintf("%2d", day.Day())
	body := fmt.Sprintf(" %s %s", dayNum, glyph)
	st := lipgloss.NewStyle().Foreground(color)
	switch {
	case isCursor:
		st = h.styles.cursorCell
	case isToday:
		st = st.Underline(true).Bold(true)
	}
	return st.Render(body) + " "
}

func monthClampDay(monthRef time.Time, day int) int {
	first := time.Date(monthRef.Year(), monthRef.Month(), 1, 0, 0, 0, 0, monthRef.Location())
	last := first.AddDate(0, 1, -1).Day()
	if day < 1 {
		return 1
	}
	if day > last {
		return last
	}
	return day
}
