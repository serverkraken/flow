package cheatsheet_test

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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
	if got := m.View(); got != "" {
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
	if got := updated.View(); !strings.Contains(got, "[r] # Cheatsheet") {
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
	if got := updated.View(); !strings.Contains(got, "Fehler: boom") {
		t.Errorf("View should surface load error, got:\n%s", got)
	}
}

func TestRendererErrorFallsBackToRaw(t *testing.T) {
	m := newModel(t, "raw content", errors.New("renderer dead"))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cmd := m.Init()
	updated, _ = updated.Update(cmd())
	if got := updated.View(); !strings.Contains(got, "raw content") {
		t.Errorf("on renderer failure View should show raw content, got:\n%s", got)
	}
}

func TestView_NonPanicSmoke(t *testing.T) {
	m := newModel(t, "# Hi\n", nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	cmd := m.Init()
	updated, _ = updated.Update(cmd())
	out := updated.View()
	if out == "" {
		t.Error("View() must produce output once loaded + sized")
	}
	if !strings.Contains(out, "Cheatsheet") {
		t.Errorf("View() must mention the title, got:\n%s", out)
	}
}
