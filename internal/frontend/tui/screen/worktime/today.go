package worktime

// Heute (today) — model + Update-Routing + state accessors + keymap.
// Render-Logik in today_render.go, Dialog-Surfaces in today_dialog.go,
// async Action-Cmds in today_actions.go (Skill §No-Monoliths). Diese
// Datei behält den schmalen "wer-zappt-was"-Kern damit das Routing in
// einem Blick erfassbar bleibt.

import (
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/confirm"
	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown_overlay"
	"github.com/serverkraken/flow/internal/frontend/tui/components/project_picker"
	"github.com/serverkraken/flow/internal/frontend/tui/components/toast"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
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
	// info marks the toast as an Info-kind notice (cyan ›) instead of
	// the default Success kind (green ✓). Used for "nothing to do"
	// branches (e.g. detach when no note is attached) where the operation
	// is neither a success nor a failure.
	info bool
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
	// heuteDialogNoteView ist der integrierte Markdown-Viewer für
	// angehängte Kompendium-Notes. Wird vom `o`-Key geöffnet und nutzt
	// einen viewport.Model + den injizierten ports.MarkdownRenderer —
	// dieselbe Pipeline wie der Cheatsheet-Tab.
	heuteDialogNoteView
)

// heute is the Heute (today) sub-model. F4.3 wave B gives it the action
// surface needed for everyday tracking: start/stop/pause/resume plus
// per-session edits (tag, note, edit, delete). Wave-B+ slice 1 adds the
// Kompendium-attach trio (`n` attach via LinkWriter, `o` view via
// NoteOpener, render-line for attached IDs).
type heute struct {
	pal  theme.Palette
	deps Deps

	width  int
	height int

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
	// confirmModel drives the destructive-delete dialog.
	confirmModel *confirm.Model

	editIdx  int
	editDate time.Time

	// attachedNotes holds Kompendium note IDs linked to today, in
	// insertion order (LinkReader keeps that). Loaded alongside the day
	// in loadCmd; render shows them as a chip line under the headline.
	attachedNotes []string

	// toast is the canonical green-✓ confirmation surface (toast.Model).
	toast  *toast.Model
	errMsg string

	// actionInFlight blocks dayRefreshMsg from running between the
	// async action being dispatched (e.g. confirm.ResultMsg → deleteCmd)
	// and heuteActionDoneMsg arriving. Without this gate, a tick fired
	// in that window would re-load the day with editIdx still pointing
	// at a session whose siblings may have just been renumbered.
	actionInFlight bool

	// notePicker hosts den `n`-Dialog (Kompendium-Note-Attach). Aktiv
	// wenn dialog == heuteDialogNoteAttach. Shared widget — history-
	// drill nutzt denselben Picker-Typ (siehe note_attach_picker.go).
	// Heute scoped immer auf h.editDate (= Clock.Now() für `n`).
	notePicker noteAttachPicker

	// noteView ist der integrierte Note-Viewer (`o`-Key,
	// dialog == heuteDialogNoteView). Pointer-Pattern, weil der
	// Dialog optional aktiv ist; nil signalisiert geschlossen.
	// Konstruktion in openNoteViewDialog; Schließen via
	// markdown_overlay.ExitMsg im heute-Update-Switch.
	noteView *markdown_overlay.Model

	// pp ist der Projekt-Picker (full-screen-Overlay), der sich öffnet wenn
	// deps.ActiveSessions + deps.UserID gesetzt sind und `s` gedrückt wird
	// (neuer Pfad). Pointer-Pattern analog noteView: nil = geschlossen,
	// non-nil = Picker übernimmt Input + Render. Die Picker-Callbacks emittieren
	// pickerPickedMsg / pickerCreateMsg / pickerCancelMsg; heute.Update
	// routet sie in activeSessionsStartCmd bzw. projectsCreateThenStartCmd.
	pp *project_picker.Model

	// activeSessions ist der zuletzt geladene Stand der laufenden Sessions
	// für den angemeldeten User (leere Liste = keine). Wird bei dayRefreshMsg
	// via activeSessionsListCmd nachgeladen wenn deps.ActiveSessions != nil.
	// Nil bedeutet "nie geladen" (Legacy-Modus); leere Slice = geladen, nichts läuft.
	activeSessions []domain.ActiveSession
}

// noteAttachPickerLimit ist die maximale Anzahl jüngster Notes, die
// der Attach-Picker anbietet. Acht passt komfortabel unter den Input
// in einer Sidekick-Pane mit ~30 Zeilen.
const noteAttachPickerLimit = 8

func newHeute(p theme.Palette, deps Deps) heute {
	return heute{
		pal:        p,
		deps:       deps,
		editIdx:    -1,
		notePicker: newNoteAttachPicker(deps, p),
	}
}

// FilterActive bubbles up to the root so global tab keys don't intercept
// while a dialog input is taking text.
// The project_picker overlay also blocks tab-switching (the picker IS
// full-screen; if tab keys leak through the user would switch tabs while the
// picker is visible).
func (h heute) FilterActive() bool { return h.dialog != heuteDialogNone || h.pp != nil }

// FullScreen reports whether the worktime root should skip its titlebox
// + tab-strip wrap. True while the inline note viewer or the project picker
// is active — beide bringen eigenes full-screen-Chrome mit; ein zweiter
// titlebox-Wrapper würde Border duplizieren und die rechte Spalte clippen.
func (h heute) FullScreen() bool {
	return h.dialog == heuteDialogNoteView || h.pp != nil
}

// TextInputActive reports whether one of Heute's text-bearing dialogs
// (tag / note / edit form / kompendium-attach) is currently the focused
// surface. Lets the worktime root treat 'q' as a literal letter in
// those fields instead of an exit key. Confirm-delete and the help
// overlay are intentionally NOT text-input — q from there exits.
// The project_picker filter is also a text input — 'q' should type into
// the filter, not quit the whole app while the picker is visible.
func (h heute) TextInputActive() bool {
	if h.pp != nil {
		return true
	}
	switch h.dialog {
	case heuteDialogTag, heuteDialogNote, heuteDialogEdit, heuteDialogNoteAttach:
		return true
	}
	return false
}

// StateFilter has no meaning here — Heute has no filter expression.
func (h heute) StateFilter() string { return "" }

// StateCursor reports the focused session index for state persistence.
func (h heute) StateCursor() int { return h.cursor }

// ConsumesKeys lists letter keys Heute claims away from the sidekick's
// global navigation. `n` (kompendium-attach) is always claimed. `p`
// (pause) wird nur beansprucht, solange eine Session läuft — sonst
// no-op't der Handler und würde das globale `p → Palette` still
// verschlucken (toter Key). So bleibt die Palette aus Heute erreichbar,
// wann immer Pause bedeutungslos ist.
func (h heute) ConsumesKeys() []string {
	keys := []string{"n"}
	if h.day.IsRunning() {
		keys = append(keys, "p")
	}
	return keys
}

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
// heuteActionDoneMsg, which itself triggers a reload. When the new
// ActiveSessions dep is wired, also kick off the initial list load.
func (h heute) Init() tea.Cmd {
	cmds := []tea.Cmd{h.loadCmd()}
	if h.deps.ActiveSessions != nil && h.deps.UserID != "" {
		cmds = append(cmds, h.activeSessionsListCmd())
	}
	return tea.Batch(cmds...)
}

func (h heute) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.width = msg.Width
		h.height = msg.Height
		// markdown_overlay re-flows in SetSize: tabletten/code-blocks
		// passen sich an tmux-pane-Resize an (F1-Regression).
		if h.dialog == heuteDialogNoteView && h.noteView != nil {
			upd := h.noteView.SetSize(msg.Width, msg.Height)
			h.noteView = &upd
		}
		if h.pp != nil {
			upd := h.pp.SetSize(msg.Width, msg.Height)
			h.pp = &upd
		}
		return h, nil

	case markdown_overlay.ExitMsg:
		if h.dialog == heuteDialogNoteView {
			h.dialog = heuteDialogNone
			h.noteView = nil
			return h, nil
		}
		return h, nil

	case pickerPickedMsg, pickerCreateMsg, pickerCancelMsg, activeSessionsListMsg:
		return h.handlePickerMsg(msg)

	case heuteLoadedMsg:
		h.loaded = true
		h.err = msg.err
		if msg.err == nil {
			h.day = msg.day
			h.attachedNotes = msg.notes
			h = h.clampCursor()
		}
		return h, nil

	case dayRefreshMsg, ChangedMsg:
		// Periodic tick (dayRefreshMsg) and cross-tab mutation signal
		// (ChangedMsg) both ask for the same response: reload today.
		// Suppressed while a dialog is open OR an async action is in
		// flight, because a reload mid-edit would shift editIdx onto
		// a different session if the list got re-numbered.
		return h.reloadIfIdle()

	case heuteActionDoneMsg:
		return h.handleActionDone(msg)

	case toast.DismissedMsg:
		h.toast = nil
		return h, nil

	case confirm.ResultMsg:
		// Auflösung des Delete-Confirm-Dialogs.
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

	case tea.KeyPressMsg:
		// Projekt-Picker läuft als vollständiger bubbletea-Sub-Model:
		// alle Keys gehen direkt an den Picker. Der Picker emittiert
		// pickerPickedMsg / pickerCreateMsg / pickerCancelMsg als Cmd,
		// die dann im nächsten Update-Zyklus hier ankommen.
		if h.pp != nil {
			upd, cmd := h.pp.Update(msg)
			h.pp = &upd
			return h, cmd
		}
		if h.dialog != heuteDialogNone {
			return h.handleDialogKey(msg)
		}
		return h.handleNormalKey(msg)
	}
	return h, nil
}

// handleActionDone routet das Result einer asynchronen Action (toast,
// reload, error). Vorher inline im Update-Switch, jetzt als eigene
// Methode — Update.gocognit war über der 25er Schwelle, weil dieser
// Case 5+ Verzweigungen trägt.
func (h heute) handleActionDone(msg heuteActionDoneMsg) (tea.Model, tea.Cmd) {
	h.actionInFlight = false
	// On error: if a dialog is still open (edit/tag/note forms), keep
	// it open and surface the error inside it via errMsg so the user
	// can retry without re-filling the form. If the dialog already
	// closed (delete confirms close on submit), surface a danger
	// toast so the failure is visible.
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
	if msg.toast == "" {
		return h, h.loadCmd()
	}
	var t toast.Model
	if msg.info {
		t = toast.NewInfo(msg.toast, h.pal)
	} else {
		t = toast.NewSuccess(msg.toast, h.pal)
	}
	h.toast = &t
	return h, tea.Batch(h.loadCmd(), t.Init())
}

// reloadIfIdle returns (h, loadCmd()) when no dialog is open and no
// async action is mid-flight; otherwise drops the reload to protect
// the dialog's editIdx invariants. Shared by dayRefreshMsg and the
// cross-tab ChangedMsg.
func (h heute) reloadIfIdle() (tea.Model, tea.Cmd) {
	if h.dialog != heuteDialogNone || h.actionInFlight {
		return h, nil
	}
	cmds := []tea.Cmd{h.loadCmd()}
	if h.deps.ActiveSessions != nil && h.deps.UserID != "" {
		cmds = append(cmds, h.activeSessionsListCmd())
	}
	return h, tea.Batch(cmds...)
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

func (h heute) clampCursor() heute {
	total := len(h.day.Sessions)
	if h.cursor >= total {
		h.cursor = total - 1
	}
	if h.cursor < 0 {
		h.cursor = 0
	}
	return h
}

func (h heute) onSession() bool {
	return h.cursor >= 0 && h.cursor < len(h.day.Sessions)
}

// — keymap (no dialog) —

func (h heute) handleNormalKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
		return h.handleSKey()
	case "p":
		if h.day.IsRunning() {
			return h, h.pauseCmd()
		}
		return h, nil
	case "o":
		return h.openNoteViewDialog()
	case "O":
		return h, h.editAttachedNoteCmd()
	case "R":
		// `R` für Note-Detach. Vorher `ctrl+d` — das ist im Terminal die
		// EOF-/Process-Kill-Sequenz, was als Soft-Delete-Action irritiert.
		// `R` (uppercase Remove) reiht sich in die destructive-uppercase-
		// Konvention der App (D = delete session, K = krank …).
		return h, h.detachAttachedNoteCmd()
	case "?":
		// Standalone-`flow worktime today`: kein sidekick-Wrapper, der
		// `?` abfängt. Im sidekick-Modus kommt der Key gar nicht hier
		// an, Heute öffnet sein eigenes Overlay nur wenn die globale
		// Hilfe nicht greift.
		h.dialog = heuteDialogHelp
		return h, nil
	}
	return h.handleDialogOpenKey(msg)
}

// handleDialogOpenKey dispatches the keys that activate a dialog. Split
// from handleNormalKey to keep gocyclo under the project ceiling — the
// session-edit family (t/N/E/⏎/D) plus the day-level Kompendium attach
// (n) read more naturally as one group anyway.
func (h heute) handleDialogOpenKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
		// focused item — **always** y/N confirms".
		if h.onSession() {
			return h.openDeleteDialog()
		}
	case "n":
		return h.openNoteAttachDialog()
	}
	return h, nil
}
