package domain

import (
	"strings"
	"time"
)

// Kind classifies a day off.
type Kind string

// Day-off categories. Persisted as the literal string in the second column
// of worktime-dayoffs.tsv, so renaming requires a migration.
const (
	KindHoliday  Kind = "holiday"  // gesetzlicher Feiertag
	KindVacation Kind = "vacation" // Urlaub
	KindSick     Kind = "sick"     // Krankheit
)

// AllKinds enumerates valid kinds in display order. Used by UI cycling and
// CLI validation so callers don't have to repeat the list.
var AllKinds = []Kind{KindHoliday, KindVacation, KindSick}

// LabelDe renders the German label for a kind ("Feiertag", "Urlaub", "Krank").
func (k Kind) LabelDe() string {
	switch k {
	case KindHoliday:
		return "Feiertag"
	case KindVacation:
		return "Urlaub"
	case KindSick:
		return "Krank"
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
