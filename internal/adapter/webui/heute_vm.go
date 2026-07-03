package webui

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

// HeuteVM is the view model for the Heute (today) worktime page rendered on the
// Slice-0 AppShell. It composes the running-session card, today's session rows,
// the daily target/balance and the week pace strip out of Task-5 components.
type HeuteVM struct {
	User        string
	Date        time.Time
	Running     *domain.WorkSession
	RunningBase int    // running session's elapsed seconds at render (data-base seed)
	RunningName string // running session's project name (or i18n "ohne Projekt")
	RunningHue  string // running session's project hue ("" → blue default)
	RunningTag  string // running session's tag without '#'
	StartedAt   string // running session start time "11:58"

	Rows     []components.SessionRowVM
	Ledger   []HeuteLedgerRow
	Nodes    []components.NodePickerItem
	HasProj  bool   // true when at least one project exists (drives picker vs plain inputs)
	DayParam string // yyyy-mm-dd for the action forms

	LoggedDur  string // "5h 12m"
	TargetDur  string // "8h 00m"
	TargetPct  int
	TargetVar  string // hit|over|under|running
	Balance    string // "+2h 18m" / "−1h 05m"
	BalancePos bool

	WeekKW    string // "KW 26"
	WeekTotal string // "32h 10m"
	WeekGoal  string // "40h 00m"
	WeekRows  []HeuteWeekRow

	Err string
}

// HeuteLedgerRow pairs a session's display row with its per-session edit-mode
// SessionDialogVM (glass ledger: clicking a block opens the shared dialog
// pre-filled). Edit is the zero SessionDialogVM (Mode "") for a RUNNING
// session — the template skips rendering its dialog.
type HeuteLedgerRow struct {
	Row  components.SessionRowVM
	Edit components.SessionDialogVM
}

// HeuteWeekRow is one day in the week pace strip (Mo..Fr), mirroring the
// studio mockup's per-day bar + pace dot.
type HeuteWeekRow struct {
	Label   string // "Mo"
	Logged  string // "7h 36m"
	Pct     int
	State   string // hit|missed|today (drives bar + dot color)
	IsToday bool
}

// FmtVerbose renders a duration as "5h 12m" (clamped at zero), matching the
// studio Heute design (distinct from format.go's HH:MM fmtDur).
func FmtVerbose(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}

// FmtSaldoVerbose renders a signed balance as "+5h 12m" / "−1h 05m" using a
// real minus glyph for negative values.
func FmtSaldoVerbose(d time.Duration) string {
	if d < 0 {
		return "−" + FmtVerbose(-d)
	}
	return "+" + FmtVerbose(d)
}

// ClampPct clamps a percentage into [0,100].
func ClampPct(p int) int {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

// heuteBarFill maps a week-row state to its bar fill utility.
func heuteBarFill(state string) string {
	switch state {
	case "today":
		return "bg-blue"
	case "hit":
		return "bg-green/70"
	default:
		return "bg-yellow/70"
	}
}

// heuteBarStyle returns the inline width style for a week-row bar.
func heuteBarStyle(r HeuteWeekRow) string {
	return fmt.Sprintf("width:%d%%", ClampPct(r.Pct))
}

// heuteLabelClass colors the week-row weekday label (blue for today).
func heuteLabelClass(r HeuteWeekRow) string {
	if r.IsToday {
		return "w-7 text-[.78rem] font-semibold text-blue"
	}
	return "w-7 text-[.78rem] font-semibold text-muted"
}

// heuteValueClass colors the week-row logged value (blue/bold for today).
func heuteValueClass(r HeuteWeekRow) string {
	if r.IsToday {
		return "w-16 text-right font-mono text-[.78rem] tnum text-blue font-semibold"
	}
	return "w-16 text-right font-mono text-[.78rem] tnum"
}

// heuteDotClass / heuteDotGlyph render the per-day pace dot.
func heuteDotClass(r HeuteWeekRow) string {
	switch r.State {
	case "running":
		return "text-blue text-[.8rem] animate-breathe" // blink ONLY when a timer is actually running this day
	case "today":
		return "text-blue text-[.8rem]" // static "today" marker
	case "hit":
		return "text-green text-[.8rem]"
	default:
		return "text-faint text-[.8rem]"
	}
}

func heuteDotGlyph(r HeuteWeekRow) string {
	if r.State == "missed" {
		return "○"
	}
	return "●"
}

// heuteDotTitle resolves the localized title/aria-label for a pace dot.
func heuteDotTitle(ctx context.Context, r HeuteWeekRow) string {
	switch r.State {
	case "running":
		return components.T(ctx, "heute.running")
	case "today":
		return components.T(ctx, "heute.todayPace")
	case "hit":
		return components.T(ctx, "heute.met")
	default:
		return components.T(ctx, "heute.missed")
	}
}

// secStr renders an int as a string for the live-timer data-base attribute.
func secStr(n int) string { return strconv.Itoa(n) }

// heuteBalanceHue colors the saldo value: green when ahead, red when behind.
func heuteBalanceHue(pos bool) string {
	if pos {
		return "text-green"
	}
	return "text-red"
}

// heuteAccentBar maps a project hue to the hero accent rail color.
func heuteAccentBar(hue string) string {
	switch hue {
	case "blue", "cyan", "green", "purple", "magenta", "yellow", "orange", "red", "teal":
		return "bg-" + hue
	default:
		return "bg-blue"
	}
}

// heuteTileClass maps a project hue to the hero glyph tile wash+text.
func heuteTileClass(hue string) string {
	switch hue {
	case "blue", "cyan", "green", "purple", "magenta", "yellow", "orange", "red", "teal":
		return "bg-" + hue + "/10 text-" + hue
	default:
		return "bg-blue/10 text-blue"
	}
}

// heutePickerNodes converts the Heute booking picker's display items
// ([]components.NodePickerItem) into the []domain.Node shape the shared
// SessionDialog's picker field expects (id + name only). Mirrors the
// same-named helper in httpserver/webui_heute.go, which does the identical
// conversion server-side for the per-row edit dialogs; this package-local
// twin is needed because heute.templ (package webui) cannot reach across the
// adapter boundary into httpserver.
func heutePickerNodes(items []components.NodePickerItem) []domain.Node {
	out := make([]domain.Node, 0, len(items))
	for _, it := range items {
		out = append(out, domain.Node{ID: it.ID, Name: it.Name})
	}
	return out
}

// heuteRowTile picks the glyph-tile wash for a ledger block (whitelisted hues).
func heuteRowTile(row components.SessionRowVM) string {
	if row.Unassigned {
		return "bg-orange/10 text-orange"
	}
	switch row.Hue {
	case "blue", "cyan", "green", "purple", "magenta", "yellow", "orange", "red", "teal":
		return "bg-" + row.Hue + "/10 text-" + row.Hue
	default:
		return "bg-sunken text-body"
	}
}

// heuteDeleteVals builds the hx-vals JSON for a session delete confirm action.
func heuteDeleteVals(dayParam, sessionID string) string {
	return fmt.Sprintf(`{"date":%q,"sessionId":%q}`, dayParam, sessionID)
}
