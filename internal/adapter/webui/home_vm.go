package webui

import (
	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

// HomeVM is the view model for the Home landing page.
// Slice 4 adds the timer-hero fields (subset of HeuteVM used by homeHero /
// homeStartCard), keeping the section-link cards below for Tasks 2/3.
type HomeVM struct {
	// Timer-hero fields (mirror of HeuteVM subset used by homeHero/homeStartCard).
	Running     *domain.WorkSession
	RunningBase int    // running session's elapsed seconds at render (data-base seed)
	RunningName string // running session's project name
	RunningHue  string // running session's project hue ("" → blue default)
	RunningTag  string // running session's tag without '#'
	StartedAt   string // running session start time "11:58"

	LoggedDur  string // "5h 12m"
	TargetDur  string // "8h 00m"
	TargetPct  int
	TargetVar  string // hit|over|under|running
	Balance    string // "+2h 18m" / "−1h 05m"
	BalancePos bool

	Nodes   []components.NodePickerItem // bookable engagement nodes for the stop picker
	HasProj bool                        // true when at least one engagement node exists

	// Saldo tiles (Heute / Woche / Monat) — fed by homeDataFor when Stats is wired.
	TodaySaldo string
	TodayPos   bool
	TodaySub   string

	WeekSaldo string
	WeekPos   bool
	WeekSub   string

	MonthSaldo string
	MonthPos   bool
	MonthSub   string

	Burndown components.BurndownVM

	Err string // inline error message (surfaced when stop fails validation)
}
