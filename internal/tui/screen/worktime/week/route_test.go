package week_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/week"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

type fakeAPI struct {
	days []apiclient.WeekDay
	err  error
}

func (f fakeAPI) GetWeek(_ context.Context, _ string) ([]apiclient.WeekDay, error) {
	return f.days, f.err
}

func (f fakeAPI) ListDayOffs(_ context.Context, _, _ string) ([]apiclient.DayOff, error) {
	return nil, nil
}

func drain(r shell.Route, cmd tea.Cmd) shell.Route {
	for i := 0; cmd != nil && i < 20; i++ {
		msg := cmd()
		if msg == nil {
			break
		}
		r, cmd = r.Update(msg)
	}
	return r
}

// stubTitle is a minimal Route used as a nav target in tests.
type stubTitle string

func (s stubTitle) Title() string                          { return string(s) }
func (s stubTitle) Init() tea.Cmd                          { return nil }
func (s stubTitle) Update(tea.Msg) (shell.Route, tea.Cmd) { return s, nil }
func (s stubTitle) View(shell.Frame) string                { return string(s) }
func (s stubTitle) KeyHints() []keyhint.Hint               { return nil }

func TestWeek_KeyHintsAdvertiseExport(t *testing.T) {
	r := week.NewRoute(nil, theme.Default, nil)
	found := false
	for _, h := range r.KeyHints() {
		if h.Key == "e" && h.Desc == "Export" {
			found = true
		}
	}
	if !found {
		t.Fatal("Woche KeyHints must advertise {e, Export} now that Export left the strip")
	}
}

func TestWeekRoute_rendersDays(t *testing.T) {
	api := fakeAPI{days: []apiclient.WeekDay{
		{Date: "2026-06-15", LoggedMin: 480, TargetMin: 480, Workday: true},
		{Date: "2026-06-16", LoggedMin: 120, TargetMin: 480, IsToday: true, Workday: true},
	}}
	var r shell.Route = week.NewRoute(api, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "2026-06-15") || !strings.Contains(body, "2026-06-16") {
		t.Fatalf("missing day rows:\n%s", body)
	}
	if !strings.Contains(body, "8h 00m") {
		t.Fatalf("missing formatted time for full day:\n%s", body)
	}
	if !strings.Contains(body, "2h 00m") {
		t.Fatalf("missing formatted logged time for partial day:\n%s", body)
	}
	if r.Title() != "Woche" {
		t.Fatalf("title = %q, want Woche", r.Title())
	}
}

func TestWeekRoute_navEmitsSwitch(t *testing.T) {
	reg := wtnav.Registry{"t": func() shell.Route { return stubTitle("Stats") }}
	r := week.NewRoute(fakeAPI{}, theme.Default, reg)
	_, cmd := r.Update(tea.KeyPressMsg{Text: "t"})
	if cmd == nil {
		t.Fatal("pressing t should emit a switch cmd")
	}
	if _, ok := cmd().(shell.SwitchRouteMsg); !ok {
		t.Fatalf("t should emit SwitchRouteMsg, got %T", cmd())
	}
}

// TestWeekRoute_loadingState verifies the placeholder text before data arrives.
func TestWeekRoute_loadingState(t *testing.T) {
	r := week.NewRoute(fakeAPI{}, theme.Default, wtnav.Registry{})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "lädt") {
		t.Fatalf("loading state should show 'lädt'; got:\n%s", body)
	}
}

// TestWeekRoute_errorState verifies the error text when load fails.
func TestWeekRoute_errorState(t *testing.T) {
	api := fakeAPI{err: errors.New("network timeout")}
	var r shell.Route = week.NewRoute(api, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Fehler") {
		t.Fatalf("error state should show 'Fehler'; got:\n%s", body)
	}
	if !strings.Contains(body, "network timeout") {
		t.Fatalf("error state should contain error message; got:\n%s", body)
	}
}

// TestWeekRoute_keyHints verifies KeyHints returns non-empty hints.
func TestWeekRoute_keyHints(t *testing.T) {
	r := week.NewRoute(fakeAPI{}, theme.Default, wtnav.Registry{})
	hints := r.KeyHints()
	if len(hints) == 0 {
		t.Fatal("KeyHints should return non-empty hints")
	}
}

// TestWeekRoute_unknownKeyNoCmd verifies a non-nav key returns nil cmd.
func TestWeekRoute_unknownKeyNoCmd(t *testing.T) {
	r := week.NewRoute(fakeAPI{}, theme.Default, wtnav.Registry{})
	_, cmd := r.Update(tea.KeyPressMsg{Text: "x"})
	if cmd != nil {
		t.Fatal("unknown key should return nil cmd")
	}
}

// TestWeekRoute_sseSessionEvent verifies that a session event triggers reload.
func TestWeekRoute_sseSessionEvent(t *testing.T) {
	var r shell.Route = week.NewRoute(fakeAPI{}, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	_, cmd := r.Update(shell.EventMsg{Ev: apiclient.ClientEvent{Type: string(domain.EventSessionStarted)}})
	if cmd == nil {
		t.Fatal("session.started event should trigger reload cmd")
	}
}

// TestWeekRoute_sseNonSessionEvent verifies that a non-session event returns nil cmd.
func TestWeekRoute_sseNonSessionEvent(t *testing.T) {
	var r shell.Route = week.NewRoute(fakeAPI{}, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	_, cmd := r.Update(shell.EventMsg{Ev: apiclient.ClientEvent{Type: string(domain.EventDocumentCreated)}})
	if cmd != nil {
		t.Fatal("non-session event should not trigger reload cmd")
	}
}

// TestWeekRoute_dayWithZeroTarget verifies that a day with TargetMin=0 doesn't panic.
func TestWeekRoute_dayWithZeroTarget(t *testing.T) {
	api := fakeAPI{days: []apiclient.WeekDay{
		{Date: "2026-06-14", LoggedMin: 0, TargetMin: 0, Workday: false},
	}}
	var r shell.Route = week.NewRoute(api, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "2026-06-14") {
		t.Fatalf("day with TargetMin=0 should still appear; got:\n%s", body)
	}
}

// TestWeekRoute_allNavKeys verifies all sibling nav keys emit SwitchRouteMsg.
func TestWeekRoute_allNavKeys(t *testing.T) {
	keys := []string{"w", "t", "d", "e"}
	for _, k := range keys {
		reg := wtnav.Registry{k: func() shell.Route { return stubTitle(k) }}
		r := week.NewRoute(fakeAPI{}, theme.Default, reg)
		_, cmd := r.Update(tea.KeyPressMsg{Text: k})
		if cmd == nil {
			t.Fatalf("key %q should emit a cmd", k)
		}
		msg := cmd()
		if _, ok := msg.(shell.SwitchRouteMsg); !ok {
			t.Fatalf("key %q should emit SwitchRouteMsg, got %T", k, msg)
		}
	}
}

func TestWeek_StripAndLeftPopsAndHideCrumb(t *testing.T) {
	reg := wtnav.Registry{
		"w": func() shell.Route { return week.NewRoute(nil, theme.Default, nil) },
		"t": func() shell.Route { return stubTitle("Stats") },
	}
	r := week.NewRoute(nil, theme.Default, reg)
	out := r.View(shell.Frame{Width: 200, Height: 24, Pal: theme.Default})
	for _, l := range []string{"Heute", "Woche", "Stats", "Frei"} {
		if !strings.Contains(out, l) {
			t.Fatalf("Woche View missing sub-tab %q", l)
		}
	}
	if strings.Contains(out, "Export") {
		t.Fatal("Woche strip must not contain Export (it is a drilled route)")
	}
	if !r.HideBreadcrumb() {
		t.Fatal("Woche must hide breadcrumb")
	}
	// ← from Woche pops back to Heute (deterministic, no registry needed).
	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if cmd == nil {
		t.Fatal("← on Woche must emit a command")
	}
	if _, ok := cmd().(shell.PopRouteMsg); !ok {
		t.Fatalf("← on Woche must pop to Heute, got %#v", cmd())
	}
}

// keyDown / keyUp build tea.KeyPressMsg for arrow key navigation.
func keyDown() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyDown} }
func keyUp() tea.KeyPressMsg   { return tea.KeyPressMsg{Code: tea.KeyUp} }

// newLoadedWeekRoute constructs a *week.Route and feeds it a loadedMsg with
// 5 day rows so cursor movement can be tested without a real server.
func newLoadedWeekRoute(t *testing.T) *week.Route {
	t.Helper()
	days := []apiclient.WeekDay{
		{Date: "2026-06-16", LoggedMin: 480, TargetMin: 480, Workday: true},
		{Date: "2026-06-17", LoggedMin: 240, TargetMin: 480, Workday: true},
		{Date: "2026-06-18", LoggedMin: 0, TargetMin: 480, Workday: true},
		{Date: "2026-06-19", LoggedMin: 60, TargetMin: 480, IsToday: true, Workday: true},
		{Date: "2026-06-20", LoggedMin: 0, TargetMin: 0, Workday: false},
	}
	api := fakeAPI{days: days}
	r := week.NewRoute(api, theme.Default, wtnav.Registry{})
	// Drain Init to populate days via the loadedMsg.
	r2 := drain(r, r.Init())
	wr, ok := r2.(*week.Route)
	if !ok {
		t.Fatalf("drain returned %T, want *week.Route", r2)
	}
	return wr
}

func TestWeekRoute_CursorMovesAndClamps(t *testing.T) {
	r := newLoadedWeekRoute(t)
	// down twice
	r2, _ := r.Update(keyDown())
	wr, ok := r2.(*week.Route)
	if !ok {
		t.Fatalf("Update returned %T, want *week.Route", r2)
	}
	wr2, _ := wr.Update(keyDown())
	wr, ok = wr2.(*week.Route)
	if !ok {
		t.Fatalf("Update returned %T, want *week.Route", wr2)
	}
	if got := wr.SelectedIndex(); got != 2 {
		t.Fatalf("cursor after 2×down = %d, want 2", got)
	}
	// up past top clamps at 0
	for i := 0; i < 5; i++ {
		x, _ := wr.Update(keyUp())
		wr, ok = x.(*week.Route)
		if !ok {
			t.Fatalf("Update returned %T, want *week.Route", x)
		}
	}
	if got := wr.SelectedIndex(); got != 0 {
		t.Fatalf("cursor clamped top = %d, want 0", got)
	}
}
