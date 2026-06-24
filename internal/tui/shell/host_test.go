package shell_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// textStubRoute is a stubRoute variant that optionally captures text (CapturesText).
type textStubRoute struct {
	text bool
}

func (r textStubRoute) Title() string                           { return "stub" }
func (r textStubRoute) Init() tea.Cmd                          { return nil }
func (r textStubRoute) Update(tea.Msg) (shell.Route, tea.Cmd)  { return r, nil }
func (r textStubRoute) View(shell.Frame) string                { return "" }
func (r textStubRoute) KeyHints() []keyhint.Hint               { return nil }
func (r textStubRoute) CapturesText() bool                     { return r.text }

// isQuit runs cmd and returns true if it produces a tea.QuitMsg.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestRouteHost_viewAndQuit(t *testing.T) {
	h := shell.NewRouteHost(stubRoute{title: "Solo"}, theme.Default)
	m, _ := h.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_ = m.(shell.RouteHost).View() // must not panic
	_, cmd := h.Update(tea.KeyPressMsg{Text: "q"})
	if cmd == nil {
		t.Fatal("q should quit the host")
	}
}

// TestRouteHost_BackForwardsTextThenQuits verifies that:
//   - q in a text-capturing route is forwarded (BackForward) and must NOT quit;
//   - q on a clean leaf route triggers BackQuit.
func TestRouteHost_BackForwardsTextThenQuits(t *testing.T) {
	// Text-capturing route: q is literal input, must not quit.
	h := shell.NewRouteHost(textStubRoute{text: true}, theme.Default)
	if _, cmd := h.Update(tea.KeyPressMsg{Text: "q"}); isQuit(cmd) {
		t.Fatal("q in a text field must NOT quit standalone host")
	}

	// Clean leaf route: q triggers BackQuit.
	h2 := shell.NewRouteHost(textStubRoute{}, theme.Default)
	if _, cmd := h2.Update(tea.KeyPressMsg{Text: "q"}); !isQuit(cmd) {
		t.Fatal("q on a clean leaf must quit standalone host")
	}
}

// TestRouteHostInit exercises RouteHost.Init which delegates to route.Init().
func TestRouteHostInit(t *testing.T) {
	h := shell.NewRouteHost(textStubRoute{}, theme.Default)
	// textStubRoute.Init returns nil; RouteHost.Init should propagate that.
	cmd := h.Init()
	if cmd != nil {
		t.Error("RouteHost.Init should return nil when the underlying route returns nil")
	}
}
