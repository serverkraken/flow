package webui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

// HeuteVM is the view model for Zeit (/zeit) — L4 Task 3 replaces the Kristall
// day dashboard (Saldo-Kacheln + Mo–Fr pace strip + sub-tab-strip) with the
// Lesesaal Tages-Ledger + vertical 7-day Wochenskala + Werkzeuge menu (Mockup
// Z.845–892). The running timer stays display-only (Spec §10 — no second
// Start/Stop surface); the LIVE ledger row never renders a stop control.
type HeuteVM struct {
	User     string
	Date     time.Time
	Running  *domain.WorkSession
	Ledger   []HeuteLedgerRow
	Nodes    []components.NodePickerItem
	HasProj  bool   // true when at least one project exists (drives picker vs plain inputs)
	DayParam string // yyyy-mm-dd for the action forms

	DateTitle  string // pagehead h1, "Donnerstag, 3. Juli" (FmtDayTitle)
	AllTimeSub string // pagehead sub, "Σ 304h 46m in 41 Sessions seit 24. April · 41 freie Tage gepflegt"

	WeekTotal    string        // this week's logged total (Mon..Sun, FmtVerbose)
	WeekGoal     string        // this week's target total (FmtVerbose)
	WeekGoalLine string        // "Soll 40h 00m · bisher 21h 34m · auf Kurs"
	WeekDays     []ZeitWeekDay // 7 vertical bars, Mo..So

	Tools []ZeitTool

	Err string
}

// HeuteLedgerRow pairs a session's display row with its per-session edit-mode
// SessionDialogVM (clicking a completed row opens the shared dialog
// pre-filled). Edit is the zero SessionDialogVM (Mode "") for a RUNNING
// session — the template skips rendering its dialog and its delete control.
// BaseSeconds feeds the LIVE row's ticking data-timer span (Row.Running only).
// DurationShort/Note are Zeit-Hub-only additions (Review Fix 1/2, kept off
// the shared components.SessionRowVM so Woche/Historie stay unaffected):
// DurationShort is the completed-row duration in the Mockup's "6:10" clock
// format (FmtClockShort), and Note carries the session's free-text note for
// the ledger row's .s sub-line (Mockup Z.858–866 — a real sentence, not tags).
type HeuteLedgerRow struct {
	Row           components.SessionRowVM
	Edit          components.SessionDialogVM
	BaseSeconds   int64
	SinceEpoch    int64 // absolute data-since anchor (LIVE row only; 0 for completed rows)
	DurationShort string
	Note          string
}

// ZeitWeekDay is one vertical bar in the Wochenskala (Mockup Z.871–879): a
// day's label, its logged-time value ("—"/"frei" when there is nothing to
// show), and the bar's height percentage. Built by zeitWeekDays
// (webui_heute.go) from the raw domain.WeekDay rows — never from the lossy
// WocheDayVM (Codex-Fund #3: its pre-formatted Pct is unusable here).
// ValueStr uses FmtClockShort (Mockup "6:10", Review Fix 1), NOT FmtVerbose.
type ZeitWeekDay struct {
	Label    string // "Mo" / "Do · heute"
	ValueStr string // "6:10" / "—" / "frei"
	Pct      int    // bar height %
	Has      bool   // logged time > 0 → accent bar (.day.has)
	Today    bool   // today → live-bright bar (.day.today)
}

// ZeitTool is one Werkzeuge row (Mockup Z.884–888, extended 3→4 with Historie
// for auffindbarkeit — Offene Entsch. #6).
type ZeitTool struct {
	TitleKey string
	DescKey  string
	Href     string
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

// FmtClockShort renders a duration as "6:10" / "40:00" / "304:46" (clamped at
// zero) — the colon clock format the Zeit-Hub Mockup uses for the Wochenskala
// day values and the ledger duration column (Review Fix 1). Deliberately
// NOT a codebase-wide reformat: FmtVerbose ("6h 10m") stays the format
// everywhere else (Home, Woche, Historie) — only the Zeit page's two Mockup-
// specified spots use this. Unlike format.go's unexported fmtDur ("06:10"),
// hours are never zero-padded, matching the Mockup's "6:10" (not "06:10").
func FmtClockShort(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%d:%02d", int(d.Hours()), int(d.Minutes())%60)
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

// zeitDayBarStyle returns the inline height style for a Wochenskala bar
// (dynamic data binding, not a design token — precedent wocheDayBarStyle).
func zeitDayBarStyle(d ZeitWeekDay) string {
	return fmt.Sprintf("height:%d%%", ClampPct(d.Pct))
}

// FmtDayTitle renders the Zeit pagehead's h1 ("Donnerstag, 3. Juli", no year
// — reuses the German weekday/month tables Home's FmtDeskDate defined).
func FmtDayTitle(t time.Time) string {
	return fmt.Sprintf("%s, %d. %s", homeWeekdaysDE[t.Weekday()], t.Day(), homeMonthsDE[t.Month()-1])
}

// FmtDayMonth renders "24. April" (day + German month, no year) for the
// AllTimeSub "seit ..." clause.
func FmtDayMonth(t time.Time) string {
	return fmt.Sprintf("%d. %s", t.Day(), homeMonthsDE[t.Month()-1])
}

// secStr renders an int as a string for the live-timer data-base attribute.
func secStr(n int) string { return strconv.Itoa(n) }

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

// zeitRunningStartLabel extracts the "HH:MM" start time from a running
// session's fmtClockRange output ("14:32–…") for the LIVE ledger row, which
// shows "{start} – {läuft}" instead (Mockup Z.862) rather than fmtClockRange's
// universal "–…" ending (shared by Woche/Historie — left untouched).
func zeitRunningStartLabel(timeRange string) string {
	return strings.TrimSuffix(timeRange, "–…")
}

// zeitLedgerSub renders a ledger row's tag line ("#deep #foo"), empty when
// the session carries no tags. Used as the .s sub-line's FALLBACK when the
// session has no Note (Review Fix 2 — the Mockup's .s is the session's free-
// text Note first; tags only stand in when there is no note at all).
func zeitLedgerSub(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	parts := make([]string, len(tags))
	for i, t := range tags {
		parts[i] = "#" + t
	}
	return strings.Join(parts, " ")
}

// heuteDeleteVals builds the hx-vals JSON for a session delete confirm action.
func heuteDeleteVals(dayParam, sessionID string) string {
	return fmt.Sprintf(`{"date":%q,"sessionId":%q}`, dayParam, sessionID)
}
