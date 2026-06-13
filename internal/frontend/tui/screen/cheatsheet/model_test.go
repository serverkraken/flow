package cheatsheet_test

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown_overlay"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/cheatsheet"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/testutil"
)

func newModel(t *testing.T, content string, renderErr error) cheatsheet.Model {
	t.Helper()
	cs := &testutil.FakeCheatsheetReader{Content: content}
	r := &testutil.FakeMarkdownRenderer{Prefix: "[r] ", Err: renderErr}
	return cheatsheet.New(theme.Load(), cs, r)
}

func TestNew_BeforeWindowSize_ViewIsEmpty(t *testing.T) {
	m := newModel(t, "# hi", nil)
	if got := m.View().Content; got != "" {
		t.Errorf("View before WindowSizeMsg should be empty, got %q", got)
	}
	if m.FilterActive() || m.StateFilter() != "" || m.StateCursor() != 0 {
		t.Error("default filter/state must be empty")
	}
}

func TestInit_LoadsViaPort(t *testing.T) {
	m := newModel(t, "# Cheatsheet\n\nSome body.", nil)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init must return a tea.Cmd")
	}
	msg := cmd()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	updated, _ = updated.Update(msg)
	if got := updated.View().Content; !strings.Contains(got, "[r] # Cheatsheet") {
		t.Errorf("View should contain renderer-prefixed content, got:\n%s", got)
	}
}

func TestInit_LoadError_DisplaysFehler(t *testing.T) {
	cs := &testutil.FakeCheatsheetReader{Err: errors.New("boom")}
	r := &testutil.FakeMarkdownRenderer{}
	m := cheatsheet.New(theme.Load(), cs, r)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cmd := m.Init()
	updated, _ = updated.Update(cmd())
	if got := updated.View().Content; !strings.Contains(got, "Fehler: boom") {
		t.Errorf("View should surface load error, got:\n%s", got)
	}
}

func TestRendererErrorFallsBackToRaw(t *testing.T) {
	m := newModel(t, "raw content", errors.New("renderer dead"))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cmd := m.Init()
	updated, _ = updated.Update(cmd())
	if got := updated.View().Content; !strings.Contains(got, "raw content") {
		t.Errorf("on renderer failure View should show raw content, got:\n%s", got)
	}
}

// isQuitCmd executes cmd (if non-nil) and returns true if the message is
// tea.QuitMsg. nil cmd → not quit.
func isQuitCmd(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	return ok
}

func TestExitMsg_EmbeddedDoesNotQuit(t *testing.T) {
	// Embedded (default, no WithStandalone) must NOT quit on ExitMsg —
	// the sidekick fan-out delivers this to every screen including cheatsheet
	// when any overlay closes anywhere.
	m := newModel(t, "# hi", nil)
	_, cmd := m.Update(markdown_overlay.ExitMsg{})
	if isQuitCmd(cmd) {
		t.Error("embedded cheatsheet must not tea.Quit on ExitMsg; got quit cmd")
	}
}

func TestExitMsg_StandaloneQuits(t *testing.T) {
	// Standalone mode (flow cheatsheet popup) must quit on ExitMsg so the
	// tmux popup closes when the user presses q/esc.
	cs := &testutil.FakeCheatsheetReader{Content: "# hi"}
	r := &testutil.FakeMarkdownRenderer{Prefix: "[r] "}
	m := cheatsheet.New(theme.Load(), cs, r, cheatsheet.WithStandalone())
	_, cmd := m.Update(markdown_overlay.ExitMsg{})
	if !isQuitCmd(cmd) {
		t.Error("standalone cheatsheet must return tea.Quit on ExitMsg")
	}
}

func TestConsumesKeys_ClaimsC(t *testing.T) {
	m := newModel(t, "# hi", nil)
	keys := m.ConsumesKeys()
	for _, k := range keys {
		if k == "c" {
			return
		}
	}
	t.Errorf("ConsumesKeys() = %v; want it to contain \"c\"", keys)
}

func TestView_NonPanicSmoke(t *testing.T) {
	m := newModel(t, "# Hi\n", nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	cmd := m.Init()
	updated, _ = updated.Update(cmd())
	out := updated.View().Content
	if out == "" {
		t.Error("View() must produce output once loaded + sized")
	}
	if !strings.Contains(out, "Cheatsheet") {
		t.Errorf("View() must mention the title, got:\n%s", out)
	}
}
