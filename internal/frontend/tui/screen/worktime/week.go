package worktime

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/components/statusbar"
	uistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
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

	width  int
	height int

	week   []domain.WeekDay
	stats  domain.Stats
	cursor int
	loaded bool
	err    error

	// styles is a palette-bound cache for the render hot-path. Built
	// once at constructor — renderDayRow runs once per visible weekday
	// per frame, renderPace once per frame; previously every call
	// allocated 4-7 lipgloss.Style values.
	styles wocheStyles
}

// wocheStyles caches the palette-dependent lipgloss styles used by
// week.go. Built once per Model at newWoche().
type wocheStyles struct {
	name        lipgloss.Style // Fg + Width(3) — weekday short label
	date        lipgloss.Style // FgMuted + Width(6) — date column
	marker      lipgloss.Style // Sem().Accent — cursor bar
	emptyBar    lipgloss.Style // Sem().Border — placeholder bar (leerer Tag)
	dur         lipgloss.Style // Fg base — total duration (Bold added per-row)
	greenPace   lipgloss.Style // Sem().Success — hit-day pace dot
	dimPace     lipgloss.Style // FgMuted — miss-day pace dot
	runningPace lipgloss.Style // Sem().Active — today-running pace dot (spec 2026-05-13)
	behindPace  lipgloss.Style // Sem().Warning — "▼ im Rückstand" track marker

	// kinds maps DayOff Kind to its pre-built style. nil-kind (zero
	// value, default) falls back to Fg. Render loops use
	// w.styles.kindStyle(k) to avoid map miss on zero-value kinds.
	kinds        map[domain.Kind]lipgloss.Style
	kindFallback lipgloss.Style

	// Balance has three colourways selected by sign. Pre-built so
	// renderKPIs doesn't allocate per call.
	balPositive lipgloss.Style
	balZero     lipgloss.Style
	balNegative lipgloss.Style
}

func newWocheStyles(p theme.Palette) wocheStyles {
	sem := p.Sem()
	// Kind-Farben kommen aus dem einen kanonischen Mapping theme.KindColor.
	// Das tmux-Bar liest dieselben Hex-Werte via theme.StatusPaletteFor →
	// domain.KindStatusColor.
	kinds := map[domain.Kind]lipgloss.Style{
		domain.KindHoliday:  lipgloss.NewStyle().Foreground(theme.KindColor(p, domain.KindHoliday)),
		domain.KindVacation: lipgloss.NewStyle().Foreground(theme.KindColor(p, domain.KindVacation)),
		domain.KindSick:     lipgloss.NewStyle().Foreground(theme.KindColor(p, domain.KindSick)),
	}
	return wocheStyles{
		name:         lipgloss.NewStyle().Foreground(p.Fg).Width(theme.DayLabelWidth),
		date:         lipgloss.NewStyle().Foreground(p.FgMuted).Width(6),
		marker:       lipgloss.NewStyle().Foreground(sem.Accent),
		emptyBar:     lipgloss.NewStyle().Foreground(sem.Border),
		dur:          lipgloss.NewStyle().Foreground(p.Fg),
		greenPace:    lipgloss.NewStyle().Foreground(sem.Success),
		dimPace:      lipgloss.NewStyle().Foreground(p.FgMuted),
		runningPace:  lipgloss.NewStyle().Foreground(sem.Active),
		behindPace:   lipgloss.NewStyle().Foreground(sem.Warning),
		kinds:        kinds,
		kindFallback: lipgloss.NewStyle().Foreground(p.Fg),
		balPositive:  lipgloss.NewStyle().Foreground(sem.Success),
		balZero:      lipgloss.NewStyle().Foreground(p.FgMuted),
		balNegative:  lipgloss.NewStyle().Foreground(sem.Warning),
	}
}

// kindStyle returns the pre-built style for k. Unknown kinds get
// kindFallback (Fg) — matches the theme.KindColor fallback.
func (s wocheStyles) kindStyle(k domain.Kind) lipgloss.Style {
	if st, ok := s.kinds[k]; ok {
		return st
	}
	return s.kindFallback
}

func newWoche(p theme.Palette, deps Deps) woche {
	return woche{pal: p, deps: deps, styles: newWocheStyles(p)}
}

// StateCursor reports the focused weekday row for state persistence.
func (w woche) StateCursor() int { return w.cursor }

// Init kicks off the week + stats load.
func (w woche) Init() tea.Cmd { return w.loadCmd() }

func (w woche) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		w.width = msg.Width
		w.height = msg.Height
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

	case ChangedMsg:
		// Cross-tab mutation signal: a sibling tab edited / added a
		// session or day-off entry. Reload so the week strip + KPIs
		// reflect the new state when the user switches back.
		return w, w.loadCmd()

	case tea.KeyPressMsg:
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

func (w woche) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

func (w woche) View() tea.View { return tea.NewView(w.viewContent()) }

func (w woche) viewContent() string {
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

	// KW heading pins at the top, footer hints at the bottom. The day rows
	// plus the totals + kennzahlen block form the scrollable middle, with
	// the selected day as the windowing anchor — on a short terminal you
	// scroll from the days down into the summary while the week identity
	// (KW) and the hints stay put.
	header := []string{w.renderHeader(now), ""}

	mid := w.renderDays(inner, now)
	focus := w.cursor
	mid = append(mid, "")
	mid = append(mid, w.renderTotals(inner, now)...)
	mid = append(mid, "")
	mid = append(mid, picker.SectionHeader("kennzahlen", inner, w.pal))
	mid = append(mid, w.renderKPIs(now, inner))
	mid = append(mid, "  "+w.renderPace(now))

	footer := []string{"", renderFooterHints(w.pal, w.footerHints(), inner)}

	return fitHeight(header, mid, footer, focus, bodyBudget(w.height), w.pal)
}

func (w woche) renderHeader(now time.Time) string {
	monday := isoMonday(now)
	sunday := monday.AddDate(0, 0, 6)
	_, weekNum := monday.ISOWeek()
	left := theme.Heading(fmt.Sprintf("KW %d", weekNum), w.pal)
	right := stDim(w.pal, domain.FmtDateRangeDe(monday, sunday))
	return theme.Gap(theme.PadSM) + left + theme.Gap(theme.PadMD) + right
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

	name := w.styles.name.Render(domain.WeekdayShortDe(d.Date.Weekday()))
	date := w.styles.date.Render(fmt.Sprintf("%02d.%02d", d.Date.Day(), d.Date.Month()))
	marker := "  "
	if idx == w.cursor {
		marker = w.styles.marker.Render(picker.AccentBarRune) + " "
	}

	dayOff, isOff := w.deps.DayOffStore.Lookup(d.Date)

	switch {
	case isWeekend && total == 0:
		return marker + name + " " + date + "  " + stDim(w.pal, "Wochenende")

	case isOff && total == 0:
		label := w.styles.kindStyle(dayOff.Kind).Render(dayOff.Kind.LabelDe())
		suffix := ""
		if dayOff.Label != "" {
			suffix = "  " + stDim(w.pal, dayOff.Label)
		}
		return marker + name + " " + date + "  " + label + suffix

	case total == 0:
		emptyBar := w.styles.emptyBar.Render(strings.Repeat("─", barW))
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
		durStr := w.styles.dur.Bold(total >= d.Target).Render(formatDur(total))
		extra := ""
		if d.IsToday && d.Active != nil {
			extra += "  " + theme.Active(glyphs.Active, w.pal)
		}
		if total >= d.Target {
			extra += "  " + theme.Success(glyphs.Done, w.pal)
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
		theme.Strong(fmt.Sprintf("%3d%%", pct), w.pal)
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
	balStyle := w.styles.balZero
	switch {
	case balance > 0:
		balStyle = w.styles.balPositive
	case balance < 0:
		balStyle = w.styles.balNegative
	}
	bal := balStyle.Render(domain.FmtSignedDuration(balance))
	chips := []string{
		stDim(w.pal, fmt.Sprintf("Schnitt %s", formatDur(avg))),
		stDim(w.pal, fmt.Sprintf("Ziele %d/%d", w.stats.Hits, weekdays)),
		stDim(w.pal, "Saldo ") + bal,
	}
	return joinWrapped(chips, stDim(w.pal, "  ·  "), "  ", "  ", inner)
}

func (w woche) renderPace(now time.Time) string {
	// Cached styles — these used to be allocated per-call before round4.
	// Track-Marker (▲ auf Kurs / ▼ im Rückstand) und Ziel-Count brauchen
	// greenStyle/dimStyle/behindStyle direkt; die Dots gehen über
	// paceDotStyle(kind, dayOff) und cachen via w.styles auch.
	greenStyle := w.styles.greenPace
	dimStyle := w.styles.dimPace
	behindStyle := w.styles.behindPace

	dots := make([]string, 0, len(w.week))
	hits, expected, workdays := 0, 0, 0
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	for _, d := range w.week {
		isWeekend := d.Date.Weekday() == time.Saturday || d.Date.Weekday() == time.Sunday
		var dayOff *domain.DayOff
		if entry, isOff := w.deps.DayOffStore.Lookup(d.Date); isOff {
			dayOff = &entry
		}
		// Decision tree shared with the tmux status segment via
		// domain.PaceDotFor — same kind selection lands on both surfaces.
		// Style selection stays here because lipgloss styles are cached
		// in w.styles for the render hot path.
		kind := domain.PaceDotFor(d, now, dayOff)
		glyph := domain.PaceDotGlyph(kind)
		style := w.paceDotStyle(kind, dayOff)
		// Weekend-Skip ist hier am Renderer, nicht im PaceDotFor — die
		// week.renderPace-Surface zeigt Mo–Fr-Dots, das Stats-Akkumulat
		// braucht auch nur Werktage.
		if isWeekend {
			continue
		}
		dots = append(dots, style.Render(glyph))

		hit := kind == domain.PaceDotHit
		if dayOff == nil {
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
	track := dimStyle.Render(glyphs.BulletDot)
	switch {
	case expected == 0:
	case hits >= expected:
		track = greenStyle.Render(glyphs.Up + " auf Kurs")
	default:
		track = behindStyle.Render(glyphs.Down + " im Rückstand")
	}
	return strings.Join(dots, " ") + theme.Gap(theme.PadMD) + count + theme.Gap(theme.PadMD) + track
}

// paceDotStyle picks the cached lipgloss style for a pace-dot kind.
// Mirrors domain.paceDotStatusColor but returns a pre-built lipgloss
// style from w.styles so the render hot path doesn't allocate.
func (w woche) paceDotStyle(k domain.PaceDotKind, dayOff *domain.DayOff) lipgloss.Style {
	switch k {
	case domain.PaceDotDayOff:
		if dayOff == nil {
			return w.styles.dimPace
		}
		return w.styles.kindStyle(dayOff.Kind)
	case domain.PaceDotHit:
		return w.styles.greenPace
	case domain.PaceDotRunning:
		return w.styles.runningPace
	}
	return w.styles.dimPace
}

func (w woche) countWorkdays() int {
	n := 0
	for _, d := range w.week {
		isWeekend := d.Date.Weekday() == time.Saturday || d.Date.Weekday() == time.Sunday
		if isWeekend {
			continue
		}
		if _, isOff := w.deps.DayOffStore.Lookup(d.Date); isOff {
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
	return []string{
		"j/k → bewegen",
		"g/G → erste/letzte",
		": → aktionen",
		uistrings.HintHelp,
	}
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
