package shell_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

func newShell() shell.Shell {
	return shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{
		shell.NewHomeRoute(nil, theme.Default, "alice"),
		stubRoute{title: "Worktime", push: stubRoute{title: "Detail"}},
	})
}

func TestShell_windowSize(t *testing.T) {
	next, _ := newShell().Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	s := next.(shell.Shell)
	if s.Width() != 120 || s.Height() != 40 {
		t.Fatalf("size = %dx%d", s.Width(), s.Height())
	}
}

func TestShell_tabSwitchByDigitAndTab(t *testing.T) {
	next, _ := newShell().Update(tea.KeyPressMsg{Text: "2"})
	if next.(shell.Shell).ActiveTab() != 1 {
		t.Fatal("digit 2 -> tab 1")
	}
	next2, _ := newShell().Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if next2.(shell.Shell).ActiveTab() != 1 {
		t.Fatal("tab -> next")
	}
}

func TestShell_paletteOpenClose(t *testing.T) {
	next, _ := newShell().Update(tea.KeyPressMsg{Text: ":"})
	s := next.(shell.Shell)
	if !s.PaletteOpen() {
		t.Fatal("':' opens palette")
	}
	// Esc inside palette emits PaletteDismissedMsg; feed it back.
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	next2, _ := s.Update(cmd())
	if next2.(shell.Shell).PaletteOpen() {
		t.Fatal("dismiss closes palette")
	}
}

func TestShell_drillDownAndBack(t *testing.T) {
	// Switch to tab 1 (stub that pushes "Detail" on Enter).
	s, _ := newShell().Update(tea.KeyPressMsg{Text: "2"})
	sh := s.(shell.Shell)
	// Enter -> route emits PushRouteMsg; feed the produced msg back.
	_, cmd := sh.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should produce a push command")
	}
	pushed, _ := sh.Update(cmd())
	sh = pushed.(shell.Shell)
	if sh.ActiveDepth() != 2 {
		t.Fatalf("after push depth = %d want 2", sh.ActiveDepth())
	}
	// Esc pops back.
	back, _ := sh.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if back.(shell.Shell).ActiveDepth() != 1 {
		t.Fatal("esc should pop back to depth 1")
	}
}

// initCountRoute records how often its Init() is invoked (through a shared
// pointer, so value copies inside the NavStack still count).
type initCountRoute struct {
	stubRoute
	calls *int
}

func (r initCountRoute) Init() tea.Cmd { *r.calls++; return nil }

// Popping back to a revealed route must re-Init it, just like push/switch/tab
// switch do — so a root route that paused background work (e.g. Today's live
// clock) while drilled-over resumes when it becomes the active top again.
func TestShell_pop_reinitsRevealedRoute(t *testing.T) {
	var rootInit, childInit int
	root := initCountRoute{stubRoute{title: "Worktime"}, &rootInit}
	child := initCountRoute{stubRoute{title: "Woche"}, &childInit}

	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{root})
	next, _ := s.Update(shell.PushRouteMsg{Route: child})
	sh := next.(shell.Shell)
	if sh.ActiveDepth() != 2 {
		t.Fatalf("after push depth = %d, want 2", sh.ActiveDepth())
	}
	before := rootInit
	next2, _ := sh.Update(shell.PopRouteMsg{})
	sh2 := next2.(shell.Shell)
	if sh2.ActiveDepth() != 1 {
		t.Fatalf("after pop depth = %d, want 1", sh2.ActiveDepth())
	}
	if rootInit != before+1 {
		t.Fatalf("pop should re-Init the revealed route (rootInit=%d, want %d)", rootInit, before+1)
	}
}

func TestShell_initsActiveTabRouteAtStartup(t *testing.T) {
	var home, work int
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{
		initCountRoute{stubRoute{title: "Home"}, &home},
		initCountRoute{stubRoute{title: "Worktime"}, &work},
	})
	_ = s.Init()
	if home != 1 {
		t.Fatalf("active tab Init calls = %d, want 1", home)
	}
}

func TestShell_initsTabRouteOnSwitch(t *testing.T) {
	var home, work int
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{
		initCountRoute{stubRoute{title: "Home"}, &home},
		initCountRoute{stubRoute{title: "Worktime"}, &work},
	})
	_ = s.Init()
	if _, _ = s.Update(tea.KeyPressMsg{Text: "2"}); work != 1 {
		t.Fatalf("switched-to tab Init calls = %d, want 1", work)
	}
}

func TestShell_quit(t *testing.T) {
	_, cmd := newShell().Update(tea.KeyPressMsg{Text: "q"})
	if cmd == nil {
		t.Fatal("q should quit")
	}
}

func TestShell_viewNoPanic(t *testing.T) {
	s, _ := newShell().Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("View panicked: %v", r)
		}
	}()
	_ = s.(shell.Shell).View()
}

// captureRoute is a stubRoute that captures input and records keys it receives.
type captureRoute struct {
	stubRoute
	capturing bool
	gotKeys   *[]string
}

func (r captureRoute) CapturesInput() bool { return r.capturing }
func (r captureRoute) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		*r.gotKeys = append(*r.gotKeys, k.Text)
	}
	return r, nil
}

func TestShell_capturingRouteReceivesDigitInsteadOfTabSwitch(t *testing.T) {
	var keys []string
	cap := captureRoute{stubRoute{title: "Form"}, true, &keys}
	other := stubRoute{title: "Other"}
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{cap, other})

	// Active tab 0 captures: "2" must reach the route, NOT switch to tab 1.
	next, _ := s.Update(tea.KeyPressMsg{Text: "2"})
	sh := next.(shell.Shell)
	if sh.ActiveTab() != 0 {
		t.Fatalf("capturing route: digit must not switch tab (activeTab=%d)", sh.ActiveTab())
	}
	if len(keys) != 1 || keys[0] != "2" {
		t.Fatalf("capturing route should receive '2', got %v", keys)
	}
}

func TestShell_nonCapturingRouteStillSwitchesTabOnDigit(t *testing.T) {
	var keys []string
	cap := captureRoute{stubRoute{title: "Form"}, false, &keys}
	other := stubRoute{title: "Other"}
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{cap, other})

	next, _ := s.Update(tea.KeyPressMsg{Text: "2"})
	if next.(shell.Shell).ActiveTab() != 1 {
		t.Fatal("non-capturing route: digit '2' should switch to tab 1")
	}
}

func TestShell_switchTabByTitle(t *testing.T) {
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{
		stubRoute{title: "Home"},
		stubRoute{title: "Worktime"},
		stubRoute{title: "Docs"},
	})
	next, _ := s.Update(shell.SwitchTabMsg{Title: "Docs"})
	if next.(shell.Shell).ActiveTab() != 2 {
		t.Fatalf("SwitchTabMsg{Docs} should activate tab 2, got %d", next.(shell.Shell).ActiveTab())
	}
	// Unknown title is a no-op (stays put).
	again, _ := next.(shell.Shell).Update(shell.SwitchTabMsg{Title: "Nope"})
	if again.(shell.Shell).ActiveTab() != 2 {
		t.Fatalf("unknown SwitchTabMsg should be a no-op, got %d", again.(shell.Shell).ActiveTab())
	}
}

func TestShell_withActiveTabStartsThere(t *testing.T) {
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{
		stubRoute{title: "Home"},
		stubRoute{title: "Worktime"},
		stubRoute{title: "Docs"},
	}).WithActiveTab(1)
	if s.ActiveTab() != 1 {
		t.Fatalf("WithActiveTab(1) => ActiveTab %d, want 1", s.ActiveTab())
	}
	// Out of range clamps to 0.
	if shell.New(nil, "a", theme.Default).WithActiveTab(9).ActiveTab() != 0 {
		t.Fatal("WithActiveTab(out-of-range) should clamp to 0")
	}
}

func TestShell_switchRoute_pushesAtRootThenReplaces(t *testing.T) {
	var rootInit, aInit, bInit int
	root := initCountRoute{stubRoute{title: "Worktime"}, &rootInit}
	routeA := initCountRoute{stubRoute{title: "Woche"}, &aInit}
	routeB := initCountRoute{stubRoute{title: "Stats"}, &bInit}

	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{root})

	// At root (depth 1): SwitchRouteMsg pushes -> depth 2, crumb tip "Woche".
	next, _ := s.Update(shell.SwitchRouteMsg{Route: routeA})
	sh := next.(shell.Shell)
	if sh.ActiveDepth() != 2 {
		t.Fatalf("after switch at root depth = %d, want 2", sh.ActiveDepth())
	}
	if aInit != 1 { // the Shell must Init the pushed route (stub Init returns nil)
		t.Fatalf("switch at root should Init the new route (aInit=%d)", aInit)
	}

	// In a sibling (depth 2): SwitchRouteMsg replaces top -> stays depth 2.
	next2, _ := sh.Update(shell.SwitchRouteMsg{Route: routeB})
	sh2 := next2.(shell.Shell)
	if sh2.ActiveDepth() != 2 {
		t.Fatalf("after lateral switch depth = %d, want 2 (replace, not push)", sh2.ActiveDepth())
	}
	if bInit != 1 {
		t.Fatalf("lateral switch should Init the replacement (bInit=%d)", bInit)
	}
}
