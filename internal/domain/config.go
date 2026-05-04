package domain

import (
	"strings"
	"time"
)

// Config is the resolved worktime configuration. Values come from any of:
// the user's ~/.tmux/worktime.conf file, the WORKTIME_* env vars, or the
// shipped defaults. ConfigReader implementations merge those sources
// before handing this struct off to use cases.
type Config struct {
	// DefaultTarget is the fallback daily work target when no per-weekday
	// override matches. Already includes the env-var fallback (WORKTIME_TARGET_HOURS).
	DefaultTarget time.Duration
	// PerWeekday maps weekdays to a duration override; missing keys mean
	// "use DefaultTarget".
	PerWeekday map[time.Weekday]time.Duration
	// TagTargets maps tag name (lowercased) to a per-tag daily target.
	TagTargets map[string]time.Duration
	// MaxStreakMin is the active-session warning threshold in minutes.
	// 0 (the default) disables the warning. The status segment turns
	// yellow at MaxStreakMin and red at 2× MaxStreakMin — see
	// status.go for the rendering logic.
	MaxStreakMin int
}

// TargetForWeekday returns the configured target for wd, or DefaultTarget
// when no per-weekday override exists.
func (c Config) TargetForWeekday(wd time.Weekday) time.Duration {
	if v, ok := c.LookupWeekday(wd); ok {
		return v
	}
	return c.DefaultTarget
}

// LookupWeekday is the presence-aware variant of TargetForWeekday.
// Returns (override, true) when the user explicitly configured the
// weekday — including when the override is zero, which means "no work
// today, do not fall through to DefaultTarget". Callers that need to
// honour an explicit zero (TargetResolver.For for the saldo math) MUST
// use this instead of TargetForWeekday.
func (c Config) LookupWeekday(wd time.Weekday) (time.Duration, bool) {
	if c.PerWeekday == nil {
		return 0, false
	}
	v, ok := c.PerWeekday[wd]
	return v, ok
}

// TagTarget returns the configured daily target for the named tag, or 0
// when none is set. Lookup is case-insensitive — "deep" and "Deep" hit the
// same key.
func (c Config) TagTarget(tag string) time.Duration {
	if c.TagTargets == nil {
		return 0
	}
	if v, ok := c.TagTargets[tag]; ok {
		return v
	}
	for k, v := range c.TagTargets {
		if strings.EqualFold(k, tag) {
			return v
		}
	}
	return 0
}
