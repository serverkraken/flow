package worktime

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/form"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/components/statusbar"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
)

// — messages —

type historyLoadedMsg struct {
	records    []domain.DayRecord
	monthStats domain.Stats
	topTags    []string
	err        error
}

type historyDrillLoadedMsg struct {
	date     time.Time
	sessions []domain.Session
	err      error
}

// — modes —

type historyMode int

const (
	historyModeList historyMode = iota
	historyModeHeatmap
	historyModeTagClock
	historyModeMonth
)

func (m historyMode) label() string {
	switch m {
	case historyModeHeatmap:
		return "heatmap"
	case historyModeTagClock:
		return "tagclock"
	case historyModeMonth:
		return "month"
	}
	return "list"
}

type historyDialog int

const (
	historyDialogNone historyDialog = iota
	historyDialogFilter
	historyDialogDrill
)

// history is the History tab sub-model. It owns four render sub-modes
// (list / heatmap / tag-clock / month) plus a filter dialog and a
// read-only day-detail drill. Edit/delete on past-day sessions from the
// drill is deferred to a post-wave-D enhancement — it shares the lock-
// guarded SessionWriter surface with the Heute dialogs and lands once
// wave F migrates the keymap-sync tests onto the new path.
type history struct {
	pal  theme.Palette
	deps Deps

	width int

	records    []domain.DayRecord
	monthStats domain.Stats
	topTags    []string
	loaded     bool
	err        error

	mode    historyMode
	listCur int

	heatCol         int
	heatRow         int
	heatOffsetWeeks int

	tagClockCol int
	tagClockRow int

	monthRef time.Time
	monthCur int

	histQuery string

	dialog historyDialog
	input  textinput.Model
	errMsg string

	drillDate     time.Time
	drillSessions []domain.Session
	drillCur      int
	drillErr      error
}

func newHistory(p theme.Palette, deps Deps) history {
	return history{pal: p, deps: deps}
}

// — capability interfaces —

func (h history) FilterActive() bool { return h.dialog != historyDialogNone }

func (h history) StateFilter() string {
	if h.histQuery != "" {
		return h.histQuery
	}
	return h.mode.label()
}

func (h history) StateCursor() int {
	switch h.mode {
	case historyModeHeatmap:
		return h.heatCol*7 + h.heatRow
	case historyModeTagClock:
		return h.tagClockRow*24 + h.tagClockCol
	case historyModeMonth:
		return h.monthCur
	}
	return h.listCur
}

// Init kicks off the history + month-stats + top-tags load.
func (h history) Init() tea.Cmd { return h.loadCmd() }

func (h history) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.width = msg.Width
		return h, nil

	case historyLoadedMsg:
		h.loaded = true
		h.err = msg.err
		if msg.err == nil {
			h.records = msg.records
			h.monthStats = msg.monthStats
			h.topTags = msg.topTags
			h.clampCursors()
		}
		return h, nil

	case historyDrillLoadedMsg:
		h.drillDate = msg.date
		h.drillErr = msg.err
		h.drillSessions = msg.sessions
		h.drillCur = 0
		return h, nil

	case dayRefreshMsg:
		return h, h.loadCmd()

	case tea.KeyMsg:
		return h.handleKey(msg)
	}
	return h, nil
}

func (h history) loadCmd() tea.Cmd {
	deps := h.deps
	return func() tea.Msg {
		records, err := deps.Reader.History()
		if err != nil {
			return historyLoadedMsg{err: err}
		}
		var monthStats domain.Stats
		if ms, sErr := deps.Stats.MonthStats(deps.Clock.Now()); sErr == nil {
			monthStats = ms
		}
		topTags, _ := deps.Tagger.TopUsage(8)
		return historyLoadedMsg{records: records, monthStats: monthStats, topTags: topTags}
	}
}

func (h history) drillLoadCmd(date time.Time) tea.Cmd {
	reader := h.deps.Reader
	from := startOfDay(date)
	return func() tea.Msg {
		sessions, err := reader.Range(domain.Range{From: from, To: from.AddDate(0, 0, 1)})
		if err != nil {
			return historyDrillLoadedMsg{date: from, err: err}
		}
		return historyDrillLoadedMsg{date: from, sessions: sessions}
	}
}

func (h *history) clampCursors() {
	records := filteredHistory(h.records, h.histQuery, h.deps.Clock.Now())
	if n := len(records); n > 0 {
		if h.listCur >= n {
			h.listCur = n - 1
		}
		if h.listCur < 0 {
			h.listCur = 0
		}
	} else {
		h.listCur = 0
	}
	if h.monthRef.IsZero() {
		now := h.deps.Clock.Now()
		h.monthRef = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		h.monthCur = now.Day()
	}
}

// — keymap dispatch —

func (h history) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if h.dialog == historyDialogFilter {
		return h.handleFilterKey(msg)
	}
	if h.dialog == historyDialogDrill {
		return h.handleDrillKey(msg)
	}
	switch h.mode {
	case historyModeMonth:
		return h.handleMonthKey(msg)
	case historyModeTagClock:
		return h.handleTagClockKey(msg)
	case historyModeHeatmap:
		return h.handleHeatmapKey(msg)
	default:
		return h.handleListKey(msg)
	}
}

func (h history) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	records := filteredHistory(h.records, h.histQuery, h.deps.Clock.Now())
	switch msg.String() {
	case "j", "down":
		if n := len(records); n > 0 {
			h.listCur = (h.listCur + 1) % n
		}
	case "k", "up":
		if n := len(records); n > 0 {
			h.listCur = (h.listCur + n - 1) % n
		}
	case "g":
		h.listCur = 0
	case "G":
		if n := len(records); n > 0 {
			h.listCur = n - 1
		}
	case "v":
		h.mode = historyModeHeatmap
		if h.listCur >= 0 && h.listCur < len(records) {
			h.heatCol, h.heatRow = h.heatmapCellFor(records[h.listCur].Date)
		} else {
			h.heatCol, h.heatRow = h.heatmapTodayCell()
		}
	case "/":
		return h.openFilter("")
	case "F":
		return h.openFilter("tag:")
	case "[":
		return h.stepFilter(-1)
	case "]":
		return h.stepFilter(+1)
	case "T":
		h.histQuery = ""
		h.listCur = 0
	case "enter":
		if h.listCur >= 0 && h.listCur < len(records) {
			return h.openDrill(records[h.listCur].Date)
		}
	}
	return h, nil
}

func (h history) handleHeatmapKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "h", "left":
		if h.heatCol > 0 {
			h.heatCol--
		}
	case "l", "right":
		weeks := h.heatmapWeeks()
		if h.heatCol < weeks-1 {
			h.heatCol++
		}
	case "j", "down":
		if h.heatRow < 6 {
			h.heatRow++
		}
	case "k", "up":
		if h.heatRow > 0 {
			h.heatRow--
		}
	case "enter":
		if d, ok := h.heatmapDateAt(h.heatCol, h.heatRow); ok {
			return h.openDrill(d)
		}
	case "[":
		h.heatOffsetWeeks -= 13
		h.heatCol, h.heatRow = h.heatmapTodayCell()
	case "]":
		h.heatOffsetWeeks += 13
		if h.heatOffsetWeeks > 0 {
			h.heatOffsetWeeks = 0
		}
		h.heatCol, h.heatRow = h.heatmapTodayCell()
	case "T":
		h.heatOffsetWeeks = 0
		h.heatCol, h.heatRow = h.heatmapTodayCell()
	case "v":
		h.mode = historyModeTagClock
		row := int(h.deps.Clock.Now().Weekday()) - 1
		if row < 0 {
			row = 6
		}
		h.tagClockRow = row
		h.tagClockCol = h.deps.Clock.Now().Hour()
	case "/":
		return h.openFilter("")
	case "F":
		return h.openFilter("tag:")
	}
	return h, nil
}

func (h history) handleTagClockKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "h", "left":
		h.tagClockCol = (h.tagClockCol + 23) % 24
	case "l", "right":
		h.tagClockCol = (h.tagClockCol + 1) % 24
	case "j", "down":
		h.tagClockRow = (h.tagClockRow + 1) % 7
	case "k", "up":
		h.tagClockRow = (h.tagClockRow + 6) % 7
	case "T":
		row := int(h.deps.Clock.Now().Weekday()) - 1
		if row < 0 {
			row = 6
		}
		h.tagClockRow = row
		h.tagClockCol = h.deps.Clock.Now().Hour()
	case "v":
		h.mode = historyModeMonth
		now := h.deps.Clock.Now()
		anchor := now
		records := filteredHistory(h.records, h.histQuery, now)
		if h.listCur >= 0 && h.listCur < len(records) {
			anchor = records[h.listCur].Date
		}
		h.monthRef = time.Date(anchor.Year(), anchor.Month(), 1, 0, 0, 0, 0, anchor.Location())
		h.monthCur = anchor.Day()
	case "/":
		return h.openFilter("")
	case "F":
		return h.openFilter("tag:")
	}
	return h, nil
}

func (h history) handleMonthKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if h.monthRef.IsZero() {
		now := h.deps.Clock.Now()
		h.monthRef = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		h.monthCur = now.Day()
	}
	switch msg.String() {
	case "h", "left":
		h.monthCur = monthClampDay(h.monthRef, h.monthCur-1)
	case "l", "right":
		h.monthCur = monthClampDay(h.monthRef, h.monthCur+1)
	case "j", "down":
		h.monthCur = monthClampDay(h.monthRef, h.monthCur+7)
	case "k", "up":
		h.monthCur = monthClampDay(h.monthRef, h.monthCur-7)
	case "[":
		h.monthRef = h.monthRef.AddDate(0, -1, 0)
		h.monthCur = monthClampDay(h.monthRef, h.monthCur)
	case "]":
		now := h.deps.Clock.Now()
		next := h.monthRef.AddDate(0, 1, 0)
		curMon := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		if !next.After(curMon) {
			h.monthRef = next
			h.monthCur = monthClampDay(h.monthRef, h.monthCur)
		}
	case "T":
		now := h.deps.Clock.Now()
		h.monthRef = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		h.monthCur = now.Day()
	case "enter":
		d := time.Date(h.monthRef.Year(), h.monthRef.Month(), h.monthCur, 0, 0, 0, 0, h.monthRef.Location())
		return h.openDrill(d)
	case "v":
		h.mode = historyModeList
	case "/":
		return h.openFilter("")
	case "F":
		return h.openFilter("tag:")
	}
	return h, nil
}

// — filter dialog —

func (h history) openFilter(seed string) (tea.Model, tea.Cmd) {
	h.dialog = historyDialogFilter
	h.input = form.NewTextInput("KWxx · YYYY · YYYY-MM · tag:foo · note:bar", h.pal)
	if seed != "" {
		h.input.SetValue(seed)
		h.input.CursorEnd()
	} else {
		h.input.SetValue(h.histQuery)
		h.input.CursorEnd()
	}
	h.input.Focus()
	h.errMsg = ""
	return h, textinput.Blink
}

func (h history) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		h.dialog = historyDialogNone
		h.input.Blur()
		h.input.SetValue("")
		h.errMsg = ""
		return h, nil
	case tea.KeyEnter:
		q := strings.TrimSpace(h.input.Value())
		if q != "" {
			if _, err := domain.ParseRange(h.deps.Clock.Now(), q); err != nil &&
				!isTagOrNote(q) && !isISOWeek(q) {
				h.errMsg = err.Error()
				return h, nil
			}
		}
		h.histQuery = q
		h.listCur = 0
		h.dialog = historyDialogNone
		h.input.Blur()
		h.input.SetValue("")
		h.errMsg = ""
		return h, nil
	}
	h.errMsg = ""
	var cmd tea.Cmd
	h.input, cmd = h.input.Update(msg)
	return h, cmd
}

func (h history) stepFilter(dir int) (tea.Model, tea.Cmd) {
	next, ok := stepHistFilter(h.histQuery, h.deps.Clock.Now(), dir)
	if !ok {
		return h, nil
	}
	h.histQuery = next
	h.listCur = 0
	return h, nil
}

// — day-detail drill (read-only for wave D) —

func (h history) openDrill(date time.Time) (tea.Model, tea.Cmd) {
	h.dialog = historyDialogDrill
	h.drillDate = startOfDay(date)
	h.drillCur = 0
	h.drillSessions = nil
	h.drillErr = nil
	return h, h.drillLoadCmd(h.drillDate)
}

func (h history) handleDrillKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "b":
		h.dialog = historyDialogNone
		h.drillSessions = nil
		return h, nil
	case "j", "down":
		if n := len(h.drillSessions); n > 0 {
			h.drillCur = (h.drillCur + 1) % n
		}
	case "k", "up":
		if n := len(h.drillSessions); n > 0 {
			h.drillCur = (h.drillCur + n - 1) % n
		}
	case "g":
		h.drillCur = 0
	case "G":
		if n := len(h.drillSessions); n > 0 {
			h.drillCur = n - 1
		}
	}
	return h, nil
}

// — render —

func (h history) View() string {
	if h.width == 0 {
		return ""
	}
	if h.dialog == historyDialogFilter {
		return h.renderFilterDialog()
	}
	if h.dialog == historyDialogDrill {
		return h.renderDrill()
	}
	if !h.loaded {
		return stDim(h.pal, "  History lädt …")
	}
	if h.err != nil {
		return stErr(h.pal, h.err.Error())
	}
	return h.renderMain()
}

func (h history) renderMain() string {
	inner := h.width - 4
	if inner <= 0 {
		inner = 80
	}
	records := filteredHistory(h.records, h.histQuery, h.deps.Clock.Now())
	rows := []string{h.renderHeader(records, inner), ""}
	switch h.mode {
	case historyModeHeatmap:
		rows = append(rows, h.renderHeatmap(records, inner))
	case historyModeTagClock:
		rows = append(rows, h.renderTagClock(records, inner))
	case historyModeMonth:
		rows = append(rows, h.renderMonth(records, inner))
	default:
		if len(records) == 0 {
			rows = append(rows, stDim(h.pal, "  Keine Treffer."))
		} else {
			rows = append(rows, h.renderList(records))
		}
	}
	rows = append(rows, "", renderFooterHints(h.pal, h.footerHints(), inner))
	return strings.Join(rows, "\n")
}

func (h history) renderHeader(records []domain.DayRecord, inner int) string {
	st := h.deps.Stats.Aggregate(records)
	if st.Days == 0 {
		filterChip := ""
		if h.histQuery != "" {
			filterChip = "  ·  " + lipgloss.NewStyle().Foreground(h.pal.Cyan).Render("filter: "+h.histQuery)
		}
		return stDim(h.pal, "  Keine Treffer.") + filterChip
	}
	balColor := h.pal.Dim
	switch {
	case st.Overtime > 0:
		balColor = h.pal.Green
	case st.Overtime < 0:
		balColor = h.pal.Yellow
	}
	bal := lipgloss.NewStyle().Foreground(balColor).Render(domain.FmtSignedDuration(st.Overtime))
	volume := []string{
		"Tage " + lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("%d", st.Days)),
		"Werktage " + fmt.Sprintf("%d", st.Workdays),
		"Total " + lipgloss.NewStyle().Bold(true).Render(formatDur(st.Total)),
		"Schnitt " + formatDur(st.Avg),
		"Max " + formatDur(st.Max),
		"Min " + formatDur(st.Min),
	}
	performance := []string{
		"Ziele " + fmt.Sprintf("%d/%d", st.Hits, st.Workdays),
		"Streak " + fmt.Sprintf("%d (best %d)", st.Streak, st.BestStreak),
		"Saldo " + bal,
	}
	header := joinWrapped(volume, "  ·  ", "  "+stDim(h.pal, "volumen:      "), "                ", inner) +
		"\n" +
		joinWrapped(performance, "  ·  ", "  "+stDim(h.pal, "performance:  "), "                ", inner)
	if h.histQuery != "" {
		header += "\n  " + stDim(h.pal, "filter: ") + lipgloss.NewStyle().Foreground(h.pal.Cyan).Render(h.histQuery)
	}
	return header
}

func (h history) renderList(records []domain.DayRecord) string {
	const barW = 12
	var lines []string
	prevWeek := -1
	prevYear := -1
	for i, rec := range records {
		y, w := rec.Date.ISOWeek()
		if w != prevWeek || y != prevYear {
			if prevWeek != -1 {
				lines = append(lines, "")
			}
			lines = append(lines, lipgloss.NewStyle().Foreground(h.pal.Accent).Bold(true).
				Render(fmt.Sprintf("  KW %d / %d", w, y)))
			prevWeek, prevYear = w, y
		}
		pct := 0
		if rec.Target > 0 {
			pct = int(rec.Total * 100 / rec.Target)
			if pct > 100 {
				pct = 100
			}
		}
		name := lipgloss.NewStyle().Foreground(h.pal.Fg).Width(3).
			Render(domain.WeekdayShortDe(rec.Date.Weekday()))
		date := lipgloss.NewStyle().Foreground(h.pal.Dim).Width(9).
			Render(fmt.Sprintf("%02d.%02d.%02d", rec.Date.Day(), rec.Date.Month(), rec.Date.Year()%100))
		bar := statusbar.Bar(pct, barW, h.pal)
		pctStr := stDim(h.pal, fmt.Sprintf("%3d%%", pct))
		durStr := lipgloss.NewStyle().Foreground(h.pal.Fg).Bold(rec.Total >= rec.Target).
			Render(formatDur(rec.Total))
		done := ""
		if rec.Total >= rec.Target {
			done = "  " + lipgloss.NewStyle().Foreground(h.pal.Green).Render("✓")
		}
		marker := "  "
		if i == h.listCur {
			marker = lipgloss.NewStyle().Foreground(h.pal.Accent).Render("▌ ")
		}
		lines = append(lines, marker+name+" "+date+"  "+bar+"  "+pctStr+"  "+durStr+done)
	}
	return strings.Join(lines, "\n")
}

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
		return ""
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
		col := h.pal.Dim
		if prevYear != -1 && yr != prevYear {
			col = h.pal.Cyan
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
	cell := " . "
	var color lipgloss.TerminalColor = h.pal.Border
	if isWeekendDate(day) {
		cell = " · "
	}
	if hasRec && rec.Target > 0 {
		cell, color = heatmapCellGlyph(h.pal, rec)
	}
	if dayOff, isOff := h.deps.DayOffReader.Lookup(day); isOff {
		if !hasRec || rec.Target == 0 {
			cell = dayOffHeatmapGlyph(dayOff.Kind)
		}
		color = h.pal.Cyan
	}
	cellStyle := lipgloss.NewStyle().Foreground(color)
	switch {
	case w == h.heatCol && d == h.heatRow:
		cellStyle = lipgloss.NewStyle().Foreground(h.pal.Bg).Background(h.pal.Accent).Bold(true)
	case sameDay(day, now):
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
	if dayOff, doh := h.deps.DayOffReader.Lookup(d); doh {
		status += "  ·  " + dayOff.Kind.LabelDe()
		if dayOff.Label != "" {
			status += " " + dayOff.Label
		}
	}
	return lipgloss.NewStyle().Foreground(h.pal.Accent).Render(status)
}

func (h history) renderHeatmapLegend(inner int) string {
	legend := []string{
		stDim(h.pal, ". leer"),
		stDim(h.pal, "░ <50%"),
		stDim(h.pal, "▒ <75%"),
		stDim(h.pal, "▓ <100%"),
		lipgloss.NewStyle().Foreground(h.pal.Green).Render("█ Ziel"),
		lipgloss.NewStyle().Foreground(h.pal.Red).Render("█ ≥150%"),
		lipgloss.NewStyle().Foreground(h.pal.Cyan).Render("★/☼/✚ frei"),
	}
	return joinWrapped(legend, "  ", "   ", "   ", inner)
}

func heatmapCellGlyph(pal theme.Palette, rec domain.DayRecord) (string, lipgloss.TerminalColor) {
	pct := float64(rec.Total) / float64(rec.Target)
	switch {
	case pct >= 1.5:
		return " █ ", pal.Red
	case pct >= 1.0:
		return " █ ", pal.Green
	case pct >= 0.75:
		return " ▓ ", pal.Green
	case pct >= 0.5:
		return " ▒ ", pal.Yellow
	case pct > 0:
		return " ░ ", pal.Yellow
	}
	return " . ", pal.Border
}

func dayOffHeatmapGlyph(k domain.Kind) string {
	switch k {
	case domain.KindHoliday:
		return " ★ "
	case domain.KindVacation:
		return " ☼ "
	case domain.KindSick:
		return " ✚ "
	}
	return " · "
}

// tagClockGrid sums per-(weekday, hour) durations across the records'
// sessions, splitting on hour boundaries. Returns the grid and the
// largest single cell so callers can scale fractions.
func tagClockGrid(records []domain.DayRecord) ([7][24]time.Duration, time.Duration) {
	var grid [7][24]time.Duration
	for _, rec := range records {
		for _, s := range rec.Sessions {
			t := s.Start
			for t.Before(s.Stop) {
				wd := int(t.Weekday()) - 1
				if wd < 0 {
					wd = 6
				}
				hour := t.Hour()
				next := time.Date(t.Year(), t.Month(), t.Day(), hour+1, 0, 0, 0, t.Location())
				if next.After(s.Stop) {
					next = s.Stop
				}
				grid[wd][hour] += next.Sub(t)
				t = next
			}
		}
	}
	var maxCell time.Duration
	for r := 0; r < 7; r++ {
		for c := 0; c < 24; c++ {
			if grid[r][c] > maxCell {
				maxCell = grid[r][c]
			}
		}
	}
	return grid, maxCell
}

func tagClockCellGlyph(pal theme.Palette, cell time.Duration, frac float64) (string, lipgloss.TerminalColor) {
	switch {
	case cell == 0:
		return "··", pal.Border
	case frac >= 0.75:
		return "██", pal.Green
	case frac >= 0.5:
		return "▓▓", pal.Green
	case frac >= 0.25:
		return "▒▒", pal.Yellow
	case frac > 0:
		return "░░", pal.Yellow
	}
	return "··", pal.Border
}

func (h history) renderTagClockHeader() string {
	hdr := "      "
	for col := 0; col < 24; col++ {
		c := h.pal.Dim
		if col == 9 || col == 12 || col == 17 {
			c = h.pal.Border
		}
		hdr += lipgloss.NewStyle().Foreground(c).Render(fmt.Sprintf("%02d", col))
	}
	return hdr
}

func (h history) renderTagClockRows(grid [7][24]time.Duration, maxCell time.Duration, dayLabels []string) []string {
	out := make([]string, 0, 7)
	for r := 0; r < 7; r++ {
		row := "  " + lipgloss.NewStyle().Foreground(h.pal.Fg).Width(3).Render(dayLabels[r]) + " "
		for c := 0; c < 24; c++ {
			frac := float64(grid[r][c]) / float64(maxCell)
			cell, color := tagClockCellGlyph(h.pal, grid[r][c], frac)
			cellStyle := lipgloss.NewStyle().Foreground(color)
			if r == h.tagClockRow && c == h.tagClockCol {
				cellStyle = lipgloss.NewStyle().Foreground(h.pal.Bg).Background(h.pal.Accent).Bold(true)
			}
			row += cellStyle.Render(cell)
		}
		out = append(out, row)
	}
	return out
}

func (h history) renderTagClockStatus(grid [7][24]time.Duration, maxCell time.Duration, dayLabels []string) string {
	col, row := h.tagClockCol, h.tagClockRow
	if row < 0 || row >= 7 || col < 0 || col >= 24 {
		return ""
	}
	dur := grid[row][col]
	var status string
	if dur == 0 {
		status = fmt.Sprintf("   %s  %02d:00–%02d:00  —",
			dayLabels[row], col, (col+1)%24)
	} else {
		pct := int(float64(dur) / float64(maxCell) * 100)
		status = fmt.Sprintf("   %s  %02d:00–%02d:00  %s  (%d%% des Maximums)",
			dayLabels[row], col, (col+1)%24, formatDur(dur), pct)
	}
	return lipgloss.NewStyle().Foreground(h.pal.Accent).Render(status)
}

func (h history) renderTagClock(records []domain.DayRecord, inner int) string {
	if len(records) == 0 {
		return stDim(h.pal, "  Keine Treffer.")
	}
	grid, maxCell := tagClockGrid(records)
	if maxCell == 0 {
		return stDim(h.pal, "  Keine Treffer.")
	}
	dayLabels := []string{"Mo", "Di", "Mi", "Do", "Fr", "Sa", "So"}
	lines := []string{h.renderTagClockHeader()}
	lines = append(lines, h.renderTagClockRows(grid, maxCell, dayLabels)...)
	lines = append(lines, "")
	if status := h.renderTagClockStatus(grid, maxCell, dayLabels); status != "" {
		lines = append(lines, status)
	}
	legend := []string{
		stDim(h.pal, "·· keine"),
		stDim(h.pal, "░░ <25%"),
		stDim(h.pal, "▒▒ <50%"),
		stDim(h.pal, "▓▓ <75%"),
		stDim(h.pal, "██ ≥75%"),
	}
	lines = append(lines, joinWrapped(legend, "  ", "   ", "   ", inner))
	return strings.Join(lines, "\n")
}

func (h history) renderMonth(records []domain.DayRecord, _ int) string {
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
	header := lipgloss.NewStyle().Foreground(h.pal.Accent).Bold(true).
		Render(fmt.Sprintf("  %s %d", domain.MonthShortDe(first.Month()), first.Year()))
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
		hdr += lipgloss.NewStyle().Foreground(h.pal.Dim).Render(fmt.Sprintf(" %-3s ", lbl))
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
	return lipgloss.NewStyle().Foreground(h.pal.Accent).Render(status)
}

func (h history) renderMonthAggregate(monthRef, now time.Time) string {
	if h.monthStats.Days == 0 || monthRef.Year() != now.Year() || monthRef.Month() != now.Month() {
		return ""
	}
	balColor := h.pal.Dim
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

	glyph := "·"
	color := h.pal.Border
	switch {
	case hasRec && rec.Target > 0:
		pct := float64(rec.Total) / float64(rec.Target)
		switch {
		case pct >= 1.5:
			glyph, color = "█", h.pal.Red
		case pct >= 1.0:
			glyph, color = "█", h.pal.Green
		case pct >= 0.75:
			glyph, color = "▓", h.pal.Green
		case pct >= 0.5:
			glyph, color = "▒", h.pal.Yellow
		case pct > 0:
			glyph, color = "░", h.pal.Yellow
		}
	case isOff:
		switch dayOff.Kind {
		case domain.KindHoliday:
			glyph = "★"
		case domain.KindVacation:
			glyph = "☼"
		case domain.KindSick:
			glyph = "✚"
		}
		color = h.pal.Cyan
	case isWeekend:
		glyph, color = " ", h.pal.Dim
	}
	dayNum := fmt.Sprintf("%2d", day.Day())
	body := fmt.Sprintf(" %s %s", dayNum, glyph)
	st := lipgloss.NewStyle().Foreground(color)
	switch {
	case isCursor:
		st = lipgloss.NewStyle().Foreground(h.pal.Bg).Background(h.pal.Accent).Bold(true)
	case isToday:
		st = st.Underline(true).Bold(true)
	}
	return st.Render(body) + " "
}

func (h history) renderDrill() string {
	inner := h.width - 4
	if inner <= 0 {
		inner = 80
	}
	rows := []string{lipgloss.NewStyle().Foreground(h.pal.Accent).Bold(true).
		Render("  Tag " + h.drillDate.Format("2006-01-02") + " (" +
			domain.WeekdayShortDe(h.drillDate.Weekday()) + ")"), ""}
	if h.drillErr != nil {
		rows = append(rows, stErr(h.pal, h.drillErr.Error()))
		rows = append(rows, "", stDim(h.pal, "  b/Esc zurück"))
		return strings.Join(rows, "\n")
	}
	if len(h.drillSessions) == 0 {
		rows = append(rows, stDim(h.pal, "  keine Sessions an diesem Tag"))
		rows = append(rows, "", stDim(h.pal, "  b/Esc zurück"))
		return strings.Join(rows, "\n")
	}
	target := h.deps.Stats.Targets.For(h.drillDate)
	var total time.Duration
	for _, s := range h.drillSessions {
		total += s.Elapsed
	}
	pct := 0
	if target > 0 {
		pct = int(total * 100 / target)
	}
	rows = append(rows, "  "+lipgloss.NewStyle().Foreground(h.pal.Fg).Bold(true).Render(formatDur(total))+
		"  "+stDim(h.pal, fmt.Sprintf("/ %s  ·  %d%%", formatDur(target), pct)))
	rows = append(rows, "")
	rows = append(rows, picker.SectionHeader(
		fmt.Sprintf("sessions (%d)", len(h.drillSessions)), inner, h.pal))
	prevStop := time.Time{}
	for i, s := range h.drillSessions {
		if !prevStop.IsZero() {
			pause := s.Start.Sub(prevStop)
			if pause > 0 {
				rows = append(rows, stDim(h.pal,
					fmt.Sprintf("       ─ %s Pause ─", formatDur(pause))))
			}
		}
		prevStop = s.Stop
		dur := lipgloss.NewStyle().Width(8).Render(formatDur(s.Elapsed))
		label := fmt.Sprintf("%s → %s   %s",
			s.Start.Format("15:04"), s.Stop.Format("15:04"), dur)
		hint := ""
		if s.Tag != "" {
			hint = "[" + s.Tag + "]"
		}
		rows = append(rows, picker.Row(i == h.drillCur, label, hint, inner, h.pal))
		if s.Note != "" {
			rows = append(rows, stDim(h.pal, "       "+s.Note))
		}
	}
	rows = append(rows, "", stDim(h.pal, "  j/k auswahl  ·  b/Esc zurück"))
	return strings.Join(rows, "\n")
}

func (h history) renderFilterDialog() string {
	inner := h.width - 4
	if inner <= 0 {
		inner = 80
	}
	rows := []string{
		picker.SectionHeader("filter", inner, h.pal),
		"  " + h.input.View(),
	}
	val := strings.ToLower(strings.TrimSpace(h.input.Value()))
	if strings.HasPrefix(val, "tag:") && len(h.topTags) > 0 {
		rows = append(rows, "")
		rows = append(rows, stDim(h.pal, "  top tags:"))
		rows = append(rows, "  "+strings.Join(h.topTags, "  ·  "))
	}
	rows = append(rows, "")
	rows = append(rows, stDim(h.pal,
		"  Beispiele:  KW18  ·  2026  ·  2026-04  ·  2026-04-01..2026-04-30  ·  tag:deep  ·  note:standup"))
	if h.errMsg != "" {
		rows = append(rows, "", lipgloss.NewStyle().Foreground(h.pal.Red).Render("  "+h.errMsg))
	}
	rows = append(rows, "", stDim(h.pal,
		"  Enter=anwenden  ·  leer=alles  ·  Esc=abbrechen"))
	return strings.Join(rows, "\n")
}

func (h history) footerHints() []string {
	return []string{
		"j/k auswahl",
		"enter drill",
		"v ansicht (" + h.mode.label() + ")",
		"/ filter",
		"[/] paginate",
		"T zurücksetzen",
	}
}

// — pure helpers (private to package) —

func filteredHistory(records []domain.DayRecord, query string, now time.Time) []domain.DayRecord {
	q := strings.TrimSpace(query)
	if q == "" {
		return records
	}
	if out, ok := filterByTag(records, q); ok {
		return out
	}
	if out, ok := filterByNote(records, q); ok {
		return out
	}
	if out, ok := filterByISOWeek(records, q, now); ok {
		return out
	}
	if out, ok := filterByRange(records, q, now); ok {
		return out
	}
	return records
}

func filterByTag(records []domain.DayRecord, q string) ([]domain.DayRecord, bool) {
	if !strings.HasPrefix(strings.ToLower(q), "tag:") {
		return nil, false
	}
	want := strings.TrimSpace(q[len("tag:"):])
	if want == "" {
		return records, true
	}
	out := make([]domain.DayRecord, 0, len(records))
	for _, rec := range records {
		var keep []domain.Session
		var total time.Duration
		for _, s := range rec.Sessions {
			if strings.EqualFold(s.Tag, want) {
				keep = append(keep, s)
				total += s.Elapsed
			}
		}
		if len(keep) > 0 {
			out = append(out, domain.DayRecord{
				Date: rec.Date, Sessions: keep, Total: total, Target: rec.Target,
			})
		}
	}
	return out, true
}

func filterByNote(records []domain.DayRecord, q string) ([]domain.DayRecord, bool) {
	if !strings.HasPrefix(strings.ToLower(q), "note:") {
		return nil, false
	}
	want := strings.ToLower(strings.TrimSpace(q[len("note:"):]))
	if want == "" {
		return records, true
	}
	out := make([]domain.DayRecord, 0, len(records))
	for _, rec := range records {
		var keep []domain.Session
		var total time.Duration
		for _, s := range rec.Sessions {
			if strings.Contains(strings.ToLower(s.Note), want) {
				keep = append(keep, s)
				total += s.Elapsed
			}
		}
		if len(keep) > 0 {
			out = append(out, domain.DayRecord{
				Date: rec.Date, Sessions: keep, Total: total, Target: rec.Target,
			})
		}
	}
	return out, true
}

func filterByISOWeek(records []domain.DayRecord, q string, now time.Time) ([]domain.DayRecord, bool) {
	if !strings.HasPrefix(strings.ToUpper(q), "KW") {
		return nil, false
	}
	var w int
	if _, err := fmt.Sscanf(strings.ToUpper(q), "KW%d", &w); err != nil || w <= 0 {
		return nil, false
	}
	out := make([]domain.DayRecord, 0, len(records))
	for _, r := range records {
		_, rw := r.Date.ISOWeek()
		if rw == w && r.Date.Year() == now.Year() {
			out = append(out, r)
		}
	}
	return out, true
}

func filterByRange(records []domain.DayRecord, q string, now time.Time) ([]domain.DayRecord, bool) {
	r, err := domain.ParseRange(now, q)
	if err != nil || (r.From.IsZero() && r.To.IsZero()) {
		return nil, false
	}
	out := make([]domain.DayRecord, 0, len(records))
	for _, rec := range records {
		if r.ContainsDate(rec.Date) {
			out = append(out, rec)
		}
	}
	return out, true
}

func isTagOrNote(q string) bool {
	low := strings.ToLower(q)
	return strings.HasPrefix(low, "tag:") || strings.HasPrefix(low, "note:")
}

func isISOWeek(q string) bool {
	if !strings.HasPrefix(strings.ToUpper(q), "KW") {
		return false
	}
	var w int
	_, err := fmt.Sscanf(strings.ToUpper(q), "KW%d", &w)
	return err == nil && w > 0
}

// stepHistFilter advances `q` by `dir` units. KWnn → ±1 week, YYYY-MM →
// ±1 month, YYYY → ±1 year. tag: / note: filters return ok=false. Empty
// is seeded to the current ISO week so paginating without a manual step
// still works.
func stepHistFilter(q string, now time.Time, dir int) (string, bool) {
	q = strings.TrimSpace(q)
	if q == "" {
		_, wn := now.ISOWeek()
		seed := fmt.Sprintf("KW%d", wn)
		return stepHistFilter(seed, now, dir)
	}
	if isTagOrNote(q) {
		return q, false
	}
	if strings.HasPrefix(strings.ToUpper(q), "KW") {
		var w int
		if _, err := fmt.Sscanf(strings.ToUpper(q), "KW%d", &w); err != nil {
			return q, false
		}
		mon := isoMondayOfISOWeek(now.Year(), w, now.Location())
		shifted := mon.AddDate(0, 0, 7*dir)
		_, ww := shifted.ISOWeek()
		return fmt.Sprintf("KW%d", ww), true
	}
	if len(q) == 7 && q[4] == '-' {
		t, err := time.ParseInLocation("2006-01", q, now.Location())
		if err != nil {
			return q, false
		}
		shifted := t.AddDate(0, dir, 0)
		return shifted.Format("2006-01"), true
	}
	if len(q) == 4 {
		var y int
		if _, err := fmt.Sscanf(q, "%d", &y); err != nil {
			return q, false
		}
		return fmt.Sprintf("%d", y+dir), true
	}
	return q, false
}

func isoMondayOfISOWeek(year, week int, loc *time.Location) time.Time {
	jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, loc)
	wd := int(jan4.Weekday())
	if wd == 0 {
		wd = 7
	}
	mon1 := jan4.AddDate(0, 0, -(wd - 1))
	return mon1.AddDate(0, 0, 7*(week-1))
}

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
	if weeks > 26 {
		weeks = 26
		startMon = endMon.AddDate(0, 0, -7*(weeks-1))
	}
	return startMon, weeks
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

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

func isWeekendDate(t time.Time) bool {
	wd := t.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// joinWrapped joins parts with sep, wrapping when the line would exceed
// maxWidth. prefix on the first wrapped line; cont on the followers.
func joinWrapped(parts []string, sep, prefix, cont string, maxWidth int) string {
	if len(parts) == 0 {
		return ""
	}
	if maxWidth <= 0 {
		return prefix + strings.Join(parts, sep)
	}
	var lines []string
	cur := prefix + parts[0]
	for _, p := range parts[1:] {
		cand := cur + sep + p
		if lipgloss.Width(cand) > maxWidth {
			lines = append(lines, cur)
			cur = cont + p
		} else {
			cur = cand
		}
	}
	lines = append(lines, cur)
	return strings.Join(lines, "\n")
}
