package webui

import (
	"strconv"
	"time"
)

// WeekdayTargetVM is one Mon–Fri override input.
type WeekdayTargetVM struct {
	Label string // "Mo"
	Name  string // form field name: mon|tue|wed|thu|fri
	Value string // minutes; "" → inherits the default
}

// StatsWeekdayVMs builds the five Mon–Fri override inputs from the stored
// per-weekday target map. An absent weekday → empty value (inherits default).
func StatsWeekdayVMs(weekday map[time.Weekday]int) []WeekdayTargetVM {
	defs := []struct {
		label, name string
		wd          time.Weekday
	}{
		{"Mo", "mon", time.Monday},
		{"Di", "tue", time.Tuesday},
		{"Mi", "wed", time.Wednesday},
		{"Do", "thu", time.Thursday},
		{"Fr", "fri", time.Friday},
	}
	out := make([]WeekdayTargetVM, 0, len(defs))
	for _, d := range defs {
		v := ""
		if m, ok := weekday[d.wd]; ok {
			v = strconv.Itoa(m)
		}
		out = append(out, WeekdayTargetVM{Label: d.label, Name: d.name, Value: v})
	}
	return out
}

// EinstellungenVM is the view model for the Einstellungen page.
// It holds the Tagesziel editor state: the current default daily target
// (in minutes, as a form string) and Mon–Fri weekday overrides.
type EinstellungenVM struct {
	DefaultTarget string            // minutes as string for the form input
	Weekdays      []WeekdayTargetVM // Mon–Fri override inputs
	Err           string

	// Screen 30 — Konto, Sollzeiten, Sprache, Zugänge, Daten. Das Konto
	// kommt vom Anmeldedienst (OIDC) und ist hier nur lesbar.
	Username, DisplayName, Email string
	WeekSoll                     string // "40:00 h" — Summe Mo–Fr aus Standard und Abweichungen
	Engagements                  int
	FeedTokens                   int    // ICS-Abos; verwaltet unter /dayoffs
	Bundesland                   string // Feiertagsgrundlage; verwaltet unter /dayoffs
	Lang                         string // "de" | "en" | "" = Browservorgabe
}

// WeekSollMinutes summiert die Sollzeit Mo–Fr: die Abweichung, wo gesetzt,
// sonst der Standard.
func WeekSollMinutes(defaultMin int, weekday map[time.Weekday]int) int {
	total := 0
	for wd := time.Monday; wd <= time.Friday; wd++ {
		if m, ok := weekday[wd]; ok {
			total += m
			continue
		}
		total += defaultMin
	}
	return total
}

// FmtMinutesClock schreibt Minuten als „40:00 h".
func FmtMinutesClock(min int) string {
	if min < 0 {
		min = 0
	}
	return strconv.Itoa(min/60) + ":" + pad2(min%60) + " h"
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}
