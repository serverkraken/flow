package worktime

// History month-grid mode — kalendarisches Monatsraster mit Heat-Cell-
// Glyphen pro Tag, plus Cursor-Status und Monatsaggregat. Plus die
// Tag-Cursor-Clamp-Helper, die bei Mode-Switches und Refreshes greifen.

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
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
		hdr += lipgloss.NewStyle().Foreground(h.pal.FgMuted).Render(fmt.Sprintf(" %-3s ", lbl))
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
	if dayOff, doh := h.deps.DayOffReader.Lookup(cursorDate); doh {
		status += "  ·  " + dayOff.Kind.LabelDe()
		if dayOff.Label != "" {
			status += " " + dayOff.Label
		}
	}
	return lipgloss.NewStyle().Foreground(h.pal.Sem().Accent).Render(status)
}

func (h history) renderMonthAggregate(monthRef, now time.Time) string {
	if h.monthStats.Days == 0 || monthRef.Year() != now.Year() || monthRef.Month() != now.Month() {
		return ""
	}
	balColor := h.pal.FgMuted
	switch {
	case h.monthStats.Overtime > 0:
		balColor = h.pal.Green
	case h.monthStats.Overtime < 0:
		balColor = h.pal.Yellow
	}
	bal := lipgloss.NewStyle().Foreground(balColor).
		Render(domain.FmtSignedDuration(h.monthStats.Overtime))
	return "   " + stDim(h.pal, fmt.Sprintf("Monat %s  ·  Ziele %d/%d  ·  Saldo ",
		formatDur(h.monthStats.Total), h.monthStats.Hits, h.monthStats.Workdays)) + bal
}

func (h history) renderMonthCell(day time.Time, inMonth bool, byKey map[string]domain.DayRecord, monthRef time.Time) string {
	if !inMonth {
		return "     "
	}
	rec, hasRec := byKey[day.Format("2006-01-02")]
	dayOff, isOff := h.deps.DayOffReader.Lookup(day)
	isCursor := day.Day() == h.monthCur && day.Month() == monthRef.Month() && day.Year() == monthRef.Year()
	isToday := sameDay(day, h.deps.Clock.Now())
	isWeekend := day.Weekday() == time.Saturday || day.Weekday() == time.Sunday

	glyph := glyphs.BulletDot
	color := h.pal.BgCode
	switch {
	case hasRec && rec.Target > 0:
		pct := float64(rec.Total) / float64(rec.Target)
		switch {
		case pct >= 1.5:
			glyph, color = glyphs.Up, h.pal.Red
		case pct >= 1.0:
			glyph, color = glyphs.HeatFull, h.pal.Green
		case pct >= 0.75:
			glyph, color = glyphs.HeatDark, h.pal.Green
		case pct >= 0.5:
			glyph, color = glyphs.HeatMedium, h.pal.Yellow
		case pct > 0:
			glyph, color = glyphs.HeatLight, h.pal.Yellow
		}
	case isOff:
		glyph = dayOffGlyph(dayOff.Kind)
		color = h.pal.Cyan
	case isWeekend:
		glyph, color = " ", h.pal.FgMuted
	}
	dayNum := fmt.Sprintf("%2d", day.Day())
	body := fmt.Sprintf(" %s %s", dayNum, glyph)
	st := lipgloss.NewStyle().Foreground(color)
	switch {
	case isCursor:
		st = lipgloss.NewStyle().Foreground(h.pal.Bg).Background(h.pal.Sem().Accent).Bold(true)
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
