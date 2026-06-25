package wtnav_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// stubRoute is a minimal shell.Route for registry factories.
type stubRoute struct{ title string }

func (s stubRoute) Title() string                         { return s.title }
func (s stubRoute) Init() tea.Cmd                         { return nil }
func (s stubRoute) Update(tea.Msg) (shell.Route, tea.Cmd) { return s, nil }
func (s stubRoute) View(shell.Frame) string               { return "" }
func (s stubRoute) KeyHints() []keyhint.Hint              { return nil }

func key(c rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: c} }
func run(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func TestRegistry_NavEmitsSwitchForKnownKey(t *testing.T) {
	reg := wtnav.Registry{"w": func() shell.Route { return stubRoute{title: "Woche"} }}
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

func testReg() wtnav.Registry {
	return wtnav.Registry{
		"w": func() shell.Route { return stubRoute{title: "Woche"} },
		"t": func() shell.Route { return stubRoute{title: "Stats"} },
		"d": func() shell.Route { return stubRoute{title: "Frei"} },
		"e": func() shell.Route { return stubRoute{title: "Export"} },
	}
}

func TestStrip_ContainsFourLabelsNotExport(t *testing.T) {
	out := wtnav.Strip(wtnav.IdxStats, 200, theme.Default)
	for _, l := range []string{"Heute", "Woche", "Stats", "Frei"} {
		if !strings.Contains(out, l) {
			t.Fatalf("strip missing %q: %q", l, out)
		}
	}
	if strings.Contains(out, "Export") {
		t.Fatalf("Export must no longer be a strip tab: %q", out)
	}
}

func TestLateral_RightFromHeutePushesWoche(t *testing.T) {
	m := run(wtnav.Lateral(testReg(), wtnav.IdxHeute, key(tea.KeyRight)))
	sw, ok := m.(shell.SwitchRouteMsg)
	if !ok || sw.Route.Title() != "Woche" {
		t.Fatalf("→ from Heute = %#v, want SwitchRouteMsg(Woche)", m)
	}
}

func TestLateral_LeftFromWochePopsToHeute(t *testing.T) {
	if _, ok := run(wtnav.Lateral(testReg(), wtnav.IdxWoche, key(tea.KeyLeft))).(shell.PopRouteMsg); !ok {
		t.Fatal("← from Woche must emit PopRouteMsg (back to Heute root)")
	}
}

func TestLateral_RightFromStatsSwitchesFrei(t *testing.T) {
	m := run(wtnav.Lateral(testReg(), wtnav.IdxStats, key(tea.KeyRight)))
	sw, ok := m.(shell.SwitchRouteMsg)
	if !ok || sw.Route.Title() != "Frei" {
		t.Fatalf("→ from Stats = %#v, want SwitchRouteMsg(Frei)", m)
	}
}

func TestLateral_ClampsAtEnds(t *testing.T) {
	if wtnav.Lateral(testReg(), wtnav.IdxHeute, key(tea.KeyLeft)) != nil {
		t.Fatal("← from Heute must clamp to nil")
	}
}

func TestLateral_RightFromFreiClamps(t *testing.T) {
	if wtnav.Lateral(testReg(), wtnav.IdxFrei, key(tea.KeyRight)) != nil {
		t.Fatal("→ from Frei (last tab) must clamp to nil")
	}
}

func TestLateral_NonArrowIsNil(t *testing.T) {
	if wtnav.Lateral(testReg(), wtnav.IdxWoche, tea.KeyPressMsg{Text: "x"}) != nil {
		t.Fatal("non-arrow key must return nil")
	}
}
