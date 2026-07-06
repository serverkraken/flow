package webui

import (
	"fmt"
	"strconv"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

// FmtCompact renders a duration as the calendar's tight form: "1h30", "2h45",
// "45m" (sub-hour). Used in block time-lines and month-cell totals where
// horizontal space is scarce (distinct from FmtVerbose's "1h 30m").
func FmtCompact(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dh%02d", h, m)
}

// HistorieVM is the view model for the Historie calendar (week/month) view on
// the Lesesaal AppShell. The list view uses HistorieListVM. Field shapes are
// the unchanged data source (httpserver's historieCalData/historieBuildWeek/
// historieBuildMonth) — L4 Task 5 rebuilds only the templ presentation +
// historie_vm.go's render helpers.
type HistorieVM struct {
	User    string
	View    string // "cal" | "list"
	CalView string // "week" | "month"

	// Range navigation (week view) / month navigation (month view).
	WeekStart   time.Time
	RangeLabel  string // "16.–22.06.2026" (week) or "Juni 2026" (month)
	MonthLabel  string // "Juni 2026" eyebrow for month view
	PrevHref    string
	NextHref    string
	ThisHref    string // "Diese Woche"/"Dieser Monat" jump ("" when already current)
	CalWeekURL  string // toggle target for Woche tab
	CalMonthURL string // toggle target for Monat tab
	ListHref    string // toggle target for the Liste tab
	FragmentURL string // hx-get for the SSE wrapper (echoes view/cal/week)

	// Week grid.
	WindowFloorMin int // minutes-of-day of grid top (e.g. 360)
	HourPx         int // 48
	GridHeightPx   int // (ceil-floor)/60*HourPx
	HourLabels     []HistorieHourLabel
	Days           []HistorieDayVM // 7 columns (week view)

	// Month grid.
	MonthCells      []HistorieMonthCellVM
	MonthTotal      string // "118h 42m"
	MonthUnassigned int

	Nodes           []components.NodePickerItem
	UnassignedCount int
	Empty           bool
	Err             string
}

// HistorieHourLabel is one hour tick on the time axis.
type HistorieHourLabel struct {
	Label string // "06"
	TopPx int    // (minute-of-day - floor)/60*HourPx, label nudged below the line
}

// HistorieDayVM is one day column (week view) + its mobile agenda rows.
type HistorieDayVM struct {
	Key          string // "Mo" — selection-JS data-day bucket
	DateKey      string // "2026-06-15" — single-edit date param
	Label        string // "Mo"
	DayNum       string // "16"
	DateLabel    string // "16.06." (agenda heading)
	Dur          string // "7h 36m"
	IsToday      bool
	IsWeekend    bool
	NowLineTopPx int    // -1 if not today
	DayOff       bool   // a day-off (Urlaub/Krank/…) falls on this day
	DayOffLabel  string // kind label, e.g. "Urlaub"
	DayOffHue    string // hue token for the day-off badge
	Blocks       []components.SessionBlockVM
	Rows         []components.SessionRowVM // mobile agenda
}

// HistorieMonthCellVM is one day cell in the month grid.
type HistorieMonthCellVM struct {
	DayNum        string
	Hours         string // "7h36" or "" when empty
	HasUnassigned bool
	IsToday       bool
	IsWeekend     bool
	WeekHref      string // jump to that week's calendar
	Empty         bool   // padding cell before the 1st / after the last
	Bars          []HistorieMonthBar
}

// HistorieMonthBar is one stacked mini project-bar in a month cell.
type HistorieMonthBar struct {
	Hue      string // project hue token; "" → unassigned dashed bar
	WidthPct int
	Dashed   bool
}

// HistorieListVM is the paginated flat list view model.
type HistorieListVM struct {
	User  string
	Rows  []components.SessionRowVM
	Nodes []components.NodePickerItem
	Page  components.PageNav
	Empty bool
	Err   string
}

// itoaPx renders an int as a "Npx" CSS length.
func itoaPx(n int) string { return strconv.Itoa(n) + "px" }

// itoa re-exports components.itoa-style int formatting for templ interpolation.
func itoa(n int) string { return strconv.Itoa(n) }

// historieUnFlag returns "1" for unassigned blocks (the selection JS reads
// data-unassigned='1'), else "0".
func historieUnFlag(unassigned bool) string {
	if unassigned {
		return "1"
	}
	return "0"
}

// historieClockStart extracts the leading "HH:MM" from a block time-range like
// "09:00–11:00 · 2h00" for the edit form's start field default.
func historieClockStart(timeRange string) string {
	if len(timeRange) >= 5 {
		return timeRange[:5]
	}
	return ""
}

// ── week grid class helpers (Lesesaal: hairline borders + a cyan accent for
// "heute", no Kristall bg-hue/[.x] washes or font-display) ──────────────────

// historieDayHeadClass styles one day-column header cell: a hairline right
// border shared by all columns, plus a bottom accent rule on today's column
// (the same --cyan "live/heute" token the Wochenskala/now-line/running chip
// already use).
func historieDayHeadClass(d HistorieDayVM) string {
	if d.IsToday {
		return "border-r border-b-2 border-line2 border-b-cyan px-2 py-3 text-left"
	}
	return "border-r border-line2 px-2 py-3 text-left"
}

func historieDayLabelClass(d HistorieDayVM) string {
	switch {
	case d.IsToday:
		return "flex items-center gap-1.5 eyebrow text-cyan whitespace-nowrap"
	case d.IsWeekend:
		return "block eyebrow text-faint"
	default:
		return "block eyebrow"
	}
}

func historieDayNumClass(d HistorieDayVM) string {
	switch {
	case d.IsToday:
		return "text-[1.05rem] font-semibold tnum text-cyan"
	case d.IsWeekend:
		return "text-[1.05rem] font-semibold tnum text-faint"
	default:
		return "text-[1.05rem] font-semibold tnum"
	}
}

func historieDayDurClass(d HistorieDayVM) string {
	if d.IsToday {
		return "hidden xl:inline font-mono text-[.66rem] tnum text-cyan"
	}
	return "hidden xl:inline font-mono text-[.66rem] tnum text-muted"
}

// historieColumnClass styles one day's grid-lines column: a subtle wash on
// today/weekend, using the standard (non-arbitrary) Tailwind opacity scale.
func historieColumnClass(d HistorieDayVM) string {
	switch {
	case d.IsToday:
		return "relative grid-lines bg-cyan/5"
	case d.IsWeekend:
		return "relative grid-lines border-r border-line2 bg-sunken/30"
	default:
		return "relative grid-lines border-r border-line2"
	}
}

func historieAgendaHeadClass(d HistorieDayVM) string {
	if d.IsToday {
		return "text-[1.05rem] font-semibold text-cyan"
	}
	return "text-[1.05rem] font-semibold"
}

// ── month cell class helpers (hairline day-cells, not Kristall cards) ───────

func historieMonthCellClass(c HistorieMonthCellVM) string {
	base := "relative border-b border-r border-line2 p-2 h-[92px] flex flex-col "
	switch {
	case c.IsToday:
		return base + "bg-cyan/5"
	case c.IsWeekend:
		return base + "bg-sunken/30"
	default:
		return base
	}
}

func historieMonthNumClass(c HistorieMonthCellVM) string {
	switch {
	case c.IsToday:
		return "text-[.72rem] tnum text-cyan font-semibold"
	case c.IsWeekend:
		return "text-[.72rem] tnum text-faint"
	default:
		return "text-[.72rem] tnum text-muted"
	}
}

func historieMonthHoursClass(c HistorieMonthCellVM) string {
	if c.IsToday {
		return "font-mono text-[.6rem] tnum text-cyan"
	}
	return "font-mono text-[.6rem] tnum text-body"
}

// historieMonthBarClass colors a stacked mini project-bar by its hue (or a
// dashed neutral bar for the unassigned portion). Hue is whitelisted to avoid
// arbitrary class injection.
func historieMonthBarClass(b HistorieMonthBar) string {
	if b.Dashed {
		return "block h-1.5 rounded-full border border-dashed border-faint bg-sunken"
	}
	switch b.Hue {
	case "blue", "cyan", "green", "purple", "magenta", "yellow", "orange", "red", "teal":
		return "block h-1.5 rounded-full bg-" + b.Hue
	default:
		return "block h-1.5 rounded-full bg-blue"
	}
}

// HistorieSelectionBar builds the bulk-action bar VM shared by calendar+list.
func HistorieSelectionBar(picker components.NodePickerVM, view string) components.SelectionBarVM {
	return components.SelectionBarVM{
		Picker:    picker,
		AssignURL: "/ui/historie/reassign?view=" + view,
		DeleteURL: "/ui/historie/bulk-delete?view=" + view,
	}
}
