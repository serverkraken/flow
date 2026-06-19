package wtnav_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

type fakeRoute struct{ title string }

func (f fakeRoute) Title() string                         { return f.title }
func (f fakeRoute) Init() tea.Cmd                         { return nil }
func (f fakeRoute) Update(tea.Msg) (shell.Route, tea.Cmd) { return f, nil }
func (f fakeRoute) View(shell.Frame) string               { return f.title }
func (f fakeRoute) KeyHints() []keyhint.Hint              { return nil }

func TestRegistry_NavEmitsSwitchForKnownKey(t *testing.T) {
	reg := wtnav.Registry{"w": func() shell.Route { return fakeRoute{title: "Woche"} }}
	cmd := reg.Nav("w")
	if cmd == nil {
		t.Fatal("Nav(known key) should return a cmd")
	}
	msg, ok := cmd().(shell.SwitchRouteMsg)
	if !ok {
		t.Fatalf("Nav cmd should emit SwitchRouteMsg, got %T", cmd())
	}
	if msg.Route.Title() != "Woche" {
		t.Fatalf("switch target = %q, want Woche", msg.Route.Title())
	}
}

func TestRegistry_NavNilForUnknownKey(t *testing.T) {
	reg := wtnav.Registry{}
	if reg.Nav("z") != nil {
		t.Fatal("Nav(unknown key) should return nil")
	}
}
