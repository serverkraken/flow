package week

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtfmt"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
	"github.com/serverkraken/flow/internal/tui/ui/statusbar"
)

// weekSummary holds the aggregated totals for the current week view.
type weekSummary struct {
	totalLogged, totalTarget int // minutes
	workdays, hits, expected int
}

// isWeekendDate reports whether the "2006-01-02" date falls on Sat/Sun.
func isWeekendDate(date string) bool {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		return false
	}
	wd := d.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}

// computeWeekSummary aggregates week totals and goal counters. Totals sum all
// days (day-off/weekend targets are already 0 from the server); workdays/hits/
// expected count only non-weekend, non-day-off days. "expected" = past workdays
// plus today if already hit, using the IsToday flag to avoid clock calls.
func computeWeekSummary(days []apiclient.WeekDay, offs map[string]apiclient.DayOff) weekSummary {
	var s weekSummary
	todayIdx := -1
	for i, d := range days {
		if d.IsToday {
			todayIdx = i
		}
	}
	for i, d := range days {
		s.totalLogged += d.LoggedMin
		s.totalTarget += d.TargetMin
		if isWeekendDate(d.Date) {
			continue
		}
		if _, off := offs[d.Date]; off {
			continue
		}
		s.workdays++
		hit := d.TargetMin > 0 && d.LoggedMin >= d.TargetMin
		if hit {
			s.hits++
		}
		past := todayIdx >= 0 && i < todayIdx
		if past || (i == todayIdx && hit) {
			s.expected++
		}
	}
	return s
}

// fgColor renders a string in an arbitrary theme.Color using lipgloss directly.
// theme has no generic Fg builder — builders cover named semantic roles only.
func fgColor(s string, c theme.Color) string {
	return lipgloss.NewStyle().Foreground(c).Render(s)
}

// renderSummary builds the WOCHE GESAMT + KENNZAHLEN block and returns it as a
// multi-line string ready to be appended to the View output.
func (r *Route) renderSummary(width int) string {
	s := computeWeekSummary(r.days, r.offs)
	pal := r.pal
	var b strings.Builder

	pct := 0
	if s.totalTarget > 0 {
		pct = s.totalLogged * 100 / s.totalTarget
	}
	barCells := width - 6
	if barCells < 10 {
		barCells = 10
	}
	b.WriteString("\n  " + theme.Dim("WOCHE GESAMT", pal) + "\n")
	fmt.Fprintf(&b, "  %s / %s\n",
		wtfmt.FormatMin(s.totalLogged), wtfmt.FormatMin(s.totalTarget))
	b.WriteString("  " + statusbar.BarColored(pct, barCells, pal.Sem().Accent, pal) + "\n")

	avg := 0
	if s.workdays > 0 {
		avg = s.totalLogged / s.workdays
	}
	saldo := s.totalLogged - s.totalTarget
	b.WriteString("\n  " + theme.Dim("KENNZAHLEN", pal) + "\n")
	fmt.Fprintf(&b, "  Schnitt %s  %s  Ziele %d/%d  %s  Saldo %s\n",
		wtfmt.FormatMin(avg), glyphs.BulletDot, s.hits, s.workdays,
		glyphs.BulletDot, wtfmt.FormatSaldo(saldo))
	b.WriteString("  " + r.renderPaceRow(s) + "\n")
	return b.String()
}

// renderPaceRow renders Mon–Fri pace dots + goal count + on-track marker.
func (r *Route) renderPaceRow(s weekSummary) string {
	pal := r.pal
	dots := make([]string, 0, len(r.days))
	for _, d := range r.days {
		if isWeekendDate(d.Date) {
			continue
		}
		var off *apiclient.DayOff
		if v, ok := r.offs[d.Date]; ok {
			off = &v
		}
		k := classifyPaceDot(d, off)
		dots = append(dots, fgColor(paceGlyph(k), paceColor(k, off, pal)))
	}
	count := theme.Dim(fmt.Sprintf("%d/%d Ziele", s.hits, s.workdays), pal)
	var track string
	switch {
	case s.expected == 0:
		track = theme.Dim(glyphs.BulletDot, pal)
	case s.hits >= s.expected:
		track = theme.Success(glyphs.Up+" auf Kurs", pal)
	default:
		track = theme.Warning(glyphs.Down+" im Rückstand", pal)
	}
	return strings.Join(dots, " ") + "   " + count + "   " + track
}
