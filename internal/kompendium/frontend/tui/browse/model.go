package browse

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/paginator"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/serverkraken/flow/internal/frontend/tui/components/modal"
	"github.com/serverkraken/flow/internal/frontend/tui/markdown"
	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/frontend/tui/view"
	"github.com/serverkraken/flow/internal/kompendium/frontend/tui/writepicker"
	"github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
	flowports "github.com/serverkraken/flow/internal/ports"
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

type entriesLoadedMsg struct {
	entries []ports.NoteEntry
	err     error
}

func loadEntriesCmd(u *usecase.ListNotes, currentRepo domain.CanonicalURL) tea.Cmd {
	return func() tea.Msg {
		entries, err := u.Execute(context.Background(), usecase.ListNotesInput{
			CurrentRepo: currentRepo,
		})
		return entriesLoadedMsg{entries: entries, err: err}
	}
}

// editFinishedMsg lands when tea.ExecProcess returns from the editor.
type editFinishedMsg struct{ err error }

// bodiesLoadedMsg lands once the background goroutine has read every
// note's body so search can match against content, not only frontmatter.
type bodiesLoadedMsg struct{ bodies map[domain.ID][]byte }

// bodyExcerptLimit caps how much of each note body the loader keeps in
// memory. The map exists to back the row excerpt + the body-search
// substring match — both only need the start of the file. Holding full
// bodies for every note OOM-killed kompendium on real notebooks with
// large Markdown files; a few KB per entry is plenty for the use cases
// here. The preview pane reloads the full body on demand from the
// store, so opening a long note still renders end-to-end.
const bodyExcerptLimit = 8 * 1024

func loadBodiesCmd(store ports.NoteStore, entries []ports.NoteEntry) tea.Cmd {
	return func() tea.Msg {
		bodies := make(map[domain.ID][]byte, len(entries))
		for _, e := range entries {
			note, err := store.Get(context.Background(), e.ID)
			if err != nil {
				continue
			}
			body := note.Body
			if len(body) > bodyExcerptLimit {
				clipped := make([]byte, bodyExcerptLimit)
				copy(clipped, body[:bodyExcerptLimit])
				body = clipped
			}
			bodies[e.ID] = body
		}
		return bodiesLoadedMsg{bodies: bodies}
	}
}

// deleteFinishedMsg lands once the delete use case returns.
type deleteFinishedMsg struct{ err error }

func deleteCmd(u *usecase.DeleteNote, id domain.ID) tea.Cmd {
	return func() tea.Msg {
		return deleteFinishedMsg{err: u.Execute(context.Background(), id)}
	}
}

// Update is the Bubble Tea reducer.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.mode == ModeView {
		return m.updateViewer(msg)
	}
	if m.mode == ModeWritePicker {
		return m.updatePicker(msg)
	}
	switch msg := msg.(type) {
	case entriesLoadedMsg:
		m.all = msg.entries
		m.loadErr = msg.err
		m.loaded = true
		m.applyFilters()
		m.refreshPreview()
		if m.store != nil && len(m.all) > 0 {
			return m, loadBodiesCmd(m.store, m.all)
		}
		return m, nil
	case bodiesLoadedMsg:
		m.bodies = msg.bodies
		m.applyFilters()
		m.refreshPreview()
		return m, nil
	case editFinishedMsg:
		if msg.err != nil {
			m.editErr = msg.err
			return m, nil
		}
		m.editErr = nil
		// Drop the rendered-preview cache + previewID so the next
		// refreshPreview (triggered by entriesLoadedMsg below) re-
		// reads the just-saved file from the store. Without this,
		// `if m.previewID == e.ID { return }` short-circuits and the
		// user keeps seeing the pre-edit body until they cursor away
		// and back.
		m.previewCached = map[domain.ID]string{}
		m.previewID = ""
		return m, loadEntriesCmd(m.list, m.currentRepo)
	case deleteFinishedMsg:
		if msg.err != nil {
			m.editErr = msg.err
			return m, nil
		}
		m.editErr = nil
		return m, loadEntriesCmd(m.list, m.currentRepo)
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.helpUI.Width = m.width
		m.layoutViewport()
		// Invalidate the cached preview AND drop previewID so
		// refreshPreview's `if previewID == e.ID { return }` short-
		// circuit doesn't keep the old-width rendering on screen
		// after a tmux pane resize / window resize.
		m.previewCached = map[domain.ID]string{}
		m.previewID = ""
		m.refreshPreview()
		if !m.loaded {
			return m, m.spin.Tick
		}
		return m, nil
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tea.KeyMsg:
		return m.handleKey(msg)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case ModeSearch:
		return m.handleSearchKey(msg)
	case ModeConfirmDelete:
		return m.handleConfirmDeleteKey(msg)
	}
	return m.handleNormalKey(msg)
}

// updateViewer routes every message to the active viewer sub-model
// while in ModeView. The window-size message is intercepted so the
// list pane has fresh dimensions when the viewer exits, and ExitMsg
// returns the model to ModeNormal.
func (m Model) updateViewer(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.helpUI.Width = m.width
		m.layoutViewport()
		m.previewCached = map[domain.ID]string{}
		m.previewID = ""
		m.refreshPreview()
		m.viewer = m.viewer.SetSize(m.width, m.height)
		return m, nil
	case view.ExitMsg:
		m.mode = ModeNormal
		m.viewer = view.Model{}
		return m, nil
	}
	var cmd tea.Cmd
	m.viewer, cmd = m.viewer.Update(msg)
	return m, cmd
}

func (m Model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.showHelp {
		if key.Matches(msg, m.keys.Help) || key.Matches(msg, m.keys.Quit) || msg.Type == tea.KeyEsc {
			m.showHelp = false
		}
		return m, nil
	}
	if model, cmd, handled := m.handleNavKey(msg); handled {
		return model, cmd
	}
	if model, cmd, handled := m.handleActionKey(msg); handled {
		return model, cmd
	}
	return m, nil
}

// handleNavKey handles cursor movement keys. Returns handled=true when one
// of the nav bindings matched so the caller doesn't fall through.
func (m Model) handleNavKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.visible)-1 {
			m.cursor++
			m.refreshPreview()
		}
	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
			m.refreshPreview()
		}
	case key.Matches(msg, m.keys.Top):
		m.cursor = 0
		m.refreshPreview()
	case key.Matches(msg, m.keys.Bottom):
		if len(m.visible) > 0 {
			m.cursor = len(m.visible) - 1
			m.refreshPreview()
		}
	case key.Matches(msg, m.keys.PageUp):
		m.cursor = max(0, m.cursor-m.pageJump())
		m.refreshPreview()
	case key.Matches(msg, m.keys.PageDown):
		m.cursor = min(len(m.visible)-1, m.cursor+m.pageJump())
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.refreshPreview()
	default:
		return m, nil, false
	}
	return m, nil, true
}

// handleActionKey handles non-navigation bindings (filter, search, edit,
// view, new, delete, help, quit).
func (m Model) handleActionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		m.quitting = true
		return m, tea.Quit, true
	case key.Matches(msg, m.keys.Filter):
		m.filter = (m.filter + 1) % 4
		m.applyFilters()
		m.refreshPreview()
		return m, nil, true
	case key.Matches(msg, m.keys.Search):
		m.mode = ModeSearch
		m.search.Focus()
		return m, textinput.Blink, true
	case key.Matches(msg, m.keys.Edit):
		model, cmd := m.runOnSelected(m.editCmd)
		return model, cmd, true
	case key.Matches(msg, m.keys.View):
		return m.openViewer(), nil, true
	case key.Matches(msg, m.keys.New):
		model, cmd := m.openWritePicker()
		return model, cmd, true
	case key.Matches(msg, m.keys.Delete):
		return m.startConfirmDelete(), nil, true
	case key.Matches(msg, m.keys.Help):
		m.showHelp = true
		return m, nil, true
	}
	return m, nil, false
}

func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = ModeNormal
		m.search.SetValue("")
		m.search.Blur()
		m.applyFilters()
		m.refreshPreview()
		return m, nil
	case tea.KeyEnter:
		m.mode = ModeNormal
		m.search.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	prev := m.search.Value()
	m.search, cmd = m.search.Update(msg)
	if m.search.Value() != prev {
		m.applyFilters()
		m.refreshPreview()
	}
	return m, cmd
}

// startConfirmDelete switches into ModeConfirmDelete and stashes the
// cursor's note ID. No-op when the cursor is on no entry or the delete
// use case wasn't wired (e.g. tests passing nil).
func (m Model) startConfirmDelete() Model {
	if m.delete == nil {
		return m
	}
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return m
	}
	m.deleteTargetID = m.visible[m.cursor].ID
	m.mode = ModeConfirmDelete
	return m
}

// handleConfirmDeleteKey — kanonisches y/Enter → ja, n/Esc → nein
// (Skill §Keybind grammar). Vorher fehlte Enter als Confirm-Variante,
// was die Konvention der restlichen Codebase (confirm.Model) uneinheitlich
// machte.
func (m Model) handleConfirmDeleteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		id := m.deleteTargetID
		m.mode = ModeNormal
		m.deleteTargetID = ""
		return m, deleteCmd(m.delete, id)
	case "n", "N", "esc", "ctrl+c":
		m.mode = ModeNormal
		m.deleteTargetID = ""
	}
	return m, nil
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if m.cursor > 0 {
			m.cursor--
			m.refreshPreview()
		}
	case tea.MouseButtonWheelDown:
		if m.cursor < len(m.visible)-1 {
			m.cursor++
			m.refreshPreview()
		}
	}
	return m, nil
}

// openWritePicker enters ModeWritePicker with a freshly built picker.
// Project is always offered — even when currentRepo is empty (cwd is
// not a repo, or is a repo without an `origin` remote). The actual
// `flow kompendium new project` invocation surfaces wrapProjectErr's
// hint (»cd into a repository«, »project notes need an origin
// remote«) via the runViaExecCapture stderr-passthrough — which is
// more discoverable than silently hiding the option from the menu.
//
// Pre-refactor only currentRepo!="" enabled Project, so users in a
// repo without origin (kompendium notebooks under ~/notes don't need
// one) saw a 2-option picker and had no signal why Project was
// missing. The hint-on-attempt UX is the better trade.
func (m Model) openWritePicker() (tea.Model, tea.Cmd) {
	m.picker = writepicker.New(true)
	m.mode = ModeWritePicker
	m.editErr = nil
	return m, m.picker.Init()
}

// runViaExecCapture wraps tea.ExecProcess with a stderr-capturing
// MultiWriter so that when the spawned process exits non-zero, the
// editFinishedMsg carries cobra's actual "Error: ..." line in
// addition to the bare exit code. Without this the alt-screen redraw
// after tea.ExecProcess wipes whatever the subprocess printed to
// stderr, leaving browse with only `*exec.ExitError`'s short
// "exit status N" — no actionable signal for the user.
//
// Stdout is left untouched (nvim and CreateX printCreateOutput need
// it to take over the TTY) and stderr keeps streaming to the user's
// terminal too — the captured copy is purely additive.
func runViaExecCapture(cmd *exec.Cmd) tea.Cmd {
	var errBuf bytes.Buffer
	if cmd.Stderr != nil {
		cmd.Stderr = io.MultiWriter(cmd.Stderr, &errBuf)
	} else {
		cmd.Stderr = &errBuf
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			if captured := strings.TrimSpace(errBuf.String()); captured != "" {
				err = fmt.Errorf("%w — %s", err, captured)
			}
		}
		return editFinishedMsg{err: err}
	})
}

// updatePicker is the reducer-branch active while ModeWritePicker is
// the input mode. The picker emits writepicker.DoneMsg when the user
// either selects a type (with optional slug) or cancels; we harvest
// the Result, return to ModeNormal, and — when the choice was not
// Cancel — fork the corresponding `flow kompendium new <type>`
// subcommand via tea.ExecProcess. That subcommand is a plain CLI
// (creates the file, opens nvim), not another tea.Program, so the
// nested-tea problem that motivated this whole refactor doesn't
// recur.
func (m Model) updatePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.helpUI.Width = m.width
		next, cmd := m.picker.Update(msg)
		m.picker = next.(writepicker.Model)
		return m, cmd
	case writepicker.DoneMsg:
		m.mode = ModeNormal
		m.picker = writepicker.Model{}
		if msg.Result.Choice == writepicker.ChoiceCancel || m.writeCmd == nil {
			return m, nil
		}
		cmd := m.writeCmd(msg.Result)
		if cmd == nil {
			return m, nil
		}
		return m, runViaExecCapture(cmd)
	}
	next, cmd := m.picker.Update(msg)
	m.picker = next.(writepicker.Model)
	return m, cmd
}

// runOnSelected resolves the cursor's note ID to a path, builds the
// edit command, and hands control to tea.ExecProcess.
func (m Model) runOnSelected(builder CmdFunc) (tea.Model, tea.Cmd) {
	if m.store == nil || builder == nil {
		return m, nil
	}
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return m, nil
	}
	id := m.visible[m.cursor].ID
	cmd := builder(m.store.Path(id))
	return m, runViaExecCapture(cmd)
}

// openViewer constructs a fresh in-process Markdown viewer for the
// cursor's note and switches to ModeView. The viewer renders the
// already-loaded entry's metadata as a Markdown header followed by
// the on-disk body — same pipeline the preview pane uses, just at
// full screen. Body fetch failures land in m.editErr and stay in
// ModeNormal.
func (m Model) openViewer() Model {
	if m.store == nil {
		return m
	}
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return m
	}
	e := m.visible[m.cursor]
	note, err := m.store.Get(context.Background(), e.ID)
	if err != nil {
		m.editErr = fmt.Errorf("open viewer: %w", err)
		return m
	}
	title := e.Meta.Title
	if title == "" {
		title = e.ID.String()
	}
	source := buildViewerSource(e, note.Body)
	meta := e.Meta
	var backlinks []usecase.BacklinkRef
	if m.backlinksFn != nil {
		backlinks = m.backlinksFn(e.ID)
	}
	v := view.New(title, source, m.wikilinkResolver(), &meta, backlinks)
	if m.width > 0 && m.height > 0 {
		v = v.SetSize(m.width, m.height)
	}
	m.viewer = v
	m.mode = ModeView
	return m
}

// buildViewerSource returns the body the viewer renders. The header
// (title + metadata) is no longer prepended as Markdown — the
// renderer's frontmatter card handles it via WithFrontmatter, which
// view.New receives separately.
func buildViewerSource(_ ports.NoteEntry, body []byte) string {
	if len(body) == 0 {
		return "*Inhalt noch nicht geladen.*"
	}
	return string(body)
}

// applyFilters recomputes the visible slice from m.all under the current
// filter + search query, clamping the cursor into the new range.
func (m *Model) applyFilters() {
	q := strings.ToLower(m.search.Value())
	m.visible = m.visible[:0]
	for _, e := range m.all {
		if !matchesFilter(e, m.filter) {
			continue
		}
		if !m.matchesQuery(e, q) {
			continue
		}
		m.visible = append(m.visible, e)
	}
	if m.cursor >= len(m.visible) {
		m.cursor = max(0, len(m.visible)-1)
	}
}

func matchesFilter(e ports.NoteEntry, f Filter) bool {
	switch f {
	case FilterDaily:
		return e.Meta.Type == domain.TypeDaily
	case FilterProject:
		return e.Meta.Type == domain.TypeProject
	case FilterFree:
		return e.Meta.Type == domain.TypeFree
	}
	return true
}

// matchesQuery searches the note body first (the user's intent) and then
// the title + project. The auto-generated ID/path is intentionally skipped
// — those are scheme-derived noise that drowns real matches.
func (m *Model) matchesQuery(e ports.NoteEntry, q string) bool {
	if q == "" {
		return true
	}
	if body, ok := m.bodies[e.ID]; ok {
		if strings.Contains(strings.ToLower(string(body)), q) {
			return true
		}
	}
	for _, h := range []string{
		strings.ToLower(e.Meta.Title),
		strings.ToLower(e.Meta.Project),
	} {
		if h != "" && strings.Contains(h, q) {
			return true
		}
	}
	return false
}

// listRows returns how many terminal lines the list pane has to work
// with. Chrome budget: outer rounded frame (2), three header lines
// (headline + separator + status) plus a blank, list panel title +
// blank, blank + footer + status bar, plus one reserved line for the
// paginator dots (always reserved so layout doesn't shift when the
// list crosses the dot threshold). Conservative — undercounting just
// trims a row, overcounting clips chrome, which is louder.
func (m Model) listRows() int {
	rows := m.height - 12
	if rows < 5 {
		return 5
	}
	return rows
}

// pageJump is the cursor delta for PageUp/PageDown. Entries can be 1–4
// rendered lines tall, so a fixed jump-by-N-entries either flies past
// the screen on dense lists or barely scrolls on sparse ones. Halving
// listRows() and clamping to a sane minimum keeps the jump close to "a
// screen of content" without re-rendering all rows just to count.
func (m Model) pageJump() int {
	jump := m.listRows() / 2
	if jump < 3 {
		return 3
	}
	return jump
}

// layoutViewport sets the preview viewport's dimensions from the current
// window size.
func (m *Model) layoutViewport() {
	w, h := m.previewSize()
	m.preview.Width = max(0, w)
	m.preview.Height = max(0, h)
}

// previewPaneWidth is the OUTER width of the preview pane — including
// the panel's NormalBorder. Zero when there's no preview pane.
func (m Model) previewPaneWidth() int {
	if !m.twoPane() {
		return 0
	}
	w := m.contentWidth() - m.listPaneWidth() - 2 // 2 = gap between panes
	if w < 0 {
		w = 0
	}
	return w
}

// previewSize is the INNER content area Glamour wraps to and the
// viewport renders into. We subtract two each from width and from
// height to reserve the panel's NormalBorder (left+right, top+bottom)
// — without that the Glamour-rendered lines were exactly two cells
// wider than the panel interior, lipgloss soft-wrapped each line into
// two, and a long Markdown body blew the body up to twice its planned
// height. The vertical budget then mirrors the list pane so
// JoinHorizontal stacks both at the same height.
func (m Model) previewSize() (int, int) {
	paneW := m.previewPaneWidth()
	if paneW <= 0 {
		return 0, 0
	}
	innerW := paneW - 2
	if innerW < 1 {
		innerW = 1
	}
	innerH := m.contentHeight() - 8
	if innerH < 1 {
		innerH = 1
	}
	return innerW, innerH
}

func (m Model) twoPane() bool {
	return m.width >= twoPaneMinWidth && m.height >= 18
}

// contentWidth is the width inside the outer rounded frame.
func (m Model) contentWidth() int {
	if m.width <= 4 {
		return 0
	}
	return m.width - 4 // 2 border + 2 padding
}

func (m Model) contentHeight() int {
	if m.height <= 4 {
		return 0
	}
	return m.height - 4
}

func (m Model) listPaneWidth() int {
	if !m.twoPane() {
		return m.contentWidth()
	}
	w := m.contentWidth() * 4 / 10
	if w < 30 {
		w = 30
	}
	return w
}

// refreshPreview updates the previewed note + viewport content based on
// the current cursor.
func (m *Model) refreshPreview() {
	if !m.twoPane() {
		m.preview.SetContent("")
		m.previewID = ""
		return
	}
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		m.preview.SetContent(dimStyle.Render("(keine Notiz ausgewählt)"))
		m.previewID = ""
		return
	}
	e := m.visible[m.cursor]
	if m.previewID == e.ID {
		// Already rendered; let the viewport keep its scroll position.
		return
	}
	m.previewID = e.ID
	rendered := m.renderPreviewBody(e)
	m.preview.SetContent(rendered)
	m.preview.GotoTop()
}

// renderPreviewBody builds the preview content for the given entry.
// Glamour does the heavy lifting for the body; the title/metadata header
// is rendered with our own styles so it stays visually consistent with
// the list pane regardless of which Markdown style is active.
//
// The body comes from the store — NOT from m.bodies — because that map
// only carries an 8 KB excerpt of each note (the loader caps it to
// avoid OOM-killing kompendium on notebooks with huge Markdown files).
// The render result is memoised in m.previewCached, so re-rendering
// from disk happens at most once per entry per layout.
func (m *Model) renderPreviewBody(e ports.NoteEntry) string {
	width, _ := m.previewSize()
	if width <= 0 {
		return ""
	}
	if cached, ok := m.previewCached[e.ID]; ok {
		return cached
	}

	var body []byte
	hasBody := false
	if m.store != nil {
		if note, err := m.store.Get(context.Background(), e.ID); err == nil {
			body = note.Body
			hasBody = true
		}
	}

	source := ""
	if hasBody {
		source = string(body)
	} else {
		source = "*No body cached yet.*"
	}

	meta := e.Meta
	rendered, err := markdown.Render(source, width,
		markdown.WithWikilinks(m.wikilinkResolver()),
		markdown.WithFrontmatter(frontmatterToMarkdown(&meta)),
	)
	if err != nil {
		rendered = source
	}
	m.previewCached[e.ID] = rendered
	return rendered
}

// wikilinkResolver builds a flowports.WikilinkResolver that consults
// the browse model's loaded entries. Used by both renderPreviewBody
// and the full-screen viewer (browse hands the same resolver to
// view.New when ModeView starts) so wikilink resolution is consistent
// across surfaces.
func (m Model) wikilinkResolver() flowports.WikilinkResolver {
	idx := make(map[domain.ID]ports.NoteEntry, len(m.all))
	for _, e := range m.all {
		idx[e.ID] = e
	}
	return browseResolver{entries: idx}
}

// browseResolver looks up wikilink targets in the loaded NoteEntry
// map. Returns kompendium://note/<id> for valid hits, ok=false for
// misses (renderer styles those as broken).
type browseResolver struct {
	entries map[domain.ID]ports.NoteEntry
}

func (r browseResolver) Resolve(target string) (uri, title string, ok bool) {
	e, found := r.entries[domain.ID(target)]
	if !found {
		return "", "", false
	}
	return "kompendium://note/" + target, e.Meta.Title, true
}

// View renders the current model as a string.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.mode == ModeView {
		return m.viewer.View()
	}
	if m.mode == ModeWritePicker {
		// Picker manages its own width/height + center placement, so it
		// gets the full screen as a passthrough — no frameContent wrap
		// (which would double-border it).
		return m.picker.View()
	}
	if m.loadErr != nil {
		return frameContent(m.width, m.height, errorStyle.Render("Fehler: "+m.loadErr.Error()))
	}
	if !m.loaded {
		loading := lipgloss.JoinHorizontal(lipgloss.Center, m.spin.View(), " ", dimStyle.Render("lädt…"))
		return frameContent(m.width, m.height, loading)
	}

	header := m.renderHeader()
	body := m.renderBody()
	footer := m.renderFooter()
	statusBar := m.renderStatusBar()

	content := lipgloss.JoinVertical(lipgloss.Left, header, "", body, "", footer, statusBar)
	base := frameContent(m.width, m.height, content)

	switch {
	case m.showHelp:
		return overlay(base, m.renderHelpOverlay(), m.width, m.height)
	case m.mode == ModeConfirmDelete:
		return overlay(base, m.renderDeleteModal(), m.width, m.height)
	}
	return base
}

// renderHeader is the top status block: kompendium counts (with per-type
// breakdown and repo context), separator, filter, search, and any inline
// error. The headline is rendered as a single styled string (no
// padding-injecting badges) so test asserts on the contiguous
// "kompendium — N/M notes" substring still match.
func (m Model) renderHeader() string {
	headline := headlineStyle.Render(
		fmt.Sprintf("kompendium — %d/%d Notizen", len(m.visible), len(m.all)),
	)

	headlineRow := []string{headline}
	if breakdown := m.renderTypeCounts(); breakdown != "" {
		headlineRow = append(headlineRow, statusLineStyle.Render("  ·  "), breakdown)
	}
	if repo := m.renderRepoChip(); repo != "" {
		headlineRow = append(headlineRow, "  ", repo)
	}
	headlineLine := lipgloss.JoinHorizontal(lipgloss.Top, headlineRow...)

	separator := m.renderSeparator()

	statusLine := m.renderStatusLine()

	headerBlock := lipgloss.JoinVertical(lipgloss.Left, headlineLine, separator, statusLine)
	if m.editErr != nil {
		headerBlock = lipgloss.JoinVertical(lipgloss.Left, headerBlock,
			errorStyle.Render("Fehler beim Bearbeiten: "+m.editErr.Error()))
	}
	return headerBlock
}

// renderTypeCounts emits the per-type breakdown shown next to the
// headline (e.g. "●3 daily  ●1 proj  ●0 free"). Counts reflect the
// currently visible (filtered + searched) set so the user can see what
// just dropped out of view.
func (m Model) renderTypeCounts() string {
	if len(m.all) == 0 {
		return ""
	}
	var d, p, f int
	for _, e := range m.visible {
		switch e.Meta.Type {
		case domain.TypeDaily:
			d++
		case domain.TypeProject:
			p++
		case domain.TypeFree:
			f++
		}
	}
	parts := []string{
		countDailyStyle.Render(fmt.Sprintf("● %d", d)) + dimStyle.Render(" daily"),
		countProjectStyle.Render(fmt.Sprintf("● %d", p)) + dimStyle.Render(" projekt"),
		countFreeStyle.Render(fmt.Sprintf("● %d", f)) + dimStyle.Render(" frei"),
	}
	return strings.Join(parts, "  ")
}

// renderRepoChip shows the current repo as a small pill when running
// inside a project. Empty when not in a repo.
func (m Model) renderRepoChip() string {
	if m.currentRepo == "" {
		return ""
	}
	return repoChipStyle.Render(shortProjectLabel(string(m.currentRepo)))
}

// renderSeparator draws a soft horizontal rule under the headline. Width
// is the inner content width — falls back to a sane minimum when the
// model hasn't received its first WindowSizeMsg yet.
func (m Model) renderSeparator() string {
	w := m.contentWidth()
	if w <= 0 {
		w = 60
	}
	return headerSeparatorStyle.Render(strings.Repeat("─", w))
}

// renderStatusLine renders the second header row: filter label and the
// (optionally bordered) search bar.
func (m Model) renderStatusLine() string {
	parts := []string{
		statusKeyStyle.Render("Filter:") + " " + statusValueStyle.Render(m.filter.label()),
	}
	if search := m.renderSearchSegment(); search != "" {
		parts = append(parts, search)
	}
	return strings.Join(parts, statusLineStyle.Render("  ·  "))
}

// renderSearchSegment is the inline search affordance: nothing when
// neither active nor populated, a passive label when the user has a
// stashed query, a yellow label + raw textinput view when in ModeSearch.
//
// The textinput view already carries its own ANSI sequences (cursor in
// reverse + colored), so we MUST NOT pipe it through another lipgloss
// style with Bold/Underline — lipgloss's ansi wrapper mangles nested
// sequences and the raw escape codes leak as visible text. Yellow label
// alone is the focus cue.
func (m Model) renderSearchSegment() string {
	if m.mode == ModeSearch {
		view := m.search.View()
		if view == "" {
			view = "▎"
		}
		return searchActiveLabelStyle.Render("Suche:") + " " + view
	}
	if v := m.search.Value(); v != "" {
		return searchPassiveLabelStyle.Render("Suche:") + " " + searchValueStyle.Render(v)
	}
	return ""
}

// renderBody returns the list pane (and preview pane in two-pane layout).
func (m Model) renderBody() string {
	listPane := m.renderListPane()
	if !m.twoPane() {
		return listPane
	}
	previewPane := m.renderPreviewPane()
	return lipgloss.JoinHorizontal(lipgloss.Top, listPane, "  ", previewPane)
}

// renderListPane returns the list block. There is intentionally no inner
// border — the outer rounded frame already provides a chrome, and a
// second nested border looked broken whenever a row overflowed and the
// border characters fell out of alignment.
//
// The paginator slot is *always* reserved (one line, blank when there
// are too few entries to need dots) so the pane's overall height stays
// constant across short and long lists. Variable chrome would push the
// total content past the frame's interior height and corrupt the
// bottom border.
func (m Model) renderListPane() string {
	w := m.listPaneWidth()
	header := panelTitleStyle.Render("notizen")
	if len(m.visible) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header, "", m.renderEmptyState(w), "")
	}
	rows := m.renderListRows(w)
	return lipgloss.JoinVertical(lipgloss.Left, header, "", rows, m.renderPaginator())
}

// renderPaginator returns a dotted page indicator plus a "X/N" counter
// for the cursor's position. Empty when there are five or fewer entries
// — at that scale the dots are noise and the entire list is visible
// anyway. PerPage is rounded so we never render more than ~12 dots,
// otherwise long notebooks turn the indicator into a wall.
func (m Model) renderPaginator() string {
	const maxDots = 12
	const minForDots = 6
	if len(m.visible) < minForDots {
		return ""
	}
	perPage := (len(m.visible) + maxDots - 1) / maxDots
	if perPage < 1 {
		perPage = 1
	}
	p := m.pager
	p.PerPage = perPage
	p.SetTotalPages(len(m.visible))
	p.Page = m.cursor / perPage
	dots := p.View()
	counter := paginatorCounterStyle.Render(fmt.Sprintf("%d/%d", m.cursor+1, len(m.visible)))
	return dots + "  " + counter
}

// renderListRows renders the visible window of entries. Entries can be
// 1–4 terminal lines tall (title, up to 2 excerpt lines, optional tag
// line), so the window is sized by total rendered height instead of a
// fixed entry count: rows render once, heights are measured, and
// computeListWindow walks heights around the cursor until the budget is
// filled.
func (m Model) renderListRows(rowWidth int) string {
	rendered := make([]string, len(m.visible))
	heights := make([]int, len(m.visible))
	for i := range m.visible {
		rendered[i] = m.renderRow(i, m.visible[i], rowWidth)
		heights[i] = lipgloss.Height(rendered[i])
	}
	budget := m.listRows()
	if budget < 1 {
		budget = 1
	}
	start, end := computeListWindow(heights, m.cursor, budget)

	rows := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		rows = append(rows, rendered[i])
	}
	return strings.Join(rows, "\n")
}

// computeListWindow picks the [start, end) slice of entries to render so
// the cursor row is included and the total stacked height stays within
// budget. It grows forward from the cursor first, then backfills upward,
// so the cursor stays anchored at the top half of the view — predictable
// behavior when scrolling through a long list.
func computeListWindow(heights []int, cursor, budget int) (int, int) {
	n := len(heights)
	if n == 0 {
		return 0, 0
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= n {
		cursor = n - 1
	}
	used := heights[cursor]
	end := cursor + 1
	for end < n && used+heights[end] <= budget {
		used += heights[end]
		end++
	}
	start := cursor
	for start > 0 && used+heights[start-1] <= budget {
		start--
		used += heights[start]
	}
	return start, end
}

func (m Model) renderRow(idx int, e ports.NoteEntry, rowWidth int) string {
	selected := idx == m.cursor
	stripe, caret := rowStripeAndCaret(selected)
	dateRendered, todayMark := renderDateCell(e)
	badge := badgeFor(e.Meta.Type)
	title := m.titleForRow(e, stripe, caret, todayMark, dateRendered, badge, rowWidth, selected)

	mainLine := lipgloss.JoinHorizontal(lipgloss.Top,
		stripe, caret, todayMark, dateRendered, "  ", badge, "  ", title)

	hang := rowHangPrefix(selected)
	excerptLines := m.excerptParagraph(e, rowWidth-lipgloss.Width(hang)-1, 2)
	tagLine := m.renderTags(e.Meta.Tags)
	if len(excerptLines) == 0 && tagLine == "" {
		return mainLine
	}

	q := strings.ToLower(m.search.Value())
	lines := []string{mainLine}
	for _, el := range excerptLines {
		lines = append(lines, hang+styleExcerptLine(el, q))
	}
	if tagLine != "" {
		lines = append(lines, hang+tagLine)
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// rowStripeAndCaret returns the two two-cell prefix columns at the row's
// left edge: a vertical stripe and a cursor caret. Both stay constant
// width so excerpt + tag lines hang under the title cleanly, and the
// test's `▶`-on-the-line check still holds.
func rowStripeAndCaret(selected bool) (string, string) {
	if selected {
		return cursorStripeStyle.Render("▎ "), cursorStyle.Render("▶ ")
	}
	return "  ", "  "
}

// renderDateCell formats the row's date column and returns the colored
// date plus the today-marker glyph (★ when today, blank otherwise).
func renderDateCell(e ports.NoteEntry) (string, string) {
	date := e.Meta.Date
	if date == "" {
		date = e.Mtime.Format("2006-01-02")
	}
	today := isToday(date)
	dateRendered := dateStyle.Render(date)
	if today {
		dateRendered = todayDateStyle.Render(date)
	}
	if today {
		return dateRendered, todayMarkerStyle.Render("★ ")
	}
	return dateRendered, "  "
}

// titleForRow truncates the title to whatever fits in the row and renders
// it with the right base style (selected rows get bold), running the
// search-match highlight on top.
func (m Model) titleForRow(e ports.NoteEntry, stripe, caret, todayMark, dateRendered, badge string, rowWidth int, selected bool) string {
	title := smartTitle(e)
	if rowWidth > 0 {
		prefixW := lipgloss.Width(stripe) + lipgloss.Width(caret) + lipgloss.Width(todayMark) +
			lipgloss.Width(dateRendered) + lipgloss.Width(badge) + 4
		avail := rowWidth - prefixW - 1
		if avail < 8 {
			avail = 8
		}
		if lipgloss.Width(title) > avail {
			title = truncateText(title, avail)
		}
	}
	base := titleStyle
	if selected {
		base = selectedTitleStyle
	}
	return highlightMatch(title, strings.ToLower(m.search.Value()), base, matchStyle)
}

// rowHangPrefix is the soft indent under the title used for excerpt and
// tag lines. Selected rows get the stripe so the whole entry reads as
// one block; non-selected rows just indent.
func rowHangPrefix(selected bool) string {
	if selected {
		return cursorStripeStyle.Render("▎ ") + "      "
	}
	return "        "
}

// styleExcerptLine renders one wrapped excerpt line with the muted
// excerpt style, layering the search-match highlight on top when the
// user has a query.
func styleExcerptLine(line, q string) string {
	if q == "" {
		return excerptStyle.Render(line)
	}
	return highlightMatch(line, q, excerptStyle, matchStyle)
}

// excerptParagraph builds a multi-line, soft-wrapped excerpt for the
// row. It collects up to ~220 chars of meaningful body lines (skipping
// the same redundant patterns excerptFor does), joins them into one
// paragraph, then word-wraps to width with maxLines as the cap. When
// width is too small to soft-wrap (e.g. tests that never sent a
// WindowSizeMsg), it falls back to the single-line excerptFor result so
// existing callers keep getting something useful.
func (m Model) excerptParagraph(e ports.NoteEntry, width, maxLines int) []string {
	if width < 8 || maxLines < 1 {
		if line := m.excerptFor(e); line != "" {
			return []string{line}
		}
		return nil
	}
	body, ok := m.bodies[e.ID]
	if !ok {
		return nil
	}
	var collected []string
	total := 0
	for _, line := range strings.Split(string(body), "\n") {
		clean := strings.TrimSpace(line)
		if clean == "" {
			continue
		}
		clean = strings.TrimLeft(clean, "#- *>`")
		clean = strings.TrimSpace(clean)
		if clean == "" {
			continue
		}
		if isDateString(clean) || clean == e.Meta.Date || clean == e.Meta.Project {
			continue
		}
		collected = append(collected, clean)
		total += len(clean) + 1
		if total >= 220 {
			break
		}
	}
	if len(collected) == 0 {
		return nil
	}
	return softWrap(strings.Join(collected, " "), width, maxLines)
}

// softWrap word-wraps s to width cells, capped at maxLines. Lines past
// the cap are dropped and the last kept line gets a "…" suffix. Words
// longer than width get hard-truncated. Empty input returns nil.
func softWrap(s string, width, maxLines int) []string {
	if width < 8 || maxLines < 1 {
		return nil
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	cur := ""
	for _, w := range words {
		next, full := wrapAppend(cur, w, width)
		if !full {
			cur = next
			continue
		}
		if cur != "" {
			lines = append(lines, cur)
			if len(lines) >= maxLines {
				lines[maxLines-1] = appendEllipsis(lines[maxLines-1], width)
				return lines
			}
		}
		cur = wrapStart(w, width)
	}
	return finishWrap(lines, cur, width, maxLines)
}

// wrapAppend tries to append w to cur with a space separator. Returns
// (newLine, false) on success or (cur, true) when the result would
// exceed width.
func wrapAppend(cur, w string, width int) (string, bool) {
	candidate := w
	if cur != "" {
		candidate = cur + " " + w
	}
	if lipgloss.Width(candidate) <= width {
		return candidate, false
	}
	return cur, true
}

// wrapStart starts a new wrapped line with w, hard-truncating if w
// alone exceeds width.
func wrapStart(w string, width int) string {
	if lipgloss.Width(w) > width {
		return truncateText(w, width)
	}
	return w
}

// finishWrap flushes the remaining cur into lines and applies the
// ellipsis-on-overflow rule.
func finishWrap(lines []string, cur string, width, maxLines int) []string {
	if cur == "" {
		return lines
	}
	if len(lines) < maxLines {
		return append(lines, cur)
	}
	lines[maxLines-1] = appendEllipsis(lines[maxLines-1], width)
	return lines
}

func appendEllipsis(s string, width int) string {
	if lipgloss.Width(s) >= width {
		return truncateText(s, width)
	}
	return s + "…"
}

// smartTitle picks a human-readable label for the row when the user
// hasn't set a frontmatter Title. Daily notes fall back to the raw ID
// (the date column already shows the date, but tests assert on the ID
// substring being present, and dropping the ID entirely would lose
// signal in older notebooks). Project notes get the canonical URL's
// last two segments — turning a 50-char `projects/github.com/.../date`
// into a clean `serverkraken/dotfiles`.
func smartTitle(e ports.NoteEntry) string {
	if e.Meta.Title != "" {
		return e.Meta.Title
	}
	if e.Meta.Type == domain.TypeProject && e.Meta.Project != "" {
		return shortProjectLabel(e.Meta.Project)
	}
	return e.ID.String()
}

func shortProjectLabel(canonicalURL string) string {
	parts := strings.Split(canonicalURL, "/")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	return canonicalURL
}

// truncateText clips s to at most `max` cells, appending an ellipsis. It
// is intentionally byte-naive — note titles are predominantly ASCII or
// latin-1 in practice, and a stricter rune-aware version would be
// overkill until a real-world note proves it wrong.
func truncateText(s string, maxCells int) string {
	if maxCells <= 1 {
		return "…"
	}
	if lipgloss.Width(s) <= maxCells {
		return s
	}
	if len(s) > maxCells-1 {
		return s[:maxCells-1] + "…"
	}
	return s + "…"
}

func (m Model) excerptFor(e ports.NoteEntry) string {
	body, ok := m.bodies[e.ID]
	if !ok {
		return ""
	}
	for _, line := range strings.Split(string(body), "\n") {
		clean := strings.TrimSpace(line)
		if clean == "" {
			continue
		}
		// Skip leading markdown clutter like "# heading" so the excerpt
		// reads as prose.
		clean = strings.TrimLeft(clean, "#- *>`")
		clean = strings.TrimSpace(clean)
		if clean == "" {
			continue
		}
		// Skip lines that are uninformative duplicates of the row's other
		// columns: a bare YYYY-MM-DD (often the project-template's only
		// body line), the row's own date, the canonical project URL.
		if isDateString(clean) || clean == e.Meta.Date || clean == e.Meta.Project {
			continue
		}
		if len(clean) > 80 {
			clean = clean[:79] + "…"
		}
		return clean
	}
	return ""
}

func isDateString(s string) bool {
	if len(s) != 10 {
		return false
	}
	for i, c := range s {
		switch i {
		case 4, 7:
			if c != '-' {
				return false
			}
		default:
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

func (m Model) renderTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	var parts []string
	for _, t := range tags {
		parts = append(parts, tagChipStyle(t).Render(t))
	}
	return strings.Join(parts, " ")
}

func (m Model) renderEmptyState(width int) string {
	glyph := emptyGlyphStyle.Render("✺")
	title := emptyTitleStyle.Render("keine Treffer")
	newKey := keyLabel(m.keys.New)
	searchKey := keyLabel(m.keys.Search)
	filterKey := keyLabel(m.keys.Filter)
	hint := footerKeyStyle.Render(newKey) +
		emptyHintStyle.Render(" → neue Notiz anlegen")
	tail := footerKeyStyle.Render(filterKey) +
		emptyHintStyle.Render(" → Filter wechseln · ") + footerKeyStyle.Render(searchKey) +
		emptyHintStyle.Render(" → Suche zurücksetzen")
	stack := lipgloss.JoinVertical(lipgloss.Center, glyph, "", title, hint, tail)
	if width <= 0 {
		return stack
	}
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, stack)
}

// keyLabel returns the first canonical label for a binding (e.g. "n", "/").
func keyLabel(b key.Binding) string {
	keys := b.Keys()
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

func (m Model) renderPreviewPane() string {
	paneW := m.previewPaneWidth()
	if paneW <= 0 {
		return ""
	}
	titleLine := panelTitleStyle.Render("vorschau")
	body := m.preview.View()
	if body == "" {
		body = dimStyle.Render("(leer)")
	}
	inner := lipgloss.JoinVertical(lipgloss.Left, titleLine, body)
	// lipgloss Style.Width sets the *content* width; the NormalBorder
	// adds 1 cell on each side on top of that. Pass paneW-2 so the
	// rendered pane is exactly paneW wide overall — otherwise the body
	// row gets two cells too wide and lipgloss wraps every line in
	// half, doubling the visible height of a long Markdown preview.
	innerW := paneW - 2
	if innerW < 1 {
		innerW = 1
	}
	return panelStyle.Width(innerW).Render(inner)
}

func (m Model) renderFooter() string {
	switch m.mode {
	case ModeSearch:
		return footerStyle.Render("tippen → filtern  ·  enter → anwenden  ·  esc → abbrechen")
	case ModeConfirmDelete:
		return footerStyle.Render("y/Enter → ja  ·  n/Esc → nein")
	}
	return m.helpUI.View(m.keys)
}

// renderStatusBar is the bar at the bottom of the frame. Left to right:
// transient-mode badge (only while searching or confirming a delete),
// current note path (cursor's ID, truncated to fit), and a meta tail
// with the index's age plus a `?` help hint. The bar paints
// pal.BgChip across its full inner width so it reads as a contiguous
// strip; the path is the elastic cell so the line never wraps onto a
// second row.
func (m Model) renderStatusBar() string {
	width := m.contentWidth()
	if width <= 0 {
		return ""
	}
	mode := m.statusBarMode()
	meta := m.statusBarMeta()

	// Reserve a single padding space between mode/path and path/meta.
	consumed := lipgloss.Width(mode) + lipgloss.Width(meta) + 2
	avail := width - consumed
	if avail < 5 {
		avail = 5
	}
	path := m.statusBarPath()
	if lipgloss.Width(path) > avail {
		path = truncateText(path, avail)
	}

	pathSegment := statusBarPathStyle.Render(" " + path)
	gap := width - lipgloss.Width(mode) - lipgloss.Width(pathSegment) - lipgloss.Width(meta)
	if gap < 0 {
		gap = 0
	}
	return mode + pathSegment + statusBarStyle.Render(strings.Repeat(" ", gap)) + meta
}

// statusBarMode returns the transient-mode badge — only Search and
// Delete-Confirm get one. Normal mode renders nothing so the bar starts
// with the path directly: there's no concept of a vim-style "NORMAL"
// state to communicate, and labelling one made the bar read as if there
// were modes the user could switch between.
func (m Model) statusBarMode() string {
	switch m.mode {
	case ModeSearch:
		return statusBarModeSearchStyle.Render("SUCHE")
	case ModeConfirmDelete:
		return statusBarModeDeleteStyle.Render("LÖSCHEN")
	}
	return ""
}

// statusBarPath returns the cursor's note ID (notebook-relative path,
// no .md suffix). Falls back to "—" when no entry is selected so the
// bar stays a stable shape across empty/loading states.
func (m Model) statusBarPath() string {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return "—"
	}
	return m.visible[m.cursor].ID.String()
}

// statusBarMeta builds the right-aligned tail. Today that's just the
// index age (when the IndexAgeFunc is wired and produced a non-zero
// time); a `? help` hint used to live here too but the help footer
// directly above the bar already lists `?`, so the second copy was just
// noise. Returns "" when there's nothing to show — keeps the bar
// flush-right without dangling padding.
func (m Model) statusBarMeta() string {
	if m.indexAge == nil {
		return ""
	}
	t := m.indexAge()
	if t.IsZero() {
		return ""
	}
	label := statusBarMetaStyle.Render("Index " + humanizeAge(time.Since(t)))
	return statusBarStyle.Render(" ") + label + statusBarStyle.Render(" ")
}

// humanizeAge renders a duration as a compact "5s" / "12m" / "3h" /
// "4d" string. Anything under one second collapses to "now".
func humanizeAge(d time.Duration) string {
	if d < time.Second {
		return "now"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// renderDeleteModal — Skill §Component vocabulary + §Visual hierarchy:
// Single-Question + Single-Hint statt vierfacher Bestätigungs-Affordance
// (vorher: Headline, Target-ID, Prompt, Key-Pillen, Hint — zu dicht).
// Wording deutsch, kanonisches y/Enter → ja, n/Esc → nein. Frame
// kommt aus components/modal (Kind = Danger → Red DoubleBorder); die
// internen Style-Vars (modalDangerStyle, modalQuestionStyle,
// modalHintStyle) bleiben für die Inhalts-Hierarchie.
func (m Model) renderDeleteModal() string {
	headline := modalDangerStyle.Render("⚠  Notiz löschen?")
	target := modalQuestionStyle.Render(m.deleteTargetID.String())
	hint := modalHintStyle.Render("y/Enter → ja  ·  n/Esc → nein")
	body := lipgloss.JoinVertical(lipgloss.Center, headline, "", target, "", hint)
	return modal.Render(body, modal.Opts{Kind: modal.KindDanger}, pal)
}

// renderHelpOverlay nutzt components/modal in der Default-Variante
// (Accent-Border) — der Help-Inhalt ist informativ, nicht safe-/danger-
// markiert. Der Inline-Title („Tastenbelegung") bleibt in body, weil
// modal.Opts.Title unter dem Border sitzt und doppelt wäre.
func (m Model) renderHelpOverlay() string {
	title := modalQuestionStyle.Render("Tastenbelegung")
	hForm := help.New()
	hForm.ShowAll = true
	hForm.Width = 70
	hForm.Styles = m.helpUI.Styles
	body := hForm.View(m.keys)
	hint := modalHintStyle.Render("? / Esc → schließen")
	return modal.Render(
		lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", hint),
		modal.Opts{}, pal,
	)
}

// frameContent wraps content in the rounded outer frame and explicitly
// pads it to fill the full terminal height. lipgloss.Style.Height does
// not reliably pad bordered styles whose vertical padding is 0, so the
// frame would otherwise stop right after the footer and leave the
// bottom half of the pane bare. Manual padding is the cheap, reliable
// fix.
func frameContent(width, height int, content string) string {
	if width <= 0 || height <= 0 {
		return content
	}
	innerW := width - 2
	if innerW <= 0 {
		return content
	}
	targetLines := height - 2 // top + bottom border
	if targetLines > 0 {
		contentLines := strings.Count(content, "\n") + 1
		if contentLines < targetLines {
			content += strings.Repeat("\n", targetLines-contentLines)
		}
	}
	return frameStyle.Width(innerW).Render(content)
}

// overlay places `top` centered over a dotted backdrop. lipgloss in this
// version doesn't expose Layer/Canvas, so a true splice over `base` would
// mean ANSI-aware line surgery — fragile next to glamour-rendered preview
// content. Instead the backdrop uses a subtle dotted fill so the modal
// reads as floating, not as a context-blanking takeover.
func overlay(base, top string, width, height int) string {
	_ = base
	if width <= 0 || height <= 0 {
		return top
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, top,
		lipgloss.WithWhitespaceChars("·"),
		lipgloss.WithWhitespaceForeground(lipgloss.Color(pal.BgChip)))
}

// badgeFor returns the colored type pill shown in the list row.
// Skill §German UI: Badges sind user-facing — DE-Kurzformen statt EN.
// Width-Padding auf einheitliche 5 Zellen, damit die Type-Pille die
// Listen-Spalten nicht ungleichmäßig schiebt.
func badgeFor(t domain.NoteType) string {
	switch t {
	case domain.TypeDaily:
		return badgeDailyStyle.Render("TÄGL.")
	case domain.TypeProject:
		return badgeProjectStyle.Render("PROJ.")
	case domain.TypeFree:
		return badgeFreeStyle.Render("FREI ")
	}
	return badgeUnknownStyle.Render("  ?  ")
}

// highlightMatch wraps the substring `q` (case-insensitive) in `match`
// styling, leaving the surrounding text in `base`. q == "" returns the
// base-styled text untouched.
func highlightMatch(text, q string, base, match lipgloss.Style) string {
	if q == "" {
		return base.Render(text)
	}
	lower := strings.ToLower(text)
	idx := strings.Index(lower, q)
	if idx < 0 {
		return base.Render(text)
	}
	end := idx + len(q)
	return base.Render(text[:idx]) + match.Render(text[idx:end]) + base.Render(text[end:])
}

// isToday reports whether date is today's YYYY-MM-DD. The clock here is
// the system clock — the browse view is a read-only surface, so the use
// case's testable Clock isn't threaded through.
func isToday(date string) bool {
	if date == "" {
		return false
	}
	return time.Now().UTC().Format("2006-01-02") == date
}
