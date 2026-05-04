package worktime

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/confirm"
	"github.com/serverkraken/flow/internal/frontend/tui/components/form"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/components/statusbar"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
	"github.com/serverkraken/flow/internal/frontend/tui/components/toast"
)

// — messages —

type heuteLoadedMsg struct {
	day   domain.Day
	notes []string
	err   error
}

type heuteActionDoneMsg struct {
	err   error
	toast string
}

// — dialog modes —

type heuteDialog int

const (
	heuteDialogNone heuteDialog = iota
	heuteDialogTag
	heuteDialogNote
	heuteDialogEdit
	heuteDialogDelete
	heuteDialogNoteAttach
	heuteDialogHelp
)

// heute is the Heute (today) sub-model. F4.3 wave B gives it the action
// surface needed for everyday tracking: start/stop/pause/resume plus
// per-session edits (tag, note, edit, delete). Wave-B+ slice 1 adds the
// Kompendium-attach trio (`n` attach via LinkWriter, `o` view via
// NoteOpener, render-line for attached IDs). Other decoration features
// (sparkline, pomodoro, typical stop time, day-off banner, best-streak
// celebration, smart stop suggestion) stay deferred — they don't block
// anything.
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
	// confirmModel drives the destructive-delete dialog. Vorher hand-rolled
	// (eigene y/z/j-Behandlung mit Enter=Abbrechen-as-Default) — das invertierte
	// die Enter=Confirm-Konvention der restlichen Codebase und verlangte
	// systemweit die gleiche Inversion. Skill §Component vocabulary verlangt
	// die kanonische Form: y/Enter → ja, n/Esc → nein.
	confirmModel *confirm.Model

	editIdx  int
	editDate time.Time

	// attachedNotes holds Kompendium note IDs linked to today, in
	// insertion order (LinkReader keeps that). Loaded alongside the day
	// in loadCmd; render shows them as a chip line under the headline.
	attachedNotes []string

	// toast is the canonical green-✓ confirmation surface (toast.Model).
	// Pre-Welle-3 this was a hand-rolled cyan-foreground render with a
	// custom heuteClearToastMsg tick — wrong color (cyan = active, not
	// success) and reinventing the toast component.
	toast  *toast.Model
	errMsg string

	// actionInFlight blocks dayRefreshMsg from running between the
	// async action being dispatched (e.g. confirm.ResultMsg → deleteCmd)
	// and heuteActionDoneMsg arriving. Without this gate, a tick fired
	// in that window would re-load the day with editIdx still pointing
	// at a session whose siblings may have just been renumbered.
	actionInFlight bool
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

// ConsumesKeys lists letter keys Heute claims away from the sidekick's
// global navigation. `n` is kompendium-attach (advertised in the
// `?`-overlay); `p` is pause-the-running-session. Both keys would
// otherwise be eaten by the sidekick to switch screens — the bindings
// would be tot in sidekick mode, only working in `flow worktime today`
// standalone.
func (h heute) ConsumesKeys() []string { return []string{"n", "p"} }

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
			h.attachedNotes = msg.notes
			h.clampCursor()
		}
		return h, nil

	case dayRefreshMsg:
		// While a dialog is open OR an async action is in flight,
		// suppress the periodic refresh: the frozen editIdx would
		// point at a different session if the reload re-numbered the
		// list (manual edit from another shell, or a session that
		// ended elsewhere). The next refresh after the dialog closes
		// is fine. heuteActionDoneMsg explicitly triggers loadCmd on
		// success, so the user sees the fresh state right after
		// submit. The actionInFlight gate covers the gap between
		// confirm.ResultMsg clearing the dialog and the async
		// deleteCmd / similar returning.
		if h.dialog != heuteDialogNone || h.actionInFlight {
			return h, nil
		}
		return h, h.loadCmd()

	case heuteActionDoneMsg:
		h.actionInFlight = false
		// On error: if a dialog is still open (edit/tag/note forms), keep
		// it open and surface the error inside it via errMsg so the user
		// can retry without re-filling the form. If the dialog already
		// closed (delete confirms close on submit), surface a danger
		// toast so the failure is visible — without this branch the
		// previous code blanked the whole tagesansicht via h.err.
		if msg.err != nil {
			if h.dialog != heuteDialogNone {
				h.errMsg = msg.err.Error()
				return h, nil
			}
			t := toast.NewDanger(msg.err.Error(), h.pal)
			h.toast = &t
			return h, tea.Batch(h.loadCmd(), t.Init())
		}
		h.dialog = heuteDialogNone
		h.input.Blur()
		h.input.SetValue("")
		h.form = nil
		h.formCur = 0
		h.confirmModel = nil
		h.errMsg = ""
		h.err = nil
		if msg.toast != "" {
			t := toast.NewDefault(msg.toast, h.pal)
			h.toast = &t
			return h, tea.Batch(h.loadCmd(), t.Init())
		}
		return h, h.loadCmd()

	case toast.DismissedMsg:
		h.toast = nil
		return h, nil

	case confirm.ResultMsg:
		// Auflösung des Delete-Confirm-Dialogs. Bei „ja" nur dispatchen, wenn
		// editIdx noch in den Bounds des aktuellen Day liegt (zwischen Open
		// und Confirm könnte ein dayRefreshMsg die Sessions umnummeriert
		// haben — defensiv nochmal prüfen).
		if h.dialog != heuteDialogDelete {
			return h, nil
		}
		h.dialog = heuteDialogNone
		h.confirmModel = nil
		if !msg.Confirmed {
			return h, nil
		}
		h.actionInFlight = true
		return h, h.deleteCmd(h.editDate, h.editIdx)

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
	linkReader := h.deps.LinkReader
	clock := h.deps.Clock
	return func() tea.Msg {
		day, err := reader.Today()
		if err != nil {
			return heuteLoadedMsg{day: day, err: err}
		}
		// Note-load errors stay silent — the day is the primary surface.
		// A broken linkstsv shouldn't blank the headline; the chip line
		// just doesn't render.
		notes, _ := linkReader.ListByDate(clock.Now())
		return heuteLoadedMsg{day: day, notes: notes}
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
	case "o":
		return h, h.viewAttachedNoteCmd()
	case "O":
		return h, h.editAttachedNoteCmd()
	case "ctrl+d":
		return h, h.detachAttachedNoteCmd()
	case "?":
		// Standalone-`flow worktime today`: kein sidekick-Wrapper, der
		// `?` abfängt. Im sidekick-Modus kommt der Key gar nicht hier
		// an (sidekick.model:159 fängt ihn vorher), Heute öffnet sein
		// eigenes Overlay nur wenn die globale Hilfe nicht greift.
		h.dialog = heuteDialogHelp
		return h, nil
	}
	return h.handleDialogOpenKey(msg)
}

// handleDialogOpenKey dispatches the keys that activate a dialog. Split
// from handleNormalKey to keep gocyclo under the project ceiling — the
// session-edit family (t/N/E/⏎/D) plus the day-level Kompendium attach
// (n) read more naturally as one group anyway.
func (h heute) handleDialogOpenKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
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
	case "D":
		// Skill §Keybind grammar: „`D` (uppercase) | Destructive action on
		// focused item — **always** y/N confirms". Vorher lowercase d, was
		// auf der Sister-Surface (Kompendium-Browse) bereits korrekt mit D
		// gebunden ist — Konsistenz wiederhergestellt.
		if h.onSession() {
			return h.openDeleteDialog()
		}
	case "n":
		return h.openNoteAttachDialog()
	}
	return h, nil
}

// viewAttachedNoteCmd opens the first attached Kompendium note for today
// via NoteOpener.View (typically a tmux split running the configured
// note viewer). When nothing is attached the action is a noop with a
// dim toast — calling NoteOpener.View("") would surface a hard error
// the user neither caused nor cares about.
func (h heute) viewAttachedNoteCmd() tea.Cmd {
	if len(h.attachedNotes) == 0 {
		return func() tea.Msg {
			return heuteActionDoneMsg{toast: "  ℹ Keine Notiz angehängt — `n` hängt eine an"}
		}
	}
	id := h.attachedNotes[0]
	opener := h.deps.NoteOpener
	return func() tea.Msg {
		if err := opener.View(id); err != nil {
			return heuteActionDoneMsg{err: err}
		}
		return heuteActionDoneMsg{toast: fmt.Sprintf("✓ Note %s geöffnet", id)}
	}
}

// editAttachedNoteCmd opens the first attached note in the editor via
// NoteOpener.Open (typically tmux split + nvim). Mirrors
// viewAttachedNoteCmd; the no-attached-notes branch toasts instead of
// dispatching through the empty-id guard.
func (h heute) editAttachedNoteCmd() tea.Cmd {
	if len(h.attachedNotes) == 0 {
		return func() tea.Msg {
			return heuteActionDoneMsg{toast: "  ℹ Keine Notiz angehängt — `n` hängt eine an"}
		}
	}
	id := h.attachedNotes[0]
	opener := h.deps.NoteOpener
	return func() tea.Msg {
		if err := opener.Open(id); err != nil {
			return heuteActionDoneMsg{err: err}
		}
		return heuteActionDoneMsg{toast: fmt.Sprintf("✓ Note %s zum Bearbeiten geöffnet", id)}
	}
}

// detachAttachedNoteCmd removes the first attached note from today via
// LinkWriter.Remove. No confirm dialog: the operation is reversible
// (re-attach via `n` with the same ID) and the underlying store is
// idempotent — over-removing a missing pair is documented as a no-op.
// `D` (uppercase) is intentionally NOT used because it already binds
// delete-session in wave B; `Ctrl+D` reads as "destructive on the
// linked entity" and stays out of the way.
func (h heute) detachAttachedNoteCmd() tea.Cmd {
	if len(h.attachedNotes) == 0 {
		return func() tea.Msg {
			return heuteActionDoneMsg{toast: "  ℹ Keine Notiz angehängt"}
		}
	}
	id := h.attachedNotes[0]
	date := h.deps.Clock.Now()
	writer := h.deps.LinkWriter
	return func() tea.Msg {
		if err := writer.Remove(date, id); err != nil {
			return heuteActionDoneMsg{err: err}
		}
		return heuteActionDoneMsg{toast: fmt.Sprintf("✓ Note %s entfernt", id)}
	}
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

// openNoteAttachDialog activates the single-textinput dialog the user
// types a Kompendium note ID into. The submit branch dispatches
// LinkWriter.Add against today's date; the load branch refreshes
// attachedNotes so the chip line picks the new ID up immediately.
// Note: this is the minimal-viable attach — no fuzzy picker, no
// recent-notes suggestions. A picker UI is a follow-up slice.
func (h heute) openNoteAttachDialog() (tea.Model, tea.Cmd) {
	h.editDate = h.deps.Clock.Now()
	h.dialog = heuteDialogNoteAttach
	h.input = form.NewTextInput("Note-ID (z.B. 2026-05-03 oder daily-2026-05-03)", h.pal)
	h.input.SetValue("")
	h.input.Focus()
	h.errMsg = ""
	return h, textinput.Blink
}

func (h heute) openDeleteDialog() (tea.Model, tea.Cmd) {
	s := h.day.Sessions[h.cursor]
	h.editIdx = h.cursor
	h.editDate = s.Date
	h.dialog = heuteDialogDelete
	h.errMsg = ""
	question := fmt.Sprintf("Session %d löschen?", h.cursor+1)
	detail := fmt.Sprintf("%s → %s   %s",
		s.Start.Format("15:04"), s.Stop.Format("15:04"), formatDur(s.Elapsed))
	cm := confirm.New(question, detail, h.pal)
	h.confirmModel = &cm
	return h, cm.Init()
}

// — dialog dispatch —

func (h heute) handleDialogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch h.dialog {
	case heuteDialogDelete:
		// Confirm-Dialog forwarded an die kanonische confirm.Model. Der
		// resultierende confirm.ResultMsg landet im Outer-Update und löst
		// h.deleteCmd aus (oder schließt den Dialog ohne Aktion).
		if h.confirmModel == nil {
			h.dialog = heuteDialogNone
			return h, nil
		}
		updated, cmd := h.confirmModel.Update(msg)
		h.confirmModel = &updated
		return h, cmd
	case heuteDialogEdit:
		return h.handleFormKey(msg)
	case heuteDialogTag, heuteDialogNote, heuteDialogNoteAttach:
		return h.handleSimpleInputKey(msg)
	case heuteDialogHelp:
		// Help-Overlay schließt explizit auf Esc oder ?. Andere Tasten
		// laufen normal weiter, damit der User direkt nach dem
		// Erinnern-an-die-Tastenbelegung nicht nochmal drücken muss
		// (z.B. `?` öffnet Help, dann `s` zum Starten — ohne diese
		// Logik wurde `s` als Dialog-Dismiss verschluckt).
		switch msg.String() {
		case "esc", "?", "q":
			h.dialog = heuteDialogNone
			return h, nil
		}
		h.dialog = heuteDialogNone
		return h.handleNormalKey(msg)
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
	case tea.KeyTab, tea.KeyShiftTab:
		// Single-input dialogs (Tag/Note/NoteAttach) have nowhere to
		// tab to — swallow the key instead of letting bubbles textinput
		// insert a literal tab character that would survive into the
		// stored field. The tsvsessions writer now sanitises tab/CR/LF
		// at write time too, but rejecting at the input boundary is
		// less surprising for the user.
		return h, nil
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

	case heuteDialogNoteAttach:
		id := strings.TrimSpace(h.input.Value())
		if id == "" {
			h.errMsg = "Note-ID darf nicht leer sein"
			return h, nil
		}
		date := h.editDate
		writer := h.deps.LinkWriter
		return h, func() tea.Msg {
			if err := writer.Add(date, id); err != nil {
				return heuteActionDoneMsg{err: err}
			}
			return heuteActionDoneMsg{toast: fmt.Sprintf("✓ Note %s angehängt", id)}
		}
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
		return stDim(h.pal, "  Heute lädt …")
	}
	if h.err != nil {
		return stErr(h.pal, h.err.Error())
	}

	inner := h.width - 4
	now := h.deps.Clock.Now()

	rows := []string{h.renderHeadline(now), "", h.renderProgressBar(inner), h.renderSummary(inner)}
	if line := h.renderAttachedNotes(); line != "" {
		rows = append(rows, "", line)
	}
	if line := h.renderPauseHint(now); line != "" {
		rows = append(rows, "", line)
	}
	rows = append(rows, h.renderSessionsList(inner, now)...)
	if h.toast != nil {
		rows = append(rows, "", "  "+h.toast.View())
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
	pctStr := theme.Dim(fmt.Sprintf("%d%%", pct), h.pal)
	// Skill §Spacing: discrete scale {0,1,2,4} — 2-Cell-Indent links, 4-Cell-Gaps
	// zwischen den drei Status-Cells. 3-Cell-Gaps (vorher) lagen außerhalb der
	// Skala und ließen die Headline ungleichmäßig ausgerichtet wirken.
	return "  " + totalStr + "    " + statusStr + "    " + pctStr
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

// renderAttachedNotes renders the chip line that surfaces today's
// linked Kompendium notes. Empty result skips the row entirely so
// the layout doesn't grow a blank gap when nothing is attached.
//
// CLAUDE.md forbids emoji pictograms in TUI output: the previous 🔗
// rendered at emoji width on some fonts and broke column alignment
// against the surrounding monospace glyphs (▶ ✓ ● ○ …). ● is in the
// approved set and reads naturally as a list-item marker.
func (h heute) renderAttachedNotes() string {
	if len(h.attachedNotes) == 0 {
		return ""
	}
	label := theme.Highlight("●", h.pal)
	ids := stDim(h.pal, strings.Join(h.attachedNotes, "  ·  "))
	hint := stDim(h.pal, "  ·  o/O → ansehen/bearbeiten  ·  Ctrl+D → entfernen")
	return "  " + label + "  " + ids + hint
}

func (h heute) renderPauseHint(now time.Time) string {
	if !h.day.IsPaused() || h.day.PausedAt == nil {
		return ""
	}
	return "  " +
		theme.Warning("⏸ in Pause", h.pal) +
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
		rows = append(rows, theme.Success(
			fmt.Sprintf("  ▶ %s → …   %s   läuft",
				h.day.Active.Format("15:04"), formatDur(elapsed)), h.pal))
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

// footerHints liefert max 4 Hints, priorisiert nach Frequenz (Skill §Hint
// format: „Maximum 4 hints in a permanent footer; if more apply, the surplus
// belongs in the `?` overlay"). Reihenfolge:
//  1. s → start/stop/resume — globaler Default-State, immer relevant.
//  2. j/k → bewegen — Listenkontext immer.
//  3. ⏎ → bearbeiten — wenn auf Session, häufigste Edit-Action.
//  4. D → löschen — wenn auf Session, einziger destructive Slot.
//
// Tag/Note/Pause sind im `?`-Overlay des Sidekick-Roots dokumentiert.
func (h heute) footerHints() []string {
	var actions []string
	switch {
	case h.day.IsRunning():
		actions = append(actions, "s → stoppen")
	case h.day.IsPaused():
		actions = append(actions, "s → fortsetzen")
	default:
		actions = append(actions, "s → starten")
	}
	actions = append(actions, "j/k → bewegen")
	if h.onSession() {
		actions = append(actions, "enter → bearbeiten", "D → löschen")
	}
	if len(actions) > 4 {
		actions = actions[:4]
	}
	return actions
}

// renderDialog rendert die vier Dialog-Modi (Tag, Notiz, Edit, Delete).
//
// Skill §Component vocabulary: Dialog-Title bekommt purple-bold (wie
// titlebox/help-Header) statt dim, sonst ist er im Footer nicht mehr von
// dem Hint-String unterscheidbar. Hint-Format folgt §Hint format mit
// `key → action  ·  …`-Separatoren.
//
// Delete-Modus delegiert komplett an confirm.Model, das selbst
// Yellow-Question, Detail-Zeile und kanonisches y/Enter-→-ja-Hint mitbringt.
func (h heute) renderDialog() string {
	inner := h.width - 4

	var rows []string
	var title, hint string

	switch h.dialog {
	case heuteDialogTag:
		title = "Tag setzen"
		hint = "Enter → speichern  ·  leer → löschen  ·  Esc → abbrechen"
		rows = append(rows, picker.SectionHeader("tag", inner, h.pal), "  "+h.input.View())

	case heuteDialogNote:
		title = "Session-Notiz"
		hint = "Enter → speichern  ·  leer → löschen  ·  Esc → abbrechen"
		rows = append(rows, picker.SectionHeader("notiz", inner, h.pal), "  "+h.input.View())

	case heuteDialogNoteAttach:
		title = "Kompendium-Note anhängen"
		hint = "Enter → anhängen  ·  Esc → abbrechen"
		rows = append(rows, picker.SectionHeader("note id", inner, h.pal), "  "+h.input.View())
		if len(h.attachedNotes) > 0 {
			rows = append(rows, "", stDim(h.pal,
				"  bereits angehängt:  "+strings.Join(h.attachedNotes, "  ·  ")))
		}

	case heuteDialogEdit:
		title = "Session bearbeiten"
		hint = "Tab/↑↓ → Feld  ·  Enter → weiter / speichern  ·  Esc → abbrechen"
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
		// confirm.Model rendert bereits seinen eigenen y/Enter-→-ja-Hint;
		// hier nur der gemeinsame Title-Strip, kein doppelter Hint nötig.
		hint = ""
		if h.confirmModel != nil {
			rows = append(rows, "  "+h.confirmModel.View())
		}

	case heuteDialogHelp:
		title = "Heute · Hilfe"
		hint = "beliebige Taste schließt"
		rows = append(rows, h.renderHelpRows(inner)...)
	}

	if h.errMsg != "" {
		rows = append(rows, "", theme.Err("  "+h.errMsg, h.pal))
	}
	titleLine := "  " + theme.Highlight(title, h.pal)
	if hint != "" {
		titleLine += theme.Dim("  ·  "+hint, h.pal)
	}
	rows = append(rows, "", titleLine)
	return strings.Join(rows, "\n")
}

// renderHelpRows enumerates Heute's keybinds for the standalone `?`
// overlay. Sections are grouped by purpose, keys ordered roughly by
// frequency. Inside the sidekick wrapper this overlay never opens —
// sidekick.model intercepts `?` first; that overlay carries its own
// "Worktime — Heute" section. Both must stay in sync; a screen-level
// drift is the easier failure mode to spot during review.
func (h heute) renderHelpRows(inner int) []string {
	type entry struct{ key, desc string }
	type section struct {
		title   string
		entries []entry
	}
	sections := []section{
		{"Cursor & Action", []entry{
			{"j/k · g/G", "bewegen · oben/unten"},
			{"s", "starten / stoppen / fortsetzen"},
			{"p", "pause (im laufenden Zustand)"},
		}},
		{"Session-Edit (auf fokussierter Zeile)", []entry{
			{"E / Enter", "Session bearbeiten"},
			{"D", "Session löschen (y/Enter bestätigt)"},
			{"t · N", "Tag · Notiz setzen"},
		}},
		{"Kompendium (für heute)", []entry{
			{"n", "Note anhängen (ID eingeben)"},
			{"o · O", "erste Note ansehen · bearbeiten"},
			{"Ctrl+D", "erste Note entfernen"},
		}},
	}
	rows := []string{}
	for i, sec := range sections {
		if i > 0 {
			rows = append(rows, "")
		}
		rows = append(rows, picker.SectionHeader(sec.title, inner, h.pal))
		for _, e := range sec.entries {
			keyCell := lipgloss.NewStyle().Width(14).Render(theme.Highlight(e.key, h.pal))
			rows = append(rows, "  "+keyCell+stDim(h.pal, e.desc))
		}
	}
	return rows
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

// stDim is the worktime-screen-local thin wrapper over the central
// theme.Dim builder. Kept as a wrapper (rather than open-coding
// theme.Dim at call-sites) because every worktime-tab calls this
// dozens of times — the short name + arg order is the screen's
// existing idiom. Same for stErr below, plus the "  "-indent prefix
// that's load-bearing for error rows under the box border.
func stDim(p theme.Palette, s string) string { return theme.Dim(s, p) }

func stErr(p theme.Palette, s string) string { return theme.Err("  "+s, p) }

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
