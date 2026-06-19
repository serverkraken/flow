package statsrange_test

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/statsrange"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

type fakeAPI struct {
	lastRng string
	stats   apiclient.Stats
	bd      apiclient.Burndown
}

func (f *fakeAPI) GetStats(_ context.Context, rng string) (apiclient.Stats, error) {
	f.lastRng = rng
	return f.stats, nil
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
