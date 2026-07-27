# flow rebuild M3c2 — Worktime Sibling-Routes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add four pushable Worktime sibling-routes (Woche · Stats · Frei · Export) to the flow TUI, reachable by lateral `w/t/d/e` switching from Today and each other, revived onto the design system and driven by `apiclient` + SSE.

**Architecture:** A leaf-free `wtfmt` (minute formatters) and `wtnav` (key→route navigation) package break the import cycle that lateral navigation would otherwise create. Each sibling is its own focused package under `internal/tui/screen/worktime/` implementing `shell.Route`. A new generic `shell.SwitchRouteMsg` pushes when at a tab's root and replaces-top otherwise, so lateral switches never deepen the stack. A nav registry built in the hub injects lazy route factories into every route.

**Tech Stack:** Go, charm.land/bubbletea/v2 + bubbles/v2 + lipgloss/v2, existing `internal/tui/{shell,theme,ui}` design system, `internal/adapter/apiclient`.

**Spec:** `docs/superpowers/specs/2026-06-17-flow-rebuild-m3c2-worktime-siblings-design.md`

---

## File Structure

**New leaf-free shared packages (no cycle):**
- `internal/tui/screen/worktime/wtfmt/wtfmt.go` — `FormatMin(int) string`, `FormatSaldo(int) string`. Imports only `fmt`. Used by leaves (which work on int-minute DTOs).
- `internal/tui/screen/worktime/wtnav/wtnav.go` — `Registry map[string]func() shell.Route` + `Nav(key) tea.Cmd`. Imports only `shell` + bubbletea. Used by every route to switch laterally.

**New generic shell message (in existing package):**
- `internal/tui/shell/route.go` — add `SwitchRouteMsg{Route}`.
- `internal/tui/shell/shell.go` — handle `SwitchRouteMsg` (push at root, else replace-top).

**New sibling route packages (leaves — import `shell`, `ui`, `apiclient`, `wtfmt`, `wtnav`; NOT each other, NOT the hub):**
- `internal/tui/screen/worktime/week/route.go` — `WeekRoute` (read-only pace strip).
- `internal/tui/screen/worktime/statsrange/route.go` — `StatsRangeRoute` (week|month + burndown).
- `internal/tui/screen/worktime/dayoffs/route.go` + `dialogs.go` — `DayOffsRoute` (list + target-edit + add + delete + Bundesland).
- `internal/tui/screen/worktime/export/route.go` + `export_logic.go` — `ExportRoute` (preset/range/format/path form, writes file).

**Hub wiring (existing `worktime` package — imports the four leaves):**
- `internal/tui/screen/worktime/nav.go` — `BuildRegistry(client, pal) wtnav.Registry`.
- `internal/tui/screen/worktime/route.go` — `TodayRoute` gains a `reg wtnav.Registry` field + `w/t/d/e` handling.

**Composition root:**
- `cmd/flow/ui.go` — build registry, inject into the Worktime-tab Today route.

**Dependency direction (acyclic):** `wtfmt` ← leaves → `wtnav` → `shell`; hub → leaves; hub → `wtnav`; `cmd/flow` → hub. No leaf imports the hub or another leaf.

---

## Task 1: `shell.SwitchRouteMsg` (push-at-root-else-replace)

**Files:**
- Modify: `internal/tui/shell/route.go` (add message type near `PushRouteMsg`)
- Modify: `internal/tui/shell/shell.go` (handle in `Update`)
- Test: `internal/tui/shell/shell_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/tui/shell/shell_test.go`:

```go
func TestShell_switchRoute_pushesAtRootThenReplaces(t *testing.T) {
	var rootInit, aInit, bInit int
	root := initCountRoute{stubRoute{title: "Worktime"}, &rootInit}
	routeA := initCountRoute{stubRoute{title: "Woche"}, &aInit}
	routeB := initCountRoute{stubRoute{title: "Stats"}, &bInit}

	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{root})

	// At root (depth 1): SwitchRouteMsg pushes -> depth 2, crumb tip "Woche".
	next, _ := s.Update(shell.SwitchRouteMsg{Route: routeA})
	sh := next.(shell.Shell)
	if sh.ActiveDepth() != 2 {
		t.Fatalf("after switch at root depth = %d, want 2", sh.ActiveDepth())
	}
	if aInit != 1 { // the Shell must Init the pushed route (stub Init returns nil)
		t.Fatalf("switch at root should Init the new route (aInit=%d)", aInit)
	}

	// In a sibling (depth 2): SwitchRouteMsg replaces top -> stays depth 2.
	next2, _ := sh.Update(shell.SwitchRouteMsg{Route: routeB})
	sh2 := next2.(shell.Shell)
	if sh2.ActiveDepth() != 2 {
		t.Fatalf("after lateral switch depth = %d, want 2 (replace, not push)", sh2.ActiveDepth())
	}
	if bInit != 1 {
		t.Fatalf("lateral switch should Init the replacement (bInit=%d)", bInit)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/shell/ -run TestShell_switchRoute_pushesAtRootThenReplaces -v`
Expected: FAIL — `shell.SwitchRouteMsg` undefined (compile error).

- [ ] **Step 3: Add the message type**

In `internal/tui/shell/route.go`, after the `PopRouteMsg` declaration:

```go
// SwitchRouteMsg performs a lateral move: if the active tab's nav-stack is at
// its root (depth 1) it pushes Route (entering the sibling group); otherwise it
// replaces the top Route, so switching between siblings never deepens the
// stack. Emit it as a tea.Cmd from a Route's Update (see wtnav.Registry.Nav).
type SwitchRouteMsg struct{ Route Route }
```

- [ ] **Step 4: Handle it in the Shell**

In `internal/tui/shell/shell.go` `Update`, add a case right after the `PopRouteMsg` case:

```go
	case SwitchRouteMsg:
		ns := s.tabs[s.activeTab]
		if ns.Len() == 1 {
			ns.Push(msg.Route)
		} else {
			ns.ReplaceTop(msg.Route)
		}
		return s, msg.Route.Init()
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/tui/shell/ -v`
Expected: PASS (all shell tests, including the new one).

- [ ] **Step 6: Commit**

```bash
git add internal/tui/shell/route.go internal/tui/shell/shell.go internal/tui/shell/shell_test.go
git commit -m "feat(m3c2): shell.SwitchRouteMsg (push at root, replace-top in siblings)"
```

---

## Task 2: `wtfmt` minute formatters (leaf-free)

**Files:**
- Create: `internal/tui/screen/worktime/wtfmt/wtfmt.go`
- Test: `internal/tui/screen/worktime/wtfmt/wtfmt_test.go`

These mirror the monolith's `fmtMin`/`fmtSaldo` (`internal/tui/worktime.go:452-466`) but live in a sink package both the hub and leaves can import without a cycle.

- [ ] **Step 1: Write the failing test**

```go
package wtfmt_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtfmt"
)

func TestFormatMin(t *testing.T) {
	cases := map[int]string{0: "0h 00m", 5: "0h 05m", 65: "1h 05m", 600: "10h 00m", -30: "0h 00m"}
	for in, want := range cases {
		if got := wtfmt.FormatMin(in); got != want {
			t.Errorf("FormatMin(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatSaldo(t *testing.T) {
	cases := map[int]string{0: "+0h 00m", 65: "+1h 05m", -65: "-1h 05m"}
	for in, want := range cases {
		if got := wtfmt.FormatSaldo(in); got != want {
			t.Errorf("FormatSaldo(%d) = %q, want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/wtfmt/`
Expected: FAIL — package `wtfmt` does not exist (build error).

- [ ] **Step 3: Write the implementation**

`internal/tui/screen/worktime/wtfmt/wtfmt.go`:

```go
// Package wtfmt holds minute-based duration formatters shared by the Worktime
// sibling routes. It imports nothing from the worktime hub or its sibling
// packages, so leaves can use it without forming an import cycle.
package wtfmt

import "fmt"

// FormatMin renders a non-negative minute count as "Xh YYm".
func FormatMin(min int) string {
	if min < 0 {
		min = 0
	}
	return fmt.Sprintf("%dh %02dm", min/60, min%60)
}

// FormatSaldo renders a signed minute count as "+Xh YYm" / "-Xh YYm".
func FormatSaldo(min int) string {
	sign := "+"
	if min < 0 {
		sign = "-"
		min = -min
	}
	return fmt.Sprintf("%s%dh %02dm", sign, min/60, min%60)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/screen/worktime/wtfmt/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen/worktime/wtfmt/
git commit -m "feat(m3c2): wtfmt minute formatters (leaf-free shared package)"
```

---

## Task 3: `wtnav` navigation registry (leaf-free)

**Files:**
- Create: `internal/tui/screen/worktime/wtnav/wtnav.go`
- Test: `internal/tui/screen/worktime/wtnav/wtnav_test.go`

- [ ] **Step 1: Write the failing test**

```go
package wtnav_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

type fakeRoute struct{ title string }

func (f fakeRoute) Title() string                            { return f.title }
func (f fakeRoute) Init() tea.Cmd                            { return nil }
func (f fakeRoute) Update(tea.Msg) (shell.Route, tea.Cmd)    { return f, nil }
func (f fakeRoute) View(shell.Frame) string                 { return f.title }
func (f fakeRoute) KeyHints() []keyhint.Hint                 { return nil }

func TestRegistry_NavEmitsSwitchForKnownKey(t *testing.T) {
	reg := wtnav.Registry{"w": func() shell.Route { return fakeRoute{title: "Woche"} }}
	cmd := reg.Nav("w")
	if cmd == nil {
		t.Fatal("Nav(known key) should return a cmd")
	}
	msg, ok := cmd().(shell.SwitchRouteMsg)
	if !ok {
		t.Fatalf("Nav cmd should emit SwitchRouteMsg, got %T", cmd())
	}
	if msg.Route.Title() != "Woche" {
		t.Fatalf("switch target = %q, want Woche", msg.Route.Title())
	}
}

func TestRegistry_NavNilForUnknownKey(t *testing.T) {
	reg := wtnav.Registry{}
	if reg.Nav("z") != nil {
		t.Fatal("Nav(unknown key) should return nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/wtnav/`
Expected: FAIL — package `wtnav` does not exist.

- [ ] **Step 3: Write the implementation**

`internal/tui/screen/worktime/wtnav/wtnav.go`:

```go
// Package wtnav routes a sibling-navigation key (w/t/d/e) to a SwitchRouteMsg
// carrying a freshly-built target Route. The factory map is built once in the
// worktime hub (which imports the leaf packages) and injected into every route,
// so leaves never import each other. wtnav imports only shell, breaking the
// cycle lateral navigation would otherwise create.
package wtnav

import (
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/shell"
)

// Registry maps a sibling key ("w","t","d","e") to a lazy Route factory.
type Registry map[string]func() shell.Route

// Nav returns a cmd emitting shell.SwitchRouteMsg for key, or nil when key has
// no registered factory (so pressing an unmapped key is a no-op).
func (r Registry) Nav(key string) tea.Cmd {
	factory, ok := r[key]
	if !ok || factory == nil {
		return nil
	}
	return func() tea.Msg { return shell.SwitchRouteMsg{Route: factory()} }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/screen/worktime/wtnav/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen/worktime/wtnav/
git commit -m "feat(m3c2): wtnav registry (key->SwitchRouteMsg, breaks import cycle)"
```

---

## Task 4: WeekRoute (read-only pace strip)

**Files:**
- Create: `internal/tui/screen/worktime/week/route.go`
- Test: `internal/tui/screen/worktime/week/route_test.go`

Ports `weekView` (`internal/tui/stats.go:51-68`): per-day row with marker, date, colored bar, logged/target. Uses design-system `statusbar.BarColored` instead of the plain `bar()`.

- [ ] **Step 1: Write the failing test**

```go
package week_test

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/week"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

type fakeAPI struct{ days []apiclient.WeekDay }

func (f fakeAPI) GetWeek(context.Context, string) ([]apiclient.WeekDay, error) {
	return f.days, nil
}

func drain(r shell.Route, cmd tea.Cmd) shell.Route {
	for i := 0; cmd != nil && i < 20; i++ {
		msg := cmd()
		if msg == nil {
			break
		}
		r, cmd = r.Update(msg)
	}
	return r
}

// stubTitle is a minimal Route used as a nav target in tests.
type stubTitle string

func (s stubTitle) Title() string                         { return string(s) }
func (s stubTitle) Init() tea.Cmd                         { return nil }
func (s stubTitle) Update(tea.Msg) (shell.Route, tea.Cmd) { return s, nil }
func (s stubTitle) View(shell.Frame) string               { return string(s) }
func (s stubTitle) KeyHints() []keyhint.Hint              { return nil }

func TestWeekRoute_rendersDays(t *testing.T) {
	api := fakeAPI{days: []apiclient.WeekDay{
		{Date: "2026-06-15", LoggedMin: 480, TargetMin: 480, Workday: true},
		{Date: "2026-06-16", LoggedMin: 120, TargetMin: 480, IsToday: true, Workday: true},
	}}
	var r shell.Route = week.NewRoute(api, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "2026-06-15") || !strings.Contains(body, "2026-06-16") {
		t.Fatalf("missing day rows:\n%s", body)
	}
	if r.Title() != "Woche" {
		t.Fatalf("title = %q, want Woche", r.Title())
	}
}

func TestWeekRoute_navEmitsSwitch(t *testing.T) {
	reg := wtnav.Registry{"t": func() shell.Route { return stubTitle("Stats") }}
	r := week.NewRoute(fakeAPI{}, theme.Default, reg)
	_, cmd := r.Update(tea.KeyPressMsg{Text: "t"})
	if cmd == nil {
		t.Fatal("pressing t should emit a switch cmd")
	}
	if _, ok := cmd().(shell.SwitchRouteMsg); !ok {
		t.Fatalf("t should emit SwitchRouteMsg, got %T", cmd())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/week/`
Expected: FAIL — package `week` does not exist.

- [ ] **Step 3: Write the implementation**

`internal/tui/screen/worktime/week/route.go`:

```go
// Package week is the Worktime "Woche" sibling route: a read-only pace strip of
// the current week's days with colored progress bars.
package week

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtfmt"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
	"github.com/serverkraken/flow/internal/tui/ui/statusbar"
)

// API is the narrow client surface WeekRoute needs (*apiclient.Client satisfies it).
type API interface {
	GetWeek(ctx context.Context, ref string) ([]apiclient.WeekDay, error)
}

type loadedMsg struct {
	days []apiclient.WeekDay
	err  error
}

// Route renders the current week. It reloads on session.* SSE events.
type Route struct {
	api    API
	pal    theme.Palette
	reg    wtnav.Registry
	days   []apiclient.WeekDay
	loaded bool
	err    error
}

// NewRoute builds the Woche route. reg drives lateral w/t/d/e navigation.
func NewRoute(api API, pal theme.Palette, reg wtnav.Registry) *Route {
	return &Route{api: api, pal: pal, reg: reg}
}

func (r *Route) Title() string { return "Woche" }

func (r *Route) Init() tea.Cmd { return r.loadCmd() }

func (r *Route) loadCmd() tea.Cmd {
	api := r.api
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		days, err := api.GetWeek(ctx, "")
		return loadedMsg{days: days, err: err}
	}
}

func (r *Route) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	switch m := msg.(type) {
	case loadedMsg:
		r.loaded, r.err, r.days = true, m.err, m.days
		return r, nil
	case shell.EventMsg:
		if isSessionEvent(m.Ev.Type) {
			return r, r.loadCmd()
		}
		return r, nil
	case tea.KeyPressMsg:
		if cmd := navKey(r.reg, m); cmd != nil {
			return r, cmd
		}
	}
	return r, nil
}

func (r *Route) View(f shell.Frame) string {
	if !r.loaded {
		return theme.Dim("  Woche lädt …", f.Pal)
	}
	if r.err != nil {
		return theme.Dim("  Fehler: "+r.err.Error(), f.Pal)
	}
	cells := 20
	var b strings.Builder
	b.WriteString("\n")
	for _, d := range r.days {
		marker := "  "
		if d.IsToday {
			marker = theme.Active(string(glyphs.Active), f.Pal) + " "
		}
		pct := 0
		if d.TargetMin > 0 {
			pct = d.LoggedMin * 100 / d.TargetMin
		}
		line := fmt.Sprintf("%s%s  %s  %s / %s",
			marker, d.Date, statusbar.Bar(pct, cells, f.Pal),
			wtfmt.FormatMin(d.LoggedMin), wtfmt.FormatMin(d.TargetMin))
		b.WriteString("  " + line + "\n")
	}
	return b.String()
}

func (r *Route) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "t", Desc: "Stats"},
		{Key: "d", Desc: "Frei"},
		{Key: "e", Desc: "Export"},
		{Key: "esc", Desc: "zurück"},
	}
}

// navKey maps the sibling-switch keys through the registry. Returns nil for any
// other key. Shared shape across all sibling routes.
func navKey(reg wtnav.Registry, k tea.KeyPressMsg) tea.Cmd {
	switch k.Text {
	case "w", "t", "d", "e":
		return reg.Nav(k.Text)
	}
	return nil
}

func isSessionEvent(t string) bool {
	switch domain.EventType(t) {
	case domain.EventSessionStarted, domain.EventSessionStopped,
		domain.EventSessionUpdated, domain.EventSessionDeleted:
		return true
	}
	return false
}
```

> `theme.Active(s, pal)` and `theme.Dim(s, pal)` exist (used by the Today route). `glyphs.Active` is the ▶ marker. `statusbar.Bar(pct, cells, pal)` renders a themed bar.

- [ ] **Step 4: Run test to verify it passes**

Fix the test's `keyhintHint` alias: replace it with a real import of `github.com/serverkraken/flow/internal/tui/ui/keyhint` and use `[]keyhint.Hint` in the `stubTitle.KeyHints` signature. Then:

Run: `go test ./internal/tui/screen/worktime/week/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen/worktime/week/
git commit -m "feat(m3c2): WeekRoute (read-only pace strip, lateral nav, SSE reload)"
```

---

## Task 5: StatsRangeRoute (week|month + burndown)

**Files:**
- Create: `internal/tui/screen/worktime/statsrange/route.go`
- Test: `internal/tui/screen/worktime/statsrange/route_test.go`

Ports `statsView` (`internal/tui/stats.go:70-95`): the stat table + range toggle, plus a burndown line (`GetBurndown`). `m`/`W` switch range and reload.

- [ ] **Step 1: Write the failing test**

```go
package statsrange_test

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/statsrange"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

type fakeAPI struct {
	lastRng string
	stats   apiclient.Stats
	bd      apiclient.Burndown
}

func (f *fakeAPI) GetStats(_ context.Context, rng string) (apiclient.Stats, error) {
	f.lastRng = rng
	return f.stats, nil
}
func (f *fakeAPI) GetBurndown(context.Context) (apiclient.Burndown, error) { return f.bd, nil }

func drain(r shell.Route, cmd tea.Cmd) shell.Route {
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			break
		}
		r, cmd = r.Update(msg)
	}
	return r
}

func TestStatsRoute_rendersTotalsAndDefaultsToWeek(t *testing.T) {
	api := &fakeAPI{stats: apiclient.Stats{TotalMin: 600, AvgMin: 120, Workdays: 5, Streak: 3}}
	var r shell.Route = statsrange.NewRoute(api, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	if api.lastRng != "week" {
		t.Fatalf("default range = %q, want week", api.lastRng)
	}
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "10h 00m") {
		t.Fatalf("missing total:\n%s", body)
	}
}

func TestStatsRoute_mSwitchesToMonth(t *testing.T) {
	api := &fakeAPI{}
	var r shell.Route = statsrange.NewRoute(api, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	_, cmd := r.Update(tea.KeyPressMsg{Text: "m"})
	r = drain(r, cmd)
	if api.lastRng != "month" {
		t.Fatalf("after m, range = %q, want month", api.lastRng)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/statsrange/`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write the implementation**

`internal/tui/screen/worktime/statsrange/route.go`:

```go
// Package statsrange is the Worktime "Stats" sibling route: aggregate stats for
// the current week or month plus a burndown line. No heatmap (deferred to M3d).
package statsrange

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtfmt"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// API is the narrow client surface StatsRangeRoute needs.
type API interface {
	GetStats(ctx context.Context, rng string) (apiclient.Stats, error)
	GetBurndown(ctx context.Context) (apiclient.Burndown, error)
}

type loadedMsg struct {
	rng   string
	stats apiclient.Stats
	bd    apiclient.Burndown
	err   error
}

// Route renders stats for rng ("week"|"month"). m/W toggle the range.
type Route struct {
	api    API
	pal    theme.Palette
	reg    wtnav.Registry
	rng    string
	stats  apiclient.Stats
	bd     apiclient.Burndown
	loaded bool
	err    error
}

// NewRoute builds the Stats route defaulting to the current week.
func NewRoute(api API, pal theme.Palette, reg wtnav.Registry) *Route {
	return &Route{api: api, pal: pal, reg: reg, rng: "week"}
}

func (r *Route) Title() string { return "Stats" }

func (r *Route) Init() tea.Cmd { return r.loadCmd() }

func (r *Route) loadCmd() tea.Cmd {
	api, rng := r.api, r.rng
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		st, err := api.GetStats(ctx, rng)
		if err != nil {
			return loadedMsg{rng: rng, err: err}
		}
		bd, _ := api.GetBurndown(ctx)
		return loadedMsg{rng: rng, stats: st, bd: bd}
	}
}

func (r *Route) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	switch m := msg.(type) {
	case loadedMsg:
		r.loaded, r.err = true, m.err
		if m.err == nil {
			r.rng, r.stats, r.bd = m.rng, m.stats, m.bd
		}
		return r, nil
	case shell.EventMsg:
		if isSessionEvent(m.Ev.Type) {
			return r, r.loadCmd()
		}
		return r, nil
	case tea.KeyPressMsg:
		switch m.Text {
		case "m":
			if r.rng != "month" {
				r.rng = "month"
				return r, r.loadCmd()
			}
			return r, nil
		case "W":
			if r.rng != "week" {
				r.rng = "week"
				return r, r.loadCmd()
			}
			return r, nil
		case "w", "t", "d", "e":
			return r, r.reg.Nav(m.Text)
		}
	}
	return r, nil
}

func (r *Route) View(f shell.Frame) string {
	if !r.loaded {
		return theme.Dim("  Stats lädt …", f.Pal)
	}
	if r.err != nil {
		return theme.Dim("  Fehler: "+r.err.Error(), f.Pal)
	}
	label := "KW"
	if r.rng == "month" {
		label = "Monat"
	}
	s := r.stats
	rows := [][2]string{
		{"Zeitraum", label},
		{"Total", wtfmt.FormatMin(s.TotalMin)},
		{"⌀/Tag", wtfmt.FormatMin(s.AvgMin)},
		{"Max", wtfmt.FormatMin(s.MaxMin)},
		{"Min", wtfmt.FormatMin(s.MinMin)},
		{"Arbeitstage", fmt.Sprintf("%d", s.Workdays)},
		{"Treffer", fmt.Sprintf("%d/%d", s.Hits, s.Workdays)},
		{"Streak", fmt.Sprintf("%d (best %d)", s.Streak, s.BestStreak)},
		{"Saldo", wtfmt.FormatSaldo(s.OvertimeMin)},
	}
	var b strings.Builder
	b.WriteString("\n")
	for _, row := range rows {
		fmt.Fprintf(&b, "  %-12s %s\n", row[0], row[1])
	}
	if r.bd.TargetMin > 0 {
		b.WriteString("\n  " + theme.Dim(fmt.Sprintf("Burndown: %s / %s · %s",
			wtfmt.FormatMin(r.bd.TotalMin), wtfmt.FormatMin(r.bd.TargetMin),
			wtfmt.FormatSaldo(r.bd.SaldoMin)), f.Pal) + "\n")
	}
	return b.String()
}

func (r *Route) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "m/W", Desc: "Monat/KW"},
		{Key: "w", Desc: "Woche"},
		{Key: "d", Desc: "Frei"},
		{Key: "esc", Desc: "zurück"},
	}
}

func isSessionEvent(t string) bool {
	switch domain.EventType(t) {
	case domain.EventSessionStarted, domain.EventSessionStopped,
		domain.EventSessionUpdated, domain.EventSessionDeleted:
		return true
	}
	return false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/screen/worktime/statsrange/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen/worktime/statsrange/
git commit -m "feat(m3c2): StatsRangeRoute (week|month toggle + burndown, lateral nav)"
```

---

## Task 6: DayOffsRoute (list + target-edit + add + delete + Bundesland)

**Files:**
- Create: `internal/tui/screen/worktime/dayoffs/route.go`
- Create: `internal/tui/screen/worktime/dayoffs/dialogs.go`
- Test: `internal/tui/screen/worktime/dayoffs/route_test.go`

Ports `dayOffView`/`reloadDayOffs`/`reloadSettings` (`internal/tui/dayoffs.go`) and **expands** per spec: `g` edit default-target, `a` add dayoff, `D` delete (confirm), `b` Bundesland picker. Dialog pattern mirrors `internal/tui/screen/worktime/dialogs.go`. Reloads on `dayoff.changed` + `settings.changed`.

- [ ] **Step 1: Write the failing test**

```go
package dayoffs_test

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/dayoffs"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

type fakeAPI struct {
	list       []apiclient.DayOff
	settings   apiclient.Settings
	deleted    string
	addedFrom  string
	bundesland string
}

func (f *fakeAPI) ListDayOffs(context.Context, string, string) ([]apiclient.DayOff, error) {
	return f.list, nil
}
func (f *fakeAPI) GetSettings(context.Context) (apiclient.Settings, error) { return f.settings, nil }
func (f *fakeAPI) SetTargetConfig(_ context.Context, def int, _ map[string]int) error {
	f.settings.DefaultTargetMin = def
	return nil
}
func (f *fakeAPI) AddDayOffs(_ context.Context, from, _, _, _ string, _ int, _ bool) error {
	f.addedFrom = from
	return nil
}
func (f *fakeAPI) DeleteDayOff(_ context.Context, day string) error { f.deleted = day; return nil }
func (f *fakeAPI) SetBundesland(_ context.Context, land string) error {
	f.bundesland = land
	return nil
}

func drain(r shell.Route, cmd tea.Cmd) shell.Route {
	for i := 0; cmd != nil && i < 20; i++ {
		msg := cmd()
		if msg == nil {
			break
		}
		r, cmd = r.Update(msg)
	}
	return r
}

func TestDayOffsRoute_listsAndTitle(t *testing.T) {
	api := &fakeAPI{
		list:     []apiclient.DayOff{{Day: "2026-12-25", Kind: "holiday", Label: "Weihnachten", Holiday: true}},
		settings: apiclient.Settings{DefaultTargetMin: 480},
	}
	var r shell.Route = dayoffs.NewRoute(api, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "2026-12-25") || !strings.Contains(body, "Weihnachten") {
		t.Fatalf("missing dayoff row:\n%s", body)
	}
	if r.Title() != "Frei" {
		t.Fatalf("title = %q, want Frei", r.Title())
	}
}

func TestDayOffsRoute_deleteConfirmFlow(t *testing.T) {
	api := &fakeAPI{list: []apiclient.DayOff{{Day: "2026-07-01", Kind: "urlaub", Label: "Urlaub"}}}
	var r shell.Route = dayoffs.NewRoute(api, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	// D opens confirm; y confirms; drain the delete + reload.
	r2, _ := r.Update(tea.KeyPressMsg{Text: "D"})
	r3, cmd := r2.Update(tea.KeyPressMsg{Text: "y"})
	_ = drain(r3, cmd)
	if api.deleted != "2026-07-01" {
		t.Fatalf("deleted = %q, want 2026-07-01", api.deleted)
	}
}

func TestDayOffsRoute_navEmitsSwitch(t *testing.T) {
	reg := wtnav.Registry{"w": func() shell.Route { return nil }}
	r := dayoffs.NewRoute(&fakeAPI{}, theme.Default, reg)
	if _, cmd := r.Update(tea.KeyPressMsg{Text: "w"}); cmd == nil {
		t.Fatal("w should emit a nav cmd")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/dayoffs/`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write `route.go`**

`internal/tui/screen/worktime/dayoffs/route.go`:

```go
// Package dayoffs is the Worktime "Frei" sibling route: day-offs/holidays list
// plus default-target editing, add/delete day-off, and Bundesland selection.
package dayoffs

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtfmt"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/confirm"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// API is the narrow client surface DayOffsRoute needs.
type API interface {
	ListDayOffs(ctx context.Context, from, to string) ([]apiclient.DayOff, error)
	GetSettings(ctx context.Context) (apiclient.Settings, error)
	SetTargetConfig(ctx context.Context, defaultMin int, weekday map[string]int) error
	AddDayOffs(ctx context.Context, from, to, kind, label string, targetMin int, skipWeekends bool) error
	DeleteDayOff(ctx context.Context, day string) error
	SetBundesland(ctx context.Context, land string) error
}

type loadedMsg struct {
	list     []apiclient.DayOff
	settings apiclient.Settings
	err      error
}
type reloadMsg struct{}

// Route renders the day-offs surface. It reloads on dayoff.changed and
// settings.changed SSE events.
type Route struct {
	api      API
	pal      theme.Palette
	reg      wtnav.Registry
	list     []apiclient.DayOff
	settings apiclient.Settings
	cursor   int
	loaded   bool
	err      error

	dialog dialogKind
	dlg    dialogState
}

// NewRoute builds the Frei route.
func NewRoute(api API, pal theme.Palette, reg wtnav.Registry) *Route {
	return &Route{api: api, pal: pal, reg: reg}
}

func (r *Route) Title() string { return "Frei" }

func (r *Route) Init() tea.Cmd { return r.loadCmd() }

func (r *Route) loadCmd() tea.Cmd {
	api := r.api
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		year := time.Now().Year()
		list, err := api.ListDayOffs(ctx, fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-12-31", year))
		if err != nil {
			return loadedMsg{err: err}
		}
		st, err := api.GetSettings(ctx)
		return loadedMsg{list: list, settings: st, err: err}
	}
}

func (r *Route) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	switch m := msg.(type) {
	case loadedMsg:
		r.loaded, r.err = true, m.err
		if m.err == nil {
			r.list, r.settings = m.list, m.settings
			if r.cursor >= len(r.list) {
				r.cursor = max(0, len(r.list)-1)
			}
		}
		return r, nil
	case reloadMsg:
		return r, r.loadCmd()
	case confirm.ResultMsg:
		// Mirror the Today route's delete idiom: confirm emits its decision as
		// a ResultMsg which we handle here, not synchronously inside the dialog.
		open := r.dialog == dialogDelete
		r.dialog = dialogNone
		if open && m.Confirmed && r.cursor < len(r.list) {
			return r, r.deleteCmd(r.list[r.cursor].Day)
		}
		return r, nil
	case shell.EventMsg:
		if isDayoffEvent(m.Ev.Type) {
			return r, r.loadCmd()
		}
		return r, nil
	case tea.KeyPressMsg:
		return r.handleKey(m)
	}
	return r, nil
}

func (r *Route) handleKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	if r.dialog != dialogNone {
		return r.handleDialogKey(k)
	}
	switch k.Text {
	case "j":
		if len(r.list) > 0 {
			r.cursor = (r.cursor + 1) % len(r.list)
		}
	case "k":
		if len(r.list) > 0 {
			r.cursor = (r.cursor + len(r.list) - 1) % len(r.list)
		}
	case "g":
		return r.openTargetEdit()
	case "a":
		return r.openAdd()
	case "D":
		return r.openDelete()
	case "b":
		return r.openBundesland()
	case "w", "t", "d", "e":
		return r, r.reg.Nav(k.Text)
	}
	return r, nil
}

func (r *Route) View(f shell.Frame) string {
	if !r.loaded {
		return theme.Dim("  Frei lädt …", f.Pal)
	}
	if r.err != nil {
		return theme.Dim("  Fehler: "+r.err.Error(), f.Pal)
	}
	if r.dialog != dialogNone {
		return r.renderDialog(f)
	}
	var b strings.Builder
	b.WriteString("\n")
	// Settings summary.
	target := fmt.Sprintf("  Tagesziel: %s", wtfmt.FormatMin(r.settings.DefaultTargetMin))
	if len(r.settings.WeekdayTargetMin) > 0 {
		type kv struct{ k, v string }
		var ov []kv
		for k, v := range r.settings.WeekdayTargetMin {
			ov = append(ov, kv{weekdayShort(k), wtfmt.FormatMin(v)})
		}
		sort.Slice(ov, func(i, j int) bool { return ov[i].k < ov[j].k })
		parts := make([]string, 0, len(ov))
		for _, o := range ov {
			parts = append(parts, o.k+" "+o.v)
		}
		target += theme.Dim("  ("+strings.Join(parts, ", ")+")", f.Pal)
	}
	land := r.settings.Bundesland
	if land == "" {
		land = "—"
	}
	b.WriteString(target + "\n")
	b.WriteString(theme.Dim("  Bundesland: "+land, f.Pal) + "\n\n")
	if len(r.list) == 0 {
		b.WriteString(theme.Dim("  keine Frei-Tage dieses Jahr", f.Pal) + "\n")
	}
	for i, d := range r.list {
		label := d.Label
		if label == "" {
			label = d.Kind
		}
		row := fmt.Sprintf("  %s %s  %s", dayOffGlyph(d.Holiday, f.Pal), d.Day, label)
		if i == r.cursor {
			row = theme.Active(row, f.Pal)
		}
		b.WriteString(row + "\n")
	}
	return b.String()
}

func (r *Route) KeyHints() []keyhint.Hint {
	if r.dialog != dialogNone {
		return r.dialogHints()
	}
	return []keyhint.Hint{
		{Key: "g/a/D", Desc: "Ziel/Add/Del"},
		{Key: "b", Desc: "Bundesland"},
		{Key: "w/t", Desc: "Woche/Stats"},
		{Key: "esc", Desc: "zurück"},
	}
}

// dayOffGlyph mirrors the Dayoff-Glyph-Unification: one ○; holidays dimmed.
func dayOffGlyph(holiday bool, pal theme.Palette) string {
	if holiday {
		return theme.Dim("○", pal)
	}
	return "○"
}

func weekdayShort(key string) string {
	names := map[string]string{"0": "So", "1": "Mo", "2": "Di", "3": "Mi", "4": "Do", "5": "Fr", "6": "Sa"}
	if n, ok := names[key]; ok {
		return n
	}
	return key
}

func isDayoffEvent(t string) bool {
	switch domain.EventType(t) {
	case domain.EventDayOffChanged, domain.EventSettingsChanged:
		return true
	}
	return false
}
```

- [ ] **Step 4: Write `dialogs.go`**

`internal/tui/screen/worktime/dayoffs/dialogs.go`:

```go
package dayoffs

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/ui/confirm"
	"github.com/serverkraken/flow/internal/tui/ui/form"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
	"github.com/serverkraken/flow/internal/tui/ui/picker"
)

type dialogKind int

const (
	dialogNone dialogKind = iota
	dialogTarget
	dialogAdd
	dialogDelete
	dialogBundesland
)

// bundeslaender is the picker list for SetBundesland (German states + "off").
var bundeslaender = []string{
	"BW", "BY", "BE", "BB", "HB", "HH", "HE", "MV",
	"NI", "NW", "RP", "SL", "SN", "ST", "SH", "TH", "",
}

type dialogState struct {
	target  string            // dialogTarget: digit input (minutes)
	addForm []textinput.Model // dialogAdd: [from, to, label]
	addCur  int
	confirm confirm.Model // dialogDelete
	blSel   int           // dialogBundesland: index into bundeslaender
}

func (r *Route) openTargetEdit() (shell.Route, tea.Cmd) {
	r.dialog = dialogTarget
	r.dlg.target = ""
	return r, nil
}

func (r *Route) openAdd() (shell.Route, tea.Cmd) {
	from := form.NewTextInput("YYYY-MM-DD", r.pal)
	to := form.NewTextInput("YYYY-MM-DD (leer = wie von)", r.pal)
	label := form.NewTextInput("z.B. Urlaub", r.pal)
	cmd := from.Focus()
	r.dlg.addForm = []textinput.Model{from, to, label}
	r.dlg.addCur = 0
	r.dialog = dialogAdd
	return r, cmd
}

func (r *Route) openDelete() (shell.Route, tea.Cmd) {
	if r.cursor >= len(r.list) {
		return r, nil
	}
	d := r.list[r.cursor]
	r.dlg.confirm = confirm.NewDanger("Frei-Tag löschen?", d.Day+" "+d.Label, r.pal)
	r.dialog = dialogDelete
	return r, nil
}

func (r *Route) openBundesland() (shell.Route, tea.Cmd) {
	r.dlg.blSel = 0
	for i, b := range bundeslaender {
		if b == r.settings.Bundesland {
			r.dlg.blSel = i
		}
	}
	r.dialog = dialogBundesland
	return r, nil
}

func (r *Route) handleDialogKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch r.dialog {
	case dialogTarget:
		return r.handleTargetKey(k)
	case dialogAdd:
		return r.handleAddKey(k)
	case dialogDelete:
		// Forward the key to the confirm widget; its decision comes back as a
		// confirm.ResultMsg, handled in route.go's Update (mirrors Today).
		m, cmd := r.dlg.confirm.Update(k)
		r.dlg.confirm = m
		return r, cmd
	case dialogBundesland:
		return r.handleBundeslandKey(k)
	}
	return r, nil
}

func (r *Route) handleTargetKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		r.dialog = dialogNone
	case k.Code == tea.KeyEnter:
		mins := parseDigits(r.dlg.target)
		if mins <= 0 {
			return r, nil // stay until valid
		}
		r.dialog = dialogNone
		return r, r.setTargetCmd(mins)
	case k.Code == tea.KeyBackspace:
		if rn := []rune(r.dlg.target); len(rn) > 0 {
			r.dlg.target = string(rn[:len(rn)-1])
		}
	case k.Text != "" && unicode.IsDigit([]rune(k.Text)[0]):
		r.dlg.target += k.Text
	}
	return r, nil
}

func (r *Route) handleAddKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		r.dialog = dialogNone
		return r, nil
	case k.Code == tea.KeyTab || k.Code == tea.KeyDown:
		r.addFocus(+1)
		return r, nil
	case k.Code == tea.KeyUp:
		r.addFocus(-1)
		return r, nil
	case k.Code == tea.KeyEnter:
		if r.dlg.addCur == len(r.dlg.addForm)-1 {
			return r, r.submitAdd()
		}
		r.addFocus(+1)
		return r, nil
	}
	var cmd tea.Cmd
	r.dlg.addForm[r.dlg.addCur], cmd = r.dlg.addForm[r.dlg.addCur].Update(k)
	return r, cmd
}

func (r *Route) addFocus(d int) {
	r.dlg.addForm[r.dlg.addCur].Blur()
	n := len(r.dlg.addForm)
	r.dlg.addCur = (r.dlg.addCur + d + n) % n
	_ = r.dlg.addForm[r.dlg.addCur].Focus()
}

func (r *Route) submitAdd() tea.Cmd {
	from := strings.TrimSpace(r.dlg.addForm[0].Value())
	to := strings.TrimSpace(r.dlg.addForm[1].Value())
	label := strings.TrimSpace(r.dlg.addForm[2].Value())
	if from == "" {
		return nil // require a start date; stay in dialog
	}
	if to == "" {
		to = from
	}
	api := r.api
	r.dialog = dialogNone
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := api.AddDayOffs(ctx, from, to, "urlaub", label, 0, true); err != nil {
			return loadedMsg{err: err}
		}
		return reloadMsg{}
	}
}

func (r *Route) handleBundeslandKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		r.dialog = dialogNone
	case k.Text == "j" || k.Code == tea.KeyDown:
		if r.dlg.blSel < len(bundeslaender)-1 {
			r.dlg.blSel++
		}
	case k.Text == "k" || k.Code == tea.KeyUp:
		if r.dlg.blSel > 0 {
			r.dlg.blSel--
		}
	case k.Code == tea.KeyEnter:
		land := bundeslaender[r.dlg.blSel]
		r.dialog = dialogNone
		return r, r.setBundeslandCmd(land)
	}
	return r, nil
}

func (r *Route) setTargetCmd(defaultMin int) tea.Cmd {
	api := r.api
	weekday := r.settings.WeekdayTargetMin
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := api.SetTargetConfig(ctx, defaultMin, weekday); err != nil {
			return loadedMsg{err: err}
		}
		return reloadMsg{}
	}
}

func (r *Route) deleteCmd(day string) tea.Cmd {
	api := r.api
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := api.DeleteDayOff(ctx, day); err != nil {
			return loadedMsg{err: err}
		}
		return reloadMsg{}
	}
}

func (r *Route) setBundeslandCmd(land string) tea.Cmd {
	api := r.api
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := api.SetBundesland(ctx, land); err != nil {
			return loadedMsg{err: err}
		}
		return reloadMsg{}
	}
}

func (r *Route) renderDialog(f shell.Frame) string {
	switch r.dialog {
	case dialogTarget:
		return "\n  Neues Tagesziel (Minuten): " + r.dlg.target + "▏\n  Ziffern · enter ok · esc ab\n"
	case dialogAdd:
		labels := []string{"Von", "Bis", "Label"}
		var b strings.Builder
		b.WriteString("\n  Frei-Tag anlegen (tab wechselt · enter speichert · esc ab)\n\n")
		for i, ti := range r.dlg.addForm {
			fmt.Fprintf(&b, "  %-6s %s\n", labels[i], ti.View())
		}
		return b.String()
	case dialogDelete:
		return r.dlg.confirm.View()
	case dialogBundesland:
		var b strings.Builder
		b.WriteString("\n  Bundesland wählen (j/k · enter · esc)\n\n")
		for i, land := range bundeslaender {
			label := land
			if label == "" {
				label = "(aus)"
			}
			b.WriteString(picker.Row(i == r.dlg.blSel, label, "", f.Width-4, f.Pal) + "\n")
		}
		return b.String()
	}
	return ""
}

func (r *Route) dialogHints() []keyhint.Hint {
	switch r.dialog {
	case dialogTarget:
		return []keyhint.Hint{{Key: "enter", Desc: "ok"}, {Key: "esc", Desc: "abbrechen"}}
	case dialogAdd:
		return []keyhint.Hint{{Key: "tab", Desc: "Feld"}, {Key: "enter", Desc: "speichern"}, {Key: "esc", Desc: "abbrechen"}}
	case dialogDelete:
		return []keyhint.Hint{{Key: "y", Desc: "löschen"}, {Key: "n", Desc: "abbrechen"}}
	case dialogBundesland:
		return []keyhint.Hint{{Key: "j/k", Desc: "wählen"}, {Key: "enter", Desc: "setzen"}, {Key: "esc", Desc: "abbrechen"}}
	}
	return nil
}

func parseDigits(s string) int {
	s = strings.TrimSpace(s)
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
```

> **Reference implementation:** This delete flow mirrors the Today route exactly — `dialogs.go` forwards the key to `confirm.Model.Update` and returns its cmd; `route.go`'s `Update` has a `case confirm.ResultMsg` that closes the dialog and issues `deleteCmd` when `Confirmed`. See Today's `internal/tui/screen/worktime/route.go:130-145` + `dialogs.go` `dialogDelete`/`handleDialogKey`. Match it; don't invent a synchronous variant.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/tui/screen/worktime/dayoffs/ -v`
Expected: PASS (list/title, delete-confirm, nav).

- [ ] **Step 6: Commit**

```bash
git add internal/tui/screen/worktime/dayoffs/
git commit -m "feat(m3c2): DayOffsRoute (list + target/add/delete/Bundesland dialogs)"
```

---

## Task 7: ExportRoute (preset/range/format/path form)

**Files:**
- Create: `internal/tui/screen/worktime/export/export_logic.go` (pure helpers — port of `internal/tui/export.go` non-Model funcs)
- Create: `internal/tui/screen/worktime/export/route.go`
- Test: `internal/tui/screen/worktime/export/route_test.go`

- [ ] **Step 1: Write the failing test**

```go
package export_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/export"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

type fakeAPI struct{ payload []byte }

func (f fakeAPI) Export(_ context.Context, _, _, _, _ string) ([]byte, error) {
	return f.payload, nil
}

func fixedNow() time.Time { return time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC) }

func TestExportRoute_rendersFormAndDefaults(t *testing.T) {
	r := export.NewRoute(fakeAPI{}, fixedNow, theme.Default, wtnav.Registry{})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Range") || !strings.Contains(body, "Format") {
		t.Fatalf("missing form fields:\n%s", body)
	}
	if r.Title() != "Export" {
		t.Fatalf("title = %q, want Export", r.Title())
	}
}

func TestExportRoute_writesFileOnEnter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.md")
	r := export.NewRoute(fakeAPI{payload: []byte("# hi")}, fixedNow, theme.Default, wtnav.Registry{})
	// Move focus to the path field (index 4) and overwrite it.
	r = export.WithPathForTest(r, path) // test helper sets expPath + marks edited
	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			break
		}
		r2, c := r.Update(msg)
		r, cmd = r2.(*export.Route), c
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "# hi" {
		t.Fatalf("export not written: err=%v content=%q", err, string(b))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/export/`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write `export_logic.go`**

Copy the pure helpers from `internal/tui/export.go` verbatim into the new package, exported where the route/test needs them. Include: `dayFmt` const, `presetRange`, `defaultPath`, `expandHome`, `cycleFormat`, `cyclePreset`, `cycle`.

`internal/tui/screen/worktime/export/export_logic.go`:

```go
package export

import (
	"os"
	"path/filepath"
	"time"
)

const dayFmt = "2006-01-02"

// presetRange maps a preset + now to an inclusive [from,to] yyyy-mm-dd range.
func presetRange(preset string, now time.Time) (string, string) {
	y, mo, d := now.Date()
	loc := now.Location()
	today := time.Date(y, mo, d, 0, 0, 0, 0, loc)
	switch preset {
	case "monat":
		from := time.Date(y, mo, 1, 0, 0, 0, 0, loc)
		return from.Format(dayFmt), today.Format(dayFmt)
	case "kw":
		off := (int(today.Weekday()) + 6) % 7
		return today.AddDate(0, 0, -off).Format(dayFmt), today.Format(dayFmt)
	case "letzter":
		firstThis := time.Date(y, mo, 1, 0, 0, 0, 0, loc)
		lastPrev := firstThis.AddDate(0, 0, -1)
		firstPrev := time.Date(lastPrev.Year(), lastPrev.Month(), 1, 0, 0, 0, 0, loc)
		return firstPrev.Format(dayFmt), lastPrev.Format(dayFmt)
	default:
		return today.Format(dayFmt), today.Format(dayFmt)
	}
}

func defaultPath(from, to, format string) string {
	return "~/Downloads/flow-export-" + from + "_" + to + "." + format
}

func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if len(path) >= 2 && path[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func cycleFormat(f string, dir int) string { return cycle([]string{"csv", "json", "md"}, f, dir) }
func cyclePreset(p string, dir int) string {
	return cycle([]string{"kw", "monat", "letzter", "custom"}, p, dir)
}

func cycle(order []string, cur string, dir int) string {
	idx := 0
	for i, v := range order {
		if v == cur {
			idx = i
			break
		}
	}
	return order[(idx+dir+len(order))%len(order)]
}
```

- [ ] **Step 4: Write `route.go`**

`internal/tui/screen/worktime/export/route.go`:

```go
// Package export is the Worktime "Export" sibling route: choose a preset/range
// + format + path and write the server export to disk.
package export

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// API is the narrow client surface ExportRoute needs.
type API interface {
	Export(ctx context.Context, from, to, format, projectID string) ([]byte, error)
}

type doneMsg struct{ path string }
type errMsg struct{ err error }

// Route is the export form. now is injected for deterministic tests.
type Route struct {
	api  API
	now  func() time.Time
	pal  theme.Palette
	reg  wtnav.Registry

	preset     string
	from, to   string
	format     string
	path       string
	pathEdited bool
	focus      int // 0=Range 1=von 2=bis 3=Format 4=Pfad
	status     string
}

// NewRoute builds the Export route with sensible defaults (current month, md).
func NewRoute(api API, now func() time.Time, pal theme.Palette, reg wtnav.Registry) *Route {
	if now == nil {
		now = time.Now
	}
	from, to := presetRange("monat", now())
	return &Route{
		api: api, now: now, pal: pal, reg: reg,
		preset: "monat", format: "md", from: from, to: to,
		path: defaultPath(from, to, "md"),
	}
}

func (r *Route) Title() string { return "Export" }
func (r *Route) Init() tea.Cmd { return nil }

func (r *Route) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	switch m := msg.(type) {
	case doneMsg:
		r.status = "✓ geschrieben: " + m.path
		return r, nil
	case errMsg:
		r.status = "Fehler: " + m.err.Error()
		return r, nil
	case tea.KeyPressMsg:
		return r.handleKey(m)
	}
	return r, nil
}

func (r *Route) handleKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch {
	case k.Text == "w" || k.Text == "t" || k.Text == "d" || k.Text == "e":
		// Sibling nav only on choice fields would clash with text entry; the
		// export form has no single-letter actions except on focus, so route
		// these letters to navigation only when NOT editing a text field.
		if r.focus == 0 || r.focus == 3 {
			return r, r.reg.Nav(k.Text)
		}
	}
	switch {
	case k.Code == tea.KeyTab && k.Mod == tea.ModShift:
		r.focus = (r.focus + 4) % 5
	case k.Code == tea.KeyTab:
		r.focus = (r.focus + 1) % 5
	case k.Code == tea.KeyEnter:
		return r, r.submit()
	case k.Code == tea.KeyLeft, k.Code == tea.KeyRight:
		dir := 1
		if k.Code == tea.KeyLeft {
			dir = -1
		}
		r.cycleField(dir)
	case k.Code == tea.KeyBackspace:
		r.editField(func(s string) string {
			if rn := []rune(s); len(rn) > 0 {
				return string(rn[:len(rn)-1])
			}
			return s
		})
	case k.Text != "":
		t := k.Text
		r.editField(func(s string) string { return s + t })
	}
	return r, nil
}

func (r *Route) cycleField(dir int) {
	switch r.focus {
	case 0:
		r.preset = cyclePreset(r.preset, dir)
		if r.preset != "custom" {
			r.from, r.to = presetRange(r.preset, r.now())
		}
		r.refreshPath()
	case 3:
		r.format = cycleFormat(r.format, dir)
		r.refreshPath()
	}
}

func (r *Route) editField(fn func(string) string) {
	switch r.focus {
	case 1:
		r.from = fn(r.from)
		r.preset = "custom"
		r.refreshPath()
	case 2:
		r.to = fn(r.to)
		r.preset = "custom"
		r.refreshPath()
	case 4:
		r.path = fn(r.path)
		r.pathEdited = true
	}
}

func (r *Route) refreshPath() {
	if !r.pathEdited {
		r.path = defaultPath(r.from, r.to, r.format)
	}
}

func (r *Route) submit() tea.Cmd {
	from, errF := time.Parse(dayFmt, r.from)
	to, errT := time.Parse(dayFmt, r.to)
	if errF != nil || errT != nil {
		r.status = "Ungültiges Datum (yyyy-mm-dd)"
		return nil
	}
	if to.Before(from) {
		r.status = "bis muss >= von sein"
		return nil
	}
	r.status = "exportiere…"
	api, fromS, toS, format, path := r.api, r.from, r.to, r.format, expandHome(r.path)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		b, err := api.Export(ctx, fromS, toS, format, "")
		if err != nil {
			return errMsg{err}
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return errMsg{err}
		}
		return doneMsg{path: path}
	}
}

func (r *Route) View(f shell.Frame) string {
	var b strings.Builder
	b.WriteString("\n")
	field := func(idx int, label, val string) {
		cur := "  "
		if r.focus == idx {
			cur = theme.Active("▸", f.Pal) + " "
			val = theme.Active(val, f.Pal)
		}
		fmt.Fprintf(&b, "%s%-8s %s\n", cur, label, val)
	}
	field(0, "Range", r.preset)
	field(1, "von", r.from)
	field(2, "bis", r.to)
	field(3, "Format", r.format)
	field(4, "Pfad", r.path)
	if r.status != "" {
		b.WriteString("\n  " + theme.Dim(r.status, f.Pal) + "\n")
	}
	return b.String()
}

func (r *Route) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "tab", Desc: "Feld"},
		{Key: "←/→", Desc: "wählen"},
		{Key: "enter", Desc: "export"},
		{Key: "esc", Desc: "zurück"},
	}
}

// WithPathForTest sets the path field directly (test seam for file-write tests).
func WithPathForTest(r *Route, path string) *Route {
	r.path = path
	r.pathEdited = true
	r.focus = 4
	return r
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/tui/screen/worktime/export/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/screen/worktime/export/
git commit -m "feat(m3c2): ExportRoute (preset/range/format/path form, writes file)"
```

---

## Task 8: Nav registry in the hub + Today gains `w/t/d/e`

**Files:**
- Create: `internal/tui/screen/worktime/nav.go`
- Modify: `internal/tui/screen/worktime/route.go` (add `reg` field + constructor param + `w/t/d/e` keys)
- Test: `internal/tui/screen/worktime/route_test.go` (Today nav emit) + `internal/tui/screen/worktime/nav_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/tui/screen/worktime/nav_test.go`:

```go
package worktime_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/tui/screen/worktime"
	"github.com/serverkraken/flow/internal/tui/theme"
)

func TestBuildRegistry_hasAllSiblingKeys(t *testing.T) {
	reg := worktime.BuildRegistry(nil, theme.Default)
	for _, k := range []string{"w", "t", "d", "e"} {
		if reg[k] == nil {
			t.Fatalf("registry missing key %q", k)
		}
		if reg[k]() == nil {
			t.Fatalf("factory %q returned nil route", k)
		}
	}
}
```

The existing `internal/tui/screen/worktime/route_test.go` is **white-box** (`package worktime`) and funnels every route construction through one helper, `newTestRoute` (line 52), which currently calls the 3-arg `NewTodayRoute`. Update that single helper to pass a registry — this fixes every existing call site at once:

```go
func newTestRoute(f *fakeAPI) *TodayRoute {
	return NewTodayRoute(f, fixedNow, theme.Load(), BuildRegistry(nil, theme.Load()))
}
```

Then add a white-box nav-emit test (reuses existing `keyPress` + `fakeAPI` helpers, no package qualifier):

```go
func TestTodayRoute_wKeyEmitsSwitch(t *testing.T) {
	r := newTestRoute(&fakeAPI{})
	_, cmd := r.Update(keyPress("w"))
	if cmd == nil {
		t.Fatal("Today: w should emit a switch cmd")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/screen/worktime/`
Expected: FAIL — `BuildRegistry` undefined and/or `NewTodayRoute` arity mismatch.

- [ ] **Step 3: Write `nav.go` and update Today**

`internal/tui/screen/worktime/nav.go`:

```go
package worktime

import (
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/dayoffs"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/export"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/statsrange"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/week"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// BuildRegistry wires the four Worktime sibling factories. The closures capture
// reg by reference so every sibling (and Today) navigates through the same
// registry — built before any closure runs, so the self-reference is safe.
// client may be nil in tests; the leaf routes only call it on Init/actions.
func BuildRegistry(client *apiclient.Client, pal theme.Palette) wtnav.Registry {
	var reg wtnav.Registry
	reg = wtnav.Registry{
		"w": func() shell.Route { return week.NewRoute(client, pal, reg) },
		"t": func() shell.Route { return statsrange.NewRoute(client, pal, reg) },
		"d": func() shell.Route { return dayoffs.NewRoute(client, pal, reg) },
		"e": func() shell.Route { return export.NewRoute(client, nil, pal, reg) },
	}
	return reg
}
```

> `client` is `*apiclient.Client`; it satisfies each leaf's narrow `API` interface. Passing a typed-nil `*apiclient.Client` in tests is fine because the factories are not invoked against the network in `TestBuildRegistry` (only constructed). `export.NewRoute`'s `now` is nil → defaults to `time.Now`.

In `internal/tui/screen/worktime/route.go`, add the field and constructor param:

```go
type TodayRoute struct {
	api todayAPI
	now func() time.Time
	pal theme.Palette
	reg wtnav.Registry   // NEW: lateral sibling navigation
	// ... existing fields unchanged ...
}

func NewTodayRoute(api todayAPI, now func() time.Time, pal theme.Palette, reg wtnav.Registry) *TodayRoute {
	if now == nil {
		now = time.Now
	}
	return &TodayRoute{api: api, now: now, pal: pal, reg: reg}
}
```

Add the import `"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"` to `route.go`. In `handleKey` (the non-dialog switch), add before the final `return`:

```go
	case k.Text == "w" || k.Text == "t" || k.Text == "d" || k.Text == "e":
		return r, r.reg.Nav(k.Text)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/screen/worktime/...`
Expected: PASS (hub + all four leaf packages).

- [ ] **Step 5: Run the full build to confirm no import cycle**

Run: `go build ./...`
Expected: clean. If Go reports an import cycle, move `wtfmt`/`wtnav` usage so no leaf imports the `worktime` hub (they must not) — the cycle-free direction is documented in the File Structure section.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/screen/worktime/nav.go internal/tui/screen/worktime/route.go internal/tui/screen/worktime/route_test.go internal/tui/screen/worktime/nav_test.go
git commit -m "feat(m3c2): worktime nav registry + Today w/t/d/e lateral switch"
```

---

## Task 9: Wire the registry into `flow ui`

**Files:**
- Modify: `cmd/flow/ui.go`

- [ ] **Step 1: Update the composition root**

In `cmd/flow/ui.go` `runUI`, build the registry once and inject it into the Worktime-tab Today route:

```go
	pal := theme.Load()
	reg := worktime.BuildRegistry(client, pal)
	m := shell.New(client, os.Getenv("USER"), pal).
		WithTabs([]shell.Route{
			shell.NewHomeRoute(os.Getenv("USER")),
			worktime.NewTodayRoute(client, time.Now, pal, reg),
		})
```

> Reuse one `pal` value (don't call `theme.Load()` twice). The registry's leaf routes share the same `client` and `pal`.

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: clean.

- [ ] **Step 3: Manual smoke (no live server needed)**

Run: `go vet ./cmd/flow/...`
Expected: clean. (Live behavior is the Task 10 done-gate.)

- [ ] **Step 4: Commit**

```bash
git add cmd/flow/ui.go
git commit -m "feat(m3c2): wire worktime sibling registry into flow ui"
```

---

## Task 10: Done-gate — full CI + live verification

**Files:** none (verification only).

- [ ] **Step 1: Run the full CI gate**

Run: `make ci`
Expected: lint + templ + build + tests green; coverage ≥ 80%.
If coverage dips below 80%, add a render-golden or reducer test to the thinnest sibling package (week/statsrange are cheapest) until the gate passes — do not lower the threshold.

- [ ] **Step 2: Live done-gate against the dev stack**

Start the dev stack ([[reference_flow_dev_env]]): `make dev-up && make dev-run` (separate shells), ensure a logged-in token (`make dev-token` / `flow login`). Then run `flow ui` and verify:

- [ ] Worktime tab → `w` pushes **Woche** (breadcrumb `Worktime › Woche`, depth 2).
- [ ] From Woche → `t` switches to **Stats** without deepening (breadcrumb still depth 2 `Worktime › Stats`).
- [ ] Stats → `m`/`W` toggles Monat/KW and the numbers change.
- [ ] `d` → **Frei**: `g` edits the default target (value updates live via SSE `settings.changed`); `a` adds a day-off (appears live via `dayoff.changed`); `D` deletes with confirm; `b` sets a Bundesland.
- [ ] `e` → **Export**: pick a range/format, `enter` writes a file (check the printed path).
- [ ] `esc` from any sibling returns to **Today** (depth 1).
- [ ] Start/stop a session elsewhere → Woche/Stats reload live.

- [ ] **Step 3: Final commit (if any coverage tests were added)**

```bash
git add -A
git commit -m "test(m3c2): coverage top-up for sibling routes"
```

---

## Notes / known cosmetics (out of scope, do not fix here)

- **Tabstrip label follows the drilled route:** the Shell renders each tab's *top* route title, so while in a sibling the Worktime tab label reads "Woche"/"Stats"/etc. The breadcrumb already shows the path; changing the tabstrip to always show the stack-root title is a separate Shell tweak (flag to Soenne, not in M3c2).
- **Standalone `flow worktime`** still uses the monolith until M3c4; `SwitchRouteMsg` is honored only by the Shell here. M3c4's `NavHost` must implement the same push-at-root/replace semantics.
- **No heatmap** in statsrange (M3d).
