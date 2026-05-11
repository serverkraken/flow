// Package browse implements the kompendium browse TUI.
//
// File-Layout (Skill §No-Monoliths):
//   - model.go    : Types, Konstruktion, State-Accessoren, Filter.label.
//   - update.go   : Update-Reducer + Mode-spezifische Key/Mouse-Handler.
//   - view.go     : View und Header-/Body-/Paginator-Rendering-Root.
//   - render_*.go : Row-, Status-, Modal-Renderer (vor M1 separated).
//   - preview.go  : Right-Pane-Preview + Full-Screen-Viewer-Aufmach-Logik.
//   - layout.go   : Pure Layout-Math (pane widths/heights, listRows).
//   - filter.go   : applyFilters / matchesFilter / matchesQuery.
//   - commands.go : tea.Cmd-Konstruktoren + zugehörige *Msg-Types +
//     runViaExecCapture.
//   - picker.go   : openWritePicker / runOnSelected.
//   - keymap.go   : Key-Bindings + defaultKeys().
//   - styles.go   : lipgloss-Style-Konstanten.
package browse

import (
	"os/exec"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/paginator"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	flowhelp "github.com/serverkraken/flow/internal/frontend/tui/components/help"
	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/frontend/tui/view"
	"github.com/serverkraken/flow/internal/kompendium/frontend/tui/writepicker"
	"github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

// Mode is the input mode of the browse view.
type Mode int

// Defined Mode values.
const (
	ModeNormal Mode = iota
	ModeSearch
	ModeConfirmDelete
	// ModeView swaps the entire browse render for the in-process
	// Markdown viewer (internal/frontend/tui/view). The viewer owns
	// the screen and emits view.ExitMsg on q/esc; the reducer
	// returns to ModeNormal then.
	ModeView
	// ModeWritePicker hosts the writepicker (Daily / Project / Free)
	// in-process. Pre-fix the picker ran as a subprocess via
	// `flow kompendium write` + tea.ExecProcess, but nested
	// tea.Programs through ExecProcess fail at /dev/tty negotiation
	// in bubbletea v1.3.x — the picker never appeared and the user
	// saw "Fehler beim Bearbeiten: exit status 1". The picker now
	// lives inside this program; only the resulting `kompendium new
	// <type>` (a non-bubbletea CLI) is forked, which behaves cleanly.
	ModeWritePicker
)

// Filter narrows the visible note list by type.
type Filter int

// Defined Filter values.
const (
	FilterAll Filter = iota
	FilterDaily
	FilterProject
	FilterFree
)

// label returns the human-readable filter label rendered in the status bar.
// Skill §German UI: User-facing-Strings auf Deutsch.
func (f Filter) label() string {
	switch f {
	case FilterDaily:
		return "Daily"
	case FilterProject:
		return "Projekt"
	case FilterFree:
		return "Frei"
	}
	return "Alle"
}

// CmdFunc returns an unstarted *exec.Cmd that takes over the terminal
// to edit (Enter) a note. The composition root builds it from
// nvimeditor.Editor.Cmd; tests inject a no-op binary like /bin/true.
// Read-only viewing (`v`) used to take a second CmdFunc that shelled
// out to glow — replaced by the in-process viewer in
// internal/frontend/tui/view, which keeps URLs OSC-8-clickable.
type CmdFunc func(path string) *exec.Cmd

// WriteCmdFunc returns an unstarted *exec.Cmd that creates the note
// the user picked: `flow kompendium new daily`, `… new project`, or
// `… new free <slug>`. Receives the picker's Result so the right
// concrete subcommand and slug can be assembled. Pre-fix this took no
// argument and always spawned `flow kompendium write` (which itself
// hosted the picker as a tea.Program); that nested-program shape
// failed under tea.ExecProcess and was replaced by the in-process
// ModeWritePicker plus this richer factory signature.
type WriteCmdFunc func(writepicker.Result) *exec.Cmd

// IndexAgeFunc returns the timestamp of the index's last on-disk write,
// used by the status bar to render "index Nm ago". A zero time hides the
// indicator. Composition root passes a closure that os.Stats the index
// file path; the model never reaches into the indexer port directly so
// the status bar stays a pure presentation concern.
type IndexAgeFunc func() time.Time

// BacklinksFunc returns the list of backlinks for one note id. Used by
// the full-screen viewer to show a "Referenced by" footer below the
// body. Composition root wires a closure backed by the
// RenderBacklinks use case so the model never reaches into the
// indexer port directly. Nil disables the footer.
type BacklinksFunc func(id domain.ID) []usecase.BacklinkRef

// twoPaneMinWidth is the threshold above which the preview pane appears to
// the right of the list. Below it, the list takes the full width. Tuned
// for a typical tmux sidekick split: panes < 90 cols stay single-pane,
// wider terminals get the live Glamour preview.
const twoPaneMinWidth = 90

// Model is the Bubble Tea state for the browse view.
type Model struct {
	list        *usecase.ListNotes
	store       ports.NoteStore
	delete      *usecase.DeleteNote
	currentRepo domain.CanonicalURL
	editCmd     CmdFunc
	writeCmd    WriteCmdFunc

	viewer view.Model
	picker writepicker.Model

	all     []ports.NoteEntry
	visible []ports.NoteEntry
	bodies  map[domain.ID][]byte
	cursor  int

	mode           Mode
	filter         Filter
	deleteTargetID domain.ID
	showHelp       bool

	search  textinput.Model
	spin    spinner.Model
	preview viewport.Model
	helpUI  help.Model
	pager   paginator.Model
	keys    keyMap

	previewID     domain.ID
	previewCached map[domain.ID]string

	loaded   bool
	loadErr  error
	editErr  error
	quitting bool
	width    int
	height   int

	indexAge    IndexAgeFunc
	backlinksFn BacklinksFunc
}

// WithIndexAge wires the optional indicator that backs the status bar's
// "index Nm" segment. Pass nil (or simply omit) to render the bar
// without it. Returns the modified model so it stays composable with
// the value-typed New() pattern.
func (m Model) WithIndexAge(fn IndexAgeFunc) Model {
	m.indexAge = fn
	return m
}

// WithBacklinks wires the closure the full-screen viewer consults
// when opening a note, to populate the "Referenced by" footer. Nil
// (or omit) keeps the footer off — useful for tests that don't
// stand up a real index.
func (m Model) WithBacklinks(fn BacklinksFunc) Model {
	m.backlinksFn = fn
	return m
}

// CurrentMode returns the model's input mode. Exposed for the
// external _test.go to assert ModeView is entered on `v` without
// needing access to the unexported field.
func (m Model) CurrentMode() Mode { return m.mode }

// HelpSections exposes the kompendium-browse key bindings to the
// sidekick `?`-overlay aggregator. The standalone overlay (kompendium
// browse `?`) renders its own bubbles/help-driven view from the
// internal keymap; this method mirrors that surface for the sidekick
// aggregation, which expects flow's components/help.Section shape.
func (Model) HelpSections() []flowhelp.Section {
	return []flowhelp.Section{{
		Title: "Notes (Kompendium)",
		Keys: [][2]string{
			{"j / k · ↑ / ↓", "Eintrag fokussieren"},
			{"Enter", "Note öffnen / View-Modus"},
			{"/", "Suche öffnen"},
			{"Esc", "Suche schließen / View verlassen"},
			{"o", "Note in Editor öffnen ($EDITOR)"},
			{"r", "Liste neu laden"},
			{"?", "Standalone-Hilfe (im Browser-TUI)"},
		},
	}}
}

// New returns a Model wired with the list use case, the note store (for
// body loading + path resolution), an optional delete use case (D + y/N
// confirm), the currently detected repo URL (empty when not in a repo),
// and the editor / write-picker command builders. Read-only viewing
// (`v`) is handled in-process by internal/frontend/tui/view, no
// CmdFunc needed.
func New(
	list *usecase.ListNotes,
	store ports.NoteStore,
	deleteUC *usecase.DeleteNote,
	currentRepo domain.CanonicalURL,
	editCmd CmdFunc,
	writeCmd WriteCmdFunc,
) Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 256
	ti.Cursor.Style = cursorStyle

	sp := spinner.New()
	sp.Spinner = spinner.Points
	sp.Style = spinnerStyle

	vp := viewport.New(0, 0)

	h := help.New()
	h.ShowAll = false
	h.Styles.ShortKey = statusKeyStyle
	h.Styles.ShortDesc = statusValueStyle
	h.Styles.ShortSeparator = statusLineStyle
	h.Styles.FullKey = statusKeyStyle
	h.Styles.FullDesc = statusValueStyle
	h.Styles.FullSeparator = statusLineStyle

	pg := paginator.New()
	pg.Type = paginator.Dots
	pg.ActiveDot = paginatorActiveDotStyle.Render("●")
	pg.InactiveDot = paginatorInactiveDotStyle.Render("○")

	return Model{
		list:          list,
		store:         store,
		delete:        deleteUC,
		currentRepo:   currentRepo,
		editCmd:       editCmd,
		writeCmd:      writeCmd,
		search:        ti,
		spin:          sp,
		preview:       vp,
		helpUI:        h,
		pager:         pg,
		keys:          defaultKeys(),
		previewCached: map[domain.ID]string{},
	}
}

// Init schedules the initial entry load. Spinner ticks are kicked off
// from the first non-load message (typically tea.WindowSizeMsg) so test
// drivers that call `m.Init()()` once still receive an entriesLoadedMsg.
func (m Model) Init() tea.Cmd {
	return loadEntriesCmd(m.list, m.currentRepo)
}
