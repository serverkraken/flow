package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// TodaySummary is the worktime header: logged-so-far, the day's target and the
// resulting saldo, plus whether a timer is running.
type TodaySummary struct {
	Date    time.Time
	Logged  time.Duration
	Target  time.Duration
	Saldo   time.Duration // Logged - Target
	Running bool
}

// StatsComputer turns stored sessions + day-offs + target config into the
// derived stats/burndown/today/week shapes. All reads are owner-scoped.
type StatsComputer struct {
	Sessions ports.SessionStore
	Settings ports.UserSettingsStore
	DayOffs  ListDayOffs // merged manual + holidays
	Clock    ports.Clock
	Loc      *time.Location
}

// resolver builds the per-request TargetResolver over [from,to] (inclusive
// bounds passed to the merged day-off list).
func (c StatsComputer) resolver(ctx context.Context, ownerID string, from, to time.Time) (TargetResolver, []domain.DayOff, error) {
	set, err := c.Settings.Get(ctx, ownerID)
	if err != nil {
		return TargetResolver{}, nil, err
	}
	offs, err := c.DayOffs.Execute(ctx, ownerID, from, to)
	if err != nil {
		return TargetResolver{}, nil, err
	}
	r := TargetResolver{
		Default: time.Duration(set.DefaultTargetMin) * time.Minute,
		DayOffs: make(map[string]domain.DayOff, len(offs)),
	}
	for d, v := range set.WeekdayTargetMin {
		if d > time.Saturday {
			continue
		}
		dur := time.Duration(v) * time.Minute
		r.Weekday[int(d)] = &dur
	}
	for _, o := range offs {
		r.DayOffs[o.Date.Format("2006-01-02")] = o
	}
	return r, offs, nil
}

func (c StatsComputer) loc() *time.Location {
	if c.Loc != nil {
		return c.Loc
	}
	return time.Local
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// Today returns the summary for the calendar day containing now.
func (c StatsComputer) Today(ctx context.Context, ownerID string) (TodaySummary, error) {
	now := c.Clock.Now().In(c.loc())
	from := startOfDay(now)
	to := from.AddDate(0, 0, 1)
	res, _, err := c.resolver(ctx, ownerID, from, to.AddDate(0, 0, -1))
	if err != nil {
		return TodaySummary{}, err
	}
	sessions, err := c.Sessions.List(ctx, ownerID, from)
	if err != nil {
		return TodaySummary{}, err
	}
	recs := domain.BuildDayRecords(sessions, now, res.For)
	sum := TodaySummary{Date: from, Target: res.For(from)}
	for _, r := range recs {
		if r.Date.Equal(from) {
			sum.Logged = r.Total
		}
	}
	for _, s := range sessions {
		if s.Running() {
			sum.Running = true
		}
	}
	sum.Saldo = sum.Logged - sum.Target
	return sum, nil
}

// Week returns the 7 WeekDay rows of the ISO week containing ref (zero ref =
// today).
func (c StatsComputer) Week(ctx context.Context, ownerID string, ref time.Time) ([]domain.WeekDay, error) {
	now := c.Clock.Now().In(c.loc())
	if ref.IsZero() {
		ref = now
	}
	mon := isoMondayLocal(ref)
	to := mon.AddDate(0, 0, 7)
	res, _, err := c.resolver(ctx, ownerID, mon, to.AddDate(0, 0, -1))
	if err != nil {
		return nil, err
	}
	sessions, err := c.Sessions.List(ctx, ownerID, mon)
	if err != nil {
		return nil, err
	}
	recs := domain.BuildDayRecords(sessions, now, res.For)
	byDay := map[string]domain.DayRecord{}
	for _, r := range recs {
		byDay[r.Date.Format("2006-01-02")] = r
	}
	today := startOfDay(now)
	out := make([]domain.WeekDay, 0, 7)
	for d := mon; d.Before(to); d = d.AddDate(0, 0, 1) {
		wd := domain.WeekDay{Date: d, Target: res.For(d), IsToday: d.Equal(today)}
		if r, ok := byDay[d.Format("2006-01-02")]; ok {
			wd.Logged = r.Total // already includes the live tail from BuildDayRecords
		}
		// Active stays nil: the live tail is already folded into Logged, so we
		// must not let WeekDay.Total add it a second time.
		out = append(out, wd)
	}
	return out, nil
}

// RangeStats aggregates the ISO week ("week") or calendar month ("month")
// containing now.
func (c StatsComputer) RangeStats(ctx context.Context, ownerID, rng string) (domain.Stats, error) {
	now := c.Clock.Now().In(c.loc())
	var from, to time.Time
	switch rng {
	case "month":
		from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, c.loc())
		to = from.AddDate(0, 1, 0)
	case "week", "":
		from = isoMondayLocal(now)
		to = from.AddDate(0, 0, 7)
	default:
		return domain.Stats{}, domain.ErrInvalidRange
	}
	res, offs, err := c.resolver(ctx, ownerID, from, to.AddDate(0, 0, -1))
	if err != nil {
		return domain.Stats{}, err
	}
	sessions, err := c.Sessions.List(ctx, ownerID, from)
	if err != nil {
		return domain.Stats{}, err
	}
	recs := domain.BuildDayRecords(sessions, now, res.For)
	listOffs := func(f, t time.Time) []domain.DayOff {
		var in []domain.DayOff
		for _, o := range offs {
			if !o.Date.Before(f) && !o.Date.After(t) {
				in = append(in, o)
			}
		}
		return in
	}
	return domain.AggregateRange(recs, from, to, res.IsWorkday, res.For, listOffs), nil
}

// Burndown reports monthly progress for the month containing now. The live
// tail is already folded into recs, so MonthBurndownCompute is called with a
// nil active marker to avoid double-counting.
func (c StatsComputer) Burndown(ctx context.Context, ownerID string) (domain.MonthBurndownReport, error) {
	now := c.Clock.Now().In(c.loc())
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, c.loc())
	to := from.AddDate(0, 1, 0)
	res, _, err := c.resolver(ctx, ownerID, from, to.AddDate(0, 0, -1))
	if err != nil {
		return domain.MonthBurndownReport{}, err
	}
	sessions, err := c.Sessions.List(ctx, ownerID, from)
	if err != nil {
		return domain.MonthBurndownReport{}, err
	}
	recs := domain.BuildDayRecords(sessions, now, res.For)
	return domain.MonthBurndownCompute(now, recs, nil, res.IsWorkday, res.For), nil
}

// isoMondayLocal returns Monday 00:00 of t's ISO week, in t's location.
func isoMondayLocal(t time.Time) time.Time {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	d := t.AddDate(0, 0, -(wd - 1))
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, t.Location())
}
