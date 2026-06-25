package shell_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// stubRoute is a minimal Route used across the shell tests.
type stubRoute struct {
	title string
	hints []keyhint.Hint
	push  shell.Route // if set, Update on Enter pushes this route
}

func (s stubRoute) Title() string { return s.title }
func (s stubRoute) Init() tea.Cmd { return nil }
func (s stubRoute) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok && k.Code == tea.KeyEnter && s.push != nil {
		next := s.push
		return s, func() tea.Msg { return shell.PushRouteMsg{Route: next} }
	}
	return s, nil
}
func (s stubRoute) View(f shell.Frame) string { return s.title }
func (s stubRoute) KeyHints() []keyhint.Hint  { return s.hints }

func TestRoute_satisfiedByStub(t *testing.T) {
	var r shell.Route = stubRoute{title: "Home"}
	if r.Title() != "Home" {
		t.Fatalf("got %q", r.Title())
	}
	if r.View(shell.Frame{Width: 10, Height: 5}) != "Home" {
		t.Fatal("view")
	}
}
