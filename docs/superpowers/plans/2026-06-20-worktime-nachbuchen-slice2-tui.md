# Worktime Nachbuchen — Slice 2 (TUI) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Worktime **Woche** route navigable (week ←/→ + day cursor) and add a pushed **Tag-Detail** route that lists a past day's sessions and supports Nachbuchen (add), edit, and delete — driven by the Slice-1 `AddSession`/`ListSessionsRange` apiclient methods.

**Architecture:** The Woche route (`internal/tui/screen/worktime/week`) gains a `listnav.Cursor` over its day rows and a `weekOffset` that recomputes the `GetWeek(ref)` anchor (forward-clamped to the current week). Pressing `enter` on a day pushes a new leaf route `internal/tui/screen/worktime/daydetail` via `shell.PushRouteMsg`. `daydetail` lists the day's sessions via `ListSessionsRange` (client-side reconstruction like the Today route) and hosts three dialogs — Nachbuchen (project `fuzzylist` + Von/Bis `HH:MM` fields → `AddSession`), Edit (HH:MM fields → `EditSession`), Delete (`confirm` → `DeleteSession`). To keep `daydetail` free of an import cycle (`worktime → week → daydetail`), the shared time parsers move from the `worktime` package into the existing acyclic `wtfmt` sink.

**Tech Stack:** Go, bubbletea v2 (`charm.land/bubbletea/v2`), bubbles v2 (`charm.land/bubbles/v2/textinput`), the in-repo `internal/tui/shell` (Route/NavStack), `internal/tui/ui/{listnav,fuzzylist,form,confirm,toast,grammar,keyhint}`, `internal/tui/screen/worktime/{wtnav,wtfmt}`, `internal/adapter/apiclient`, TDD.

## Global Constraints

- `make ci` must stay green: golangci-lint `0 issues`, `go tool templ generate` drift check (no templ files touched here), build, all tests, coverage gate ~80% (currently 84.0%). Run `make ci`, not just `go test` — lint (e.g. QF1002/staticcheck) is part of the gate.
- Keine Monolithen: one responsibility per file. `daydetail` route logic, its dialogs, and its API interface live in separate files under the new package.
- Hexagonal/acyclic TUI layering: `daydetail` and `week` may import `shell`, `ui/*`, `wtnav`, `wtfmt`, `apiclient`, `theme`, `domain` — but **must NOT import the `worktime` package** (that would cycle: `worktime`→`week`→`daydetail`/`week`→`worktime`). Verify with `go build ./...` after each task.
- Keyboard grammar is single-source in `internal/tui/ui/grammar`: new advertised keys get a `grammar.Binding`; never hard-code a hint string that the binding doesn't back. `j/k/g/G` are NOT list-nav (clamped `listnav.Cursor` only, driven by `↑/↓/Home/End/PgUp/PgDn`).
- Keys already claimed in the Woche route: `←` `→` (sub-tab lateral via `wtnav.Lateral`), `w` `t` `d` `e` (sub-tab jump via `wtnav` + `e` Export drill), `esc`, `q`. **New Woche keys:** `↑`/`↓` (day cursor, `listnav`), `[`/`]` (prev/next week), `enter` (open day). Inside `daydetail` (a pushed child, NOT using `wtnav.Lateral`) the keys `n` (nachbuchen), `e` (edit), `d` (delete), `↑`/`↓` (session cursor), `esc` (back) are local and do not clash.
- Interval/validation semantics are enforced server-side (Slice 1): Bis>Von, no-future, same-day, no-overlap. The TUI surfaces server errors (overlap→HTTP 409, validation→400) as a `toast`, never re-implements the rules.
- Time entry format matches the Today edit dialog: `HH:MM` for Von; `HH:MM` **or** `+1h30m` for Bis; parsed by the shared `wtfmt.ParseHM`/`wtfmt.ParseStop` (Task 1). The context **date** comes from the route, never typed.
- SSE live-reload: a route reloads on `shell.EventMsg` whose `Ev.Type` is a `session.*` event, re-firing its `loadCmd`. Reuse the exact pattern from `worktime/route.go`.

## File Structure

- `internal/tui/screen/worktime/wtfmt/timeparse.go` — **new**: exported `ParseHM`, `ParseStop`, `NormalizeDurationArg` (moved from `worktime/format.go`) (Task 1)
- `internal/tui/screen/worktime/format.go` — **modify**: delete the moved helpers, repoint callers to `wtfmt.*` (Task 1)
- `internal/tui/ui/grammar/grammar.go` — **modify**: add `WeekPrev` (`[`), `WeekNext` (`]`) bindings (Task 3)
- `internal/tui/screen/worktime/week/route.go` — **modify**: `listnav.Cursor` day selection + render highlight (Task 2); `weekOffset` + `loadCmd(ref)` + forward clamp + header range (Task 3); `enter`→`PushRouteMsg` (Task 5)
- `internal/tui/screen/worktime/daydetail/route.go` — **new**: the Tag-Detail `shell.Route` (Task 4)
- `internal/tui/screen/worktime/daydetail/api.go` — **new**: the `API` interface for daydetail (Task 4)
- `internal/tui/screen/worktime/daydetail/state.go` — **new**: client-side day reconstruction (`[]domain.WorkSession` → rows) (Task 4)
- `internal/tui/screen/worktime/daydetail/dialogs.go` — **new**: Nachbuchen (Task 6) + Edit/Delete (Task 7) dialog state
- Tests beside each: `week/route_test.go` (append), `daydetail/route_test.go`, `daydetail/state_test.go`, `wtfmt/timeparse_test.go`.

Each route test uses a hand-rolled fake API struct (the established pattern — see `worktime/route_test.go` and `week/route_test.go`), not a generated mock.

---

## Task 1: Extract time parsers to `wtfmt` (enables `daydetail` without an import cycle)

**Files:**
- Create: `internal/tui/screen/worktime/wtfmt/timeparse.go`
- Create: `internal/tui/screen/worktime/wtfmt/timeparse_test.go`
- Modify: `internal/tui/screen/worktime/format.go` (remove moved funcs, add `wtfmt` import, repoint callers)
- Modify: any other `worktime`-package callers of `parseHM`/`parseStop`/`normalizeDurationArg` (find them first)

**Interfaces:**
- Consumes: nothing new.
- Produces: `wtfmt.ParseHM(s string) (time.Duration, error)`, `wtfmt.ParseStop(arg string, start, now time.Time) (time.Time, error)`, `wtfmt.NormalizeDurationArg(s string) string`.

- [ ] **Step 1: Read the current helpers and find every caller**

Run: `rg -n "parseHM|parseStop|normalizeDurationArg" internal/tui/screen/worktime/`
Read `internal/tui/screen/worktime/format.go` around line 48 to copy the three function bodies **verbatim**. Confirm `internal/tui/screen/worktime/wtfmt/` already exists and is an acyclic sink (it must not import the `worktime` package): `rg -n "import" internal/tui/screen/worktime/wtfmt/*.go`.

- [ ] **Step 2: Write the failing test**

Create `internal/tui/screen/worktime/wtfmt/timeparse_test.go`. Mirror the existing behavior exactly (copy any existing `format_test.go` cases for these funcs if present; otherwise assert the contract):

```go
package wtfmt_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtfmt"
)

func TestParseHM(t *testing.T) {
	got, err := wtfmt.ParseHM("09:30")
	if err != nil {
		t.Fatalf("ParseHM: %v", err)
	}
	if got != 9*time.Hour+30*time.Minute {
		t.Fatalf("ParseHM = %v, want 9h30m", got)
	}
	if _, err := wtfmt.ParseHM("nope"); err == nil {
		t.Fatal("ParseHM(nope) should error")
	}
}

func TestParseStop(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	start := day.Add(9 * time.Hour)
	now := day.Add(23 * time.Hour)
	// absolute HH:MM
	got, err := wtfmt.ParseStop("12:00", start, now)
	if err != nil {
		t.Fatalf("ParseStop abs: %v", err)
	}
	if got.Hour() != 12 || got.Day() != 18 {
		t.Fatalf("ParseStop abs = %v, want 12:00 on the start's day", got)
	}
	// relative +1h30m off the start
	got2, err := wtfmt.ParseStop("+1h30m", start, now)
	if err != nil {
		t.Fatalf("ParseStop rel: %v", err)
	}
	if !got2.Equal(start.Add(90 * time.Minute)) {
		t.Fatalf("ParseStop rel = %v, want start+1h30m", got2)
	}
}
```

If `ParseStop`'s real signature differs from `(arg string, start, now time.Time)` (the Today edit dialog calls `parseStop(arg, start, _ time.Time)`), match the REAL signature and adjust the test — do not invent a new shape.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/wtfmt/ -run 'ParseHM|ParseStop' -v`
Expected: FAIL (undefined `wtfmt.ParseHM`).

- [ ] **Step 4: Move the helpers**

Create `internal/tui/screen/worktime/wtfmt/timeparse.go` with the three functions, exported (`ParseHM`, `ParseStop`, `NormalizeDurationArg`), bodies copied verbatim from `format.go`. Then in `format.go` delete the three originals; replace every in-package call (`parseHM(...)`→`wtfmt.ParseHM(...)`, etc.) found in Step 1, adding the `wtfmt` import. If a thin unexported shim is cleaner for many callers, you may keep `func parseHM(s string) (time.Duration, error) { return wtfmt.ParseHM(s) }` — but prefer direct repointing (DRY).

- [ ] **Step 5: Run tests to verify everything passes**

Run: `go test ./internal/tui/screen/worktime/... -count=1` then `go build ./...`
Expected: PASS and clean build (the `worktime` package compiles against `wtfmt.*`).

- [ ] **Step 6: Commit**

```bash
git add internal/tui/screen/worktime/wtfmt/timeparse.go internal/tui/screen/worktime/wtfmt/timeparse_test.go internal/tui/screen/worktime/format.go
git commit -m "refactor(worktime): move time parsers to wtfmt sink (unblocks daydetail without import cycle)"
```

---

## Task 2: Woche — day cursor (`listnav`) + selection highlight

**Files:**
- Modify: `internal/tui/screen/worktime/week/route.go`
- Test: `internal/tui/screen/worktime/week/route_test.go` (append; create if absent, mirroring `worktime/route_test.go`'s fake-API style)

**Interfaces:**
- Consumes: `listnav.New()`, `listnav.Cursor.Handle(k, count, pageSize)`, `listnav.Cursor.Index()`, `listnav.Cursor.Clamp(count)` (`internal/tui/ui/listnav`).
- Produces: `week.Route` now tracks a selected day index; no new exported surface.

- [ ] **Step 1: Write the failing test**

Read `internal/tui/screen/worktime/week/route.go` (struct ~line 36, `Update`, `View`) and any existing `week/route_test.go` to reuse its fake `API` (the fake implements `GetWeek` + `ListDayOffs`). Append:

```go
func TestWeekRoute_CursorMovesAndClamps(t *testing.T) {
	r := newLoadedWeekRoute(t) // helper: constructs Route, feeds a loadedMsg with N day rows, returns *Route
	// down twice
	r2, _ := r.Update(keyDown())
	wr := r2.(*week.Route)
	wr2, _ := wr.Update(keyDown())
	wr = wr2.(*week.Route)
	if got := wr.SelectedIndex(); got != 2 {
		t.Fatalf("cursor after 2×down = %d, want 2", got)
	}
	// up past top clamps at 0
	for i := 0; i < 5; i++ {
		x, _ := wr.Update(keyUp())
		wr = x.(*week.Route)
	}
	if got := wr.SelectedIndex(); got != 0 {
		t.Fatalf("cursor clamped top = %d, want 0", got)
	}
}
```

Add a tiny test-only accessor `func (r *Route) SelectedIndex() int { return r.cur.Index() }` in `route.go` (it is small, documented, and used by the route's own `View`; export is acceptable for testability — or place the test in `package week` instead of `week_test` to read the unexported field). `keyDown()`/`keyUp()` build `tea.KeyPressMsg` for `tea.KeyDown`/`tea.KeyUp` — copy the helper from `worktime/route_test.go` or `listnav`'s tests.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/week/ -run CursorMovesAndClamps -v`
Expected: FAIL (no cursor; `SelectedIndex` undefined).

- [ ] **Step 3: Add the cursor**

In `week.Route`: add field `cur listnav.Cursor`; initialize in `NewRoute` with `listnav.New()`. In `Update`, before the existing `wtnav.Lateral`/`navKey` handling of `tea.KeyPressMsg`, feed list-nav keys to the cursor:

```go
case tea.KeyPressMsg:
	if c, ok := r.cur.Handle(m, len(r.days), 0); ok {
		r.cur = c
		return r, nil
	}
	// ... existing wtnav.Lateral / navKey handling unchanged ...
```

After `loadedMsg` populates `r.days`, clamp: `r.cur = r.cur.Clamp(len(r.days))`. In `View`, render the row at `r.cur.Index()` with the selected style (use the same selection treatment the Today list uses — read `worktime/route.go` `View` for the cursor row style, e.g. a `▸`/reverse style from `theme`). Keep the existing `▶ IsToday` marker distinct from the cursor marker.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/screen/worktime/week/ -v` then `go build ./...`
Expected: PASS, clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen/worktime/week/route.go internal/tui/screen/worktime/week/route_test.go
git commit -m "feat(worktime/week): cursor over day rows via listnav (clamped, ↑/↓)"
```

---

## Task 3: Woche — week prev/next navigation (forward-clamped) + header range

**Files:**
- Modify: `internal/tui/ui/grammar/grammar.go` (add `WeekPrev`, `WeekNext`)
- Modify: `internal/tui/screen/worktime/week/route.go` (`weekOffset`, `loadCmd(ref)`, clamp, header, KeyHints)
- Test: `internal/tui/screen/worktime/week/route_test.go` (append)

**Interfaces:**
- Consumes: `grammar.WeekPrev`, `grammar.WeekNext` (new); existing `GetWeek(ctx, ref string)` and `ListDayOffs(ctx, from, to string)` on the route's `API`.
- Produces: no new exported surface; `loadCmd` now takes the computed week ref.

- [ ] **Step 1: Read current `loadCmd` + day/dayoff loading**

Read `week/route.go` `loadCmd()` to see exactly how it computes `from`/`to` for `ListDayOffs` and how it calls `GetWeek` (currently `GetWeek(ctx, "")`). Note: `GetWeek(ref)` accepts any `YYYY-MM-DD` within the target week; `""` = current week.

- [ ] **Step 2: Write the failing test**

```go
func TestWeekRoute_PrevNextWeekRefAndForwardClamp(t *testing.T) {
	fake := &fakeWeekAPI{} // records the last ref passed to GetWeek
	r := week.NewRoute(fake, testPalette(), testRegistry())
	_ = r.Init() // triggers initial load (ref "" or current-week ref)
	// prev week → ref should be 7 days before the current week's anchor
	r2, cmd := r.Update(keyRune('['))
	runCmd(t, cmd) // execute the returned tea.Cmd so GetWeek is invoked on the fake
	wr := r2.(*week.Route)
	if !fake.lastRefIsOneWeekBack(t) {
		t.Fatalf("after '[' the GetWeek ref = %q, want one week back", fake.lastRef)
	}
	// next from offset 0 must clamp (no future week): go back once, forward twice → offset 0, not +1
	r3, _ := wr.Update(keyRune(']')) // offset -1 -> 0
	wr = r3.(*week.Route)
	r4, cmd2 := wr.Update(keyRune(']')) // offset 0 -> clamp at 0
	runCmd(t, cmd2)
	if fake.offsetWentPositive {
		t.Fatal("next-week past current week must be clamped to offset 0")
	}
}
```

Provide the `fakeWeekAPI` (records `lastRef`; `GetWeek` returns a fixed 7-day slice; `ListDayOffs` returns nil) and `runCmd`/`keyRune` helpers in the test file (copy `keyRune` from an existing route test). `lastRefIsOneWeekBack` compares `fake.lastRef` against `time.Now()` minus 7 days (same ISO week). Keep the assertion tolerant to the Monday-anchor convention (assert the ref's ISO week == now's ISO week − 1).

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/week/ -run PrevNextWeek -v`
Expected: FAIL (`[`/`]` do nothing; offset not tracked).

- [ ] **Step 4: Implement**

In `grammar.go` add (model on the existing `Open`/`Back` var entries):

```go
WeekPrev = register(Binding{ID: "week.prev", Keys: []Key{Rune("[")}, KeyLabel: "[", Desc: "Woche zurück"})
WeekNext = register(Binding{ID: "week.next", Keys: []Key{Rune("]")}, KeyLabel: "]", Desc: "Woche vor"})
```

(Use the file's actual registration mechanism — match how `MoveUp` etc. are declared; if they are plain `var X = Binding{...}` appended to a registry slice, follow that exact form.)

In `week.Route`: add `offset int` (0 = current week). Add a `weekRef()` helper:

```go
// weekRef returns a YYYY-MM-DD inside the week `offset` weeks from now (offset<=0).
func (r *Route) weekRef() string {
	if r.offset == 0 {
		return ""
	}
	return time.Now().AddDate(0, 0, r.offset*7).Format("2006-01-02")
}
```

Make `loadCmd()` use `r.weekRef()` for `GetWeek` and compute the matching `from`/`to` for `ListDayOffs` from that same anchor (derive Monday..Sunday of the target week; reuse whatever weekday math `loadCmd` already does, shifted by `offset*7` days). In `Update`'s `tea.KeyPressMsg` branch (after the cursor handling from Task 2, before `wtnav.Lateral`):

```go
switch {
case grammar.WeekPrev.Matches(m):
	r.offset--
	return r, r.loadCmd()
case grammar.WeekNext.Matches(m):
	if r.offset < 0 {
		r.offset++
		return r, r.loadCmd()
	}
	return r, nil // clamp: no future weeks
}
```

In `View`/header, show the week range or `‹ KW.. ›` derived from `r.days` (first/last date). Add `grammar.WeekPrev.Hint()`, `grammar.WeekNext.Hint()` to `KeyHints()`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/tui/screen/worktime/week/ -v` then `go build ./...`
Expected: PASS, clean build.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/ui/grammar/grammar.go internal/tui/screen/worktime/week/route.go internal/tui/screen/worktime/week/route_test.go
git commit -m "feat(worktime/week): [/] week prev/next (forward-clamped) + week-range header"
```

---

## Task 4: `daydetail` route skeleton — list a day's sessions

**Files:**
- Create: `internal/tui/screen/worktime/daydetail/api.go`
- Create: `internal/tui/screen/worktime/daydetail/state.go`
- Create: `internal/tui/screen/worktime/daydetail/route.go`
- Test: `internal/tui/screen/worktime/daydetail/state_test.go`
- Test: `internal/tui/screen/worktime/daydetail/route_test.go`

**Interfaces:**
- Consumes: `apiclient` types (`domain.WorkSession`), `shell.Route`/`shell.Frame`/`shell.EventMsg`/`shell.PopRouteMsg`, `listnav`, `toast`, `theme`, `wtfmt`.
- Produces:
  - `daydetail.API` interface (Task 4 minimum): `ListSessionsRange(ctx, since, until time.Time) ([]domain.WorkSession, error)`. (Extended in Tasks 6/7.)
  - `daydetail.NewRoute(api API, pal theme.Palette, date time.Time) *Route` implementing `shell.Route`.
  - `daydetail.dayRow` (unexported) + `buildRows(sessions []domain.WorkSession, day time.Time) []dayRow`.

- [ ] **Step 1: Write the failing state test**

Create `internal/tui/screen/worktime/daydetail/state_test.go`:

```go
package daydetail

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestBuildRows(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)
	mk := func(id string, sh, sm, eh, em int) domain.WorkSession {
		s := day.Add(time.Duration(sh)*time.Hour + time.Duration(sm)*time.Minute)
		e := day.Add(time.Duration(eh)*time.Hour + time.Duration(em)*time.Minute)
		return domain.WorkSession{ID: id, Start: s, Stop: &e, Tag: "t-" + id}
	}
	rows := buildRows([]domain.WorkSession{mk("b", 13, 0, 14, 0), mk("a", 9, 0, 11, 0)}, day)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].ID != "a" {
		t.Fatalf("rows not sorted ascending by start: %+v", rows)
	}
	if rows[0].Dur != 2*time.Hour {
		t.Fatalf("row a duration = %v, want 2h", rows[0].Dur)
	}
}
```

`dayRow` carries at least `ID string`, `Start, Stop time.Time`, `Dur time.Duration`, `Project, Tag string`. `buildRows` sorts ascending by `Start` and computes `Dur = Stop.Sub(Start)` (skip a nil-Stop running session, or include it with `Dur=0` — choose include-with-0 and a "läuft" marker, since a backfilled day normally has only completed sessions). Test is in `package daydetail` to read unexported types.

- [ ] **Step 2: Run state test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/daydetail/ -run BuildRows -v`
Expected: FAIL (package/`buildRows` undefined).

- [ ] **Step 3: Implement `state.go` + `api.go`**

`api.go`:

```go
package daydetail

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// API is the daydetail route's view of the backend. Extended in later tasks.
type API interface {
	ListSessionsRange(ctx context.Context, since, until time.Time) ([]domain.WorkSession, error)
}
```

`state.go`: `dayRow` struct + `buildRows`. Use `sort.Slice` ascending by `Start`. Resolve `Project` from `domain.WorkSession.ProjectID`? The Today route shows a project NAME, not id — read how `today_state.go` derives the project label. If sessions carry only `ProjectID`, the row shows the id or a separate `ListProjects` lookup (defer name resolution to Task 6 when `ListProjects` joins the interface; for Task 4 the row may show the tag + Von–Bis + Dauer, project column added in Task 6). Keep Task 4 to: Von–Bis · Dauer · Tag.

- [ ] **Step 4: Write the failing route test**

Create `internal/tui/screen/worktime/daydetail/route_test.go`:

```go
package daydetail_test

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/daydetail"
	"github.com/serverkraken/flow/internal/tui/theme"
)

type fakeAPI struct {
	since, until time.Time
	sessions     []domain.WorkSession
}

func (f *fakeAPI) ListSessionsRange(_ context.Context, since, until time.Time) ([]domain.WorkSession, error) {
	f.since, f.until = since, until
	return f.sessions, nil
}

func TestDayDetail_LoadsRangedSessions(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	s := day.Add(9 * time.Hour)
	e := day.Add(11 * time.Hour)
	f := &fakeAPI{sessions: []domain.WorkSession{{ID: "a", Start: s, Stop: &e, Tag: "deep"}}}
	r := daydetail.NewRoute(f, theme.Default(), day) // use the real palette accessor; adjust name
	cmd := r.Init()
	msg := cmd() // execute loadCmd → loadedMsg
	r2, _ := r.Update(msg)
	out := r2.View(shellFrame()) // small helper building shell.Frame{Width:80,Height:24,Pal:...}
	if !strings.Contains(out, "deep") {
		t.Fatalf("day view missing session: %q", out)
	}
	// loadCmd brackets exactly [startOfDay, startOfDay+24h)
	if !f.since.Equal(day) || !f.until.Equal(day.Add(24*time.Hour)) {
		t.Fatalf("range = [%v,%v), want the day's bounds", f.since, f.until)
	}
	_ = tea.KeyPressMsg{} // ensure import used if helpers live elsewhere
}
```

Adjust `theme.Default()`/palette accessor and the `shell.Frame` helper to the real APIs (read another route test for the exact palette constructor and a `View` test helper).

- [ ] **Step 5: Run route test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/daydetail/ -run LoadsRanged -v`
Expected: FAIL (`NewRoute` undefined).

- [ ] **Step 6: Implement `route.go`**

`Route` struct: `api API`, `pal theme.Palette`, `day time.Time`, `rows []dayRow`, `cur listnav.Cursor`, `loaded bool`, `err error`, `toast toast.Model`. Implement `shell.Route`:
- `Title()` → e.g. `"Tag · " + r.day.Format("Mon 02.01.")` (German weekday optional; keep simple).
- `Init()` → `r.loadCmd()`.
- `loadCmd()` → returns a `tea.Cmd` calling `api.ListSessionsRange(ctx, startOfDay, startOfDay+24h)` and wrapping the result in a `loadedMsg{rows, err}` (build rows via `buildRows`). `startOfDay := time.Date(y,m,d,0,0,0,0, r.day.Location())`.
- `Update`: handle `loadedMsg` (set rows, clamp cursor); `shell.EventMsg` (if session event → `r.loadCmd()`); `tea.KeyPressMsg` → cursor via `listnav` (return early if handled), `esc`/`grammar.Back` → `shell.PopRouteMsg{}` cmd. Update the `toast`.
- `View(f shell.Frame)`: title box + the rows (`Von–Bis · Dauer · Tag`), selected row highlighted; empty-state line when `len(rows)==0` ("Keine Buchungen — n zum Nachbuchen"); render `r.toast`.
- `KeyHints()`: `↑/↓ Auswahl`, `n Nachbuchen` (added Task 6), `esc zurück`. For Task 4 advertise `↑/↓` + `esc` only.

Define `loadedMsg struct{ rows []dayRow; err error }`.

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/tui/screen/worktime/daydetail/ -v` then `go build ./...`
Expected: PASS, clean build. Confirm no import cycle: `go list -deps ./internal/tui/screen/worktime/daydetail/ | grep -c 'screen/worktime$'` must print `0`.

- [ ] **Step 8: Commit**

```bash
git add internal/tui/screen/worktime/daydetail/
git commit -m "feat(worktime/daydetail): pushed route lists a day's sessions (ListSessionsRange, cursor, SSE reload, esc back)"
```

---

## Task 5: Wire Woche `enter` → push `daydetail`

**Files:**
- Modify: `internal/tui/screen/worktime/week/route.go` (import `daydetail`, push on `enter`)
- Test: `internal/tui/screen/worktime/week/route_test.go` (append)

**Interfaces:**
- Consumes: `daydetail.NewRoute(api, pal, date)` (Task 4), `shell.PushRouteMsg`, `grammar.Open` (Enter).
- Produces: pressing `enter` on a week-day row emits `shell.PushRouteMsg{Route: *daydetail.Route}` carrying that row's date.

- [ ] **Step 1: Write the failing test**

The week route's `API` (Task 0 state) only declares `GetWeek`+`ListDayOffs`. The push needs a `daydetail.API` (`ListSessionsRange`). Decide the wiring: the week route is constructed in `BuildRegistry` with the concrete `*apiclient.Client`, which satisfies BOTH interfaces. So widen the week route's stored client to also pass to `daydetail.NewRoute`. Cleanest: have `week.NewRoute` accept the concrete `*apiclient.Client` (it already does — `BuildRegistry` passes `client`). Confirm `week.Route.api` is the concrete client or add a field holding it.

```go
func TestWeekRoute_EnterPushesDayDetail(t *testing.T) {
	r := newLoadedWeekRoute(t) // has >=1 day row with a known Date "2026-06-18"
	// move cursor to the row whose Date is 2026-06-18 (or assert against row 0's date)
	_, cmd := r.Update(keyEnter())
	if cmd == nil {
		t.Fatal("enter produced no command")
	}
	msg := cmd()
	push, ok := msg.(shell.PushRouteMsg)
	if !ok {
		t.Fatalf("enter msg = %T, want shell.PushRouteMsg", msg)
	}
	if push.Route == nil || !strings.Contains(push.Route.Title(), "18") {
		t.Fatalf("pushed route title = %q, want the selected day", titleOf(push.Route))
	}
}
```

`keyEnter()` builds a `tea.KeyPressMsg` for Enter. Parse the selected row's `Date` (`apiclient.WeekDay.Date`, `"2006-01-02"`) into a `time.Time` (`time.ParseInLocation("2006-01-02", d.Date, time.Local)`).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/week/ -run EnterPushes -v`
Expected: FAIL (enter not handled / no push).

- [ ] **Step 3: Implement**

In `week.Route.Update`, after cursor + week-nav handling, before `wtnav.Lateral`:

```go
if grammar.Open.Matches(m) && len(r.days) > 0 {
	d := r.days[r.cur.Index()]
	day, err := time.ParseInLocation("2006-01-02", d.Date, time.Local)
	if err != nil {
		return r, nil
	}
	child := daydetail.NewRoute(r.client, r.pal, day) // r.client = the *apiclient.Client
	return r, func() tea.Msg { return shell.PushRouteMsg{Route: child} }
}
```

If `week.Route` doesn't already hold the concrete client, add `client *apiclient.Client` set in `NewRoute` (keep the narrow `api API` for `GetWeek`/`ListDayOffs`, or just use the client directly for both — pick the smaller diff). Add `enter` to `KeyHints()` (`grammar.Open.Hint()` relabeled "Tag öffnen").

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/screen/worktime/... -v` then `go build ./...`
Expected: PASS, clean build. Re-confirm no cycle: `go build ./...` (week now imports daydetail; daydetail must still not import worktime/week).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen/worktime/week/route.go internal/tui/screen/worktime/week/route_test.go
git commit -m "feat(worktime/week): enter on a day pushes the daydetail route"
```

---

## Task 6: `daydetail` Nachbuchen (Add) dialog

**Files:**
- Create: `internal/tui/screen/worktime/daydetail/dialogs.go` (Nachbuchen state + helpers)
- Modify: `internal/tui/screen/worktime/daydetail/api.go` (extend interface)
- Modify: `internal/tui/screen/worktime/daydetail/route.go` (`n` opens dialog; dialog is an `InputCapturer`)
- Test: `internal/tui/screen/worktime/daydetail/route_test.go` (append)

**Interfaces:**
- Consumes: `wtfmt.ParseHM`/`wtfmt.ParseStop` (Task 1), `fuzzylist` (`internal/tui/ui/fuzzylist`), `form.NewTextInput`, `apiclient.AddSession`/`ListProjects`/`CreateProject`, `domain`.
- Produces: extended `daydetail.API`:
  ```go
  AddSession(ctx context.Context, projectID *string, start, stop time.Time, tag, note string) (domain.WorkSession, error)
  ListProjects(ctx context.Context) ([]domain.Project, error)
  CreateProject(ctx context.Context, name string) (domain.Project, error)
  ```
  (Match the EXACT existing client signatures — read `apiclient` for `ListProjects`/`CreateProject` return types and adjust verbatim.)

- [ ] **Step 1: Read the Today booking flow as the template**

Read `internal/tui/screen/worktime/dialogs.go` (the `bookingState` + project `fuzzylist` glue: how items load via `ListProjects`, `WithCreateHint`, how Enter→`Selection()` resolves create-vs-pick, how `CreateProject` is called) and `route.go`'s `dialogBooking` handling + the `InputCapturer` integration. The Nachbuchen dialog is **booking + two HH:MM fields**.

- [ ] **Step 2: Write the failing test**

```go
func TestDayDetail_NachbuchenSubmitsAddSession(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	f := &fakeAPI{ /* sessions: nil; projects: [{ID:"p1",Name:"Acme"}] */ }
	r := daydetail.NewRoute(f, theme.Default(), day)
	drive(t, r, r.Init()) // load empty day + project list
	r = press(t, r, keyRune('n'))           // open Nachbuchen
	// pick the existing project (Enter on the prefilled fuzzylist), then fill Von/Bis
	r = press(t, r, keyEnter())             // select project p1
	r = typeInto(t, r, "09:00")             // Von
	r = press(t, r, keyTab())               // next field
	r = typeInto(t, r, "12:00")             // Bis
	r = press(t, r, keyEnter())             // submit
	if f.addCalls != 1 {
		t.Fatalf("AddSession calls = %d, want 1", f.addCalls)
	}
	if !f.lastStart.Equal(day.Add(9*time.Hour)) || !f.lastStop.Equal(day.Add(12*time.Hour)) {
		t.Fatalf("AddSession times = [%v,%v), want 09:00–12:00 on the context day", f.lastStart, f.lastStop)
	}
	if f.lastProjectID == nil || *f.lastProjectID != "p1" {
		t.Fatalf("AddSession project = %v, want p1", f.lastProjectID)
	}
}

func TestDayDetail_OverlapErrorShowsToast(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	f := &fakeAPI{addErr: apiErr(409)} // fake returns an error mapping to the overlap message
	r := daydetail.NewRoute(f, theme.Default(), day)
	drive(t, r, r.Init())
	r = press(t, r, keyRune('n'))
	r = press(t, r, keyEnter())
	r = typeInto(t, r, "09:00"); r = press(t, r, keyTab()); r = typeInto(t, r, "10:00")
	r = press(t, r, keyEnter())
	if !strings.Contains(r.View(shellFrame()), "berlapp") { // "Überlappung"/"overlaps"
		t.Fatalf("expected overlap toast, got: %q", r.View(shellFrame()))
	}
}
```

Provide test helpers `drive`/`press`/`typeInto`/`keyTab`/`apiErr` (model on existing route tests; `typeInto` feeds each rune as a `tea.KeyPressMsg`). Extend `fakeAPI` with `addCalls`, `lastStart/lastStop/lastProjectID`, `addErr`, `projects`, and the `ListProjects`/`CreateProject`/`AddSession` methods. The exact apiclient error shape for a 409: read how `apiclient` surfaces non-2xx (a typed error / status) so the route can detect overlap vs. generic; the toast text can be generic ("Konnte nicht speichern: …") echoing the server message — the test asserts the server message substring. If apiclient returns the server body in the error string, assert that substring instead of "berlapp".

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/tui/screen/worktime/daydetail/ -run 'Nachbuchen|OverlapError' -v`
Expected: FAIL (`n` not handled).

- [ ] **Step 4: Implement**

`dialogs.go`: `nachbuchenState{ proj fuzzylist.Model; projItems []fuzzylist.Item; von textinput.Model; bis textinput.Model; tag textinput.Model; note textinput.Model; focus int }` (focus order: project, Von, Bis, Tag, Note). Constructor `openNachbuchen(pal, projects)` builds the fuzzylist (`fuzzylist.New(projectItems(projects), pal).WithCreateHint("neu: %s")`) and the text fields (`form.NewTextInput("HH:MM", pal)`, `form.NewTextInput("HH:MM oder +1h30m", pal)`, etc.). Make the dialog satisfy `shell.InputCapturer` so the shell routes all keys to it while open (read how Today's dialogs declare capture).

Submit (`grammar.Open` on the last field, or a dedicated submit when focus is past Note): resolve project from `proj.Selection()` (create via `CreateProject` when `isCreate`), parse `start := startOfDay + wtfmt.ParseHM(von)`, `stop := wtfmt.ParseStop(bis, start, now)`, call `api.AddSession(ctx, &projectID, start, stop, tag, note)`. On success → close dialog + `r.loadCmd()` (+ optional success toast). On error → `r.toast = toast.Error(err.Error(), pal)` (use the real toast constructor), keep the dialog open so the user can fix the input. `Tab`/`Shift+Tab` move focus; `Esc` cancels (close dialog, no call).

In `route.go`: add `dialog` field (a small kind enum or a `*nachbuchenState`), open on `grammar`-matched `n` (add a `grammar.Nachbuchen` binding `n`, "Nachbuchen", OR match `tea.KeyPressMsg` rune `n` directly and advertise it via a local hint — prefer a grammar binding for single-source). While the dialog is open, forward keys to it (InputCapturer) and short-circuit the route's own key handling.

Add `n` to `KeyHints()`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/tui/screen/worktime/daydetail/ -v` then `go build ./...`
Expected: PASS, clean build.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/screen/worktime/daydetail/
git commit -m "feat(worktime/daydetail): n = Nachbuchen form (project picker + Von/Bis) → AddSession; errors as toast"
```

---

## Task 7: `daydetail` edit + delete

**Files:**
- Modify: `internal/tui/screen/worktime/daydetail/dialogs.go` (edit + delete state)
- Modify: `internal/tui/screen/worktime/daydetail/api.go` (extend interface)
- Modify: `internal/tui/screen/worktime/daydetail/route.go` (`e`/`d` handling)
- Test: `internal/tui/screen/worktime/daydetail/route_test.go` (append)

**Interfaces:**
- Consumes: `confirm` (`internal/tui/ui/confirm`), `wtfmt`, `apiclient.EditSession`/`DeleteSession`.
- Produces: extended `daydetail.API`:
  ```go
  EditSession(ctx context.Context, id string, projectID *string, tag, note string, start time.Time, stop *time.Time) (domain.WorkSession, error)
  DeleteSession(ctx context.Context, id string) error
  ```
  (Match the EXACT existing client signatures.)

- [ ] **Step 1: Write the failing test**

```go
func TestDayDetail_EditSubmitsEditSession(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	s := day.Add(9 * time.Hour); e := day.Add(11 * time.Hour)
	f := &fakeAPI{sessions: []domain.WorkSession{{ID: "a", Start: s, Stop: &e, Tag: "old"}}}
	r := daydetail.NewRoute(f, theme.Default(), day)
	drive(t, r, r.Init())
	r = press(t, r, keyRune('e'))     // open edit on the selected (row 0)
	// edit dialog prefilled with 09:00 / 11:00; change Bis to 12:00
	r = focusBisField(t, r)           // helper: Tab to the Bis field
	r = clearAndType(t, r, "12:00")
	r = press(t, r, keyEnter())       // submit
	if f.editCalls != 1 || f.lastEditID != "a" {
		t.Fatalf("EditSession calls=%d id=%q, want 1/a", f.editCalls, f.lastEditID)
	}
	if f.lastEditStop == nil || !f.lastEditStop.Equal(day.Add(12*time.Hour)) {
		t.Fatalf("edit stop = %v, want 12:00", f.lastEditStop)
	}
}

func TestDayDetail_DeleteConfirms(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	s := day.Add(9 * time.Hour); e := day.Add(11 * time.Hour)
	f := &fakeAPI{sessions: []domain.WorkSession{{ID: "a", Start: s, Stop: &e}}}
	r := daydetail.NewRoute(f, theme.Default(), day)
	drive(t, r, r.Init())
	r = press(t, r, keyRune('d'))     // open delete confirm
	r = press(t, r, keyRune('y'))     // confirm (or however confirm accepts — read confirm.Model)
	if f.delCalls != 1 || f.lastDelID != "a" {
		t.Fatalf("DeleteSession calls=%d id=%q, want 1/a", f.delCalls, f.lastDelID)
	}
}
```

Read `internal/tui/ui/confirm` for how confirmation is accepted (it emits `confirm.ResultMsg{Confirmed bool}` per the Today delete flow) and drive it the same way Today's route test does. Extend `fakeAPI` with edit/delete recorders.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/screen/worktime/daydetail/ -run 'Edit|Delete' -v`
Expected: FAIL (`e`/`d` not handled).

- [ ] **Step 3: Implement**

`dialogs.go`: `editState{ id string; date time.Time; von,bis,tag,note textinput.Model; focus int }`, opened by `openEdit(row dayRow, pal)` prefilling Von/Bis from the row's `Start`/`Stop` (`Format("15:04")`) — model VERBATIM on Today's `openEdit`/`submitEdit` (`worktime/dialogs.go:124-204`), substituting `wtfmt.ParseHM`/`wtfmt.ParseStop` and the row's date. Submit → `api.EditSession(ctx, id, nil, tag, note, start, &stop)` → success closes + reloads, error → toast (keep open). Delete: reuse `confirm.NewDanger("Session löschen?", …, pal)`; on `confirm.ResultMsg{Confirmed:true}` → `api.DeleteSession(ctx, id)` → reload, error → toast.

`route.go`: handle `e`/`d` (prefer `grammar.Edit`/`grammar.Delete` bindings if they exist; else local rune match + hint) to open the dialog for the cursor's row (`r.rows[r.cur.Index()]`); guard `len(rows)>0`. Route dialog keys via `InputCapturer`; on `confirm.ResultMsg`/edit-submit-msg act then close. Add `e`/`d` to `KeyHints()`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/screen/worktime/daydetail/ -v` then `go build ./...`
Expected: PASS, clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen/worktime/daydetail/
git commit -m "feat(worktime/daydetail): e/d edit + delete the selected session (EditSession/DeleteSession, confirm)"
```

---

## Task 8: Full CI + live done-gate

**Files:** none (verification only)

- [ ] **Step 1: Run the full CI**

Run: `make ci`
Expected: green — lint `0 issues`, templ drift OK, build, all tests, coverage gate (~80%, currently ~84%). If coverage dipped below the gate because the new route packages are light on tests, add a focused `View`/`KeyHints` test rather than lowering the gate.

- [ ] **Step 2: Live TUI dogfood vs the dev stack**

Bring up the stack and run the TUI (see `reference_flow_dev_env`):

```bash
make dev-up
make dev-run            # one shell
export FLOW_TOKEN=$(make -s dev-token)
go run ./cmd/flow ui worktime   # or the documented launch; land on Heute, → to Woche
```

Verify by hand:
- Woche: `[`/`]` moves week back/forward; forward stops at the current week; `↑/↓` moves the day cursor; the week-range header updates.
- `enter` on a past day opens Tag-Detail listing that day's sessions.
- `n` → Nachbuchen: pick a project, Von 09:00 / Bis 12:00, submit → the row appears (SSE reload).
- Re-run `n` with an overlapping range → a toast shows the overlap/error; the dialog stays open.
- `e` on a row edits its Bis; `d` deletes after confirm. Both reflect immediately.
- `esc` returns from Tag-Detail to Woche; the Heute route is unchanged.

- [ ] **Step 3: Final confirmation**

No commit. Report `make ci` output + the dogfood observations. (Interactive TUI dogfood may be handed to Soenne, as in prior slices — capture the data-path via the route tests + a `flow` smoke if interactive isn't run here.)

---

## Self-Review

**Spec coverage (Slice 2 scope, spec lines 148-181):**
- Woche `weekOffset` + prev/next + forward-clamp → Task 3 ✓
- `loadCmd` passes computed `ref` to `GetWeek(ref)` + header range → Task 3 ✓
- Distinct keys for week nav (not clashing with sub-tab `←/→`) → `[`/`]`, Task 3 ✓
- New `daydetail` `shell.Route` pushed via NavStack on `enter`, carries the date → Tasks 4, 5 ✓
- Loads via `ListSessionsRange(startOfDay,endOfDay)`, renders Von–Bis · Projekt · Dauer → Tasks 4 (Von–Bis·Dauer·Tag), 6 (project column with `ListProjects`) ✓
- `n` Nachbuchen form (project `fuzzylist` + Von/Bis + tag/note → `AddSession`, context date) → Task 6 ✓
- `e`/`d` edit/delete reusing edit/delete dialog pattern, targeting the chosen day → Task 7 ✓
- SSE `session.*` reloads the day → Task 4 ✓
- Heute route unchanged → no task touches `worktime/route.go` behavior (only Task 1 repoints parser calls) ✓
- Tests: week prev/next + forward-clamp; enter→push; daydetail lists ranged; nachbuchen submits with context date; edit/delete target selection; overlap/future error → toast → Tasks 3,5,4,6,7 ✓
- Import-cycle avoidance via `wtfmt` extraction → Task 1 ✓ (architecture-enabling, not in spec but required by the package graph)

**Placeholder scan:** The plan instructs the implementer to read specific file:line templates (Today booking/edit/delete dialogs, the week `loadCmd`, the palette/frame test helpers) and copy them verbatim with named substitutions, because the exact bubbletea scaffolding (focus handling, `InputCapturer` wiring, palette constructor, confirm result plumbing) is established in-repo and must match its siblings rather than be reinvented. Every NEW logic path (offset math, week ref, push msg, AddSession submit, parser extraction) has concrete code. No "TBD"/"add error handling"/"write tests for the above".

**Type consistency:** `daydetail.API` grows monotonically across Tasks 4/6/7 (`ListSessionsRange` → `+AddSession/ListProjects/CreateProject` → `+EditSession/DeleteSession`), each method copied to match the EXACT `apiclient.Client` signature (Slice-1 `AddSession(ctx, *string, start, stop, tag, note)`, `ListSessionsRange(ctx, since, until)`, `EditSession(ctx, id, *string, tag, note, start, *stop)`, `DeleteSession(ctx, id)`). `*apiclient.Client` satisfies the interface at every stage, so the same `client` flows Woche→daydetail. `buildRows`/`dayRow`/`loadedMsg` are package-internal and consistent. `grammar.WeekPrev`/`WeekNext` (Task 3) are the single source for the `[`/`]` hints.

**Open implementation choices left to the implementer (each with a rule):** the project-name resolution in rows (defer to Task 6's `ListProjects`); whether `n`/`e`/`d` use new `grammar` bindings vs. local rune match (prefer grammar for single-source); the exact overlap toast substring (assert the server message the apiclient surfaces). None are placeholders — each names the deciding factor and the file to read.
