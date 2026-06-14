package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ListDayOffs returns the merged set of manual day-offs (vacation/sick) and
// computed German holidays for the user's Bundesland over [from,to]. On a
// date collision the manual entry wins (holidays are dropped for that day).
// This is the single read source for TUI, WebUI and the ICS feed.
type ListDayOffs struct {
	Store    ports.DayOffStore
	Settings ports.UserSettingsStore
	Loc      *time.Location
}

func (uc ListDayOffs) Execute(ctx context.Context, ownerID string, from, to time.Time) ([]domain.DayOff, error) {
	manual, err := uc.Store.ListRange(ctx, ownerID, from, to)
	if err != nil {
		return nil, err
	}
	set, err := uc.Settings.Get(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	taken := make(map[string]bool, len(manual))
	out := make([]domain.DayOff, 0, len(manual))
	for _, d := range manual {
		taken[d.Date.Format("2006-01-02")] = true
		out = append(out, d)
	}
	for y := from.Year(); y <= to.Year(); y++ {
		for _, h := range domain.GermanHolidays(y, set.Bundesland, uc.Loc) {
			if h.Date.Before(from) || h.Date.After(to) {
				continue
			}
			if taken[h.Date.Format("2006-01-02")] {
				continue
			}
			out = append(out, h)
		}
	}
	return out, nil
}
