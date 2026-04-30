package worktime

import (
	"fmt"
	"strings"
	"time"
)

// GermanHolidays returns the gesetzliche Feiertage for a given Bundesland
// and year. Empty land defaults to "DE" (only the bundesweit set).
//
// Bundesland codes (case-insensitive, "NRW" alias for "NW"):
//
//	BW BY BE BB HB HH HE MV NI NW RP SL SN ST SH TH  ·  DE
//
// Movable feasts (Karfreitag, Ostermontag, Christi Himmelfahrt, Pfingstmontag,
// Fronleichnam) are computed from the Gauss-Knuth Easter algorithm.
//
// Used by the `flow worktime dayoff sync` command and the TUI bundle import
// to populate worktime-dayoffs.tsv without manual entry.
func GermanHolidays(year int, land string) []DayOff {
	land = normalizeLand(land)
	easter := easterSunday(year)
	loc := time.Local

	type entry struct {
		date  time.Time
		label string
		lands []string // empty = bundesweit
	}
	d := func(m, day int) time.Time {
		return time.Date(year, time.Month(m), day, 0, 0, 0, 0, loc)
	}

	all := []entry{
		{d(1, 1), "Neujahr", nil},
		{d(1, 6), "Heilige Drei Könige", []string{"BW", "BY", "ST"}},
		{d(3, 8), "Internationaler Frauentag", []string{"BE", "MV"}},
		{easter.AddDate(0, 0, -2), "Karfreitag", nil},
		{easter.AddDate(0, 0, 1), "Ostermontag", nil},
		{d(5, 1), "Tag der Arbeit", nil},
		{easter.AddDate(0, 0, 39), "Christi Himmelfahrt", nil},
		{easter.AddDate(0, 0, 50), "Pfingstmontag", nil},
		{easter.AddDate(0, 0, 60), "Fronleichnam", []string{"BW", "BY", "HE", "NW", "RP", "SL"}},
		{d(8, 15), "Mariä Himmelfahrt", []string{"SL"}},
		{d(9, 20), "Weltkindertag", []string{"TH"}},
		{d(10, 3), "Tag der Deutschen Einheit", nil},
		{d(10, 31), "Reformationstag", []string{"BB", "HB", "HH", "MV", "NI", "SN", "ST", "SH", "TH"}},
		{d(11, 1), "Allerheiligen", []string{"BW", "BY", "NW", "RP", "SL"}},
		{busBettag(year), "Buß- und Bettag", []string{"SN"}},
		{d(12, 25), "1. Weihnachtstag", nil},
		{d(12, 26), "2. Weihnachtstag", nil},
	}

	out := make([]DayOff, 0, len(all))
	for _, e := range all {
		if !appliesIn(e.lands, land) {
			continue
		}
		out = append(out, DayOff{
			Date:   e.date,
			Kind:   KindHoliday,
			Label:  e.label,
			Target: 0,
		})
	}
	return out
}

// normalizeLand uppercases and resolves common aliases.
func normalizeLand(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	switch s {
	case "":
		return "DE"
	case "NRW":
		return "NW"
	case "BAYERN":
		return "BY"
	case "BADEN-WÜRTTEMBERG", "BADEN-WUERTTEMBERG", "BAWÜ", "BAWUE":
		return "BW"
	}
	return s
}

// appliesIn reports whether a holiday with the given land-set applies in
// the requested land. Empty lands list means bundesweit (always applies).
// "DE" matches only bundesweit holidays.
func appliesIn(lands []string, land string) bool {
	if len(lands) == 0 {
		return true
	}
	if land == "DE" {
		return false
	}
	for _, l := range lands {
		if l == land {
			return true
		}
	}
	return false
}

// easterSunday returns Easter Sunday for the given Gregorian year using
// Anonymous Gregorian algorithm (a.k.a. Meeus/Jones/Butcher).
func easterSunday(year int) time.Time {
	a := year % 19
	b := year / 100
	c := year % 100
	d := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i := c / 4
	k := c % 4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := ((h + l - 7*m + 114) % 31) + 1
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.Local)
}

// busBettag is the Wednesday before November 23rd (always falls between Nov 16
// and Nov 22). Sachsen-only public holiday.
func busBettag(year int) time.Time {
	nov23 := time.Date(year, time.November, 23, 0, 0, 0, 0, time.Local)
	for d := nov23.AddDate(0, 0, -1); ; d = d.AddDate(0, 0, -1) {
		if d.Weekday() == time.Wednesday {
			return d
		}
	}
}

// SyncGermanHolidays adds (or replaces) entries for all holidays of the given
// year and land. Returns count added/skipped/replaced. Idempotent: re-running
// is a no-op when nothing changed.
func SyncGermanHolidays(year int, land string) (added, skipped int, err error) {
	hs := GermanHolidays(year, land)
	if len(hs) == 0 {
		return 0, 0, fmt.Errorf("keine Feiertage für %s/%d", land, year)
	}
	for _, h := range hs {
		existing, ok := LookupDayOff(h.Date)
		if ok && existing.Kind == KindHoliday && existing.Label == h.Label {
			skipped++
			continue
		}
		if ok && existing.Kind != KindHoliday {
			// Don't overwrite vacation/sick — user intent wins.
			skipped++
			continue
		}
		if err := AddDayOff(h.Date, h.Kind, h.Label); err != nil {
			return added, skipped, err
		}
		added++
	}
	return added, skipped, nil
}
