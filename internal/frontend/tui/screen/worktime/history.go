package worktime

// History — model + Update routing + state accessors + key dispatch +
// View root. Mode-spezifische Render-Files: history_list.go (Default-
// Liste + Header), history_heatmap.go, history_tagclock.go,
// history_month.go. Filter-Surface: history_filter.go. Drill-Surface:
// history_drill.go (View + Keys) und history_edit.go (Edit/Add/Delete-
// Forms mit Form-State).

import (
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/confirm"
	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown_overlay"
	"github.com/serverkraken/flow/internal/frontend/tui/components/toast"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// historyStyles caches the palette-dependent lipgloss styles whose
// foreground/background don't change per render. Built once at
// newHistory(); the render hot paths (renderHeatmapRows ~182 cells,
// renderMonthGridRows ~35 cells) were the canonical wocheStyles-
// pattern's natural follow-up — pre-builds cut allocations without
// changing call-site signatures.
type historyStyles struct {
	dayLabelFg       lipgloss.Style // Heatmap weekday label — Fg, Width(3)
	dayLabelMuted    lipgloss.Style // Month weekday label — FgMuted, pad-formatted
	headerWeekNum    lipgloss.Style // Heatmap week-number column — FgMuted
	headerYearChange lipgloss.Style // Heatmap year-boundary column — Sem.Highlight
	cursorCell       lipgloss.Style // Heat/Month cursor cell — Bg-on-Accent, Bold
	balPositive      lipgloss.Style // Month aggregate Saldo +
	balZero          lipgloss.Style // Month aggregate Saldo 0
	balNegative      lipgloss.Style // Month aggregate Saldo -
}

func newHistoryStyles(p theme.Palette) historyStyles {
	sem := p.Sem()
	return historyStyles{
		dayLabelFg:       lipgloss.NewStyle().Foreground(p.Fg).Width(3),
		dayLabelMuted:    lipgloss.NewStyle().Foreground(p.FgMuted),
		headerWeekNum:    lipgloss.NewStyle().Foreground(p.FgMuted),
		headerYearChange: lipgloss.NewStyle().Foreground(sem.Highlight),
		cursorCell:       lipgloss.NewStyle().Foreground(p.Bg).Background(sem.Accent).Bold(true),
		balPositive:      lipgloss.NewStyle().Foreground(sem.Success),
		balZero:          lipgloss.NewStyle().Foreground(p.FgMuted),
		balNegative:      lipgloss.NewStyle().Foreground(sem.Warning),
	}
}

// — messages —

type historyLoadedMsg struct {
	records    []domain.DayRecord
	monthStats domain.Stats
	topTags    []string
	// attachedCounts mappt YYYY-MM-DD → Anzahl angehaengter Notes; leer
	// wenn LinkReader nicht verdrahtet ist oder das Lesen scheitert
	// (siehe loadCmd: Note-Load-Fehler degradieren still, weil die
	// Sessions-Liste die primaere Surface bleibt).
	attachedCounts map[string]int
	err            error
}

type historyDrillLoadedMsg struct {
	date     time.Time
	sessions []domain.Session
	attached []string // Kompendium note IDs linked to date, in insertion order
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

// label — deutsche User-facing-Bezeichnung des Modes (Skill §German UI).
// Vorher EN-internalia (heatmap/tagclock/month/list) im Footer-Hint sichtbar.
func (m historyMode) label() string {
	switch m {
	case historyModeHeatmap:
		return "Heatmap"
	case historyModeTagClock:
		return "Tag-Clock"
	case historyModeMonth:
		return "Monat"
	}
	return "Liste"
}

// next gibt den nächsten Mode in der v-Cycle-Reihenfolge zurück
// (Liste → Heatmap → Tag-Clock → Monat → Liste). Für den Footer-Hint
// "v → <next>", damit der Nutzer sieht, welche Ansicht ein Druck auf
// `v` aufruft, statt den aktuellen Mode zu raten.
func (m historyMode) next() historyMode {
	switch m {
	case historyModeList:
		return historyModeHeatmap
	case historyModeHeatmap:
		return historyModeTagClock
	case historyModeTagClock:
		return historyModeMonth
	}
	return historyModeList
}

type historyDialog int

const (
	historyDialogNone historyDialog = iota
	historyDialogFilter
	historyDialogDrill
	historyDialogDrillEdit       // edit start/stop/tag/note of selected session
	historyDialogDrillAdd        // add a new manual session to the drill day
	historyDialogDrillDelete     // confirm-delete selected session
	historyDialogDrillNoteAttach // Kompendium-note attach to drillDate
	historyDialogDrillNoteView   // inline markdown_overlay viewer for first attached note
)

// historyActionDoneMsg carries the result of a drill mutation (edit /
// add / delete). The history sub-model consumes it to display a toast
// + reload the drill so the new state surfaces immediately.
type historyActionDoneMsg struct {
	err   error
	toast string
	date  time.Time
}

// history is the History tab sub-model. It owns four render sub-modes
// (list / heatmap / tag-clock / month) plus a filter dialog and a
// day-detail drill that supports session edit / add / delete on
// past-day rows. Mutations route through the same SessionWriter the
// Heute view uses — locking, overlap checks and split-at-midnight
// invariants stay enforced in one place.
type history struct {
	pal  theme.Palette
	deps Deps

	width  int
	height int

	records    []domain.DayRecord
	monthStats domain.Stats
	topTags    []string
	loaded     bool
	err        error
	// attachedCounts spiegelt LinkReader.CountsByDate aus dem letzten
	// loadCmd-Run; List/Heatmap/Month-Renderer lesen daraus, um Tage
	// mit angehaengten Notes zu markieren („● 2", o.ae.). Map-Kontrakt
	// per ports/links.go: Schluessel YYYY-MM-DD, Wert > 0.
	attachedCounts map[string]int

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
	// drillAttached holds the Kompendium note IDs linked to drillDate.
	// Loaded together with drillSessions; surfaces in renderDrill as a
	// chip line and gates the `o`-key inline viewer.
	drillAttached []string
	// drillNoteView is the inline markdown_overlay for `o` on the drill.
	// Active when dialog == historyDialogDrillNoteView. nil when closed.
	drillNoteView *markdown_overlay.Model

	// drillEditIdx is the session row the active drill-edit / drill-
	// delete dialog targets. -1 in drill-add mode (no row reference).
	drillEditIdx int
	// drillForm drives the multi-input edit / add dialog. Same shape
	// as today.go's edit form: [start, stop, tag, note].
	drillForm    []textinput.Model
	drillFormCur int
	// drillConfirm drives the delete-confirm dialog (canonical
	// confirm.Model component).
	drillConfirm *confirm.Model
	// drillToast surfaces the result of the last mutation. Vorher als
	// roher String — der dismisste sich nie. toast.Model dismisst nach 2 s.
	drillToast *toast.Model

	// notePicker hosts the Kompendium-note attach dialog when triggered
	// from the drill view. Active when dialog == historyDialogDrillNoteAttach.
	// Construction is one-shot at newHistory(); state resets per Open call.
	notePicker noteAttachPicker

	// styles is a palette-bound cache for the heatmap / month render
	// hot path (canonical wocheStyles-Pattern).
	styles historyStyles
}

func newHistory(p theme.Palette, deps Deps) history {
	return history{
		pal:        p,
		deps:       deps,
		notePicker: newNoteAttachPicker(deps, p),
		styles:     newHistoryStyles(p),
	}
}

// — capability interfaces —

func (h history) FilterActive() bool { return h.dialog != historyDialogNone }

// FullScreen reports whether the worktime root should skip its titlebox
// wrap. True nur fuer den Drill-Note-Viewer (`o` im Drill) — gleicher
// Grund wie heute.FullScreen: markdown_overlay bringt eigenes Chrome
// mit, ein zweiter Wrapper produziert Doppelborder + Clipping.
func (h history) FullScreen() bool { return h.dialog == historyDialogDrillNoteView }

// TextInputActive reports whether History's current dialog has a
// textinput focused — the filter expression, the drill-edit form, the
// drill-add form, or the drill note-attach picker. The drill-delete
// confirm and the bare drill view are intentionally NOT text-input —
// q from there should exit.
func (h history) TextInputActive() bool {
	switch h.dialog {
	case historyDialogFilter, historyDialogDrillEdit, historyDialogDrillAdd, historyDialogDrillNoteAttach:
		return true
	}
	return false
}

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
		h.height = msg.Height
		// Forward resize to the inline note viewer if open so the
		// markdown re-flows with the new pane dimensions.
		if h.dialog == historyDialogDrillNoteView && h.drillNoteView != nil {
			upd := h.drillNoteView.SetSize(msg.Width, msg.Height)
			h.drillNoteView = &upd
		}
		return h, nil

	case historyLoadedMsg:
		h.loaded = true
		h.err = msg.err
		if msg.err == nil {
			h.records = msg.records
			h.monthStats = msg.monthStats
			h.topTags = msg.topTags
			h.attachedCounts = msg.attachedCounts
			h = h.clampCursors()
		}
		return h, nil

	case historyDrillLoadedMsg:
		// Discard late drill loads when the dialog is closed or the
		// user already opened a different day's drill — without this
		// guard the next manual open briefly flashes the stale day's
		// rows as the in-flight load lands.
		//
		// Drill-edit / drill-add / drill-delete / NoteAttach / NoteView
		// dialogs sit ON TOP of the drill (we only enter them from the
		// drill view), so the load must accept those modes too —
		// otherwise a dialog open during an async reload would discard
		// the fresh sessions.
		if !h.drillModeActive() || !sameDay(h.drillDate, msg.date) {
			return h, nil
		}
		h.drillErr = msg.err
		h.drillSessions = msg.sessions
		h.drillAttached = msg.attached
		if h.drillCur >= len(h.drillSessions) {
			h.drillCur = 0
		}
		return h, nil

	case markdown_overlay.ExitMsg:
		// Close-Key (q/esc/b) from the inline note viewer. Drill stays
		// open with the chip line below the sessions list, so the user
		// can press `o` again immediately. Other drill-dialog modes
		// don't consume ExitMsg — markdown_overlay only lives in
		// drillNoteView.
		if h.dialog == historyDialogDrillNoteView {
			h.dialog = historyDialogDrill
			h.drillNoteView = nil
			return h, nil
		}
		return h, nil

	case historyActionDoneMsg:
		if msg.err != nil {
			h.errMsg = msg.err.Error()
			return h, nil
		}
		t := toast.NewDefault(msg.toast, h.pal)
		h.drillToast = &t
		// Mutations change day totals → reload the outer history list
		// so the bar / pct of this day stay in sync. The drill load
		// reloads the session list visible in the dialog.
		var cmds []tea.Cmd
		if !msg.date.IsZero() {
			cmds = append(cmds, h.drillLoadCmd(startOfDay(msg.date)))
		}
		cmds = append(cmds, h.loadCmd(), t.Init())
		return h, tea.Batch(cmds...)

	case toast.DismissedMsg:
		h.drillToast = nil
		return h, nil

	case confirm.ResultMsg:
		return h.handleDrillConfirmResult(msg)

	case dayRefreshMsg:
		return h, h.loadCmd()

	case tea.KeyPressMsg:
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
		// Note-Counts: Fehler degradieren still — Sessions/Targets bleiben
		// die primaere Surface, ein kaputter LinkStore soll die History
		// nicht blanken. Mirror von today.loadCmd / drillLoadCmd.
		var counts map[string]int
		if deps.LinkReader != nil {
			counts, _ = deps.LinkReader.CountsByDate()
		}
		return historyLoadedMsg{
			records:        records,
			monthStats:     monthStats,
			topTags:        topTags,
			attachedCounts: counts,
		}
	}
}

func (h history) drillLoadCmd(date time.Time) tea.Cmd {
	reader := h.deps.Reader
	linkReader := h.deps.LinkReader
	from := startOfDay(date)
	return func() tea.Msg {
		sessions, err := reader.Range(domain.Range{From: from, To: from.AddDate(0, 0, 1)})
		if err != nil {
			return historyDrillLoadedMsg{date: from, err: err}
		}
		// Note-load errors stay silent — sessions are the primary
		// surface. A broken LinkReader shouldn't blank the headline;
		// the chip line just doesn't render. Mirrors today.loadCmd
		// (heute follows the same pattern).
		var attached []string
		if linkReader != nil {
			attached, _ = linkReader.ListByDate(from)
		}
		return historyDrillLoadedMsg{date: from, sessions: sessions, attached: attached}
	}
}

func (h history) clampCursors() history {
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

	// Heatmap: a dayRefreshMsg can shift records (oldest day rolls
	// off, today rolls in), pushing heatCol past the new week count
	// and producing an invisible cursor on heatmapDateAt-miss. Snap
	// to today's cell when the current cell falls out of range.
	weeks := h.heatmapWeeks()
	if weeks > 0 {
		if h.heatCol < 0 || h.heatCol >= weeks || h.heatRow < 0 || h.heatRow > 6 {
			h.heatCol, h.heatRow = h.heatmapTodayCell()
		}
	}

	// Tag-clock: the grid is fixed 24×7 so the cursor can only be out
	// of range by uninitialised state, but defend in depth so the
	// renderer never indexes out of bounds.
	if h.tagClockCol < 0 || h.tagClockCol > 23 {
		h.tagClockCol = 0
	}
	if h.tagClockRow < 0 || h.tagClockRow > 6 {
		h.tagClockRow = 0
	}

	// Month grid: clamp the day cursor to the current month length.
	if !h.monthRef.IsZero() {
		h.monthCur = monthClampDay(h.monthRef, h.monthCur)
	}
	return h
}

// — keymap dispatch —

func (h history) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if h.dialog == historyDialogFilter {
		return h.handleFilterKey(msg)
	}
	if h.dialog == historyDialogDrillEdit || h.dialog == historyDialogDrillAdd {
		return h.handleDrillFormKey(msg)
	}
	if h.dialog == historyDialogDrillDelete {
		return h.handleDrillDeleteKey(msg)
	}
	if h.dialog == historyDialogDrillNoteAttach {
		return h.handleDrillNoteAttachKey(msg)
	}
	if h.dialog == historyDialogDrillNoteView {
		return h.handleDrillNoteViewKey(msg)
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

func (h history) handleListKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

func (h history) handleHeatmapKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

func (h history) handleTagClockKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

func (h history) handleMonthKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

// — view root —

func (h history) View() tea.View { return tea.NewView(h.viewContent()) }

func (h history) viewContent() string {
	if h.width == 0 {
		return ""
	}
	if h.dialog == historyDialogFilter {
		return h.renderFilterDialog()
	}
	if h.drillModeActive() {
		// Edit / Add / Delete modes render on top of the drill body
		// (sessions list + summary stays visible above the dialog) so
		// the user sees what they're editing without losing context.
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

// drillModeActive reports whether any drill-rooted dialog is open.
// Edit / Add / Delete / NoteAttach / NoteView render on top of the drill
// list, so they all participate in the drill's load/render flow.
func (h history) drillModeActive() bool {
	switch h.dialog {
	case historyDialogDrill, historyDialogDrillEdit,
		historyDialogDrillAdd, historyDialogDrillDelete,
		historyDialogDrillNoteAttach, historyDialogDrillNoteView:
		return true
	}
	return false
}
