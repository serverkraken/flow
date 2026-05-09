package worktime

// Heute (today) — model + Update-Routing + state accessors + keymap.
// Render-Logik in today_render.go, Dialog-Surfaces in today_dialog.go,
// async Action-Cmds in today_actions.go (Skill §No-Monoliths). Diese
// Datei behält den schmalen "wer-zappt-was"-Kern damit das Routing in
// einem Blick erfassbar bleibt.

import (
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/confirm"
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

	// noteSuggestions / noteSuggCur drive den Kompendium-Note-Attach-
	// Picker. Beim Öffnen des `n`-Dialogs einmal aus deps.NoteLister
	// geladen; Up/Down navigiert die nach Input gefilterte Liste; Enter
	// nimmt die gewählte Suggestion ODER (wenn Liste leer) den
	// getippten Raw-ID.
	noteSuggestions []NoteSuggestion
	noteSuggCur     int

	// noteView Felder treiben den integrierten Note-Viewer (`o`-Key,
	// dialog == heuteDialogNoteView). noteViewVP ist der Scroll-Container,
	// noteViewID die geladene Note für den Title, noteViewErr ein
	// Render-/Read-Fehler der inline statt als Toast gezeigt wird.
	noteViewVP    viewport.Model
	noteViewReady bool
	noteViewID    string
	noteViewErr   error
}

// noteAttachPickerLimit ist die maximale Anzahl jüngster Notes, die
// der Attach-Picker anbietet. Acht passt komfortabel unter den Input
// in einer Sidekick-Pane mit ~30 Zeilen.
const noteAttachPickerLimit = 8

func newHeute(p theme.Palette, deps Deps) heute {
	return heute{pal: p, deps: deps, editIdx: -1}
}

// FilterActive bubbles up to the root so global tab keys don't intercept
// while a dialog input is taking text.
func (h heute) FilterActive() bool { return h.dialog != heuteDialogNone }

// TextInputActive reports whether one of Heute's text-bearing dialogs
// (tag / note / edit form / kompendium-attach) is currently the focused
// surface. Lets the worktime root treat 'q' as a literal letter in
// those fields instead of an exit key. Confirm-delete and the help
// overlay are intentionally NOT text-input — q from there exits.
func (h heute) TextInputActive() bool {
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
		// ended elsewhere).
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
		if msg.toast != "" {
			var t toast.Model
			if msg.info {
				t = toast.NewInfo(msg.toast, h.pal)
			} else {
				t = toast.NewDefault(msg.toast, h.pal)
			}
			h.toast = &t
			return h, tea.Batch(h.loadCmd(), t.Init())
		}
		return h, h.loadCmd()

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
		// focused item — **always** y/N confirms".
		if h.onSession() {
			return h.openDeleteDialog()
		}
	case "n":
		return h.openNoteAttachDialog()
	}
	return h, nil
}
