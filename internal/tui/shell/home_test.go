package shell_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/shell"
)

func TestHomeRoute_basics(t *testing.T) {
	r := shell.NewHomeRoute("alice")
	if r.Title() != "Home" {
		t.Fatalf("title %q", r.Title())
	}
	if !strings.Contains(r.View(shell.Frame{Width: 80, Height: 20}), "alice") {
		t.Fatal("home view should contain user")
	}
	if len(r.KeyHints()) == 0 {
		t.Fatal("home should expose key hints")
	}
}

func TestHomeRoute_enterPushesAbout(t *testing.T) {
	r := shell.NewHomeRoute("alice")
	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should emit a command")
	}
	msg, ok := cmd().(shell.PushRouteMsg)
	if !ok || msg.Route.Title() != "About" {
		t.Fatalf("enter should push AboutRoute, got %#v", cmd())
	}
}
