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

type heuteLoadedMsg struct {
	day domain.Day
	err error
}

type heuteActionDoneMsg struct {
	err   error
	toast string
}

type heuteClearToastMsg struct{}

// — dialog modes —

type heuteDialog int

const (
	heuteDialogNone heuteDialog = iota
	heuteDialogTag
	heuteDialogNote
	heuteDialogEdit
	heuteDialogDelete
)

// heute is the Heute (today) sub-model. F4.3 wave B gives it the action
// surface needed for everyday tracking: start/stop/pause/resume plus
// per-session edits (tag, note, edit, delete). Decoration features
// (sparkline, pomodoro, typical stop time, day-off banner, best-streak
// celebration, smart stop suggestion, Kompendium notes) are deferred to
// post-wave-B enhancements — they don't block the architectural lift.
type heute struct {
	pal  theme.Palette
	deps Deps

	width int

	day    domain.Day
	cursor int
	loaded bool
	err    error

	dialog heuteDialog
	// input drives single-input dialogs (tag, note).
	input textinput.Model
	// form drives the multi-input edit dialog.
	form    []textinput.Model
	formCur int

	editIdx  int
	editDate time.Time

	toast  string
	errMsg string
}

func newHeute(p theme.Palette, deps Deps) heute {
	return heute{pal: p, deps: deps, editIdx: -1}
}

// FilterActive bubbles up to the root so global tab keys don't intercept
// while a dialog input is taking text.
func (h heute) FilterActive() bool { return h.dialog != heuteDialogNone }

// StateFilter has no meaning here — Heute has no filter expression.
func (h heute) StateFilter() string { return "" }

// StateCursor reports the focused session index for state persistence.
func (h heute) StateCursor() int { return h.cursor }

// FastTick reports whether the root should schedule the fast (1 s) tick.
// True during the first minute of an active session — the live elapsed
// counter only shows seconds for that window, then drops to minutes.
func (h heute) FastTick(now time.Time) bool {
	if h.day.Active == nil {
		return false
	}
	return now.Sub(*h.day.Active) < time.Minute
}

// Init kicks off the day load. Action results all return through
// heuteActionDoneMsg, which itself triggers a reload.
func (h heute) Init() tea.Cmd { return h.loadCmd() }

func (h heute) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.width = msg.Width
		return h, nil

	case heuteLoadedMsg:
		h.loaded = true
		h.err = msg.err
		if msg.err == nil {
			h.day = msg.day
			h.clampCursor()
		}
		return h, nil

	case dayRefreshMsg:
		return h, h.loadCmd()

	case heuteActionDoneMsg:
		h.dialog = heuteDialogNone
		h.input.Blur()
		h.input.SetValue("")
		h.form = nil
		h.formCur = 0
		h.errMsg = ""
		h.err = msg.err
		if msg.err == nil && msg.toast != "" {
			h.toast = msg.toast
			return h, tea.Batch(h.loadCmd(),
				tea.Tick(3*time.Second, func(time.Time) tea.Msg { return heuteClearToastMsg{} }))
		}
		return h, h.loadCmd()

	case heuteClearToastMsg:
		h.toast = ""
		return h, nil

	case tea.KeyMsg:
		if h.dialog != heuteDialogNone {
			return h.handleDialogKey(msg)
		}
		return h.handleNormalKey(msg)
	}
	return h, nil
}

func (h heute) loadCmd() tea.Cmd {
	reader := h.deps.Reader
	return func() tea.Msg {
		day, err := reader.Today()
		return heuteLoadedMsg{day: day, err: err}
	}
}

func (h *heute) clampCursor() {
	total := len(h.day.Sessions)
	if h.cursor >= total {
		h.cursor = total - 1
	}
	if h.cursor < 0 {
		h.cursor = 0
	}
}

func (h heute) onSession() bool {
	return h.cursor >= 0 && h.cursor < len(h.day.Sessions)
}

// — keymap (no dialog) —

func (h heute) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if total := len(h.day.Sessions); total > 0 {
			h.cursor = (h.cursor + 1) % total
		}
		return h, nil
	case "k", "up":
		if total := len(h.day.Sessions); total > 0 {
			h.cursor = (h.cursor + total - 1) % total
		}
		return h, nil
	case "g":
		h.cursor = 0
		return h, nil
	case "G":
		if total := len(h.day.Sessions); total > 0 {
			h.cursor = total - 1
		}
		return h, nil
	case "s":
		return h, h.toggleStartStopCmd()
	case "p":
		if h.day.IsRunning() {
			return h, h.pauseCmd()
		}
		return h, nil
	case "t":
		if h.onSession() {
			return h.openTagDialog()
		}
	case "N":
		if h.onSession() {
			return h.openNoteDialog()
		}
	case "E", "enter":
		if h.onSession() {
			return h.openEditDialog()
		}
	case "d":
		if h.onSession() {
			return h.openDeleteDialog()
		}
	}
	return h, nil
}

// toggleStartStopCmd maps the legacy `s` key to the simplest reasonable
// behavior: start when idle, resume when paused, stop when running. The
// smart stop-choice prompt for very short running sessions is deferred.
func (h heute) toggleStartStopCmd() tea.Cmd {
	sw := h.deps.SessionWriter
	clock := h.deps.Clock
	switch {
	case h.day.IsRunning():
		return func() tea.Msg {
			s, err := sw.Stop()
			if err != nil {
				return heuteActionDoneMsg{err: err}
			}
			return heuteActionDoneMsg{toast: fmt.Sprintf("■ Gestoppt — Session %s", formatDur(s.Elapsed))}
		}
	case h.day.IsPaused():
		return func() tea.Msg {
			if err := sw.Resume(); err != nil {
				return heuteActionDoneMsg{err: err}
			}
			return heuteActionDoneMsg{toast: "▶ Worktime fortgesetzt"}
		}
	default:
		return func() tea.Msg {
			now := clock.Now()
			if err := sw.Start(now); err != nil {
				return heuteActionDoneMsg{err: err}
			}
			return heuteActionDoneMsg{toast: "▶ Worktime gestartet — " + now.Format("15:04")}
		}
	}
}

func (h heute) pauseCmd() tea.Cmd {
	sw := h.deps.SessionWriter
	return func() tea.Msg {
		s, err := sw.Pause()
		if err != nil {
			return heuteActionDoneMsg{err: err}
		}
		return heuteActionDoneMsg{toast: fmt.Sprintf("⏸ Pausiert nach %s", formatDur(s.Elapsed))}
	}
}

// — dialog open —

func (h heute) openTagDialog() (tea.Model, tea.Cmd) {
	s := h.day.Sessions[h.cursor]
	h.editIdx = h.cursor
	h.editDate = s.Date
	h.dialog = heuteDialogTag
	h.input = form.NewTextInput("tag (z.B. deep, meeting)", h.pal)
	h.input.SetValue(s.Tag)
	h.input.Focus()
	h.errMsg = ""
	return h, textinput.Blink
}

func (h heute) openNoteDialog() (tea.Model, tea.Cmd) {
	s := h.day.Sessions[h.cursor]
	h.editIdx = h.cursor
	h.editDate = s.Date
	h.dialog = heuteDialogNote
	h.input = form.NewTextInput("kurzer Text", h.pal)
	h.input.SetValue(s.Note)
	h.input.Focus()
	h.errMsg = ""
	return h, textinput.Blink
}

func (h heute) openEditDialog() (tea.Model, tea.Cmd) {
	s := h.day.Sessions[h.cursor]
	h.editIdx = h.cursor
	h.editDate = s.Date
	h.dialog = heuteDialogEdit

	start := form.NewTextInput("HH:MM", h.pal)
	start.SetValue(s.Start.Format("15:04"))
	stop := form.NewTextInput("HH:MM oder +1h30m", h.pal)
	stop.SetValue(s.Stop.Format("15:04"))
	tag := form.NewTextInput("z.B. deep, meeting", h.pal)
	tag.SetValue(s.Tag)
	note := form.NewTextInput("kurzer Text", h.pal)
	note.SetValue(s.Note)
	start.Focus()
	h.form = []textinput.Model{start, stop, tag, note}
	h.formCur = 0
	h.errMsg = ""
	return h, textinput.Blink
}

func (h heute) openDeleteDialog() (tea.Model, tea.Cmd) {
	s := h.day.Sessions[h.cursor]
	h.editIdx = h.cursor
	h.editDate = s.Date
	h.dialog = heuteDialogDelete
	h.errMsg = ""
	return h, nil
}

// — dialog dispatch —

func (h heute) handleDialogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch h.dialog {
	case heuteDialogDelete:
		return h.handleDeleteKey(msg)
	case heuteDialogEdit:
		return h.handleFormKey(msg)
	case heuteDialogTag, heuteDialogNote:
		return h.handleSimpleInputKey(msg)
	}
	return h, nil
}

func (h heute) handleSimpleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		h.dialog = heuteDialogNone
		h.input.Blur()
		h.input.SetValue("")
		h.errMsg = ""
		return h, nil
	case tea.KeyEnter:
		return h.submitDialog()
	}
	h.errMsg = ""
	var cmd tea.Cmd
	h.input, cmd = h.input.Update(msg)
	return h, cmd
}

func (h heute) handleFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	maxCur := len(h.form) - 1
	switch msg.Type {
	case tea.KeyEsc:
		h.dialog = heuteDialogNone
		h.form = nil
		h.formCur = 0
		h.errMsg = ""
		return h, nil
	case tea.KeyTab, tea.KeyDown:
		next := h.formCur + 1
		if next > maxCur {
			next = 0
		}
		h.focusForm(next)
		return h, textinput.Blink
	case tea.KeyShiftTab, tea.KeyUp:
		next := h.formCur - 1
		if next < 0 {
			next = maxCur
		}
		h.focusForm(next)
		return h, textinput.Blink
	case tea.KeyEnter:
		if h.formCur < maxCur {
			h.focusForm(h.formCur + 1)
			return h, textinput.Blink
		}
		return h.submitDialog()
	}
	h.errMsg = ""
	if h.formCur >= 0 && h.formCur < len(h.form) {
		var cmd tea.Cmd
		h.form[h.formCur], cmd = h.form[h.formCur].Update(msg)
		return h, cmd
	}
	return h, nil
}

func (h heute) handleDeleteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "z", "j":
		return h, h.deleteCmd(h.editDate, h.editIdx)
	case "n", "esc", "enter":
		h.dialog = heuteDialogNone
		return h, nil
	}
	return h, nil
}

func (h *heute) focusForm(i int) {
	for j := range h.form {
		if j == i {
			h.form[j].Focus()
		} else {
			h.form[j].Blur()
		}
	}
	h.formCur = i
}

// — dialog submit —

func (h heute) submitDialog() (tea.Model, tea.Cmd) {
	sw := h.deps.SessionWriter
	switch h.dialog {
	case heuteDialogTag:
		tag := strings.TrimSpace(h.input.Value())
		date, idx := h.editDate, h.editIdx
		return h, func() tea.Msg {
			if err := sw.SetTag(date, idx, tag); err != nil {
				return heuteActionDoneMsg{err: err}
			}
			if tag == "" {
				return heuteActionDoneMsg{toast: fmt.Sprintf("✓ Tag entfernt (Session %d)", idx+1)}
			}
			return heuteActionDoneMsg{toast: fmt.Sprintf("✓ Tag »%s« gesetzt (Session %d)", tag, idx+1)}
		}

	case heuteDialogNote:
		note := strings.TrimSpace(h.input.Value())
		date, idx := h.editDate, h.editIdx
		return h, func() tea.Msg {
			if err := sw.SetNote(date, idx, note); err != nil {
				return heuteActionDoneMsg{err: err}
			}
			if note == "" {
				return heuteActionDoneMsg{toast: fmt.Sprintf("✓ Notiz entfernt (Session %d)", idx+1)}
			}
			return heuteActionDoneMsg{toast: fmt.Sprintf("✓ Notiz gespeichert (Session %d)", idx+1)}
		}

	case heuteDialogEdit:
		return h.submitEdit()
	}
	return h, nil
}

func (h heute) submitEdit() (tea.Model, tea.Cmd) {
	if len(h.form) < 2 {
		return h, nil
	}
	startStr := strings.TrimSpace(h.form[0].Value())
	stopStr := strings.TrimSpace(h.form[1].Value())
	tag, note := "", ""
	if len(h.form) >= 3 {
		tag = strings.TrimSpace(h.form[2].Value())
	}
	if len(h.form) >= 4 {
		note = strings.TrimSpace(h.form[3].Value())
	}

	startD, err := domain.ParseHM(startStr)
	if err != nil {
		h.errMsg = err.Error()
		return h, nil
	}
	base := time.Date(h.editDate.Year(), h.editDate.Month(), h.editDate.Day(),
		0, 0, 0, 0, h.editDate.Location())
	startTime := base.Add(startD)
	stopTime, err := domain.ParseStop(stopStr, startTime, h.deps.Clock.Now())
	if err != nil {
		h.errMsg = err.Error()
		return h, nil
	}
	// HH:MM stop on a non-today date: rebase to the edit's date so we don't
	// pick up "today + HH:MM" from ParseStartArg's now-anchored logic.
	if stopStr != "" && stopStr[0] != '+' {
		if stopHM, perr := domain.ParseHM(stopStr); perr == nil {
			stopTime = base.Add(stopHM)
		}
	}

	sw := h.deps.SessionWriter
	date, idx := h.editDate, h.editIdx
	return h, func() tea.Msg {
		if err := sw.Edit(date, idx, startTime, stopTime); err != nil {
			return heuteActionDoneMsg{err: err}
		}
		if err := sw.SetTag(date, idx, tag); err != nil {
			return heuteActionDoneMsg{err: err}
		}
		if err := sw.SetNote(date, idx, note); err != nil {
			return heuteActionDoneMsg{err: err}
		}
		return heuteActionDoneMsg{toast: fmt.Sprintf("✓ Session %d aktualisiert", idx+1)}
	}
}

func (h heute) deleteCmd(date time.Time, idx int) tea.Cmd {
	sw := h.deps.SessionWriter
	return func() tea.Msg {
		if err := sw.Delete(date, idx); err != nil {
			return heuteActionDoneMsg{err: err}
		}
		return heuteActionDoneMsg{toast: fmt.Sprintf("✓ Session %d gelöscht", idx+1)}
	}
}

// — render —

func (h heute) View() string {
	if h.width == 0 {
		return ""
	}
	if h.dialog != heuteDialogNone {
		return h.renderDialog()
	}
	return h.renderBody()
}

func (h heute) renderBody() string {
	if !h.loaded {
		return stDim(h.pal, "  lade …")
	}
	if h.err != nil {
		return stErr(h.pal, h.err.Error())
	}

	inner := h.width - 4
	now := h.deps.Clock.Now()

	rows := []string{h.renderHeadline(now), "", h.renderProgressBar(inner), h.renderSummary(inner)}
	if line := h.renderPauseHint(now); line != "" {
		rows = append(rows, "", line)
	}
	rows = append(rows, h.renderSessionsList(inner, now)...)
	if h.toast != "" {
		rows = append(rows, "", lipgloss.NewStyle().Foreground(h.pal.Cyan).Render("  "+h.toast))
	}
	rows = append(rows, "", renderFooterHints(h.pal, h.footerHints(), inner))
	return strings.Join(rows, "\n")
}

func (h heute) renderHeadline(now time.Time) string {
	total := h.day.Total(now)
	target := h.day.Target
	pct := 0
	if target > 0 {
		pct = int(total * 100 / target)
		if pct > 100 {
			pct = 100
		}
	}
	statusGlyph, statusLabel, statusColor := todayStatusBadge(h.pal, h.day.IsRunning(), target == 0 || total >= target)

	totalText := formatDur(total)
	if h.day.IsRunning() && h.day.Active != nil && now.Sub(*h.day.Active) < time.Minute {
		totalText = formatDurLive(total)
	}
	totalStr := lipgloss.NewStyle().Foreground(totalThresholdColor(h.pal, total, target, h.day.IsRunning())).Bold(true).Render(totalText)
	statusStr := lipgloss.NewStyle().Foreground(statusColor).Render(statusGlyph + " " + statusLabel)
	pctStr := lipgloss.NewStyle().Foreground(h.pal.Dim).Render(fmt.Sprintf("%d%%", pct))
	return "  " + totalStr + "   " + statusStr + "   " + pctStr
}

func (h heute) renderProgressBar(inner int) string {
	target := h.day.Target
	total := h.day.Total(h.deps.Clock.Now())
	pct := 0
	if target > 0 {
		pct = int(total * 100 / target)
		if pct > 100 {
			pct = 100
		}
	}
	barCells := inner - 4
	if barCells < 4 {
		barCells = 4
	}
	return "  " + statusbar.Bar(pct, barCells, h.pal)
}

func (h heute) renderSummary(inner int) string {
	target := h.day.Target
	total := h.day.Total(h.deps.Clock.Now())
	remaining := target - total
	if remaining < 0 {
		remaining = 0
	}
	parts := []string{
		fmt.Sprintf("Ziel %s", formatDur(target)),
		fmt.Sprintf("noch %s", formatDur(remaining)),
	}
	if h.day.Active != nil {
		eta := h.day.Active.Add(target - h.day.Logged)
		parts = append(parts, "ETA "+eta.Format("15:04"))
	}
	return renderFooterHints(h.pal, parts, inner)
}

func (h heute) renderPauseHint(now time.Time) string {
	if !h.day.IsPaused() || h.day.PausedAt == nil {
		return ""
	}
	return "  " +
		lipgloss.NewStyle().Foreground(h.pal.Yellow).Bold(true).Render("⏸ in Pause") +
		stDim(h.pal, fmt.Sprintf("  seit %s  ·  %s — `s` setzt fort",
			h.day.PausedAt.Format("15:04"), formatDur(now.Sub(*h.day.PausedAt))))
}

func (h heute) renderSessionsList(inner int, now time.Time) []string {
	totalRows := len(h.day.Sessions)
	if h.day.IsRunning() {
		totalRows++
	}
	if totalRows == 0 {
		if h.day.IsPaused() {
			return nil
		}
		return []string{"", stDim(h.pal, "  Noch nichts erfasst — `s` startet")}
	}

	rows := []string{"", picker.SectionHeader(
		fmt.Sprintf("sessions heute (%d)", totalRows), inner, h.pal)}

	if h.day.IsRunning() && h.day.Active != nil {
		elapsed := now.Sub(*h.day.Active)
		rows = append(rows, lipgloss.NewStyle().Foreground(h.pal.Green).Bold(true).Render(
			fmt.Sprintf("  ▶ %s → …   %s   läuft",
				h.day.Active.Format("15:04"), formatDur(elapsed))))
	}
	for i, s := range h.day.Sessions {
		dur := lipgloss.NewStyle().Width(8).Render(formatDur(s.Elapsed))
		label := fmt.Sprintf("%s → %s   %s",
			s.Start.Format("15:04"), s.Stop.Format("15:04"), dur)
		hint := ""
		if s.Tag != "" {
			hint = "[" + s.Tag + "]"
		}
		rows = append(rows, picker.Row(i == h.cursor, label, hint, inner, h.pal))
		if s.Note != "" {
			rows = append(rows, stDim(h.pal, "       "+s.Note))
		}
	}
	return rows
}

func (h heute) footerHints() []string {
	var actions []string
	switch {
	case h.day.IsRunning():
		actions = append(actions, "s stoppen", "p pause")
	case h.day.IsPaused():
		actions = append(actions, "s resume")
	default:
		actions = append(actions, "s starten")
	}
	if h.onSession() {
		actions = append(actions, "E/⏎ bearbeiten", "d löschen", "t tag", "N notiz")
	}
	actions = append(actions, "j/k auswahl")
	return actions
}

func (h heute) renderDialog() string {
	inner := h.width - 4
	var rows []string
	var title, hint string

	switch h.dialog {
	case heuteDialogTag:
		title = "Tag setzen"
		hint = "Enter=speichern  ·  leer=löschen  ·  Esc=abbrechen"
		rows = append(rows, picker.SectionHeader("tag", inner, h.pal), "  "+h.input.View())

	case heuteDialogNote:
		title = "Session-Notiz"
		hint = "Enter=speichern  ·  leer=löschen  ·  Esc=abbrechen"
		rows = append(rows, picker.SectionHeader("notiz", inner, h.pal), "  "+h.input.View())

	case heuteDialogEdit:
		title = "Session bearbeiten"
		hint = "Tab/↑↓ Feld  ·  Enter=weiter/speichern  ·  Esc=abbrechen"
		if h.editIdx >= 0 && h.editIdx < len(h.day.Sessions) {
			s := h.day.Sessions[h.editIdx]
			rows = append(rows, stDim(h.pal, fmt.Sprintf("  Session %d:  %s → %s",
				h.editIdx+1, s.Start.Format("15:04"), s.Stop.Format("15:04"))), "")
		}
		labels := []string{"Start", "Stop", "Tag", "Notiz"}
		for i, ti := range h.form {
			rows = append(rows, picker.SectionHeader(labels[i], inner, h.pal))
			if i == h.formCur {
				rows = append(rows, "  "+ti.View())
			} else {
				v := ti.Value()
				if v == "" {
					v = stDim(h.pal, ti.Placeholder)
				}
				rows = append(rows, "    "+v)
			}
		}

	case heuteDialogDelete:
		title = "Session löschen"
		hint = "y/z/j=löschen  ·  Enter/n/Esc=abbrechen (default)"
		if h.editIdx >= 0 && h.editIdx < len(h.day.Sessions) {
			s := h.day.Sessions[h.editIdx]
			rows = append(rows,
				lipgloss.NewStyle().Foreground(h.pal.Yellow).Bold(true).Render(
					fmt.Sprintf("  Session %d:  %s → %s  (%s)",
						h.editIdx+1, s.Start.Format("15:04"), s.Stop.Format("15:04"), formatDur(s.Elapsed))),
				"",
				lipgloss.NewStyle().Foreground(h.pal.Red).Render("  Wirklich löschen?"))
		}
	}

	if h.errMsg != "" {
		rows = append(rows, "", lipgloss.NewStyle().Foreground(h.pal.Red).Render("  "+h.errMsg))
	}
	rows = append(rows, "", stDim(h.pal, "  "+title+"  ·  "+hint))
	return strings.Join(rows, "\n")
}

// — small helpers (private to package) —

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

func stDim(p theme.Palette, s string) string {
	return lipgloss.NewStyle().Foreground(p.Dim).Render(s)
}

func stErr(p theme.Palette, s string) string {
	return lipgloss.NewStyle().Foreground(p.Red).Render("  " + s)
}

// renderFooterHints joins the action chips into one or more dim lines that
// fit inside `inner`. Each wrapped line is dim-styled separately because
// lipgloss pads multi-line styled strings (see TestStDimMultilinePadsShorterLines)
// — passing the whole "\n"-joined string through stDim would leak trailing
// spaces into the previous box border.
func renderFooterHints(p theme.Palette, parts []string, inner int) string {
	wrapped := joinWrapped(parts, "  ·  ", "  ", "  ", inner)
	lines := strings.Split(wrapped, "\n")
	for i, l := range lines {
		lines[i] = stDim(p, l)
	}
	return strings.Join(lines, "\n")
}

func todayStatusBadge(p theme.Palette, running, achieved bool) (string, string, lipgloss.TerminalColor) {
	switch {
	case running && achieved:
		return "▶", "läuft ✓", p.Green
	case running:
		return "▶", "läuft", p.Green
	case achieved:
		return "✓", "Ziel erreicht", p.Green
	}
	return "⏸", "pausiert", p.Dim
}

// totalThresholdColor picks the today-total foreground based on running
// state and target progress. Red is reserved for "really a lot" so a
// normal hour of overtime doesn't look like an alarm.
func totalThresholdColor(p theme.Palette, total, target time.Duration, running bool) lipgloss.TerminalColor {
	switch {
	case total >= target+4*time.Hour:
		return p.Red
	case total >= target:
		return p.Green
	case running && total >= target-2*time.Hour:
		return p.Yellow
	case running:
		return p.Cyan
	}
	return p.Dim
}
