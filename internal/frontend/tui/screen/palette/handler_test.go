package palette_test

// Coverage for handleNormalKey navigation branches, jumpSection,
// handleFilterKey edges, and renderEmptyState that the existing
// model_test.go suite leaves uncovered.

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/palette"
)

func makeFixtureWithSections() *fixture {
	return newFixture(
		domain.PaletteEntry{Label: "A1", Action: "x", Section: "System"},
		domain.PaletteEntry{Label: "A2", Action: "x", Section: "System"},
		domain.PaletteEntry{Label: "A3", Action: "x", Section: "System"},
		domain.PaletteEntry{Label: "B1", Action: "x", Section: "Misc"},
		domain.PaletteEntry{Label: "B2", Action: "x", Section: "Misc"},
		domain.PaletteEntry{Label: "C1", Action: "x", Section: "Workflow"},
	)
}

// — handleNormalKey navigation —

func TestHandleNormalKey_JK_MovesCursor(t *testing.T) {
	f := makeFixtureWithSections()
	m := runUntilLoaded(t, f.model())
	// j three times → cursor 3
	for i := 0; i < 3; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Text: "j"})
	}
	if got := m.(palette.Model).StateCursor(); got != 3 {
		t.Errorf("cursor after 3×j: got %d want 3", got)
	}
	// k twice → 1
	for i := 0; i < 2; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Text: "k"})
	}
	if got := m.(palette.Model).StateCursor(); got != 1 {
		t.Errorf("cursor after 2×k: got %d want 1", got)
	}
}

func TestHandleNormalKey_GAndCapitalG(t *testing.T) {
	f := makeFixtureWithSections()
	m := runUntilLoaded(t, f.model())
	m, _ = m.Update(tea.KeyPressMsg{Text: "G"})
	if got := m.(palette.Model).StateCursor(); got != 5 {
		t.Errorf("G should jump to last (5), got %d", got)
	}
	m, _ = m.Update(tea.KeyPressMsg{Text: "g"})
	if got := m.(palette.Model).StateCursor(); got != 0 {
		t.Errorf("g should jump to 0, got %d", got)
	}
}

func TestHandleNormalKey_PgDownPgUp(t *testing.T) {
	f := makeFixtureWithSections()
	m := runUntilLoaded(t, f.model())
	// pgdown advances by maxVisible (capped at len-1)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	if got := m.(palette.Model).StateCursor(); got <= 0 {
		t.Errorf("pgdown should move cursor forward, got %d", got)
	}
	// pgup moves back toward 0
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if got := m.(palette.Model).StateCursor(); got != 0 {
		t.Errorf("pgup should clamp to 0 from a single pgdown, got %d", got)
	}
}

func TestHandleNormalKey_CtrlDCtrlU(t *testing.T) {
	f := makeFixtureWithSections()
	m := runUntilLoaded(t, f.model())
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyCtrlD})
	if got := m.(palette.Model).StateCursor(); got <= 0 {
		t.Errorf("ctrl+d should move cursor forward, got %d", got)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyCtrlU})
	if got := m.(palette.Model).StateCursor(); got != 0 {
		t.Errorf("ctrl+u should clamp to 0, got %d", got)
	}
}

func TestHandleNormalKey_BracketsJumpSection(t *testing.T) {
	f := makeFixtureWithSections()
	m := runUntilLoaded(t, f.model())
	// ] from cursor 0 (first System) → first Misc (index 3)
	m, _ = m.Update(tea.KeyPressMsg{Text: "]"})
	if got := m.(palette.Model).StateCursor(); got != 3 {
		t.Errorf("] from System[0] should land on Misc[0]=3, got %d", got)
	}
	// ] again → first Workflow (index 5)
	m, _ = m.Update(tea.KeyPressMsg{Text: "]"})
	if got := m.(palette.Model).StateCursor(); got != 5 {
		t.Errorf("] from Misc[0] should land on Workflow[0]=5, got %d", got)
	}
	// ] from last section is no-op
	m, _ = m.Update(tea.KeyPressMsg{Text: "]"})
	if got := m.(palette.Model).StateCursor(); got != 5 {
		t.Errorf("] from last section should stay at 5, got %d", got)
	}
}

func TestHandleNormalKey_BracketsJumpSectionBack(t *testing.T) {
	f := makeFixtureWithSections()
	m := runUntilLoaded(t, f.model())
	// Move to Workflow first via two ]
	m, _ = m.Update(tea.KeyPressMsg{Text: "]"})
	m, _ = m.Update(tea.KeyPressMsg{Text: "]"})
	// [ from Workflow[0] → first Misc[0]=3 (jumps over the two-step branch:
	// since cursor is already at section top, jump to start of previous).
	m, _ = m.Update(tea.KeyPressMsg{Text: "["})
	if got := m.(palette.Model).StateCursor(); got != 3 {
		t.Errorf("[ from Workflow[0] should land on Misc[0]=3, got %d", got)
	}
	// Now scroll mid-Misc with j, then [ should jump to top of Misc first.
	m, _ = m.Update(tea.KeyPressMsg{Text: "j"})
	if got := m.(palette.Model).StateCursor(); got != 4 {
		t.Errorf("after j: cursor 4, got %d", got)
	}
	m, _ = m.Update(tea.KeyPressMsg{Text: "["})
	if got := m.(palette.Model).StateCursor(); got != 3 {
		t.Errorf("[ from mid-section should jump to section start (3), got %d", got)
	}
	// [ again → System[0] = 0
	m, _ = m.Update(tea.KeyPressMsg{Text: "["})
	if got := m.(palette.Model).StateCursor(); got != 0 {
		t.Errorf("[ from Misc[0] should land on System[0]=0, got %d", got)
	}
	// [ again → no-op (already at first section)
	m, _ = m.Update(tea.KeyPressMsg{Text: "["})
	if got := m.(palette.Model).StateCursor(); got != 0 {
		t.Errorf("[ from first section should stay at 0, got %d", got)
	}
}

func TestHandleNormalKey_DigitDirectDispatch(t *testing.T) {
	f := makeFixtureWithSections()
	m := runUntilLoaded(t, f.model())
	m, cmd := m.Update(tea.KeyPressMsg{Text: "3"})
	if cmd == nil {
		t.Fatal("digit 3 should dispatch")
	}
	_ = cmd()
	if got := m.(palette.Model).StateCursor(); got != 2 {
		t.Errorf("digit 3 should set cursor to 2 (3-1), got %d", got)
	}
	if len(f.tmux.Actions) != 1 {
		t.Errorf("digit dispatch should call RunTmuxAction once, got %d", len(f.tmux.Actions))
	}
}

func TestHandleNormalKey_DigitOutOfRange_NoDispatch(t *testing.T) {
	f := makeFixtureWithSections()
	m := runUntilLoaded(t, f.model())
	// 9 is out of range with 6 entries
	m, cmd := m.Update(tea.KeyPressMsg{Text: "9"})
	if cmd != nil {
		t.Errorf("digit 9 with 6 entries should not dispatch, got cmd=%v", cmd)
	}
	if len(f.tmux.Actions) != 0 {
		t.Errorf("expected no RunTmuxAction calls, got %d", len(f.tmux.Actions))
	}
	_ = m
}

func TestHandleNormalKey_DotPinFromEmpty_NoOp(t *testing.T) {
	f := newFixture() // zero entries
	m := runUntilLoaded(t, f.model())
	m, cmd := m.Update(tea.KeyPressMsg{Text: "."})
	if cmd != nil {
		t.Errorf(". with no entries must be a no-op, got cmd=%v", cmd)
	}
	_ = m
}

func TestHandleNormalKey_TypeToFilter_AutoFocuses(t *testing.T) {
	f := makeFixtureWithSections()
	m := runUntilLoaded(t, f.model())
	// "a" auto-focuses filter and routes the keystroke into it.
	m, _ = m.Update(tea.KeyPressMsg{Text: "a"})
	if !m.(palette.Model).FilterActive() {
		t.Error("typing a printable char should auto-focus filter")
	}
	if got := m.(palette.Model).StateFilter(); got == "" {
		t.Errorf("filter should carry typed char, got empty")
	}
}

// — handleFilterKey edges —

func TestHandleFilterKey_EscClearsFirst_BlursSecond(t *testing.T) {
	f := makeFixtureWithSections()
	m := runUntilLoaded(t, f.model())
	m, _ = m.Update(tea.KeyPressMsg{Text: "/"})
	for _, r := range "abc" {
		m, _ = m.Update(tea.KeyPressMsg{Text: string(r)})
	}
	// First esc clears the value AND blurs so j/k navigate again.
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil {
		t.Errorf("first esc should clear, not produce a cmd, got %v", cmd)
	}
	if got := m.(palette.Model).StateFilter(); got != "" {
		t.Errorf("first esc should clear filter, got %q", got)
	}
	if m.(palette.Model).FilterActive() {
		t.Error("first esc should also blur the filter")
	}
	// Second esc on the now-blurred empty filter is a no-op — palette
	// must NOT tea.Quit here because that would tear down the sidekick
	// host. The host owns the quit key.
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil {
		t.Errorf("second esc must not produce tea.Quit (would kill sidekick), got %v", cmd)
	}
}

func TestHandleFilterKey_EnterDispatches(t *testing.T) {
	f := makeFixtureWithSections()
	m := runUntilLoaded(t, f.model())
	m, _ = m.Update(tea.KeyPressMsg{Text: "/"})
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter from filter should dispatch")
	}
	_ = cmd()
	if len(f.tmux.Actions) != 1 {
		t.Errorf("expected 1 RunTmuxAction call, got %d", len(f.tmux.Actions))
	}
	_ = m
}

func TestHandleFilterKey_EnterEmptyEntries_NoOp(t *testing.T) {
	f := newFixture()
	m := runUntilLoaded(t, f.model())
	m, _ = m.Update(tea.KeyPressMsg{Text: "/"})
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Error("enter with no entries should not dispatch")
	}
}

func TestHandleFilterKey_BackspaceEditsValue(t *testing.T) {
	f := makeFixtureWithSections()
	m := runUntilLoaded(t, f.model())
	m, _ = m.Update(tea.KeyPressMsg{Text: "/"})
	for _, r := range "abc" {
		m, _ = m.Update(tea.KeyPressMsg{Text: string(r)})
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if got := m.(palette.Model).StateFilter(); got != "ab" {
		t.Errorf("after backspace got %q want ab", got)
	}
}

// — renderEmptyState —

func TestRenderEmptyState_NoEntries_RendersHint(t *testing.T) {
	f := newFixture()
	m := runUntilLoaded(t, f.model())
	out := m.View()
	if !strings.Contains(out, "noch keine Aktionen geladen") {
		t.Errorf("empty entries should render the no-plugins hint, got:\n%s", out)
	}
}

func TestRenderEmptyState_FilteredToZero_RendersHint(t *testing.T) {
	f := makeFixtureWithSections()
	m := runUntilLoaded(t, f.model())
	m, _ = m.Update(tea.KeyPressMsg{Text: "/"})
	for _, r := range "zzzzz" {
		m, _ = m.Update(tea.KeyPressMsg{Text: string(r)})
	}
	out := m.View()
	if !strings.Contains(out, "keine Treffer für") {
		t.Errorf("filtered empty should render »keine Treffer«, got:\n%s", out)
	}
}
