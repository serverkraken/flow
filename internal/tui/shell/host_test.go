package shell_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

func TestRouteHost_viewAndQuit(t *testing.T) {
	h := shell.NewRouteHost(stubRoute{title: "Solo"}, theme.Default)
	m, _ := h.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_ = m.(shell.RouteHost).View() // must not panic
	_, cmd := h.Update(tea.KeyPressMsg{Text: "q"})
	if cmd == nil {
		t.Fatal("q should quit the host")
	}
}
