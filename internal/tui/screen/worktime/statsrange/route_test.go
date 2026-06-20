package statsrange_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/statsrange"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

type fakeAPI struct {
	lastRng string
	stats   apiclient.Stats
	bd      apiclient.Burndown
	statsErr error
}

func (f *fakeAPI) GetStats(_ context.Context, rng string) (apiclient.Stats, error) {
	f.lastRng = rng
	return f.stats, f.statsErr
}
func (f *fakeAPI) GetBurndown(context.Context) (apiclient.Burndown, error) { return f.bd, nil }

func drain(r shell.Route, cmd tea.Cmd) shell.Route {
	for cmd != nil {
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

func TestStatsRoute_rendersTotalsAndDefaultsToWeek(t *testing.T) {
	api := &fakeAPI{stats: apiclient.Stats{TotalMin: 600, AvgMin: 120, Workdays: 5, Streak: 3}}
	var r shell.Route = statsrange.NewRoute(api, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	if api.lastRng != "week" {
		t.Fatalf("default range = %q, want week", api.lastRng)
	}
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "10h 00m") {
		t.Fatalf("missing total:\n%s", body)
	}
}

func TestStatsRoute_mSwitchesToMonth(t *testing.T) {
	api := &fakeAPI{}
	var r shell.Route = statsrange.NewRoute(api, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	_, cmd := r.Update(tea.KeyPressMsg{Text: "m"})
	r = drain(r, cmd)
	if api.lastRng != "month" {
		t.Fatalf("after m, range = %q, want month", api.lastRng)
	}
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Monat") {
		t.Fatalf("after m, view should show Monat label:\n%s", body)
	}
}

func TestStatsRoute_WRevertsToWeek(t *testing.T) {
	api := &fakeAPI{}
	var r shell.Route = statsrange.NewRoute(api, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	_, cmd := r.Update(tea.KeyPressMsg{Text: "m"})
	r = drain(r, cmd)
	_, cmd = r.Update(tea.KeyPressMsg{Text: "W"})
	r = drain(r, cmd)
	if api.lastRng != "week" {
		t.Fatalf("after W, range = %q, want week", api.lastRng)
	}
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "KW") {
		t.Fatalf("after W, view should show KW label:\n%s", body)
	}
}

// TestStatsRoute_loadingState verifies the placeholder text before data arrives.
func TestStatsRoute_loadingState(t *testing.T) {
	r := statsrange.NewRoute(&fakeAPI{}, theme.Default, wtnav.Registry{})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "lädt") {
		t.Fatalf("loading state should show 'lädt'; got:\n%s", body)
	}
}

// TestStatsRoute_errorState verifies the error text when load fails.
func TestStatsRoute_errorState(t *testing.T) {
	api := &fakeAPI{statsErr: errors.New("db error")}
	var r shell.Route = statsrange.NewRoute(api, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Fehler") {
		t.Fatalf("error state should show 'Fehler'; got:\n%s", body)
	}
	if !strings.Contains(body, "db error") {
		t.Fatalf("error state should contain error text; got:\n%s", body)
	}
}

// TestStatsRoute_burndownLine verifies the burndown row appears when TargetMin>0.
func TestStatsRoute_burndownLine(t *testing.T) {
	api := &fakeAPI{
		bd: apiclient.Burndown{TargetMin: 9600, TotalMin: 4800, SaldoMin: -100},
	}
	var r shell.Route = statsrange.NewRoute(api, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Burndown") {
		t.Fatalf("burndown line missing when TargetMin>0; got:\n%s", body)
	}
}

// TestStatsRoute_burndownAbsentWhenNoTarget verifies burndown row is absent when TargetMin=0.
func TestStatsRoute_burndownAbsentWhenNoTarget(t *testing.T) {
	api := &fakeAPI{bd: apiclient.Burndown{TargetMin: 0}}
	var r shell.Route = statsrange.NewRoute(api, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if strings.Contains(body, "Burndown") {
		t.Fatalf("burndown line should be absent when TargetMin=0; got:\n%s", body)
	}
}

// TestStatsRoute_keyHints verifies KeyHints returns non-empty hints.
func TestStatsRoute_keyHints(t *testing.T) {
	r := statsrange.NewRoute(&fakeAPI{}, theme.Default, wtnav.Registry{})
	hints := r.KeyHints()
	if len(hints) == 0 {
		t.Fatal("KeyHints should return non-empty hints")
	}
}

// TestStatsRoute_title verifies the Title.
func TestStatsRoute_title(t *testing.T) {
	r := statsrange.NewRoute(&fakeAPI{}, theme.Default, wtnav.Registry{})
	if r.Title() != "Stats" {
		t.Fatalf("title = %q, want Stats", r.Title())
	}
}

// TestStatsRoute_navKeyEmitsSwitch verifies that nav keys emit SwitchRouteMsg.
func TestStatsRoute_navKeyEmitsSwitch(t *testing.T) {
	keys := []string{"w", "d", "e"}
	for _, k := range keys {
		reg := wtnav.Registry{k: func() shell.Route { return stubTitle(k) }}
		r := statsrange.NewRoute(&fakeAPI{}, theme.Default, reg)
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

// TestStatsRoute_mNoopWhenAlreadyMonth verifies pressing m when already on month returns nil cmd.
func TestStatsRoute_mNoopWhenAlreadyMonth(t *testing.T) {
	api := &fakeAPI{}
	var r shell.Route = statsrange.NewRoute(api, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	// Switch to month first
	_, cmd := r.Update(tea.KeyPressMsg{Text: "m"})
	r = drain(r, cmd)
	// Press m again - should be a noop (nil cmd)
	_, cmd = r.Update(tea.KeyPressMsg{Text: "m"})
	if cmd != nil {
		t.Fatal("pressing m when already on month should return nil cmd")
	}
}

// TestStatsRoute_wNoopWhenAlreadyWeek verifies pressing W when already on week returns nil cmd.
func TestStatsRoute_wNoopWhenAlreadyWeek(t *testing.T) {
	api := &fakeAPI{}
	var r shell.Route = statsrange.NewRoute(api, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	// Already on week - pressing W should be a noop
	_, cmd := r.Update(tea.KeyPressMsg{Text: "W"})
	if cmd != nil {
		t.Fatal("pressing W when already on week should return nil cmd")
	}
}

// TestStatsRoute_sseSessionEvent verifies that a session event triggers reload.
func TestStatsRoute_sseSessionEvent(t *testing.T) {
	var r shell.Route = statsrange.NewRoute(&fakeAPI{}, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	_, cmd := r.Update(shell.EventMsg{Ev: apiclient.ClientEvent{Type: string(domain.EventSessionStarted)}})
	if cmd == nil {
		t.Fatal("session.started event should trigger reload cmd")
	}
}

// TestStatsRoute_sseNonSessionEvent verifies that a non-session event returns nil cmd.
func TestStatsRoute_sseNonSessionEvent(t *testing.T) {
	var r shell.Route = statsrange.NewRoute(&fakeAPI{}, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	_, cmd := r.Update(shell.EventMsg{Ev: apiclient.ClientEvent{Type: string(domain.EventDocumentCreated)}})
	if cmd != nil {
		t.Fatal("non-session event should not trigger reload cmd")
	}
}

// wocheStub is a minimal Route stub for lateral nav tests in statsrange.
type wocheStub struct{}

func (wocheStub) Title() string                          { return "Woche" }
func (wocheStub) Init() tea.Cmd                          { return nil }
func (wocheStub) Update(tea.Msg) (shell.Route, tea.Cmd) { return wocheStub{}, nil }
func (wocheStub) View(shell.Frame) string                { return "Woche" }
func (wocheStub) KeyHints() []keyhint.Hint               { return nil }

func TestStats_StripAndLateralAndHideCrumb(t *testing.T) {
	reg := wtnav.Registry{"w": func() shell.Route { return wocheStub{} }}
	r := statsrange.NewRoute(nil, theme.Default, reg)
	out := r.View(shell.Frame{Width: 200, Height: 24, Pal: theme.Default})
	if !strings.Contains(out, "Stats") {
		t.Fatalf("Stats View missing strip labels:\n%s", out)
	}
	if strings.Contains(out, "Export") {
		t.Fatal("Stats strip must not contain Export (it is a drilled route)")
	}
	if !r.HideBreadcrumb() {
		t.Fatal("Stats must hide breadcrumb")
	}
	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyLeft}) // → Woche via reg
	if cmd == nil {
		t.Fatal("← on Stats must emit a command")
	}
}
