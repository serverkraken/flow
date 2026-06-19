package week_test

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/week"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

type fakeAPI struct{ days []apiclient.WeekDay }

func (f fakeAPI) GetWeek(_ context.Context, _ string) ([]apiclient.WeekDay, error) {
	return f.days, nil
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
