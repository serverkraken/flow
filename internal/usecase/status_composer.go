package usecase

import (
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// StatusComposer assembles the tmux status-right segment string. It
// orchestrates all the read services (WorktimeReader / TargetResolver /
// StatsComputer / DayOffStore / Tmux options) and hands their output
// to domain.BuildStatusSegment.
type StatusComposer struct {
	Reader  *WorktimeReader
	DayOffs ports.DayOffStore
	Targets *TargetResolver
	Stats   *StatsComputer
	Tmux    ports.Tmux
	Clock   ports.Clock
	// MaxStreakMin is the active-session warning threshold. 0 disables.
	MaxStreakMin int
}

// Compose returns the status-right segment string. Empty string when
// nothing happened today and there's no week activity (the original
// "render nothing" branch in BuildStatusSegment).
//
// History/state read failures degrade gracefully — a partial segment is
// preferable to no segment at all, since the user often reads the bar
// just to confirm "yes, the binary still works".
func (c *StatusComposer) Compose() string {
	now := c.Clock.Now()
	day, err := c.Reader.Today()
	if err != nil {
		return ""
	}
	week, _ := c.Reader.Week()
	burndown, _ := c.Stats.Burndown(now)

	var dayOff *domain.DayOff
	if d, ok := c.DayOffs.Lookup(now); ok {
		dayOff = &d
	}

	return domain.BuildStatusSegment(domain.StatusInputs{
		Now:          now,
		Day:          day,
		Week:         week,
		DayOff:       dayOff,
		Target:       c.Targets.For(now),
		Streak:       c.Stats.CurrentStreak(),
		Burndown:     burndown,
		LookupDayOff: c.DayOffs.Lookup,
		Palette:      c.palette(),
		MaxStreakMin: c.MaxStreakMin,
	})
}

// palette resolves the StatusPalette by layering tmux @tn_* option
// overrides on top of the tokyonight defaults.
func (c *StatusComposer) palette() domain.StatusPalette {
	pick := func(opt, fallback string) string {
		if v := c.Tmux.ShowOption(opt); v != "" {
			return v
		}
		return fallback
	}
	def := domain.DefaultStatusPalette()
	return domain.StatusPalette{
		Green:  pick("tn_green", def.Green),
		Yellow: pick("tn_yellow", def.Yellow),
		Red:    pick("tn_red", def.Red),
		Cyan:   pick("tn_cyan", def.Cyan),
		Dim:    pick("tn_dim", def.Dim),
	}
}
