package project_picker_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/project_picker"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// plainView strips ANSI escape codes from View() output so tests can
// check for string content without dealing with interleaved SGR sequences
// (per-rune fuzzy-match highlights split every character).
func plainView(m project_picker.Model) string {
	return ansi.Strip(m.View())
}

// fakeMsg is an arbitrary tea.Msg used in callback captures during tests.
type fakeMsg struct{ name string }

// testItems returns a fixed project list for use across tests.
func testItems() []domain.Project {
	return []domain.Project{
		{ID: "1", Name: "alpha"},
		{ID: "2", Name: "beta"},
		{ID: "3", Name: "gamma"},
	}
}

// newTestModel constructs a Model with known-good callbacks and a
// fixed palette (theme.Default == TokyonightNight).
func newTestModel(items []domain.Project) (project_picker.Model, *domain.Project, *string) {
	var pickedProject domain.Project
	var createdName string
	pal := theme.Default

	onPick := func(p domain.Project) tea.Msg {
		pickedProject = p
		return fakeMsg{name: "picked:" + p.Name}
	}
	onCreate := func(name string) tea.Msg {
		createdName = name
		return fakeMsg{name: "create:" + name}
	}
	onCancel := fakeMsg{name: "cancelled"}

	m := project_picker.New(items, pal, onPick, onCreate, onCancel)
	m = m.SetSize(80, 24)
	return m, &pickedProject, &createdName
}

// dispatchKey sends a single key event to the model and returns the
// updated model plus the tea.Msg produced by any returned cmd.
func dispatchKey(m project_picker.Model, key string) (project_picker.Model, tea.Msg) {
	var kp tea.KeyPressMsg
	switch key {
	case "up":
		kp = tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		kp = tea.KeyPressMsg{Code: tea.KeyDown}
	case "enter":
		kp = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		kp = tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		kp = tea.KeyPressMsg{Code: tea.KeyTab}
	case "backspace":
		kp = tea.KeyPressMsg{Code: tea.KeyBackspace}
	default:
		// Single printable rune.
		kp = tea.KeyPressMsg{Text: key}
	}

	updated, cmd := m.Update(kp)
	var got tea.Msg
	if cmd != nil {
		got = cmd()
	}
	return updated, got
}

// TestUnit_Picker_FilterNarrowsList verifies that typing characters
// into the filter reduces the visible list to matching items only.
func TestUnit_Picker_FilterNarrowsList(t *testing.T) {
	t.Parallel()
	m, _, _ := newTestModel(testItems())

	// Type "al" — should match only "alpha".
	m, _ = dispatchKey(m, "a")
	m, _ = dispatchKey(m, "l")

	// Use plain view: fuzzy-match highlighting applies per-rune ANSI codes
	// that interleave between characters, so a raw strings.Contains on
	// "alpha" would fail even though the text is present.
	view := plainView(m)
	if !strings.Contains(view, "alpha") {
		t.Error("expected 'alpha' in filtered view")
	}
	if strings.Contains(view, "beta") {
		t.Error("expected 'beta' to be filtered out")
	}
	if strings.Contains(view, "gamma") {
		t.Error("expected 'gamma' to be filtered out")
	}
	// The sticky "+ Neu" entry must always be present.
	if !strings.Contains(view, "+ Neues Projekt anlegen") {
		t.Error("expected '+ Neues Projekt anlegen' to always appear in filtered view")
	}
}

// TestUnit_Picker_CursorWrapsDown verifies that pressing down from the
// last entry ("+Neu") wraps around to index 0.
func TestUnit_Picker_CursorWrapsDown(t *testing.T) {
	t.Parallel()
	items := testItems()
	m, _, _ := newTestModel(items)

	// Press down len(items) times to move from index 0 to index len(items)
	// (the "+Neu" pseudo-row). With items={alpha,beta,gamma} (len=3):
	//   down×1 → 1, down×2 → 2, down×3 → neuIdx=3.
	for i := 0; i < len(items); i++ {
		m, _ = dispatchKey(m, "down")
	}
	if m.Cursor() != len(items) {
		t.Fatalf("setup: expected cursor at %d (+Neu), got %d", len(items), m.Cursor())
	}
	// One more down wraps to index 0.
	m, _ = dispatchKey(m, "down")
	if m.Cursor() != 0 {
		t.Errorf("expected cursor to wrap to 0, got %d", m.Cursor())
	}
}

// TestUnit_Picker_CursorWrapsUp verifies that pressing up from index 0
// wraps to the last entry ("+Neu" position = len(filteredItems)).
func TestUnit_Picker_CursorWrapsUp(t *testing.T) {
	t.Parallel()
	items := testItems()
	m, _, _ := newTestModel(items)

	// Cursor starts at 0; press up once to wrap to last.
	m, _ = dispatchKey(m, "up")
	// last valid index = len(items) because "+Neu" is at that position
	want := len(items)
	if m.Cursor() != want {
		t.Errorf("expected cursor to wrap to %d, got %d", want, m.Cursor())
	}
}

// TestUnit_Picker_EnterOnNewWithFilter verifies that pressing enter when
// the cursor is on "+Neu" and the filter is non-empty emits onCreate(filter).
func TestUnit_Picker_EnterOnNewWithFilter(t *testing.T) {
	t.Parallel()
	m, _, createdName := newTestModel(testItems())

	// Type a term that produces no fuzzy matches.
	for _, ch := range "zzzquery" {
		m, _ = dispatchKey(m, string(ch))
	}
	// With no matches cursor is already on "+Neu" (index 0 = len(0 filtered items)).
	// Press enter.
	_, msg := dispatchKey(m, "enter")

	if *createdName != "zzzquery" {
		t.Errorf("expected onCreate called with 'zzzquery', got %q", *createdName)
	}
	f, ok := msg.(fakeMsg)
	if !ok {
		t.Fatalf("expected fakeMsg, got %T", msg)
	}
	if !strings.HasPrefix(f.name, "create:") {
		t.Errorf("expected create: msg, got %q", f.name)
	}
}

// TestUnit_Picker_EnterOnRow verifies that pressing enter on a regular
// row emits onPick(items[cursor]).
func TestUnit_Picker_EnterOnRow(t *testing.T) {
	t.Parallel()
	items := testItems()
	m, pickedProject, _ := newTestModel(items)

	// Move to index 1 (beta).
	m, _ = dispatchKey(m, "down")
	_, msg := dispatchKey(m, "enter")

	if pickedProject.Name != "beta" {
		t.Errorf("expected onPick called with 'beta', got %q", pickedProject.Name)
	}
	f, ok := msg.(fakeMsg)
	if !ok {
		t.Fatalf("expected fakeMsg, got %T", msg)
	}
	if !strings.HasPrefix(f.name, "picked:") {
		t.Errorf("expected picked: msg, got %q", f.name)
	}
}

// TestUnit_Picker_EscEmitsCancel verifies that pressing esc returns the
// onCancel message.
func TestUnit_Picker_EscEmitsCancel(t *testing.T) {
	t.Parallel()
	m, _, _ := newTestModel(testItems())

	_, msg := dispatchKey(m, "esc")
	f, ok := msg.(fakeMsg)
	if !ok {
		t.Fatalf("expected fakeMsg from esc, got %T", msg)
	}
	if f.name != "cancelled" {
		t.Errorf("expected 'cancelled', got %q", f.name)
	}
}

// TestUnit_Picker_BackspaceRemovesFilterRune verifies that backspace
// removes the last character from the filter text.
func TestUnit_Picker_BackspaceRemovesFilterRune(t *testing.T) {
	t.Parallel()
	m, _, _ := newTestModel(testItems())

	m, _ = dispatchKey(m, "a")
	m, _ = dispatchKey(m, "l")
	if m.Filter() != "al" {
		t.Errorf("expected filter 'al', got %q", m.Filter())
	}
	m, _ = dispatchKey(m, "backspace")
	if m.Filter() != "a" {
		t.Errorf("expected filter 'a' after backspace, got %q", m.Filter())
	}
}

// TestUnit_Picker_TabJumpsToNew verifies that pressing tab moves the
// cursor to the "+Neu" position (len(filteredItems)).
func TestUnit_Picker_TabJumpsToNew(t *testing.T) {
	t.Parallel()
	items := testItems()
	m, _, _ := newTestModel(items)

	// Start at 0; tab should jump to len(items).
	m, _ = dispatchKey(m, "tab")
	if m.Cursor() != len(items) {
		t.Errorf("expected cursor at %d ('+Neu'), got %d", len(items), m.Cursor())
	}
}

// TestUnit_Picker_ViewContainsAllItems verifies that View() renders all
// items (no filter) plus the "+Neu" entry and the filter hint text.
func TestUnit_Picker_ViewContainsAllItems(t *testing.T) {
	t.Parallel()
	items := testItems()
	m, _, _ := newTestModel(items)

	// No filter — all items visible. Use plainView to strip ANSI codes
	// so string searches work even if styles wrap individual characters.
	view := plainView(m)
	for _, label := range []string{"alpha", "beta", "gamma", "+ Neues Projekt anlegen", "Projekt wählen"} {
		if !strings.Contains(view, label) {
			t.Errorf("expected %q in view", label)
		}
	}
}

// TestUnit_Picker_EmptyItems verifies that when no projects exist the
// picker shows only the "+Neu" entry and cursor is at index 0.
func TestUnit_Picker_EmptyItems(t *testing.T) {
	t.Parallel()
	m, _, _ := newTestModel([]domain.Project{})

	view := plainView(m)
	if !strings.Contains(view, "+ Neues Projekt anlegen") {
		t.Error("expected '+ Neues Projekt anlegen' even with empty items list")
	}
	if m.Cursor() != 0 {
		t.Errorf("expected cursor at 0 for empty list, got %d", m.Cursor())
	}
}

// TestUnit_Picker_EnterOnNewEmptyItems verifies that pressing enter with
// an empty list (cursor=0="+Neu") calls onCreate("").
func TestUnit_Picker_EnterOnNewEmptyItems(t *testing.T) {
	t.Parallel()
	m, _, createdName := newTestModel([]domain.Project{})

	// Empty list: cursor=0 == "+Neu" position.
	_, msg := dispatchKey(m, "enter")
	f, ok := msg.(fakeMsg)
	if !ok {
		t.Fatalf("expected fakeMsg, got %T", msg)
	}
	if !strings.HasPrefix(f.name, "create:") {
		t.Errorf("expected create: msg, got %q", f.name)
	}
	// Filter is empty so createdName is ""
	if *createdName != "" {
		t.Errorf("expected empty created name, got %q", *createdName)
	}
}

// TestUnit_Picker_JKTypeIntoFilter verifies that j and k are treated as
// printable characters that append to the filter, not as navigation aliases.
// Typing "kj" should produce filter="kj" and leave the cursor at 0.
func TestUnit_Picker_JKTypeIntoFilter(t *testing.T) {
	t.Parallel()
	m, _, _ := newTestModel(testItems())

	m, _ = dispatchKey(m, "k")
	if m.Filter() != "k" {
		t.Errorf("expected filter \"k\" after pressing k, got %q", m.Filter())
	}
	if m.Cursor() != 0 {
		t.Errorf("expected cursor to stay at 0 after k, got %d", m.Cursor())
	}

	m, _ = dispatchKey(m, "j")
	if m.Filter() != "kj" {
		t.Errorf("expected filter \"kj\" after pressing k then j, got %q", m.Filter())
	}
	if m.Cursor() != 0 {
		t.Errorf("expected cursor to stay at 0 after kj, got %d", m.Cursor())
	}
}

// TestUnit_Picker_UpDownNavigation verifies that up/down (arrow keys) still
// navigate the cursor as before; j/k must NOT be aliases for them.
func TestUnit_Picker_UpDownNavigation(t *testing.T) {
	t.Parallel()
	m, _, _ := newTestModel(testItems())

	m, _ = dispatchKey(m, "down")
	if m.Cursor() != 1 {
		t.Errorf("expected cursor 1 after down, got %d", m.Cursor())
	}
	m, _ = dispatchKey(m, "up")
	if m.Cursor() != 0 {
		t.Errorf("expected cursor 0 after up, got %d", m.Cursor())
	}
}

// TestUnit_Picker_TabOnFilteredListJumpsToNew verifies that tab jumps to
// "+Neu" even when the list is filtered (len(filteredItems) != len(allItems)).
func TestUnit_Picker_TabOnFilteredListJumpsToNew(t *testing.T) {
	t.Parallel()
	m, _, _ := newTestModel(testItems())

	// Filter to "al" — only "alpha" matches, so filteredLen=1.
	m, _ = dispatchKey(m, "a")
	m, _ = dispatchKey(m, "l")
	// Tab should jump to len(filtered) = 1.
	m, _ = dispatchKey(m, "tab")
	if m.Cursor() != 1 {
		t.Errorf("expected cursor at 1 ('+Neu' in filtered view), got %d", m.Cursor())
	}
}
