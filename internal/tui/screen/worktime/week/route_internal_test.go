package week

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/tui/theme"
)

type fakeWeekAPI struct {
	days    []apiclient.WeekDay
	offs    []apiclient.DayOff
	gotFrom string
	gotTo   string
}

func (f *fakeWeekAPI) GetWeek(_ context.Context, _ string) ([]apiclient.WeekDay, error) {
	return f.days, nil
}

func (f *fakeWeekAPI) ListDayOffs(_ context.Context, from, to string) ([]apiclient.DayOff, error) {
	f.gotFrom, f.gotTo = from, to
	return f.offs, nil
}

func TestWeek_LoadsDayOffsForRange(t *testing.T) {
	api := &fakeWeekAPI{
		days: []apiclient.WeekDay{
			{Date: "2026-06-15"}, {Date: "2026-06-16"}, {Date: "2026-06-21"},
		},
		offs: []apiclient.DayOff{{Day: "2026-06-16", Kind: "vacation", Label: "Urlaub"}},
	}
	r := NewRoute(api, theme.Default, nil)
	msg := r.Init()() // run the load cmd
	r2, _ := r.Update(msg)
	rr := r2.(*Route)
	if api.gotFrom != "2026-06-15" || api.gotTo != "2026-06-21" {
		t.Fatalf("ListDayOffs range = %s..%s, want 2026-06-15..2026-06-21", api.gotFrom, api.gotTo)
	}
	if _, ok := rr.offs["2026-06-16"]; !ok {
		t.Fatal("day-off map must contain 2026-06-16 after load")
	}
}
