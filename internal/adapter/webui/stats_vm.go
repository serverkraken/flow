package webui

import (
	"strconv"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

// StatsVM is the view model for the Stats page on the Slice-0 AppShell: three
// saldo tiles (Heute/Woche/Monat), the month burndown banner, and the daily
// target config (default + Mon–Fri overrides).
type StatsVM struct {
	TodaySaldo string // "+2h 18m" / "−1h 05m"
	TodayPos   bool
	TodaySub   string // "5h 12m / 8h 00m"
	WeekSaldo  string
	WeekPos    bool
	WeekSub    string
	MonthSaldo string
	MonthPos   bool
	MonthSub   string

	Burndown components.BurndownVM

	DefaultTarget string // minutes as string for the form input
	Weekdays      []WeekdayTargetVM
	Err           string
}

// WeekdayTargetVM is one Mon–Fri override input.
type WeekdayTargetVM struct {
	Label string // "Mo"
	Name  string // form field name: mon|tue|wed|thu|fri
	Value string // minutes; "" → inherits the default
}

// statsSaldoHue colors a saldo value green when ahead, red when behind.
func statsSaldoHue(pos bool) string {
	if pos {
		return "text-green"
	}
	return "text-red"
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
