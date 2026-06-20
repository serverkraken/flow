// Package week pace-dot classification: one Mon–Fri dot per workday, ported
// from the old domain/pace_dot.go but adapted to the apiclient.WeekDay DTO
// (minute ints, no live Active — today-not-yet-hit renders as Running).
package week

import (
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
)

type paceDotKind int

const (
	paceDotMissed  paceDotKind = iota
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

// paceColor is consumed by the Woche route view (see route.go) to color each
// Mon–Fri dot. The blank assignment below keeps the linter happy while the
// caller is being wired up in the next task.
var _ = paceColor

// paceColor maps a pace-dot kind to a theme color. Day-off sub-kinds reuse
// the semantic slots that match their visual identity per design-system-audit:
// holiday=Schedule (blue, calendar event), vacation=Highlight (purple,
// Urlaub-identity per semantic.go comment), sick=Notice (orange, Krank-class).
// The brief suggested Accent for vacation and Info for holiday; the actual
// semantic.go slots are more specific — Schedule and Highlight are correct.
func paceColor(k paceDotKind, off *apiclient.DayOff, p theme.Palette) theme.Color {
	sem := p.Sem()
	switch k {
	case paceDotHit:
		return sem.Success
	case paceDotRunning:
		return sem.Active
	case paceDotDayOff:
		if off != nil {
			switch domain.Kind(off.Kind) {
			case domain.KindHoliday:
				return sem.Schedule
			case domain.KindVacation:
				return sem.Highlight
			case domain.KindSick:
				return sem.Notice
			}
		}
		return p.FgMuted
	}
	// paceDotMissed
	return sem.Border
}
