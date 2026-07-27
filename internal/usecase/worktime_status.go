package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// WorktimeStatus composes the read models the tmux status segment needs into
// ONE owner-scoped snapshot: today (logged/target/running), the running session
// (id/start/node), the ISO week with per-day day-off kinds, the current streak
// and the monthly burndown. Pure composition of existing readers — NO new store
// methods (Spec §1). Mirrors the old usecase.StatusComposer.
type WorktimeStatus struct {
	Stats   StatsComputer
	Running GetRunningSession
	DayOffs ListDayOffs
	Clock   ports.Clock
	Loc     *time.Location
}

// WorktimeStatusResult is the domain-typed snapshot; the handler maps it to the
// wire DTO.
type WorktimeStatusResult struct {
	Date         time.Time
	Logged       time.Duration // COMPLETED today, running tail already subtracted
	Target       time.Duration
	Running      bool
	ActiveID     string
	ActiveStart  time.Time
	ActiveNodeID *string
	DayOff       *domain.DayOff // today's, or nil
	Week         []WorktimeStatusWeekDay
	Streak       int
	Burndown     domain.MonthBurndownReport
}

// WorktimeStatusWeekDay is one Mon..Sun row; Logged INCLUDES today's live tail
// (server snapshot, matches handleWeek), only for pace-dot classification.
type WorktimeStatusWeekDay struct {
	Date       time.Time
	Logged     time.Duration
	Target     time.Duration
	IsToday    bool
	DayOffKind domain.Kind // "" = none
}

func (uc WorktimeStatus) loc() *time.Location {
	if uc.Loc != nil {
		return uc.Loc
	}
	return time.Local
}

func (uc WorktimeStatus) Execute(ctx context.Context, ownerID string) (WorktimeStatusResult, error) {
	now := uc.Clock.Now().In(uc.loc())

	today, err := uc.Stats.Today(ctx, ownerID)
	if err != nil {
		return WorktimeStatusResult{}, err
	}
	week, err := uc.Stats.Week(ctx, ownerID, time.Time{})
	if err != nil {
		return WorktimeStatusResult{}, err
	}
	streak, err := uc.Stats.CurrentStreak(ctx, ownerID) // windowless (Task 1c), NOT RangeStats("month")
	if err != nil {
		return WorktimeStatusResult{}, err
	}
	burndown, err := uc.Stats.Burndown(ctx, ownerID)
	if err != nil {
		return WorktimeStatusResult{}, err
	}
	sess, running, err := uc.Running.Execute(ctx, ownerID)
	if err != nil {
		return WorktimeStatusResult{}, err
	}

	res := WorktimeStatusResult{
		Date: today.Date, Logged: today.Logged, Target: today.Target,
		Running: running, Streak: streak, Burndown: burndown,
	}

	// Subtract the running session's tail from Logged so it is COMPLETED-only —
	// but ONLY when Today() actually counted that tail. StatsComputer.Today()
	// loads sessions via SessionStore.List(from = today-midnight) (verified
	// `WHERE start_at >= $2`, pgstore/sessions.go), so a session that STARTED
	// BEFORE today (running across midnight) was never loaded and its tail is
	// NOT in Today().Logged — subtracting it would eat OTHER completed same-day
	// sessions and clamp real time to 0. The client re-adds the midnight-clamped
	// tail from ActiveStart in BOTH cases (Finding #1).
	if running {
		res.ActiveID = sess.ID
		res.ActiveStart = sess.Start
		res.ActiveNodeID = sess.NodeID
		if midnight := startOfDay(now); !sess.Start.Before(midnight) {
			res.Logged -= now.Sub(sess.Start) // start is today → Today() folded this tail in
			if res.Logged < 0 {
				res.Logged = 0
			}
		}
	}

	// One merged day-off read over the week span → today's banner + per-day kinds.
	var offs []domain.DayOff
	if len(week) > 0 {
		offs, err = uc.DayOffs.Execute(ctx, ownerID, week[0].Date, week[len(week)-1].Date)
		if err != nil {
			return WorktimeStatusResult{}, err
		}
	}
	byDay := make(map[string]domain.DayOff, len(offs))
	for _, o := range offs {
		byDay[o.Date.Format("2006-01-02")] = o
	}
	if o, ok := byDay[startOfDay(now).Format("2006-01-02")]; ok {
		res.DayOff = &o
	}
	res.Week = make([]WorktimeStatusWeekDay, 0, len(week))
	for _, d := range week {
		wd := WorktimeStatusWeekDay{Date: d.Date, Logged: d.Total(now), Target: d.Target, IsToday: d.IsToday}
		if o, ok := byDay[d.Date.Format("2006-01-02")]; ok {
			wd.DayOffKind = o.Kind
		}
		res.Week = append(res.Week, wd)
	}
	return res, nil
}
