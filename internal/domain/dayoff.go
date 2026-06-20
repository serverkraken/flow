package domain

import (
	"strings"
	"time"
)

// Kind classifies a day off.
type Kind string

// Day-off categories. Persisted as the literal string in the day_offs.kind
// column, so renaming a value requires a migration. Adding a new value does
// not (the column is free-text, validated via ParseKind).
const (
	KindHoliday   Kind = "holiday"   // gesetzlicher Feiertag
	KindVacation  Kind = "vacation"  // Urlaub
	KindSick      Kind = "sick"      // Krankheit
	KindFlex      Kind = "flex"      // Gleittag / Überstundenabbau
	KindSpecial   Kind = "special"   // Sonderurlaub
	KindChildSick Kind = "childsick" // Kind krank
	KindTraining  Kind = "training"  // Fortbildung / Schulung
)

// AllKinds enumerates valid kinds in display order. Used by UI cycling and
// CLI validation so callers don't have to repeat the list.
var AllKinds = []Kind{
	KindHoliday, KindVacation, KindSick,
	KindFlex, KindSpecial, KindChildSick, KindTraining,
}

// SelectableKinds enumerates the kinds a user may pick when adding a day-off,
// in picker display order. Excludes KindHoliday (holidays are computed from the
// Bundesland, never stored manually — see AddDayOffs.ErrHolidayNotManual).
var SelectableKinds = []Kind{
	KindVacation, KindSick, KindFlex,
	KindSpecial, KindChildSick, KindTraining,
}

// LabelDe renders the German label for a kind ("Feiertag", "Urlaub", "Krank").
func (k Kind) LabelDe() string {
	switch k {
	case KindHoliday:
		return "Feiertag"
	case KindVacation:
		return "Urlaub"
	case KindSick:
		return "Krank"
	case KindFlex:
		return "Gleittag"
	case KindSpecial:
		return "Sonderurlaub"
	case KindChildSick:
		return "Kind krank"
	case KindTraining:
		return "Fortbildung"
	}
	return string(k)
}

// ParseKind tolerates German UI strings ("Urlaub", "Krank", "Feiertag") and
// short forms ("v", "s", "h"). Returns ("", false) on unknown input.
func ParseKind(s string) (Kind, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "h", "holiday", "feiertag":
		return KindHoliday, true
	case "v", "vacation", "urlaub":
		return KindVacation, true
	case "s", "sick", "krank", "krankheit":
		return KindSick, true
	case "flex", "gleittag", "gleit":
		return KindFlex, true
	case "special", "sonderurlaub", "sonder":
		return KindSpecial, true
	case "childsick", "kindkrank", "kind krank":
		return KindChildSick, true
	case "training", "fortbildung", "schulung":
		return KindTraining, true
	}
	return "", false
}

// DayOff is one named day-off entry with an optional target override.
// Target == 0 means "full day off"; > 0 reduces the day's target (half-day);
// -1 means "no override" (rare, kept for forward compat).
type DayOff struct {
	Date   time.Time
	Kind   Kind
	Label  string
	Target time.Duration
}
