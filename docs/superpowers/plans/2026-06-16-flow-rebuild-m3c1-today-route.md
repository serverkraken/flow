# flow rebuild M3c1 — Worktime Today-Route Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the mature `main` Worktime **Today** surface into the rebuild as a focused `shell.Route` package (`internal/tui/screen/worktime/`) — design-faithful render (date · headline with status-pill + threshold color + Ziel% · `BarColored` · Ziel·noch·**ETA** · sessions list with gap-trenner + cursor) **plus** interactivity (j/k cursor, `s` start/stop+booking, edit dialog, delete confirm) wired to the M3c0 `apiclient`, with SSE live-update — and mount it live as a `flow ui` Worktime tab.

**Architecture:** A new self-contained route package reconstructs the `main` design's rich `domain.Day` presentation **client-side** from `apiclient.Today` + `ListSessions` (the rebuild has no `domain.Day`). The route implements the M3b `shell.Route` contract; the host renders the keyhint footer, the route's `View(Frame)` renders the whole body. SSE arrives via the Shell's broadcast — which M3c1 must **export** (currently `shellEventMsg` is unexported, invisible to external route packages). Render/scroll/parse helpers absent from the rebuild are ported locally. No `domain.Day`, no new server endpoint, no Pause-state, no Note-Attach (deferred slices).

**Tech Stack:** Go 1.25.7; `charm.land/bubbletea/v2` (`tea.KeyPressMsg{Text,Code,Mod}`, `tea.Tick`); `charm.land/bubbles/v2/textinput`; the rebuild's `internal/tui/{theme,ui/*,shell}` (ported in M3a/M3b); `internal/adapter/apiclient` (M3c0 surface).

---

## Context for the implementer — VERIFIED APIs (do not redefine, do not guess)

These signatures were read from the real rebuild source on 2026-06-16. Use them exactly.

### Route contract + shell (package `internal/tui/shell`, all exported unless noted)
```go
// route.go
type Frame struct { Width int; Height int; Pal theme.Palette }
type Route interface {
	Title() string
	Init() tea.Cmd
	Update(msg tea.Msg) (Route, tea.Cmd)
	View(f Frame) string
	KeyHints() []keyhint.Hint
}
type PushRouteMsg struct{ Route Route }
type PopRouteMsg struct{}

// shell.go
func New(client *apiclient.Client, user string, pal theme.Palette) Shell
func (s Shell) WithTabs(routes []Route) Shell   // "Used in New, tests, and future M3c wiring"

// host.go — RouteHost.View draws ONLY the footer:
//   body := h.route.View(Frame{Width, Height: h.height-1, Pal})
//   footer := keyhint.Render(h.route.KeyHints(), h.pal)
// => the ROUTE draws its whole body; the HOST draws the 1-line keyhint footer.

// SSE broadcast — CURRENTLY UNEXPORTED (Task 5 exports it):
//   type shellEventMsg struct{ ev apiclient.ClientEvent }   // shell.go:37
//   Shell.Update broadcasts it to every tab via ns.UpdateTop(msg).
```

### apiclient (package `internal/adapter/apiclient`)
```go
// stats.go
type Today struct {
	Date      string `json:"date"`
	LoggedMin int    `json:"loggedMin"`
	TargetMin int    `json:"targetMin"`
	SaldoMin  int    `json:"saldoMin"`
	Running   bool   `json:"running"`
}
func (c *Client) GetToday(ctx context.Context) (Today, error)

// client.go
func (c *Client) ListSessions(ctx context.Context) ([]domain.WorkSession, error)
func (c *Client) StartSession(ctx context.Context, projectID *string, tag, note string) (domain.WorkSession, error)
func (c *Client) StopSession(ctx context.Context, id, projectID string) (domain.WorkSession, error)        // projectID REQUIRED at stop
func (c *Client) EditSession(ctx context.Context, id string, projectID *string, tag, note string, start time.Time, stop *time.Time) (domain.WorkSession, error)
func (c *Client) DeleteSession(ctx context.Context, id string) error
func (c *Client) CreateProject(ctx context.Context, name string) (domain.Project, error)
func (c *Client) ListProjects(ctx context.Context) ([]domain.Project, error)

// events.go
type ClientEvent struct { Type string; Data map[string]any }
func (c *Client) Events(ctx context.Context) (<-chan ClientEvent, error)
```

### domain (package `internal/domain`)
```go
type WorkSession struct {
	ID        string;     OwnerID   string
	ProjectID *string;    Tag       string;  Note string
	Start     time.Time;  Stop      *time.Time;  CreatedAt time.Time
}
func (s WorkSession) Running() bool              // Stop == nil
func (s WorkSession) Elapsed(now time.Time) time.Duration
type Project struct { ID, Name, Slug, Color, Glyph string; Rate *Money; Status ProjectStatus; /*…*/ }

// Event type strings (domain/event.go):
//   "session.started" "session.stopped" "session.updated" "session.deleted"
```

### UI primitives that EXIST in the rebuild (reuse, do not reimplement)
```go
statusbar.BarColored(pct, cells int, filled color.Color, p theme.Palette) string   // ui/statusbar/progress.go
picker.Row(selected bool, label, hint string, width int, p theme.Palette) string   // NOTE: param is `width`
picker.SectionHeader(name string, width int, p theme.Palette) string
toast.NewSuccess(text string, p theme.Palette) toast.Model                          // + NewWarning/NewDanger/NewInfo/NewDefault
func (m toast.Model) Init() tea.Cmd                                                  // schedules auto-dismiss tea.Tick
func (m toast.Model) Update(msg tea.Msg) (toast.Model, tea.Cmd)                      // acts on toast.DismissedMsg
func (m toast.Model) Visible() bool
toast.SlotRows(t *toast.Model, indent string) []string                              // reserves slot rows (pointer receiver)
form.NewTextInput(placeholder string, p theme.Palette) textinput.Model              // ui/form/textinput.go
confirm.New(question, detail string, p theme.Palette) confirm.Model                 // + NewDanger
func (m confirm.Model) Update(msg tea.Msg) (confirm.Model, tea.Cmd)                 // emits confirm.ResultMsg{Confirmed bool}
func (m confirm.Model) View() string
type confirm.ResultMsg struct{ Confirmed bool }
keyhint.Hint{ Key string; Desc string }                                             // host renders via keyhint.Render
glyphs: Active="▶" Done="✓" Paused="‖" BulletDot="·" BarFilled="▰" BarEmpty="▱" Up="▲" Down="▼" AccentBar="▎"
strings.HintHelp = "? → Hilfe"                                                       // ui/strings
theme.Dim(s string, p theme.Palette) string;  theme.Active(s, p) string;  theme.Gap(n int) string
theme spacing: PadXS=1 PadSM=2 PadMD=3 WideBox=80
theme.Palette.FgMuted (field);  p.Sem().{Success,Danger,Warning,Active,Border}      // semantic colors via Sem()
```

### Helpers that DO NOT EXIST in the rebuild — port locally into the route package
- Scroll/layout: `fitHeight`, `windowRows`, `bodyBudget`, `renderFooterHints`, `joinWrapped` (main `screen/worktime/scroll.go` + helpers).
- Duration/format: `formatDur`, `formatDurLive`, `pctOfTarget` (main `screen/worktime/helpers.go`).
- Status helpers: `todayStatusBadge`, `totalThresholdColor` (main `today_render.go` — local copy per M3c spec; promote to `ui/status_adapter` in M3d).
- Parsers: `parseHM`, `parseStop`, `normalizeDurationArg`, `fmtDateDe` (main `domain.ParseHM`/`ParseStop`/`FmtDateDe` — port as unexported route-package helpers; promote to `internal/domain` if M3c2 needs them).

### What NOT to touch / NOT to build
- No `domain.Day`, no new `/today` endpoint, no migration. Reconstruction stays client-side.
- No Pause/Resume state (gaps only), no Note-Attach (`o/O/R/n`), no inline markdown note-viewer, no help dialog inside the route (the Shell's `?` overlay reads `KeyHints()`).
- Do not touch `internal/tui/worktime.go` (the legacy monolith) — it is retired in M3c4, not here. Copy the tiny formatters; don't import the `tui` package.
- Sibling routes (Woche/Stats/DayOffs/Export) and their `w/t/d/e` push wiring are **M3c2** — this route exposes no such keys yet.

---

## Reconstruction map (`main` `domain.Day` → rebuild client-side)

| `main` concept | Rebuild reconstruction (computed in Task 2) |
|---|---|
| `day.Total(now)` | `Logged + (now - *Active)` if running, else `Logged`. `Logged = sum(Elapsed of completed today-sessions)`. Cross-check vs `today.LoggedMin` (server authoritative for completed+running total). |
| `day.Target` | `time.Duration(today.TargetMin) * time.Minute`. |
| `day.IsRunning()` | `today.Running` (authoritative) — equivalently a today-session with `Stop==nil`. |
| `day.Active` | `Start` of the running today-session (`Stop==nil`), else `nil`. |
| `day.Logged` | `sum(Elapsed)` of **completed** today-sessions only (excludes the running one). Used by the ETA formula. |
| `day.Sessions` | `ListSessions` filtered to `Start`'s local-TZ date == today, **completed only** (running rendered separately), sorted by `Start`. |
| ETA | `*Active + (Target - Logged)`, formatted `15:04`; omitted when `Target<=0` or not running. |
| gap/Pause trenner | consecutive completed sessions: `s.Start - prevStop`, rendered when `> 0`. |
| `IsPaused()`/`PausedAt` | **dropped** (no glyph/label/logic). |

---

## File map

| File | Change | Task |
|---|---|---|
| `internal/tui/screen/worktime/format.go` (+ `_test.go`) | port `formatDur`/`formatDurLive`/`pctOfTarget` + parsers `parseHM`/`parseStop`/`normalizeDurationArg`/`fmtDateDe` | 1 |
| `internal/tui/screen/worktime/today_state.go` (+ `_test.go`) | `todayState` + `reconstruct(today, sessions, now)` | 2 |
| `internal/tui/screen/worktime/scroll.go` (+ `_test.go`) | `fitHeight`/`windowRows`/`bodyBudget`/`joinHints` ported to `Frame` | 3 |
| `internal/tui/screen/worktime/render.go` (+ `_test.go`) | View body pipeline + `todayStatusBadge`/`totalThresholdColor` | 4 |
| `internal/tui/shell/shell.go` | export SSE broadcast as `shell.EventMsg{Ev apiclient.ClientEvent}` | 5 |
| `internal/tui/shell/event_test.go` (new) | external-consumer receives `shell.EventMsg` | 5 |
| `internal/tui/screen/worktime/route.go` (+ `_test.go`) | `TodayRoute` (Route contract), `todayAPI` iface, data-load + SSE + cursor + live-tick | 5 |
| `internal/tui/screen/worktime/dialogs.go` (+ route `_test.go` additions) | start/stop+booking overlay, edit form, delete confirm → apiclient | 6 |
| `cmd/flow/ui.go` | wire `TodayRoute` as a Worktime tab via `WithTabs` | 7 |

Build order: pure helpers → reconstruction → layout → render → route skeleton+SSE → actions/dialogs → mount+done-gate. Each route-package file has one responsibility ([[feedback_no_monoliths]]).

---

## Task 1: Port pure helpers (format + parse)

**Files:**
- Create: `internal/tui/screen/worktime/format.go`
- Test: `internal/tui/screen/worktime/format_test.go`

- [ ] **Step 1: Write the failing test** — `internal/tui/screen/worktime/format_test.go`

```go
package worktime

import (
	"testing"
	"time"
)

func TestFormatDur(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0h 00m"},
		{-time.Hour, "0h 00m"},
		{90 * time.Minute, "1h 30m"},
		{2*time.Hour + 5*time.Minute, "2h 05m"},
	}
	for _, c := range cases {
		if got := formatDur(c.d); got != c.want {
			t.Errorf("formatDur(%v) = %q, want %q", c.d, got, c.want)
		}
	}
	if got := formatDurLive(2*time.Hour + 5*time.Minute + 9*time.Second); got != "2h 05m 09s" {
		t.Errorf("formatDurLive = %q", got)
	}
}

func TestPctOfTarget(t *testing.T) {
	if got := pctOfTarget(time.Hour, 2*time.Hour); got != 50 {
		t.Errorf("50%% = %d", got)
	}
	if got := pctOfTarget(3*time.Hour, 2*time.Hour); got != 100 { // clamped
		t.Errorf("clamp = %d", got)
	}
	if got := pctOfTarget(time.Hour, 0); got != 0 { // no target
		t.Errorf("zero target = %d", got)
	}
}

func TestParseHM(t *testing.T) {
	d, err := parseHM("09:30")
	if err != nil || d != 9*time.Hour+30*time.Minute {
		t.Fatalf("parseHM(09:30) = %v, %v", d, err)
	}
	if _, err := parseHM("nonsense"); err == nil {
		t.Fatal("parseHM(nonsense) should error")
	}
}

func TestParseStopRelativeAndAbsolute(t *testing.T) {
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	now := time.Date(2026, 6, 14, 18, 0, 0, 0, time.UTC)
	// relative +1h30m
	got, err := parseStop(normalizeDurationArg("+1h30m"), start, now)
	if err != nil || !got.Equal(start.Add(90*time.Minute)) {
		t.Fatalf("parseStop(+1h30m) = %v, %v", got, err)
	}
}

func TestFmtDateDe(t *testing.T) {
	d := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC) // a Sunday
	if got := fmtDateDe(d); got != "So · 14.06.2026" {
		t.Errorf("fmtDateDe = %q", got)
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`undefined: formatDur` …)

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/screen/worktime/ 2>&1 | head`

- [ ] **Step 3: Implement** — `internal/tui/screen/worktime/format.go`

```go
package worktime

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss/v2"
)

// --- duration formatters (ported verbatim from main screen/worktime/helpers.go) ---

func formatDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}

func formatDurLive(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%dh %02dm %02ds", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}

func pctOfTarget(total, target time.Duration) int {
	if target <= 0 {
		return 0
	}
	pct := int(total * 100 / target)
	if pct > 100 {
		pct = 100
	}
	return pct
}

// shared style vars (ported from main helpers.go) — duration cells align at 8.
var (
	durationWidth8Style = lipgloss.NewStyle().Width(8)
	boldStyle           = lipgloss.NewStyle().Bold(true)
	fgStyle             = lipgloss.NewStyle()
)

// --- date + time parsing (ported from main domain.FmtDateDe/ParseHM/ParseStop) ---

var deWeekday = [...]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}

// fmtDateDe renders "Mo · 14.06.2026".
func fmtDateDe(t time.Time) string {
	return fmt.Sprintf("%s · %02d.%02d.%04d", deWeekday[int(t.Weekday())], t.Day(), int(t.Month()), t.Year())
}

// parseHM parses "HH:MM" into a duration since midnight.
func parseHM(s string) (time.Duration, error) {
	var h, m int
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d:%d", &h, &m); err != nil {
		return 0, fmt.Errorf("invalid HH:MM %q", s)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("out of range %q", s)
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute, nil
}

// normalizeDurationArg strips a leading '+' so a relative stop like "+1h30m"
// becomes a Go duration string "1h30m".
func normalizeDurationArg(s string) string {
	return strings.TrimPrefix(strings.TrimSpace(s), "+")
}

// parseStop resolves a stop time. A pure Go duration (from normalizeDurationArg)
// is added to start; otherwise it tries "HH:MM" on start's date. now is accepted
// for parity with the main signature (future "now"-relative forms); unused today.
func parseStop(arg string, start, _ time.Time) (time.Time, error) {
	if d, err := time.ParseDuration(arg); err == nil {
		return start.Add(d), nil
	}
	hm, err := parseHM(arg)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid stop %q", arg)
	}
	base := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	return base.Add(hm), nil
}
```

> Verify the lipgloss import path matches the rebuild (M3a used `charm.land/...`). Open any `internal/tui/ui/*` file and copy its exact lipgloss import path; adjust the import above to match before running.

- [ ] **Step 4: Run — expect PASS**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/screen/worktime/ -v 2>&1 | tail`

- [ ] **Step 5: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add internal/tui/screen/worktime/format.go internal/tui/screen/worktime/format_test.go && git commit -m "feat(m3c1): port worktime format+parse helpers (dur/pct/HM/stop/dateDe)"
```

---

## Task 2: Client-side reconstruction (`todayState`)

**Files:**
- Create: `internal/tui/screen/worktime/today_state.go`
- Test: `internal/tui/screen/worktime/today_state_test.go`

This is the **central technical risk** (M3c spec): reconstruct the `main` `domain.Day` presentation fields from `apiclient.Today` + today-filtered sessions. Test TZ edges, ETA, and gaps thoroughly.

- [ ] **Step 1: Write the failing test** — `internal/tui/screen/worktime/today_state_test.go`

```go
package worktime

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

func ptr[T any](v T) *T { return &v }

func TestReconstruct_FiltersTodayAndComputesFields(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, loc)
	mk := func(start, stop string) domain.WorkSession {
		s, _ := time.ParseInLocation("2006-01-02 15:04", start, loc)
		ws := domain.WorkSession{ID: start, Start: s}
		if stop != "" {
			e, _ := time.ParseInLocation("2006-01-02 15:04", stop, loc)
			ws.Stop = &e
		}
		return ws
	}
	sessions := []domain.WorkSession{
		mk("2026-06-13 09:00", "2026-06-13 17:00"), // yesterday — excluded
		mk("2026-06-14 09:00", "2026-06-14 10:00"), // today, 1h
		mk("2026-06-14 10:30", "2026-06-14 11:00"), // today, 0.5h (gap 30m before)
		mk("2026-06-14 11:30", ""),                 // today, running
	}
	today := apiclient.Today{TargetMin: 480, LoggedMin: 90, Running: true}

	st := reconstruct(today, sessions, now)

	if len(st.Completed) != 2 {
		t.Fatalf("Completed = %d, want 2 (today, stopped)", len(st.Completed))
	}
	if !st.Running || st.Active == nil || !st.Active.Equal(mk("2026-06-14 11:30", "").Start) {
		t.Fatalf("running/active wrong: %+v", st)
	}
	if st.Target != 8*time.Hour {
		t.Fatalf("Target = %v", st.Target)
	}
	if st.Logged != 90*time.Minute { // completed only
		t.Fatalf("Logged = %v, want 90m", st.Logged)
	}
	// Total(now) = Logged + (now - Active) = 90m + 30m = 2h
	if got := st.Total(now); got != 2*time.Hour {
		t.Fatalf("Total = %v, want 2h", got)
	}
	// gap before the 2nd completed session = 30m
	if st.Completed[1].GapBefore != 30*time.Minute {
		t.Fatalf("gap = %v, want 30m", st.Completed[1].GapBefore)
	}
}

func TestReconstruct_ETA(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, loc)
	active := time.Date(2026, 6, 14, 11, 0, 0, 0, loc)
	st := todayState{Running: true, Active: &active, Logged: 6 * time.Hour, Target: 8 * time.Hour}
	// ETA = Active + (Target - Logged) = 11:00 + 2h = 13:00
	eta, ok := st.ETA()
	if !ok || eta.Format("15:04") != "13:00" {
		t.Fatalf("ETA = %v ok=%v", eta, ok)
	}
	// no target -> no ETA
	st.Target = 0
	if _, ok := st.ETA(); ok {
		t.Fatal("no-target ETA should be absent")
	}
}

func TestReconstruct_LocalTZBoundary(t *testing.T) {
	// 23:30 local on 06-14 must count as 06-14 even though it is 21:30 UTC.
	loc := time.FixedZone("CEST", 2*3600)
	now := time.Date(2026, 6, 14, 23, 45, 0, 0, loc)
	s := time.Date(2026, 6, 14, 23, 30, 0, 0, loc)
	e := time.Date(2026, 6, 14, 23, 40, 0, 0, loc)
	st := reconstruct(apiclient.Today{}, []domain.WorkSession{{ID: "x", Start: s, Stop: &e}}, now)
	if len(st.Completed) != 1 {
		t.Fatalf("late-night session dropped: %d", len(st.Completed))
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`undefined: reconstruct` / `todayState`)

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/screen/worktime/ -run TestReconstruct 2>&1 | head`

- [ ] **Step 3: Implement** — `internal/tui/screen/worktime/today_state.go`

```go
package worktime

import (
	"sort"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// completedSession is one finished session enriched for rendering.
type completedSession struct {
	ID        string
	Start     time.Time
	Stop      time.Time
	Elapsed   time.Duration
	Tag       string
	Note      string
	GapBefore time.Duration // idle gap since the previous session's stop (0 = none)
}

// todayState is the client-side reconstruction of the main design's domain.Day
// presentation, built from apiclient.Today + today-filtered sessions.
type todayState struct {
	Completed []completedSession
	Running   bool
	Active    *time.Time    // start of the running session, nil if idle
	ActiveID  string        // id of the running session (for stop/booking)
	Logged    time.Duration // sum of completed Elapsed (excludes running)
	Target    time.Duration
}

// sameLocalDay reports whether a and b fall on the same calendar day in a's location.
func sameLocalDay(a, b time.Time) bool {
	b = b.In(a.Location())
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// reconstruct builds todayState from the server today-totals and the full
// session list, filtering to `now`'s local calendar day.
func reconstruct(today apiclient.Today, sessions []domain.WorkSession, now time.Time) todayState {
	st := todayState{Target: time.Duration(today.TargetMin) * time.Minute, Running: today.Running}

	todays := make([]domain.WorkSession, 0, len(sessions))
	for _, s := range sessions {
		if sameLocalDay(now, s.Start) {
			todays = append(todays, s)
		}
	}
	sort.Slice(todays, func(i, j int) bool { return todays[i].Start.Before(todays[j].Start) })

	var prevStop time.Time
	for _, s := range todays {
		if s.Running() {
			start := s.Start
			st.Active = &start
			st.ActiveID = s.ID
			st.Running = true
			continue
		}
		gap := time.Duration(0)
		if !prevStop.IsZero() {
			if g := s.Start.Sub(prevStop); g > 0 {
				gap = g
			}
		}
		el := s.Stop.Sub(s.Start)
		st.Completed = append(st.Completed, completedSession{
			ID: s.ID, Start: s.Start, Stop: *s.Stop, Elapsed: el,
			Tag: s.Tag, Note: s.Note, GapBefore: gap,
		})
		st.Logged += el
		prevStop = *s.Stop
	}
	return st
}

// Total is Logged plus the live increment of the running session.
func (st todayState) Total(now time.Time) time.Duration {
	t := st.Logged
	if st.Running && st.Active != nil {
		if d := now.Sub(*st.Active); d > 0 {
			t += d
		}
	}
	return t
}

// ETA returns the projected target-completion clock time (Active + remaining
// unlogged duration). ok=false when not running or no target.
func (st todayState) ETA() (time.Time, bool) {
	if !st.Running || st.Active == nil || st.Target <= 0 {
		return time.Time{}, false
	}
	return st.Active.Add(st.Target - st.Logged), true
}
```

> Note: `Logged` is reconstructed from completed sessions (needed for ETA). `today.LoggedMin` (server) is the authoritative *display* total incl. the running session; the render uses `Total(now)` for live ticking. The two agree at whole-minute granularity; do not assert exact equality across them.

- [ ] **Step 4: Run — expect PASS**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/screen/worktime/ -run TestReconstruct -v 2>&1 | tail`

- [ ] **Step 5: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add internal/tui/screen/worktime/today_state.go internal/tui/screen/worktime/today_state_test.go && git commit -m "feat(m3c1): client-side todayState reconstruction (today-filter, Total, ETA, gaps)"
```

---

## Task 3: Scroll/layout port (`fitHeight`/`windowRows`/`bodyBudget`)

**Files:**
- Create: `internal/tui/screen/worktime/scroll.go`
- Test: `internal/tui/screen/worktime/scroll_test.go`

Port the `main` scroll helpers, adapted to the rebuild's `Frame` model. The host reserves the footer line, so the route's budget is `Frame.Height` and the route's "footer" rows are only the toast slot.

- [ ] **Step 1: Write the failing test** — `internal/tui/screen/worktime/scroll_test.go`

```go
package worktime

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/tui/theme"
)

func TestFitHeight_WindowsAroundFocus(t *testing.T) {
	pal := theme.Load()
	header := []string{"H1", "H2"}
	mid := make([]string, 0, 20)
	for i := 0; i < 20; i++ {
		mid = append(mid, "row")
	}
	footer := []string{"F"}
	// budget 8: 2 header + 1 footer => 5 mid rows visible, windowed around focus=15
	out := fitHeight(header, mid, footer, 15, 8, pal)
	lines := strings.Split(out, "\n")
	if len(lines) > 8 {
		t.Fatalf("over budget: %d lines", len(lines))
	}
	if lines[0] != "H1" || lines[len(lines)-1] != "F" {
		t.Fatalf("header/footer not pinned: %q…%q", lines[0], lines[len(lines)-1])
	}
}

func TestBodyBudget(t *testing.T) {
	if got := bodyBudget(20); got != 20 { // host already reserved its footer; no titlebox here
		t.Fatalf("bodyBudget = %d", got)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/screen/worktime/ -run 'TestFitHeight|TestBodyBudget' 2>&1 | head`

- [ ] **Step 3: Implement** — `internal/tui/screen/worktime/scroll.go`

```go
package worktime

import (
	"fmt"
	"strings"

	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
)

// bodyBudget is the row budget for the route body. The RouteHost already
// reserves its own 1-line keyhint footer, so the full Frame.Height is ours.
func bodyBudget(height int) int { return height }

// fitHeight lays out fixed header + scrollable mid + fixed footer within budget,
// windowing the mid region around `focus`. Ported from main screen/worktime/scroll.go,
// adapted to the rebuild (footer here = toast slot only).
func fitHeight(header, mid, footer []string, focus, budget int, pal theme.Palette) string {
	rows := append([]string(nil), header...)
	remaining := budget - len(header)
	foot := footer
	if len(foot) >= remaining {
		foot = nil
	}
	midBudget := remaining - len(foot)
	rows = append(rows, windowRows(mid, focus, midBudget, pal)...)
	rows = append(rows, foot...)
	return strings.Join(rows, "\n")
}

// windowRows returns at most `budget` rows from mid, scrolled so `focus` is
// visible, with dim "▲ N darüber" / "▼ N darunter" overflow markers.
func windowRows(mid []string, focus, budget int, pal theme.Palette) []string {
	if budget <= 0 {
		return nil
	}
	if len(mid) <= budget {
		return mid
	}
	start := focus - budget/2
	if start < 0 {
		start = 0
	}
	if start+budget > len(mid) {
		start = len(mid) - budget
	}
	end := start + budget
	out := make([]string, 0, budget)
	if start > 0 {
		out = append(out, theme.Dim(fmt.Sprintf("  %s %d darüber", glyphs.Up, start), pal))
		out = append(out, mid[start+1:end]...)
	} else {
		out = append(out, mid[start:end]...)
	}
	if end < len(mid) {
		out[len(out)-1] = theme.Dim(fmt.Sprintf("  %s %d darunter", glyphs.Down, len(mid)-end+1), pal)
	}
	return out
}

// joinHints renders a "  ·  "-joined dim hint line (main renderFooterHints).
func joinHints(parts []string, pal theme.Palette) string {
	return theme.Dim("  "+strings.Join(parts, "  ·  "), pal)
}
```

> The exact overflow-marker arithmetic is a faithful adaptation, not a byte-for-byte port; the test only asserts the budget bound + pinned header/footer. If you prefer, open `main:internal/frontend/tui/screen/worktime/scroll.go` and match `windowRows` precisely — but keep the `Frame`-budget semantics above.

- [ ] **Step 4: Run — expect PASS**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/screen/worktime/ -run 'TestFitHeight|TestBodyBudget' -v 2>&1 | tail`

- [ ] **Step 5: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add internal/tui/screen/worktime/scroll.go internal/tui/screen/worktime/scroll_test.go && git commit -m "feat(m3c1): port scroll/window layout helpers adapted to Frame budget"
```

---

## Task 4: Render pipeline (body View)

**Files:**
- Create: `internal/tui/screen/worktime/render.go`
- Test: `internal/tui/screen/worktime/render_test.go`

Render the design-faithful body from a `todayState`. Local copies of `todayStatusBadge`/`totalThresholdColor` (M3c spec: promote to `ui/status_adapter` in M3d). Golden tests compare ANSI-stripped plain text for stability.

- [ ] **Step 1: Write the failing test** — `internal/tui/screen/worktime/render_test.go`

```go
package worktime

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/tui/theme"
)

var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

func plain(s string) string { return ansiRE.ReplaceAllString(s, "") }

func TestRenderBody_HeadlineBarSummarySessions(t *testing.T) {
	pal := theme.Load()
	loc := time.UTC
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, loc)
	c1Start := time.Date(2026, 6, 14, 9, 0, 0, 0, loc)
	c1Stop := time.Date(2026, 6, 14, 10, 0, 0, 0, loc)
	active := time.Date(2026, 6, 14, 11, 0, 0, 0, loc)
	st := todayState{
		Completed: []completedSession{{ID: "a", Start: c1Start, Stop: c1Stop, Elapsed: time.Hour, Tag: "deep"}},
		Running:   true, Active: &active, ActiveID: "run", Logged: time.Hour, Target: 8 * time.Hour,
	}
	body := plain(renderBody(st, 0, 80, 24, now, nil, pal))

	if !strings.Contains(body, "So · 14.06.2026") {
		t.Errorf("missing date line:\n%s", body)
	}
	if !strings.Contains(body, "läuft") {
		t.Errorf("missing running badge")
	}
	if !strings.Contains(body, "Ziel 8h 00m") || !strings.Contains(body, "ETA") {
		t.Errorf("missing summary/ETA:\n%s", body)
	}
	if !strings.Contains(body, "[deep]") {
		t.Errorf("missing tag hint")
	}
	if !strings.Contains(body, "09:00 → 10:00") {
		t.Errorf("missing completed session line")
	}
}

func TestRenderBody_EmptyState(t *testing.T) {
	pal := theme.Load()
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	body := plain(renderBody(todayState{Target: 8 * time.Hour}, 0, 80, 24, now, nil, pal))
	if !strings.Contains(body, "Noch nichts erfasst") {
		t.Errorf("missing empty state:\n%s", body)
	}
}

func TestRenderBody_GapSeparator(t *testing.T) {
	pal := theme.Load()
	loc := time.UTC
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, loc)
	mk := func(h1, h2 int, gap time.Duration) completedSession {
		s := time.Date(2026, 6, 14, h1, 0, 0, 0, loc)
		e := time.Date(2026, 6, 14, h2, 0, 0, 0, loc)
		return completedSession{Start: s, Stop: e, Elapsed: e.Sub(s), GapBefore: gap}
	}
	st := todayState{Completed: []completedSession{mk(9, 10, 0), mk(11, 12, time.Hour)}, Target: 8 * time.Hour}
	body := plain(renderBody(st, 0, 80, 24, now, nil, pal))
	if !strings.Contains(body, "Pause 1h 00m") {
		t.Errorf("missing gap separator:\n%s", body)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/screen/worktime/ -run TestRenderBody 2>&1 | head`

- [ ] **Step 3: Implement** — `internal/tui/screen/worktime/render.go`

```go
package worktime

import (
	"fmt"
	"image/color"
	"time"

	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
	"github.com/serverkraken/flow/internal/tui/ui/picker"
	"github.com/serverkraken/flow/internal/tui/ui/statusbar"
	"github.com/serverkraken/flow/internal/tui/ui/toast"
)

// renderBody is the full Today body (no keyhint footer — the host draws that).
// cursor selects a completed-session row; tt is an optional toast slot.
func renderBody(st todayState, cursor, width, height int, now time.Time, tt *toast.Model, pal theme.Palette) string {
	inner := width - 4
	if inner <= 0 {
		inner = theme.WideBox
	}
	header := []string{
		renderDateLine(now, pal),
		renderHeadline(st, now, pal),
		"",
		renderProgressBar(st, inner, now, pal),
		renderSummary(st, now, pal),
	}
	mid, focus := renderSessionsList(st, cursor, inner, now, pal)

	var footer []string
	if tt != nil {
		footer = toast.SlotRows(tt, "  ")
	}
	return fitHeight(header, mid, footer, focus, bodyBudget(height), pal)
}

func renderDateLine(now time.Time, pal theme.Palette) string {
	return theme.Gap(theme.PadSM) + theme.Dim(fmtDateDe(now), pal)
}

func renderHeadline(st todayState, now time.Time, pal theme.Palette) string {
	total := st.Total(now)
	target := st.Target
	glyph, label, statusColor := todayStatusBadge(pal, st.Running, target > 0 && total >= target)

	totalText := formatDur(total)
	if st.Running && st.Active != nil && now.Sub(*st.Active) < time.Minute {
		totalText = formatDurLive(total)
	}
	totalStr := fgStyle.Foreground(totalThresholdColor(pal, total, target, st.Running)).Render(totalText)
	statusStr := boldStyle.Foreground(statusColor).Render(glyph + " " + label)
	pctStr := theme.Dim("kein Ziel", pal)
	if target > 0 {
		pctStr = theme.Dim(fmt.Sprintf("Ziel %d%%", pctOfTarget(total, target)), pal)
	}
	gap4 := theme.Gap(theme.PadMD + theme.PadXS)
	return theme.Gap(theme.PadSM) + totalStr + gap4 + statusStr + gap4 + pctStr
}

func renderProgressBar(st todayState, inner int, now time.Time, pal theme.Palette) string {
	total := st.Total(now)
	pct := pctOfTarget(total, st.Target)
	cells := inner - 4
	if cells < 4 {
		cells = 4
	}
	barColor := totalThresholdColor(pal, total, st.Target, st.Running)
	return "  " + statusbar.BarColored(pct, cells, barColor, pal)
}

func renderSummary(st todayState, now time.Time, pal theme.Palette) string {
	if st.Target <= 0 {
		return joinHints([]string{"kein Tagesziel"}, pal)
	}
	total := st.Total(now)
	remaining := st.Target - total
	if remaining < 0 {
		remaining = 0
	}
	parts := []string{
		fmt.Sprintf("Ziel %s", formatDur(st.Target)),
		fmt.Sprintf("noch %s", formatDur(remaining)),
	}
	if eta, ok := st.ETA(); ok {
		parts = append(parts, "ETA "+eta.Format("15:04"))
	}
	return joinHints(parts, pal)
}

// renderSessionsList returns mid rows and the focus index (cursor row position
// within mid) for windowing. The running session is a non-selectable top row;
// the cursor indexes Completed.
func renderSessionsList(st todayState, cursor, inner int, now time.Time, pal theme.Palette) (rows []string, focus int) {
	total := len(st.Completed)
	if st.Running {
		total++
	}
	if total == 0 {
		return []string{"", theme.Dim("  Noch nichts erfasst — `s` startet", pal)}, 0
	}
	rows = []string{"", picker.SectionHeader(fmt.Sprintf("sessions heute (%d)", total), inner, pal)}

	if st.Running && st.Active != nil {
		elapsed := now.Sub(*st.Active)
		rows = append(rows, theme.Active(
			fmt.Sprintf("  %s %s → …   %s", glyphs.Active, st.Active.Format("15:04"), formatDur(elapsed)), pal))
	}
	for i, s := range st.Completed {
		if s.GapBefore > 0 {
			rows = append(rows, theme.Dim(
				fmt.Sprintf("%s%s Pause %s", theme.Gap(theme.PadMD*2+theme.PadXS), glyphs.BulletDot, formatDur(s.GapBefore)), pal))
		}
		dur := durationWidth8Style.Render(formatDur(s.Elapsed))
		label := fmt.Sprintf("%s → %s   %s", s.Start.Format("15:04"), s.Stop.Format("15:04"), dur)
		hint := ""
		if s.Tag != "" {
			hint = "[" + s.Tag + "]"
		}
		if i == cursor {
			focus = len(rows)
		}
		rows = append(rows, picker.Row(i == cursor, label, hint, inner, pal))
		if s.Note != "" {
			rows = append(rows, theme.Dim("       "+s.Note, pal))
		}
	}
	return rows, focus
}

// --- local status helpers (ported from main today_render.go; → ui/status_adapter in M3d) ---

func todayStatusBadge(p theme.Palette, running, achieved bool) (string, string, color.Color) {
	sem := p.Sem()
	switch {
	case running && achieved:
		return glyphs.Active, "läuft " + glyphs.Done, sem.Success
	case running:
		return glyphs.Active, "läuft", sem.Active
	case achieved:
		return glyphs.Done, "Ziel erreicht", sem.Success
	}
	return glyphs.Paused, "pausiert", p.FgMuted
}

func totalThresholdColor(p theme.Palette, total, target time.Duration, running bool) color.Color {
	sem := p.Sem()
	switch {
	case total >= target+4*time.Hour:
		return sem.Danger
	case total >= target:
		return sem.Success
	case running && total >= target-2*time.Hour:
		return sem.Warning
	case running:
		return sem.Active
	}
	return p.FgMuted
}
```

> Verify two import/type details before running: (1) the rebuild's color type for `BarColored`/style `.Foreground(...)` — main used `image/color`'s `color.Color`; confirm `theme.Color` vs `color.Color` by opening `ui/statusbar/progress.go` (its `filled color.Color` param) and `theme/builders.go`. If the rebuild aliases `theme.Color`, change the helper return types and the `fgStyle.Foreground(...)` argument types to match. (2) `p.Sem()` field names (`Success/Danger/Warning/Active/Border`) per the verified palette.

- [ ] **Step 4: Run — expect PASS**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/screen/worktime/ -run TestRenderBody -v 2>&1 | tail`

- [ ] **Step 5: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add internal/tui/screen/worktime/render.go internal/tui/screen/worktime/render_test.go && git commit -m "feat(m3c1): Today body render (date/headline/bar/summary/sessions) + status helpers"
```

---

## Task 5: Route skeleton — Route contract, data load, SSE export, cursor, live tick

**Files:**
- Modify: `internal/tui/shell/shell.go` (export the SSE broadcast message)
- Create: `internal/tui/shell/event_test.go`
- Create: `internal/tui/screen/worktime/route.go`
- Test: `internal/tui/screen/worktime/route_test.go`

The Shell broadcasts SSE to tabs via an **unexported** `shellEventMsg` — invisible to external route packages. M3c1 is the first external route, so it must export it.

- [ ] **Step 1a: Export the SSE broadcast** — in `internal/tui/shell/shell.go`:
  - Rename the type `shellEventMsg` → `EventMsg` and its field `ev` → `Ev` (exported):
    ```go
    // EventMsg carries one server SSE event, broadcast by the Shell to every
    // tab's top route so routes can refresh. Exported so route packages outside
    // `shell` can type-switch on it.
    type EventMsg struct{ Ev apiclient.ClientEvent }
    ```
  - Update every internal reference in `shell.go` (the `case shellEventMsg:` in `Update`, and `waitForShellEvent` which returns `shellEventMsg{ev}` → `EventMsg{Ev: ev}`). Use `rg -n "shellEventMsg" internal/tui/` first and fix ALL hits.

- [ ] **Step 1b: Write the failing tests** — `internal/tui/shell/event_test.go`

```go
package shell_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/tui/shell"
)

// an external package can construct & read shell.EventMsg (compile-level proof
// the broadcast type is exported for route packages).
func TestEventMsgExported(t *testing.T) {
	m := shell.EventMsg{Ev: apiclient.ClientEvent{Type: "session.started"}}
	if m.Ev.Type != "session.started" {
		t.Fatalf("EventMsg field not accessible: %+v", m)
	}
}
```

- [ ] **Step 2: Run — expect FAIL then (after 1a) the shell package compiles; expect the worktime route test to fail next**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/shell/ 2>&1 | tail`
Expected: shell tests PASS after the rename is consistent.

- [ ] **Step 3: Write the failing route test** — `internal/tui/screen/worktime/route_test.go`

```go
package worktime

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
)

// fakeAPI implements todayAPI for reducer tests.
type fakeAPI struct {
	today    apiclient.Today
	sessions []domain.WorkSession
	projects []domain.Project
	started  bool
	stopped  [2]string // {id, projectID}
	edited   string
	deleted  string
}

func (f *fakeAPI) GetToday(context.Context) (apiclient.Today, error)            { return f.today, nil }
func (f *fakeAPI) ListSessions(context.Context) ([]domain.WorkSession, error)   { return f.sessions, nil }
func (f *fakeAPI) ListProjects(context.Context) ([]domain.Project, error)       { return f.projects, nil }
func (f *fakeAPI) StartSession(context.Context, *string, string, string) (domain.WorkSession, error) {
	f.started = true
	return domain.WorkSession{ID: "new"}, nil
}
func (f *fakeAPI) StopSession(_ context.Context, id, pid string) (domain.WorkSession, error) {
	f.stopped = [2]string{id, pid}
	return domain.WorkSession{ID: id}, nil
}
func (f *fakeAPI) EditSession(_ context.Context, id string, _ *string, _, _ string, _ time.Time, _ *time.Time) (domain.WorkSession, error) {
	f.edited = id
	return domain.WorkSession{ID: id}, nil
}
func (f *fakeAPI) DeleteSession(_ context.Context, id string) error { f.deleted = id; return nil }
func (f *fakeAPI) CreateProject(_ context.Context, name string) (domain.Project, error) {
	return domain.Project{ID: "p-" + name, Name: name}, nil
}

func fixedNow() time.Time { return time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC) }

func newTestRoute(f *fakeAPI) *TodayRoute { return NewTodayRoute(f, fixedNow) }

func TestRoute_LoadPopulatesState(t *testing.T) {
	f := &fakeAPI{today: apiclient.Today{TargetMin: 480, LoggedMin: 60}, sessions: []domain.WorkSession{
		{ID: "s1", Start: time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC), Stop: ptr(time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC))},
	}}
	r := newTestRoute(f)
	cmd := r.Init()
	msg := cmd() // run the load cmd
	r2, _ := r.Update(msg)
	rt := r2.(*TodayRoute)
	if len(rt.st.Completed) != 1 || rt.st.Target != 8*time.Hour {
		t.Fatalf("state not loaded: %+v", rt.st)
	}
}

func TestRoute_SSETriggersReload(t *testing.T) {
	f := &fakeAPI{}
	r := newTestRoute(f)
	_, cmd := r.Update(shell.EventMsg{Ev: apiclient.ClientEvent{Type: "session.started"}})
	if cmd == nil {
		t.Fatal("session.* event should trigger a reload cmd")
	}
	// an unrelated event should NOT reload
	if _, c := r.Update(shell.EventMsg{Ev: apiclient.ClientEvent{Type: "document.created"}}); c != nil {
		t.Fatal("unrelated event should not reload")
	}
}

func TestRoute_CursorMoves(t *testing.T) {
	f := &fakeAPI{}
	r := newTestRoute(f)
	r.st = todayState{Completed: make([]completedSession, 3)}
	r.applyKey("j")
	if r.cursor != 1 {
		t.Fatalf("cursor j = %d", r.cursor)
	}
	r.applyKey("k")
	r.applyKey("k") // wraps
	if r.cursor != 2 {
		t.Fatalf("cursor wrap = %d", r.cursor)
	}
}
```

> `applyKey(string)` is a tiny test seam on `*TodayRoute` that routes a printable key through the same logic as `Update(tea.KeyPressMsg{Text:…})`. Implement it as an unexported method used by both.

- [ ] **Step 4: Implement** — `internal/tui/screen/worktime/route.go`

```go
package worktime

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
	"github.com/serverkraken/flow/internal/tui/ui/toast"
)

// todayAPI is the apiclient surface this route consumes (consumer-defined for
// testability; *apiclient.Client satisfies it structurally).
type todayAPI interface {
	GetToday(context.Context) (apiclient.Today, error)
	ListSessions(context.Context) ([]domain.WorkSession, error)
	ListProjects(context.Context) ([]domain.Project, error)
	StartSession(context.Context, *string, string, string) (domain.WorkSession, error)
	StopSession(ctx context.Context, id, projectID string) (domain.WorkSession, error)
	EditSession(ctx context.Context, id string, projectID *string, tag, note string, start time.Time, stop *time.Time) (domain.WorkSession, error)
	DeleteSession(ctx context.Context, id string) error
	CreateProject(ctx context.Context, name string) (domain.Project, error)
}

type dialogKind int

const (
	dialogNone dialogKind = iota
	dialogBooking
	dialogEdit
	dialogDelete
)

// loadedMsg carries a completed data fetch.
type loadedMsg struct {
	today    apiclient.Today
	sessions []domain.WorkSession
	err      error
}

// projectsMsg carries the project list (for the booking overlay).
type projectsMsg struct{ projects []domain.Project }

// liveTickMsg drives the running-session live total.
type liveTickMsg struct{}

// TodayRoute is the Worktime Today shell.Route.
type TodayRoute struct {
	api    todayAPI
	now    func() time.Time
	pal    theme.Palette

	st      todayState
	cursor  int
	loaded  bool
	err     error
	toast   toast.Model

	dialog   dialogKind
	booking  bookingState
	edit     editState
	confirm  confirmState
}

// NewTodayRoute builds the route. `now` is injectable for tests (pass time.Now in prod).
func NewTodayRoute(api todayAPI, now func() time.Time) *TodayRoute {
	if now == nil {
		now = time.Now
	}
	return &TodayRoute{api: api, now: now}
}

func (r *TodayRoute) Title() string { return "Worktime" }

func (r *TodayRoute) Init() tea.Cmd { return r.loadCmd() }

func (r *TodayRoute) loadCmd() tea.Cmd {
	api := r.api
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		today, err := api.GetToday(ctx)
		if err != nil {
			return loadedMsg{err: err}
		}
		sessions, err := api.ListSessions(ctx)
		return loadedMsg{today: today, sessions: sessions, err: err}
	}
}

func liveTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return liveTickMsg{} })
}

func (r *TodayRoute) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	switch m := msg.(type) {
	case loadedMsg:
		r.loaded = true
		r.err = m.err
		if m.err == nil {
			r.st = reconstruct(m.today, m.sessions, r.now())
			if r.cursor >= len(r.st.Completed) {
				r.cursor = max(0, len(r.st.Completed)-1)
			}
		}
		if r.st.Running {
			return r, liveTickCmd()
		}
		return r, nil
	case liveTickMsg:
		if r.st.Running {
			return r, liveTickCmd() // re-render via tick; View reads now()
		}
		return r, nil
	case projectsMsg:
		r.booking.projects = m.projects
		return r, nil
	case toast.DismissedMsg:
		r.toast, _ = r.toast.Update(m)
		return r, nil
	case shell.EventMsg:
		if isSessionEvent(m.Ev.Type) {
			return r, r.loadCmd()
		}
		return r, nil
	case tea.KeyPressMsg:
		return r.handleKey(m)
	}
	return r, nil
}

func isSessionEvent(t string) bool {
	switch domain.EventType(t) {
	case domain.EventSessionStarted, domain.EventSessionStopped,
		domain.EventSessionUpdated, domain.EventSessionDeleted:
		return true
	}
	return false
}

// handleKey dispatches to the active dialog or the normal-mode handler.
func (r *TodayRoute) handleKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	if r.dialog != dialogNone {
		return r.handleDialogKey(k) // implemented in Task 6
	}
	// normal mode
	switch {
	case k.Text == "j" || k.Code == tea.KeyDown:
		r.applyKey("j")
	case k.Text == "k" || k.Code == tea.KeyUp:
		r.applyKey("k")
	case k.Text == "g":
		r.cursor = 0
	case k.Text == "G":
		r.cursor = max(0, len(r.st.Completed)-1)
	case k.Text == "s":
		return r.startOrStop() // Task 6
	case k.Text == "E" || k.Code == tea.KeyEnter:
		return r.openEdit() // Task 6
	case k.Text == "D":
		return r.openDelete() // Task 6
	}
	return r, nil
}

// applyKey is the cursor seam shared by Update and tests.
func (r *TodayRoute) applyKey(key string) {
	n := len(r.st.Completed)
	if n == 0 {
		r.cursor = 0
		return
	}
	switch key {
	case "j":
		r.cursor = (r.cursor + 1) % n
	case "k":
		r.cursor = (r.cursor + n - 1) % n
	}
}

func (r *TodayRoute) View(f shell.Frame) string {
	r.pal = f.Pal
	if !r.loaded {
		return theme.Dim("  Heute lädt …", f.Pal)
	}
	if r.err != nil {
		return theme.Dim("  Fehler: "+r.err.Error(), f.Pal)
	}
	if r.dialog != dialogNone {
		return r.renderDialog(f) // Task 6
	}
	return renderBody(r.st, r.cursor, f.Width, f.Height, r.now(), &r.toast, f.Pal)
}

func (r *TodayRoute) KeyHints() []keyhint.Hint {
	if r.dialog != dialogNone {
		return r.dialogHints() // Task 6
	}
	hints := []keyhint.Hint{}
	if r.st.Running {
		hints = append(hints, keyhint.Hint{Key: "s", Desc: "stoppen"})
	} else {
		hints = append(hints, keyhint.Hint{Key: "s", Desc: "starten"})
	}
	hints = append(hints, keyhint.Hint{Key: "j/k", Desc: "bewegen"})
	if len(r.st.Completed) > 0 {
		hints = append(hints, keyhint.Hint{Key: "enter", Desc: "bearbeiten"})
	}
	hints = append(hints, keyhint.Hint{Key: "?", Desc: "Hilfe"})
	if len(hints) > 4 {
		hints = hints[:4]
	}
	return hints
}
```

> The methods `handleDialogKey`, `startOrStop`, `openEdit`, `openDelete`, `renderDialog`, `dialogHints`, and the `bookingState`/`editState`/`confirmState` types are implemented in **Task 6** (in `dialogs.go`). To keep Task 5 compiling on its own, add a minimal `dialogs_stub.go` with no-op stubs returning `(r, nil)` / empty, then DELETE the stub at the start of Task 6. (Alternatively, fold Tasks 5+6 into one commit — but the two-stage review prefers them split.) Confirm Go's builtin `max` is available (Go 1.21+); the module is on 1.25.

- [ ] **Step 5: Run — expect PASS**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/shell/ ./internal/tui/screen/worktime/ 2>&1 | tail`

- [ ] **Step 6: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add internal/tui/shell/shell.go internal/tui/shell/event_test.go internal/tui/screen/worktime/route.go internal/tui/screen/worktime/route_test.go internal/tui/screen/worktime/dialogs_stub.go && git commit -m "feat(m3c1): TodayRoute skeleton (Route contract, load, SSE export+reload, cursor, live tick)"
```

---

## Task 6: Actions + dialogs (start/stop+booking, edit, delete)

**Files:**
- Delete: `internal/tui/screen/worktime/dialogs_stub.go`
- Create: `internal/tui/screen/worktime/dialogs.go`
- Test: extend `internal/tui/screen/worktime/route_test.go`

Wire the real mutations through `todayAPI`: `s` start/stop (stop opens a project booking overlay), `E`/`enter` edit (4-field form → `EditSession`), `D` delete (`confirm.Model` → `DeleteSession`). Booking reuses the legacy UX (j/k pick existing project; typing a name creates-and-stops).

- [ ] **Step 1: Write the failing tests** — append to `internal/tui/screen/worktime/route_test.go`

```go
func TestActions_StartWhenIdle(t *testing.T) {
	f := &fakeAPI{}
	r := newTestRoute(f)
	r.loaded = true
	r.st = todayState{Running: false}
	_, cmd := r.handleKey(keyPress("s"))
	if cmd == nil {
		t.Fatal("expected start cmd")
	}
	cmd() // run it
	if !f.started {
		t.Fatal("StartSession not called")
	}
}

func TestActions_StopOpensBookingThenBooks(t *testing.T) {
	f := &fakeAPI{projects: []domain.Project{{ID: "p1", Name: "Flow"}}}
	r := newTestRoute(f)
	r.loaded = true
	r.st = todayState{Running: true, ActiveID: "run"}
	// `s` while running opens booking overlay (and loads projects)
	_, _ = r.handleKey(keyPress("s"))
	if r.dialog != dialogBooking {
		t.Fatalf("dialog = %v, want booking", r.dialog)
	}
	r.booking.projects = f.projects // simulate projectsMsg
	// enter books the selected project
	_, cmd := r.handleKey(keyPressCode(teaKeyEnter()))
	if cmd != nil {
		cmd()
	}
	if r.dialog != dialogNone || f.stopped[1] != "p1" {
		t.Fatalf("stop booking failed: dialog=%v stopped=%v", r.dialog, f.stopped)
	}
}

func TestActions_DeleteConfirmCallsDelete(t *testing.T) {
	f := &fakeAPI{}
	r := newTestRoute(f)
	r.loaded = true
	r.st = todayState{Completed: []completedSession{{ID: "s1"}}}
	r.cursor = 0
	_, _ = r.handleKey(keyPress("D"))
	if r.dialog != dialogDelete {
		t.Fatalf("dialog = %v, want delete", r.dialog)
	}
	// confirm -> DeleteSession("s1")
	_, cmd := r.Update(confirmResult(true))
	if cmd != nil {
		cmd()
	}
	if f.deleted != "s1" {
		t.Fatalf("DeleteSession not called: %q", f.deleted)
	}
}

func TestActions_EditSubmitCallsEdit(t *testing.T) {
	f := &fakeAPI{}
	r := newTestRoute(f)
	r.loaded = true
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	stop := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	r.st = todayState{Completed: []completedSession{{ID: "s1", Start: start, Stop: stop}}}
	r.cursor = 0
	_, _ = r.handleKey(keyPress("E"))
	if r.dialog != dialogEdit {
		t.Fatalf("dialog = %v, want edit", r.dialog)
	}
	cmd := r.submitEdit() // fields prefilled from the session; submit as-is
	if cmd != nil {
		cmd()
	}
	if f.edited != "s1" {
		t.Fatalf("EditSession not called: %q", f.edited)
	}
}
```

> Add these tiny test helpers (in route_test.go) using the rebuild's bubbletea v2 key API verified earlier (`tea.KeyPressMsg{Text}`, `tea.KeyEnter`):
> ```go
> func keyPress(s string) tea.KeyPressMsg        { return tea.KeyPressMsg{Text: s} }
> func keyPressCode(c tea.KeyCode) tea.KeyPressMsg { return tea.KeyPressMsg{Code: c} }
> func teaKeyEnter() tea.KeyCode                 { return tea.KeyEnter }
> func confirmResult(ok bool) tea.Msg            { return confirm.ResultMsg{Confirmed: ok} }
> ```
> (import `tea "charm.land/bubbletea/v2"` and `"…/internal/tui/ui/confirm"` in the test.)

- [ ] **Step 2: Run — expect FAIL** (stub methods don't perform actions)

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/screen/worktime/ -run TestActions 2>&1 | head`

- [ ] **Step 3: Implement** — delete `dialogs_stub.go`, create `internal/tui/screen/worktime/dialogs.go`

```go
package worktime

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/textinput"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/ui/confirm"
	"github.com/serverkraken/flow/internal/tui/ui/form"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
	"github.com/serverkraken/flow/internal/tui/ui/picker"
	"github.com/serverkraken/flow/internal/tui/ui/toast"
)

// --- booking overlay (stop flow) ---

type bookingState struct {
	projects []domain.Project
	sel      int
	newName  string
}

func (r *TodayRoute) startOrStop() (shell.Route, tea.Cmd) {
	if !r.st.Running {
		api := r.api
		return r, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err := api.StartSession(ctx, nil, "", "")
			if err != nil {
				return loadedMsg{err: err}
			}
			return reloadAfterMutation()
		}
	}
	// running -> open booking overlay and load projects
	r.dialog = dialogBooking
	r.booking = bookingState{}
	api := r.api
	return r, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ps, _ := api.ListProjects(ctx)
		return projectsMsg{projects: ps}
	}
}

// reloadAfterMutation is a sentinel asking the route to refetch. We model it as
// a loadedMsg-producing cmd path; simplest is to return a small msg the Update
// loop turns into loadCmd. Here we just trigger a fresh load directly.
func reloadAfterMutation() tea.Msg { return reloadMsg{} }

type reloadMsg struct{}

func (r *TodayRoute) handleDialogKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch r.dialog {
	case dialogBooking:
		return r.handleBookingKey(k)
	case dialogEdit:
		return r.handleEditKey(k)
	case dialogDelete:
		m, cmd := r.confirm.model.Update(k)
		r.confirm.model = m
		return r, cmd
	}
	return r, nil
}

func (r *TodayRoute) handleBookingKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		r.dialog = dialogNone
		return r, nil
	case k.Code == tea.KeyEnter:
		id := r.st.ActiveID
		name := strings.TrimSpace(r.booking.newName)
		r.dialog = dialogNone
		api := r.api
		if name != "" {
			return r, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				p, err := api.CreateProject(ctx, name)
				if err != nil {
					return loadedMsg{err: err}
				}
				if _, err := api.StopSession(ctx, id, p.ID); err != nil {
					return loadedMsg{err: err}
				}
				return reloadMsg{}
			}
		}
		if len(r.booking.projects) == 0 {
			r.dialog = dialogBooking // nothing to book; reopen
			return r, nil
		}
		pid := r.booking.projects[r.booking.sel].ID
		return r, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := api.StopSession(ctx, id, pid); err != nil {
				return loadedMsg{err: err}
			}
			return reloadMsg{}
		}
	case k.Code == tea.KeyBackspace:
		if rn := []rune(r.booking.newName); len(rn) > 0 {
			r.booking.newName = string(rn[:len(rn)-1])
		}
	case k.Text == "j" && r.booking.newName == "":
		if r.booking.sel < len(r.booking.projects)-1 {
			r.booking.sel++
		}
	case k.Text == "k" && r.booking.newName == "":
		if r.booking.sel > 0 {
			r.booking.sel--
		}
	case k.Text != "":
		r.booking.newName += k.Text
	}
	return r, nil
}

// --- edit dialog (4 fields) ---

type editState struct {
	id     string
	date   time.Time
	form   []textinput.Model // [start, stop, tag, note]
	cur    int
}

func (r *TodayRoute) openEdit() (shell.Route, tea.Cmd) {
	if len(r.st.Completed) == 0 {
		return r, nil
	}
	s := r.st.Completed[r.cursor]
	start := form.NewTextInput("HH:MM", r.pal)
	start.SetValue(s.Start.Format("15:04"))
	stop := form.NewTextInput("HH:MM oder +1h30m", r.pal)
	stop.SetValue(s.Stop.Format("15:04"))
	tag := form.NewTextInput("z.B. deep, meeting", r.pal)
	tag.SetValue(s.Tag)
	note := form.NewTextInput("kurzer Text", r.pal)
	note.SetValue(s.Note)
	start.Focus()
	r.edit = editState{id: s.ID, date: s.Start, form: []textinput.Model{start, stop, tag, note}, cur: 0}
	r.dialog = dialogEdit
	return r, textinput.Blink
}

func (r *TodayRoute) handleEditKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		r.dialog = dialogNone
		return r, nil
	case k.Code == tea.KeyTab || k.Code == tea.KeyDown:
		r.editFocus(+1)
		return r, nil
	case k.Code == tea.KeyShiftTab || k.Code == tea.KeyUp:
		r.editFocus(-1)
		return r, nil
	case k.Code == tea.KeyEnter:
		if r.edit.cur == len(r.edit.form)-1 {
			return r, r.submitEdit()
		}
		r.editFocus(+1)
		return r, nil
	}
	var cmd tea.Cmd
	r.edit.form[r.edit.cur], cmd = r.edit.form[r.edit.cur].Update(k)
	return r, cmd
}

func (r *TodayRoute) editFocus(d int) {
	r.edit.form[r.edit.cur].Blur()
	n := len(r.edit.form)
	r.edit.cur = (r.edit.cur + d + n) % n
	r.edit.form[r.edit.cur].Focus()
}

func (r *TodayRoute) submitEdit() tea.Cmd {
	startStr := strings.TrimSpace(r.edit.form[0].Value())
	stopStr := strings.TrimSpace(r.edit.form[1].Value())
	tag := strings.TrimSpace(r.edit.form[2].Value())
	note := strings.TrimSpace(r.edit.form[3].Value())
	startD, err := parseHM(startStr)
	if err != nil {
		r.toast = toast.NewDanger("Start ungültig (HH:MM)", r.pal)
		return r.toast.Init()
	}
	base := time.Date(r.edit.date.Year(), r.edit.date.Month(), r.edit.date.Day(), 0, 0, 0, 0, r.edit.date.Location())
	startTime := base.Add(startD)
	stopTime, err := parseStop(normalizeDurationArg(stopStr), startTime, r.now())
	if err != nil {
		r.toast = toast.NewDanger("Stop ungültig", r.pal)
		return r.toast.Init()
	}
	id := r.edit.id
	api := r.api
	r.dialog = dialogNone
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := api.EditSession(ctx, id, nil, tag, note, startTime, &stopTime); err != nil {
			return loadedMsg{err: err}
		}
		return reloadMsg{}
	}
}

// --- delete confirm ---

type confirmState struct{ model confirm.Model }

func (r *TodayRoute) openDelete() (shell.Route, tea.Cmd) {
	if len(r.st.Completed) == 0 {
		return r, nil
	}
	s := r.st.Completed[r.cursor]
	r.confirm = confirmState{model: confirm.NewDanger(
		"Session löschen?",
		fmt.Sprintf("%s → %s", s.Start.Format("15:04"), s.Stop.Format("15:04")), r.pal)}
	r.dialog = dialogDelete
	return r, nil
}

// --- dialog render + hints ---

func (r *TodayRoute) renderDialog(f shell.Frame) string {
	switch r.dialog {
	case dialogBooking:
		return r.renderBooking(f)
	case dialogEdit:
		return r.renderEdit(f)
	case dialogDelete:
		return r.confirm.model.View()
	}
	return ""
}

func (r *TodayRoute) renderBooking(f shell.Frame) string {
	var b strings.Builder
	b.WriteString("\n  Projekt buchen (j/k wählen · tippen = neu · enter)\n\n")
	if r.booking.newName != "" {
		b.WriteString("  neu: " + r.booking.newName + "\n")
	} else {
		for i, p := range r.booking.projects {
			b.WriteString(picker.Row(i == r.booking.sel, p.Name, "", f.Width-4, f.Pal) + "\n")
		}
	}
	return b.String()
}

func (r *TodayRoute) renderEdit(f shell.Frame) string {
	labels := []string{"Start", "Stop", "Tag", "Note"}
	var b strings.Builder
	b.WriteString("\n  Session bearbeiten (tab wechselt · enter speichert · esc bricht ab)\n\n")
	for i, ti := range r.edit.form {
		b.WriteString(fmt.Sprintf("  %-6s %s\n", labels[i], ti.View()))
	}
	return b.String()
}

func (r *TodayRoute) dialogHints() []keyhint.Hint {
	switch r.dialog {
	case dialogBooking:
		return []keyhint.Hint{{Key: "j/k", Desc: "wählen"}, {Key: "enter", Desc: "buchen"}, {Key: "esc", Desc: "abbrechen"}}
	case dialogEdit:
		return []keyhint.Hint{{Key: "tab", Desc: "Feld"}, {Key: "enter", Desc: "speichern"}, {Key: "esc", Desc: "abbrechen"}}
	case dialogDelete:
		return []keyhint.Hint{{Key: "y", Desc: "löschen"}, {Key: "n", Desc: "abbrechen"}}
	}
	return nil
}
```

- [ ] **Step 3b: Handle `reloadMsg` + `confirm.ResultMsg` in `route.go` `Update`** — add these cases to the `Update` type-switch (before the `tea.KeyPressMsg` case):

```go
	case reloadMsg:
		return r, r.loadCmd()
	case confirm.ResultMsg:
		open := r.dialog == dialogDelete
		r.dialog = dialogNone
		if open && m.Confirmed && r.cursor < len(r.st.Completed) {
			id := r.st.Completed[r.cursor].ID
			api := r.api
			return r, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := api.DeleteSession(ctx, id); err != nil {
					return loadedMsg{err: err}
				}
				return reloadMsg{}
			}
		}
		return r, nil
```
(add `"github.com/serverkraken/flow/internal/tui/ui/confirm"` to `route.go` imports.)

- [ ] **Step 4: Run — expect PASS**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/screen/worktime/ -v 2>&1 | tail -25`
Also full build: `go build ./... && echo OK`.

> If `textinput` import path or `.Update/.View/.Focus/.Blur/.SetValue/.Value` differ in bubbles/v2, open `internal/tui/ui/form/textinput.go` and any existing caller (M3a `uidemo`/`picker`) to copy the exact API; adjust. Confirm `tea.KeyShiftTab`/`tea.KeyTab`/`tea.KeyBackspace` constant names in the rebuild's bubbletea v2 (grep `internal/tui` for usages).

- [ ] **Step 5: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git rm internal/tui/screen/worktime/dialogs_stub.go && git add internal/tui/screen/worktime/dialogs.go internal/tui/screen/worktime/route.go internal/tui/screen/worktime/route_test.go && git commit -m "feat(m3c1): Today actions — start/stop+booking, edit form, delete confirm via apiclient"
```

---

## Task 7: Live mount + done-gate

**Files:**
- Modify: `cmd/flow/ui.go` (wire the Worktime tab)

- [ ] **Step 1: Wire the Worktime tab** — in `cmd/flow/ui.go` `runUI`, replace the `shell.New(...)` line so the shell has Home **and** Worktime tabs:

```go
	m := shell.New(client, os.Getenv("USER"), theme.Load()).
		WithTabs([]shell.Route{
			shell.NewHomeRoute(os.Getenv("USER")),
			worktime.NewTodayRoute(client, time.Now),
		})
```
Add imports: `"time"` and `"github.com/serverkraken/flow/internal/tui/screen/worktime"`. (`*apiclient.Client` satisfies `worktime.todayAPI` structurally — no adapter needed. If Go complains that `todayAPI` is unexported and cannot be named, that's fine: you are passing a concrete `*apiclient.Client`, not naming the interface.)

- [ ] **Step 2: Build + vet + full unit tests**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go build ./... && go vet ./internal/tui/... ./cmd/... && go test ./internal/tui/... 2>&1 | tail -20`
Expected: build OK, vet clean, tests PASS.

- [ ] **Step 3: `make ci`** (coverage gate ≥ 80 %)

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && make ci 2>&1 | tail -15`
Expected: lint + verify-generate + cover (≥ 80 %) + build green. Fix any lint nit minimally and re-run.

- [ ] **Step 4: Live done-gate** (against the dev stack — document the result in the completion note)

```bash
# Terminal A: cd …/flow-rebuild && make dev-up && make dev-run   (or run a fresh server; see [[reference_flow_dev_env]])
# Terminal B: log in if needed, then:
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go run ./cmd/flow ui
```
Verify in the TUI (Tab to the **Worktime** tab):
- Today renders design-faithfully: date line, status pill (`▶ läuft` when running), threshold-colored total, `Ziel N%`, `BarColored`, `Ziel · noch · ETA`, sessions list with `[tag]`, gap `· Pause …` separators, cursor `▎`.
- `s` starts a session; the running row appears and the total ticks live (1s).
- `s` again opens the booking overlay; pick/create a project → session books, list updates.
- `j/k` move the cursor; `E`/`enter` opens the edit dialog (Start/Stop/Tag/Note); submit → row updates.
- `D` opens the delete confirm; confirm → row disappears.
- SSE: trigger a change from another client (e.g. `flow start`/`flow stop` CLI or a second `flow ui`) → the Today tab refreshes without a keypress.

- [ ] **Step 5: Commit any lint fixups** (skip if clean)

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add -A && git commit -m "chore(m3c1): lint fixups + wire Worktime Today tab into flow ui"
```

---

## Self-review

### Spec coverage (M3c1 row of the M3c spec)
| Spec item | Task |
|---|---|
| Datumszeile · Headline (Status-Pille + Total-Threshold + Ziel%) | 4 (`renderDateLine`, `renderHeadline`, `todayStatusBadge`, `totalThresholdColor`) |
| `BarColored` | 4 (`renderProgressBar`) |
| Ziel · noch · **ETA** prominent | 2 (`ETA()`) + 4 (`renderSummary`) |
| Sessions-Liste mit Gap-Trennern + Cursor `▎` | 2 (`GapBefore`) + 4 (`renderSessionsList`, `picker.Row`) |
| Footer max 4 | 5 (`KeyHints()` cap) |
| Interaktivität j/k-Cursor | 5 (`applyKey`/`handleKey`) |
| `s` start, stop+Booking | 6 (`startOrStop`, `handleBookingKey`) |
| Edit-Dialog Start/Stop/Tag/Note via M3c0 | 6 (`openEdit`/`handleEditKey`/`submitEdit` → `EditSession`) |
| Delete via confirm | 6 (`openDelete` + `confirm.ResultMsg` → `DeleteSession`) |
| Kein Pause (Gaps), kein Note-Attach | 2/4 (no PausedAt; gaps only) — confirmed absent |
| Daten rekonstruiert aus `Today`+`ListSessions` | 2 (`reconstruct`) |
| Live via SSE | 5 (`shell.EventMsg` export + `isSessionEvent` reload) |
| Lokale threshold/badge/eta-Helfer | 4 (local copies; M3d promotion noted) |
| Done-Gate: design-treu + start/stop/booking/edit/delete live + SSE + Unit (Render-Golden + Reducer) | 4 (golden) + 5/6 (reducer) + 7 (live) |

### Deviations from the spec text (flagged, not silent)
- **`s`-toggle has no "Resume" branch** (spec drops Pause). `s` = start when idle, stop+booking when running. No `‖ pausiert`/Resume path.
- **No in-route help dialog**: the Shell's `?` overlay reads `KeyHints()` (M3b), so `heuteDialogHelp` is dropped.
- **The toast lives in the body** (host footer is keyhints-only), unlike main where it shared the footer slot. `renderBody` reserves the toast slot via `toast.SlotRows`.
- **`o/O/R/n` note-attach + inline note-viewer dropped** (separate later slice per Locked Decision 1).
- **Reducer tests use a consumer-defined `todayAPI` interface** rather than `*apiclient.Client` directly — cleaner test seam; `*apiclient.Client` satisfies it structurally, mount stays zero-adapter.

### Placeholder scan
Every code step has complete code. The one deliberate seam: Task 5 ships `dialogs_stub.go` (no-op stubs) so the skeleton compiles before Task 6 fills it; Task 6 Step 5 `git rm`s the stub. Three "verify the exact import path / API" notes (lipgloss path, color type, bubbles/v2 textinput + key-constant names) are real-API confirmations the implementer must do against the rebuild source — not missing logic.

### Type consistency
- `todayAPI` methods mirror the verified `apiclient.Client` signatures exactly (incl. `StopSession(ctx, id, projectID string)` — projectID required; `EditSession(ctx, id, *string, tag, note, start, *stop)`).
- `todayState{Completed []completedSession; Running bool; Active *time.Time; ActiveID string; Logged, Target time.Duration}` — defined Task 2, consumed Task 4 (`renderBody`) and Task 5/6 (route).
- `completedSession{ID,Start,Stop,Elapsed,Tag,Note,GapBefore}` — Task 2, read in Task 4 render + Task 6 edit/delete.
- `shell.EventMsg{Ev apiclient.ClientEvent}` — exported Task 5, consumed in the route's `Update`.
- Messages `loadedMsg`/`projectsMsg`/`liveTickMsg`/`reloadMsg` + `confirm.ResultMsg`/`toast.DismissedMsg` — all handled in `Update`.
- `dialogKind{None,Booking,Edit,Delete}` consistent across `route.go`/`dialogs.go`.

### Notes for the executor
- [[feedback_subagent_git_commits_isolated]]: verify HEAD advances after each subagent commit; recover orphans via reflog; do final wiring yourself.
- [[feedback_long_lived_integration_branch]]: commit on `rebuild`; do not merge to main per milestone.
- [[feedback_no_monoliths]]: each route-package file is single-responsibility (format/state/scroll/render/route/dialogs).
- This is the **first external `shell.Route`** — the `shell.EventMsg` export (Task 5) and the zero-adapter `WithTabs` mount (Task 7) are the M3b-contract validations. M3c2 (sibling routes + `w/t/d/e` push) and M3c3 (3-tab shell + deep-link) build on this; their plans are written just-in-time after this lands ([[project_flow_rebuild_m3c]]).
- Parent spec: `docs/superpowers/specs/2026-06-16-flow-rebuild-m3c-home-worktime-design.md`. `main` render reference: `internal/frontend/tui/screen/worktime/today_render.go` (in the `flow` worktree).
```
