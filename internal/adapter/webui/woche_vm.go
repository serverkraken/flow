package webui

import (
	"fmt"
	"strconv"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

// WocheVM is the view model for the Woche (week) worktime page rendered on the
// Slice-0 AppShell: per-day bars (Mo–So), the WOCHE GESAMT banner and the
// KENNZAHLEN panel, plus calendar-week (KW) navigation.
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
	Total       components.WeekTotalVM
	Kennzahlen  components.KennzahlenVM
	WorkdayGoal string // "8h 00m" per-weekday target (header hint)
	Empty       bool   // no logged time anywhere in the week
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
	Variant     string // hit|over|under|running|weekend (drives the bar fill + dot)
	Weekend     bool
	DayOff      bool
	DayOffLabel string // kind label, e.g. "Urlaub"
	DayOffHue   string // web hue token for the day-off chip, e.g. "purple"
	IsToday     bool
}

// wocheBarFill maps a day variant to the bar fill utility (mirrors the studio
// micro-legend: green=hit/over, cyan=running/today, yellow=under).
func wocheBarFill(variant string) string {
	switch variant {
	case "hit", "over":
		return "bg-green"
	case "running":
		return "bg-cyan"
	default:
		return "bg-yellow"
	}
}

// wocheDayDotClass / wocheDayDotGlyph render the per-day trailing status dot.
func wocheDayDotClass(v WocheDayVM) string {
	switch {
	case v.Weekend:
		return "text-faint text-[.85rem] shrink-0 w-3 text-center"
	case v.Variant == "running":
		return "text-cyan text-[.85rem] shrink-0 w-3 text-center"
	case v.Variant == "hit" || v.Variant == "over":
		return "text-green text-[.85rem] shrink-0 w-3 text-center"
	default:
		return "text-faint text-[.85rem] shrink-0 w-3 text-center"
	}
}

func wocheDayDotGlyph(v WocheDayVM) string {
	switch {
	case v.Weekend:
		return "·"
	case v.Variant == "under":
		return "○"
	default:
		return "●"
	}
}

// wocheLabelClass colors today's weekday label blue.
func wocheLabelClass(v WocheDayVM) string {
	if v.IsToday {
		return "text-[.92rem] font-semibold leading-none text-blue"
	}
	if v.Weekend {
		return "text-[.92rem] font-medium leading-none text-muted"
	}
	return "text-[.92rem] font-semibold leading-none"
}

// wocheSaldoHue colors the per-day delta line.
func wocheSaldoHue(pos bool) string {
	if pos {
		return "font-mono text-[.72rem] tnum text-green leading-none mt-0.5"
	}
	return "font-mono text-[.72rem] tnum text-yellow leading-none mt-0.5"
}

// wocheDayOffChip maps a day-off hue token onto its chip wash+text classes.
func wocheDayOffChip(hue string) string {
	switch hue {
	case "blue", "cyan", "green", "purple", "magenta", "yellow", "orange", "red", "teal":
		return "bg-" + hue + "/10 text-" + hue
	default:
		return "bg-blue/10 text-blue"
	}
}

// wochePctStr renders a clamped percentage for the bar aria-valuenow.
func wochePctStr(pct int) string { return strconv.Itoa(ClampPct(pct)) }

// wocheDayBarStyle returns the inline width style for a day bar.
func wocheDayBarStyle(d WocheDayVM) string {
	return fmt.Sprintf("width:%d%%", ClampPct(d.Pct))
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
