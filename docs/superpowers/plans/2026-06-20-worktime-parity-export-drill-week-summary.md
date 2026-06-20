# Worktime Parity (Export Drill + Woche Summary) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the rebuild's Worktime match the old design: Export becomes a drilled form (out of the sub-tab strip, fully keyboard-navigable, Esc returns), and the Woche view regains its WOCHE GESAMT total + KENNZAHLEN block (Schnitt/Ziele/Saldo, pace dots, auf-Kurs/Rückstand) with day-off accounting.

**Architecture:** Part A removes Export from `wtnav.SubTabs` (back to four tabs) and makes the Export route capture all input + show the normal breadcrumb. Part B ports the pace-dot classification into the worktime layer and composes the Woche summary client-side from the already-day-off-netted per-day data plus `ListDayOffs` (no server change).

**Tech Stack:** Go, charm.land/bubbletea/v2, charm.land/lipgloss/v2, existing `internal/tui/{ui/statusbar,ui/glyphs,theme}`, `internal/tui/screen/worktime/{wtnav,wtfmt}`, `internal/adapter/apiclient`.

## Global Constraints

- Branch: `rebuild` (unmerged). Do not merge to main.
- `make ci` must stay green (lint + templ + build + tests; coverage gate ≥80%). Run it, not just `go test`. Lint QF1002: a switch that becomes all-`k.Code` must be tagged `switch k.Code {}`.
- German UI strings, proper umlauts (Wochenende/Feiertag/Urlaub/Krank, Schnitt/Ziele/Saldo, „auf Kurs"/„im Rückstand"). Code/comments English.
- No emoji; glyphs only from `ui/glyphs` (`▲`=Up, `▼`=Down, `●`=Filled, `○`=Empty, `·`=BulletDot). No raw hex; colours via `theme.Sem()` / palette fields.
- Hints use the ` → ` connector and `  ·  ` separator (never `=`).
- Day-off netting stays server-side: `WeekDay.TargetMin` is already reduced for holidays/vacation/sick — do not re-net client-side.

---

### Task 1: wtnav — drop Export from the strip (four tabs)

**Files:**
- Modify: `internal/tui/screen/worktime/wtnav/wtnav.go` (`SubTabs`, `Idx*`)
- Test: `internal/tui/screen/worktime/wtnav/wtnav_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `SubTabs` with four entries; `Idx{Heute,Woche,Stats,Frei}` (no `IdxExport`). `Lateral`/`Strip` unchanged in shape.

- [ ] **Step 1: Update the tests first (red)**

In `wtnav_test.go`: change `TestStrip_ContainsAllLabels` to assert the four labels `Heute/Woche/Stats/Frei` and assert `Export` is NOT present:

```go
func TestStrip_ContainsFourLabelsNotExport(t *testing.T) {
	out := wtnav.Strip(wtnav.IdxStats, 200, theme.Default)
	for _, l := range []string{"Heute", "Woche", "Stats", "Frei"} {
		if !strings.Contains(out, l) {
			t.Fatalf("strip missing %q: %q", l, out)
		}
	}
	if strings.Contains(out, "Export") {
		t.Fatalf("Export must no longer be a strip tab: %q", out)
	}
}
```

Add a clamp test at the new last index (Frei):

```go
func TestLateral_RightFromFreiClamps(t *testing.T) {
	if wtnav.Lateral(testReg(), wtnav.IdxFrei, key(tea.KeyRight)) != nil {
		t.Fatal("→ from Frei (last tab) must clamp to nil")
	}
}
```

Remove/replace any existing test that references `wtnav.IdxExport` or asserts five labels (e.g. the old `TestStrip_ContainsAllLabels`, and any `TestLateral_*` that stepped onto Export). Keep `testReg()` (its `"e"` entry is harmless — Export still opens via the registry).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/screen/worktime/wtnav/ -v`
Expected: FAIL — `IdxExport` undefined (in removed test) / Strip still contains "Export".

- [ ] **Step 3: Edit SubTabs + Idx**

In `wtnav.go`, change the const block and `SubTabs` to four entries:

```go
const (
	IdxHeute = iota
	IdxWoche
	IdxStats
	IdxFrei
)

var SubTabs = []SubTab{
	{Label: "Heute", Key: ""},
	{Label: "Woche", Key: "w"},
	{Label: "Stats", Key: "t"},
	{Label: "Frei", Key: "d"},
}
```

`Lateral` already clamps via `target >= len(SubTabs)`, so Frei (index 3) `→` → target 4 ≥ 4 → nil. No other change needed in `wtnav.go`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/screen/worktime/wtnav/ -v`
Expected: PASS.

- [ ] **Step 5: Run make ci**

Run: `make ci`
Expected: FAIL to compile — `export/route.go` still references `wtnav.IdxExport`. That is fixed in Task 2; if you want a green checkpoint, do Task 2 before re-running. Otherwise note the expected breakage and proceed.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/screen/worktime/wtnav/
git commit -m "feat(worktime): drop Export from the sub-tab strip (four tabs)"
```

---

### Task 2: Export route — capture all input, leave the strip, show breadcrumb

**Files:**
- Modify: `internal/tui/screen/worktime/export/route.go` (`CapturesInput`, `View`, `HideBreadcrumb`, `KeyHints`)
- Test: `internal/tui/screen/worktime/export/route_test.go`

**Interfaces:**
- Consumes: nothing new. (Drops the `wtnav.Strip`/`wtnav.IdxExport` usage from Task 6 of the strip plan.)

- [ ] **Step 1: Update the tests first (red)**

Replace the strip-era test `TestExport_StripAndHideCrumbAndArrowsStillEditDate` with:

```go
func TestExport_CapturesAllFocusesNoStripNoCrumbHider(t *testing.T) {
	r := export.NewRoute(nil, time.Now, theme.Default, nil)
	// CapturesInput is true at every focus (whole form), incl. Range(0)/Format(3).
	for focus := 0; focus <= 4; focus++ {
		r.SetFocusForTest(focus) // add a tiny test helper, or set the field if exported; see Step 3
		if !r.CapturesInput() {
			t.Fatalf("CapturesInput must be true at focus %d (whole form captures)", focus)
		}
	}
	// View no longer renders the sub-tab strip.
	out := r.View(shell.Frame{Width: 200, Height: 24, Pal: theme.Default})
	for _, l := range []string{"Heute", "Woche", "Stats", "Frei"} {
		if strings.Contains(out, l) {
			t.Fatalf("Export View must not render the strip tab %q: %q", l, out)
		}
	}
	// Export must NOT implement BreadcrumbHider (breadcrumb shows for the drill).
	if _, ok := interface{}(r).(interface{ HideBreadcrumb() bool }); ok {
		t.Fatal("Export must not implement HideBreadcrumb (it is a drilled route)")
	}
}
```

If adding a `SetFocusForTest` helper is undesirable, instead assert `CapturesInput()` returns true on a fresh route (focus 0) and after stepping focus via `Update(tea.KeyPressMsg{Code: tea.KeyTab})` to reach focus 1..4 — but note Tab handling requires the shell; simplest is a minimal unexported test helper in the same package: `func (r *Route) SetFocusForTest(f int) { r.focus = f }` in a `_test.go` file is not possible across the method set — add it to `route.go` guarded by intent, OR make the test live in package `export` (internal test) and set `r.focus` directly. Prefer the internal-test approach: put this test in `package export` (not `export_test`) and set `r.focus = focus` directly, dropping the helper.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/screen/worktime/export/ -v`
Expected: FAIL — CapturesInput false at focus 0/3; strip still rendered; HideBreadcrumb present.

- [ ] **Step 3: Capture all input**

Replace `CapturesInput` (and its now-stale comment):

```go
// CapturesInput reports that the export form owns the keyboard at every focus:
// it is a multi-field form (Range/von/bis/Format/Pfad), so Tab/Shift+Tab field
// cycling, ←/→ value+date editing, and digits must all reach the route rather
// than the shell. q/Esc still reach the back chain (handled in the shell before
// this guard), returning to Heute. Implements shell.InputCapturer.
func (r *Route) CapturesInput() bool { return true }
```

- [ ] **Step 4: Remove the strip from View**

Change the final return of `View` back to the plain body (drop the `wtnav.Strip(...)` prefix):

```go
	return b.String()
```

- [ ] **Step 5: Remove HideBreadcrumb and the Bereich hint**

Delete the method:

```go
func (r *Route) HideBreadcrumb() bool { return true }
```

and drop the 5th KeyHint so it reads:

```go
func (r *Route) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "tab", Desc: "Feld"},
		{Key: "←/→", Desc: "wählen"},
		{Key: "enter", Desc: "export"},
		{Key: "esc", Desc: "zurück"},
	}
}
```

Remove the now-unused `wtnav` import from `export/route.go` **only if** nothing else there uses it (it still uses `wtnav.Registry`/`navKey` — keep the import; just ensure `wtnav.Strip`/`wtnav.IdxExport` are gone).

- [ ] **Step 6: Run tests + make ci**

Run: `go test ./internal/tui/screen/worktime/export/ -v`
Expected: PASS.
Run: `make ci`
Expected: GREEN (Task 1's `IdxExport` removal now compiles).

- [ ] **Step 7: Commit**

```bash
git add internal/tui/screen/worktime/export/
git commit -m "feat(worktime): Export is a drilled form — captures input, shows breadcrumb, no strip"
```

---

### Task 3: Advertise `e → Export` on the four strip routes

**Files:**
- Modify: `internal/tui/screen/worktime/route.go` (Heute `KeyHints`)
- Modify: `internal/tui/screen/worktime/week/route.go` (`KeyHints`)
- Modify: `internal/tui/screen/worktime/statsrange/route.go` (`KeyHints`)
- Modify: `internal/tui/screen/worktime/dayoffs/route.go` (`KeyHints`)
- Test: `internal/tui/screen/worktime/week/route_test.go` (one assertion is enough)

**Interfaces:** none new.

- [ ] **Step 1: Write the failing test**

In `week/route_test.go`:

```go
func TestWeek_KeyHintsAdvertiseExport(t *testing.T) {
	r := week.NewRoute(nil, theme.Default, nil)
	found := false
	for _, h := range r.KeyHints() {
		if h.Key == "e" && h.Desc == "Export" {
			found = true
		}
	}
	if !found {
		t.Fatal("Woche KeyHints must advertise {e, Export} now that Export left the strip")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/week/ -run KeyHintsAdvertiseExport -v`
Expected: FAIL.

- [ ] **Step 3: Add the `{e, Export}` hint to all four**

- `week/route.go` `KeyHints`:

```go
func (r *Route) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "←/→", Desc: "Bereich"},
		{Key: "e", Desc: "Export"},
		{Key: "esc", Desc: "zurück"},
	}
}
```

- `statsrange/route.go` `KeyHints`:

```go
func (r *Route) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "m/W", Desc: "Monat/KW"},
		{Key: "←/→", Desc: "Bereich"},
		{Key: "e", Desc: "Export"},
		{Key: "esc", Desc: "zurück"},
	}
}
```

- `dayoffs/route.go` `KeyHints` (non-dialog branch):

```go
	return []keyhint.Hint{
		{Key: "g/a/D", Desc: "Ziel/Add/Del"},
		{Key: "b", Desc: "Bundesland"},
		{Key: "←/→", Desc: "Bereich"},
		{Key: "e", Desc: "Export"},
		{Key: "esc", Desc: "zurück"},
	}
```

- Heute `route.go` `KeyHints`: insert `{Key: "e", Desc: "Export"}` before the `?` hint:

```go
	hints = append(hints, keyhint.Hint{Key: "←/→", Desc: "Bereich"})
	hints = append(hints, keyhint.Hint{Key: "e", Desc: "Export"})
	hints = append(hints, keyhint.Hint{Key: "?", Desc: "Hilfe"})
	return hints
```

(The `e` accelerator already opens Export via each route's `navKey`/`w/t/d/e` case — only the hint is added. Footer caps at 4; overflow surfaces in `?`-help.)

- [ ] **Step 4: Run tests + make ci**

Run: `go test ./internal/tui/screen/worktime/... -v`
Expected: PASS.
Run: `make ci`
Expected: GREEN.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen/worktime/route.go internal/tui/screen/worktime/week/ internal/tui/screen/worktime/statsrange/ internal/tui/screen/worktime/dayoffs/
git commit -m "feat(worktime): advertise e → Export on the four strip routes"
```

---

### Task 4: Pace-dot classification (Woche-local)

**Files:**
- Create: `internal/tui/screen/worktime/week/pacedot.go`
- Test: `internal/tui/screen/worktime/week/pacedot_test.go`

**Interfaces:**
- Consumes: `apiclient.WeekDay`, `apiclient.DayOff`, `domain.Kind*`, `theme.Palette`.
- Produces:
  - `type paceDotKind int`; consts `paceDotMissed=0, paceDotHit, paceDotRunning, paceDotDayOff`
  - `func classifyPaceDot(d apiclient.WeekDay, off *apiclient.DayOff) paceDotKind`
  - `func paceGlyph(k paceDotKind) string`
  - `func paceColor(k paceDotKind, off *apiclient.DayOff, p theme.Palette) theme.Color`

- [ ] **Step 1: Write the failing test**

```go
package week

import (
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
)

func TestClassifyPaceDot(t *testing.T) {
	off := &apiclient.DayOff{Kind: "vacation"}
	if got := classifyPaceDot(apiclient.WeekDay{LoggedMin: 0}, off); got != paceDotDayOff {
		t.Fatalf("dayoff present → DayOff, got %v", got)
	}
	if got := classifyPaceDot(apiclient.WeekDay{TargetMin: 480, LoggedMin: 480}, nil); got != paceDotHit {
		t.Fatalf("logged>=target → Hit, got %v", got)
	}
	if got := classifyPaceDot(apiclient.WeekDay{TargetMin: 480, LoggedMin: 60, IsToday: true}, nil); got != paceDotRunning {
		t.Fatalf("today, not yet hit → Running, got %v", got)
	}
	if got := classifyPaceDot(apiclient.WeekDay{TargetMin: 480, LoggedMin: 60}, nil); got != paceDotMissed {
		t.Fatalf("past open workday → Missed, got %v", got)
	}
}

func TestPaceGlyph(t *testing.T) {
	if paceGlyph(paceDotMissed) != glyphs.Empty {
		t.Fatal("Missed must use ○ (glyphs.Empty)")
	}
	for _, k := range []paceDotKind{paceDotHit, paceDotRunning, paceDotDayOff} {
		if paceGlyph(k) != glyphs.Filled {
			t.Fatalf("kind %v must use ● (glyphs.Filled)", k)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/week/ -run 'ClassifyPaceDot|PaceGlyph' -v`
Expected: FAIL — undefined.

- [ ] **Step 3: Write the implementation**

```go
// Package week pace-dot classification: one Mon–Fri dot per workday, ported
// from the old domain/pace_dot.go but adapted to the apiclient.WeekDay DTO
// (minute ints, no live Active — today-not-yet-hit renders as Running).
package week

import (
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
)

type paceDotKind int

const (
	paceDotMissed paceDotKind = iota
	paceDotHit
	paceDotRunning
	paceDotDayOff
)

// classifyPaceDot maps one weekday to its pace-dot slot. off is non-nil when a
// day-off (holiday/vacation/sick) covers the date.
func classifyPaceDot(d apiclient.WeekDay, off *apiclient.DayOff) paceDotKind {
	if off != nil {
		return paceDotDayOff
	}
	if d.TargetMin > 0 && d.LoggedMin >= d.TargetMin {
		return paceDotHit
	}
	if d.IsToday {
		return paceDotRunning
	}
	return paceDotMissed
}

// paceGlyph returns ○ for a missed/open day and ● for every accounted slot.
func paceGlyph(k paceDotKind) string {
	if k == paceDotMissed {
		return glyphs.Empty
	}
	return glyphs.Filled
}

// paceColor maps a pace-dot kind to a theme colour. Day-off kinds reuse the
// available Sem slots (rebuild has no Purple/Orange): holiday=Info,
// vacation=Accent, sick=Warning.
func paceColor(k paceDotKind, off *apiclient.DayOff, p theme.Palette) theme.Color {
	sem := p.Sem()
	switch k {
	case paceDotHit:
		return sem.Success
	case paceDotRunning:
		return sem.Active
	case paceDotDayOff:
		if off != nil {
			switch domain.Kind(off.Kind) {
			case domain.KindHoliday:
				return sem.Info
			case domain.KindVacation:
				return sem.Accent
			case domain.KindSick:
				return sem.Warning
			}
		}
		return p.FgMuted
	}
	return sem.Border
}
```

Verify the real `theme.Palette` has `Sem().{Success,Active,Info,Accent,Warning,Border}` and a `FgMuted` field, and that `theme.Color` is the return type used elsewhere (e.g. `kindcolor.Color`). Adjust the type name if the codebase calls it differently.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/screen/worktime/week/ -run 'ClassifyPaceDot|PaceGlyph' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen/worktime/week/pacedot.go internal/tui/screen/worktime/week/pacedot_test.go
git commit -m "feat(worktime): Woche pace-dot classification (hit/running/dayoff/missed)"
```

---

### Task 5: Woche route loads day-offs for the week range

**Files:**
- Modify: `internal/tui/screen/worktime/week/route.go` (`API`, `loadedMsg`, `loadCmd`, `Update`, `Route` struct)
- Test: `internal/tui/screen/worktime/week/route_test.go`

**Interfaces:**
- Consumes: `apiclient.{WeekDay,DayOff}`.
- Produces: `Route.offs map[string]apiclient.DayOff` populated on load; `API` interface gains `ListDayOffs`.

- [ ] **Step 1: Write the failing test**

Use a fake API implementing both `GetWeek` and `ListDayOffs`, asserting the day-off range is requested and stored:

```go
type fakeWeekAPI struct {
	days    []apiclient.WeekDay
	offs    []apiclient.DayOff
	gotFrom string
	gotTo   string
}

func (f *fakeWeekAPI) GetWeek(ctx context.Context, ref string) ([]apiclient.WeekDay, error) {
	return f.days, nil
}
func (f *fakeWeekAPI) ListDayOffs(ctx context.Context, from, to string) ([]apiclient.DayOff, error) {
	f.gotFrom, f.gotTo = from, to
	return f.offs, nil
}

func TestWeek_LoadsDayOffsForRange(t *testing.T) {
	api := &fakeWeekAPI{
		days: []apiclient.WeekDay{
			{Date: "2026-06-15"}, {Date: "2026-06-16"}, {Date: "2026-06-21"},
		},
		offs: []apiclient.DayOff{{Day: "2026-06-16", Kind: "vacation", Label: "Urlaub"}},
	}
	r := week.NewRoute(api, theme.Default, nil)
	msg := r.Init()() // run the load cmd
	r2, _ := r.Update(msg)
	rr := r2.(*week.Route)
	if api.gotFrom != "2026-06-15" || api.gotTo != "2026-06-21" {
		t.Fatalf("ListDayOffs range = %s..%s, want 2026-06-15..2026-06-21", api.gotFrom, api.gotTo)
	}
	if _, ok := rr.DayOffForTest("2026-06-16"); !ok {
		t.Fatal("day-off map must contain 2026-06-16 after load")
	}
}
```

(Add a tiny exported test accessor `func (r *Route) DayOffForTest(date string) (apiclient.DayOff, bool) { v, ok := r.offs[date]; return v, ok }`, or put this test in `package week` and read `r.offs` directly — prefer the internal-test approach and drop the accessor.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/week/ -run LoadsDayOffs -v`
Expected: FAIL — `ListDayOffs` not on `API`; `offs` not stored.

- [ ] **Step 3: Extend the route**

In `week/route.go`:

- Extend the `API` interface:

```go
type API interface {
	GetWeek(ctx context.Context, ref string) ([]apiclient.WeekDay, error)
	ListDayOffs(ctx context.Context, from, to string) ([]apiclient.DayOff, error)
}
```

- Extend `loadedMsg` and the `Route` struct:

```go
type loadedMsg struct {
	days []apiclient.WeekDay
	offs []apiclient.DayOff
	err  error
}
```

```go
type Route struct {
	api    API
	pal    theme.Palette
	reg    wtnav.Registry
	days   []apiclient.WeekDay
	offs   map[string]apiclient.DayOff
	loaded bool
	err    error
}
```

- `loadCmd` fetches both; the day-off range is the first/last loaded day:

```go
func (r *Route) loadCmd() tea.Cmd {
	api := r.api
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		days, err := api.GetWeek(ctx, "")
		if err != nil {
			return loadedMsg{err: err}
		}
		var offs []apiclient.DayOff
		if len(days) > 0 {
			offs, err = api.ListDayOffs(ctx, days[0].Date, days[len(days)-1].Date)
		}
		return loadedMsg{days: days, offs: offs, err: err}
	}
}
```

- In `Update`, the `loadedMsg` case builds the map:

```go
	case loadedMsg:
		r.loaded, r.err, r.days = true, m.err, m.days
		r.offs = make(map[string]apiclient.DayOff, len(m.offs))
		for _, o := range m.offs {
			r.offs[o.Day] = o
		}
		return r, nil
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/screen/worktime/week/ -v`
Expected: PASS (existing week tests still pass — fakes that only implement `GetWeek` must be updated to also implement `ListDayOffs`; update any such test fake in this file).

- [ ] **Step 5: Run make ci**

Run: `make ci`
Expected: GREEN. (`*apiclient.Client` already has `ListDayOffs`, so it still satisfies `API`.)

- [ ] **Step 6: Commit**

```bash
git add internal/tui/screen/worktime/week/
git commit -m "feat(worktime): Woche loads day-offs for the week range"
```

---

### Task 6: Woche summary render (WOCHE GESAMT + KENNZAHLEN + day-off labels)

**Files:**
- Modify: `internal/tui/screen/worktime/week/route.go` (`View`)
- Create: `internal/tui/screen/worktime/week/summary.go` (aggregation + section render helpers)
- Test: `internal/tui/screen/worktime/week/summary_test.go`

**Interfaces:**
- Consumes: `paceDotKind`/`classifyPaceDot`/`paceGlyph`/`paceColor` (Task 4), `Route.offs` (Task 5), `statusbar.BarColored`, `wtfmt.{FormatMin,FormatSaldo}`, `glyphs.{Up,Down}`, `theme`.
- Produces:
  - `type weekSummary struct{ totalLogged, totalTarget, workdays, hits, expected int }`
  - `func computeWeekSummary(days []apiclient.WeekDay, offs map[string]apiclient.DayOff) weekSummary`
  - `func isWeekendDate(date string) bool`

- [ ] **Step 1: Write the failing test**

```go
package week

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

func TestComputeWeekSummary(t *testing.T) {
	// Mon hit, Tue today+open, Wed open(past? no — before today), Thu vacation, Fri open; Sat/Sun weekend.
	days := []apiclient.WeekDay{
		{Date: "2026-06-15", TargetMin: 480, LoggedMin: 480},               // Mon hit (past)
		{Date: "2026-06-16", TargetMin: 480, LoggedMin: 120, IsToday: true}, // Tue today, not hit
		{Date: "2026-06-17", TargetMin: 480, LoggedMin: 0},                  // Wed future
		{Date: "2026-06-18", TargetMin: 0, LoggedMin: 0},                    // Thu vacation (netted target 0)
		{Date: "2026-06-19", TargetMin: 480, LoggedMin: 0},                  // Fri future
		{Date: "2026-06-20", TargetMin: 0, LoggedMin: 0},                    // Sat weekend
		{Date: "2026-06-21", TargetMin: 0, LoggedMin: 0},                    // Sun weekend
	}
	offs := map[string]apiclient.DayOff{"2026-06-18": {Day: "2026-06-18", Kind: "vacation", Label: "Urlaub"}}
	s := computeWeekSummary(days, offs)
	if s.workdays != 4 { // Mon,Tue,Wed,Fri (Thu is vacation, Sat/Sun weekend)
		t.Fatalf("workdays=%d, want 4", s.workdays)
	}
	if s.hits != 1 { // only Mon
		t.Fatalf("hits=%d, want 1", s.hits)
	}
	if s.expected != 1 { // Mon is past; Tue today not hit → not expected; Wed/Fri future
		t.Fatalf("expected=%d, want 1", s.expected)
	}
	if s.totalLogged != 600 || s.totalTarget != 1920 { // logged 480+120; target 480*4
		t.Fatalf("totals logged=%d target=%d, want 600/1920", s.totalLogged, s.totalTarget)
	}
}

func TestIsWeekendDate(t *testing.T) {
	if !isWeekendDate("2026-06-20") { // Saturday
		t.Fatal("2026-06-20 is a Saturday")
	}
	if isWeekendDate("2026-06-15") { // Monday
		t.Fatal("2026-06-15 is a Monday, not weekend")
	}
}

func TestWeekView_HasSummarySectionsAndDayOffLabel(t *testing.T) {
	api := &fakeWeekAPI{
		days: []apiclient.WeekDay{
			{Date: "2026-06-15", TargetMin: 480, LoggedMin: 480},
			{Date: "2026-06-18", TargetMin: 0, LoggedMin: 0},
			{Date: "2026-06-20", TargetMin: 0, LoggedMin: 0},
		},
		offs: []apiclient.DayOff{{Day: "2026-06-18", Kind: "vacation", Label: "Urlaub"}},
	}
	r := week.NewRoute(api, theme.Default, nil)
	r2, _ := r.Update(r.Init()())
	out := r2.(*week.Route).View(shell.Frame{Width: 100, Height: 30, Pal: theme.Default})
	for _, want := range []string{"WOCHE GESAMT", "KENNZAHLEN", "Schnitt", "Ziele", "Saldo", "Urlaub", "Wochenende"} {
		if !strings.Contains(out, want) {
			t.Fatalf("week View missing %q:\n%s", want, out)
		}
	}
}
```

(`TestWeekView_*` lives in `package week_test` with the `fakeWeekAPI` from Task 5; the pure helper tests live in `package week`. Split across files if the package decls differ.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/screen/worktime/week/ -run 'WeekSummary|IsWeekend|WeekView_Has' -v`
Expected: FAIL — helpers undefined / View lacks sections.

- [ ] **Step 3: Write summary.go**

```go
package week

import (
	"fmt"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtfmt"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
	"github.com/serverkraken/flow/internal/tui/ui/statusbar"
)

type weekSummary struct {
	totalLogged, totalTarget int // minutes
	workdays, hits, expected int
}

// isWeekendDate reports whether the "2006-01-02" date falls on Sat/Sun.
func isWeekendDate(date string) bool {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		return false
	}
	wd := d.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}

// computeWeekSummary aggregates week totals and goal counters. Totals sum all
// days (day-off/weekend targets are already 0); workdays/hits/expected count
// only non-weekend, non-day-off days. "expected" = past workdays plus today if
// already hit, using the IsToday flag to split past/today/future (no clock).
func computeWeekSummary(days []apiclient.WeekDay, offs map[string]apiclient.DayOff) weekSummary {
	var s weekSummary
	todayIdx := -1
	for i, d := range days {
		if d.IsToday {
			todayIdx = i
		}
	}
	for i, d := range days {
		s.totalLogged += d.LoggedMin
		s.totalTarget += d.TargetMin
		if isWeekendDate(d.Date) {
			continue
		}
		if _, off := offs[d.Date]; off {
			continue
		}
		s.workdays++
		hit := d.TargetMin > 0 && d.LoggedMin >= d.TargetMin
		if hit {
			s.hits++
		}
		past := todayIdx >= 0 && i < todayIdx
		if past || (i == todayIdx && hit) {
			s.expected++
		}
	}
	return s
}

// renderSummary builds the WOCHE GESAMT + KENNZAHLEN block.
func (r *Route) renderSummary(width int) string {
	s := computeWeekSummary(r.days, r.offs)
	pal := r.pal
	var b strings.Builder

	pct := 0
	if s.totalTarget > 0 {
		pct = s.totalLogged * 100 / s.totalTarget
	}
	barCells := width - 6
	if barCells < 10 {
		barCells = 10
	}
	b.WriteString("\n  " + theme.Dim("WOCHE GESAMT", pal) + "\n")
	b.WriteString(fmt.Sprintf("  %s / %s\n",
		wtfmt.FormatMin(s.totalLogged), wtfmt.FormatMin(s.totalTarget)))
	b.WriteString("  " + statusbar.BarColored(pct, barCells, pal.Sem().Accent, pal) + "\n")

	avg := 0
	if s.workdays > 0 {
		avg = s.totalLogged / s.workdays
	}
	saldo := s.totalLogged - s.totalTarget
	b.WriteString("\n  " + theme.Dim("KENNZAHLEN", pal) + "\n")
	b.WriteString(fmt.Sprintf("  Schnitt %s  %s  Ziele %d/%d  %s  Saldo %s\n",
		wtfmt.FormatMin(avg), glyphs.BulletDot, s.hits, s.workdays,
		glyphs.BulletDot, wtfmt.FormatSaldo(saldo)))
	b.WriteString("  " + r.renderPaceRow(s) + "\n")
	return b.String()
}

// renderPaceRow renders the Mon–Fri pace dots + goal count + track marker.
func (r *Route) renderPaceRow(s weekSummary) string {
	pal := r.pal
	dots := make([]string, 0, len(r.days))
	for _, d := range r.days {
		if isWeekendDate(d.Date) {
			continue
		}
		var off *apiclient.DayOff
		if v, ok := r.offs[d.Date]; ok {
			off = &v
		}
		k := classifyPaceDot(d, off)
		dots = append(dots, theme.Fg(paceGlyph(k), paceColor(k, off, pal), pal))
	}
	count := theme.Dim(fmt.Sprintf("%d/%d Ziele", s.hits, s.workdays), pal)
	var track string
	switch {
	case s.expected == 0:
		track = theme.Dim(glyphs.BulletDot, pal)
	case s.hits >= s.expected:
		track = theme.Success(glyphs.Up+" auf Kurs", pal)
	default:
		track = theme.Warning(glyphs.Down+" im Rückstand", pal)
	}
	return strings.Join(dots, " ") + "   " + count + "   " + track
}
```

Confirm the helpers exist with these names: `wtfmt.FormatSaldo` (used by statsrange), `theme.Success`/`theme.Warning`/`theme.Dim` builders, and a foreground helper for an arbitrary colour — if `theme.Fg(s, color, pal)` does not exist, use a local `lipgloss.NewStyle().Foreground(c).Render(g)` instead (import lipgloss). Adjust `pal.Sem().Accent`/colour field names to the real palette API.

- [ ] **Step 4: Enrich per-day rows + append the summary in View**

Replace `week/route.go` `View`'s day loop and final return so off days show a label and the summary is appended:

```go
	cells := 20
	var b strings.Builder
	b.WriteString("\n")
	for _, d := range r.days {
		marker := "  "
		if d.IsToday {
			marker = theme.Active(glyphs.Active, f.Pal) + " "
		}
		var detail string
		if off, ok := r.offs[d.Date]; ok {
			label := off.Label
			if label == "" {
				label = off.Kind
			}
			detail = theme.Dim(label, f.Pal)
		} else if isWeekendDate(d.Date) {
			detail = theme.Dim("Wochenende", f.Pal)
		} else {
			pct := 0
			if d.TargetMin > 0 {
				pct = d.LoggedMin * 100 / d.TargetMin
			}
			detail = fmt.Sprintf("%s  %s / %s",
				statusbar.Bar(pct, cells, f.Pal),
				wtfmt.FormatMin(d.LoggedMin), wtfmt.FormatMin(d.TargetMin))
		}
		b.WriteString("  " + marker + d.Date + "  " + detail + "\n")
	}
	b.WriteString(r.renderSummary(f.Width))
	return strip + b.String()
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/tui/screen/worktime/week/ -v`
Expected: PASS (helpers + View tests + earlier tasks' tests).

- [ ] **Step 6: Run make ci**

Run: `make ci`
Expected: GREEN.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/screen/worktime/week/
git commit -m "feat(worktime): Woche summary — WOCHE GESAMT + KENNZAHLEN + pace dots + dayoff labels"
```

---

### Task 7: Wiring verification + manual walk

**Files:** none (verification).

- [ ] **Step 1: Confirm Export left the strip and nav is whole**

Run: `rg -n 'IdxExport|wtnav.Strip' internal/tui/screen/worktime/export`
Expected: no matches (Export no longer references the strip).
Run: `rg -n 'func \(r \*Route\) CapturesInput' internal/tui/screen/worktime/export/route.go`
Expected: returns `true` unconditionally.

- [ ] **Step 2: Confirm the four-tab strip + Export hint**

Run: `rg -n 'SubTabs' internal/tui/screen/worktime/wtnav/wtnav.go` and confirm four entries.
Run: `rg -n '"e", Desc: "Export"' internal/tui/screen/worktime` and confirm all four strip routes.

- [ ] **Step 3: `make ci` green**

Run: `make ci`
Expected: lint + templ + build + tests pass; coverage ≥ gate.

- [ ] **Step 4: Manual walk against the dev stack**

Start the dev stack and `flow ui worktime` (see `reference_flow_dev_env`: `make dev-up`, `make dev-run`). Verify:
- Strip shows four tabs `Heute · Woche · Stats · Frei`; `e` opens Export as a drilled form with a `Worktime › Export` breadcrumb (no strip).
- In Export: `Tab`/`Shift+Tab` cycle all five fields (incl. Range/Format — no top-tab jump); `←/→` change the value / edit dates; `Esc`/`q` returns to Heute.
- Woche shows per-day rows (weekend → „Wochenende", a day-off → its label), then WOCHE GESAMT (total + bar) and KENNZAHLEN (Schnitt · Ziele N/N · Saldo, pace dots ○/●, „▲ auf Kurs"/„▼ im Rückstand"); a vacation/holiday/sick day is excluded from the workday/Ziele counts and its target is netted.

- [ ] **Step 5: Commit any final note (if needed)**

```bash
git add -A && git commit -m "docs: worktime parity wiring verified" || true
```

---

## Self-Review notes

- **Spec coverage:** Part A → Tasks 1 (strip), 2 (Export route), 3 (e-hint); Part B → Tasks 4 (pace dots), 5 (load day-offs), 6 (summary render); verification → Task 7.
- **Type consistency:** `classifyPaceDot(apiclient.WeekDay, *apiclient.DayOff) paceDotKind`, `paceGlyph`/`paceColor`, `computeWeekSummary(days, offs) weekSummary`, `isWeekendDate(string) bool`, `Route.offs map[string]apiclient.DayOff` used identically across Tasks 4–6.
- **Build-order note:** Task 1 alone breaks compilation (Export references `IdxExport`); Task 2 restores green. Run Tasks 1+2 back-to-back; the controller should treat the first green checkpoint as end-of-Task-2.
- **No clock in aggregation:** `computeWeekSummary` splits past/today/future via the `IsToday` flag, staying pure and testable.
- **Out of scope:** server changes, progress-bar colour tweak, live per-day running distinction, mouse.
