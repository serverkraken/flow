// Package worktime implements the worktime sidekick screen with three sub-views:
// today (live), week, history. Navigation via Tab.
package worktime

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
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
	viewToday    subView = 0
	viewWeek     subView = 1
	viewHistory  subView = 2
	viewDayOffs  subView = 3
	subViewCount         = 4
)

// dialogMode tracks which inline form is open.
type dialogMode int

const (
	dialogNone          dialogMode = 0
	dialogStart         dialogMode = 1  // single-input: custom start time
	dialogEntryForm     dialogMode = 2  // single multi-field form: date+start+stop
	dialogStopAt        dialogMode = 3  // single-input: custom stop time
	dialogCorrect       dialogMode = 4  // single-input: corrected start of running session
	dialogEditForm      dialogMode = 5  // single multi-field form: start+stop of existing session
	dialogDeleteConfirm dialogMode = 6  // confirm session deletion
	dialogNotePicker    dialogMode = 7  // pick a Kompendium note to attach
	dialogTagForm       dialogMode = 8  // single-field: set tag on selected session
	dialogNoteForm      dialogMode = 9  // single-field: set free-text note on selected session
	dialogDayDetail     dialogMode = 10 // drill-down sub-view of a single day's sessions
	dialogHelp          dialogMode = 11 // local help overlay
	dialogHistFilter    dialogMode = 12 // history view: inline filter expression
	dialogDayOffAdd     dialogMode = 13 // add a day-off (date, kind, label)
	dialogDayOffConfirm dialogMode = 14 // confirm removal of a day-off
	dialogStopChoice    dialogMode = 15 // smart stop suggestion: Pause vs. Feierabend
)

// noteEntry is one renderable line in the today view's notes section.
type noteEntry struct {
	isDaily bool
	id      string
	label   string
}

// — messages —

type tickMsg time.Time

type todayLoadedMsg struct {
	day     wt.Day
	notes   []noteEntry
	stats   wt.Stats
	history []wt.DayRecord
	err     error
}

// dayRefreshMsg is the lightweight per-tick day reload. Only refreshes
// the day state (cheap file reads); notes are intentionally not touched
// because that would shell out to kompendium every second.
type dayRefreshMsg struct {
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

type dayOffsLoadedMsg struct {
	dayoffs []wt.DayOff
	year    int
	err     error
}

type recentTagsLoadedMsg struct {
	tags    []string
	topTags []string
	err     error
}

type templatesLoadedMsg struct {
	templates []wt.SessionTemplate
	err       error
}

// actionDoneMsg signals a backend mutation has completed. `toast`, when
// non-empty, is shown in the bottom toast slot for ~3s — gives the user
// confirmation that something happened (the changed list is otherwise the
// only feedback). Set by the caller; empty toast = no toast.
type actionDoneMsg struct {
	err   error
	toast string
}

type clearToastMsg struct{}

type undoEntry struct {
	date    time.Time
	session wt.Session
}
type clearErrMsg struct{}

// historyMode toggles between the four history sub-views: list (default),
// the day-percent heatmap, the time-of-day clock that visualises *when*
// during the week sessions happen, and a calendar-month grid for picking
// individual days.
type historyMode int

const (
	historyList     historyMode = 0
	historyHeatmap  historyMode = 1
	historyTagClock historyMode = 2
	historyMonth    historyMode = 3
)

// Model is the bubbletea model for the worktime screen.
type Model struct {
	day     wt.Day
	notes   []noteEntry
	week    []wt.WeekDay
	weekRef time.Time // anchor used for week computation; defaults to time.Now
	history []wt.DayRecord
	now     time.Time
	stats   wt.Stats

	view   subView
	dialog dialogMode
	input  textinput.Model
	histVp viewport.Model

	// Multi-field forms (entry, edit). When the dialog is form-based,
	// formInputs holds 2–3 textinputs and formCur is the focused index.
	formInputs []textinput.Model
	formCur    int

	cursor    int
	weekCur   int // cursor in week view
	histCur   int // cursor over m.history rows
	histMode  historyMode
	histQuery string // active filter expression for history view

	editDate time.Time
	editIdx  int

	// drill sub-view state
	drillDate     time.Time
	drillSessions []wt.Session
	drillCur      int

	picker    []wt.KompendiumNote
	pickerCur int
	pickerErr error

	lastDeleted *undoEntry

	// dayoffs sub-view state.
	dayoffs       []wt.DayOff
	dayoffsCur    int
	dayoffsLoaded bool
	dayoffsYear   int

	// Add-form state for dayoff dialog.
	dayoffKindCur int // 0 = holiday, 1 = vacation, 2 = sick

	// Recent tags cache for tag-autocomplete in dialogTagForm.
	recentTags []string
	topTags    []string // top-by-usage, second suggestion strip
	tagSugCur  int      // -1 = no suggestion focus, otherwise index into recentTags
	topSugCur  int      // mirror of tagSugCur for the top-tags strip

	// Session templates cache for the entry form's quick-insert chips.
	// Loaded on dialogEntryForm open via loadTemplatesCmd. templateCur is
	// the index applied via Ctrl+T (-1 when none has been applied yet).
	templates   []wt.SessionTemplate
	templateCur int

	// Heatmap navigation cursor in history view.
	heatCol int // week column 0..weeks-1
	heatRow int // 0..6 (Mo..So)

	// heatOffsetWeeks shifts the heatmap window by N weeks relative to the
	// newest record. Adjusted with [/] in heatmap mode (±13 weeks per press).
	// Zero means the default window ending on the newest record.
	heatOffsetWeeks int

	// TagClock navigation cursor (Mo..So × 0..23h grid).
	tagClockCol int // hour 0..23
	tagClockRow int // 0..6 (Mo..So)

	// Month-grid navigation. monthRef is the first-of-month for the displayed
	// month; monthCur is the day-of-month under the cursor (1..31). Initialised
	// when entering historyMonth from the v-cycle.
	monthRef time.Time
	monthCur int

	// lastBestStreakSeen is the BestStreak value from the previous load, used
	// to detect when an action just produced a *new* best. -1 means "no value
	// observed yet" (initial load) — we don't celebrate the first observation
	// since that would fire the first time the screen ever opens.
	lastBestStreakSeen int

	// celebrateBestStreak holds a "✦ neuer Best-Streak" hint that should be
	// surfaced once. Cleared on next load. Batch 4's toast system will pick
	// this up; until then, the secondary line renders it inline.
	celebrateBestStreak int

	prevView subView // remembers previous tab for context-aware "b" handling

	weekLoaded    bool
	historyLoaded bool
	loading       bool
	err           error

	// errMsg is shown as an inline error line under the active dialog input,
	// instead of replacing the input's placeholder.
	errMsg string

	// toast is a transient success message rendered above the footer. Cleared
	// after ~3s by clearToastMsg or when a new action runs.
	toast string

	theme  tk.Palette
	width  int
	height int
}

// — cursor helpers (today view) —

func (m Model) totalRows() int  { return len(m.day.Sessions) + len(m.notes) }
func (m Model) onSession() bool { return m.cursor < len(m.day.Sessions) }
func (m Model) sessionIdx() int { return m.cursor }
func (m Model) noteIdx() int    { return m.cursor - len(m.day.Sessions) }
func (m Model) onNote() bool {
	idx := m.noteIdx()
	return idx >= 0 && idx < len(m.notes)
}

// New creates a new worktime Model.
func New(p tk.Palette) Model {
	ti := textinput.New()
	ti.CharLimit = 60
	return Model{
		theme:              p,
		now:                time.Now(),
		weekRef:            time.Now(),
		loading:            true,
		input:              ti,
		lastBestStreakSeen: -1,
		templateCur:        -1,
	}
}

// — form helpers —

func (m *Model) buildEntryForm(now time.Time) {
	date := newField("Datum", "YYYY-MM-DD", now.Format("2006-01-02"))
	start := newField("Start", "HH:MM oder -1h30m", "")
	stop := newField("Stop", "HH:MM, +1h30m oder Enter=jetzt", "")
	date.Focus()
	m.formInputs = []textinput.Model{date, start, stop}
	m.formCur = 0
}

func (m *Model) buildEditForm(s wt.Session) {
	start := newField("Start", "HH:MM", s.Start.Format("15:04"))
	stop := newField("Stop", "HH:MM oder +1h30m", s.Stop.Format("15:04"))
	tag := newField("Tag", "z.B. deep, meeting", s.Tag)
	note := newField("Notiz", "kurzer Text (optional)", s.Note)
	start.Focus()
	m.formInputs = []textinput.Model{start, stop, tag, note}
	m.formCur = 0
}

func newField(_, placeholder, value string) textinput.Model {
	ti := textinput.New()
	ti.CharLimit = 60
	ti.Placeholder = placeholder
	ti.SetValue(value)
	return ti
}

func (m *Model) focusForm(i int) {
	// i may legitimately be == len(m.formInputs) when the form has a virtual
	// trailing field (e.g. the kind-picker on dialogDayOffAdd). In that case
	// blur all real inputs so the kind row visually owns the focus.
	for j := range m.formInputs {
		if j == i {
			m.formInputs[j].Focus()
		} else {
			m.formInputs[j].Blur()
		}
	}
	m.formCur = i
}

func (m *Model) clearForm() {
	m.formInputs = nil
	m.formCur = 0
}

func (m *Model) formValues() []string {
	out := make([]string, len(m.formInputs))
	for i, ti := range m.formInputs {
		out[i] = strings.TrimSpace(ti.Value())
	}
	return out
}

// FilterActive returns true when a dialog (input/form/overlay) is open and
// should consume keys before the global router does.
func (m Model) FilterActive() bool { return m.dialog != dialogNone }

// HandlesBack reports whether `b` should be handled by this screen instead of
// jumping to the palette. True when not on the Heute tab — first `b` returns
// to the previous tab, only on Heute does `b` propagate to the global router.
func (m Model) HandlesBack() bool {
	return m.dialog == dialogNone && m.view != viewToday
}

// StateFilter returns "" — worktime has no filter to persist.
func (m Model) StateFilter() string { return "" }

// StateCursor returns 0 — worktime has no cursor to persist.
func (m Model) StateCursor() int { return 0 }

// Init loads today's data and starts the per-second tick.
func (m Model) Init() tea.Cmd {
	return tea.Batch(loadTodayCmd(time.Now()), tickCmd())
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
		// Adaptive tick: sub-second display only matters for the first ~60s
		// of a running session (when sub-second feedback feels live). After
		// that, drop to 10s to avoid the whole header zappling on slow ttys.
		next := time.Second
		if m.day.Active != nil {
			elapsed := m.now.Sub(*m.day.Active)
			if elapsed >= time.Minute {
				next = 10 * time.Second
			}
		} else {
			next = 10 * time.Second
		}
		return m, tea.Batch(tickCmdEvery(next), refreshDayCmd())

	case dayRefreshMsg:
		if msg.err == nil {
			// Pick up external state changes (start/stop via tmux binding,
			// CLI usage, etc.) without disturbing the cursor or notes.
			m.day = msg.day
			m.clampCursor()
		}
		return m, nil

	case todayLoadedMsg:
		m.loading = false
		if m.err == nil {
			m.err = msg.err
		}
		if msg.err == nil {
			m.day = msg.day
			m.notes = msg.notes
			var streakToast tea.Cmd
			// Best-streak celebration: only fire when the value strictly
			// increases AND we've already observed at least one prior value
			// (not the initial load — that would always celebrate on first
			// run). Sets a toast that survives until the next clear timer.
			if m.lastBestStreakSeen > 0 && msg.stats.BestStreak > m.lastBestStreakSeen {
				m.celebrateBestStreak = msg.stats.BestStreak
				m.toast = fmt.Sprintf("✦ Neuer Best-Streak %d Tage!", msg.stats.BestStreak)
				streakToast = tea.Tick(5*time.Second, func(time.Time) tea.Msg { return clearToastMsg{} })
			}
			m.lastBestStreakSeen = msg.stats.BestStreak
			m.stats = msg.stats
			// Keep history in sync if it isn't already loaded — sparkline +
			// "vs Schnitt" depend on it. Don't overwrite when the user is
			// actively viewing the History tab (m.historyLoaded already true).
			if !m.historyLoaded && len(msg.history) > 0 {
				m.history = msg.history
			}
			m.clampCursor()
			return m, streakToast
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

	case dayOffsLoadedMsg:
		m.dayoffsLoaded = true
		if msg.err == nil {
			m.dayoffs = msg.dayoffs
			m.dayoffsYear = msg.year
			if m.dayoffsCur >= len(m.dayoffs) {
				m.dayoffsCur = max(0, len(m.dayoffs)-1)
			}
		} else {
			m.err = msg.err
		}
		return m, nil

	case recentTagsLoadedMsg:
		if msg.err == nil {
			m.recentTags = msg.tags
			m.topTags = msg.topTags
		}
		return m, nil

	case templatesLoadedMsg:
		if msg.err == nil {
			m.templates = msg.templates
		}
		return m, nil

	case actionDoneMsg:
		drilledDate := m.drillDate
		wasDrilling := m.dialog == dialogDayDetail || m.drillSessions != nil
		m.dialog = dialogNone
		m.input.Blur()
		m.input.SetValue("")
		m.clearForm()
		m.err = msg.err
		// Success toast: only when no error. Replaces any prior toast.
		if msg.err == nil && msg.toast != "" {
			m.toast = msg.toast
		}
		m.weekLoaded = false
		m.historyLoaded = false
		// Day-offs may have been added/removed as part of this action — even
		// when the user isn't on that tab right now, mark it stale so the
		// next switch reloads. When the user *is* on the Frei tab, kick off
		// a refresh immediately so the list reflects the change.
		m.dayoffsLoaded = false
		cmds := []tea.Cmd{loadTodayCmd(m.now)}
		if wasDrilling && !drilledDate.IsZero() {
			cmds = append(cmds, loadDayDetailCmd(drilledDate))
		}
		if m.view == viewDayOffs {
			year := m.dayoffsYear
			if year == 0 {
				year = m.now.Year()
			}
			cmds = append(cmds, loadDayOffsCmd(year))
		}
		if msg.err != nil {
			cmds = append(cmds, tea.Tick(3*time.Second, func(time.Time) tea.Msg { return clearErrMsg{} }))
		}
		if m.toast != "" {
			cmds = append(cmds, tea.Tick(3*time.Second, func(time.Time) tea.Msg { return clearToastMsg{} }))
		}
		return m, tea.Batch(cmds...)

	case clearToastMsg:
		m.toast = ""
		return m, nil

	case dayDetailLoadedMsg:
		if msg.err == nil {
			m.drillSessions = msg.sessions
			if m.drillCur >= len(msg.sessions) {
				m.drillCur = max(0, len(msg.sessions)-1)
			}
			if m.dialog == dialogNone {
				m.dialog = dialogDayDetail
			}
		} else {
			m.err = msg.err
		}
		return m, nil

	case clearErrMsg:
		m.err = nil
		return m, nil

	case notesLoadedMsg:
		m.picker = msg.notes
		m.pickerErr = msg.err
		m.pickerCur = 0
		return m, nil

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
	key := msg.String()
	if model, cmd, ok := m.handleTabKey(key); ok {
		return model, cmd
	}
	if model, cmd, ok := m.handleGlobalActionKey(key); ok {
		return model, cmd
	}

	switch m.view {
	case viewToday:
		if model, cmd, ok := m.handleTodayKey(key); ok {
			return model, cmd
		}
	case viewWeek:
		if model, cmd, ok := m.handleWeekKey(key); ok {
			return model, cmd
		}
	case viewHistory:
		if model, cmd, ok := m.handleHistoryKey(key, msg); ok {
			return model, cmd
		}
	case viewDayOffs:
		if model, cmd, ok := m.handleDayOffsKey(key); ok {
			return model, cmd
		}
	}

	if m.view == viewHistory {
		var cmd tea.Cmd
		m.histVp, cmd = m.histVp.Update(msg)
		return m, cmd
	}
	return m, nil
}

// handleTabKey routes pure tab-switching shortcuts (tab/shift+tab/1..4/?).
// Returns ok=false when the key isn't a tab-switch.
func (m Model) handleTabKey(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "tab":
		mm, cmd := m.nextView()
		return mm, cmd, true
	case "shift+tab":
		mm, cmd := m.prevTab()
		return mm, cmd, true
	case "1":
		mm, cmd := m.gotoView(viewToday)
		return mm, cmd, true
	case "2":
		mm, cmd := m.gotoView(viewWeek)
		return mm, cmd, true
	case "3":
		mm, cmd := m.gotoView(viewHistory)
		return mm, cmd, true
	case "4":
		mm, cmd := m.gotoView(viewDayOffs)
		return mm, cmd, true
	case "?":
		mm, cmd := m.openHelp()
		return mm, cmd, true
	case "b":
		if m.HandlesBack() {
			target := m.prevView
			if target == m.view {
				target = viewToday
			}
			mm, cmd := m.gotoView(target)
			return mm, cmd, true
		}
	}
	return m, nil, false
}

// handleGlobalActionKey routes view-agnostic action keys (r, s, S, p, C, e,
// f, u, j/k, g/G). Returns ok=false when the key isn't claimed.
func (m Model) handleGlobalActionKey(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "r":
		m.weekLoaded = false
		m.historyLoaded = false
		m.dayoffsLoaded = false
		return m, tea.Batch(loadTodayCmd(m.now), loadWeekCmd(m.weekRefOrNow()), loadHistoryCmd()), true
	case "s":
		mm, cmd := m.openStartStopDialog()
		return mm, cmd, true
	case "S":
		// Force-start: ignore Pause state, ask for a start time afresh.
		mm, cmd := m.openForceStartDialog()
		return mm, cmd, true
	case "p":
		// Pause is only meaningful when something is running; otherwise no-op.
		if m.day.IsRunning() {
			return m, pauseCmd(), true
		}
		return m, nil, true
	case "C":
		mm, cmd := m.openCorrectDialog()
		return mm, cmd, true
	case "e":
		mm, cmd := m.openEntryDialog()
		return mm, cmd, true
	case "f":
		mm, cmd := m.enterFocusMode()
		return mm, cmd, true
	case "u":
		if m.lastDeleted != nil {
			entry := m.lastDeleted
			m.lastDeleted = nil
			return m, undoDeleteCmd(entry.date, entry.session), true
		}
	case "j", "down":
		mm, cmd := m.moveCursor(1)
		return mm, cmd, true
	case "k", "up":
		mm, cmd := m.moveCursor(-1)
		return mm, cmd, true
	case "g":
		mm, cmd := m.cursorExtreme(false)
		return mm, cmd, true
	case "G":
		mm, cmd := m.cursorExtreme(true)
		return mm, cmd, true
	}
	return m, nil, false
}

// cursorExtreme moves the per-view cursor to top (false) or bottom (true).
func (m Model) cursorExtreme(end bool) (tea.Model, tea.Cmd) {
	switch m.view {
	case viewToday:
		if total := m.totalRows(); total > 0 {
			if end {
				m.cursor = total - 1
			} else {
				m.cursor = 0
			}
		}
	case viewWeek:
		if n := len(m.week); n > 0 {
			if end {
				m.weekCur = n - 1
			} else {
				m.weekCur = 0
			}
		}
	case viewHistory:
		if n := len(m.history); n > 0 {
			if end {
				m.histCur = n - 1
				m.histVp.GotoBottom()
			} else {
				m.histCur = 0
				m.histVp.GotoTop()
			}
		}
	case viewDayOffs:
		if n := len(m.dayoffs); n > 0 {
			if end {
				m.dayoffsCur = n - 1
			} else {
				m.dayoffsCur = 0
			}
		}
	}
	return m, nil
}

// handleTodayKey routes today-only keys (E/d/n/o/enter/D/t/N). The bool
// reports whether the key was handled; false → fall through to the generic
// dispatcher.
func (m Model) handleTodayKey(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "E":
		if m.onSession() {
			model, cmd := m.openEditDialog()
			return model, cmd, true
		}
	case "enter":
		switch {
		case m.onSession():
			model, cmd := m.openEditDialog()
			return model, cmd, true
		case m.onNote():
			return m, viewNoteCmd(m.notes[m.noteIdx()].id), true
		}
	case "d":
		if m.onSession() {
			model, cmd := m.openDeleteDialog()
			return model, cmd, true
		}
	case "t":
		if m.onSession() {
			s := m.day.Sessions[m.sessionIdx()]
			model, cmd := m.openTagDialog(s.Date, m.sessionIdx(), s.Tag)
			return model, cmd, true
		}
	case "N":
		if m.onSession() {
			s := m.day.Sessions[m.sessionIdx()]
			model, cmd := m.openNoteDialogForSession(s.Date, m.sessionIdx(), s.Note)
			return model, cmd, true
		}
	case "n":
		model, cmd := m.openNotePicker()
		return model, cmd, true
	case "o":
		if m.onNote() {
			return m, viewNoteCmd(m.notes[m.noteIdx()].id), true
		}
	case "O":
		if m.onNote() {
			return m, openNoteCmd(m.notes[m.noteIdx()].id), true
		}
	case "D":
		if m.onNote() {
			n := m.notes[m.noteIdx()]
			if n.isDaily {
				return m, nil, true
			}
			return m, detachNoteCmd(m.now, n.id), true
		}
	case "Y":
		// Yesterday quick-drill: open the day-detail for yesterday without
		// the History → search → enter detour. Backend returns "no sessions"
		// if yesterday has none, which is fine.
		yest := m.now.AddDate(0, 0, -1)
		mm, cmd := m.openDayDetail(yest)
		return mm, cmd, true
	}
	return m, nil, false
}

// handleWeekKey routes week-only keys.
func (m Model) handleWeekKey(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "h", "[":
		m.weekRef = m.weekRefOrNow().AddDate(0, 0, -7)
		m.weekLoaded = false
		return m, loadWeekCmd(m.weekRef), true
	case "l", "]":
		m.weekRef = m.weekRefOrNow().AddDate(0, 0, 7)
		m.weekLoaded = false
		return m, loadWeekCmd(m.weekRef), true
	case "t", "T":
		// Jump back to current week. T is the project-wide "now"-jump key.
		m.weekRef = m.now
		m.weekLoaded = false
		return m, loadWeekCmd(m.weekRef), true
	case "enter":
		if m.weekCur >= 0 && m.weekCur < len(m.week) {
			d := m.week[m.weekCur].Date
			// Drill on today switches to the live Heute tab instead of the
			// drill dialog — the Today view has more affordances and is the
			// canonical surface for "right now".
			if sameDay(d, m.now) {
				mm, cmd := m.gotoView(viewToday)
				return mm, cmd, true
			}
			model, cmd := m.openDayDetail(d)
			return model, cmd, true
		}
	}
	return m, nil, false
}

// handleHistoryKey routes history-only keys.
func (m Model) handleHistoryKey(key string, _ tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	if m.histMode == historyMonth {
		if mm, cmd, ok := m.handleHistoryMonthKey(key); ok {
			return mm, cmd, true
		}
	}
	if m.histMode == historyTagClock {
		switch key {
		case "h", "left":
			m.tagClockCol = (m.tagClockCol + 23) % 24
			if m.width > 0 {
				m.histVp.SetContent(m.renderHistoryContent())
			}
			return m, nil, true
		case "l", "right":
			m.tagClockCol = (m.tagClockCol + 1) % 24
			if m.width > 0 {
				m.histVp.SetContent(m.renderHistoryContent())
			}
			return m, nil, true
		case "j", "down":
			m.tagClockRow = (m.tagClockRow + 1) % 7
			if m.width > 0 {
				m.histVp.SetContent(m.renderHistoryContent())
			}
			return m, nil, true
		case "k", "up":
			m.tagClockRow = (m.tagClockRow + 6) % 7
			if m.width > 0 {
				m.histVp.SetContent(m.renderHistoryContent())
			}
			return m, nil, true
		case "T":
			// Anchor cursor at "now" in tagclock: today's weekday + current hour.
			row := int(m.now.Weekday()) - 1
			if row < 0 {
				row = 6 // Sunday
			}
			m.tagClockRow = row
			m.tagClockCol = m.now.Hour()
			if m.width > 0 {
				m.histVp.SetContent(m.renderHistoryContent())
			}
			return m, nil, true
		}
	}
	if m.histMode == historyHeatmap {
		switch key {
		case "h", "left":
			m.heatCol = max(0, m.heatCol-1)
			if m.width > 0 {
				m.histVp.SetContent(m.renderHistoryContent())
			}
			return m, nil, true
		case "l", "right":
			weeks, _ := m.heatmapWeeks()
			m.heatCol = min(weeks-1, m.heatCol+1)
			if m.width > 0 {
				m.histVp.SetContent(m.renderHistoryContent())
			}
			return m, nil, true
		case "j", "down":
			m.heatRow = min(6, m.heatRow+1)
			if m.width > 0 {
				m.histVp.SetContent(m.renderHistoryContent())
			}
			return m, nil, true
		case "k", "up":
			m.heatRow = max(0, m.heatRow-1)
			if m.width > 0 {
				m.histVp.SetContent(m.renderHistoryContent())
			}
			return m, nil, true
		case "enter":
			if d, ok := m.heatmapDateAt(m.heatCol, m.heatRow); ok {
				model, cmd := m.openDayDetail(d)
				return model, cmd, true
			}
			return m, nil, true
		case "[":
			// Pan the visible heatmap window backward by ~half its width. The
			// cursor stays at the same calendar date so the user keeps their
			// reference point.
			m.heatOffsetWeeks -= 13
			if m.width > 0 {
				m.histVp.SetContent(m.renderHistoryContent())
			}
			m.heatCol, m.heatRow = m.heatmapTodayCell()
			return m, nil, true
		case "]":
			m.heatOffsetWeeks += 13
			if m.heatOffsetWeeks > 0 {
				m.heatOffsetWeeks = 0
			}
			if m.width > 0 {
				m.histVp.SetContent(m.renderHistoryContent())
			}
			m.heatCol, m.heatRow = m.heatmapTodayCell()
			return m, nil, true
		case "T":
			// In heatmap mode, T resets both offset and cursor to "today".
			m.heatOffsetWeeks = 0
			if m.width > 0 {
				m.histVp.SetContent(m.renderHistoryContent())
			}
			m.heatCol, m.heatRow = m.heatmapTodayCell()
			return m, nil, true
		}
	}
	switch key {
	case "v":
		switch m.histMode {
		case historyList:
			m.histMode = historyHeatmap
			// Anchor cursor at the date the user had focused in the list,
			// not always today — that way "find day in list, then v" lands
			// on the right cell.
			records := m.filteredHistory()
			if m.histCur >= 0 && m.histCur < len(records) {
				m.heatCol, m.heatRow = m.heatmapCellFor(records[m.histCur].Date)
			} else {
				m.heatCol, m.heatRow = m.heatmapTodayCell()
			}
		case historyHeatmap:
			m.histMode = historyTagClock
			// Anchor cursor on "now" so the user immediately sees where they
			// typically work. h/l shifts hour, j/k shifts weekday.
			row := int(m.now.Weekday()) - 1
			if row < 0 {
				row = 6
			}
			m.tagClockRow = row
			m.tagClockCol = m.now.Hour()
		case historyTagClock:
			m.histMode = historyMonth
			// Default month: try to anchor on the date the user had focused
			// in the list, otherwise fall back to today. Cursor on that day.
			anchor := m.now
			records := m.filteredHistory()
			if m.histCur >= 0 && m.histCur < len(records) {
				anchor = records[m.histCur].Date
			}
			m.monthRef = time.Date(anchor.Year(), anchor.Month(), 1, 0, 0, 0, 0, anchor.Location())
			m.monthCur = anchor.Day()
		default:
			m.histMode = historyList
		}
		if m.width > 0 {
			m.histVp.SetContent(m.renderHistoryContent())
		}
		return m, nil, true
	case "/":
		m.dialog = dialogHistFilter
		m.input.Placeholder = "KWxx · YYYY · YYYY-MM · tag:foo · note:bar · leer=alle"
		m.input.SetValue(m.histQuery)
		m.input.Focus()
		return m, tea.Batch(textinput.Blink, loadRecentTagsCmd()), true
	case "F":
		// Quick tag-filter picker: open the filter dialog pre-filled with
		// "tag:" so the user only has to pick a tag from the suggestions strip.
		m.dialog = dialogHistFilter
		m.input.Placeholder = "tag:NAME — Suggestions unten"
		m.input.SetValue("tag:")
		m.input.CursorEnd()
		m.input.Focus()
		return m, tea.Batch(textinput.Blink, loadRecentTagsCmd()), true
	case "[", "]":
		// Paginate the active filter context.
		// "" → seed with current month; KWnn / YYYY-MM / YYYY → step ±1.
		dir := -1
		if key == "]" {
			dir = +1
		}
		next, ok := stepHistFilter(m.histQuery, m.now, dir)
		if !ok {
			return m, nil, true
		}
		m.histQuery = next
		m.histCur = 0
		if m.width > 0 {
			m.histVp.SetContent(m.renderHistoryContent())
			m.histVp.GotoTop()
		}
		return m, nil, true
	case "T":
		// Reset filter — jump back to "all" (and to top, which is newest).
		m.histQuery = ""
		m.histCur = 0
		if m.width > 0 {
			m.histVp.SetContent(m.renderHistoryContent())
			m.histVp.GotoTop()
		}
		return m, nil, true
	case "y":
		// Yank: copy the focused day's stats as Markdown to stdout via tmux
		// load-buffer. Works across History-list and -heatmap modes.
		var d time.Time
		if m.histMode == historyHeatmap {
			if dd, ok := m.heatmapDateAt(m.heatCol, m.heatRow); ok {
				d = dd
			}
		} else {
			records := m.filteredHistory()
			if m.histCur >= 0 && m.histCur < len(records) {
				d = records[m.histCur].Date
			}
		}
		if !d.IsZero() {
			return m, yankDayMarkdownCmd(d), true
		}
		return m, nil, true
	case "Y":
		// Yank the entire current filter range as a Markdown brief.
		return m, yankBriefMarkdownCmd(m.histQuery, m.now), true
	case "enter":
		if m.histCur >= 0 && m.histCur < len(m.history) {
			d := m.history[m.histCur].Date
			if sameDay(d, m.now) {
				mm, cmd := m.gotoView(viewToday)
				return mm, cmd, true
			}
			model, cmd := m.openDayDetail(d)
			return model, cmd, true
		}
	}
	return m, nil, false
}

// handleHistoryMonthKey routes navigation, pagination and drilldown keys for
// the month-grid mode. Returns ok=false to fall through to shared keys
// (filter, yank, etc.) that aren't month-specific.
func (m Model) handleHistoryMonthKey(key string) (tea.Model, tea.Cmd, bool) {
	if m.monthRef.IsZero() {
		m.monthRef = time.Date(m.now.Year(), m.now.Month(), 1, 0, 0, 0, 0, m.now.Location())
		m.monthCur = m.now.Day()
	}
	switch key {
	case "h", "left":
		m.monthCur = monthClampDay(m.monthRef, m.monthCur-1)
		if m.width > 0 {
			m.histVp.SetContent(m.renderHistoryContent())
		}
		return m, nil, true
	case "l", "right":
		m.monthCur = monthClampDay(m.monthRef, m.monthCur+1)
		if m.width > 0 {
			m.histVp.SetContent(m.renderHistoryContent())
		}
		return m, nil, true
	case "j", "down":
		m.monthCur = monthClampDay(m.monthRef, m.monthCur+7)
		if m.width > 0 {
			m.histVp.SetContent(m.renderHistoryContent())
		}
		return m, nil, true
	case "k", "up":
		m.monthCur = monthClampDay(m.monthRef, m.monthCur-7)
		if m.width > 0 {
			m.histVp.SetContent(m.renderHistoryContent())
		}
		return m, nil, true
	case "[":
		m.monthRef = m.monthRef.AddDate(0, -1, 0)
		m.monthCur = monthClampDay(m.monthRef, m.monthCur)
		if m.width > 0 {
			m.histVp.SetContent(m.renderHistoryContent())
		}
		return m, nil, true
	case "]":
		// Don't paginate past the current month — there's nothing newer.
		next := m.monthRef.AddDate(0, 1, 0)
		if !next.After(time.Date(m.now.Year(), m.now.Month(), 1, 0, 0, 0, 0, m.now.Location())) {
			m.monthRef = next
			m.monthCur = monthClampDay(m.monthRef, m.monthCur)
			if m.width > 0 {
				m.histVp.SetContent(m.renderHistoryContent())
			}
		}
		return m, nil, true
	case "T":
		m.monthRef = time.Date(m.now.Year(), m.now.Month(), 1, 0, 0, 0, 0, m.now.Location())
		m.monthCur = m.now.Day()
		if m.width > 0 {
			m.histVp.SetContent(m.renderHistoryContent())
		}
		return m, nil, true
	case "enter":
		d := time.Date(m.monthRef.Year(), m.monthRef.Month(), m.monthCur, 0, 0, 0, 0, m.monthRef.Location())
		if sameDay(d, m.now) {
			mm, cmd := m.gotoView(viewToday)
			return mm, cmd, true
		}
		mm, cmd := m.openDayDetail(d)
		return mm, cmd, true
	}
	return m, nil, false
}

// monthClampDay clamps `day` to the [1, last-of-month] range for `monthRef`.
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

// stepHistFilter advances the active filter expression by `dir` units. The
// unit is inferred from the syntax: KWnn → ±1 week, YYYY-MM → ±1 month,
// YYYY → ±1 year, tag:* → unchanged (returns ok=false). An empty filter is
// seeded to the current month so paging works without a manual filter step.
func stepHistFilter(q string, now time.Time, dir int) (string, bool) {
	q = strings.TrimSpace(q)
	if q == "" {
		// Seed: current ISO week. Stepping immediately produces previous/next.
		_, wn := now.ISOWeek()
		seed := fmt.Sprintf("KW%d", wn)
		return stepHistFilter(seed, now, dir)
	}
	if strings.HasPrefix(strings.ToLower(q), "tag:") || strings.HasPrefix(strings.ToLower(q), "note:") {
		return q, false
	}
	if strings.HasPrefix(strings.ToUpper(q), "KW") {
		var w int
		if _, err := fmt.Sscanf(strings.ToUpper(q), "KW%d", &w); err != nil {
			return q, false
		}
		// Walk dir × 7 days from the Monday of this week and re-extract KW.
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

// isoMondayOfISOWeek returns the Monday 00:00 of the given ISO year+week.
// Approximates by walking from January 4th (always in ISO week 1).
func isoMondayOfISOWeek(year, week int, loc *time.Location) time.Time {
	jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, loc)
	wd := int(jan4.Weekday())
	if wd == 0 {
		wd = 7
	}
	mon1 := jan4.AddDate(0, 0, -(wd - 1))
	return mon1.AddDate(0, 0, 7*(week-1))
}

// heatmapCellFor returns the (col, row) for a given calendar date, clamped
// to the visible heatmap. Mirrors heatmapTodayCell but parameterised.
func (m Model) heatmapCellFor(d time.Time) (int, int) {
	startMon, weeks := m.heatmapBounds()
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

// — heatmap cursor helpers —

// heatmapBounds returns the start-Monday and number of weeks in the current
// heatmap. Honours `m.heatOffsetWeeks` so [/] in heatmap mode pans the visible
// window. Returns zero/0 when the history is empty.
func (m Model) heatmapBounds() (time.Time, int) {
	records := m.filteredHistory()
	if len(records) == 0 {
		return time.Time{}, 0
	}
	newest := records[0].Date
	oldest := records[len(records)-1].Date
	if m.heatOffsetWeeks != 0 {
		// Shift the right edge of the window by offset*7 days. We cap it at
		// the newest record (no future panning) and at the oldest minus 25
		// weeks (no panning past existing data).
		shifted := newest.AddDate(0, 0, 7*m.heatOffsetWeeks)
		if shifted.After(newest) {
			shifted = newest
		}
		minEdge := isoMonday(oldest).AddDate(0, 0, 7*0)
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

func (m Model) heatmapWeeks() (int, time.Time) {
	start, weeks := m.heatmapBounds()
	return weeks, start
}

// heatmapDateAt resolves a (col, row) to a calendar date inside the visible
// heatmap. Returns ok=false when the input is empty / out of bounds.
func (m Model) heatmapDateAt(col, row int) (time.Time, bool) {
	startMon, weeks := m.heatmapBounds()
	if weeks == 0 || col < 0 || col >= weeks || row < 0 || row > 6 {
		return time.Time{}, false
	}
	return startMon.AddDate(0, 0, 7*col+row), true
}

// heatmapTodayCell finds the column/row that points at today, clamped.
func (m Model) heatmapTodayCell() (int, int) {
	startMon, weeks := m.heatmapBounds()
	if weeks == 0 {
		return 0, 0
	}
	row := int(m.now.Weekday()) - 1
	if row < 0 {
		row = 6 // Sunday → row 6
	}
	mon := isoMonday(m.now)
	col := int(mon.Sub(startMon).Hours() / 24 / 7)
	if col < 0 {
		col = 0
	}
	if col >= weeks {
		col = weeks - 1
	}
	return col, row
}

func (m Model) openStartStopDialog() (tea.Model, tea.Cmd) {
	if m.day.IsRunning() {
		// Smart suggestion: when the running session is very short, the user
		// probably hit `s` by accident or is about to take a quick break.
		// Offer a choice instead of silently committing the stop.
		if m.day.Active != nil && m.now.Sub(*m.day.Active) < 5*time.Minute {
			m.dialog = dialogStopChoice
			return m, nil
		}
		m.dialog = dialogStopAt
		m.input.Placeholder = time.Now().Format("15:04") + "  ·  -30m  ·  Enter=jetzt"
	} else if m.day.IsPaused() {
		// Pause-aware: `s` resumes immediately. The user that wants a fresh
		// start time presses `S` (force-start dialog) instead. Pass PausedAt
		// so the resume toast can spell out "nach 12m Pause (seit 14:32)".
		var since time.Time
		if m.day.PausedAt != nil {
			since = *m.day.PausedAt
		}
		return m, resumeCmd(since)
	} else {
		m.dialog = dialogStart
		m.input.Placeholder = time.Now().Format("15:04") + "  ·  -1h30m  ·  Enter=jetzt"
	}
	m.input.SetValue("")
	m.input.Focus()
	m.errMsg = ""
	return m, textinput.Blink
}

// openForceStartDialog opens a fresh-start dialog regardless of pause state.
// Bound to capital `S`. Useful when the user wants to discard the pause and
// start a new session at a chosen time.
func (m Model) openForceStartDialog() (tea.Model, tea.Cmd) {
	m.dialog = dialogStart
	m.input.Placeholder = time.Now().Format("15:04") + "  ·  -1h30m  ·  Enter=jetzt"
	m.input.SetValue("")
	m.input.Focus()
	m.errMsg = ""
	return m, textinput.Blink
}

// enterFocusMode starts a session if idle, then opens (or surfaces) the daily
// note in a horizontal tmux split. Already-running sessions are left alone —
// the daily note still opens.
func (m Model) enterFocusMode() (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{}
	if !m.day.IsRunning() {
		cmds = append(cmds, startCmd(m.now))
	}
	cmds = append(cmds, openNoteCmd(wt.DailyNoteID(m.now)))
	return m, tea.Batch(cmds...)
}

// handleDayOffsKey routes keys for the "Frei" tab.
func (m Model) handleDayOffsKey(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "a":
		return m.openDayOffAdd(), nil, true
	case "A":
		// Quick-add today as vacation (most common ad-hoc case: "I'm taking
		// today off"). Replaces the form roundtrip.
		return m, addDayOffCmd(m.now.Format("2006-01-02"), wt.KindVacation, "", m.dayoffsYear), true
	case "K":
		// Quick-add today as sick. Mirrors A but for the sick-day case.
		return m, addDayOffCmd(m.now.Format("2006-01-02"), wt.KindSick, "", m.dayoffsYear), true
	case "d", "x":
		if m.dayoffsCur >= 0 && m.dayoffsCur < len(m.dayoffs) {
			m.dialog = dialogDayOffConfirm
			return m, nil, true
		}
	case "h", "left", "[":
		// Previous year.
		year := m.dayoffsYear
		if year == 0 {
			year = m.now.Year()
		}
		year--
		m.dayoffsLoaded = false
		m.dayoffsYear = year
		m.dayoffsCur = 0
		return m, loadDayOffsCmd(year), true
	case "l", "right", "]":
		year := m.dayoffsYear
		if year == 0 {
			year = m.now.Year()
		}
		year++
		m.dayoffsLoaded = false
		m.dayoffsYear = year
		m.dayoffsCur = 0
		return m, loadDayOffsCmd(year), true
	case "t", "T":
		// Jump to current year. T is the project-wide "now"-jump key.
		m.dayoffsLoaded = false
		m.dayoffsYear = m.now.Year()
		m.dayoffsCur = 0
		return m, loadDayOffsCmd(m.now.Year()), true
	case "B":
		// Bundesland sync for the displayed year. Defaults to NW (NRW)
		// because that's the user's home; overridable via WORKTIME_LAND env.
		year := m.dayoffsYear
		if year == 0 {
			year = m.now.Year()
		}
		land := os.Getenv("WORKTIME_LAND")
		if land == "" {
			land = "NW"
		}
		return m, syncGermanHolidaysCmd(year, land), true
	}
	return m, nil, false
}

func (m Model) openDayOffAdd() Model {
	date := newField("Datum", "YYYY-MM-DD oder YYYY-MM-DD..YYYY-MM-DD", m.now.Format("2006-01-02"))
	label := newField("Label", "z.B. Brückentag", "")
	date.Focus()
	m.formInputs = []textinput.Model{date, label}
	m.formCur = 0
	m.dialog = dialogDayOffAdd
	m.dayoffKindCur = 1 // default: vacation (most common manual add)
	m.errMsg = ""
	return m
}

func (m Model) handleDayOffConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "z", "j": // QWERTZ-friendly: explicit confirm. Enter is safe default.
		if m.dayoffsCur >= 0 && m.dayoffsCur < len(m.dayoffs) {
			d := m.dayoffs[m.dayoffsCur].Date
			m.dialog = dialogNone
			return m, removeDayOffCmd(d, m.dayoffsYear)
		}
		m.dialog = dialogNone
		return m, nil
	case "n", "esc", "enter":
		m.dialog = dialogNone
		return m, nil
	}
	return m, nil
}

func (m Model) handleStopChoiceKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "z", "j":
		// Explicit confirm: stop now. (`z` covers QWERTZ users hitting the
		// wrong y; `j` for "ja" matches Delete/DayOff confirm.) Enter is
		// bound to the safe "weiterlaufen" default below.
		m.dialog = dialogNone
		return m, stopAtCmd(m.now)
	case "t":
		// Time-pick: switch to the standard stop-at dialog.
		m.dialog = dialogStopAt
		m.input.Placeholder = time.Now().Format("15:04") + "  ·  -30m  ·  Enter=jetzt"
		m.input.SetValue("")
		m.input.Focus()
		m.errMsg = ""
		return m, textinput.Blink
	case "n", "esc", "enter":
		// Keep running — Enter is the safe default for an "are you sure"
		// prompt that fires after a near-empty session.
		m.dialog = dialogNone
		return m, nil
	}
	return m, nil
}

// — handleFormKey extension: cycle Kind via Tab when on dialogDayOffAdd —

// Tab on dayoff-add cycles through (date, label, kind-cycler) — but kind is a
// pseudo-field rendered as picker, not a textinput. We treat Tab on the kind
// "field" as cycling through worktime.AllKinds.

func (m Model) openCorrectDialog() (tea.Model, tea.Cmd) {
	if !m.day.IsRunning() || m.day.Active == nil {
		return m, nil
	}
	m.dialog = dialogCorrect
	m.input.Placeholder = "neue Startzeit"
	m.input.SetValue(m.day.Active.Format("15:04"))
	m.input.Focus()
	return m, textinput.Blink
}

func (m Model) openEntryDialog() (tea.Model, tea.Cmd) {
	m.dialog = dialogEntryForm
	m.buildEntryForm(time.Now())
	return m, tea.Batch(textinput.Blink, loadTemplatesCmd())
}

func (m Model) openEditDialog() (tea.Model, tea.Cmd) {
	if !m.onSession() {
		return m, nil
	}
	s := m.day.Sessions[m.sessionIdx()]
	m.editDate = s.Date
	m.editIdx = m.sessionIdx()
	m.dialog = dialogEditForm
	m.buildEditForm(s)
	return m, textinput.Blink
}

// openEditFromDrill is the same as openEditDialog but works against the day
// the user drilled into rather than today.
func (m Model) openEditFromDrill() (tea.Model, tea.Cmd) {
	if m.drillCur < 0 || m.drillCur >= len(m.drillSessions) {
		return m, nil
	}
	s := m.drillSessions[m.drillCur]
	m.editDate = m.drillDate
	m.editIdx = m.drillCur
	m.dialog = dialogEditForm
	m.buildEditForm(s)
	return m, textinput.Blink
}

func (m Model) openTagDialog(date time.Time, idx int, current string) (tea.Model, tea.Cmd) {
	m.editDate = date
	m.editIdx = idx
	m.dialog = dialogTagForm
	m.input.Placeholder = "tag (z.B. deep, meeting, support)"
	m.input.SetValue(current)
	m.input.Focus()
	m.tagSugCur = -1
	m.errMsg = ""
	return m, tea.Batch(textinput.Blink, loadRecentTagsCmd())
}

func (m Model) openNoteDialogForSession(date time.Time, idx int, current string) (tea.Model, tea.Cmd) {
	m.editDate = date
	m.editIdx = idx
	m.dialog = dialogNoteForm
	m.input.Placeholder = "kurzer Text  ·  Enter=speichern"
	m.input.SetValue(current)
	m.input.Focus()
	return m, textinput.Blink
}

func (m Model) openDeleteDialog() (tea.Model, tea.Cmd) {
	if !m.onSession() {
		return m, nil
	}
	m.editIdx = m.sessionIdx()
	m.editDate = m.day.Sessions[m.sessionIdx()].Date
	m.dialog = dialogDeleteConfirm
	return m, nil
}

func (m Model) openDeleteFromDrill() (tea.Model, tea.Cmd) {
	if m.drillCur < 0 || m.drillCur >= len(m.drillSessions) {
		return m, nil
	}
	m.editIdx = m.drillCur
	m.editDate = m.drillDate
	m.dialog = dialogDeleteConfirm
	return m, nil
}

// openDayDetail opens the drill-down sub-view for the given date.
func (m Model) openDayDetail(date time.Time) (tea.Model, tea.Cmd) {
	m.drillDate = startOfDay(date)
	m.drillCur = 0
	m.dialog = dialogDayDetail
	return m, loadDayDetailCmd(m.drillDate)
}

// openHelp opens the worktime-local help overlay.
func (m Model) openHelp() (tea.Model, tea.Cmd) {
	m.dialog = dialogHelp
	return m, nil
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func (m Model) openNotePicker() (tea.Model, tea.Cmd) {
	m.dialog = dialogNotePicker
	m.picker = nil
	m.pickerCur = 0
	m.pickerErr = nil
	m.input.Placeholder = "filter (typ oder substring)"
	m.input.SetValue("")
	m.input.Focus()
	return m, tea.Batch(textinput.Blink, loadNotesCmd())
}

func (m Model) filteredPicker() []wt.KompendiumNote {
	q := strings.ToLower(strings.TrimSpace(m.input.Value()))
	if q == "" {
		return m.picker
	}
	out := make([]wt.KompendiumNote, 0, len(m.picker))
	for _, n := range m.picker {
		if strings.Contains(strings.ToLower(n.ID), q) ||
			strings.Contains(strings.ToLower(n.Type), q) ||
			strings.Contains(strings.ToLower(n.Project), q) {
			out = append(out, n)
		}
	}
	return out
}

func (m Model) handleNotePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.dialog = dialogNone
		m.input.Blur()
		m.input.SetValue("")
		return m, nil
	case tea.KeyEnter:
		filtered := m.filteredPicker()
		if m.pickerCur < 0 || m.pickerCur >= len(filtered) {
			return m, nil
		}
		picked := filtered[m.pickerCur]
		m.dialog = dialogNone
		m.input.Blur()
		m.input.SetValue("")
		// Toggle: if the note is already attached for today, detach it.
		// Daily notes auto-appear and shouldn't be detachable here.
		for _, n := range m.notes {
			if n.id == picked.ID && !n.isDaily {
				return m, detachNoteCmd(m.now, picked.ID)
			}
		}
		return m, attachNoteCmd(m.now, picked.ID)
	case tea.KeyUp:
		if m.pickerCur > 0 {
			m.pickerCur--
		}
		return m, nil
	case tea.KeyDown:
		if m.pickerCur < len(m.filteredPicker())-1 {
			m.pickerCur++
		}
		return m, nil
	}
	// Forward all other keys to the filter input.
	prev := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != prev {
		m.pickerCur = 0
	}
	return m, cmd
}

func (m Model) moveCursor(delta int) (tea.Model, tea.Cmd) {
	switch m.view {
	case viewToday:
		total := m.totalRows()
		if total == 0 {
			return m, nil
		}
		m.cursor = clamp(m.cursor+delta, 0, total-1)
	case viewWeek:
		if len(m.week) == 0 {
			return m, nil
		}
		m.weekCur = clamp(m.weekCur+delta, 0, len(m.week)-1)
	case viewHistory:
		if len(m.history) == 0 {
			return m, nil
		}
		m.histCur = clamp(m.histCur+delta, 0, len(m.history)-1)
		// Forward to viewport so it scrolls along.
		if delta > 0 {
			m.histVp.ScrollDown(delta)
		} else {
			m.histVp.ScrollUp(-delta)
		}
	case viewDayOffs:
		if len(m.dayoffs) == 0 {
			return m, nil
		}
		m.dayoffsCur = clamp(m.dayoffsCur+delta, 0, len(m.dayoffs)-1)
	}
	return m, nil
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (m *Model) clampCursor() {
	total := m.totalRows()
	if total == 0 {
		m.cursor = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	} else if m.cursor >= total {
		m.cursor = total - 1
	}
}

func (m Model) handleDialogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.dialog {
	case dialogNotePicker:
		return m.handleNotePickerKey(msg)
	case dialogDeleteConfirm:
		return m.handleDeleteConfirmKey(msg)
	case dialogDayOffConfirm:
		return m.handleDayOffConfirmKey(msg)
	case dialogStopChoice:
		return m.handleStopChoiceKey(msg)
	case dialogDayDetail:
		return m.handleDayDetailKey(msg)
	case dialogHelp:
		// Only specific keys close the overlay — accidental j/k/letters keep
		// the help up so the user can read it without playing whack-a-mole
		// with their cursor finger.
		switch msg.String() {
		case "esc", "q", "?", "enter", " ":
			m.dialog = dialogNone
		}
		return m, nil
	case dialogEntryForm, dialogEditForm, dialogDayOffAdd:
		return m.handleFormKey(msg)
	case dialogTagForm:
		return m.handleTagFormKey(msg)
	}

	// All remaining dialogs are single-input.
	switch msg.Type {
	case tea.KeyEsc:
		m.dialog = dialogNone
		m.input.Blur()
		m.input.SetValue("")
		m.errMsg = ""
		return m, nil
	case tea.KeyEnter:
		return m.confirmDialog()
	}
	m.errMsg = "" // typing clears stale errors
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// handleTagFormKey is single-input but with Tab-cycling through two
// suggestion strips: Tab walks the recency strip, Shift+Tab walks the
// top-by-usage strip. Both can also be reached by typing — autocomplete
// stays the primary path.
func (m Model) handleTagFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.dialog = dialogNone
		m.input.Blur()
		m.input.SetValue("")
		m.errMsg = ""
		m.tagSugCur = -1
		m.topSugCur = -1
		return m, nil
	case tea.KeyEnter:
		return m.confirmDialog()
	case tea.KeyTab:
		if len(m.recentTags) == 0 {
			return m, nil
		}
		m.tagSugCur = (m.tagSugCur + 1) % len(m.recentTags)
		m.topSugCur = -1
		m.input.SetValue(m.recentTags[m.tagSugCur])
		return m, nil
	case tea.KeyShiftTab:
		if len(m.topTags) == 0 {
			return m, nil
		}
		m.topSugCur = (m.topSugCur + 1) % len(m.topTags)
		m.tagSugCur = -1
		m.input.SetValue(m.topTags[m.topSugCur])
		return m, nil
	}
	m.errMsg = ""
	m.tagSugCur = -1
	m.topSugCur = -1
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) handleDeleteConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "z", "j":
		// Explicit confirm only — Enter is the safe default (cancel).
		idx := m.editIdx
		date := m.editDate
		// Source-of-truth list depends on which screen launched the delete.
		var src []wt.Session
		switch {
		case sameDay(date, m.now) && idx < len(m.day.Sessions):
			src = m.day.Sessions
		case len(m.drillSessions) > 0:
			src = m.drillSessions
		}
		if idx < len(src) {
			m.lastDeleted = &undoEntry{date: date, session: src[idx]}
		}
		if sameDay(date, m.now) && m.cursor >= len(m.day.Sessions)-1 {
			m.cursor = max(0, len(m.day.Sessions)-2)
		}
		m.dialog = dialogNone
		return m, deleteCmd(date, idx)
	case "n", "esc", "enter":
		m.dialog = dialogNone
		return m, nil
	}
	return m, nil
}

func (m Model) handleFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// In the dayoff-add form there's a virtual third "field" — the kind picker.
	// formCur == len(formInputs) targets that virtual field.
	maxCur := len(m.formInputs) - 1
	if m.dialog == dialogDayOffAdd {
		maxCur = len(m.formInputs)
	}
	onKindField := m.dialog == dialogDayOffAdd && m.formCur == len(m.formInputs)

	// Ctrl+T cycles session templates in the entry form: each press fills
	// date/start/stop with the next template's shape. Skip when no templates
	// loaded or in non-entry forms (edit/dayoff have their own semantics).
	if m.dialog == dialogEntryForm && msg.String() == "ctrl+t" && len(m.templates) > 0 {
		m.templateCur = (m.templateCur + 1) % len(m.templates)
		m = m.applyTemplate(m.templates[m.templateCur])
		return m, nil
	}

	switch msg.Type {
	case tea.KeyEsc:
		m.dialog = dialogNone
		m.clearForm()
		m.errMsg = ""
		return m, nil
	case tea.KeyTab, tea.KeyDown:
		next := m.formCur + 1
		if next > maxCur {
			next = 0
		}
		m.focusForm(next)
		return m, textinput.Blink
	case tea.KeyShiftTab, tea.KeyUp:
		next := m.formCur - 1
		if next < 0 {
			next = maxCur
		}
		m.focusForm(next)
		return m, textinput.Blink
	case tea.KeyEnter:
		// Enter on the last targetable field submits; on earlier fields advances.
		if m.formCur < maxCur {
			m.focusForm(m.formCur + 1)
			return m, textinput.Blink
		}
		return m.submitForm()
	}
	if onKindField {
		switch msg.String() {
		case "h", "left":
			if m.dayoffKindCur > 0 {
				m.dayoffKindCur--
			} else {
				m.dayoffKindCur = len(wt.AllKinds) - 1
			}
			return m, nil
		case "l", "right", " ":
			m.dayoffKindCur = (m.dayoffKindCur + 1) % len(wt.AllKinds)
			return m, nil
		}
		return m, nil
	}
	if m.formCur >= 0 && m.formCur < len(m.formInputs) {
		// Typing into a form clears any inline error so the user isn't staring
		// at a stale red line while fixing the input.
		m.errMsg = ""
		var cmd tea.Cmd
		m.formInputs[m.formCur], cmd = m.formInputs[m.formCur].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) submitForm() (tea.Model, tea.Cmd) {
	values := m.formValues()
	switch m.dialog {
	case dialogEntryForm:
		if len(values) != 3 {
			return m, nil
		}
		date := values[0]
		if date == "" {
			date = time.Now().Format("2006-01-02")
		}
		// Live-validate before dispatching: surfaces malformed time inputs and
		// stop-before-start without round-tripping through actionDoneMsg.
		if errStr := validateEntry(date, values[1], values[2]); errStr != "" {
			m.errMsg = errStr
			return m, nil
		}
		if errStr := m.overlapCheck(date, values[1], values[2], -1); errStr != "" {
			m.errMsg = errStr
			return m, nil
		}
		return m, addManualCmd(date, values[1], values[2])
	case dialogEditForm:
		if len(values) < 2 {
			return m, nil
		}
		if errStr := validateEntry("", values[0], values[1]); errStr != "" {
			m.errMsg = errStr
			return m, nil
		}
		if errStr := m.overlapCheck(m.editDate.Format("2006-01-02"), values[0], values[1], m.editIdx); errStr != "" {
			m.errMsg = errStr
			return m, nil
		}
		tag, note := "", ""
		if len(values) >= 3 {
			tag = values[2]
		}
		if len(values) >= 4 {
			note = values[3]
		}
		return m, editFullCmd(m.editDate, m.editIdx, values[0], values[1], tag, note)
	case dialogDayOffAdd:
		if len(values) < 2 {
			return m, nil
		}
		dateExpr := values[0]
		label := ""
		if len(values) >= 2 {
			label = values[1]
		}
		kind := wt.AllKinds[m.dayoffKindCur%len(wt.AllKinds)]
		return m, addDayOffCmd(dateExpr, kind, label, m.dayoffsYear)
	}
	return m, nil
}

// applyTemplate fills the entry-form fields from a SessionTemplate. Date
// stays at "today" so the user can press Enter immediately; start and stop
// are rendered as HH:MM. Existing field values are overwritten — the user
// can still edit afterwards.
func (m Model) applyTemplate(t wt.SessionTemplate) Model {
	if len(m.formInputs) < 3 {
		return m
	}
	startH := int(t.Start.Hours())
	startM := int(t.Start.Minutes()) % 60
	stop := t.Start + t.Duration
	stopH := int(stop.Hours()) % 24
	stopM := int(stop.Minutes()) % 60
	m.formInputs[0].SetValue(time.Now().Format("2006-01-02"))
	m.formInputs[1].SetValue(fmt.Sprintf("%02d:%02d", startH, startM))
	m.formInputs[2].SetValue(fmt.Sprintf("%02d:%02d", stopH, stopM))
	return m
}

// overlapCheck reports a user-facing string when the given (start, stop)
// overlaps an existing session on the same date (other than `skipIdx`, which
// is the session being edited). Catches errors before round-tripping through
// the backend's ErrOverlap so the user sees the conflicting times directly.
//
// Empty stop on today is treated as "now"; on other days it skips the check
// (the backend will reject anyway, but with a clearer error).
func (m Model) overlapCheck(dateStr, startStr, stopStr string, skipIdx int) string {
	date, err := time.ParseInLocation("2006-01-02", dateStr, m.now.Location())
	if err != nil {
		return ""
	}
	startD, err := wt.ParseHM(startStr)
	if err != nil {
		return ""
	}
	base := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	startTime := base.Add(startD)
	var stopTime time.Time
	switch {
	case stopStr == "":
		if !sameDay(date, m.now) {
			return ""
		}
		stopTime = m.now
	case stopStr[0] == '+':
		t, err := wt.ParseStop(stopStr, startTime)
		if err != nil {
			return ""
		}
		stopTime = t
	default:
		stopD, err := wt.ParseHM(stopStr)
		if err != nil {
			return ""
		}
		stopTime = base.Add(stopD)
	}
	if !stopTime.After(startTime) {
		return ""
	}
	var existing []wt.Session
	switch {
	case sameDay(date, m.now):
		existing = m.day.Sessions
	case sameDay(date, m.drillDate):
		existing = m.drillSessions
	default:
		// We don't have other dates loaded in memory; defer to backend.
		return ""
	}
	for i, s := range existing {
		if i == skipIdx {
			continue
		}
		// Half-open interval test: [a,b) overlaps [c,d) iff a < d && c < b.
		if startTime.Before(s.Stop) && s.Start.Before(stopTime) {
			return fmt.Sprintf("Überschneidet Session %d (%s → %s)",
				i+1, s.Start.Format("15:04"), s.Stop.Format("15:04"))
		}
	}
	return ""
}

// validateEntry parses date + start + stop. date may be "" when the form has
// no date field (edit mode). Returns the first user-facing error, or "" on
// success.
//
// Stop accepts an additional "+1h30m" shorthand (duration relative to start);
// validation handles that path separately because ParseStartArg rejects it.
func validateEntry(date, startStr, stopStr string) string {
	if date != "" {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return "Datum ungültig: " + date + " (YYYY-MM-DD)"
		}
	}
	if _, err := wt.ParseStartArg(startStr); err != nil {
		return "Start: " + err.Error()
	}
	// Stop may be "+1h30m" — parse via ParseStop so the +-form passes validation.
	startD, errStartD := wt.ParseHM(startStr)
	now := time.Now()
	startAnchor := now
	if errStartD == nil {
		startAnchor = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(startD)
	}
	if stopStr != "" {
		if _, err := wt.ParseStop(stopStr, startAnchor); err != nil {
			return "Stop: " + err.Error()
		}
	}
	if errStartD == nil && stopStr != "" {
		if stopStr[0] != '+' {
			stopD, err := wt.ParseHM(stopStr)
			if err == nil && stopD <= startD {
				return "Stop muss nach Start liegen"
			}
		}
		// "+Xh" forms are inherently positive, no extra check required.
	}
	return ""
}

func (m Model) handleDayDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "b":
		m.dialog = dialogNone
		m.drillSessions = nil
		return m, nil
	case "j", "down":
		if m.drillCur < len(m.drillSessions)-1 {
			m.drillCur++
		}
		return m, nil
	case "k", "up":
		if m.drillCur > 0 {
			m.drillCur--
		}
		return m, nil
	case "g":
		m.drillCur = 0
		return m, nil
	case "G":
		m.drillCur = max(0, len(m.drillSessions)-1)
		return m, nil
	case "E", "enter":
		return m.openEditFromDrill()
	case "d":
		return m.openDeleteFromDrill()
	case "t":
		if m.drillCur < len(m.drillSessions) {
			s := m.drillSessions[m.drillCur]
			return m.openTagDialog(m.drillDate, m.drillCur, s.Tag)
		}
	case "N":
		if m.drillCur < len(m.drillSessions) {
			s := m.drillSessions[m.drillCur]
			return m.openNoteDialogForSession(m.drillDate, m.drillCur, s.Note)
		}
	}
	return m, nil
}

func (m Model) confirmDialog() (tea.Model, tea.Cmd) {
	val := strings.TrimSpace(m.input.Value())

	switch m.dialog {
	case dialogStart:
		ts, err := wt.ParseStartArg(val)
		if err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
		return m, startCmd(ts)

	case dialogStopAt:
		ts, err := wt.ParseStartArg(val)
		if err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
		return m, stopAtCmd(ts)

	case dialogCorrect:
		ts, err := wt.ParseStartArg(val)
		if err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
		return m, correctCmd(ts)

	case dialogTagForm:
		m.tagSugCur = -1
		return m, setTagCmd(m.editDate, m.editIdx, val)

	case dialogNoteForm:
		return m, setNoteCmd(m.editDate, m.editIdx, val)

	case dialogHistFilter:
		m.histQuery = val
		m.dialog = dialogNone
		m.input.Blur()
		m.input.SetValue("")
		m.histCur = 0
		if m.width > 0 {
			m.histVp.SetContent(m.renderHistoryContent())
			m.histVp.GotoTop()
		}
		return m, nil
	}
	return m, nil
}

func (m Model) nextView() (tea.Model, tea.Cmd) {
	return m.gotoView((m.view + 1) % subViewCount)
}

func (m Model) prevTab() (tea.Model, tea.Cmd) {
	return m.gotoView((m.view + subViewCount - 1) % subViewCount)
}

func (m Model) gotoView(next subView) (tea.Model, tea.Cmd) {
	if next != m.view {
		m.prevView = m.view
	}
	m.view = next
	var cmd tea.Cmd
	switch next {
	case viewWeek:
		if !m.weekLoaded {
			cmd = loadWeekCmd(m.weekRefOrNow())
		}
	case viewHistory:
		if !m.historyLoaded {
			cmd = loadHistoryCmd()
		}
	case viewDayOffs:
		year := m.dayoffsYear
		if year == 0 {
			year = m.now.Year()
		}
		if !m.dayoffsLoaded {
			cmd = loadDayOffsCmd(year)
		}
	}
	return m, cmd
}

func (m Model) weekRefOrNow() time.Time {
	if m.weekRef.IsZero() {
		return m.now
	}
	return m.weekRef
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
	case viewDayOffs:
		return m.renderDayOffs()
	default:
		return m.renderToday()
	}
}

func (m Model) renderTabBar() string {
	type tab struct {
		idx     string
		label   string
		shortL  string // single-letter fallback for very narrow panes
		view    subView
	}
	tabs := []tab{
		{"1", "Heute", "H", viewToday},
		{"2", "Woche", "W", viewWeek},
		{"3", "History", "Hi", viewHistory},
		{"4", "Frei", "F", viewDayOffs},
	}
	active := lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true)
	inactive := lipgloss.NewStyle().Foreground(m.theme.Dim)
	idxActive := lipgloss.NewStyle().Foreground(m.theme.Accent)
	idxInactive := lipgloss.NewStyle().Foreground(m.theme.Border)

	// Three-step degradation: full labels with " │ " separators, mid uses
	// "·" separator, compact uses single-char labels with " " separator.
	// `inner` = box content width; the bar must fit in there with its 2-space
	// left indent.
	inner := m.width - 4
	if inner <= 0 {
		inner = 80
	}
	type form struct {
		label string
		sep   string
	}
	tries := []form{
		{"full", " │ "},
		{"full", " · "},
		{"short", " · "},
	}
	render := func(labelMode, sep string) string {
		sepR := lipgloss.NewStyle().Foreground(m.theme.Border).Render(sep)
		var parts []string
		for _, t := range tabs {
			lbl := t.label
			if labelMode == "short" {
				lbl = t.shortL
			}
			activeStyle := active
			idxStyle := idxActive
			if t.view != m.view {
				activeStyle = inactive
				idxStyle = idxInactive
			}
			parts = append(parts, idxStyle.Render(t.idx+" ")+activeStyle.Render(lbl))
		}
		return "  " + strings.Join(parts, sepR)
	}
	for _, f := range tries {
		out := render(f.label, f.sep)
		if lipgloss.Width(out) <= inner {
			return out
		}
	}
	// Fall through: still too wide, return the compact form anyway — better
	// than overflowing.
	return render("short", " ")
}

// — today view —

func (m Model) renderToday() string {
	inner := m.width - 4
	var rows []string
	rows = append(rows, m.renderTabBar())

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
	// Title built as chips so boxTitle can drop the lower-priority pieces
	// (clock, weekday) before the corner of the box gets pushed off-screen
	// in narrow tmux panes — that overflow was rendering a "doubled" header.
	title := boxTitle([]string{
		"Worktime",
		fmt.Sprintf("%s, %02d. %s",
			germanWeekday(m.now.Weekday()), m.now.Day(), germanMonth(m.now.Month())),
		m.now.Format("15:04:05"),
		status,
	}, m.width)
	box := titlebox.Render(title, body, m.width, m.theme)

	return box + m.renderToastRow() + "\n" + wrapFooter(m.theme, m.todayFooter(), m.width)
}

func (m Model) todayFooter() string {
	var actions []string
	switch {
	case m.day.IsRunning():
		actions = append(actions, "s stoppen", "p pause", "C startzeit")
	case m.day.IsPaused():
		actions = append(actions, "s resume", "S neu starten")
	default:
		actions = append(actions, "s starten")
	}
	actions = append(actions, "e eintrag", "n attach", "f fokus")

	var ctx []string
	switch {
	case m.onSession():
		ctx = append(ctx, "E/⏎ bearbeiten", "d löschen", "t tag", "N notiz")
	case m.onNote():
		n := m.notes[m.noteIdx()]
		ctx = append(ctx, "⏎ ansehen", "O editor")
		if !n.isDaily {
			ctx = append(ctx, "D detach")
		}
	}
	if m.lastDeleted != nil {
		ctx = append(ctx, "u undo")
	}

	nav := []string{"j/k auswahl", "tab/1·2·3 wechseln", "? hilfe", "b zurück", "q schließen"}

	parts := []string{strings.Join(actions, "  ")}
	if len(ctx) > 0 {
		parts = append(parts, strings.Join(ctx, "  "))
	}
	parts = append(parts, strings.Join(nav, "  "))
	// wrapFooter at the call site handles the per-width line break; here we
	// just produce the canonical "  ·  "-separated chip list.
	return strings.Join(parts, "  ·  ")
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

	// Day-off banner: when today is a configured holiday/vacation/sick day,
	// surface it prominently. Helps the user understand why target == 0.
	var dayOffBanner string
	if dayOff, ok := wt.LookupDayOff(now); ok {
		dayOffBanner = "  " + lipgloss.NewStyle().Foreground(m.theme.Cyan).Bold(true).
			Render(fmt.Sprintf("☼ %s", dayOff.Kind.LabelDe())) +
			"  " + stDim(m.theme, dayOff.Label)
	}

	totalColor := totalThresholdColor(m.theme, total, target, m.day.IsRunning())
	statusGlyph, statusLabel, statusColor := todayStatusBadge(m.theme, m.day.IsRunning(), target == 0 || total >= target)

	// Live seconds only useful in the first 60s of a fresh session — after
	// that the minute-level display avoids the whole header zappling.
	totalText := formatDur(total)
	if m.day.IsRunning() && m.day.Active != nil && m.now.Sub(*m.day.Active) < time.Minute {
		totalText = formatDurLive(total)
	}
	totalStr := lipgloss.NewStyle().Foreground(totalColor).Bold(true).Render(totalText)
	statusStr := lipgloss.NewStyle().Foreground(statusColor).Render(statusGlyph + " " + statusLabel)
	pctStr := lipgloss.NewStyle().Foreground(m.theme.Dim).Render(fmt.Sprintf("%d%%", pct))

	// Primary line: total · status · % — the quick "where am I today" glance.
	headline := "  " + totalStr + "   " + statusStr + "   " + pctStr

	// Secondary line: streak · vs Schnitt · monthly burndown saldo. Demoted
	// from the primary so the headline stays scannable; this row is for
	// follow-up reading.
	var secondary []string
	if m.stats.Streak >= 2 {
		secondary = append(secondary, stDim(m.theme, fmt.Sprintf("Streak %d", m.stats.Streak)))
	}
	if avg := m.recentWorkdayAvg(); avg > 0 {
		diff := total - avg
		signColor := m.theme.Dim
		switch {
		case diff > 0:
			signColor = m.theme.Green
		case diff < 0:
			signColor = m.theme.Yellow
		}
		secondary = append(secondary, lipgloss.NewStyle().Foreground(signColor).Render(
			fmt.Sprintf("%s vs Schnitt", formatSignedDur(diff))))
	}
	if rep := wt.MonthBurndown(now); rep.Target > 0 {
		monthColor := m.theme.Dim
		// Trend glyph: ▲ on track, ▼ well behind. Subtle "·" between when
		// only mildly behind (-2h..0) so we don't over-alarm a single bad day.
		trend := "·"
		switch {
		case rep.OnTrack:
			monthColor = m.theme.Green
			trend = "▲"
		case rep.Saldo < -2*time.Hour:
			monthColor = m.theme.Yellow
			trend = "▼"
		}
		secondary = append(secondary, lipgloss.NewStyle().Foreground(monthColor).Render(
			fmt.Sprintf("Monat %s/%s %s %s",
				formatDur(rep.Total), formatDur(rep.Target), formatSignedDur(rep.Saldo), trend)))
	}
	// Pause stats: total gap-time today + longest single gap. Surfaces the
	// "how much actual break did I take" question that's otherwise hidden in
	// the per-session pause markers — useful for compliance/self-reflection.
	if pauseTotal, longestPause := pauseStats(m.day, now); pauseTotal > 0 {
		secondary = append(secondary, stDim(m.theme,
			fmt.Sprintf("Pause %s  (max %s)", formatDur(pauseTotal), formatDur(longestPause))))
	}
	// Pomodoro strip: only meaningful while a session is running. Shows
	// completed cycles + the in-progress one + remaining cycles to hit the
	// daily target. Adds an "Zeit für Pause" hint at cycle completion.
	if pom := m.renderPomodoroStrip(now); pom != "" {
		secondary = append(secondary, pom)
	}
	// Typical stop time: median of last 14 workdays' end-of-day, projected onto
	// today. Complements the linear ETA (below) — linear assumes no pauses,
	// typical assumes "you'll do what you usually do". When they disagree the
	// user has a useful signal.
	if t, ok := m.typicalStopTime(now); ok && total < target {
		col := m.theme.Dim
		if t.After(now) {
			col = m.theme.Cyan
		}
		secondary = append(secondary, lipgloss.NewStyle().Foreground(col).Render(
			"fertig typisch "+t.Format("15:04")))
	}

	barCells := inner - 4
	if barCells < 4 {
		barCells = 4
	}
	bar := "  " + statusbar.Bar(pct, barCells, m.theme)

	remaining := target - total
	if remaining < 0 {
		remaining = 0
	}
	summaryChips := []string{
		stDim(m.theme, "Ziel "+formatDur(target)),
		stDim(m.theme, "noch "+formatDur(remaining)),
	}
	if m.day.Active != nil {
		etaT := m.day.Active.Add(target - m.day.Logged)
		summaryChips = append(summaryChips, stDim(m.theme, "ETA "+etaT.Format("15:04")))
	}
	summary := joinWrapped(summaryChips, "  ·  ", "  ", "  ", inner)

	rows := []string{}
	if dayOffBanner != "" {
		rows = append(rows, dayOffBanner, "")
	}
	rows = append(rows, headline)
	if len(secondary) > 0 {
		rows = append(rows, joinWrapped(secondary, "  ·  ", "  ", "  ", inner))
	}
	if spark := m.renderTodaySparkline(); spark != "" {
		rows = append(rows, spark)
	}
	rows = append(rows, "", bar, summary)

	// Pause hint: tells the user "you stopped on purpose, press s to resume".
	// Without this, IsPaused looks identical to plain idle, but Resume picks
	// up after Pause whereas Start opens a fresh session.
	if m.day.IsPaused() && m.day.PausedAt != nil {
		pauseDur := now.Sub(*m.day.PausedAt)
		rows = append(rows, "",
			"  "+lipgloss.NewStyle().Foreground(m.theme.Yellow).Bold(true).Render("⏸ in Pause")+
				stDim(m.theme, fmt.Sprintf("  seit %s  ·  %s — `s` setzt fort",
					m.day.PausedAt.Format("15:04"), formatDur(pauseDur))))
	}

	// Empty state: no sessions, no active, no notes — invite to action.
	// Distinct treatment for "should be working" workday vs "free day".
	if len(m.day.Sessions) == 0 && m.day.Active == nil && len(m.notes) == 0 && !m.day.IsPaused() {
		// First-time onboarding: when history is also empty, the user has
		// never tracked anything in this install. Surface a brief welcome
		// instead of dropping them straight into the empty workday view.
		if len(m.history) == 0 {
			rows = append(rows, "")
			rows = append(rows, picker.SectionHeader("willkommen", inner, m.theme))
			welcomeChips := []string{
				lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true).Render("Worktime"),
				stDim(m.theme, "trackt deine Arbeitszeit lokal"),
				stDim(m.theme, "→ ~/.tmux/worktime.log"),
			}
			rows = append(rows, joinWrapped(welcomeChips, "  ·  ", "  ", "  ", inner))
			startChips := []string{
				lipgloss.NewStyle().Foreground(m.theme.Green).Render("s starten"),
				lipgloss.NewStyle().Foreground(m.theme.Cyan).Render("e manueller eintrag"),
				lipgloss.NewStyle().Foreground(m.theme.Cyan).Render("? Hilfe"),
			}
			rows = append(rows, joinWrapped(startChips, "  ·  ", "  ", "  ", inner))
			rows = append(rows, "")
		}
		rows = append(rows, "")
		rows = append(rows, picker.SectionHeader("heute", inner, m.theme))
		// On a workday past the user's typical start window → louder yellow
		// "Vergessen zu starten?" prompt. The cutoff adapts: 90 min after the
		// median start-of-day across the last 14 workdays. Falls back to a
		// fixed 10:00 when there isn't enough history yet.
		threshold := m.forgetfulnessThreshold(now)
		var hintChips []string
		switch {
		case wt.IsWorkday(now) && !now.Before(threshold):
			rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Yellow).Bold(true).
				Render("  Heute noch nichts erfasst."))
			hintChips = []string{
				stDim(m.theme, "Vergessen zu starten?"),
				stDim(m.theme, "e → manuell nachtragen"),
				stDim(m.theme, "s → jetzt starten"),
			}
		default:
			rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Fg).
				Render("  Noch nichts erfasst."))
			hintChips = []string{
				stDim(m.theme, "s starten"),
				stDim(m.theme, "e manueller eintrag"),
				stDim(m.theme, "f fokus-modus"),
				stDim(m.theme, "n attach"),
			}
		}
		rows = append(rows, joinWrapped(hintChips, "  ·  ", "  ", "  ", inner))
		navChips := []string{
			stDim(m.theme, "Tab / 1·2·3·4 wechseln"),
			stDim(m.theme, "? Hilfe"),
		}
		rows = append(rows, joinWrapped(navChips, "  ·  ", "  ", "  ", inner))
		rows = append(rows, "")
		rows = append(rows, picker.SectionHeader("notizen", inner, m.theme))
		rows = append(rows, stDim(m.theme, "  keine"))
		return rows
	}

	// Active session: render as the first row of the sessions list with a
	// distinct ▶ marker. Display-only — not a cursor target — because the
	// available actions ('s' stop, 'C' correct-start) aren't list-driven.
	totalSessRows := len(m.day.Sessions)
	if m.day.IsRunning() {
		totalSessRows++
	}
	if totalSessRows > 0 {
		rows = append(rows, "")
		rows = append(rows, picker.SectionHeader(
			fmt.Sprintf("sessions heute (%d)", totalSessRows), inner, m.theme))
		if m.day.IsRunning() && m.day.Active != nil {
			elapsed := now.Sub(*m.day.Active)
			runColor := m.theme.Green
			if wt.MaxStreakMinutes > 0 && int(elapsed.Minutes()) >= wt.MaxStreakMinutes {
				runColor = m.theme.Yellow
			}
			if wt.MaxStreakMinutes > 0 && int(elapsed.Minutes()) >= 2*wt.MaxStreakMinutes {
				runColor = m.theme.Red
			}
			// Bar reference for the running row alone is itself — we want
			// elapsed-vs-max-of-day, computed below in the loop. Use elapsed
			// directly so the running session always shows a sense of progress.
			activeMax := elapsed
			for _, s := range m.day.Sessions {
				if s.Elapsed > activeMax {
					activeMax = s.Elapsed
				}
			}
			marker := lipgloss.NewStyle().Foreground(runColor).Bold(true).Render("  ▶ ")
			label := lipgloss.NewStyle().Foreground(runColor).Bold(true).Render(
				fmt.Sprintf("%s → …   %s",
					m.day.Active.Format("15:04"), formatDur(elapsed)))
			// Skip mini-bar + "läuft" tail at very narrow widths so the row
			// fits within the box; the marker + time alone communicate state.
			row := marker + label
			if inner >= 50 {
				bar := sessionMiniBar(m.theme, elapsed, activeMax, 10)
				row += "  " + bar + "   " + stDim(m.theme, "läuft")
			}
			rows = append(rows, row)
		}
		// Pause-Marker zwischen aktiver Session und letzter abgeschlossener.
		if m.day.IsRunning() && m.day.Active != nil && len(m.day.Sessions) > 0 {
			lastStop := m.day.Sessions[len(m.day.Sessions)-1].Stop
			if pause := m.day.Active.Sub(lastStop); pause > 0 {
				rows = append(rows, stDim(m.theme,
					fmt.Sprintf("       ─ %s Pause ─", formatDur(pause))))
			}
		}
		// Bar reference: the longest session of the day. Lets each row's bar
		// fill ratio communicate "how big is this session relative to my
		// biggest one today" — the most useful comparison at a glance.
		maxSess := time.Duration(0)
		for _, s := range m.day.Sessions {
			if s.Elapsed > maxSess {
				maxSess = s.Elapsed
			}
		}
		if m.day.IsRunning() && m.day.Active != nil {
			if running := now.Sub(*m.day.Active); running > maxSess {
				maxSess = running
			}
		}
		var prevStop time.Time
		for i, s := range m.day.Sessions {
			if !prevStop.IsZero() {
				if pause := s.Start.Sub(prevStop); pause > 0 {
					rows = append(rows, stDim(m.theme,
						fmt.Sprintf("       ─ %s Pause ─", formatDur(pause))))
				}
			}
			prevStop = s.Stop
			bar := sessionMiniBar(m.theme, s.Elapsed, maxSess, 10)
			dur := lipgloss.NewStyle().Width(8).Render(formatDur(s.Elapsed))
			label := fmt.Sprintf("%s → %s   %s  %s",
				s.Start.Format("15:04"), s.Stop.Format("15:04"), dur, bar)
			hint := ""
			if s.Tag != "" {
				hint = stDim(m.theme, "["+s.Tag+"]")
			}
			rows = append(rows, m.renderTodayRow(
				m.onSession() && i == m.sessionIdx(),
				label, hint, inner, sectionSession))
			if s.Note != "" {
				rows = append(rows, stDim(m.theme, "       "+s.Note))
			}
		}
	}

	rows = append(rows, "")
	rows = append(rows, picker.SectionHeader("notizen", inner, m.theme))
	if len(m.notes) == 0 {
		rows = append(rows, joinWrapped(
			[]string{
				stDim(m.theme, "○  keine Tagesnotiz"),
				stDim(m.theme, "n → notiz anhängen"),
			}, "  ·  ", "  ", "  ", inner))
	} else {
		for i, n := range m.notes {
			marker := "◆"
			if n.isDaily {
				marker = "●"
			}
			label := marker + "  " + n.label
			hint := ""
			if n.isDaily {
				hint = stDim(m.theme, n.id)
			}
			rows = append(rows, m.renderTodayRow(
				m.onNote() && i == m.noteIdx(),
				label, hint, inner, sectionNote))
		}
	}
	return rows
}

// — today row helpers —

type todaySection int

const (
	sectionSession todaySection = 0
	sectionNote    todaySection = 1
)

// renderTodayRow renders a list row in today-view with a section-coloured
// cursor marker. Sessions get the accent colour, notes get cyan — so when the
// cursor crosses the section boundary, the marker visually changes too.
// Mirrors picker.Row but with a per-section accent.
func (m Model) renderTodayRow(focused bool, label, hint string, width int, sec todaySection) string {
	if !focused {
		return picker.Row(false, label, hint, width, m.theme)
	}
	color := m.theme.Accent
	if sec == sectionNote {
		color = m.theme.Cyan
	}
	marker := lipgloss.NewStyle().Foreground(color).Render("▌ ")
	body := lipgloss.NewStyle().Foreground(m.theme.Fg).Render(label)
	if hint != "" {
		body += "  " + hint
	}
	return marker + body
}

// — week view —

func (m Model) renderWeek() string {
	inner := m.width - 4
	var rows []string
	rows = append(rows, m.renderTabBar())

	if !m.weekLoaded {
		rows = append(rows, stDim(m.theme, "  lade…"))
	} else {
		rows = append(rows, m.renderWeekBody(inner)...)
	}

	rows = append(rows, "")
	body := strings.Join(rows, "\n")

	ref := m.weekRef
	if ref.IsZero() {
		ref = m.now
	}
	_, weekNum := ref.ISOWeek()
	monday := isoMonday(ref)
	sunday := monday.AddDate(0, 0, 6)
	suffix := ""
	switch {
	case isoMonday(m.now).Equal(monday):
		suffix = "diese Woche"
	case isoMonday(m.now.AddDate(0, 0, -7)).Equal(monday):
		suffix = "letzte Woche"
	case isoMonday(m.now.AddDate(0, 0, 7)).Equal(monday):
		suffix = "nächste Woche"
	}
	titleParts := []string{
		"Worktime",
		fmt.Sprintf("KW %d", weekNum),
		fmt.Sprintf("%02d. %s – %02d. %s",
			monday.Day(), germanMonth(monday.Month()),
			sunday.Day(), germanMonth(sunday.Month())),
	}
	if suffix != "" {
		titleParts = append(titleParts, suffix)
	}
	title := boxTitle(titleParts, m.width)
	box := titlebox.Render(title, body, m.width, m.theme)
	sLabel := "s starten"
	if m.day.IsRunning() {
		sLabel = "s stoppen"
	}
	return box + m.renderToastRow() + "\n" + wrapFooter(m.theme,
		sLabel+"  ·  enter drill  ·  h/l vorw./zurück  ·  e eintrag  ·  tab history  ·  ? hilfe  ·  b zurück  ·  q schließen",
		m.width)
}

func (m Model) renderWeekBody(inner int) []string {
	now := m.now
	barW := 12
	var rows []string
	var weekTotal time.Duration
	var weekTarget time.Duration
	var hits, weekdays int

	for i, day := range m.week {
		total := day.Total(now)
		weekTotal += total
		isWeekend := day.Date.Weekday() == time.Saturday || day.Date.Weekday() == time.Sunday
		if !isWeekend {
			weekTarget += day.Target
			weekdays++
			if total >= day.Target {
				hits++
			}
		}

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
		focused := i == m.weekCur
		marker := "  "
		if focused {
			marker = lipgloss.NewStyle().Foreground(m.theme.Accent).Render("▌ ")
		}
		isOff, dayOff := dayOffOf(day.Date)
		switch {
		case isWeekend && total == 0:
			tag := stDim(m.theme, "Wochenende")
			line = marker + nameStr + " " + dateStr + "  " + tag
		case isOff && total == 0:
			label := lipgloss.NewStyle().Foreground(kindColor(m.theme, dayOff.Kind)).
				Render(dayOff.Kind.LabelDe())
			suffix := ""
			if dayOff.Label != "" {
				suffix = "  " + stDim(m.theme, dayOff.Label)
			}
			line = marker + nameStr + " " + dateStr + "  " + label + suffix
		case total == 0:
			emptyBar := lipgloss.NewStyle().Foreground(m.theme.Border).Render(strings.Repeat("─", barW))
			todayMark := ""
			if day.IsToday {
				todayMark = "  " + stDim(m.theme, "heute")
			}
			line = marker + nameStr + " " + dateStr + "  " + emptyBar + todayMark
		default:
			bar := statusbar.Bar(pct, barW, m.theme)
			// When today is running, indicate the live fragment after the bar.
			if day.IsToday && day.Active != nil && day.Target > 0 {
				doneCells := int(time.Duration(barW) * day.Logged / day.Target)
				if doneCells < 0 {
					doneCells = 0
				}
				if doneCells > barW {
					doneCells = barW
				}
				_ = doneCells // bar already handles % — keep marker simple
			}
			pctStr := stDim(m.theme, fmt.Sprintf("%3d%%", pct))
			durStr := lipgloss.NewStyle().Foreground(m.theme.Fg).Bold(total >= day.Target).Render(formatDur(total))
			extra := ""
			if day.IsToday && day.Active != nil {
				extra += "  " + lipgloss.NewStyle().Foreground(m.theme.Green).Render("▶")
			}
			if total >= day.Target {
				extra += "  " + lipgloss.NewStyle().Foreground(m.theme.Green).Render("✓")
			}
			line = marker + nameStr + " " + dateStr + "  " + bar + "  " + pctStr + "  " + durStr + extra
		}
		rows = append(rows, line)
	}

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
	rows = append(rows, "  "+m.renderWeekPace())

	// KPI strip: average per worked weekday, projected end-of-week, balance.
	rows = append(rows, "")
	rows = append(rows, picker.SectionHeader("kennzahlen", inner, m.theme))
	avg := time.Duration(0)
	if weekdays > 0 {
		avg = weekTotal / time.Duration(weekdays)
	}
	balance := weekTotal - weekTarget
	balanceStr := formatSignedDur(balance)
	balColor := m.theme.Dim
	switch {
	case balance > 0:
		balColor = m.theme.Green
	case balance < 0:
		balColor = m.theme.Yellow
	}
	bal := lipgloss.NewStyle().Foreground(balColor).Render(balanceStr)

	// Vorwochen-Vergleich: Saldo gegenüber der Vorwoche desselben Anker-Tags.
	// Liefert "vs KW17 +45m" — sofortige Verdacht-Antwort auf "ist diese
	// Woche besser oder schlechter".
	prev := m.weekRefOrNow().AddDate(0, 0, -7)
	prevStats := wt.WeekStats(prev)
	_, prevWN := isoMonday(prev).ISOWeek()
	kpis := []string{
		fmt.Sprintf("Schnitt %s", formatDur(avg)),
		fmt.Sprintf("Ziele %d/%d", hits, weekdays),
		"Saldo " + bal,
	}
	if prevStats.Workdays > 0 {
		diff := weekTotal - prevStats.Total
		col := m.theme.Dim
		switch {
		case diff > 0:
			col = m.theme.Green
		case diff < 0:
			col = m.theme.Yellow
		}
		kpis = append(kpis, lipgloss.NewStyle().Foreground(col).Render(
			fmt.Sprintf("%s vs KW%d", formatSignedDur(diff), prevWN)))
	}
	rows = append(rows, joinWrapped(kpis, "  ·  ", "  ", "  ", inner))

	return rows
}

// renderWeekPace builds the pace strip "●●●○○  3/5 Ziele · ▲ on track".
// "Expected by now" = past weekdays plus today if it already met its target,
// minus configured day-offs (Feiertag/Urlaub/Krank). Day-offs render as a
// distinct cyan glyph so they're not mistaken for a missed day.
func (m Model) renderWeekPace() string {
	greenStyle := lipgloss.NewStyle().Foreground(m.theme.Green)
	dimStyle := lipgloss.NewStyle().Foreground(m.theme.Dim)
	yellowStyle := lipgloss.NewStyle().Foreground(m.theme.Yellow)
	cyanStyle := lipgloss.NewStyle().Foreground(m.theme.Cyan)

	var dots []string
	hits := 0
	expected := 0
	workdays := 0
	for _, d := range m.week {
		isWeekend := d.Date.Weekday() == time.Saturday || d.Date.Weekday() == time.Sunday
		isOff, dayOff := dayOffOf(d.Date)
		total := d.Total(m.now)
		hit := total >= d.Target

		switch {
		case isOff && !isWeekend:
			// Distinct glyph for free days — kind picks the color.
			glyph := dayOffPaceGlyph(dayOff.Kind)
			dots = append(dots, cyanStyle.Foreground(kindColor(m.theme, dayOff.Kind)).Render(glyph))
		case hit:
			dots = append(dots, greenStyle.Render("●"))
		case d.IsToday && d.Active != nil:
			dots = append(dots, yellowStyle.Render("●"))
		default:
			dots = append(dots, dimStyle.Render("○"))
		}

		if !isWeekend && !isOff {
			workdays++
			past := d.Date.Before(time.Date(m.now.Year(), m.now.Month(), m.now.Day(), 0, 0, 0, 0, m.now.Location()))
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

// dayOffOf is a small wrapper that hides the wt import behind a simpler API
// for the renderers in this file.
func dayOffOf(t time.Time) (bool, wt.DayOff) {
	d, ok := wt.LookupDayOff(t)
	return ok, d
}

// dayOffPaceGlyph picks a per-kind monospace glyph for the pace strip.
func dayOffPaceGlyph(k wt.Kind) string {
	switch k {
	case wt.KindHoliday:
		return "★"
	case wt.KindVacation:
		return "☼"
	case wt.KindSick:
		return "✚"
	}
	return "○"
}

// — history view —

func (m Model) renderHistory() string {
	tabBar := m.renderTabBar()
	var content string
	// IMPORTANT: pass each line through lipgloss.Render *separately*. Putting
	// a "\n" inside the rendered string causes lipgloss to pad the shorter
	// line up to the width of the longest line, which inflates the tab-bar
	// row inside the titlebox and overflows narrow tmux panes.
	if !m.historyLoaded {
		content = tabBar + "\n" + stDim(m.theme, "  lade…")
	} else if len(m.history) == 0 {
		content = tabBar + "\n" + stDim(m.theme, "  Noch keine Daten.")
	} else {
		content = tabBar + "\n" + m.renderHistoryHeader() + "\n" + m.histVp.View()
	}
	titleParts := []string{"Worktime", "History"}
	if m.histQuery != "" {
		titleParts = append(titleParts, "filter: "+m.histQuery)
	}
	title := boxTitle(titleParts, m.width)
	box := titlebox.Render(title, content, m.width, m.theme)
	var mode string
	switch m.histMode {
	case historyHeatmap:
		mode = "heatmap"
	case historyTagClock:
		mode = "tagclock"
	case historyMonth:
		mode = "month"
	default:
		mode = "list"
	}
	return box + m.renderToastRow() + "\n" + wrapFooter(m.theme,
		"j/k auswahl  ·  enter drill  ·  v ansicht ("+mode+")  ·  / filter  ·  [/] paginate  ·  T alle  ·  y/Y kopieren  ·  g/G top/bot  ·  tab heute  ·  ? hilfe  ·  b zurück  ·  q schließen",
		m.width)
}

// renderHistoryHeader is the small stats strip above the scrollable body.
func (m Model) renderHistoryHeader() string {
	records := m.filteredHistory()
	st := wt.Aggregate(records)
	if st.Days == 0 {
		return stDim(m.theme, "  Keine Treffer.")
	}
	balColor := m.theme.Dim
	switch {
	case st.Overtime > 0:
		balColor = m.theme.Green
	case st.Overtime < 0:
		balColor = m.theme.Yellow
	}
	bal := lipgloss.NewStyle().Foreground(balColor).Render(formatSignedDur(st.Overtime))
	// Two grouped lines instead of one nine-value chain — readable on terminals
	// under ~110 cols. First row = the volume snapshot; second = performance.
	// Each row wraps to subsequent lines when the terminal is narrow.
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
	inner := m.width - 4
	if inner <= 0 {
		inner = 80
	}
	header := joinWrapped(volume, "  ·  ",
		"  "+stDim(m.theme, "volumen:      "),
		"                ", inner) +
		"\n" + joinWrapped(performance, "  ·  ",
		"  "+stDim(m.theme, "performance:  "),
		"                ", inner)

	// Tag strip on a second line, with proportional bars so a tag's relative
	// weight in the range is readable at a glance instead of having to compare
	// numbers. Untagged time hides at the end if it's >5% of total.
	if tags := st.TopTags(6); len(tags) > 0 {
		header += "\n" + m.renderTagBars(tags, inner)
	}
	if st.Untagged > 0 && st.Total > 0 {
		untaggedPct := int(st.Untagged * 100 / st.Total)
		if untaggedPct >= 5 {
			tagTarget := wt.TagTarget("untagged") // typically zero — TagTargets covers the named ones
			_ = tagTarget
			header += "  " + stDim(m.theme, fmt.Sprintf("· untagged %s (%d%%)",
				formatDur(st.Untagged), untaggedPct))
		}
	}
	// Days off summary (only the kinds that actually appear).
	if len(st.DaysOff) > 0 {
		byKind := map[wt.Kind]int{}
		for _, d := range st.DaysOff {
			byKind[d.Kind]++
		}
		var freeParts []string
		for _, k := range wt.AllKinds {
			if c := byKind[k]; c > 0 {
				freeParts = append(freeParts, fmt.Sprintf("%s %d", k.LabelDe(), c))
			}
		}
		header += "\n" + joinWrapped(freeParts, "  ·  ",
			"  "+stDim(m.theme, "frei: "), "        ", inner)
	}
	// Tag-targets row: when the user configured tag_target_NAME entries,
	// surface progress for them. Only visible when a tag- or note-filter is
	// not active (otherwise the bar redundancy is loud).
	q := strings.ToLower(strings.TrimSpace(m.histQuery))
	if !strings.HasPrefix(q, "tag:") && !strings.HasPrefix(q, "note:") {
		if line := m.renderTagTargetsLine(st, inner); line != "" {
			header += "\n" + line
		}
	}
	return header
}

// renderTagBars renders a horizontal bar strip for the given tag stats,
// scaled to the largest tag in the slice. Each chip is
// "name  ▎▎▎▎▎▎▎▎    1h 30m  ·  4× ⌀ 22m" — bar + total + count + avg per
// session. The avg helps the user spot "lots of short pings" vs. "few deep
// blocks" patterns even when totals match.
func (m Model) renderTagBars(tags []wt.TagDur, inner int) string {
	if len(tags) == 0 {
		return ""
	}
	maxDur := tags[0].Total
	if maxDur <= 0 {
		return ""
	}
	const cells = 10
	parts := make([]string, 0, len(tags))
	for _, t := range tags {
		filled := int(float64(t.Total) / float64(maxDur) * cells)
		if filled < 1 && t.Total > 0 {
			filled = 1
		}
		bar := strings.Repeat("▎", filled) + strings.Repeat(" ", cells-filled)
		barStyled := lipgloss.NewStyle().Foreground(m.theme.Accent).Render(bar)
		eff := ""
		if t.Count > 0 {
			avg := t.Total / time.Duration(t.Count)
			eff = stDim(m.theme, fmt.Sprintf("  %d× ⌀ %s", t.Count, formatDur(avg)))
		}
		parts = append(parts, fmt.Sprintf("%-10s %s %s%s", t.Tag, barStyled, formatDur(t.Total), eff))
	}
	return joinWrapped(parts, "  ·  ",
		"  "+stDim(m.theme, "tags:")+"  ", "         ", inner)
}

// renderTagTargetsLine renders progress against configured tag_target_*
// entries from worktime.conf. Empty string when no targets are set.
//
// Targets are *daily* — for the History header, we scale by the number of
// workdays in the range so "deep 4h × 5 workdays = 20h" is the implied bar.
// Wraps onto multiple lines at `inner` width.
func (m Model) renderTagTargetsLine(st wt.Stats, inner int) string {
	targets := wt.TagTargets()
	if len(targets) == 0 {
		return ""
	}
	parts := make([]string, 0, len(targets))
	for tag, dailyTarget := range targets {
		got := st.ByTag[tag]
		// Lookup is case-insensitive — try matching anywhere in ByTag.
		if got == 0 {
			for k, v := range st.ByTag {
				if strings.EqualFold(k, tag) {
					got = v
					break
				}
			}
		}
		want := dailyTarget * time.Duration(st.Workdays)
		if want <= 0 {
			continue
		}
		col := m.theme.Yellow
		if got >= want {
			col = m.theme.Green
		}
		val := lipgloss.NewStyle().Foreground(col).Render(
			fmt.Sprintf("%s/%s", formatDur(got), formatDur(want)))
		parts = append(parts, fmt.Sprintf("%s %s", tag, val))
	}
	if len(parts) == 0 {
		return ""
	}
	sort.Strings(parts) // stable display order
	return joinWrapped(parts, "  ·  ",
		"  "+stDim(m.theme, "tag-ziele: "), "             ", inner)
}

// filteredHistory applies m.histQuery against m.history and returns the
// matching records (newest first preserved). Filter syntax:
//
//   - ""               → all records
//   - "KWnn"           → ISO week number (current year)
//   - "YYYY"           → year
//   - "YYYY-MM"        → month
//   - "FROM..TO"       → date range
//   - "tag:foo"        → records with at least one session tagged "foo".
//     Each kept record is reduced to only those sessions
//     and its Total recomputed, so the bar reflects the
//     tag's slice of the day.
//   - "note:foo"       → records with at least one session whose note contains
//     "foo" (case-insensitive substring). Like tag-filter,
//     the kept record is reduced to matching sessions only.
func (m Model) filteredHistory() []wt.DayRecord {
	q := strings.TrimSpace(m.histQuery)
	if q == "" {
		return m.history
	}
	if out, ok := m.filterByTag(q); ok {
		return out
	}
	if out, ok := m.filterByNote(q); ok {
		return out
	}
	if out, ok := m.filterByISOWeek(q); ok {
		return out
	}
	if out, ok := m.filterByRange(q); ok {
		return out
	}
	return m.history
}

// filterByNote handles "note:SUBSTR" — keeps only sessions whose note contains
// the substring (case-insensitive) and recomputes per-day totals so the bar
// reflects only matching sessions.
func (m Model) filterByNote(q string) ([]wt.DayRecord, bool) {
	if !strings.HasPrefix(strings.ToLower(q), "note:") {
		return nil, false
	}
	want := strings.ToLower(strings.TrimSpace(q[len("note:"):]))
	if want == "" {
		return m.history, true
	}
	out := make([]wt.DayRecord, 0, len(m.history))
	for _, rec := range m.history {
		var keep []wt.Session
		var total time.Duration
		for _, s := range rec.Sessions {
			if strings.Contains(strings.ToLower(s.Note), want) {
				keep = append(keep, s)
				total += s.Elapsed
			}
		}
		if len(keep) > 0 {
			out = append(out, wt.DayRecord{
				Date: rec.Date, Sessions: keep, Total: total, Target: rec.Target,
			})
		}
	}
	return out, true
}

// filterByTag handles "tag:NAME" — keeps only sessions with that tag and
// recomputes per-day totals so the bar reflects the tag's slice of the day.
func (m Model) filterByTag(q string) ([]wt.DayRecord, bool) {
	if !strings.HasPrefix(strings.ToLower(q), "tag:") {
		return nil, false
	}
	want := strings.TrimSpace(q[len("tag:"):])
	if want == "" {
		return m.history, true
	}
	out := make([]wt.DayRecord, 0, len(m.history))
	for _, rec := range m.history {
		var keep []wt.Session
		var total time.Duration
		for _, s := range rec.Sessions {
			if strings.EqualFold(s.Tag, want) {
				keep = append(keep, s)
				total += s.Elapsed
			}
		}
		if len(keep) > 0 {
			out = append(out, wt.DayRecord{
				Date: rec.Date, Sessions: keep, Total: total, Target: rec.Target,
			})
		}
	}
	return out, true
}

// filterByISOWeek handles "KWnn" — ISO week number (current year only).
func (m Model) filterByISOWeek(q string) ([]wt.DayRecord, bool) {
	if !strings.HasPrefix(strings.ToUpper(q), "KW") {
		return nil, false
	}
	var w int
	if _, err := fmt.Sscanf(strings.ToUpper(q), "KW%d", &w); err != nil || w <= 0 {
		return nil, false
	}
	out := make([]wt.DayRecord, 0, len(m.history))
	for _, r := range m.history {
		_, rw := r.Date.ISOWeek()
		if rw == w && r.Date.Year() == m.now.Year() {
			out = append(out, r)
		}
	}
	return out, true
}

// filterByRange handles YYYY / YYYY-MM / FROM..TO via wt.ParseRange.
func (m Model) filterByRange(q string) ([]wt.DayRecord, bool) {
	r, err := wt.ParseRange(m.now, q)
	if err != nil || (r.From.IsZero() && r.To.IsZero()) {
		return nil, false
	}
	out := make([]wt.DayRecord, 0, len(m.history))
	for _, rec := range m.history {
		if r.ContainsDate(rec.Date) {
			out = append(out, rec)
		}
	}
	return out, true
}

func (m Model) renderHistoryContent() string {
	records := m.filteredHistory()
	if len(records) == 0 {
		return stDim(m.theme, "  Keine Treffer.")
	}
	switch m.histMode {
	case historyHeatmap:
		return m.renderHistoryHeatmap(records)
	case historyTagClock:
		return m.renderHistoryTagClock(records)
	case historyMonth:
		return m.renderHistoryMonth(records)
	}
	return m.renderHistoryList(records)
}

func (m Model) renderHistoryList(records []wt.DayRecord) string {
	barW := 12
	var lines []string
	prevWeek := -1
	prevYear := -1
	for i, rec := range records {
		y, w := rec.Date.ISOWeek()
		if w != prevWeek || y != prevYear {
			if prevWeek != -1 {
				lines = append(lines, "")
			}
			lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true).
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
		marker := "  "
		if i == m.histCur {
			marker = lipgloss.NewStyle().Foreground(m.theme.Accent).Render("▌ ")
		}
		lines = append(lines, marker+nameStr+" "+dateStr+"  "+bar+"  "+pctStr+"  "+durStr+done)
	}
	return strings.Join(lines, "\n")
}

// renderHistoryHeatmap draws a Mon–Sun × N-week grid of block glyphs whose
// saturation reflects the day's percent-of-target. Newest week on top.
func (m Model) renderHistoryHeatmap(records []wt.DayRecord) string {
	if len(records) == 0 {
		return ""
	}
	byKey := make(map[string]wt.DayRecord, len(records))
	for _, r := range records {
		byKey[r.Date.Format("2006-01-02")] = r
	}
	// Bounds come from heatmapBounds() so [/]-panning, cursor positioning
	// and rendering all agree on which cells are visible.
	startMon, weeks := m.heatmapBounds()
	if weeks == 0 {
		return ""
	}

	var lines []string
	// Header row: KW numbers, with a faint year-change hint when the column
	// crosses Jan 1. The hint is a colour shift on the new-year column rather
	// than a separator (which would offset the grid below).
	header := "       "
	prevYear := -1
	for w := 0; w < weeks; w++ {
		mon := startMon.AddDate(0, 0, 7*w)
		yr, wn := mon.ISOWeek()
		col := m.theme.Dim
		if prevYear != -1 && yr != prevYear {
			// First week of a new ISO year — render in cyan to mark the
			// boundary visually. Don't underline (would shift baseline).
			col = m.theme.Cyan
		}
		header += lipgloss.NewStyle().Foreground(col).Render(fmt.Sprintf("%2d ", wn%100))
		prevYear = yr
	}
	lines = append(lines, header)

	dayLabels := []string{"Mo", "Di", "Mi", "Do", "Fr", "Sa", "So"}
	for d := 0; d < 7; d++ {
		row := "   " + lipgloss.NewStyle().Foreground(m.theme.Fg).Width(3).Render(dayLabels[d]) + "  "
		for w := 0; w < weeks; w++ {
			day := startMon.AddDate(0, 0, 7*w+d)
			rec, ok := byKey[day.Format("2006-01-02")]
			// Empty-cell glyph: '.' for missing weekday data, '·' (mid-dot)
			// for an empty weekend cell. The two glyphs distinguish "expected
			// to be empty" from "data gap" at a glance.
			cell := " . "
			color := m.theme.Border
			if isWeekendDate(day) {
				cell = " · "
			}
			if ok && rec.Target > 0 {
				pct := float64(rec.Total) / float64(rec.Target)
				switch {
				case pct >= 1.5:
					cell = " █ "
					color = m.theme.Red
				case pct >= 1.0:
					cell = " █ "
					color = m.theme.Green
				case pct >= 0.75:
					cell = " ▓ "
					color = m.theme.Green
				case pct >= 0.5:
					cell = " ▒ "
					color = m.theme.Yellow
				case pct > 0:
					cell = " ░ "
					color = m.theme.Yellow
				}
			}
			// Weekend default already set to ' · ' above; nothing to override
			// here for the empty-data path.
			// Day-off marker takes priority over empty cells but not over
			// data cells — those keep their pct-derived glyph and recolour
			// to cyan for visibility.
			if dayOff, isOff := wt.LookupDayOff(day); isOff {
				if !ok || rec.Target == 0 {
					cell = " · "
					switch dayOff.Kind {
					case wt.KindHoliday:
						cell = " ★ "
					case wt.KindVacation:
						cell = " ☼ "
					case wt.KindSick:
						cell = " ✚ "
					}
				}
				color = m.theme.Cyan
			}
			isCursor := w == m.heatCol && d == m.heatRow
			isToday := sameDay(day, m.now)
			cellStyle := lipgloss.NewStyle().Foreground(color)
			switch {
			case isCursor:
				// 2D grid cursor: full Accent block (bg-colored glyph against
				// Accent background) gives a uniform highlight that's readable
				// regardless of the underlying data colour. Mirrored in tagclock.
				cellStyle = lipgloss.NewStyle().Foreground(m.theme.Bg).Background(m.theme.Accent).Bold(true)
			case isToday:
				// Permanent "today" marker — keeps the user oriented after
				// they navigate the cursor away. Subtler than cursor.
				cellStyle = cellStyle.Underline(true).Bold(true)
			}
			row += cellStyle.Render(cell)
		}
		lines = append(lines, row)
	}
	lines = append(lines, "")

	// Cursor status line: date + total/target for the focused cell.
	if d, ok := m.heatmapDateAt(m.heatCol, m.heatRow); ok {
		var status string
		if rec, hit := byKey[d.Format("2006-01-02")]; hit {
			status = fmt.Sprintf("   %s  %s  %s / %s",
				germanWeekdayShort(d.Weekday()),
				d.Format("2006-01-02"),
				formatDur(rec.Total),
				formatDur(rec.Target),
			)
		} else {
			status = fmt.Sprintf("   %s  %s  —", germanWeekdayShort(d.Weekday()), d.Format("2006-01-02"))
		}
		if dayOff, doh := wt.LookupDayOff(d); doh {
			status += "  ·  " + dayOff.Kind.LabelDe() + " " + dayOff.Label
		}
		lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.Accent).Render(status))
	}

	legendChips := []string{
		stDim(m.theme, ". leer"),
		stDim(m.theme, "░ <50%"),
		stDim(m.theme, "▒ <75%"),
		stDim(m.theme, "▓ <100%"),
		lipgloss.NewStyle().Foreground(m.theme.Green).Render("█ Ziel"),
		lipgloss.NewStyle().Foreground(m.theme.Red).Render("█ ≥150%"),
		lipgloss.NewStyle().Foreground(m.theme.Cyan).Render("★/☼/✚ frei"),
	}
	navChips := []string{
		stDim(m.theme, "h/j/k/l navigieren"),
		stDim(m.theme, "enter drilldown"),
		stDim(m.theme, "y yank"),
		stDim(m.theme, "[/] ±13 Wochen"),
		stDim(m.theme, "T jetzt"),
	}
	innerW := m.width - 4
	if innerW <= 0 {
		innerW = 80
	}
	lines = append(lines, joinWrapped(legendChips, "  ", "   ", "   ", innerW))
	lines = append(lines, joinWrapped(navChips, "  ·  ", "   ", "   ", innerW))
	return strings.Join(lines, "\n")
}

func isWeekendDate(t time.Time) bool {
	wd := t.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}

// renderHistoryTagClock renders a 7×24h grid (rows = Mo..So, cols = 0..23)
// where each cell's intensity reflects how often work happened in that
// (weekday, hour) slot across the filtered records. Answers "wann arbeite
// ich?" — combine with a tag:NAME filter to see "wann mache ich deep work?".
func (m Model) renderHistoryTagClock(records []wt.DayRecord) string {
	if len(records) == 0 {
		return stDim(m.theme, "  Keine Treffer.")
	}
	var grid [7][24]time.Duration
	for _, rec := range records {
		for _, s := range rec.Sessions {
			t := s.Start
			for t.Before(s.Stop) {
				wd := int(t.Weekday()) - 1
				if wd < 0 {
					wd = 6 // Sunday
				}
				hour := t.Hour()
				next := time.Date(t.Year(), t.Month(), t.Day(),
					hour+1, 0, 0, 0, t.Location())
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
	if maxCell == 0 {
		return stDim(m.theme, "  Keine Treffer.")
	}

	dayLabels := []string{"Mo", "Di", "Mi", "Do", "Fr", "Sa", "So"}
	var lines []string
	// Header: hour numbers 0..23, two chars wide, dim.
	hdr := "      "
	for h := 0; h < 24; h++ {
		col := m.theme.Dim
		// Slight emphasis on the on-the-hour markers that bracket typical
		// workday windows.
		if h == 9 || h == 12 || h == 17 {
			col = m.theme.Border
		}
		hdr += lipgloss.NewStyle().Foreground(col).Render(fmt.Sprintf("%02d", h))
	}
	lines = append(lines, hdr)

	for r := 0; r < 7; r++ {
		row := "  " + lipgloss.NewStyle().Foreground(m.theme.Fg).Width(3).Render(dayLabels[r]) + " "
		for c := 0; c < 24; c++ {
			frac := float64(grid[r][c]) / float64(maxCell)
			cell := "  "
			color := m.theme.Border
			switch {
			case grid[r][c] == 0:
				cell = "··"
				color = m.theme.Border
			case frac >= 0.75:
				cell = "██"
				color = m.theme.Green
			case frac >= 0.5:
				cell = "▓▓"
				color = m.theme.Green
			case frac >= 0.25:
				cell = "▒▒"
				color = m.theme.Yellow
			case frac > 0:
				cell = "░░"
				color = m.theme.Yellow
			}
			isCursor := r == m.tagClockRow && c == m.tagClockCol
			cellStyle := lipgloss.NewStyle().Foreground(color)
			if isCursor {
				// Same Accent-block cursor as in the heatmap — readable across
				// every data colour because foreground is forced to theme bg.
				cellStyle = lipgloss.NewStyle().Foreground(m.theme.Bg).Background(m.theme.Accent).Bold(true)
			}
			row += cellStyle.Render(cell)
		}
		lines = append(lines, row)
	}
	lines = append(lines, "")

	// Cursor status line: weekday, hour-range, total time spent in that slot.
	row := m.tagClockRow
	col := m.tagClockCol
	if row >= 0 && row < 7 && col >= 0 && col < 24 {
		dur := grid[row][col]
		var status string
		switch {
		case dur == 0:
			status = fmt.Sprintf("   %s  %02d:00–%02d:00  —",
				dayLabels[row], col, (col+1)%24)
		default:
			pct := int(float64(dur) / float64(maxCell) * 100)
			status = fmt.Sprintf("   %s  %02d:00–%02d:00  %s  (%d%% des Maximums)",
				dayLabels[row], col, (col+1)%24, formatDur(dur), pct)
		}
		lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.Accent).Render(status))
	}

	innerW := m.width - 4
	if innerW <= 0 {
		innerW = 80
	}
	legendChips := []string{
		stDim(m.theme, "·· keine"),
		stDim(m.theme, "░░ <25%"),
		stDim(m.theme, "▒▒ <50%"),
		stDim(m.theme, "▓▓ <75%"),
		stDim(m.theme, "██ ≥75%"),
	}
	navChips := []string{
		stDim(m.theme, "h/j/k/l navigieren"),
		stDim(m.theme, "T jetzt"),
		stDim(m.theme, "v ansicht"),
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(m.histQuery)), "tag:") {
		legendChips = append([]string{
			stDim(m.theme, "filter:") + " " +
				lipgloss.NewStyle().Foreground(m.theme.Cyan).Render(m.histQuery),
		}, legendChips...)
	}
	lines = append(lines, joinWrapped(legendChips, "  ", "   ", "   ", innerW))
	lines = append(lines, joinWrapped(navChips, "  ·  ", "   ", "   ", innerW))
	return strings.Join(lines, "\n")
}

// renderHistoryMonth draws a calendar grid for the month at `m.monthRef`,
// with a status glyph per day:
//
//	··  no data (workday) / weekend
//	░▒▓█  scaled to %-of-target (matches heatmap)
//	★/☼/✚  holiday / vacation / sick
//
// The cursor cell is highlighted with the same Accent block used by the
// heatmap and tagclock so the visual language stays consistent.
func (m Model) renderHistoryMonth(records []wt.DayRecord) string {
	if m.monthRef.IsZero() {
		m.monthRef = time.Date(m.now.Year(), m.now.Month(), 1, 0, 0, 0, 0, m.now.Location())
		m.monthCur = m.now.Day()
	}
	first := time.Date(m.monthRef.Year(), m.monthRef.Month(), 1, 0, 0, 0, 0, m.monthRef.Location())
	last := first.AddDate(0, 1, -1)

	byKey := make(map[string]wt.DayRecord, len(records))
	for _, r := range records {
		byKey[r.Date.Format("2006-01-02")] = r
	}

	var lines []string
	header := lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true).
		Render(fmt.Sprintf("  %s %d", germanMonth(first.Month()), first.Year()))
	lines = append(lines, header)
	lines = append(lines, "")

	dayLabels := []string{"Mo", "Di", "Mi", "Do", "Fr", "Sa", "So"}
	hdr := "    "
	for _, lbl := range dayLabels {
		hdr += lipgloss.NewStyle().Foreground(m.theme.Dim).Render(fmt.Sprintf(" %-3s ", lbl))
	}
	lines = append(lines, hdr)

	// Compute the Monday on/before the 1st so the grid starts correctly.
	wd := int(first.Weekday())
	if wd == 0 {
		wd = 7
	}
	gridStart := first.AddDate(0, 0, -(wd - 1))

	for week := 0; week < 6; week++ {
		row := "    "
		anyInMonth := false
		for d := 0; d < 7; d++ {
			day := gridStart.AddDate(0, 0, week*7+d)
			inMonth := day.Month() == first.Month() && day.Year() == first.Year()
			if inMonth {
				anyInMonth = true
			}
			cell := m.renderMonthCell(day, inMonth, byKey)
			row += cell
		}
		if !anyInMonth && week > 0 {
			break
		}
		lines = append(lines, row)
	}
	lines = append(lines, "")

	// Cursor status line.
	cursorDate := time.Date(first.Year(), first.Month(), m.monthCur, 0, 0, 0, 0, first.Location())
	if m.monthCur < 1 || m.monthCur > last.Day() {
		cursorDate = first
	}
	rec, hit := byKey[cursorDate.Format("2006-01-02")]
	var status string
	switch {
	case hit:
		pct := 0
		if rec.Target > 0 {
			pct = int(rec.Total * 100 / rec.Target)
		}
		status = fmt.Sprintf("   %s  %s  %s / %s  ·  %d%%",
			germanWeekdayShort(cursorDate.Weekday()),
			cursorDate.Format("2006-01-02"),
			formatDur(rec.Total), formatDur(rec.Target), pct)
	default:
		status = fmt.Sprintf("   %s  %s  —",
			germanWeekdayShort(cursorDate.Weekday()),
			cursorDate.Format("2006-01-02"))
	}
	if dayOff, doh := wt.LookupDayOff(cursorDate); doh {
		status += "  ·  " + dayOff.Kind.LabelDe() + " " + dayOff.Label
	}
	lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.Accent).Render(status))

	// Month aggregate strip.
	monthStats := wt.MonthStats(first)
	if monthStats.Days > 0 {
		balColor := m.theme.Dim
		switch {
		case monthStats.Overtime > 0:
			balColor = m.theme.Green
		case monthStats.Overtime < 0:
			balColor = m.theme.Yellow
		}
		bal := lipgloss.NewStyle().Foreground(balColor).Render(formatSignedDur(monthStats.Overtime))
		lines = append(lines, "")
		lines = append(lines, "   "+stDim(m.theme, fmt.Sprintf("Monat %s  ·  Ziele %d/%d  ·  Saldo ",
			formatDur(monthStats.Total), monthStats.Hits, monthStats.Workdays))+bal)
	}

	innerW := m.width - 4
	if innerW <= 0 {
		innerW = 80
	}
	navChips := []string{
		stDim(m.theme, "h/j/k/l navigieren"),
		stDim(m.theme, "enter drilldown"),
		stDim(m.theme, "[/] Monat ±"),
		stDim(m.theme, "T jetzt"),
		stDim(m.theme, "v ansicht"),
	}
	lines = append(lines, joinWrapped(navChips, "  ·  ", "   ", "   ", innerW))
	return strings.Join(lines, "\n")
}

// renderMonthCell renders one day cell of the month grid as a fixed-width
// 5-char block: " DD G " where DD is the day number and G is a status glyph.
// Out-of-month days render as blank padding so the column alignment holds.
func (m Model) renderMonthCell(day time.Time, inMonth bool, byKey map[string]wt.DayRecord) string {
	if !inMonth {
		return "     "
	}
	rec, hasRec := byKey[day.Format("2006-01-02")]
	dayOff, isOff := wt.LookupDayOff(day)
	isCursor := day.Day() == m.monthCur
	isToday := sameDay(day, m.now)
	isWeekend := day.Weekday() == time.Saturday || day.Weekday() == time.Sunday

	glyph := "·"
	color := m.theme.Border
	switch {
	case hasRec && rec.Target > 0:
		pct := float64(rec.Total) / float64(rec.Target)
		switch {
		case pct >= 1.5:
			glyph = "█"
			color = m.theme.Red
		case pct >= 1.0:
			glyph = "█"
			color = m.theme.Green
		case pct >= 0.75:
			glyph = "▓"
			color = m.theme.Green
		case pct >= 0.5:
			glyph = "▒"
			color = m.theme.Yellow
		case pct > 0:
			glyph = "░"
			color = m.theme.Yellow
		}
	case isOff:
		switch dayOff.Kind {
		case wt.KindHoliday:
			glyph = "★"
		case wt.KindVacation:
			glyph = "☼"
		case wt.KindSick:
			glyph = "✚"
		}
		color = m.theme.Cyan
	case isWeekend:
		glyph = " "
		color = m.theme.Dim
	}

	dayNum := fmt.Sprintf("%2d", day.Day())
	body := fmt.Sprintf(" %s %s", dayNum, glyph)

	st := lipgloss.NewStyle().Foreground(color)
	switch {
	case isCursor:
		st = lipgloss.NewStyle().Foreground(m.theme.Bg).Background(m.theme.Accent).Bold(true)
	case isToday:
		st = st.Underline(true).Bold(true)
	case !inMonth:
		st = st.Foreground(m.theme.Border)
	}
	return st.Render(body) + " "
}

// — dayoffs view —

func (m Model) renderDayOffs() string {
	inner := m.width - 4
	var rows []string
	rows = append(rows, m.renderTabBar())

	year := m.dayoffsYear
	if year == 0 {
		year = m.now.Year()
	}

	if !m.dayoffsLoaded {
		rows = append(rows, stDim(m.theme, "  lade…"))
	} else {
		rows = append(rows, m.renderDayOffsBody(inner)...)
	}

	rows = append(rows, "")
	body := strings.Join(rows, "\n")
	titleParts := []string{"Worktime", fmt.Sprintf("Frei %d", year)}
	if year == m.now.Year() {
		titleParts = append(titleParts, "aktuelles Jahr")
	}
	title := boxTitle(titleParts, m.width)
	box := titlebox.Render(title, body, m.width, m.theme)
	return box + m.renderToastRow() + "\n" + wrapFooter(m.theme,
		"a anlegen  ·  A heute=Urlaub  ·  K heute=krank  ·  B Feiertage-sync  ·  d/x löschen  ·  h/l/[/] Jahr ±  ·  T aktuell  ·  j/k auswahl  ·  tab heute  ·  ? hilfe  ·  b zurück  ·  q schließen",
		m.width)
}

func (m Model) renderDayOffsBody(inner int) []string {
	if len(m.dayoffs) == 0 {
		hintChips := []string{
			stDim(m.theme, "a → Tag eintragen"),
			stDim(m.theme, "Feiertag"),
			stDim(m.theme, "Urlaub"),
			stDim(m.theme, "Krankheit"),
		}
		return []string{
			stDim(m.theme, "  Noch keine Daten in diesem Jahr."),
			"",
			joinWrapped(hintChips, "  ·  ", "  ", "  ", inner),
		}
	}

	var rows []string
	// Per-kind summary strip.
	byKind := map[wt.Kind]int{}
	for _, d := range m.dayoffs {
		byKind[d.Kind]++
	}
	parts := []string{}
	for _, k := range wt.AllKinds {
		if c := byKind[k]; c > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", k.LabelDe(), c))
		}
	}
	dimStyled := make([]string, len(parts))
	for i, p := range parts {
		dimStyled[i] = stDim(m.theme, p)
	}
	rows = append(rows, joinWrapped(dimStyled, stDim(m.theme, "  ·  "), "  ", "  ", inner))
	rows = append(rows, "")
	rows = append(rows, picker.SectionHeader(fmt.Sprintf("einträge (%d)", len(m.dayoffs)), inner, m.theme))

	for i, d := range m.dayoffs {
		date := germanWeekdayShort(d.Date.Weekday()) + " " + d.Date.Format("02.01.")
		dur := lipgloss.NewStyle().Width(10).Foreground(m.theme.Dim).Render(date)
		kindStr := lipgloss.NewStyle().Width(10).Foreground(kindColor(m.theme, d.Kind)).Render(d.Kind.LabelDe())
		label := dur + "  " + kindStr + "  " + d.Label
		rows = append(rows, picker.Row(i == m.dayoffsCur, label, "", inner, m.theme))
	}
	return rows
}

func kindColor(p tk.Palette, k wt.Kind) lipgloss.TerminalColor {
	switch k {
	case wt.KindHoliday:
		return p.Cyan
	case wt.KindVacation:
		return p.Green
	case wt.KindSick:
		return p.Yellow
	}
	return p.Fg
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
		rows = append(rows, m.parsePreview(m.input.Value()))
		if m.errMsg != "" {
			rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Red).Render("    "+m.errMsg))
		}

	case dialogEntryForm:
		title = "Worktime · Manueller Eintrag"
		hint = "Tab/↑↓ Feld  ·  Ctrl+T Vorlage  ·  Enter=weiter/speichern  ·  Esc=abbrechen"
		// Render the template strip above the form so the affordance is the
		// first thing the user sees on opening — Ctrl+T is otherwise easy to
		// miss at the bottom.
		if line := m.renderTemplateStrip(); line != "" {
			rows = append(rows, picker.SectionHeader("vorlagen  (Ctrl+T cyclen)", inner, m.theme))
			rows = append(rows, "  "+line)
			rows = append(rows, "")
		}
		rows = append(rows, m.renderForm(inner)...)

	case dialogStopAt:
		title = "Worktime · Stoppen"
		hint = "HH:MM  ·  -30m  ·  Enter=jetzt  ·  Esc=abbrechen"
		rows = append(rows, picker.SectionHeader("stoppzeit", inner, m.theme))
		rows = append(rows, "  "+m.input.View())
		rows = append(rows, m.parsePreview(m.input.Value()))
		if m.errMsg != "" {
			rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Red).Render("    "+m.errMsg))
		}

	case dialogCorrect:
		title = "Worktime · Startzeit korrigieren"
		hint = "HH:MM  ·  -1h30m  ·  Enter=bestätigen  ·  Esc=abbrechen"
		rows = append(rows, picker.SectionHeader("neue startzeit", inner, m.theme))
		rows = append(rows, "  "+m.input.View())
		rows = append(rows, m.parsePreview(m.input.Value()))
		if m.errMsg != "" {
			rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Red).Render("    "+m.errMsg))
		}

	case dialogEditForm:
		title = "Worktime · Session bearbeiten"
		hint = "Tab/↑↓ Feld  ·  Enter=weiter/speichern  ·  Esc=abbrechen"
		if m.editIdx < len(m.day.Sessions) && sameDay(m.editDate, m.now) {
			s := m.day.Sessions[m.editIdx]
			rows = append(rows, stDim(m.theme, fmt.Sprintf("  Session %d:  %s → %s",
				m.editIdx+1, s.Start.Format("15:04"), s.Stop.Format("15:04"))))
			rows = append(rows, "")
		} else if m.editIdx < len(m.drillSessions) {
			s := m.drillSessions[m.editIdx]
			rows = append(rows, stDim(m.theme, fmt.Sprintf("  Session %d:  %s → %s",
				m.editIdx+1, s.Start.Format("15:04"), s.Stop.Format("15:04"))))
			rows = append(rows, "")
		}
		rows = append(rows, m.renderForm(inner)...)

	case dialogTagForm:
		title = "Worktime · Tag setzen"
		hint = "Tab=letzte  ·  Shift+Tab=top  ·  Enter=speichern  ·  leer=löschen  ·  Esc=abbrechen"
		rows = append(rows, picker.SectionHeader("tag", inner, m.theme))
		rows = append(rows, "  "+m.input.View())
		prefix := m.input.Value()
		if len(m.recentTags) > 0 {
			rows = append(rows, "")
			rows = append(rows, stDim(m.theme, "  letzte tags:"))
			rows = append(rows, "  "+m.renderTagSuggestionsList(m.recentTags, m.tagSugCur, prefix))
		}
		if len(m.topTags) > 0 {
			rows = append(rows, "")
			rows = append(rows, stDim(m.theme, "  top by usage:"))
			rows = append(rows, "  "+m.renderTagSuggestionsList(m.topTags, m.topSugCur, prefix))
		}

	case dialogNoteForm:
		title = "Worktime · Session-Notiz"
		hint = "Enter=speichern  ·  leer=löschen  ·  Esc=abbrechen"
		rows = append(rows, picker.SectionHeader("notiz", inner, m.theme))
		rows = append(rows, "  "+m.input.View())

	case dialogDeleteConfirm:
		title = "Worktime · Session löschen"
		hint = "y/z/j=löschen  ·  Enter/n/Esc=abbrechen (default)"
		s, ok := m.deleteTarget()
		if ok {
			afterTotal := m.day.Total(m.now) - s.Elapsed
			if !sameDay(m.editDate, m.now) {
				afterTotal = drillTotal(m.drillSessions) - s.Elapsed
			}
			rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Yellow).Bold(true).
				Render(fmt.Sprintf("  Session %d:  %s → %s  (%s)",
					m.editIdx+1, s.Start.Format("15:04"), s.Stop.Format("15:04"), formatDur(s.Elapsed))))
			rows = append(rows, stDim(m.theme,
				fmt.Sprintf("  Tagestotal danach:  %s", formatDur(afterTotal))))
			rows = append(rows, "")
			rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Red).
				Render("  Wirklich löschen?"))
			rows = append(rows, "  "+confirmButton(m.theme, "y/z/j löschen", false)+
				"   "+confirmButton(m.theme, "Enter abbrechen", true))
		}

	case dialogNotePicker:
		title = "Worktime · Notiz anhängen"
		hint = "↑/↓ wählen  ·  Enter=anhängen/detachen  ·  Esc=abbrechen  ·  tippen=filtern"
		rows = append(rows, m.renderNotePickerBody(inner)...)

	case dialogDayDetail:
		title = fmt.Sprintf("Worktime · Tag %s", m.drillDate.Format("2006-01-02"))
		hint = "j/k auswahl  ·  E bearbeiten  ·  d löschen  ·  t tag  ·  N notiz  ·  b/Esc zurück"
		rows = append(rows, m.renderDayDetailBody(inner)...)

	case dialogHelp:
		title = "Worktime · Hilfe"
		hint = "irgendeine Taste schließt"
		rows = append(rows, m.renderHelpBody(inner)...)

	case dialogHistFilter:
		title = "Worktime · History-Filter"
		hint = "Enter=anwenden  ·  leer=alles  ·  Esc=abbrechen"
		rows = append(rows, picker.SectionHeader("filter", inner, m.theme))
		rows = append(rows, "  "+m.input.View())
		// Tag suggestions: when the user has typed (or pressed F to seed)
		// "tag:", show the top-usage tags as click-to-paste chips. Highlights
		// matches against the prefix the user has typed after "tag:".
		val := strings.ToLower(strings.TrimSpace(m.input.Value()))
		if strings.HasPrefix(val, "tag:") {
			tagPrefix := strings.TrimSpace(val[len("tag:"):])
			if len(m.topTags) > 0 || len(m.recentTags) > 0 {
				rows = append(rows, "")
				if len(m.topTags) > 0 {
					rows = append(rows, stDim(m.theme, "  top tags:"))
					rows = append(rows, "  "+m.renderTagSuggestionsList(m.topTags, -1, tagPrefix))
				}
				if len(m.recentTags) > 0 {
					rows = append(rows, stDim(m.theme, "  letzte tags:"))
					rows = append(rows, "  "+m.renderTagSuggestionsList(m.recentTags, -1, tagPrefix))
				}
				rows = append(rows, "")
			}
		}
		rows = append(rows, stDim(m.theme,
			"  Beispiele:  KW18  ·  2026  ·  2026-04  ·  2026-04-01..2026-04-30  ·  tag:deep  ·  note:standup"))

	case dialogDayOffAdd:
		title = "Worktime · Tag(e) frei eintragen"
		hint = "Tab/↑↓ Feld  ·  h/l Kategorie  ·  Enter=speichern  ·  Esc=abbrechen"
		// renderDayOffAddForm renders the inline errMsg under the focused field.
		rows = append(rows, m.renderDayOffAddForm(inner)...)

	case dialogDayOffConfirm:
		title = "Worktime · Eintrag löschen"
		hint = "y/z/j=löschen  ·  Enter/n/Esc=abbrechen (default)"
		if m.dayoffsCur >= 0 && m.dayoffsCur < len(m.dayoffs) {
			d := m.dayoffs[m.dayoffsCur]
			rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Yellow).Bold(true).
				Render(fmt.Sprintf("  %s  %s  %s",
					d.Date.Format("2006-01-02"), d.Kind.LabelDe(), d.Label)))
			rows = append(rows, "")
			rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Red).
				Render("  Wirklich löschen?"))
			rows = append(rows, "  "+confirmButton(m.theme, "y/z/j löschen", false)+
				"   "+confirmButton(m.theme, "Enter abbrechen", true))
		}

	case dialogStopChoice:
		title = "Worktime · Sehr kurze Session"
		hint = "y/z/j=stoppen  ·  t=zeit wählen  ·  Enter/n/Esc=weiter (default)"
		elapsed := time.Duration(0)
		if m.day.Active != nil {
			elapsed = m.now.Sub(*m.day.Active)
		}
		rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Yellow).
			Render(fmt.Sprintf("  Erst %s gelaufen — versehentlich gestoppt?", formatDur(elapsed))))
		rows = append(rows, "")
		rows = append(rows, "  "+lipgloss.NewStyle().Bold(true).Render("y/z/j")+stDim(m.theme, "  jetzt stoppen"))
		rows = append(rows, "  "+lipgloss.NewStyle().Bold(true).Render("t  ")+stDim(m.theme, "  Stop-Zeit wählen"))
		rows = append(rows, "  "+confirmButton(m.theme, "Enter / n  weiterlaufen lassen", true))
	}

	rows = append(rows, "")
	// Dialog titles already use " · " as separator; route through boxTitle so
	// long titles (e.g. "Worktime · Tag YYYY-MM-DD") don't push the corner off.
	titleClamped := boxTitle(strings.Split(title, " · "), m.width)
	box := titlebox.Render(titleClamped, strings.Join(rows, "\n"), m.width, m.theme)
	return box + m.renderToastRow() + "\n" + wrapFooter(m.theme, hint, m.width)
}

// renderForm renders the active multi-field form (entry/edit).
//
// Layout per field:
//
//	SectionHeader (label)
//	value or live input
//	parsePreview / inline errMsg (focused field only)
//	thin separator (between fields)
//
// errMsg appears under the focused field whose validation failed instead of
// at the end of the form — that way the eye doesn't have to jump down past
// the duration line to find the problem, and the layout above stays stable.
func (m Model) renderForm(inner int) []string {
	var rows []string
	labels := m.formLabels()
	sep := stDim(m.theme, "  "+strings.Repeat("─", maxInt(8, inner-4)))
	for i, ti := range m.formInputs {
		focused := i == m.formCur
		rows = append(rows, picker.SectionHeader(labels[i], inner, m.theme))
		var line string
		if focused {
			line = "  " + ti.View()
		} else {
			val := ti.Value()
			if val == "" {
				val = stDim(m.theme, ti.Placeholder)
			}
			line = "    " + val
		}
		rows = append(rows, line)
		switch {
		case focused && m.errMsg != "":
			rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Red).Render("    "+m.errMsg))
		case focused:
			rows = append(rows, m.parsePreview(ti.Value()))
		default:
			rows = append(rows, "")
		}
		// Thin separator between fields (skipped after the last one).
		if i < len(m.formInputs)-1 {
			rows = append(rows, sep)
		}
	}
	if line := m.renderFormDurationLine(); line != "" {
		rows = append(rows, "", line)
	}
	if line := m.renderEditDiffLine(); line != "" {
		rows = append(rows, line)
	}
	return rows
}

// renderEditDiffLine builds a "Δ Start 09:00 → 09:30  ·  Tag deep → focus"
// line for dialogEditForm, listing only the fields that differ from the
// session being edited. Returns "" outside edit mode or when nothing changed.
// Helps the user verify a non-trivial edit before pressing Enter.
func (m Model) renderEditDiffLine() string {
	if m.dialog != dialogEditForm || len(m.formInputs) < 2 {
		return ""
	}
	var orig wt.Session
	switch {
	case sameDay(m.editDate, m.now) && m.editIdx >= 0 && m.editIdx < len(m.day.Sessions):
		orig = m.day.Sessions[m.editIdx]
	case sameDay(m.editDate, m.drillDate) && m.editIdx >= 0 && m.editIdx < len(m.drillSessions):
		orig = m.drillSessions[m.editIdx]
	default:
		return ""
	}
	values := m.formValues()
	curStart := values[0]
	curStop := values[1]
	curTag := ""
	curNote := ""
	if len(values) >= 3 {
		curTag = values[2]
	}
	if len(values) >= 4 {
		curNote = values[3]
	}
	var diffs []string
	yellow := lipgloss.NewStyle().Foreground(m.theme.Yellow)
	if origStart := orig.Start.Format("15:04"); curStart != "" && curStart != origStart {
		diffs = append(diffs, yellow.Render("Start "+origStart+" → "+curStart))
	}
	if origStop := orig.Stop.Format("15:04"); curStop != "" && curStop != origStop {
		diffs = append(diffs, yellow.Render("Stop "+origStop+" → "+curStop))
	}
	if curTag != orig.Tag {
		from := orig.Tag
		if from == "" {
			from = "—"
		}
		to := curTag
		if to == "" {
			to = "—"
		}
		diffs = append(diffs, yellow.Render("Tag "+from+" → "+to))
	}
	if curNote != orig.Note {
		from := orig.Note
		if from == "" {
			from = "—"
		}
		to := curNote
		if to == "" {
			to = "—"
		}
		diffs = append(diffs, yellow.Render("Notiz "+from+" → "+to))
	}
	if len(diffs) == 0 {
		return ""
	}
	return stDim(m.theme, "    Δ ") + strings.Join(diffs, stDim(m.theme, "  ·  "))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// renderFormDurationLine computes "Dauer: Xh Ym  ·  Tagestotal danach: A/B"
// for the active entry- or edit-form. Returns "" when start/stop can't be
// parsed yet — the line stays hidden until the user has typed something
// meaningful instead of showing zero.
func (m Model) renderFormDurationLine() string {
	var startStr, stopStr string
	var anchor time.Time
	switch m.dialog {
	case dialogEntryForm:
		if len(m.formInputs) < 3 {
			return ""
		}
		dateStr := strings.TrimSpace(m.formInputs[0].Value())
		startStr = strings.TrimSpace(m.formInputs[1].Value())
		stopStr = strings.TrimSpace(m.formInputs[2].Value())
		if dateStr == "" {
			anchor = m.now
		} else if d, err := time.ParseInLocation("2006-01-02", dateStr, m.now.Location()); err == nil {
			anchor = d
		} else {
			return ""
		}
	case dialogEditForm:
		if len(m.formInputs) < 2 {
			return ""
		}
		startStr = strings.TrimSpace(m.formInputs[0].Value())
		stopStr = strings.TrimSpace(m.formInputs[1].Value())
		anchor = m.editDate
		if anchor.IsZero() {
			anchor = m.now
		}
	default:
		return ""
	}
	startD, errStart := wt.ParseHM(startStr)
	if errStart != nil {
		return ""
	}
	base := time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 0, 0, 0, 0, anchor.Location())
	startTime := base.Add(startD)
	var stopTime time.Time
	switch {
	case stopStr == "":
		// Empty stop → "now" only when anchor is today; otherwise leave blank.
		if !sameDay(anchor, m.now) {
			return ""
		}
		stopTime = m.now
	default:
		t, err := wt.ParseStop(stopStr, startTime)
		if err != nil {
			return ""
		}
		// HH:MM forms are anchored on time.Now's date — rebase to the form's
		// anchor so cross-day computation stays sane.
		if stopStr[0] != '+' {
			if stopHM, err := wt.ParseHM(stopStr); err == nil {
				t = base.Add(stopHM)
			}
		}
		stopTime = t
	}
	dur := stopTime.Sub(startTime)
	if dur <= 0 {
		return ""
	}

	// Day-total projection: simulate the edit/entry against the day's other
	// sessions so the user sees the saldo before committing.
	var existing []wt.Session
	switch {
	case sameDay(anchor, m.now):
		existing = m.day.Sessions
	case len(m.drillSessions) > 0 && sameDay(anchor, m.drillDate):
		existing = m.drillSessions
	}
	var afterTotal time.Duration
	skipIdx := -1
	if m.dialog == dialogEditForm {
		skipIdx = m.editIdx
	}
	for i, s := range existing {
		if i == skipIdx {
			continue
		}
		afterTotal += s.Elapsed
	}
	afterTotal += dur

	target := wt.TargetFor(anchor)
	parts := []string{
		stDim(m.theme, "    Dauer: ") +
			lipgloss.NewStyle().Foreground(m.theme.Fg).Bold(true).Render(formatDur(dur)),
	}
	if target > 0 {
		col := m.theme.Yellow
		if afterTotal >= target {
			col = m.theme.Green
		}
		parts = append(parts, stDim(m.theme, "Tagestotal danach: ")+
			lipgloss.NewStyle().Foreground(col).Render(
				fmt.Sprintf("%s / %s", formatDur(afterTotal), formatDur(target))))
	} else {
		parts = append(parts, stDim(m.theme, "Tagestotal danach: ")+
			lipgloss.NewStyle().Foreground(m.theme.Fg).Render(formatDur(afterTotal)))
	}
	return strings.Join(parts, stDim(m.theme, "  ·  "))
}

// renderDayOffAddForm renders the dayoff-add form: date, label, kind picker.
// Kind is a virtual third "field" rendered as horizontal radio buttons.
// Inline errMsg appears under the focused field; thin separators visually
// group fields on narrow panes.
func (m Model) renderDayOffAddForm(inner int) []string {
	var rows []string
	labels := []string{"datum (YYYY-MM-DD oder YYYY-MM-DD..YYYY-MM-DD)", "label (z.B. Brückentag)"}
	sep := stDim(m.theme, "  "+strings.Repeat("─", maxInt(8, inner-4)))
	totalFields := len(m.formInputs) + 1 // +1 for the virtual kind picker
	for i, ti := range m.formInputs {
		focused := i == m.formCur
		rows = append(rows, picker.SectionHeader(labels[i], inner, m.theme))
		var line string
		if focused {
			line = "  " + ti.View()
		} else {
			val := ti.Value()
			if val == "" {
				val = stDim(m.theme, ti.Placeholder)
			}
			line = "    " + val
		}
		rows = append(rows, line)
		if focused && m.errMsg != "" {
			rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Red).Render("    "+m.errMsg))
		} else {
			rows = append(rows, "")
		}
		if i < totalFields-1 {
			rows = append(rows, sep)
		}
	}
	// Kind picker as a virtual trailing field.
	kindFocused := m.formCur == len(m.formInputs)
	rows = append(rows, picker.SectionHeader("kategorie  (h/l zum Wechseln)", inner, m.theme))
	chips := make([]string, 0, len(wt.AllKinds))
	for i, k := range wt.AllKinds {
		label := k.LabelDe()
		st := lipgloss.NewStyle().Foreground(m.theme.Dim)
		if i == m.dayoffKindCur {
			if kindFocused {
				st = lipgloss.NewStyle().Foreground(m.theme.Bg).Background(m.theme.Accent).Bold(true)
			} else {
				st = lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true)
			}
		}
		chips = append(chips, st.Render(" "+label+" "))
	}
	rows = append(rows, "  "+strings.Join(chips, "  "))
	if kindFocused && m.errMsg != "" {
		rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Red).Render("    "+m.errMsg))
	}
	return rows
}

// renderTemplateStrip renders the loaded session-templates as chips. The
// currently applied chip (templateCur) gets the inverted-style; the rest
// stay neutral. Empty when nothing is loaded.
func (m Model) renderTemplateStrip() string {
	if len(m.templates) == 0 {
		return ""
	}
	chips := make([]string, 0, len(m.templates))
	for i, t := range m.templates {
		startH := int(t.Start.Hours())
		startM := int(t.Start.Minutes()) % 60
		durMin := int(t.Duration.Minutes())
		var label string
		switch {
		case t.Tag != "":
			label = fmt.Sprintf("%02d:%02d +%dm %s", startH, startM, durMin, t.Tag)
		default:
			label = fmt.Sprintf("%02d:%02d +%dm", startH, startM, durMin)
		}
		st := lipgloss.NewStyle().Foreground(m.theme.Fg)
		if i == m.templateCur {
			st = lipgloss.NewStyle().Foreground(m.theme.Bg).Background(m.theme.Accent).Bold(true)
		}
		chips = append(chips, st.Render(" "+label+" "))
	}
	return strings.Join(chips, "  ")
}

// renderTagSuggestions renders the recentTags chips, highlighting tagSugCur
// when set. Used inside the dialogTagForm overlay.
func (m Model) renderTagSuggestions() string {
	return m.renderTagSuggestionsList(m.recentTags, m.tagSugCur, "")
}

// renderTagSuggestionsList is the parametrised renderer used for both the
// recency strip and the usage-top strip in the tag form. `prefix` (when
// non-empty) underlines chips whose name starts with it — so as the user
// types, matching suggestions stand out alongside the cursor highlight.
func (m Model) renderTagSuggestionsList(tags []string, cur int, prefix string) string {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	chips := make([]string, 0, len(tags))
	for i, t := range tags {
		st := lipgloss.NewStyle().Foreground(m.theme.Fg)
		isMatch := prefix != "" && strings.HasPrefix(strings.ToLower(t), prefix)
		switch {
		case i == cur:
			st = lipgloss.NewStyle().Foreground(m.theme.Bg).Background(m.theme.Accent).Bold(true)
		case isMatch:
			st = lipgloss.NewStyle().Foreground(m.theme.Cyan).Bold(true).Underline(true)
		}
		chips = append(chips, st.Render(" "+t+" "))
	}
	return strings.Join(chips, "  ")
}

func (m Model) formLabels() []string {
	switch m.dialog {
	case dialogEntryForm:
		return []string{
			"datum (YYYY-MM-DD)",
			"start (HH:MM oder -1h30m)",
			"stop (HH:MM, +1h30m oder leer=jetzt)",
		}
	case dialogEditForm:
		return []string{
			"start (HH:MM)",
			"stop (HH:MM oder +1h30m)",
			"tag (leer=keiner)",
			"notiz (leer=keine)",
		}
	}
	out := make([]string, len(m.formInputs))
	for i := range out {
		out[i] = fmt.Sprintf("feld %d", i+1)
	}
	return out
}

// parsePreview renders a "→ HH:MM" hint for time strings (HH:MM or relative
// offsets). Returns an empty styled line when nothing meaningful to show so
// row positions stay stable.
func (m Model) parsePreview(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Accept date-style values silently (no time preview).
	if _, err := time.Parse("2006-01-02", s); err == nil {
		return ""
	}
	ts, err := wt.ParseStartArg(s)
	if err != nil {
		return lipgloss.NewStyle().Foreground(m.theme.Red).Render("    " + err.Error())
	}
	return stDim(m.theme, "    → "+ts.Format("15:04"))
}

func (m Model) deleteTarget() (wt.Session, bool) {
	if sameDay(m.editDate, m.now) && m.editIdx < len(m.day.Sessions) {
		return m.day.Sessions[m.editIdx], true
	}
	if m.editIdx < len(m.drillSessions) {
		return m.drillSessions[m.editIdx], true
	}
	return wt.Session{}, false
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

func drillTotal(sessions []wt.Session) time.Duration {
	var total time.Duration
	for _, s := range sessions {
		total += s.Elapsed
	}
	return total
}

func (m Model) renderDayDetailBody(inner int) []string {
	var rows []string
	if len(m.drillSessions) == 0 {
		rows = append(rows, stDim(m.theme, "  keine Sessions an diesem Tag"))
		return rows
	}
	target := wt.TargetFor(m.drillDate)
	total := drillTotal(m.drillSessions)
	pct := 0
	if target > 0 {
		pct = int(total * 100 / target)
	}
	rows = append(rows, "  "+lipgloss.NewStyle().Foreground(m.theme.Fg).Bold(true).Render(formatDur(total))+
		"  "+stDim(m.theme, fmt.Sprintf("/ %s  ·  %d%%", formatDur(target), pct)))
	rows = append(rows, "")
	rows = append(rows, picker.SectionHeader(fmt.Sprintf("sessions (%d)", len(m.drillSessions)), inner, m.theme))
	maxSess := time.Duration(0)
	for _, s := range m.drillSessions {
		if s.Elapsed > maxSess {
			maxSess = s.Elapsed
		}
	}
	prevStop := time.Time{}
	for i, s := range m.drillSessions {
		if !prevStop.IsZero() {
			pause := s.Start.Sub(prevStop)
			if pause > 0 {
				rows = append(rows, stDim(m.theme,
					fmt.Sprintf("       ─ %s Pause ─", formatDur(pause))))
			}
		}
		prevStop = s.Stop
		bar := sessionMiniBar(m.theme, s.Elapsed, maxSess, 10)
		dur := lipgloss.NewStyle().Width(8).Render(formatDur(s.Elapsed))
		label := fmt.Sprintf("%s → %s   %s  %s",
			s.Start.Format("15:04"), s.Stop.Format("15:04"), dur, bar)
		hint := ""
		if s.Tag != "" {
			hint = stDim(m.theme, "["+s.Tag+"]")
		}
		rows = append(rows, picker.Row(i == m.drillCur, label, hint, inner, m.theme))
		if s.Note != "" {
			rows = append(rows, stDim(m.theme, "       "+s.Note))
		}
	}
	return rows
}

func (m Model) renderHelpBody(inner int) []string {
	rows := []string{picker.SectionHeader("tabs", inner, m.theme)}
	rows = append(rows, "  tab/1/2/3/4 Heute / Woche / History / Frei")
	rows = append(rows, "  shift+tab   rückwärts")
	rows = append(rows, "  b           voriger Tab (auf Heute → Palette)")
	rows = append(rows, "")
	rows = append(rows, picker.SectionHeader("heute", inner, m.theme))
	rows = append(rows, "  s           starten / stoppen / resume (im Pause-Modus)")
	rows = append(rows, "  S           neu starten (Dialog) — verwirft Pause")
	rows = append(rows, "  p           pause (laufend → Pause)")
	rows = append(rows, "  f           fokus-modus: start + daily note")
	rows = append(rows, "  C           startzeit fix (laufend)")
	rows = append(rows, "  e           manueller eintrag")
	rows = append(rows, "  E / enter   session bearbeiten (Start, Stop, Tag, Notiz)")
	rows = append(rows, "  d           session löschen")
	rows = append(rows, "  u           undo letzte löschung")
	rows = append(rows, "  t / N       tag / notiz für session")
	rows = append(rows, "  n           kompendium-notiz attach / detach")
	rows = append(rows, "  o / O / D   notiz: ansehen / editor / lösen")
	rows = append(rows, "  Y           gestern drilldown")
	rows = append(rows, "")
	rows = append(rows, picker.SectionHeader("woche / history", inner, m.theme))
	rows = append(rows, "  enter       tag drill-down (heute → Heute-Tab)")
	rows = append(rows, "  h / l       woche zurück / vor (woche-tab)")
	rows = append(rows, "  T           sprung zu aktueller Woche / kein Filter")
	rows = append(rows, "  v           cyclet history-mode: list → heatmap → tagclock → month")
	rows = append(rows, "  /           filter öffnen (history)")
	rows = append(rows, "  F           tag-quickpicker (history) — top tags als chips")
	rows = append(rows, "  [ / ]       paginate KW/Monat im Filter (history)")
	rows = append(rows, "")
	rows = append(rows, picker.SectionHeader("history-filter syntax", inner, m.theme))
	rows = append(rows, "  KW18                       ISO-Woche (aktuelles Jahr)")
	rows = append(rows, "  2026                       ganzes Jahr")
	rows = append(rows, "  2026-04                    ein Monat")
	rows = append(rows, "  2026-04-01..2026-04-30     beliebiger Bereich")
	rows = append(rows, "  tag:deep                   nur Sessions mit Tag »deep«")
	rows = append(rows, "  note:standup               Notiz-Substring-Suche")
	rows = append(rows, "  f                          Tag-Quick-Picker (Multi-Select)")
	rows = append(rows, "  y / Y       yank tag / range als Markdown")
	rows = append(rows, "  h/j/k/l     heatmap-cursor (history-heatmap)")
	rows = append(rows, "")
	rows = append(rows, picker.SectionHeader("frei", inner, m.theme))
	rows = append(rows, "  a           tag(e) frei eintragen (Form)")
	rows = append(rows, "  A / K       quick: heute=Urlaub / heute=Krank")
	rows = append(rows, "  B           Bundesland-Feiertage syncen ($WORKTIME_LAND, default NW)")
	rows = append(rows, "  d / x       eintrag löschen")
	rows = append(rows, "  h / l       jahr zurück / vor")
	rows = append(rows, "  T           sprung zu aktuellem jahr")
	rows = append(rows, "")
	rows = append(rows, stDim(m.theme, "  CLI: flow worktime dayoff export --format ics > kalender.ics"))
	rows = append(rows, "")
	rows = append(rows, picker.SectionHeader("eingabe / dialoge", inner, m.theme))
	rows = append(rows, "  +1h30m      stop-feld: dauer ab start (statt absolute Zeit)")
	rows = append(rows, "  Tab/⇧Tab    tag-form: letzte / top-by-usage")
	rows = append(rows, "  Ctrl+T      entry-form: vorlage cyclen")
	rows = append(rows, "  y/z/j       confirm (QWERTZ-friendly)")
	rows = append(rows, "  esc/q       help schließen")
	rows = append(rows, "")
	rows = append(rows, picker.SectionHeader("global", inner, m.theme))
	rows = append(rows, "  j/k ↑/↓     auswahl")
	rows = append(rows, "  g / G       oben / unten")
	rows = append(rows, "  r           neu laden")
	rows = append(rows, "  ?           diese hilfe")
	rows = append(rows, "  q           schließen")
	return rows
}

func (m Model) renderNotePickerBody(inner int) []string {
	rows := []string{
		picker.SectionHeader("Kompendium-Notizen", inner, m.theme),
		"  " + m.input.View(),
		"",
	}

	switch {
	case m.pickerErr != nil:
		rows = append(rows, stErr(m.theme, m.pickerErr.Error()))
		return rows
	case m.picker == nil:
		rows = append(rows, stDim(m.theme, "  lade…"))
		return rows
	}

	filtered := m.filteredPicker()
	if len(filtered) == 0 {
		rows = append(rows, stDim(m.theme, "  Keine Treffer."))
		return rows
	}

	attached := make(map[string]bool)
	for _, n := range m.notes {
		attached[n.id] = true
	}

	const maxRows = 10
	start := 0
	if m.pickerCur >= maxRows {
		start = m.pickerCur - maxRows + 1
	}
	end := start + maxRows
	if end > len(filtered) {
		end = len(filtered)
	}
	for i := start; i < end; i++ {
		n := filtered[i]
		label := wt.HumanizeNoteID(n.ID)
		hintStr := ""
		if attached[n.ID] {
			hintStr = stDim(m.theme, "● angehängt → Enter detacht")
		}
		rows = append(rows, picker.Row(i == m.pickerCur, label, hintStr, inner, m.theme))
	}
	if len(filtered) > maxRows {
		rows = append(rows, stDim(m.theme,
			fmt.Sprintf("  %d / %d", m.pickerCur+1, len(filtered))))
	}
	return rows
}

// — commands —

func refreshDayCmd() tea.Cmd {
	return func() tea.Msg {
		day, err := wt.LoadToday()
		return dayRefreshMsg{day: day, err: err}
	}
}

func loadTodayCmd(now time.Time) tea.Cmd {
	return func() tea.Msg {
		day, err := wt.LoadToday()
		if err != nil {
			return todayLoadedMsg{err: err}
		}
		var notes []noteEntry
		if wt.DailyExists(now) {
			notes = append(notes, noteEntry{
				isDaily: true,
				id:      wt.DailyNoteID(now),
				label:   "Tagesnotiz",
			})
		}
		ids, _ := wt.ListLinks(now)
		for _, id := range ids {
			notes = append(notes, noteEntry{id: id, label: wt.HumanizeNoteID(id)})
		}
		// History walk for streak/aggregates is cheap enough at this cadence
		// (only on action and screen open, not on the per-second tick).
		hist, _ := wt.LoadHistory()
		st := wt.Aggregate(hist)
		return todayLoadedMsg{day: day, notes: notes, stats: st, history: hist}
	}
}

func openNoteCmd(id string) tea.Cmd {
	return func() tea.Msg {
		_ = wt.OpenNote(id)
		return nil
	}
}

func viewNoteCmd(id string) tea.Cmd {
	return func() tea.Msg {
		_ = wt.ViewNote(id)
		return nil
	}
}

func detachNoteCmd(date time.Time, id string) tea.Cmd {
	return func() tea.Msg {
		if err := wt.RemoveLink(date, id); err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{toast: "✓ Notiz losgelöst"}
	}
}

func attachNoteCmd(date time.Time, id string) tea.Cmd {
	return func() tea.Msg {
		if err := wt.AddLink(date, id); err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{toast: "✓ Notiz angehängt"}
	}
}

type notesLoadedMsg struct {
	notes []wt.KompendiumNote
	err   error
}

func loadNotesCmd() tea.Cmd {
	return func() tea.Msg {
		notes, err := wt.ListKompendiumNotes()
		return notesLoadedMsg{notes: notes, err: err}
	}
}

func loadWeekCmd(ref time.Time) tea.Cmd {
	return func() tea.Msg {
		week, err := wt.LoadWeek(ref)
		return weekLoadedMsg{week: week, err: err}
	}
}

func loadHistoryCmd() tea.Cmd {
	return func() tea.Msg {
		h, err := wt.LoadHistory()
		return historyLoadedMsg{history: h, err: err}
	}
}

type dayDetailLoadedMsg struct {
	date     time.Time
	sessions []wt.Session
	err      error
}

func loadDayDetailCmd(date time.Time) tea.Cmd {
	return func() tea.Msg {
		all, err := wt.LoadHistory()
		if err != nil {
			return dayDetailLoadedMsg{date: date, err: err}
		}
		var found []wt.Session
		key := date.Format("2006-01-02")
		for _, d := range all {
			if d.Date.Format("2006-01-02") == key {
				found = d.Sessions
				break
			}
		}
		return dayDetailLoadedMsg{date: date, sessions: found}
	}
}

func setTagCmd(date time.Time, idx int, tag string) tea.Cmd {
	return func() tea.Msg {
		if err := wt.SetTag(date, idx, tag); err != nil {
			return actionDoneMsg{err: err}
		}
		if tag == "" {
			return actionDoneMsg{toast: fmt.Sprintf("✓ Tag entfernt (Session %d)", idx+1)}
		}
		return actionDoneMsg{toast: fmt.Sprintf("✓ Tag »%s« gesetzt (Session %d)", tag, idx+1)}
	}
}

func setNoteCmd(date time.Time, idx int, note string) tea.Cmd {
	return func() tea.Msg {
		if err := wt.SetNote(date, idx, note); err != nil {
			return actionDoneMsg{err: err}
		}
		if note == "" {
			return actionDoneMsg{toast: fmt.Sprintf("✓ Notiz entfernt (Session %d)", idx+1)}
		}
		return actionDoneMsg{toast: fmt.Sprintf("✓ Notiz gespeichert (Session %d)", idx+1)}
	}
}

func startCmd(ts time.Time) tea.Cmd {
	return func() tea.Msg {
		if err := wt.Start(ts); err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{toast: "▶ Worktime gestartet — " + ts.Format("15:04")}
	}
}

// startForceCmd is used after the user confirms an "trotzdem starten" prompt
// raised by ErrAlreadyRunning, or via the capital `S` key from the Pause
// state.
func startForceCmd(ts time.Time) tea.Cmd {
	return func() tea.Msg { return actionDoneMsg{err: wt.StartForce(ts)} }
}

func pauseCmd() tea.Cmd {
	return func() tea.Msg {
		s, err := wt.Pause()
		if err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{toast: fmt.Sprintf("⏸ Pausiert nach %s", formatDur(s.Elapsed))}
	}
}

// resumeCmd resumes a paused session. `pausedAt` is the wall-clock time the
// user paused at (or zero when unknown) — included in the toast so the user
// sees how long the break lasted at a glance.
func resumeCmd(pausedAt time.Time) tea.Cmd {
	return func() tea.Msg {
		if err := wt.Resume(); err != nil {
			return actionDoneMsg{err: err}
		}
		if !pausedAt.IsZero() {
			gap := time.Since(pausedAt)
			return actionDoneMsg{toast: fmt.Sprintf("▶ Resume nach %s Pause (seit %s)",
				formatDur(gap), pausedAt.Format("15:04"))}
		}
		return actionDoneMsg{toast: "▶ Worktime fortgesetzt"}
	}
}

// yankDayMarkdownCmd writes a Markdown summary of the given date's sessions
// into the tmux paste buffer. Falls back to pbcopy on macOS, xclip on Linux.
// Best-effort: any error is folded into actionDoneMsg.
func yankDayMarkdownCmd(d time.Time) tea.Cmd {
	return func() tea.Msg {
		hist, err := wt.LoadHistory()
		if err != nil {
			return actionDoneMsg{err: err}
		}
		var rec wt.DayRecord
		key := d.Format("2006-01-02")
		for _, r := range hist {
			if r.Date.Format("2006-01-02") == key {
				rec = r
				break
			}
		}
		var b strings.Builder
		b.WriteString(fmt.Sprintf("**%s** — %s / %s\n",
			d.Format("2006-01-02 (Mon)"),
			formatDur(rec.Total), formatDur(rec.Target)))
		for _, s := range rec.Sessions {
			tagBit := ""
			if s.Tag != "" {
				tagBit = " [" + s.Tag + "]"
			}
			noteBit := ""
			if s.Note != "" {
				noteBit = " — " + s.Note
			}
			b.WriteString(fmt.Sprintf("- %s–%s (%s)%s%s\n",
				s.Start.Format("15:04"), s.Stop.Format("15:04"),
				formatDur(s.Elapsed), tagBit, noteBit))
		}
		if err := copyToClipboard(b.String()); err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{toast: "✓ Tag in Clipboard"}
	}
}

// yankBriefMarkdownCmd writes a brief-style Markdown summary of the active
// filter range to the clipboard.
func yankBriefMarkdownCmd(filter string, ref time.Time) tea.Cmd {
	return func() tea.Msg {
		// Resolve range: use the ISO week containing ref by default; otherwise
		// step the filter into a Range.
		var b strings.Builder
		scope := wt.ReportWeek
		anchor := ref
		switch {
		case strings.HasPrefix(strings.ToUpper(strings.TrimSpace(filter)), "KW"):
			var w int
			if _, err := fmt.Sscanf(strings.ToUpper(filter), "KW%d", &w); err == nil {
				anchor = isoMondayOfISOWeek(ref.Year(), w, ref.Location())
			}
		case len(filter) == 7 && filter[4] == '-':
			if t, err := time.ParseInLocation("2006-01", filter, ref.Location()); err == nil {
				scope = wt.ReportMonth
				anchor = t
			}
		case len(filter) == 4:
			if t, err := time.Parse("2006", filter); err == nil {
				scope = wt.ReportMonth
				anchor = t // year-wide brief still emitted as monthly heading
			}
		}
		if err := wt.WriteMarkdownBrief(&b, anchor, scope); err != nil {
			return actionDoneMsg{err: err}
		}
		if err := copyToClipboard(b.String()); err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{toast: "✓ Range in Clipboard"}
	}
}

// copyToClipboard writes s into the system clipboard via the most likely
// available command. Tries (in order): tmux load-buffer (if inside tmux),
// pbcopy (macOS), xclip (Linux). Returns the first error if none worked.
func copyToClipboard(s string) error {
	candidates := [][]string{}
	if os.Getenv("TMUX") != "" {
		candidates = append(candidates, []string{"tmux", "load-buffer", "-"})
	}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates, []string{"pbcopy"})
	}
	candidates = append(candidates,
		[]string{"xclip", "-selection", "clipboard"},
		[]string{"wl-copy"},
	)
	var lastErr error
	for _, c := range candidates {
		cmd := exec.Command(c[0], c[1:]...)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			lastErr = err
			continue
		}
		if err := cmd.Start(); err != nil {
			lastErr = err
			continue
		}
		_, _ = stdin.Write([]byte(s))
		_ = stdin.Close()
		if err := cmd.Wait(); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("kein Clipboard-Tool gefunden (tmux/pbcopy/xclip/wl-copy)")
}

func stopAtCmd(ts time.Time) tea.Cmd {
	return func() tea.Msg {
		s, err := wt.StopAt(ts)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{toast: fmt.Sprintf("■ Gestoppt — Session %s (%s)",
			formatDur(s.Elapsed), ts.Format("15:04"))}
	}
}

func correctCmd(ts time.Time) tea.Cmd {
	return func() tea.Msg {
		if err := wt.CorrectStart(ts); err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{toast: "✓ Startzeit korrigiert auf " + ts.Format("15:04")}
	}
}

func deleteCmd(date time.Time, idx int) tea.Cmd {
	return func() tea.Msg {
		if err := wt.DeleteSession(date, idx); err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{toast: fmt.Sprintf("✓ Session %d gelöscht  ·  u Undo", idx+1)}
	}
}

func undoDeleteCmd(date time.Time, s wt.Session) tea.Cmd {
	return func() tea.Msg {
		if err := wt.AddManual(date, s.Start, s.Stop); err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{toast: "✓ Löschung rückgängig"}
	}
}

func editCmd(date time.Time, idx int, startStr, stopStr string) tea.Cmd {
	return editFullCmd(date, idx, startStr, stopStr, "", "")
}

// editFullCmd updates start, stop, tag, and note in one user-facing
// operation. Tag and note are passed through unchanged when they match the
// existing values (avoid spurious rewrites). Empty tag/note clears.
//
// We keep the signature wide rather than introducing a struct because the
// invocation site is the only caller and reading "args here, output one
// command" is clearer than passing an EditPayload.
func editFullCmd(date time.Time, idx int, startStr, stopStr, tag, note string) tea.Cmd {
	return func() tea.Msg {
		startD, err := wt.ParseHM(startStr)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		base := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.Local)
		startTime := base.Add(startD)
		// Stop accepts +1h30m as a duration shorthand; ParseStop falls back
		// to ParseStartArg for "HH:MM" and "-Nm" forms.
		stopTime, err := wt.ParseStop(stopStr, startTime)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		// If the parsed stop is in HH:MM form on a different anchor day, it
		// will already be in today's date (ParseStartArg uses time.Now);
		// rebase to the edit's actual date.
		if stopStr != "" && stopStr[0] != '+' {
			stopHM, err := wt.ParseHM(stopStr)
			if err == nil {
				stopTime = base.Add(stopHM)
			}
		}
		if err := wt.EditSession(date, idx, startTime, stopTime); err != nil {
			return actionDoneMsg{err: err}
		}
		if err := wt.SetTag(date, idx, tag); err != nil {
			return actionDoneMsg{err: err}
		}
		if err := wt.SetNote(date, idx, note); err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{toast: fmt.Sprintf("✓ Session %d aktualisiert", idx+1)}
	}
}

func addManualCmd(dateStr, startStr, stopStr string) tea.Cmd {
	return func() tea.Msg {
		date, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		startD, err := wt.ParseHM(startStr)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		base := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.Local)
		startTime := base.Add(startD)

		// Stop accepts "+1h30m" as a duration shorthand: stop = start + dur.
		// "" means "now" (only valid when the date is today).
		var stopTime time.Time
		switch {
		case stopStr == "":
			stopTime = time.Now()
		case stopStr[0] == '+':
			stopTime, err = wt.ParseStop(stopStr, startTime)
			if err != nil {
				return actionDoneMsg{err: err}
			}
		default:
			stopD, err := wt.ParseHM(stopStr)
			if err != nil {
				return actionDoneMsg{err: err}
			}
			stopTime = base.Add(stopD)
		}
		if err := wt.AddManual(date, startTime, stopTime); err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{toast: fmt.Sprintf("✓ Eintrag %s → %s erfasst",
			startTime.Format("15:04"), stopTime.Format("15:04"))}
	}
}

// tickCmd returns the next tick. The interval depends on whether a session
// is currently active and how long it has been running:
//
//   - idle / first 60s of a running session → 1s tick (sub-second display)
//   - longer-running session → 10s tick (minute-level display, less flicker)
//
// The tick interval is recomputed in Update on every tick, so the cadence
// adapts as the day progresses.
func tickCmd() tea.Cmd {
	return tickCmdEvery(time.Second)
}

func tickCmdEvery(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func loadDayOffsCmd(year int) tea.Cmd {
	return func() tea.Msg {
		from := time.Date(year, time.January, 1, 0, 0, 0, 0, time.Local)
		to := time.Date(year, time.December, 31, 0, 0, 0, 0, time.Local)
		return dayOffsLoadedMsg{
			dayoffs: wt.ListDayOffs(from, to),
			year:    year,
		}
	}
}

func loadRecentTagsCmd() tea.Cmd {
	return func() tea.Msg {
		tags, err := wt.RecentTags(8)
		if err != nil {
			return recentTagsLoadedMsg{err: err}
		}
		top, err := wt.TopUsageTags(8)
		if err != nil {
			return recentTagsLoadedMsg{tags: tags, err: err}
		}
		return recentTagsLoadedMsg{tags: tags, topTags: top}
	}
}

func loadTemplatesCmd() tea.Cmd {
	return func() tea.Msg {
		t, err := wt.RecentSessionTemplates(5)
		return templatesLoadedMsg{templates: t, err: err}
	}
}

func addDayOffCmd(dateExpr string, kind wt.Kind, label string, currentYear int) tea.Cmd {
	return func() tea.Msg {
		// Range support: YYYY-MM-DD..YYYY-MM-DD
		from, to, isRange, err := parseDateOrRangeExpr(dateExpr)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		if isRange {
			n, err := wt.AddDayOffRange(from, to, kind, label)
			if err != nil {
				return actionDoneMsg{err: err}
			}
			_ = currentYear
			return actionDoneMsg{toast: fmt.Sprintf("✓ %d Tag(e) als %s eingetragen",
				n, kind.LabelDe())}
		}
		if err := wt.AddDayOff(from, kind, label); err != nil {
			return actionDoneMsg{err: err}
		}
		_ = currentYear
		return actionDoneMsg{toast: fmt.Sprintf("✓ %s eingetragen für %s",
			kind.LabelDe(), from.Format("2006-01-02"))}
	}
}

func removeDayOffCmd(date time.Time, _ int) tea.Cmd {
	return func() tea.Msg {
		if err := wt.RemoveDayOff(date); err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{toast: "✓ Eintrag entfernt für " + date.Format("2006-01-02")}
	}
}

// syncGermanHolidaysCmd populates the day-offs file with the gesetzliche
// Feiertage for the given year and Bundesland. Idempotent.
func syncGermanHolidaysCmd(year int, land string) tea.Cmd {
	return func() tea.Msg {
		added, _, err := wt.SyncGermanHolidays(year, land)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{toast: fmt.Sprintf("✓ %d Feiertag(e) für %s/%d", added, land, year)}
	}
}

// parseDateOrRangeExpr parses YYYY-MM-DD or YYYY-MM-DD..YYYY-MM-DD; mirrors
// the CLI helper but is duplicated here to keep the screen package free of
// cmd/flow imports.
func parseDateOrRangeExpr(s string) (time.Time, time.Time, bool, error) {
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '.' && s[i+1] == '.' {
			fromStr := s[:i]
			toStr := s[i+2:]
			f, e1 := time.ParseInLocation("2006-01-02", fromStr, time.Local)
			t, e2 := time.ParseInLocation("2006-01-02", toStr, time.Local)
			if e1 != nil {
				return time.Time{}, time.Time{}, false, fmt.Errorf("from: %v", e1)
			}
			if e2 != nil {
				return time.Time{}, time.Time{}, false, fmt.Errorf("to: %v", e2)
			}
			return f, t, true, nil
		}
	}
	d, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, false, fmt.Errorf("ungültiges datum: %s", s)
	}
	return d, d, false, nil
}

// — formatting —

// recentWorkdays returns up to n DayRecords that are workdays (no weekend,
// no day-off), newest first, excluding today.
func (m Model) recentWorkdays(n int) []wt.DayRecord {
	out := make([]wt.DayRecord, 0, n)
	today := startOfDay(m.now)
	for _, r := range m.history {
		if !sameDay(r.Date, today) && wt.IsWorkday(r.Date) {
			out = append(out, r)
			if len(out) >= n {
				break
			}
		}
	}
	return out
}

// recentWorkdayAvg computes the average duration over the last ~7 workdays
// (excluding today). Returns 0 when there's not enough data — the header
// caller skips the "vs Schnitt" suffix in that case.
func (m Model) recentWorkdayAvg() time.Duration {
	recs := m.recentWorkdays(7)
	if len(recs) == 0 {
		return 0
	}
	var sum time.Duration
	for _, r := range recs {
		sum += r.Total
	}
	return sum / time.Duration(len(recs))
}

// renderTodaySparkline renders the last 7 workdays as ▁▂▃▄▅▆▇ glyphs whose
// height is proportional to that day's total / target. Returns "" when there
// are fewer than 2 workdays of history.
func (m Model) renderTodaySparkline() string {
	recs := m.recentWorkdays(7)
	if len(recs) < 2 {
		return ""
	}
	// Reverse to oldest-first so the line reads left-to-right "ago → now".
	rev := make([]wt.DayRecord, len(recs))
	for i, r := range recs {
		rev[len(recs)-1-i] = r
	}
	glyphs := []rune("▁▂▃▄▅▆▇█")
	var b strings.Builder
	b.WriteString("  ")
	for _, r := range rev {
		bucket := 0
		if r.Target > 0 {
			pct := float64(r.Total) / float64(r.Target)
			bucket = int(pct * float64(len(glyphs)-1))
			if bucket < 0 {
				bucket = 0
			}
			if bucket >= len(glyphs) {
				bucket = len(glyphs) - 1
			}
		}
		c := m.theme.Dim
		if r.Target > 0 && r.Total >= r.Target {
			c = m.theme.Green
		}
		b.WriteString(lipgloss.NewStyle().Foreground(c).Render(string(glyphs[bucket])))
	}
	b.WriteString(stDim(m.theme, "  letzte Werktage"))
	return b.String()
}

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

// formatSignedDur renders +/- duration like "+1h 12m" / "-45m".
func formatSignedDur(d time.Duration) string {
	sign := "+"
	if d < 0 {
		sign = "-"
		d = -d
	}
	return fmt.Sprintf("%s%dh %02dm", sign, int(d.Hours()), int(d.Minutes())%60)
}

var (
	weekdayLong  = [7]string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"}
	weekdayShort = [7]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}
	monthNames   = [13]string{"", "Jan", "Feb", "Mär", "Apr", "Mai", "Jun", "Jul", "Aug", "Sep", "Okt", "Nov", "Dez"}
)

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

// totalThresholdColor picks the today-total foreground based on running
// state and target progress. Red is reserved for "really a lot" so a normal
// hour of overtime doesn't look like an alarm.
func totalThresholdColor(p tk.Palette, total, target time.Duration, running bool) lipgloss.TerminalColor {
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

// todayStatusBadge returns the glyph, label, and color for the today status badge.
// The "running + achieved" label is intentionally compact ("läuft ✓") so the
// headline line stays inside narrow tmux panes — the long form
// "läuft (Ziel erreicht)" pushed the header past 40 cols.
func todayStatusBadge(p tk.Palette, running, achieved bool) (string, string, lipgloss.TerminalColor) {
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

func stErr(p tk.Palette, s string) string {
	return lipgloss.NewStyle().Foreground(p.Red).Render("  " + s)
}

func stFooter(p tk.Palette, s string) string {
	return lipgloss.NewStyle().Foreground(p.Dim).Padding(0, 1).Render(s)
}

// boxTitle builds a titlebox title from `parts` (joined by " · "), but drops
// trailing parts progressively until the result fits in the available
// horizontal budget. The titlebox draws `╭─ <title> ─╮` so each title char
// past `maxWidth - 5` would push the right corner past the terminal edge,
// at which point the terminal hard-wraps and the box appears doubled.
//
// As a last resort the first (most-important) segment is hard-truncated.
func boxTitle(parts []string, maxWidth int) string {
	const overhead = 5
	if len(parts) == 0 {
		return ""
	}
	if maxWidth <= 0 {
		return strings.Join(parts, " · ")
	}
	budget := maxWidth - overhead
	if budget < 4 {
		budget = 4
	}
	full := strings.Join(parts, " · ")
	if lipgloss.Width(full) <= budget {
		return full
	}
	for i := len(parts) - 1; i > 0; i-- {
		cand := strings.Join(parts[:i], " · ")
		if lipgloss.Width(cand) <= budget {
			return cand
		}
	}
	return lipgloss.NewStyle().MaxWidth(budget).Render(parts[0])
}

// joinWrapped joins `parts` with `sep`, breaking onto the next line whenever
// the next item would push the current line past `maxWidth` (visible columns,
// not bytes — matters for ANSI/styled strings). The first line starts with
// `prefix`; continuation lines indent with `cont` so the eye visually groups
// the wrapped chunk back together.
//
// `maxWidth <= 0` disables wrapping (returns the plain joined string), which
// keeps callers safe before the model has received a WindowSizeMsg.
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

// wrapFooter renders a footer string, wrapping at the terminal width. The
// footer is split on the "  ·  " primary separator; for groups that are
// still wider than the budget after that, we fall back to splitting on the
// "  " (double-space) inner separator that the chip-builders use. Lines
// passing through `stFooter` get its single-char horizontal padding so the
// effective budget is `maxWidth - 2`.
func wrapFooter(p tk.Palette, s string, maxWidth int) string {
	const groupSep = "  ·  "
	const chipSep = "  "
	budget := maxWidth - 2
	if maxWidth <= 0 || lipgloss.Width(s) <= budget {
		return stFooter(p, s)
	}
	groups := strings.Split(s, groupSep)
	// Flatten oversize groups by re-splitting them on the chip separator;
	// keep groups that still fit as a single token. This way we get tight
	// wrapping without losing the chip texture entirely.
	var chips []string
	for _, g := range groups {
		if lipgloss.Width(g) <= budget {
			chips = append(chips, g)
			continue
		}
		chips = append(chips, strings.Split(g, chipSep)...)
	}
	wrapped := joinWrapped(chips, chipSep, "", "  ", budget)
	return stFooter(p, wrapped)
}

// renderToastRow returns the toast line prefixed with a leading newline so
// it slots neatly between the titlebox and the footer. Empty string when
// no toast is set — the surrounding render code stays unchanged.
func (m Model) renderToastRow() string {
	if m.toast == "" {
		return ""
	}
	col := m.theme.Green
	// Heuristic: if the toast starts with "Neuer Best-Streak" or similar
	// celebration glyph, use the accent colour to make it pop.
	if strings.HasPrefix(m.toast, "✦") {
		col = m.theme.Accent
	}
	return "\n" + lipgloss.NewStyle().Foreground(col).Padding(0, 1).Render(m.toast)
}

// confirmButton renders a chip-style label for a confirm-dialog option. The
// `isDefault` flag inverts colours so the safe default (typically Cancel) is
// the visually obvious target — Enter is wired to that branch.
func confirmButton(p tk.Palette, label string, isDefault bool) string {
	if isDefault {
		return lipgloss.NewStyle().Foreground(p.Bg).Background(p.Accent).Bold(true).
			Render(" " + label + " ")
	}
	return lipgloss.NewStyle().Foreground(p.Fg).
		Render(" " + label + " ")
}

// pomodoroMinutes returns the configured cycle length in minutes for the
// pomodoro strip. Configurable via WORKTIME_POMODORO_MIN, default 25.
func pomodoroMinutes() int {
	if v := os.Getenv("WORKTIME_POMODORO_MIN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 25
}

// renderPomodoroStrip returns "●●●◐○○  3/6 Pomodori" describing the user's
// progress through cycles of the running session. Returns "" when nothing
// is currently running — Pomodoros only make sense for active focus time.
//
// Layout choices:
//   - ● = completed cycle (green)
//   - ◐ = current cycle (cyan/yellow/red by progress within cycle)
//   - ○ = pending cycle (dim)
//   - When the current cycle is overdue (>=99%), a "Zeit für Pause" tail
//     hint nudges without forcing an action.
func (m Model) renderPomodoroStrip(now time.Time) string {
	if !m.day.IsRunning() || m.day.Active == nil {
		return ""
	}
	cycleLen := time.Duration(pomodoroMinutes()) * time.Minute
	if cycleLen <= 0 {
		return ""
	}
	elapsed := now.Sub(*m.day.Active)
	if elapsed <= 0 {
		return ""
	}
	completed := int(elapsed / cycleLen)
	progress := float64(elapsed%cycleLen) / float64(cycleLen)

	// Cap the visible cycles: enough to show the trajectory without flooding
	// the line. Use the day's target as the soft upper bound.
	target := m.day.Target
	totalCycles := completed + 1
	if target > 0 {
		needed := int(target / cycleLen)
		if needed > totalCycles {
			totalCycles = needed
		}
	}
	if totalCycles > 12 {
		totalCycles = 12
	}

	var b strings.Builder
	for i := 0; i < totalCycles; i++ {
		switch {
		case i < completed:
			b.WriteString(lipgloss.NewStyle().Foreground(m.theme.Green).Render("●"))
		case i == completed:
			color := m.theme.Cyan
			glyph := "◐"
			switch {
			case progress >= 0.99:
				color = m.theme.Red
				glyph = "◉"
			case progress >= 0.5:
				color = m.theme.Yellow
			}
			b.WriteString(lipgloss.NewStyle().Foreground(color).Render(glyph))
		default:
			b.WriteString(lipgloss.NewStyle().Foreground(m.theme.Border).Render("○"))
		}
	}
	tail := fmt.Sprintf("  %d/%d Pomodori", completed, totalCycles)
	if progress >= 0.99 {
		tail += "  ·  Zeit für Pause"
	}
	return b.String() + stDim(m.theme, tail)
}

// typicalStopTime computes the median end-of-day across the last 14 workdays
// and projects it onto today's date. Returns ok=false when there are fewer
// than 3 workdays of history (median is meaningless on tiny samples).
func (m Model) typicalStopTime(now time.Time) (time.Time, bool) {
	stops := make([]int, 0, 14)
	for _, rec := range m.recentWorkdays(14) {
		if len(rec.Sessions) == 0 {
			continue
		}
		last := rec.Sessions[0].Stop
		for _, s := range rec.Sessions {
			if s.Stop.After(last) {
				last = s.Stop
			}
		}
		stops = append(stops, last.Hour()*60+last.Minute())
	}
	if len(stops) < 3 {
		return time.Time{}, false
	}
	sort.Ints(stops)
	median := stops[len(stops)/2]
	h := median / 60
	mi := median % 60
	return time.Date(now.Year(), now.Month(), now.Day(), h, mi, 0, 0, now.Location()), true
}

// forgetfulnessThreshold returns the wall-clock time after which a workday
// with zero entries should trigger the "vergessen zu starten?" prompt. Based
// on the median earliest-start over the user's last 14 workdays plus a 90 min
// grace window. Returns a fixed 10:00 anchor if history is too sparse.
func (m Model) forgetfulnessThreshold(now time.Time) time.Time {
	startOf := func(d time.Time, h, mi int) time.Time {
		return time.Date(d.Year(), d.Month(), d.Day(), h, mi, 0, 0, d.Location())
	}
	const fallbackHour = 10
	starts := make([]int, 0, 14)
	for _, rec := range m.recentWorkdays(14) {
		if len(rec.Sessions) == 0 {
			continue
		}
		earliest := rec.Sessions[0].Start
		for _, s := range rec.Sessions {
			if s.Start.Before(earliest) {
				earliest = s.Start
			}
		}
		starts = append(starts, earliest.Hour()*60+earliest.Minute())
	}
	if len(starts) < 3 {
		return startOf(now, fallbackHour, 0)
	}
	sort.Ints(starts)
	median := starts[len(starts)/2]
	median += 90 // grace
	h := median / 60
	mi := median % 60
	if h >= 24 {
		h, mi = 23, 59
	}
	return startOf(now, h, mi)
}

// pauseStats walks the sessions of `d` plus the gap to the active session
// (or the time since the last stop, when paused) and returns the cumulative
// pause time and the longest single pause. Returns (0, 0) when there is at
// most one session and no gaps to measure.
func pauseStats(d wt.Day, now time.Time) (time.Duration, time.Duration) {
	var total, longest time.Duration
	var prevStop time.Time
	for _, s := range d.Sessions {
		if !prevStop.IsZero() {
			if gap := s.Start.Sub(prevStop); gap > 0 {
				total += gap
				if gap > longest {
					longest = gap
				}
			}
		}
		prevStop = s.Stop
	}
	switch {
	case d.Active != nil && !prevStop.IsZero():
		if gap := d.Active.Sub(prevStop); gap > 0 {
			total += gap
			if gap > longest {
				longest = gap
			}
		}
	case d.IsPaused() && d.PausedAt != nil:
		// The current pause is open — count it from PausedAt to now.
		if gap := now.Sub(*d.PausedAt); gap > 0 {
			total += gap
			if gap > longest {
				longest = gap
			}
		}
	}
	return total, longest
}

// sessionMiniBar renders a small horizontal bar whose fill ratio equals
// dur/maxDur. Used in session lists so two sessions of vastly different sizes
// look different, not just textually but visually. Always shows ≥1 cell as
// long as dur > 0 — a 1-minute session shouldn't render as totally empty.
func sessionMiniBar(p tk.Palette, dur, maxDur time.Duration, cells int) string {
	if cells <= 0 {
		return ""
	}
	if maxDur <= 0 || dur <= 0 {
		return lipgloss.NewStyle().Foreground(p.Border).Render(strings.Repeat("·", cells))
	}
	filled := int(time.Duration(cells) * dur / maxDur)
	if filled < 1 {
		filled = 1
	}
	if filled > cells {
		filled = cells
	}
	f := lipgloss.NewStyle().Foreground(p.Accent).Render(strings.Repeat("▰", filled))
	e := lipgloss.NewStyle().Foreground(p.Border).Render(strings.Repeat("▱", cells-filled))
	return f + e
}
