// Package week pace-dot classification: one Mon–Fri dot per workday, ported
// from the old domain/pace_dot.go but adapted to the apiclient.WeekDay DTO
// (minute ints, no live Active — today-not-yet-hit renders as Running).
package week

import (
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/kindcolor"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
)

type paceDotKind int

const (
	paceDotMissed paceDotKind = iota
	paceDotHit
	paceDotRunning
	paceDotDayOff
)

// classifyPaceDot maps one weekday to its pace-dot slot. off is non-nil when a
// day-off (holiday/vacation/sick) covers the date.
func classifyPaceDot(d apiclient.WeekDay, off *apiclient.DayOff) paceDotKind {
	if off != nil {
		return paceDotDayOff
	}
	if d.TargetMin > 0 && d.LoggedMin >= d.TargetMin {
		return paceDotHit
	}
	if d.IsToday {
		return paceDotRunning
	}
	return paceDotMissed
}

// paceGlyph returns the display glyph for a pace-dot kind. Missed (open day)
// uses ○ (glyphs.Empty); every accounted slot uses ● (glyphs.Filled).
func paceGlyph(k paceDotKind) string {
	if k == paceDotMissed {
		return glyphs.Empty
	}
	return glyphs.Filled
}

// paceColor maps a pace-dot kind to a theme color. Day-off hues are delegated
// to kindcolor.DayOffColor so the Frei list and Woche dots share a single
// source of truth and all 7 kinds (holiday/vacation/sick/flex/special/
// childsick/training) are covered automatically.
func paceColor(k paceDotKind, off *apiclient.DayOff, p theme.Palette) theme.Color {
	sem := p.Sem()
	switch k {
	case paceDotHit:
		return sem.Success
	case paceDotRunning:
		return sem.Active
	case paceDotDayOff:
		if off != nil {
			return kindcolor.DayOffColor(domain.Kind(off.Kind), p)
		}
		return p.FgMuted
	}
	// paceDotMissed
	return sem.Border
}
