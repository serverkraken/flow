package browse_test

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown_overlay"
	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/frontend/tui/browse"
	"github.com/serverkraken/flow/internal/kompendium/frontend/tui/writepicker"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

// TestBrowse_WithIndexAgeAndBacklinks_BuilderChain confirms the
// optional WithIndexAge / WithBacklinks setters return a Model
// without dropping the underlying state. Cheap dispatch coverage —
// the actual age + backlinks are exercised by the view tests.
func TestBrowse_WithIndexAgeAndBacklinks_BuilderChain(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	m := browse.New(usecase.NewListNotes(store), nil, nil, "", nil, nil)
	m = m.WithIndexAge(func() time.Time { return time.Unix(42, 0) })
	m = m.WithBacklinks(func(domain.ID) []usecase.BacklinkRef {
		return []usecase.BacklinkRef{{ID: "x", Title: "X"}}
	})
	if got := m.View(); got == "" {
		t.Errorf("post-setter View() should still produce output")
	}
}

func TestBrowse_LoadsEntriesAndRenders(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustNote("daily/2026-04-25", domain.TypeDaily, "today"), time.Unix(2, 0))
	store.Seed(mustNote("notes/setup", domain.TypeFree, "setup"), time.Unix(1, 0))

	got := drive(t, newModel(usecase.NewListNotes(store)))

	// Welle 4: Header-Label ist deutsch („Notizen").
	if !strings.Contains(got, "kompendium — 2/2 Notizen") {
		t.Errorf("missing header in view:\n%s", got)
	}
	if !strings.Contains(got, "today") || !strings.Contains(got, "setup") {
		t.Errorf("missing entries in view:\n%s", got)
	}
}

func TestBrowse_NavigatesCursor(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustNote("daily/2026-04-25", domain.TypeDaily, "first"), time.Unix(2, 0))
	store.Seed(mustNote("daily/2026-04-22", domain.TypeDaily, "second"), time.Unix(1, 0))

	model := initialised(newModel(usecase.NewListNotes(store)))
	model, _ = model.Update(key("j"))

	view := model.View()
	if !cursorOnLineWith(view, "second") {
		t.Errorf("cursor did not move to second entry:\n%s", view)
	}
}

func TestBrowse_QuitOnQ(t *testing.T) {
	t.Parallel()

	m := newModel(usecase.NewListNotes(testutil.NewFakeNoteStore()))
	_, cmd := m.Update(key("q"))
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("expected QuitMsg, got %T", cmd())
	}
}

func TestBrowse_CursorClampedAtEdges(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustNote("daily/2026-04-25", domain.TypeDaily, "x"), time.Unix(1, 0))

	model := initialised(newModel(usecase.NewListNotes(store)))
	for range 10 {
		model, _ = model.Update(key("k"))
	}
	for range 10 {
		model, _ = model.Update(key("j"))
	}
	if !strings.Contains(model.View(), "▶") {
		t.Errorf("cursor disappeared after edge navigation:\n%s", model.View())
	}
}

func TestBrowse_GoToTopAndBottom(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	for i, body := range []string{"first", "second", "third"} {
		store.Seed(mustNote(domain.ID("daily/2026-04-2"+string(rune('0'+i+3))), domain.TypeDaily, body), time.Unix(int64(10-i), 0))
	}

	model := initialised(newModel(usecase.NewListNotes(store)))
	model, _ = model.Update(key("G"))
	if !cursorOnLineWith(model.View(), "third") {
		t.Errorf("G should move to last entry, got\n%s", model.View())
	}

	model, _ = model.Update(key("g"))
	if !cursorOnLineWith(model.View(), "first") {
		t.Errorf("g should move to first entry, got\n%s", model.View())
	}
}

func TestBrowse_FilterCyclesByType(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustNote("daily/2026-04-25", domain.TypeDaily, "the daily"), time.Unix(3, 0))
	store.Seed(mustProject("projects/p/2026-04-25", "the project"), time.Unix(2, 0))
	store.Seed(mustNote("notes/setup", domain.TypeFree, "the free"), time.Unix(1, 0))

	model := initialised(newModel(usecase.NewListNotes(store)))

	if !strings.Contains(model.View(), "the daily") || !strings.Contains(model.View(), "the project") {
		t.Errorf("All filter should show every entry:\n%s", model.View())
	}

	model, _ = model.Update(tabKey())
	view := model.View()
	if !strings.Contains(view, "the daily") || strings.Contains(view, "the project") {
		t.Errorf("Daily filter wrong:\n%s", view)
	}
	if !strings.Contains(view, "Filter:") || !strings.Contains(view, "Daily") {
		t.Errorf("filter label not Daily:\n%s", view)
	}

	model, _ = model.Update(tabKey())
	view = model.View()
	if !strings.Contains(view, "the project") || strings.Contains(view, "the daily") {
		t.Errorf("Project filter wrong:\n%s", view)
	}

	model, _ = model.Update(tabKey())
	view = model.View()
	if !strings.Contains(view, "the free") || strings.Contains(view, "the daily") {
		t.Errorf("Free filter wrong:\n%s", view)
	}

	model, _ = model.Update(tabKey())
	view = model.View()
	if !strings.Contains(view, "the daily") || !strings.Contains(view, "the project") {
		t.Errorf("filter wrap to All wrong:\n%s", view)
	}
}

func TestBrowse_SearchFiltersOnTitleAndProject(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustNote("daily/2026-04-25", domain.TypeDaily, "kompendium architecture"), time.Unix(2, 0))
	store.Seed(mustProject("projects/github.com/foo/bar/2026-04-25", "Foo work"), time.Unix(1, 0))

	model := initialised(newModel(usecase.NewListNotes(store)))

	model, _ = model.Update(key("/"))
	if !strings.Contains(model.View(), "Suche:") {
		t.Errorf("search bar should appear:\n%s", model.View())
	}

	for _, r := range "kompendium" {
		model, _ = model.Update(runeKey(r))
	}
	view := model.View()
	if !strings.Contains(view, "kompendium architecture") || strings.Contains(view, "Foo work") {
		t.Errorf("search did not narrow:\n%s", view)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if !strings.Contains(model.View(), "Suche:") {
		t.Errorf("search bar lost after backspace:\n%s", model.View())
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if strings.Contains(model.View(), "tippen → filtern") {
		t.Errorf("enter should leave search mode:\n%s", model.View())
	}
	model, _ = model.Update(key("/"))
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	view = model.View()
	if !strings.Contains(view, "Foo work") {
		t.Errorf("esc should clear search query (showing all entries):\n%s", view)
	}
}

func TestBrowse_SearchSpace(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustNote("daily/2026-04-25", domain.TypeDaily, "two words"), time.Unix(1, 0))

	model := initialised(newModel(usecase.NewListNotes(store)))
	model, _ = model.Update(key("/"))
	for _, r := range "two" {
		model, _ = model.Update(runeKey(r))
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	for _, r := range "wo" {
		model, _ = model.Update(runeKey(r))
	}
	if !strings.Contains(model.View(), "two words") {
		t.Errorf("space-containing search should match:\n%s", model.View())
	}
}

func TestBrowse_NavigationLiteralInSearchMode(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustNote("daily/2026-04-25", domain.TypeDaily, "first"), time.Unix(2, 0))
	store.Seed(mustNote("daily/2026-04-22", domain.TypeDaily, "second"), time.Unix(1, 0))

	model := initialised(newModel(usecase.NewListNotes(store)))
	model, _ = model.Update(key("/"))
	model, _ = model.Update(runeKey('j'))
	if !strings.Contains(model.View(), "Suche: j") {
		t.Errorf("j should land in search query, not move the cursor:\n%s", model.View())
	}
}

func TestBrowse_EmptyStateHints(t *testing.T) {
	t.Parallel()

	// Welle 4: Empty-State-Hint ist deutsch („keine Treffer").
	model := initialised(newModel(usecase.NewListNotes(testutil.NewFakeNoteStore())))
	if !strings.Contains(model.View(), "keine Treffer") {
		t.Errorf("missing empty-state hint:\n%s", model.View())
	}
}

func TestBrowse_LoadError(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.ListErr = errForTest("forced list err")

	model := initialised(newModel(usecase.NewListNotes(store)))
	if !strings.Contains(model.View(), "Fehler:") {
		t.Errorf("missing error indicator in view:\n%s", model.View())
	}
}

func TestBrowse_QuittingViewIsEmpty(t *testing.T) {
	t.Parallel()

	m := newModel(usecase.NewListNotes(testutil.NewFakeNoteStore()))
	model, _ := m.Update(key("q"))
	if model.View() != "" {
		t.Errorf("quitting view should be empty, got %q", model.View())
	}
}

func TestBrowse_LoadingState(t *testing.T) {
	t.Parallel()

	// Welle 4: Loading-Label ist deutsch („lädt…").
	m := newModel(usecase.NewListNotes(testutil.NewFakeNoteStore()))
	if !strings.Contains(m.View(), "lädt") {
		t.Errorf("missing loading state in initial view: %q", m.View())
	}
}

func TestBrowse_WindowResize(t *testing.T) {
	t.Parallel()

	m := newModel(usecase.NewListNotes(testutil.NewFakeNoteStore()))
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if model.View() == "" {
		t.Error("WindowSizeMsg should not blank the view")
	}
}

func TestBrowse_SearchMatchesBodyContent(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	// Two notes with bland identical schema-ish titles; only the body
	// distinguishes them. Real-world payoff for searching content.
	hit, _ := domain.NewNote(
		domain.ID("daily/2026-04-25"),
		domain.Frontmatter{ID: "daily/2026-04-25", Type: domain.TypeDaily, Date: "2026-04-25"},
		[]byte("today I worked on Feldspar mineral classification"),
	)
	miss, _ := domain.NewNote(
		domain.ID("daily/2026-04-22"),
		domain.Frontmatter{ID: "daily/2026-04-22", Type: domain.TypeDaily, Date: "2026-04-22"},
		[]byte("plain text without that word"),
	)
	store.Seed(hit, time.Unix(2, 0))
	store.Seed(miss, time.Unix(1, 0))

	noopCmd := func(_ string) *exec.Cmd { return exec.Command("true") }
	noopWrite := func(writepicker.Result) *exec.Cmd { return exec.Command("true") }
	m := browse.New(usecase.NewListNotes(store), store, nil, "", noopCmd, noopWrite)

	// Init → entriesLoadedMsg → loadBodiesCmd → bodiesLoadedMsg.
	model, cmd := m.Update(m.Init()())
	if cmd == nil {
		t.Fatal("entriesLoadedMsg should schedule a body load")
	}
	model, _ = model.Update(cmd())

	// Now type a search query that only matches the body of `hit`.
	model, _ = model.Update(key("/"))
	for _, r := range "feldspar" {
		model, _ = model.Update(runeKey(r))
	}
	view := model.View()
	if !strings.Contains(view, "daily/2026-04-25") {
		t.Errorf("body match should keep the matching note visible:\n%s", view)
	}
	if strings.Contains(view, "daily/2026-04-22") {
		t.Errorf("non-matching note should be hidden:\n%s", view)
	}
}

func TestBrowse_EnterRunsEditCmdOnSelected(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustNote("daily/2026-04-25", domain.TypeDaily, "today"), time.Unix(1, 0))

	var capturedPath string
	editCmd := func(path string) *exec.Cmd {
		capturedPath = path
		return exec.Command("true")
	}
	noopWrite := func(writepicker.Result) *exec.Cmd { return exec.Command("true") }

	m := browse.New(usecase.NewListNotes(store), store, nil, "", editCmd, noopWrite)
	model := initialised(m)
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on a selected entry should return a tea.Cmd")
	}
	if capturedPath == "" {
		t.Errorf("editCmd was not invoked — path resolver did not run")
	}
	if !strings.Contains(capturedPath, "daily/2026-04-25.md") {
		t.Errorf("path got %q, want it to contain daily/2026-04-25.md", capturedPath)
	}
}

// TestBrowse_VOpensInProcessViewer asserts that pressing v on the
// selected entry switches the model into ModeView and that View()
// then renders the in-process viewer (which carries the note title)
// instead of the regular browse list. Replaces the previous test
// that exercised a now-removed glow shell-out.
func TestBrowse_VOpensInProcessViewer(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustNote("daily/2026-04-25", domain.TypeDaily, "today"), time.Unix(1, 0))

	noopCmd := func(_ string) *exec.Cmd { return exec.Command("true") }
	noopWrite := func(writepicker.Result) *exec.Cmd { return exec.Command("true") }

	m := browse.New(usecase.NewListNotes(store), store, nil, "", noopCmd, noopWrite)
	model := initialised(m)
	model, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model, _ = model.Update(runeKey('v'))

	bm, ok := model.(browse.Model)
	if !ok {
		t.Fatalf("model is not browse.Model, got %T", model)
	}
	if bm.CurrentMode() != browse.ModeView {
		t.Errorf("after v, mode = %v, want ModeView", bm.CurrentMode())
	}
	if !strings.Contains(model.View(), "today") {
		t.Errorf("viewer should render note title 'today'\n%s", model.View())
	}
}

// TestBrowse_VViewerExitReturnsToNormal sends a markdown_overlay.ExitMsg
// and asserts the model leaves ModeView. Lives here rather than in
// markdown_overlay's package tests because it exercises the parent
// reducer's ExitMsg handling.
func TestBrowse_VViewerExitReturnsToNormal(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustNote("daily/2026-04-25", domain.TypeDaily, "today"), time.Unix(1, 0))

	noopCmd := func(_ string) *exec.Cmd { return exec.Command("true") }
	noopWrite := func(writepicker.Result) *exec.Cmd { return exec.Command("true") }

	m := browse.New(usecase.NewListNotes(store), store, nil, "", noopCmd, noopWrite)
	model := initialised(m)
	model, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model, _ = model.Update(runeKey('v'))

	if got := model.(browse.Model).CurrentMode(); got != browse.ModeView {
		t.Fatalf("setup: mode = %v, want ModeView", got)
	}
	model, _ = model.Update(markdown_overlay.ExitMsg{})
	if got := model.(browse.Model).CurrentMode(); got != browse.ModeNormal {
		t.Errorf("after ExitMsg, mode = %v, want ModeNormal", got)
	}
}

func TestBrowse_EnterNoOpWithoutPather(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustNote("daily/2026-04-25", domain.TypeDaily, "x"), time.Unix(1, 0))
	// New() with nil store/editCmd should silently no-op on Enter.
	m := browse.New(usecase.NewListNotes(store), nil, nil, "", nil, nil)
	model := initialised(m)
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Errorf("Enter without pather/editCmd should be a no-op, got cmd=%v", cmd())
	}
}

func TestBrowse_EnterNoOpOnEmptyList(t *testing.T) {
	t.Parallel()

	editCmd := func(_ string) *exec.Cmd { return exec.Command("true") }
	noopWrite := func(writepicker.Result) *exec.Cmd { return exec.Command("true") }
	store := testutil.NewFakeNoteStore()
	m := browse.New(usecase.NewListNotes(store), store, nil, "", editCmd, noopWrite)
	model := initialised(m)
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Errorf("Enter on empty list should be a no-op, got cmd=%v", cmd())
	}
}

func TestBrowse_NEntersWritePickerAndDispatchesOnDone(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustNote("daily/2026-04-25", domain.TypeDaily, "today"), time.Unix(1, 0))

	noopCmd := func(_ string) *exec.Cmd { return exec.Command("true") }
	var (
		writeCalled bool
		gotResult   writepicker.Result
	)
	writeCmd := func(r writepicker.Result) *exec.Cmd {
		writeCalled = true
		gotResult = r
		return exec.Command("true")
	}

	m := browse.New(usecase.NewListNotes(store), store, nil, "", noopCmd, writeCmd)
	model := initialised(m)
	// Press `n` → enters ModeWritePicker; the writeCmd MUST NOT have run yet.
	model, _ = model.Update(runeKey('n'))
	if writeCalled {
		t.Fatal("writeCmd must not be called on `n` press — picker hasn't resolved")
	}
	if got := model.(browse.Model).CurrentMode(); got != browse.ModeWritePicker {
		t.Errorf("n should enter ModeWritePicker, got %v", got)
	}
	// Simulate the picker resolving with ChoiceDaily — the DoneMsg
	// is what the embedded picker would emit on Enter for the Daily row.
	model, cmd := model.Update(writepicker.DoneMsg{Result: writepicker.Result{Choice: writepicker.ChoiceDaily}})
	if cmd == nil {
		t.Fatal("DoneMsg with non-Cancel choice should schedule writeCmd via tea.ExecProcess")
	}
	if !writeCalled {
		t.Error("writeCmd should have been invoked once the picker resolved")
	}
	if gotResult.Choice != writepicker.ChoiceDaily {
		t.Errorf("writeCmd Result Choice = %v, want ChoiceDaily", gotResult.Choice)
	}
	// After DoneMsg the model should leave ModeWritePicker.
	if got := model.(browse.Model).CurrentMode(); got != browse.ModeNormal {
		t.Errorf("after DoneMsg mode = %v, want ModeNormal", got)
	}
}

func TestBrowse_NWritePickerCancelIsNoOp(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustNote("daily/2026-04-25", domain.TypeDaily, "today"), time.Unix(1, 0))
	noopCmd := func(_ string) *exec.Cmd { return exec.Command("true") }
	var writeCalled bool
	writeCmd := func(writepicker.Result) *exec.Cmd {
		writeCalled = true
		return exec.Command("true")
	}

	m := browse.New(usecase.NewListNotes(store), store, nil, "", noopCmd, writeCmd)
	model := initialised(m)
	model, _ = model.Update(runeKey('n'))
	model, cmd := model.Update(writepicker.DoneMsg{Result: writepicker.Result{Choice: writepicker.ChoiceCancel}})
	if writeCalled {
		t.Error("Cancel from picker must not invoke writeCmd")
	}
	if cmd != nil {
		t.Errorf("Cancel should not schedule any cmd, got %v", cmd())
	}
	if got := model.(browse.Model).CurrentMode(); got != browse.ModeNormal {
		t.Errorf("after Cancel DoneMsg mode = %v, want ModeNormal", got)
	}
}

func TestBrowse_NNoOpWithoutWriteCmd(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustNote("daily/2026-04-25", domain.TypeDaily, "x"), time.Unix(1, 0))
	noopCmd := func(_ string) *exec.Cmd { return exec.Command("true") }
	m := browse.New(usecase.NewListNotes(store), store, nil, "", noopCmd, nil)
	model := initialised(m)
	// `n` still enters ModeWritePicker; resolving the picker with no
	// writeCmd wired must leave the model harmless (no panic, no cmd).
	model, _ = model.Update(runeKey('n'))
	model, cmd := model.Update(writepicker.DoneMsg{Result: writepicker.Result{Choice: writepicker.ChoiceDaily}})
	if cmd != nil {
		t.Errorf("DoneMsg without writeCmd wired should not schedule a cmd, got %v", cmd())
	}
	if got := model.(browse.Model).CurrentMode(); got != browse.ModeNormal {
		t.Errorf("after DoneMsg mode = %v, want ModeNormal", got)
	}
}

func TestBrowse_DOpensConfirmPrompt(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustNote("daily/2026-04-25", domain.TypeDaily, "today"), time.Unix(1, 0))
	noopCmd := func(_ string) *exec.Cmd { return exec.Command("true") }
	noopWrite := func(writepicker.Result) *exec.Cmd { return exec.Command("true") }
	deleteUC := usecase.NewDeleteNote(store, nil)

	m := browse.New(usecase.NewListNotes(store), store, deleteUC, "", noopCmd, noopWrite)
	model := initialised(m)
	model, cmd := model.Update(runeKey('D'))
	if cmd != nil {
		t.Errorf("D should only switch mode, not schedule a cmd, got %v", cmd())
	}
	view := model.View()
	// Welle 4: Modal vereinfacht — single-question + DE-Hint, keine
	// vierfache Affordance. Note-ID erscheint im Modal, der Hint ist
	// die kanonische y/Enter-Variante.
	if !strings.Contains(view, "daily/2026-04-25") {
		t.Errorf("confirm prompt should render the note ID:\n%s", view)
	}
	if !strings.Contains(view, "Notiz löschen?") {
		t.Errorf("confirm prompt not rendered:\n%s", view)
	}
	if !strings.Contains(view, "y/Enter → ja") {
		t.Errorf("confirm footer hint missing:\n%s", view)
	}
}

func TestBrowse_DConfirmDeletesAndReloads(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustNote("daily/2026-04-25", domain.TypeDaily, "today"), time.Unix(1, 0))
	noopCmd := func(_ string) *exec.Cmd { return exec.Command("true") }
	noopWrite := func(writepicker.Result) *exec.Cmd { return exec.Command("true") }
	deleteUC := usecase.NewDeleteNote(store, nil)

	m := browse.New(usecase.NewListNotes(store), store, deleteUC, "", noopCmd, noopWrite)
	model := initialised(m)
	model, _ = model.Update(runeKey('D'))
	model, cmd := model.Update(runeKey('y'))
	if cmd == nil {
		t.Fatal("y on confirm prompt should schedule a delete cmd")
	}
	// Drain delete cmd → deleteFinishedMsg → reload cmd → entriesLoadedMsg.
	model, cmd = model.Update(cmd())
	if cmd == nil {
		t.Fatal("deleteFinishedMsg{nil} should schedule a reload cmd")
	}
	model, _ = model.Update(cmd())

	view := model.View()
	if strings.Contains(view, "daily/2026-04-25") {
		t.Errorf("note should be gone after delete, got:\n%s", view)
	}
	if strings.Contains(view, "Delete daily/2026-04-25?") {
		t.Errorf("confirm prompt should be cleared:\n%s", view)
	}
}

func TestBrowse_DCancelOnN(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustNote("daily/2026-04-25", domain.TypeDaily, "today"), time.Unix(1, 0))
	noopCmd := func(_ string) *exec.Cmd { return exec.Command("true") }
	noopWrite := func(writepicker.Result) *exec.Cmd { return exec.Command("true") }
	deleteUC := usecase.NewDeleteNote(store, nil)

	m := browse.New(usecase.NewListNotes(store), store, deleteUC, "", noopCmd, noopWrite)
	model := initialised(m)
	model, _ = model.Update(runeKey('D'))
	model, cmd := model.Update(runeKey('n'))
	if cmd != nil {
		t.Errorf("n on confirm prompt must not schedule a cmd, got %v", cmd())
	}
	view := model.View()
	if strings.Contains(view, "Delete daily/2026-04-25?") {
		t.Errorf("confirm prompt should be dismissed:\n%s", view)
	}
	if !strings.Contains(view, "today") {
		t.Errorf("note should still be present after cancel:\n%s", view)
	}
}

func TestBrowse_DCancelOnEsc(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustNote("daily/2026-04-25", domain.TypeDaily, "today"), time.Unix(1, 0))
	noopCmd := func(_ string) *exec.Cmd { return exec.Command("true") }
	noopWrite := func(writepicker.Result) *exec.Cmd { return exec.Command("true") }
	deleteUC := usecase.NewDeleteNote(store, nil)

	m := browse.New(usecase.NewListNotes(store), store, deleteUC, "", noopCmd, noopWrite)
	model := initialised(m)
	model, _ = model.Update(runeKey('D'))
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Errorf("esc on confirm prompt must not schedule a cmd, got %v", cmd())
	}
	if strings.Contains(model.View(), "Delete daily/2026-04-25?") {
		t.Errorf("confirm prompt should be dismissed by esc:\n%s", model.View())
	}
}

func TestBrowse_DNoOpWithoutDeleteUC(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustNote("daily/2026-04-25", domain.TypeDaily, "today"), time.Unix(1, 0))
	noopCmd := func(_ string) *exec.Cmd { return exec.Command("true") }
	noopWrite := func(writepicker.Result) *exec.Cmd { return exec.Command("true") }

	m := browse.New(usecase.NewListNotes(store), store, nil, "", noopCmd, noopWrite)
	model := initialised(m)
	model, cmd := model.Update(runeKey('D'))
	if cmd != nil {
		t.Errorf("D without delete usecase should be a no-op, got %v", cmd())
	}
	if strings.Contains(model.View(), "Delete daily/2026-04-25?") {
		t.Errorf("D should not open prompt when delete usecase is nil:\n%s", model.View())
	}
}

func TestBrowse_DNoOpOnEmptyList(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	noopCmd := func(_ string) *exec.Cmd { return exec.Command("true") }
	noopWrite := func(writepicker.Result) *exec.Cmd { return exec.Command("true") }
	deleteUC := usecase.NewDeleteNote(store, nil)

	m := browse.New(usecase.NewListNotes(store), store, deleteUC, "", noopCmd, noopWrite)
	model := initialised(m)
	model, cmd := model.Update(runeKey('D'))
	if cmd != nil {
		t.Errorf("D on empty list should be a no-op, got %v", cmd())
	}
	if strings.Contains(model.View(), "Delete") {
		t.Errorf("D on empty list should not open a confirm prompt:\n%s", model.View())
	}
}

func TestBrowse_DDeleteErrorSurfacesInView(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustNote("daily/2026-04-25", domain.TypeDaily, "today"), time.Unix(1, 0))
	store.DeleteErr = errForTest("disk full")
	noopCmd := func(_ string) *exec.Cmd { return exec.Command("true") }
	noopWrite := func(writepicker.Result) *exec.Cmd { return exec.Command("true") }
	deleteUC := usecase.NewDeleteNote(store, nil)

	m := browse.New(usecase.NewListNotes(store), store, deleteUC, "", noopCmd, noopWrite)
	model := initialised(m)
	model, _ = model.Update(runeKey('D'))
	model, cmd := model.Update(runeKey('y'))
	if cmd == nil {
		t.Fatal("y should schedule a delete cmd")
	}
	model, followUp := model.Update(cmd())
	if followUp != nil {
		t.Errorf("delete error should not schedule another cmd, got %v", followUp)
	}
	view := model.View()
	if !strings.Contains(view, "Fehler beim Bearbeiten") || !strings.Contains(view, "disk full") {
		t.Errorf("delete error should surface in view, got:\n%s", view)
	}
}

// --- helpers ----------------------------------------------------------------

type errString string

func (e errString) Error() string { return string(e) }

func errForTest(s string) error { return errString(s) }

func key(s string) tea.KeyMsg   { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
func runeKey(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }
func tabKey() tea.KeyMsg        { return tea.KeyMsg{Type: tea.KeyTab} }

// newModel returns a Model with the given list use case, a fresh in-memory
// store, and no-op edit/view/write cmds. Tests that need to assert what
// those callbacks received use browse.New directly.
func newModel(list *usecase.ListNotes) browse.Model {
	noopCmd := func(_ string) *exec.Cmd { return exec.Command("true") }
	noopWrite := func(writepicker.Result) *exec.Cmd { return exec.Command("true") }
	return browse.New(list, testutil.NewFakeNoteStore(), nil, "", noopCmd, noopWrite)
}

func drive(t *testing.T, m browse.Model) string {
	t.Helper()
	model, _ := m.Update(m.Init()())
	return model.View()
}

func initialised(m browse.Model) tea.Model {
	model, _ := m.Update(m.Init()())
	return model
}

func cursorOnLineWith(view, want string) bool {
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "▶") && strings.Contains(line, want) {
			return true
		}
	}
	return false
}

func mustNote(id domain.ID, typ domain.NoteType, title string) domain.Note {
	fm := domain.Frontmatter{ID: id.String(), Type: typ, Title: title}
	if typ == domain.TypeDaily {
		fm.Date = "2026-04-25"
	}
	n, err := domain.NewNote(id, fm, []byte(title+" body\n"))
	if err != nil {
		panic(err)
	}
	return n
}

func mustProject(id, title string) domain.Note {
	fm := domain.Frontmatter{ID: id, Type: domain.TypeProject, Project: "github.com/foo/bar", Title: title, Date: "2026-04-25"}
	n, err := domain.NewNote(domain.ID(id), fm, []byte(title+" body\n"))
	if err != nil {
		panic(err)
	}
	return n
}
