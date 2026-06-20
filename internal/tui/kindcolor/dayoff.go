package kindcolor

import (
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// DayOffColor maps a day-off Kind to its semantic color. Single source of truth
// so the Frei list glyph and the Woche pace dots can never drift. Unknown kinds
// fall back to the muted foreground.
//
// Holiday=Schedule (blue, calendar event), Vacation=Highlight (purple,
// Urlaub-identity), Sick=Notice (orange, Krank-class) match the semantic.go
// role tokens; Flex/Special/ChildSick/Training take the remaining distinct hues.
func DayOffColor(k domain.Kind, p theme.Palette) theme.Color {
	sem := p.Sem()
	switch k {
	case domain.KindHoliday:
		return sem.Schedule
	case domain.KindVacation:
		return sem.Highlight
	case domain.KindSick:
		return sem.Notice
	case domain.KindFlex:
		return sem.Success
	case domain.KindSpecial:
		return sem.Warning
	case domain.KindChildSick:
		return sem.Danger
	case domain.KindTraining:
		return sem.Info
	}
	return p.FgMuted
}
