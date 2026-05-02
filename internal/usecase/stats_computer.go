package usecase

import (
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// StatsComputer aggregates worktime data into Stats / streak / burndown
// shapes. Reader supplies the underlying history; Targets supplies the
// is-workday + target-for predicates that the pure domain functions need.
type StatsComputer struct {
	Reader  *WorktimeReader
	Targets *TargetResolver
	DayOffs ports.DayOffStore
	State   ports.ActiveSessionStore
}

// Aggregate computes Stats over the given DayRecords.
func (c *StatsComputer) Aggregate(records []domain.DayRecord) domain.Stats {
	return domain.Aggregate(records, c.Targets.IsWorkday, c.DayOffs.List)
}

// CurrentStreak walks history and returns the current workday-hit streak
// ending today. Returns 0 on read failure rather than an error — callers
// (status segment, header) treat this as best-effort eye candy.
func (c *StatsComputer) CurrentStreak() int {
	hist, err := c.Reader.History()
	if err != nil || len(hist) == 0 {
		return 0
	}
	return c.Aggregate(hist).Streak
}

// WeekStats returns aggregated stats for the ISO week containing ref.
func (c *StatsComputer) WeekStats(ref time.Time) (domain.Stats, error) {
	hist, err := c.Reader.History()
	if err != nil {
		return domain.Stats{}, err
	}
	wd := int(ref.Weekday())
	if wd == 0 {
		wd = 7
	}
	mon := time.Date(ref.Year(), ref.Month(), ref.Day(), 0, 0, 0, 0, ref.Location()).
		AddDate(0, 0, -(wd - 1))
	return c.Aggregate(domain.FilterRecords(hist, mon, mon.AddDate(0, 0, 7))), nil
}

// MonthStats returns aggregated stats for the calendar month containing ref.
func (c *StatsComputer) MonthStats(ref time.Time) (domain.Stats, error) {
	hist, err := c.Reader.History()
	if err != nil {
		return domain.Stats{}, err
	}
	from := time.Date(ref.Year(), ref.Month(), 1, 0, 0, 0, 0, ref.Location())
	return c.Aggregate(domain.FilterRecords(hist, from, from.AddDate(0, 1, 0))), nil
}

// Burndown computes the monthly burndown report for the month containing now.
// The active session, if any, contributes its live tail to Total.
func (c *StatsComputer) Burndown(now time.Time) (domain.MonthBurndownReport, error) {
	hist, err := c.Reader.History()
	if err != nil {
		return domain.MonthBurndownReport{}, err
	}
	active, _ := c.State.GetActive()
	return domain.MonthBurndownCompute(now, hist, active, c.Targets.IsWorkday, c.Targets.For), nil
}
