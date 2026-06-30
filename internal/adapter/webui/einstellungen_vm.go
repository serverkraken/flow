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
}
