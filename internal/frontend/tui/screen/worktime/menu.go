// Worktime-Aktions-Menü — vollflächiges Modal, das beim Druck von `:`
// über dem aktiven Tab erscheint. Slice B (dieser File) liefert die
// Schale: Öffnen, filterbare Liste mit Sections, j/k-Navigation,
// Esc-Close (zweistufig: leert Filter, schließt). Aktionen liefern in
// Slice B nur einen TODO-Toast; Slice C/D/E ersetzen runAction durch
// die echten Sub-Flows (Output-Target-Picker, Range-Form, etc.).
//
// Architektur-Notiz: das Menü lebt am Worktime-Root, nicht in einem
// Sub-Tab. Damit landen Tab-spezifische Aktionen über die Predicate
// im selben Menü, der User braucht keinen Kontext-Wechsel.

package worktime

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
	"github.com/serverkraken/flow/internal/frontend/tui/components/toast"
)

// menuSubMode discriminates which inner state the menu is currently
// rendering and routing keys to. List is the default; Range is the
// 1-input form for Export/Stats; Target is the output-sink picker;
// Correct is the HH:MM form for the running session's start; Land
// is the Bundesland picker for SyncGermanHolidays.
type menuSubMode int

const (
	menuSubModeList menuSubMode = iota
	menuSubModeRange
	menuSubModeTarget
	menuSubModeCorrect
	menuSubModeLand
)

// menuActionDoneMsg carries the result of an action's async dispatch
// (Brief render → output target). Slice C uses it for Brief; Slice D
// adds Export/Stats which produce the same shape. Empty toast +
// non-nil err surfaces the error inside the modal; non-empty toast
// pops the green ✓ and resets the menu to its list view.
type menuActionDoneMsg struct {
	err   error
	toast string
}

// menuModel ist das Aktions-Menü-Sub-Modell. Es liegt am Worktime-Root
// neben den vier Tab-Sub-Modellen — `Active() == true` blockiert global
// Tab-Switching und routet alle Keys durchs Menü, bis Esc/Run die Modal
// schließen.
type menuModel struct {
	pal  theme.Palette
	deps Deps

	width  int
	height int

	open      bool
	activeTab tab

	subMode menuSubMode

	cursor   int
	query    string
	filtered []menuAction

	// pending is the action the user picked in list-mode; held while
	// the user steps through the range form (when applicable) and the
	// target sub-picker so dispatchPending knows what to render.
	pending menuAction
	rangeF  rangeForm
	// rangeExpr holds the expression captured from rangeF after a
	// successful submit. Empty when the action skips the range form
	// (Brief) or the user typed nothing (which means "all time").
	rangeExpr string
	target    targetPicker
	// correctF / landP carry the modal state for Slice E's two
	// non-output actions: HH:MM start-correction and Bundesland sync.
	// Both flows skip the target sub-picker — they're side-effects on
	// the worktime store, not artifacts the user wants to read back.
	correctF correctForm
	landP    landPicker

	toast  *toast.Model
	errMsg string
}

// newMenuModel constructs a closed menu. openMenu primes activeTab + the
// filtered action set when `:` is pressed at the root.
func newMenuModel(p theme.Palette, deps Deps) menuModel {
	return menuModel{pal: p, deps: deps}
}

// openMenu primes the model state for a fresh open. Must be re-called
// each time `:` opens the menu so predicates re-evaluate against the
// active tab and current worktime state.
func (m menuModel) openMenu(activeTab tab) menuModel {
	m.open = true
	m.activeTab = activeTab
	m.subMode = menuSubModeList
	m.cursor = 0
	m.query = ""
	m.filtered = computeMenuActions(activeTab, m.deps, "")
	m.pending = menuAction{}
	m.rangeF = rangeForm{}
	m.rangeExpr = ""
	m.target = targetPicker{}
	m.correctF = correctForm{}
	m.landP = landPicker{}
	m.errMsg = ""
	m.toast = nil
	return m
}

// Active reports whether the menu currently consumes input. The Worktime
// root checks this before tab-switching keys (1/2/3/4/Tab/b) so they
// don't fire while the menu owns the focus.
func (m menuModel) Active() bool { return m.open }

// TextInputActive reports whether the menu's current sub-mode has a
// textinput.Model focused — Range and Correct take HH:MM / range-
// expression input and 'q' must land in the field, not quit the
// program. List mode handles letters as filter through manual rune
// dispatch (no textinput), and Target / Land are pickers — q in
// either is unambiguously "quit".
func (m menuModel) TextInputActive() bool {
	if !m.open {
		return false
	}
	switch m.subMode {
	case menuSubModeRange, menuSubModeCorrect:
		return true
	}
	return false
}

// SetSize is called by the worktime root on every WindowSizeMsg so the
// modal renders against the same width/height the tab bodies use.
func (m menuModel) SetSize(w, h int) menuModel {
	m.width, m.height = w, h
	return m
}

// Update routes a tea.Msg into the menu when it's open. The worktime
// root only forwards messages here while Active() is true.
func (m menuModel) Update(msg tea.Msg) (menuModel, tea.Cmd) {
	switch msg := msg.(type) {
	case toast.DismissedMsg:
		m.toast = nil
		return m, nil
	case menuActionDoneMsg:
		return m.applyActionDone(msg)
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// applyActionDone consumes the result of a dispatched action. Errors
// stay in the modal as a red error line; success surfaces a green
// toast and resets the sub-mode to the list. The menu does not
// auto-close on success — the user can read the toast, then Esc to
// dismiss.
func (m menuModel) applyActionDone(msg menuActionDoneMsg) (menuModel, tea.Cmd) {
	if msg.err != nil {
		m.errMsg = msg.err.Error()
		return m, nil
	}
	m.errMsg = ""
	if msg.toast != "" {
		t := toast.NewDefault(msg.toast, m.pal)
		m.toast = &t
		return m, t.Init()
	}
	return m, nil
}

// handleKey routes one key into the menu, dispatched by the active
// sub-mode. List → handleListKey (filter / nav / Enter); Range →
// handleRangeKey (text input + parse); Target → handleTargetKey
// (Output sink picker, c/s/f hotkeys); Correct → handleCorrectKey
// (HH:MM input); Land → handleLandKey (Bundesland picker).
func (m menuModel) handleKey(msg tea.KeyMsg) (menuModel, tea.Cmd) {
	switch m.subMode {
	case menuSubModeRange:
		return m.handleRangeKey(msg)
	case menuSubModeTarget:
		return m.handleTargetKey(msg)
	case menuSubModeCorrect:
		return m.handleCorrectKey(msg)
	case menuSubModeLand:
		return m.handleLandKey(msg)
	}
	return m.handleListKey(msg)
}

// handleCorrectKey forwards a key to the start-time correction form.
// canceled rolls back to the action list; submitted dispatches
// SessionWriter.CorrectStart and returns to the list — the result
// lands as menuActionDoneMsg in Update.
func (m menuModel) handleCorrectKey(msg tea.KeyMsg) (menuModel, tea.Cmd) {
	next, cmd, ev := m.correctF.handleKey(msg, m.deps.Clock.Now())
	m.correctF = next
	if ev.canceled {
		m.subMode = menuSubModeList
		m.pending = menuAction{}
		m.correctF = correctForm{}
		return m, nil
	}
	if ev.submitted {
		m.subMode = menuSubModeList
		m.pending = menuAction{}
		m.correctF = correctForm{}
		return m, correctCmd(m.deps, ev.parsed)
	}
	return m, cmd
}

// handleLandKey forwards a key to the Bundesland picker. canceled
// rolls back; picked dispatches SyncGermanHolidays for the chosen
// Land + current year and returns to the list.
func (m menuModel) handleLandKey(msg tea.KeyMsg) (menuModel, tea.Cmd) {
	next, ev := m.landP.handleKey(msg)
	m.landP = next
	if ev.canceled {
		m.subMode = menuSubModeList
		m.pending = menuAction{}
		m.landP = landPicker{}
		return m, nil
	}
	if ev.picked {
		m.subMode = menuSubModeList
		m.pending = menuAction{}
		m.landP = landPicker{}
		return m, landSyncCmd(m.deps, ev.entry.code)
	}
	return m, nil
}

// handleRangeKey forwards a key to the range-form sub-picker.
// rangeEvent.canceled rolls back to the action list (drops pending);
// rangeEvent.submitted captures rangeExpr and transitions to the
// output-target picker so the user picks how to read the result.
func (m menuModel) handleRangeKey(msg tea.KeyMsg) (menuModel, tea.Cmd) {
	next, cmd, ev := m.rangeF.handleKey(msg, m.deps.Clock.Now())
	m.rangeF = next
	if ev.canceled {
		m.subMode = menuSubModeList
		m.pending = menuAction{}
		m.rangeF = rangeForm{}
		return m, nil
	}
	if ev.submitted {
		m.rangeExpr = ev.expr
		m.subMode = menuSubModeTarget
		m.target = newTargetPicker(viewerForKind(m.pending.kind))
		return m, nil
	}
	return m, cmd
}

// handleListKey is the action-list key-router. Skill §Keybind grammar:
// Esc cancels (clears query first, closes second); j/k/down/up
// navigate; g/G jump to first/last; Backspace shrinks query; Enter
// runs the focused action; any other rune extends the query (palette-
// style live-filter). Rune-handling lives in handleRuneKey to keep
// cyclomatic complexity within the project budget.
func (m menuModel) handleListKey(msg tea.KeyMsg) (menuModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m.handleEsc(), nil
	case "j", "down":
		if n := len(m.filtered); n > 0 {
			m.cursor = (m.cursor + 1) % n
		}
		return m, nil
	case "k", "up":
		if n := len(m.filtered); n > 0 {
			m.cursor = (m.cursor + n - 1) % n
		}
		return m, nil
	case "g":
		m.cursor = 0
		return m, nil
	case "G":
		if n := len(m.filtered); n > 0 {
			m.cursor = n - 1
		}
		return m, nil
	case "backspace":
		return m.handleBackspace(), nil
	case "enter":
		if m.cursor >= 0 && m.cursor < len(m.filtered) {
			return m.runAction(m.filtered[m.cursor])
		}
		return m, nil
	}
	return m.handleRuneKey(msg), nil
}

// handleTargetKey forwards a key to the output-target sub-picker.
// targetEvent.canceled rolls back to the action list (preserves the
// list filter so the user keeps their place). targetEvent.picked
// dispatches the pending action through dispatchPending and returns
// to the list — the result lands as menuActionDoneMsg in Update.
func (m menuModel) handleTargetKey(msg tea.KeyMsg) (menuModel, tea.Cmd) {
	next, ev := m.target.handleKey(msg)
	m.target = next
	if ev.canceled {
		m.subMode = menuSubModeList
		m.pending = menuAction{}
		return m, nil
	}
	if ev.picked {
		cmd := m.dispatchPending(ev.target)
		m.subMode = menuSubModeList
		m.pending = menuAction{}
		m.rangeF = rangeForm{}
		m.rangeExpr = ""
		return m, cmd
	}
	return m, nil
}

// dispatchPending builds the tea.Cmd that renders + outputs the
// pending action's content for the picked target. Slice C/D wires
// Brief / Export CSV+JSON / Stats; the remaining kinds still surface
// the placeholder TODO toast until Slice E lands.
func (m menuModel) dispatchPending(target outputTarget) tea.Cmd {
	switch m.pending.kind {
	case menuActionBriefWeek, menuActionBriefMonth:
		return briefCmd(m.deps, target, briefScopeFor(m.pending.kind))
	case menuActionExportCSV:
		return exportCmd(m.deps, target, m.rangeExpr, exportFormatCSV)
	case menuActionExportJSON:
		return exportCmd(m.deps, target, m.rangeExpr, exportFormatJSON)
	case menuActionStats:
		return statsCmd(m.deps, target, m.rangeExpr)
	}
	label := m.pending.label
	return func() tea.Msg {
		return menuActionDoneMsg{toast: "TODO: " + label}
	}
}

// handleEsc implements the two-step cancel: 1st press clears the
// filter query (and resets cursor to 0); 2nd press closes the menu.
func (m menuModel) handleEsc() menuModel {
	if m.query != "" {
		m.query = ""
		m.cursor = 0
		m.filtered = computeMenuActions(m.activeTab, m.deps, "")
		return m
	}
	m.open = false
	return m
}

// handleBackspace shrinks the filter query by one rune and rebuilds
// the visible list.
func (m menuModel) handleBackspace() menuModel {
	r := []rune(m.query)
	if len(r) == 0 {
		return m
	}
	m.query = string(r[:len(r)-1])
	m.filtered = computeMenuActions(m.activeTab, m.deps, m.query)
	m.clampCursor()
	return m
}

// handleRuneKey extends the filter query by the typed rune unless the
// rune is a navigation key (j/k/g/G — those take precedence over the
// filter, matching sidekick-palette muscle memory).
func (m menuModel) handleRuneKey(msg tea.KeyMsg) menuModel {
	if msg.Type != tea.KeyRunes || len(msg.Runes) != 1 {
		return m
	}
	r := msg.Runes[0]
	if r == 'j' || r == 'k' || r == 'g' || r == 'G' {
		return m
	}
	m.query += string(r)
	m.filtered = computeMenuActions(m.activeTab, m.deps, m.query)
	m.clampCursor()
	return m
}

func (m *menuModel) clampCursor() {
	n := len(m.filtered)
	if n == 0 {
		m.cursor = 0
		return
	}
	if m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// runAction is the dispatch point for action.kind. Brief skips
// straight to the target picker (range fixed by action variant);
// Export/Stats enter the range form first; Correct opens the HH:MM
// form; Land opens the Bundesland picker. The default branch is dead
// in the current registry — kept as a defence so a future kind that
// forgets a runAction wiring at least surfaces *something* instead of
// silently dropping the keypress.
func (m menuModel) runAction(a menuAction) (menuModel, tea.Cmd) {
	switch a.kind {
	case menuActionBriefWeek, menuActionBriefMonth:
		m.pending = a
		m.subMode = menuSubModeTarget
		m.target = newTargetPicker(briefViewer)
		m.errMsg = ""
		m.toast = nil
		return m, nil
	case menuActionExportCSV, menuActionExportJSON, menuActionStats:
		m.pending = a
		m.subMode = menuSubModeRange
		m.rangeF = newRangeForm(m.pal, defaultRangeFor(a.kind), a.label)
		m.errMsg = ""
		m.toast = nil
		return m, textinput.Blink
	case menuActionCorrect:
		m.pending = a
		m.subMode = menuSubModeCorrect
		m.correctF = newCorrectForm(m.pal, a.label, correctDefaultFor(m.deps))
		m.errMsg = ""
		m.toast = nil
		return m, textinput.Blink
	case menuActionLand:
		m.pending = a
		m.subMode = menuSubModeLand
		m.landP = newLandPicker(currentLand())
		m.errMsg = ""
		m.toast = nil
		return m, nil
	}
	t := toast.NewDefault("TODO: "+a.label, m.pal)
	m.toast = &t
	return m, t.Init()
}

// defaultRangeFor mirrors the CLI defaults for each action: `flow
// worktime stats` defaults to "month"; `flow worktime export` defaults
// to empty (everything). Both are reasonable starting points the user
// can edit before submitting.
func defaultRangeFor(kind menuActionKind) string {
	switch kind {
	case menuActionStats:
		return "month"
	case menuActionExportCSV, menuActionExportJSON:
		return "month"
	}
	return ""
}

// viewerForKind picks the pager viewer for the tmux-Split target.
// Brief is Markdown → glow renders the headers/list nicely; Export
// (CSV/JSON) and Stats are plain text → less -S preserves columns
// without word-wrap.
func viewerForKind(kind menuActionKind) string {
	switch kind {
	case menuActionBriefWeek, menuActionBriefMonth:
		return briefViewer
	case menuActionExportCSV, menuActionExportJSON:
		return exportPager
	case menuActionStats:
		return statsPager
	}
	return briefViewer
}

// View renders the modal body. The Worktime root composites this over
// the tab body when Active() is true; we don't draw the surrounding
// titlebox here — that stays in the root's View() so the tab strip
// keeps rendering as the modal's frame.
//
// Sub-mode dispatch: List renders the action picker; Target renders
// the output-sink sub-picker. Toast / errMsg overlays are drawn under
// whichever body is active so the user sees feedback in either mode.
func (m menuModel) View() string {
	if !m.open || m.width == 0 {
		return ""
	}
	inner := m.width - 4
	if inner < 30 {
		inner = 30
	}
	var body string
	switch m.subMode {
	case menuSubModeTarget:
		body = m.target.view(m.pending.label, m.pal, inner)
	case menuSubModeRange:
		body = m.rangeF.view(m.pal, inner)
	case menuSubModeCorrect:
		body = m.correctF.view(m.pal, inner)
	case menuSubModeLand:
		body = m.landP.view(m.pending.label, m.pal, inner)
	default:
		body = m.renderListBody(inner)
	}
	rows := []string{body}
	if m.toast != nil {
		rows = append(rows, "", "  "+m.toast.View())
	}
	if m.errMsg != "" {
		rows = append(rows, "", theme.Err("  "+m.errMsg, m.pal))
	}
	return strings.Join(rows, "\n")
}

// renderListBody renders the list-mode contents: title, filter line,
// sectioned action list, and the in-modal footer hint. Toast / errMsg
// are appended by View() so they appear under both List and Target.
func (m menuModel) renderListBody(inner int) string {
	rows := []string{
		theme.Highlight("  Aktionen", m.pal),
		"",
		m.renderFilterLine(),
		"",
	}
	rows = append(rows, m.renderList(inner)...)
	rows = append(rows, "", m.renderFooter(inner))
	return strings.Join(rows, "\n")
}

func (m menuModel) renderFilterLine() string {
	prompt := lipgloss.NewStyle().Foreground(m.pal.Accent).Render("/ ")
	body := m.query
	if body == "" {
		return "  " + prompt + theme.Dim("(tippen → filter)", m.pal)
	}
	val := lipgloss.NewStyle().Foreground(m.pal.Fg).Render(body)
	return "  " + prompt + val
}

// renderList renders the visible action set, grouped by section. The
// cursor index is computed in flat order across all visible rows
// (matches handleKey's m.cursor semantics — cursor counts entries in
// m.filtered, not section breaks).
func (m menuModel) renderList(inner int) []string {
	if len(m.filtered) == 0 {
		return []string{theme.Dim("  Keine Aktion entspricht dem Filter.", m.pal)}
	}
	var rows []string
	prevSection := ""
	for i, a := range m.filtered {
		if a.section != prevSection {
			if prevSection != "" {
				rows = append(rows, "")
			}
			rows = append(rows, picker.SectionHeader(a.section, inner, m.pal))
			prevSection = a.section
		}
		rows = append(rows, picker.Row(i == m.cursor, a.label, a.hint, inner, m.pal))
	}
	return rows
}

// renderFooter draws the in-modal hint line. Skill §Hint format: max 4
// hints, German wording, ` → ` connector, `  ·  ` separator.
func (m menuModel) renderFooter(inner int) string {
	hints := []string{
		"j/k → bewegen",
		"enter → öffnen",
		"tippen → filter",
		"esc → zu",
	}
	return renderFooterHints(m.pal, hints, inner)
}
