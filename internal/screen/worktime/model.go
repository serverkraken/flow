// Package worktime implements the worktime sidekick screen with three sub-views:
// today (live), week, history. Navigation via Tab.
package worktime

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	wt "github.com/serverkraken/flow/internal/worktime"
	"github.com/serverkraken/tui-kit/components/picker"
	"github.com/serverkraken/tui-kit/components/statusbar"
	"github.com/serverkraken/tui-kit/components/titlebox"
	tk "github.com/serverkraken/tui-kit/theme"
)

// subView identifies which tab is active.
type subView int

const (
	viewToday   subView = 0
	viewWeek    subView = 1
	viewHistory subView = 2
)

// dialogMode tracks which inline form is open.
type dialogMode int

const (
	dialogNone   dialogMode = 0
	dialogStart  dialogMode = 1 // time input for custom start
	dialogEntry1 dialogMode = 2 // manual entry: date
	dialogEntry2 dialogMode = 3 // manual entry: start time
	dialogEntry3 dialogMode = 4 // manual entry: stop time
)

// — messages —

type tickMsg time.Time

type dayLoadedMsg struct {
	day wt.Day
	err error
}

type weekLoadedMsg struct {
	week []wt.WeekDay
	err  error
}

type historyLoadedMsg struct {
	history []wt.DayRecord
	err     error
}

type actionDoneMsg struct{ err error }

// Model is the bubbletea model for the worktime screen.
type Model struct {
	day     wt.Day
	week    []wt.WeekDay
	history []wt.DayRecord
	now     time.Time

	view   subView
	dialog dialogMode
	input  textinput.Model
	histVp viewport.Model

	entryDate  string
	entryStart string

	weekLoaded    bool
	historyLoaded bool
	loading       bool
	err           error

	theme  tk.Palette
	width  int
	height int
}

// New creates a new worktime Model.
func New(p tk.Palette) Model {
	ti := textinput.New()
	ti.CharLimit = 20
	return Model{
		theme:   p,
		now:     time.Now(),
		loading: true,
		input:   ti,
	}
}

// FilterActive returns true when a dialog input is focused.
func (m Model) FilterActive() bool { return m.dialog != dialogNone }

// StateFilter returns "" — worktime has no filter to persist.
func (m Model) StateFilter() string { return "" }

// StateCursor returns 0 — worktime has no cursor to persist.
func (m Model) StateCursor() int { return 0 }

// Init loads today's data and starts the per-second tick.
func (m Model) Init() tea.Cmd {
	return tea.Batch(loadTodayCmd(), tickCmd())
}

// Update handles all messages for the worktime screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.histVp = viewport.New(msg.Width-4, msg.Height-4)
		if m.historyLoaded {
			m.histVp.SetContent(m.renderHistoryContent())
		}
		return m, nil

	case tickMsg:
		m.now = time.Time(msg)
		return m, tickCmd()

	case dayLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.day = msg.day
		}
		return m, nil

	case weekLoadedMsg:
		m.weekLoaded = true
		if msg.err == nil {
			m.week = msg.week
		}
		return m, nil

	case historyLoadedMsg:
		m.historyLoaded = true
		if msg.err == nil {
			m.history = msg.history
		}
		if m.width > 0 {
			m.histVp.SetContent(m.renderHistoryContent())
		}
		return m, nil

	case actionDoneMsg:
		m.dialog = dialogNone
		m.input.Blur()
		m.input.SetValue("")
		m.weekLoaded = false
		m.historyLoaded = false
		return m, loadTodayCmd()

	case tea.KeyMsg:
		if m.dialog != dialogNone {
			return m.handleDialogKey(msg)
		}
		return m.handleNormalKey(msg)
	}

	if m.view == viewHistory {
		var cmd tea.Cmd
		m.histVp, cmd = m.histVp.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab":
		return m.nextView()
	case "s":
		if m.day.IsRunning() {
			return m, stopCmd()
		}
		m.dialog = dialogStart
		m.input.Placeholder = time.Now().Format("15:04") + " · -1h30m · Enter=jetzt"
		m.input.SetValue("")
		m.input.Focus()
		return m, textinput.Blink
	case "e":
		m.dialog = dialogEntry1
		m.input.Placeholder = time.Now().Format("2006-01-02")
		m.input.SetValue("")
		m.input.Focus()
		return m, textinput.Blink
	}
	if m.view == viewHistory {
		var cmd tea.Cmd
		m.histVp, cmd = m.histVp.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleDialogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.dialog = dialogNone
		m.input.Blur()
		m.input.SetValue("")
		return m, nil
	case tea.KeyEnter:
		return m.confirmDialog()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) confirmDialog() (tea.Model, tea.Cmd) {
	val := strings.TrimSpace(m.input.Value())

	switch m.dialog {
	case dialogStart:
		ts, err := wt.ParseStartArg(val)
		if err != nil {
			m.input.Placeholder = "Fehler: " + err.Error()
			m.input.SetValue("")
			return m, textinput.Blink
		}
		return m, startCmd(ts)

	case dialogEntry1:
		if val == "" {
			val = time.Now().Format("2006-01-02")
		}
		if _, err := time.ParseInLocation("2006-01-02", val, time.Local); err != nil {
			m.input.Placeholder = "Format: YYYY-MM-DD"
			m.input.SetValue("")
			return m, textinput.Blink
		}
		m.entryDate = val
		m.dialog = dialogEntry2
		m.input.Placeholder = "HH:MM"
		m.input.SetValue("")
		return m, textinput.Blink

	case dialogEntry2:
		if _, err := time.Parse("15:04", val); err != nil {
			m.input.Placeholder = "Format: HH:MM"
			m.input.SetValue("")
			return m, textinput.Blink
		}
		m.entryStart = val
		m.dialog = dialogEntry3
		m.input.Placeholder = "HH:MM"
		m.input.SetValue("")
		return m, textinput.Blink

	case dialogEntry3:
		if _, err := time.Parse("15:04", val); err != nil {
			m.input.Placeholder = "Format: HH:MM"
			m.input.SetValue("")
			return m, textinput.Blink
		}
		return m, addManualCmd(m.entryDate, m.entryStart, val)
	}
	return m, nil
}

func (m Model) nextView() (tea.Model, tea.Cmd) {
	next := (m.view + 1) % 3
	m.view = next
	var cmd tea.Cmd
	switch next {
	case viewWeek:
		if !m.weekLoaded {
			cmd = loadWeekCmd(m.now)
		}
	case viewHistory:
		if !m.historyLoaded {
			cmd = loadHistoryCmd()
		}
	}
	return m, cmd
}

// View renders the current sub-view.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	if m.dialog != dialogNone {
		return m.renderDialog()
	}
	switch m.view {
	case viewWeek:
		return m.renderWeek()
	case viewHistory:
		return m.renderHistory()
	default:
		return m.renderToday()
	}
}

// — today view —

func (m Model) renderToday() string {
	inner := m.width - 4
	var rows []string
	rows = append(rows, "")

	if m.loading {
		rows = append(rows, stDim(m.theme, "  lade…"))
	} else if m.err != nil {
		rows = append(rows, stErr(m.theme, m.err.Error()))
	} else {
		rows = append(rows, m.renderTodayBody(inner)...)
	}

	rows = append(rows, "")
	body := strings.Join(rows, "\n")

	status := "pausiert"
	if m.day.IsRunning() {
		status = "läuft ▶"
	}
	title := fmt.Sprintf("Worktime · %s, %02d. %s · %s · %s",
		germanWeekday(m.now.Weekday()),
		m.now.Day(),
		germanMonth(m.now.Month()),
		m.now.Format("15:04:05"),
		status,
	)
	box := titlebox.Render(title, body, m.width, m.theme)

	toggle := "s → starten"
	if m.day.IsRunning() {
		toggle = "s → stoppen"
	}
	return box + "\n" + stFooter(m.theme, toggle+"  ·  e → eintrag  ·  tab → woche  ·  b → zurück")
}

func (m Model) renderTodayBody(inner int) []string {
	now := m.now
	total := m.day.Total(now)
	target := m.day.Target
	pct := 0
	if target > 0 {
		pct = int(total * 100 / target)
		if pct > 100 {
			pct = 100
		}
	}

	barCells := inner - 8
	if barCells < 4 {
		barCells = 4
	}
	pctStr := lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true).
		Render(fmt.Sprintf("%3d%%", pct))
	bar := "  " + statusbar.Bar(pct, barCells, m.theme) + "  " + pctStr

	remaining := target - total
	if remaining < 0 {
		remaining = 0
	}
	eta := ""
	if m.day.Active != nil {
		etaT := m.day.Active.Add(target - m.day.Logged)
		eta = " · ETA " + etaT.Format("15:04")
	}
	summary := stDim(m.theme, fmt.Sprintf("  %s  ·  Ziel %s  ·  noch %s%s",
		formatDur(total), formatDur(target), formatDur(remaining), eta))

	rows := []string{bar, summary}

	if m.day.Active != nil {
		elapsed := now.Sub(*m.day.Active)
		if elapsed < 0 {
			elapsed = 0
		}
		rows = append(rows, "")
		rows = append(rows, picker.SectionHeader("läuft seit "+m.day.Active.Format("15:04"), inner, m.theme))
		rows = append(rows,
			lipgloss.NewStyle().Foreground(m.theme.Cyan).Bold(true).
				Render("  "+formatDurLive(elapsed)),
		)
	}

	if len(m.day.Sessions) > 0 {
		rows = append(rows, "")
		rows = append(rows, picker.SectionHeader("sessions heute", inner, m.theme))
		for _, s := range m.day.Sessions {
			rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Fg).Render(
				fmt.Sprintf("  %s → %s   %s", s.Start.Format("15:04"), s.Stop.Format("15:04"), formatDur(s.Elapsed)),
			))
		}
	}
	return rows
}

// — week view —

func (m Model) renderWeek() string {
	inner := m.width - 4
	var rows []string
	rows = append(rows, "")

	if !m.weekLoaded {
		rows = append(rows, stDim(m.theme, "  lade…"))
	} else {
		rows = append(rows, m.renderWeekBody(inner)...)
	}

	rows = append(rows, "")
	body := strings.Join(rows, "\n")

	_, weekNum := m.now.ISOWeek()
	monday := isoMonday(m.now)
	friday := monday.AddDate(0, 0, 4)
	title := fmt.Sprintf("Worktime · Woche %d · %02d. %s – %02d. %s",
		weekNum,
		monday.Day(), germanMonth(monday.Month()),
		friday.Day(), germanMonth(friday.Month()),
	)
	box := titlebox.Render(title, body, m.width, m.theme)
	return box + "\n" + stFooter(m.theme, "s → start/stopp  ·  e → eintrag  ·  tab → history  ·  b → zurück")
}

func (m Model) renderWeekBody(inner int) []string {
	now := m.now
	barW := 12
	var rows []string
	var weekTotal time.Duration

	for _, day := range m.week {
		total := day.Total(now)
		weekTotal += total

		name := germanWeekdayShort(day.Date.Weekday())
		date := fmt.Sprintf("%02d.%02d", day.Date.Day(), day.Date.Month())
		pct := 0
		if day.Target > 0 {
			pct = int(total * 100 / day.Target)
			if pct > 100 {
				pct = 100
			}
		}

		nameStr := lipgloss.NewStyle().Foreground(m.theme.Fg).Width(3).Render(name)
		dateStr := lipgloss.NewStyle().Foreground(m.theme.Dim).Width(6).Render(date)

		var line string
		if total == 0 {
			emptyBar := lipgloss.NewStyle().Foreground(m.theme.Border).Render(strings.Repeat("─", barW))
			todayMark := ""
			if day.IsToday {
				todayMark = "  " + stDim(m.theme, "heute")
			}
			line = "  " + nameStr + " " + dateStr + "  " + emptyBar + todayMark
		} else {
			bar := statusbar.Bar(pct, barW, m.theme)
			pctStr := stDim(m.theme, fmt.Sprintf("%3d%%", pct))
			durStr := lipgloss.NewStyle().Foreground(m.theme.Fg).Bold(total >= day.Target).Render(formatDur(total))
			extra := ""
			if day.IsToday && day.Active != nil {
				extra += "  " + lipgloss.NewStyle().Foreground(m.theme.Green).Render("▶")
			}
			if total >= day.Target {
				extra += "  " + lipgloss.NewStyle().Foreground(m.theme.Green).Render("✓")
			}
			line = "  " + nameStr + " " + dateStr + "  " + bar + "  " + pctStr + "  " + durStr + extra
		}
		rows = append(rows, line)
	}

	weekTarget := time.Duration(len(m.week)) * wt.TargetHours * time.Hour
	pct := 0
	if weekTarget > 0 {
		pct = int(weekTotal * 100 / weekTarget)
		if pct > 100 {
			pct = 100
		}
	}
	rows = append(rows, "")
	rows = append(rows, picker.SectionHeader("woche gesamt", inner, m.theme))
	rows = append(rows,
		"  "+lipgloss.NewStyle().Foreground(m.theme.Fg).Bold(true).Render(formatDur(weekTotal))+
			"  "+stDim(m.theme, "/ "+formatDur(weekTarget)),
	)
	rows = append(rows,
		"  "+statusbar.Bar(pct, inner-8, m.theme)+"  "+
			lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true).Render(fmt.Sprintf("%3d%%", pct)),
	)
	return rows
}

// — history view —

func (m Model) renderHistory() string {
	var content string
	if !m.historyLoaded {
		content = stDim(m.theme, "\n  lade…")
	} else if len(m.history) == 0 {
		content = stDim(m.theme, "\n  Noch keine Einträge.")
	} else {
		content = m.histVp.View()
	}
	box := titlebox.Render("Worktime · History", content, m.width, m.theme)
	return box + "\n" + stFooter(m.theme, "j/k ↑/↓ → scrollen  ·  tab → heute  ·  b → zurück")
}

func (m Model) renderHistoryContent() string {
	barW := 12
	var lines []string
	for _, rec := range m.history {
		pct := 0
		if rec.Target > 0 {
			pct = int(rec.Total * 100 / rec.Target)
			if pct > 100 {
				pct = 100
			}
		}
		name := germanWeekdayShort(rec.Date.Weekday())
		date := fmt.Sprintf("%02d.%02d.%02d", rec.Date.Day(), rec.Date.Month(), rec.Date.Year()%100)
		nameStr := lipgloss.NewStyle().Foreground(m.theme.Fg).Width(3).Render(name)
		dateStr := lipgloss.NewStyle().Foreground(m.theme.Dim).Width(9).Render(date)
		bar := statusbar.Bar(pct, barW, m.theme)
		pctStr := stDim(m.theme, fmt.Sprintf("%3d%%", pct))
		durStr := lipgloss.NewStyle().Foreground(m.theme.Fg).Bold(rec.Total >= rec.Target).Render(formatDur(rec.Total))
		done := ""
		if rec.Total >= rec.Target {
			done = "  " + lipgloss.NewStyle().Foreground(m.theme.Green).Render("✓")
		}
		lines = append(lines, "  "+nameStr+" "+dateStr+"  "+bar+"  "+pctStr+"  "+durStr+done)
	}
	return strings.Join(lines, "\n")
}

// — dialog overlay —

func (m Model) renderDialog() string {
	inner := m.width - 4
	var rows []string
	rows = append(rows, "")

	var title, hint string
	switch m.dialog {
	case dialogStart:
		title = "Worktime · Start"
		hint = "Enter=jetzt  ·  HH:MM  ·  -1h30m  ·  Esc=abbrechen"
		rows = append(rows, picker.SectionHeader("startzeit", inner, m.theme))
		rows = append(rows, "  "+m.input.View())

	case dialogEntry1:
		title = "Worktime · Manueller Eintrag (1/3)"
		hint = "Datum  ·  Enter=weiter  ·  Esc=abbrechen"
		rows = append(rows, picker.SectionHeader("datum (YYYY-MM-DD)", inner, m.theme))
		rows = append(rows, "  "+m.input.View())

	case dialogEntry2:
		title = "Worktime · Manueller Eintrag (2/3)"
		hint = "Startzeit  ·  Enter=weiter  ·  Esc=abbrechen"
		rows = append(rows, stDim(m.theme, "  Datum:  "+m.entryDate))
		rows = append(rows, "")
		rows = append(rows, picker.SectionHeader("start (HH:MM)", inner, m.theme))
		rows = append(rows, "  "+m.input.View())

	case dialogEntry3:
		title = "Worktime · Manueller Eintrag (3/3)"
		hint = "Endzeit  ·  Enter=speichern  ·  Esc=abbrechen"
		rows = append(rows, stDim(m.theme, "  Datum:  "+m.entryDate))
		rows = append(rows, stDim(m.theme, "  Start:  "+m.entryStart))
		rows = append(rows, "")
		rows = append(rows, picker.SectionHeader("stop (HH:MM)", inner, m.theme))
		rows = append(rows, "  "+m.input.View())
	}

	rows = append(rows, "")
	box := titlebox.Render(title, strings.Join(rows, "\n"), m.width, m.theme)
	return box + "\n" + stFooter(m.theme, hint)
}

// — commands —

func loadTodayCmd() tea.Cmd {
	return func() tea.Msg {
		day, err := wt.LoadToday()
		return dayLoadedMsg{day: day, err: err}
	}
}

func loadWeekCmd(now time.Time) tea.Cmd {
	return func() tea.Msg {
		week, err := wt.LoadWeek(now)
		return weekLoadedMsg{week: week, err: err}
	}
}

func loadHistoryCmd() tea.Cmd {
	return func() tea.Msg {
		h, err := wt.LoadHistory()
		return historyLoadedMsg{history: h, err: err}
	}
}

func startCmd(ts time.Time) tea.Cmd {
	return func() tea.Msg { return actionDoneMsg{err: wt.Start(ts)} }
}

func stopCmd() tea.Cmd {
	return func() tea.Msg {
		_, err := wt.Stop()
		return actionDoneMsg{err: err}
	}
}

func addManualCmd(dateStr, startStr, stopStr string) tea.Cmd {
	return func() tea.Msg {
		date, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		startD, err := parseHM(startStr)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		stopD, err := parseHM(stopStr)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		base := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.Local)
		return actionDoneMsg{err: wt.AddManual(date, base.Add(startD), base.Add(stopD))}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// — formatting —

func formatDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}

func formatDurLive(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%dh %02dm %02ds", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}

func parseHM(s string) (time.Duration, error) {
	parts := strings.SplitN(strings.TrimSpace(s), ":", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid HH:MM: %s", s)
	}
	var h, m int
	if _, err := fmt.Sscanf(parts[0], "%d", &h); err != nil {
		return 0, err
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &m); err != nil {
		return 0, err
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute, nil
}

var weekdayLong = [7]string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"}
var weekdayShort = [7]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}
var monthNames = [13]string{"", "Jan", "Feb", "Mär", "Apr", "Mai", "Jun", "Jul", "Aug", "Sep", "Okt", "Nov", "Dez"}

func germanWeekday(wd time.Weekday) string      { return weekdayLong[wd] }
func germanWeekdayShort(wd time.Weekday) string { return weekdayShort[wd] }
func germanMonth(m time.Month) string           { return monthNames[m] }

func isoMonday(t time.Time) time.Time {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	d := t.AddDate(0, 0, -(wd - 1))
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, t.Location())
}

func stDim(p tk.Palette, s string) string {
	return lipgloss.NewStyle().Foreground(p.Dim).Render(s)
}

func stErr(p tk.Palette, s string) string {
	return lipgloss.NewStyle().Foreground(p.Red).Render("  " + s)
}

func stFooter(p tk.Palette, s string) string {
	return lipgloss.NewStyle().Foreground(p.Dim).Padding(0, 1).Render(s)
}
