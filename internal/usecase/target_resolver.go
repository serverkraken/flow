package usecase

import (
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// TargetResolver computes the daily work target for a given date, applying
// the priority order day-off override > per-weekday config > default.
type TargetResolver struct {
	Config  ports.ConfigReader
	DayOffs ports.DayOffStore
	// DefaultTarget is used when ConfigReader.Load fails. The adapter is
	// expected to merge env+file+8h-fallback into Config.DefaultTarget,
	// so this field is just the lifeline for unhealthy configs.
	DefaultTarget time.Duration
}

// For returns the target work duration for date.
//
// Priority:
//
//  1. day-off entry with Target >= 0 wins (0 = full day off, >0 = override).
//  2. per-weekday config override.
//  3. config default target.
//  4. r.DefaultTarget — only when the config load failed.
func (r *TargetResolver) For(date time.Time) time.Duration {
	if d, ok := r.DayOffs.Lookup(date); ok && d.Target >= 0 {
		return d.Target
	}
	cfg, err := r.Config.Load()
	if err != nil {
		return r.DefaultTarget
	}
	// LookupWeekday distinguishes "explicitly set to N" (including 0,
	// which a user configures to mean 'no work this weekday') from
	// "no override, use the default". The previous `t > 0` check
	// silently overwrote an explicit 0 with the default — so a user
	// who set target_sun=0 still saw an 8h Sunday saldo gap.
	if t, ok := cfg.LookupWeekday(date.Weekday()); ok {
		return t
	}
	// DefaultTarget > 0 distinguishes "user explicitly set a positive
	// default" from "zero-value Config struct" (e.g. config load
	// returned the zero default for unsupported keys). The asymmetry
	// with LookupWeekday's ok-semantic is intentional: an unset
	// DefaultTarget should NOT silently override the resolver's 8h
	// safety net. A user who genuinely wants "no work by default" is
	// expected to configure per-weekday entries (target_mon=0 etc.) —
	// those go through LookupWeekday which respects explicit zero.
	if cfg.DefaultTarget > 0 {
		return cfg.DefaultTarget
	}
	return r.DefaultTarget
}

// IsWorkday wraps domain.IsWorkday with the resolver's day-off predicate.
// Centralised so use cases that need a workday classification (Aggregate,
// PlannedTarget, MonthBurndownCompute) all share the same definition.
func (r *TargetResolver) IsWorkday(date time.Time) bool {
	return domain.IsWorkday(date, r.isDayOff)
}

// IsDayOff returns the bare predicate for callers that need the closure
// without the workday combination.
func (r *TargetResolver) IsDayOff(date time.Time) bool { return r.isDayOff(date) }

func (r *TargetResolver) isDayOff(date time.Time) bool {
	_, ok := r.DayOffs.Lookup(date)
	return ok
}
