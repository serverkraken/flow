package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// CurrentStreak returns the owner's current workday-hit streak — walking back
// from the newest relevant workday to the first miss, weekends/day-offs skipped
// (not counted as misses). Windowless (Entscheidung Soenne): unlike
// RangeStats("month") it never cuts a streak at a month boundary. It loads the
// owner's full history and aggregates from the earliest session's day; a 3-year
// safety cap keeps the AggregateRange day-loop bounded against a stray ancient
// record (a >3-year perfect-attendance streak is not achievable). Semantics
// mirror the old repo's StatsComputer.CurrentStreak. Owner-scoped.
func (c StatsComputer) CurrentStreak(ctx context.Context, ownerID string) (int, error) {
	now := c.Clock.Now().In(c.loc())
	sessions, err := c.Sessions.List(ctx, ownerID, time.Time{}) // full history
	if err != nil {
		return 0, err
	}
	from := startOfDay(now)
	for _, s := range sessions {
		if d := startOfDay(s.Start.In(c.loc())); d.Before(from) {
			from = d
		}
	}
	if floor := startOfDay(now).AddDate(-3, 0, 0); from.Before(floor) {
		from = floor // 3-year safety floor (avoid shadowing builtin cap)
	}
	to := startOfDay(now).AddDate(0, 0, 1)
	res, offs, err := c.resolver(ctx, ownerID, from, to.AddDate(0, 0, -1))
	if err != nil {
		return 0, err
	}
	countsToward, err := c.countsTowardFn(ctx, ownerID)
	if err != nil {
		return 0, err
	}
	recs := domain.BuildDayRecords(sessions, now, res.For, countsToward)
	listOffs := func(f, t time.Time) []domain.DayOff {
		var in []domain.DayOff
		for _, o := range offs {
			if !o.Date.Before(f) && !o.Date.After(t) {
				in = append(in, o)
			}
		}
		return in
	}
	return domain.AggregateRange(recs, from, to, res.IsWorkday, res.For, listOffs).Streak, nil
}
