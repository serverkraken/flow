package webui

import (
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

// WocheVM is the view model for the Woche (week) worktime page — L4 Task 4's
// full Lesesaal rebuild (pagehead + "‹ Zeit" spine, weekbar skyline, Mo–So
// row-list, two Kennzahlen panels, the Statistik/Burndown panel). The
// underlying per-day/week computation (wocheDataFor, httpserver package) and
// the KW-nav fields stay the unchanged data source (Interfaces: "unangetastet
// als Datenquelle") — Total/Kennzahlen/Days keep their pre-L4 field shapes.
// Stats is additive: the monthly Burndown/Saldo metric that used to live on
// Home now lives here (Offene Entsch. #4, "Woche ist die Statistik-
// Destination").
type WocheVM struct {
	User        string
	WeekStart   time.Time // Monday of the displayed week
	KWLabel     string    // "KW 25"
	WeekLabel   string    // "15.–21.06.2026"
	PrevWeek    string    // yyyy-mm-dd of the previous Monday (KW nav back)
	NextWeek    string    // yyyy-mm-dd of the next Monday ("" → clamped, current week)
	CanForward  bool      // false on (or past) the current week → hide next/„Diese Woche"
	IsCurrent   bool      // true when the displayed week is the current week
	Days        []WocheDayVM
	WeekDays    []ZeitWeekDay // .weekbar skyline bars, Mo..So (BuildWeekBars — same builder/format as Zeit-Hub, Finding 1)
	Total       components.WeekTotalVM
	Kennzahlen  components.KennzahlenVM
	WorkdayGoal string // "8h 00m" per-weekday target (header hint)
	Empty       bool   // no logged time anywhere in the week
	Stats       WocheStatsVM
}

// WocheDayVM is one day row in the week list (Mo..So).
type WocheDayVM struct {
	Label       string // "Mo"
	DateLabel   string // "15.06."
	Dur         string // "7h 36m"
	TargetDur   string // "8h 00m"
	Saldo       string // "+1h 24m" / "−0h 20m"; "" hides the delta line
	SaldoPos    bool
	Pct         int
	Variant     string // hit|over|under|running|weekend (drives the .v hue + weekbar bar height)
	Weekend     bool
	DayOff      bool
	DayOffLabel string // kind label, e.g. "Urlaub"
	DayOffHue   string // web hue token for the day-off chip, e.g. "purple"
	IsToday     bool
}

// WocheStatsVM drives the Woche page's "Statistik" panel: the monthly
// Burndown/Saldo glance metric that moved here from Home (Offene Entsch. #4).
// Built by httpserver's wocheDataFor from domain.MonthBurndownReport
// (s.Stats.Burndown) — deliberately NOT components.BurndownBanner (that
// Kristall glass card stays retired); this renders as a plain Lesesaal
// .panel/.krow block (wocheStatsPanel, woche.templ).
type WocheStatsVM struct {
	Total   string // "78h 00m" — month logged so far
	Target  string // "160h 00m" — month target
	Saldo   string // "+5h 12m" / "−2h 30m" (FmtSaldoVerbose)
	OnTrack bool
}

// wocheDayOffTypeChip maps a WocheDayVM.DayOffHue (the same semantic hue
// token dayOffHue/TUI kindcolor.DayOffColor produce) onto its .typechip tc-*
// tone (Farb-Gesetz §7: fixed & semantic, never per-project). Only 6 tc-
// tokens exist (b/v/t/o/g/r) for 7 day-off hues — yellow (Special) shares
// tc-o with orange (Sick); the kind LABEL text (not the chip color) carries
// that distinction. Replaces the retired wocheDayOffChip (Tailwind
// bg-hue/10 text-hue).
func wocheDayOffTypeChip(hue string) string {
	switch hue {
	case "purple":
		return "tc-v"
	case "orange", "yellow":
		return "tc-o"
	case "green":
		return "tc-g"
	case "red":
		return "tc-r"
	case "cyan":
		return "tc-t"
	default: // "blue" (Holiday) + any unknown fallback
		return "tc-b"
	}
}

// wocheVariantHue maps a day/week Variant (hit|over|running|under|weekend —
// WocheDayVM.Variant and components.WeekTotalVM.Variant share this
// vocabulary) onto a semantic text-color utility for the Woche row-list's
// duration value and the "Woche gesamt" panel's Ist figure (status color,
// not project color — Farb-Gesetz §7 only bans per-project hue outside the
// avatar). Replaces the retired wocheBarFill, which colored the now-gone
// horizontal per-day progress-bar fill.
func wocheVariantHue(variant string) string {
	switch variant {
	case "hit", "over":
		return "text-green"
	case "running":
		return "text-cyan"
	default: // under, weekend
		return "text-muted"
	}
}

// wocheStatsHue colors the Statistik panel's "Kurs" krow value: green when
// the month is on track, orange when behind (mirrors components.balanceHue's
// on/off semantics for the same on-track vocabulary).
func wocheStatsHue(onTrack bool) string {
	if onTrack {
		return "text-green"
	}
	return "text-orange"
}

// wocheFragmentURL builds the SSE hx-get target for the displayed week. The
// current week omits ?week= so live reloads always follow "now".
func wocheFragmentURL(weekStart time.Time, isCurrent bool) string {
	if isCurrent {
		return "/ui/woche/fragment"
	}
	return "/ui/woche/fragment?week=" + weekStart.Format("2006-01-02")
}

// wocheWeekURL builds a KW-nav hx-get target for the given Monday (yyyy-mm-dd).
// Empty monday → "this week" (no ?week=).
func wocheWeekURL(monday string) string {
	if monday == "" {
		return "/ui/woche/fragment"
	}
	return "/ui/woche/fragment?week=" + monday
}
