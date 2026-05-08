package worktime

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/components/statusbar"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// — messages —

type wocheLoadedMsg struct {
	week  []domain.WeekDay
	stats domain.Stats
	err   error
}

// woche is the Woche (current week) sub-model. F4.3 wave C scope: the
// per-day list with weekend / day-off / empty / has-data states, a week-
// total bar, the pace strip and the KPI strip (Schnitt, Ziele, Saldo).
// Cursor j/k/g/G navigates rows. Enter drill is deferred to wave D where
// it shares logic with the History sub-model; h/l week navigation is a
// post-wave-C enhancement.
type woche struct {
	pal  theme.Palette
	deps Deps

	width int

	week   []domain.WeekDay
	stats  domain.Stats
	cursor int
	loaded bool
	err    error
}

func newWoche(p theme.Palette, deps Deps) woche {
	return woche{pal: p, deps: deps}
}

// StateCursor reports the focused weekday row for state persistence.
func (w woche) StateCursor() int { return w.cursor }

// Init kicks off the week + stats load.
func (w woche) Init() tea.Cmd { return w.loadCmd() }

func (w woche) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		w.width = msg.Width
		return w, nil

	case wocheLoadedMsg:
		w.loaded = true
		w.err = msg.err
		if msg.week != nil {
			w.week = msg.week
			w.stats = msg.stats
			w.clampCursor()
		}
		return w, nil

	case dayRefreshMsg:
		return w, w.loadCmd()

	case tea.KeyMsg:
		return w.handleKey(msg)
	}
	return w, nil
}

func (w woche) loadCmd() tea.Cmd {
	reader := w.deps.Reader
	stats := w.deps.Stats
	clock := w.deps.Clock
	return func() tea.Msg {
		week, err := reader.Week()
		if err != nil {
			return wocheLoadedMsg{err: err}
		}
		s, sErr := stats.WeekStats(clock.Now())
		if sErr != nil {
			return wocheLoadedMsg{week: week, err: sErr}
		}
		return wocheLoadedMsg{week: week, stats: s}
	}
}

func (w *woche) clampCursor() {
	total := len(w.week)
	if w.cursor >= total {
		w.cursor = total - 1
	}
	if w.cursor < 0 {
		w.cursor = 0
	}
}

func (w woche) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	total := len(w.week)
	switch msg.String() {
	case "j", "down":
		if total > 0 {
			w.cursor = (w.cursor + 1) % total
		}
	case "k", "up":
		if total > 0 {
			w.cursor = (w.cursor + total - 1) % total
		}
	case "g":
		w.cursor = 0
	case "G":
		if total > 0 {
			w.cursor = total - 1
		}
	}
	return w, nil
}

// — render —

func (w woche) View() string {
	if w.width == 0 {
		return ""
	}
	if !w.loaded {
		return stDim(w.pal, "  Woche lädt …")
	}
	if w.err != nil {
		return stErr(w.pal, w.err.Error())
	}

	inner := w.width - 4
	now := w.deps.Clock.Now()

	rows := []string{w.renderHeader(now), ""}
	rows = append(rows, w.renderDays(inner, now)...)
	rows = append(rows, "")
	rows = append(rows, w.renderTotals(inner, now)...)
	rows = append(rows, "")
	rows = append(rows, picker.SectionHeader("kennzahlen", inner, w.pal))
	rows = append(rows, w.renderKPIs(now, inner))
	rows = append(rows, "  "+w.renderPace(now))
	rows = append(rows, "", renderFooterHints(w.pal, w.footerHints(), inner))
	return strings.Join(rows, "\n")
}

func (w woche) renderHeader(now time.Time) string {
	monday := isoMonday(now)
	sunday := monday.AddDate(0, 0, 6)
	_, weekNum := monday.ISOWeek()
	left := theme.Heading(fmt.Sprintf("KW %d", weekNum), w.pal)
	right := stDim(w.pal, fmt.Sprintf("%02d. %s – %02d. %s",
		monday.Day(), domain.MonthShortDe(monday.Month()),
		sunday.Day(), domain.MonthShortDe(sunday.Month())))
	return "  " + left + "   " + right
}

func (w woche) renderDays(inner int, now time.Time) []string {
	barW := 12
	if inner-30 < barW {
		barW = inner - 30
	}
	if barW < 4 {
		barW = 4
	}

	out := make([]string, 0, len(w.week))
	for i, d := range w.week {
		out = append(out, w.renderDayRow(i, d, barW, now))
	}
	return out
}

func (w woche) renderDayRow(idx int, d domain.WeekDay, barW int, now time.Time) string {
	total := d.Total(now)
	isWeekend := d.Date.Weekday() == time.Saturday || d.Date.Weekday() == time.Sunday

	name := lipgloss.NewStyle().Foreground(w.pal.Fg).Width(3).Render(domain.WeekdayShortDe(d.Date.Weekday()))
	date := lipgloss.NewStyle().Foreground(w.pal.FgMuted).Width(6).
		Render(fmt.Sprintf("%02d.%02d", d.Date.Day(), d.Date.Month()))
	marker := "  "
	if idx == w.cursor {
		marker = lipgloss.NewStyle().Foreground(w.pal.Sem().Accent).Render(picker.AccentBarRune) + " "
	}

	dayOff, isOff := w.deps.DayOffReader.Lookup(d.Date)

	switch {
	case isWeekend && total == 0:
		return marker + name + " " + date + "  " + stDim(w.pal, "Wochenende")

	case isOff && total == 0:
		label := lipgloss.NewStyle().Foreground(kindColor(w.pal, dayOff.Kind)).Render(dayOff.Kind.LabelDe())
		suffix := ""
		if dayOff.Label != "" {
			suffix = "  " + stDim(w.pal, dayOff.Label)
		}
		return marker + name + " " + date + "  " + label + suffix

	case total == 0:
		emptyBar := lipgloss.NewStyle().Foreground(w.pal.BgCode).Render(strings.Repeat("─", barW))
		todayMark := ""
		if d.IsToday {
			todayMark = "  " + stDim(w.pal, "heute")
		}
		return marker + name + " " + date + "  " + emptyBar + todayMark

	default:
		pct := 0
		if d.Target > 0 {
			pct = int(total * 100 / d.Target)
			if pct > 100 {
				pct = 100
			}
		}
		bar := statusbar.Bar(pct, barW, w.pal)
		pctStr := stDim(w.pal, fmt.Sprintf("%3d%%", pct))
		durStr := lipgloss.NewStyle().Foreground(w.pal.Fg).Bold(total >= d.Target).Render(formatDur(total))
		extra := ""
		if d.IsToday && d.Active != nil {
			extra += "  " + lipgloss.NewStyle().Foreground(w.pal.Green).Render("▶")
		}
		if total >= d.Target {
			extra += "  " + lipgloss.NewStyle().Foreground(w.pal.Green).Render("✓")
		}
		return marker + name + " " + date + "  " + bar + "  " + pctStr + "  " + durStr + extra
	}
}

func (w woche) renderTotals(inner int, now time.Time) []string {
	var weekTotal, weekTarget time.Duration
	for _, d := range w.week {
		weekTotal += d.Total(now)
		isWeekend := d.Date.Weekday() == time.Saturday || d.Date.Weekday() == time.Sunday
		if !isWeekend {
			weekTarget += d.Target
		}
	}
	pct := 0
	if weekTarget > 0 {
		pct = int(weekTotal * 100 / weekTarget)
		if pct > 100 {
			pct = 100
		}
	}
	barW := inner - 8
	if barW < 8 {
		barW = 8
	}
	totals := "  " + theme.Strong(formatDur(weekTotal), w.pal) +
		"  " + stDim(w.pal, "/ "+formatDur(weekTarget))
	bar := "  " + statusbar.Bar(pct, barW, w.pal) + "  " +
		theme.Heading(fmt.Sprintf("%3d%%", pct), w.pal)
	return []string{
		picker.SectionHeader("woche gesamt", inner, w.pal),
		totals,
		bar,
	}
}

func (w woche) renderKPIs(now time.Time, inner int) string {
	weekdays := w.countWorkdays()
	weekTotal, weekTarget := w.totals(now)
	avg := time.Duration(0)
	if weekdays > 0 {
		avg = weekTotal / time.Duration(weekdays)
	}
	balance := weekTotal - weekTarget
	balColor := w.pal.FgMuted
	switch {
	case balance > 0:
		balColor = w.pal.Green
	case balance < 0:
		balColor = w.pal.Yellow
	}
	bal := lipgloss.NewStyle().Foreground(balColor).Render(domain.FmtSignedDuration(balance))
	chips := []string{
		stDim(w.pal, fmt.Sprintf("Schnitt %s", formatDur(avg))),
		stDim(w.pal, fmt.Sprintf("Ziele %d/%d", w.stats.Hits, weekdays)),
		stDim(w.pal, "Saldo ") + bal,
	}
	return joinWrapped(chips, stDim(w.pal, "  ·  "), "  ", "  ", inner)
}

func (w woche) renderPace(now time.Time) string {
	greenStyle := lipgloss.NewStyle().Foreground(w.pal.Green)
	dimStyle := lipgloss.NewStyle().Foreground(w.pal.FgMuted)
	yellowStyle := lipgloss.NewStyle().Foreground(w.pal.Yellow)

	dots := make([]string, 0, len(w.week))
	hits, expected, workdays := 0, 0, 0
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	for _, d := range w.week {
		isWeekend := d.Date.Weekday() == time.Saturday || d.Date.Weekday() == time.Sunday
		dayOff, isOff := w.deps.DayOffReader.Lookup(d.Date)
		total := d.Total(now)
		hit := d.Target > 0 && total >= d.Target

		switch {
		case isOff && !isWeekend:
			dots = append(dots, lipgloss.NewStyle().Foreground(kindColor(w.pal, dayOff.Kind)).
				Render(dayOffPaceGlyph(dayOff.Kind)))
		case hit:
			dots = append(dots, greenStyle.Render("●"))
		case d.IsToday && d.Active != nil:
			dots = append(dots, yellowStyle.Render("●"))
		default:
			dots = append(dots, dimStyle.Render("○"))
		}

		if !isWeekend && !isOff {
			workdays++
			past := d.Date.Before(today)
			if past || (d.IsToday && hit) {
				expected++
			}
			if hit {
				hits++
			}
		}
	}

	count := dimStyle.Render(fmt.Sprintf("%d/%d Ziele", hits, workdays))
	track := dimStyle.Render("·")
	switch {
	case expected == 0:
	case hits >= expected:
		track = greenStyle.Render("▲ on track")
	default:
		track = yellowStyle.Render("▼ behind")
	}
	return strings.Join(dots, " ") + "   " + count + "   " + track
}

func (w woche) countWorkdays() int {
	n := 0
	for _, d := range w.week {
		isWeekend := d.Date.Weekday() == time.Saturday || d.Date.Weekday() == time.Sunday
		if isWeekend {
			continue
		}
		if _, isOff := w.deps.DayOffReader.Lookup(d.Date); isOff {
			continue
		}
		n++
	}
	return n
}

func (w woche) totals(now time.Time) (total, target time.Duration) {
	for _, d := range w.week {
		total += d.Total(now)
		isWeekend := d.Date.Weekday() == time.Saturday || d.Date.Weekday() == time.Sunday
		if !isWeekend {
			target += d.Target
		}
	}
	return total, target
}

// footerHints — Skill §Hint format: nur context-relevante Keys. Tab-
// Navigation (1/2/3/4) ist parent-level (worktime-Root), gehört nicht in
// den Screen-Footer; `:` öffnet das Aktions-Menü und ist auf jeder
// Worktime-Surface gleich.
func (w woche) footerHints() []string {
	return []string{"j/k → bewegen", "g/G → erste/letzte", ": → aktionen"}
}

// — small helpers (private to package) —

// isoMonday returns the Monday of the ISO week containing t (Sunday is
// treated as the previous week's tail, matching the legacy renderer).
func isoMonday(t time.Time) time.Time {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).
		AddDate(0, 0, -(wd - 1))
}

// kindColor maps a day-off kind to a palette color. Lives on the screen
// side because the mapping mixes a domain enum with frontend palette
// concerns. Wave E (Frei tab) reuses this when listing day-offs.
func kindColor(p theme.Palette, k domain.Kind) lipgloss.TerminalColor {
	switch k {
	case domain.KindHoliday:
		return p.Cyan
	case domain.KindVacation:
		return p.Green
	case domain.KindSick:
		return p.Yellow
	}
	return p.Fg
}

// dayOffPaceGlyph picks a per-kind monospace glyph for the pace strip.
func dayOffPaceGlyph(k domain.Kind) string {
	switch k {
	case domain.KindHoliday:
		return "★"
	case domain.KindVacation:
		return "☼"
	case domain.KindSick:
		return "✚"
	}
	return "○"
}
