// Integration tests in package week_test that drive the Route as an external caller.
package week_test

import (
	"context"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/week"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// fakeWeekAPI provides day-off data in ListDayOffs for View integration tests.
// (distinct from route_test.go's fakeAPI which always returns nil offs)
type fakeWeekAPI struct {
	days []apiclient.WeekDay
	offs []apiclient.DayOff
}

func (f *fakeWeekAPI) GetWeek(_ context.Context, _ string) ([]apiclient.WeekDay, error) {
	return f.days, nil
}

func (f *fakeWeekAPI) ListDayOffs(_ context.Context, _, _ string) ([]apiclient.DayOff, error) {
	return f.offs, nil
}

// TestWeekView_HasSummarySectionsAndDayOffLabel checks that the rendered view
// contains WOCHE GESAMT, KENNZAHLEN, Schnitt, Ziele, Saldo, Urlaub, Wochenende.
func TestWeekView_HasSummarySectionsAndDayOffLabel(t *testing.T) {
	api := &fakeWeekAPI{
		days: []apiclient.WeekDay{
			{Date: "2026-06-15", TargetMin: 480, LoggedMin: 480},
			{Date: "2026-06-18", TargetMin: 0, LoggedMin: 0},
			{Date: "2026-06-20", TargetMin: 0, LoggedMin: 0},
		},
		offs: []apiclient.DayOff{{Day: "2026-06-18", Kind: "vacation", Label: "Urlaub"}},
	}
	r := week.NewRoute(api, theme.Default, nil)
	// Drain the init command to load data.
	cmd := r.Init()
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			break
		}
		var next shell.Route
		next, cmd = r.Update(msg)
		if wr, ok := next.(*week.Route); ok {
			r = wr
		}
	}
	out := r.View(shell.Frame{Width: 100, Height: 30, Pal: theme.Default})
	for _, want := range []string{"WOCHE GESAMT", "KENNZAHLEN", "Schnitt", "Ziele", "Saldo", "Urlaub", "Wochenende"} {
		if !strings.Contains(out, want) {
			t.Fatalf("week View missing %q:\n%s", want, out)
		}
	}
}
