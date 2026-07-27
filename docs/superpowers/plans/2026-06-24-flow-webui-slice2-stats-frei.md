# WebUI Slice 2 — Stats & Frei Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the two remaining Worktime sub-tabs — **Stats** and **Frei** — from the pre-Slice-0 markup onto the Slice-0 AppShell + Slice-1 component system, completing the worktime sub-tab strip (Heute · Woche · Historie · Stats · Frei).

**Architecture:** Pure composition on the existing foundation — no new usecase or mechanism. Two page-templ rewrites/creations render `components.Base → AppShell("today", nil, worktimeSubnav(...), #content-SSE-wrapper)` exactly like `heute.templ`/`woche.templ`. Handlers are rewritten to build new view models from the **existing** usecases (`Stats.Today/Week/Burndown`, `SetTarget`, `GetSettings`, `ListDayOffs/AddDayOffs/DeleteDayOff`, `RegenIcsToken`, `SetBundesland`). One new presentational component (the burndown banner with a pace marker) and one tiny dependency-free clipboard helper.

**Tech Stack:** Go 1.23+, templ (`github.com/a-h/templ`), htmx + SSE (vendored, offline), Tailwind v4 (built via `make web`), the Slice-0 `components` package, i18n via `components.T`/`Tn`.

## Global Constraints

Copied verbatim from the spec — every task's requirements implicitly include this section.

- **Module path:** `github.com/serverkraken/flow`.
- **Offline:** every asset local under `/static/`; pages MUST render via `components.Base` (through `AppShell`), never a hand-rolled `<head>`. No `unpkg`/`googleapis`/`gstatic`/`fontshare`/`cdn.tailwindcss`.
- **No browser popups:** every destructive action (delete a day-off, regenerate the ICS token) goes through `components.ConfirmDialog`; never `window.alert/confirm/prompt`. New JS must pass `verify-no-popups` (banned regex: a bare `alert(`/`confirm(`/`prompt(` or `window.alert|confirm|prompt`).
- **i18n:** no hardcoded display strings in templates — all via `components.T(ctx,"key")`/`Tn`; every new key added to BOTH `internal/i18n/catalog_de.go` (full German) and `internal/i18n/catalog_en.go` (English stub). German primary. (Day-off **Kind** labels are the one allowed exception: they come from `domain.Kind.LabelDe()`, mirroring `woche.templ`.)
- **Owner-scoping:** every store/usecase call scoped to `u.ID`.
- **`.templ` files must NOT `import "github.com/a-h/templ"`** — it is auto-injected. (`templ.Attributes{}`, `templ.KV`, `templ.SafeURL` are usable without an import.)
- **Glyph whitelist (monospace):** `▶ ■ ‖ ✓ ✗ ▲ ▼ ● ○ ★ ☼ ✚ ▎ ▰▱ › · ◆ ▤ ▾ ▴`. No colored emoji.
- **CI:** `make ci` green — `lint` (gofumpt + staticcheck, incl. **U1000 unused** — delete dead code), `verify-generate` (committed `_templ.go` matches `templ generate`), `verify-css` (committed `app.css` matches a fresh `tailwindcss` build — run `make web` after any new utility class), `verify-no-popups`, `cover` (coverage gate **75%**), `build`.

### Per-task workflow rules (apply to every task)

- After editing any `.templ`: run `go tool templ generate` (or `make generate`) and commit the regenerated `*_templ.go`.
- After adding/removing any Tailwind utility class in a `.templ`: run `make web` (needs the `tailwindcss` CLI on PATH) and commit `internal/adapter/webui/static/app.css`.
- A task is DONE only when `make ci` is green. Run `go test ./...` during the loop, but gate the commit on `make ci`.
- Commit at the end of every task with a focused message.

---

## File Structure

**Create:**
- `internal/adapter/webui/components/burndownbanner.templ` — the Monats-Burndown banner component (Total/Target, on-track chip, progress bar + pace marker).
- `internal/adapter/webui/stats_vm.go` — `StatsVM`, `WeekdayTargetVM`, `statsSaldoHue`, `StatsWeekdayVMs`.
- `internal/adapter/webui/frei.templ` — the Frei page (replaces `dayoffs.templ`).
- `internal/adapter/webui/frei_vm.go` — `FreiVM`, `FreiRowVM`, `FreiBundeslandOption`, `FreiKindOption`, `freiKinds`, `freiKindChip`, `freiDeleteVals`.
- `internal/adapter/webui/static/js/clipboard.js` — dependency-free copy-to-clipboard helper.

**Modify:**
- `internal/adapter/webui/components/worktime_vm.go` — add `BurndownVM`.
- `internal/adapter/webui/stats.templ` — rewrite onto AppShell (saldo tiles + burndown banner + target form).
- `internal/adapter/webui/format.go` — delete the stats-only helpers `fmtInt`, `monthBarStyle`, `monthBarPct`, `weekBarStyle` (keep `fmtDur`).
- `internal/adapter/webui/render_test.go` — delete `TestMonthBarPct_Clamping` + `TestWeekBarStyle_Clamping` (keep the rest).
- `internal/adapter/httpserver/webui_stats.go` — rewrite `statsData`/handlers; add `burndownBannerVM`; extend `handleWebSetTarget`; add `parseWeekdayTargets`; delete `fmtMin`/`fmtSaldo`.
- `internal/adapter/httpserver/webui_dayoffs.go` — rewrite `dayOffData`/handlers to `FreiVM`; add `handleWebSetBundesland`, `bundeslandOptions`, `bundeslandName`.
- `internal/adapter/httpserver/server.go` — add the `POST /ui/dayoffs/bundesland` route.
- `internal/adapter/httpserver/webui_stats_test.go` — rewrite the two SetTarget tests for the new authoritative form semantics.
- `internal/adapter/httpserver/webui_dayoffs_test.go` — update assertions for the new `FreiVM` markup; add the bundesland test.
- `internal/i18n/catalog_de.go` + `internal/i18n/catalog_en.go` — new keys.

**Delete:**
- `internal/adapter/webui/dayoffs.templ` + `internal/adapter/webui/dayoffs_templ.go` (once `DayOffPage`/`DayOffFragment`/`DayOffData` are unreferenced — in Task 4).

> **Do NOT touch** `internal/adapter/webui/nav.templ` — the old `Nav(active, user)` helper is still used by `docs.templ`, `export.templ`, `projects.templ`. `worktimeSubnav` in `heute.templ` already carries the `stats`→`/stats` and `frei`→`/dayoffs` tabs — reuse unchanged.

---

# SLICE 1 — Stats page

## Task 1: BurndownBanner component

The presentational banner: a Card-style surface with "Monat gesamt", an on-track/behind chip, a `ProgressBar` filled to the logged percentage, and a thin vertical **pace marker** at `PacePct%`. All math arrives pre-computed in the VM (the pace-marker math is unit-tested in Task 2's `burndownBannerVM`).

**Files:**
- Create: `internal/adapter/webui/components/burndownbanner.templ`
- Modify: `internal/adapter/webui/components/worktime_vm.go` (add `BurndownVM`)
- Test: `internal/adapter/webui/components/burndownbanner_test.go`

**Interfaces:**
- Consumes: `ProgressBar(pct int, variant string)`, `T(ctx, key)`, `itoa(int) string` — all existing in the `components` package.
- Produces: `type BurndownVM struct { Total, Target string; Pct, PacePct int; Variant string; OnTrack bool }` and `templ BurndownBanner(vm BurndownVM)` — consumed by Task 2's `stats.templ` and `burndownBannerVM`.

- [ ] **Step 1: Add `BurndownVM` to `worktime_vm.go`**

Append to `internal/adapter/webui/components/worktime_vm.go` (after `WeekTotalVM`):

```go
// BurndownVM drives the Monats-Burndown banner: the month total/target, the
// progress-bar fill (Pct/Variant) and a pace marker (PacePct = where one
// should stand by today). OnTrack toggles the auf-Kurs / Rückstand chip.
type BurndownVM struct {
	Total   string // "78h 00m"
	Target  string // "160h 00m"
	Pct     int    // logged fill, 0..100
	PacePct int    // pace-marker position, 0..100
	Variant string // hit|over|under|running (ProgressBar color)
	OnTrack bool
}
```

- [ ] **Step 2: Write the failing render test**

Create `internal/adapter/webui/components/burndownbanner_test.go`:

```go
package components_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

func TestBurndownBanner_OnTrack(t *testing.T) {
	out := render(t, components.BurndownBanner(components.BurndownVM{
		Total: "78h 00m", Target: "160h 00m", Pct: 48, PacePct: 47, Variant: "hit", OnTrack: true,
	}))
	for _, w := range []string{"78h 00m", "160h 00m", "role=\"progressbar\"", "left:47%", "▲"} {
		if !strings.Contains(out, w) {
			t.Errorf("BurndownBanner(on-track) missing %q\n%s", w, out)
		}
	}
}

func TestBurndownBanner_Behind(t *testing.T) {
	out := render(t, components.BurndownBanner(components.BurndownVM{
		Total: "40h 00m", Target: "160h 00m", Pct: 25, PacePct: 60, Variant: "under", OnTrack: false,
	}))
	for _, w := range []string{"left:60%", "▼"} {
		if !strings.Contains(out, w) {
			t.Errorf("BurndownBanner(behind) missing %q\n%s", w, out)
		}
	}
}
```

(The shared `render(t, c)` helper lives in `components/base_test.go`. It renders with `context.Background()`, so the test asserts only structural/locale-independent output — never translated label text.)

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/adapter/webui/components/ -run TestBurndownBanner -v`
Expected: FAIL — `undefined: components.BurndownBanner`.

- [ ] **Step 4: Create the component**

Create `internal/adapter/webui/components/burndownbanner.templ`:

```templ
package components

// BurndownBanner renders the Monats-Burndown card: "Monat gesamt {Total}",
// its target, an auf-Kurs / Rückstand chip, a ProgressBar filled to the logged
// percentage, and a thin vertical pace marker at PacePct (where one should
// stand by today). No charting library — the marker is an absolutely
// positioned <span> over the bar.
templ BurndownBanner(vm BurndownVM) {
	<article class="relative overflow-hidden rounded-3xl bg-surface border border-line shadow-lift p-6 sm:p-7">
		<div class="flex items-center justify-between gap-3 flex-wrap">
			<p class="eyebrow uppercase text-[.7rem] font-semibold text-muted">{ T(ctx, "stats.monthTotal") }</p>
			if vm.OnTrack {
				<span class="inline-flex items-center gap-1.5 rounded-full bg-green/10 text-green px-3 py-1 text-[.78rem] font-medium"><span aria-hidden="true">▲</span> { T(ctx, "stats.onTrack") }</span>
			} else {
				<span class="inline-flex items-center gap-1.5 rounded-full bg-yellow/10 text-yellow px-3 py-1 text-[.78rem] font-medium"><span aria-hidden="true">▼</span> { T(ctx, "stats.behind") }</span>
			}
		</div>
		<div class="mt-3">
			<div class="font-mono font-medium tnum text-ink leading-none text-[2.4rem] sm:text-[2.8rem]">{ vm.Total }</div>
			<p class="mt-1.5 text-[.9rem] text-muted">{ T(ctx, "week.target") } <span class="font-mono tnum text-body font-medium">{ vm.Target }</span></p>
		</div>
		<div class="relative mt-5">
			@ProgressBar(vm.Pct, vm.Variant)
			<span class="absolute -top-1 h-4 w-0.5 rounded-full bg-ink/70" style={ "left:" + itoa(vm.PacePct) + "%" } title={ T(ctx, "stats.pace") } aria-label={ T(ctx, "stats.pace") }></span>
		</div>
	</article>
}
```

(The `stats.*` i18n keys it references are added in Task 2. With `context.Background()` and a missing key, `T` returns gracefully and the render test does not assert on translated text, so this compiles and passes now.)

- [ ] **Step 5: Generate templ + run the test**

Run: `go tool templ generate && go test ./internal/adapter/webui/components/ -run TestBurndownBanner -v`
Expected: PASS.

- [ ] **Step 6: Rebuild CSS + full CI**

Run: `make web && make ci`
Expected: all green.

- [ ] **Step 7: Commit**

```bash
git add internal/adapter/webui/components/burndownbanner.templ \
        internal/adapter/webui/components/burndownbanner_templ.go \
        internal/adapter/webui/components/worktime_vm.go \
        internal/adapter/webui/components/burndownbanner_test.go \
        internal/adapter/webui/static/app.css
git commit -m "feat(webui): BurndownBanner component with pace marker (Slice 2 Stats)"
```

---

## Task 2: Stats page rewrite (saldo tiles + burndown banner + target form)

Rewrite `stats.templ` onto the AppShell with a new `StatsVM`: three saldo tiles (Heute · Woche · Monat), the `BurndownBanner`, and the Tagesziel config form (default + five Mon–Fri override inputs). Rewrite `statsData` + the home/fragment handlers, add the pure `burndownBannerVM` helper (with its unit test), add the i18n keys, and clean up the now-dead old helpers. The `POST /ui/stats/target` handler is **left functionally unchanged** here (it still compiles and passes its tests) — Task 3 extends it.

**Files:**
- Modify: `internal/adapter/webui/stats.templ` (full rewrite)
- Create: `internal/adapter/webui/stats_vm.go`
- Modify: `internal/adapter/webui/format.go` (delete `fmtInt`, `monthBarStyle`, `monthBarPct`, `weekBarStyle`)
- Modify: `internal/adapter/webui/render_test.go` (delete two clamping tests)
- Modify: `internal/adapter/httpserver/webui_stats.go` (rewrite `statsData`/handlers, add `burndownBannerVM`, delete `fmtMin`/`fmtSaldo`, update the `handleWebSetTarget` tail call)
- Modify: `internal/i18n/catalog_de.go` + `internal/i18n/catalog_en.go`
- Test: `internal/adapter/httpserver/webui_stats_internal_test.go` (new file, for `burndownBannerVM`)

**Interfaces:**
- Consumes (from Task 1): `components.BurndownVM`, `components.BurndownBanner`. From existing code: `s.Stats.Today(ctx, ownerID) (usecase.TodaySummary{Logged,Target,Saldo}, error)`, `s.Stats.Burndown(ctx, ownerID) (domain.MonthBurndownReport{Total,Target,Saldo,OnTrack}, error)`, `s.Stats.Week(ctx, ownerID, time.Time{}) ([]domain.WeekDay, error)` (each `WeekDay` has `.Date time.Time`, `.Target time.Duration`, `.Total(now) time.Duration`), `s.GetSettings.Execute(ctx, ownerID) (domain.Settings{DefaultTargetMin int, WeekdayTargetMin map[time.Weekday]int}, []domain.FeedToken, error)`, `webui.FmtVerbose`/`webui.FmtSaldoVerbose`, `clampPct(int) int` (stays in `webui_stats.go`).
- Produces: `webui.StatsVM`, `webui.StatsWeekdayVMs(map[time.Weekday]int) []webui.WeekdayTargetVM`, `templ webui.StatsPage(StatsVM)` / `webui.StatsFragment(StatsVM)`, and `httpserver.burndownBannerVM(domain.MonthBurndownReport) components.BurndownVM`. Consumed by Task 3 (which reuses `StatsVM`/`StatsWeekdayVMs` and `renderStatsFragment`).

- [ ] **Step 1: Create `stats_vm.go`**

Create `internal/adapter/webui/stats_vm.go`:

```go
package webui

import (
	"strconv"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

// StatsVM is the view model for the Stats page on the Slice-0 AppShell: three
// saldo tiles (Heute/Woche/Monat), the month burndown banner, and the daily
// target config (default + Mon–Fri overrides).
type StatsVM struct {
	TodaySaldo string // "+2h 18m" / "−1h 05m"
	TodayPos   bool
	TodaySub   string // "5h 12m / 8h 00m"
	WeekSaldo  string
	WeekPos    bool
	WeekSub    string
	MonthSaldo string
	MonthPos   bool
	MonthSub   string

	Burndown components.BurndownVM

	DefaultTarget string // minutes as string for the form input
	Weekdays      []WeekdayTargetVM
	Err           string
}

// WeekdayTargetVM is one Mon–Fri override input.
type WeekdayTargetVM struct {
	Label string // "Mo"
	Name  string // form field name: mon|tue|wed|thu|fri
	Value string // minutes; "" → inherits the default
}

// statsSaldoHue colors a saldo value green when ahead, red when behind.
func statsSaldoHue(pos bool) string {
	if pos {
		return "text-green"
	}
	return "text-red"
}

// StatsWeekdayVMs builds the five Mon–Fri override inputs from the stored
// per-weekday target map. An absent weekday → empty value (inherits default).
func StatsWeekdayVMs(weekday map[time.Weekday]int) []WeekdayTargetVM {
	defs := []struct {
		label, name string
		wd          time.Weekday
	}{
		{"Mo", "mon", time.Monday},
		{"Di", "tue", time.Tuesday},
		{"Mi", "wed", time.Wednesday},
		{"Do", "thu", time.Thursday},
		{"Fr", "fri", time.Friday},
	}
	out := make([]WeekdayTargetVM, 0, len(defs))
	for _, d := range defs {
		v := ""
		if m, ok := weekday[d.wd]; ok {
			v = strconv.Itoa(m)
		}
		out = append(out, WeekdayTargetVM{Label: d.label, Name: d.name, Value: v})
	}
	return out
}
```

- [ ] **Step 2: Rewrite `stats.templ`**

Replace the entire contents of `internal/adapter/webui/stats.templ` with:

```templ
package webui

import "github.com/serverkraken/flow/internal/adapter/webui/components"

// StatsPage is the full Stats page on the Slice-0 AppShell. Worktime sub-tabs
// share the "Heute" top-tab, so AppShell active stays "today" and the worktime
// sub-tab strip marks "stats".
templ StatsPage(vm StatsVM) {
	@components.Base("stats", statsBody(vm))
}

templ statsBody(vm StatsVM) {
	@components.AppShell("today", nil, worktimeSubnav("stats"), statsOuter(vm))
}

// statsOuter is the SSE-swap container: it reloads StatsFragment on any
// worktime / settings / day-off mutation (holidays change targets).
templ statsOuter(vm StatsVM) {
	<div id="content"
		hx-get="/ui/stats/fragment"
		hx-trigger="sse:session.started, sse:session.stopped, sse:session.updated, sse:session.deleted, sse:settings.changed, sse:dayoff.changed"
		hx-swap="innerHTML">
		@StatsFragment(vm)
	</div>
}

// StatsFragment is the inner content re-rendered live and after every mutation.
templ StatsFragment(vm StatsVM) {
	<div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between mb-5 md:mb-6">
		<div>
			<p class="eyebrow uppercase text-[.72rem] font-semibold text-blue mb-1">{ components.T(ctx, "stats.eyebrow") }</p>
			<h1 class="font-display text-[2rem] sm:text-[2.5rem] font-semibold leading-none tracking-tight">{ components.T(ctx, "nav.stats") }</h1>
		</div>
	</div>
	if vm.Err != "" {
		<p class="mb-5 rounded-2xl bg-red/10 px-4 py-3 text-[.9rem] font-medium text-red" role="alert">{ vm.Err }</p>
	}
	<section class="grid grid-cols-1 sm:grid-cols-3 gap-4 md:gap-5">
		@statsSaldoTile("stats.tileToday", vm.TodaySaldo, vm.TodayPos, vm.TodaySub)
		@statsSaldoTile("stats.tileWeek", vm.WeekSaldo, vm.WeekPos, vm.WeekSub)
		@statsSaldoTile("stats.tileMonth", vm.MonthSaldo, vm.MonthPos, vm.MonthSub)
	</section>
	<section class="mt-5 md:mt-6">
		@components.BurndownBanner(vm.Burndown)
	</section>
	<section class="mt-5 md:mt-6">
		@statsTargetCard(vm)
	</section>
}

// statsSaldoTile is one saldo tile: label · big signed saldo · geloggt/Ziel sub.
templ statsSaldoTile(labelKey, saldo string, pos bool, sub string) {
	<div class="rounded-3xl bg-surface border border-line shadow-soft p-5 sm:p-6 text-center">
		<div class="eyebrow uppercase text-[.66rem] font-semibold text-muted">{ components.T(ctx, labelKey) }</div>
		<div class={ "mt-1.5 font-mono text-2xl sm:text-[1.7rem] font-semibold tnum", statsSaldoHue(pos) }>{ saldo }</div>
		<div class="mt-1 text-[.8rem] text-muted font-mono tnum">{ sub }</div>
	</div>
}

// statsTargetCard is the Tagesziel config form: a default daily target plus five
// optional Mon–Fri overrides. Posts to POST /ui/stats/target (Task 3 makes the
// form the authoritative source of both values).
templ statsTargetCard(vm StatsVM) {
	<article class="rounded-3xl bg-surface border border-line shadow-soft p-6">
		<h2 class="font-display text-lg font-semibold mb-4">{ components.T(ctx, "stats.targetConfig") }</h2>
		<form hx-post="/ui/stats/target" hx-target="#content" hx-swap="innerHTML" class="space-y-5">
			<label class="block">
				<span class="eyebrow uppercase text-[.68rem] font-semibold text-muted">{ components.T(ctx, "stats.defaultTarget") }</span>
				<input type="number" name="defaultTargetMin" value={ vm.DefaultTarget } min="0" max="1440" inputmode="numeric"
					class="mt-1 w-32 rounded-lg border border-line bg-sunken/60 px-3 py-2 text-[.9rem] font-mono tnum focus:border-blue/40 transition-colors"/>
			</label>
			<div>
				<p class="eyebrow uppercase text-[.68rem] font-semibold text-muted mb-2">{ components.T(ctx, "stats.weekdayOverrides") } <span class="text-faint normal-case font-normal">· { components.T(ctx, "stats.weekdayHint") }</span></p>
				<div class="grid grid-cols-2 sm:grid-cols-5 gap-3">
					for _, wd := range vm.Weekdays {
						<label class="block">
							<span class="text-[.74rem] font-semibold text-body">{ wd.Label }</span>
							<input type="number" name={ wd.Name } value={ wd.Value } min="0" max="1440" inputmode="numeric"
								class="mt-1 w-full rounded-lg border border-line bg-sunken/60 px-3 py-2 text-[.85rem] font-mono tnum focus:border-blue/40 transition-colors"/>
						</label>
					}
				</div>
			</div>
			<div class="flex justify-end">
				@components.Button(components.BtnPrimary, components.T(ctx, "common.save"), "✓", templ.Attributes{"type": "submit"})
			</div>
		</form>
	</article>
}
```

- [ ] **Step 3: Add the i18n keys (both catalogs)**

In `internal/i18n/catalog_de.go`, add to the `strings` map (group near the existing `week.*` keys):

```go
"stats.eyebrow":          "Auswertung",
"stats.tileToday":        "Heute",
"stats.tileWeek":         "Woche",
"stats.tileMonth":        "Monat",
"stats.monthTotal":       "Monat gesamt",
"stats.onTrack":          "auf Kurs",
"stats.behind":           "Rückstand",
"stats.pace":             "Soll-Stand heute",
"stats.targetConfig":     "Tagesziel",
"stats.defaultTarget":    "Standard-Tagesziel (Min.)",
"stats.weekdayOverrides": "Pro Wochentag (optional)",
"stats.weekdayHint":      "Leer = Standard",
"stats.invalidTarget":    "Ungültiges Ziel",
```

In `internal/i18n/catalog_en.go`, add the same keys with English stubs:

```go
"stats.eyebrow":          "Analysis",
"stats.tileToday":        "Today",
"stats.tileWeek":         "Week",
"stats.tileMonth":        "Month",
"stats.monthTotal":       "Month total",
"stats.onTrack":          "on track",
"stats.behind":           "behind",
"stats.pace":             "Target by today",
"stats.targetConfig":     "Daily target",
"stats.defaultTarget":    "Default daily target (min)",
"stats.weekdayOverrides": "Per weekday (optional)",
"stats.weekdayHint":      "Empty = default",
"stats.invalidTarget":    "Invalid target",
```

- [ ] **Step 4: Rewrite `statsData` + handlers + add `burndownBannerVM`; delete dead helpers**

In `internal/adapter/httpserver/webui_stats.go`:

(a) **Delete** `fmtMin` and `fmtSaldo` (now unused — they would trip staticcheck U1000). **Keep** `clampPct`.

(b) Replace `statsData`, `renderStatsFragment`, `handleWebStatsHome`, `handleWebStatsFragment` with:

```go
// burndownBannerVM maps a month burndown report onto the banner VM. The pace
// marker sits at expected-by-now / Target; expected-by-now = Total − Saldo
// (Saldo is defined as Total − expected). Pct is the logged fill. Both clamp
// to [0,100]; a zero target leaves both at 0.
func burndownBannerVM(rep domain.MonthBurndownReport) components.BurndownVM {
	pct, pace := 0, 0
	if rep.Target > 0 {
		pct = clampPct(int(rep.Total * 100 / rep.Target))
		expected := rep.Total - rep.Saldo
		pace = clampPct(int(expected * 100 / rep.Target))
	}
	variant := "under"
	if rep.OnTrack {
		variant = "hit"
	}
	return components.BurndownVM{
		Total:   webui.FmtVerbose(rep.Total),
		Target:  webui.FmtVerbose(rep.Target),
		Pct:     pct,
		PacePct: pace,
		Variant: variant,
		OnTrack: rep.OnTrack,
	}
}

func (s *Server) statsData(ctx context.Context, u domain.User) (webui.StatsVM, error) {
	now := s.Clock.Now()

	today, err := s.Stats.Today(ctx, u.ID)
	if err != nil {
		return webui.StatsVM{}, err
	}
	burndown, err := s.Stats.Burndown(ctx, u.ID)
	if err != nil {
		return webui.StatsVM{}, err
	}
	weekDays, err := s.Stats.Week(ctx, u.ID, time.Time{})
	if err != nil {
		return webui.StatsVM{}, err
	}
	set, _, err := s.GetSettings.Execute(ctx, u.ID)
	if err != nil {
		return webui.StatsVM{}, err
	}

	// Week saldo = Mon–Fri logged − target (weekends excluded, matching Woche).
	var weekLogged, weekTarget time.Duration
	for _, wd := range weekDays {
		if wd.Date.Weekday() == time.Saturday || wd.Date.Weekday() == time.Sunday {
			continue
		}
		weekLogged += wd.Total(now)
		weekTarget += wd.Target
	}
	weekSaldo := weekLogged - weekTarget

	return webui.StatsVM{
		TodaySaldo:    webui.FmtSaldoVerbose(today.Saldo),
		TodayPos:      today.Saldo >= 0,
		TodaySub:      webui.FmtVerbose(today.Logged) + " / " + webui.FmtVerbose(today.Target),
		WeekSaldo:     webui.FmtSaldoVerbose(weekSaldo),
		WeekPos:       weekSaldo >= 0,
		WeekSub:       webui.FmtVerbose(weekLogged) + " / " + webui.FmtVerbose(weekTarget),
		MonthSaldo:    webui.FmtSaldoVerbose(burndown.Saldo),
		MonthPos:      burndown.OnTrack,
		MonthSub:      webui.FmtVerbose(burndown.Total) + " / " + webui.FmtVerbose(burndown.Target),
		Burndown:      burndownBannerVM(burndown),
		DefaultTarget: strconv.Itoa(set.DefaultTargetMin),
		Weekdays:      webui.StatsWeekdayVMs(set.WeekdayTargetMin),
	}, nil
}

func (s *Server) renderStatsFragment(w http.ResponseWriter, r *http.Request, u domain.User) {
	vm, err := s.statsData(r.Context(), u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.StatsFragment(vm).Render(r.Context(), w)
}

func (s *Server) handleWebStatsHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	vm, err := s.statsData(r.Context(), u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.StatsPage(vm).Render(r.Context(), w)
}

func (s *Server) handleWebStatsFragment(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	s.renderStatsFragment(w, r, u)
}
```

(c) In the **existing** `handleWebSetTarget`, change the tail (its body is otherwise unchanged in this task) — drop the range read and call `renderStatsFragment` with the new signature:

```go
	s.Bus.Publish(domain.Event{Type: domain.EventSettingsChanged, UserID: u.ID})
	s.renderStatsFragment(w, r, u)
```

(d) Fix the imports: this file now needs `"github.com/serverkraken/flow/internal/adapter/webui/components"` (for `burndownBannerVM`'s return type). Remove `"fmt"` if it is no longer used after deleting `fmtMin`/`fmtSaldo` (let `gofumpt`/`make ci` tell you). `RangeStats` is no longer called from here — that usecase stays for the REST `/api/v1/stats` path; just stop calling it.

- [ ] **Step 5: Delete the stats-only helpers in `format.go` + their tests**

In `internal/adapter/webui/format.go`, delete `fmtInt`, `monthBarStyle`, `monthBarPct`, `weekBarStyle`. **Keep** `fmtDur` (still referenced by `TestFmtDur`). The `fmt` and `time` imports remain (used by `fmtDur`).

In `internal/adapter/webui/render_test.go`, delete `TestMonthBarPct_Clamping` and `TestWeekBarStyle_Clamping` (they reference the deleted helpers + the removed `StatsData`/`StatsWeekRow` types). Keep `TestFmtDur`, `TestWeekDay_Total_ActivePath`, `TestWeekDay_Total_ActiveBeforeMidnight`.

- [ ] **Step 6: Write the failing unit test for `burndownBannerVM`**

Create `internal/adapter/httpserver/webui_stats_internal_test.go`:

```go
package httpserver

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestBurndownBannerVM_PaceMath(t *testing.T) {
	rep := domain.MonthBurndownReport{
		Total:   78 * time.Hour,
		Target:  160 * time.Hour,
		Saldo:   2 * time.Hour, // expected-by-now = 78 − 2 = 76h
		OnTrack: true,
	}
	vm := burndownBannerVM(rep)
	if vm.Pct != 48 { // 78/160 = 48.75 → int 48
		t.Errorf("Pct: want 48, got %d", vm.Pct)
	}
	if vm.PacePct != 47 { // 76/160 = 47.5 → int 47
		t.Errorf("PacePct: want 47, got %d", vm.PacePct)
	}
	if vm.Variant != "hit" {
		t.Errorf("Variant: want hit, got %q", vm.Variant)
	}
}

func TestBurndownBannerVM_Behind_ZeroTarget(t *testing.T) {
	behind := burndownBannerVM(domain.MonthBurndownReport{
		Total: 40 * time.Hour, Target: 160 * time.Hour, Saldo: -36 * time.Hour, OnTrack: false,
	})
	if behind.Variant != "under" {
		t.Errorf("Variant: want under, got %q", behind.Variant)
	}
	if behind.PacePct != 47 { // expected = 40 − (−36) = 76h → 47
		t.Errorf("PacePct: want 47, got %d", behind.PacePct)
	}
	zero := burndownBannerVM(domain.MonthBurndownReport{Total: 10 * time.Hour, Target: 0})
	if zero.Pct != 0 || zero.PacePct != 0 {
		t.Errorf("zero target: want 0/0, got %d/%d", zero.Pct, zero.PacePct)
	}
}
```

- [ ] **Step 7: Generate, build, test**

Run: `go tool templ generate && go test ./internal/adapter/webui/... ./internal/adapter/httpserver/... -v`
Expected: PASS. The pre-existing handler tests (`TestWebStatsHome` expects `flow · stats` + `Woche`; `TestWebStatsFragment`/`_MonthRange` expect `Monat`; `_WithOvertime` expects `+`) still pass — the page always renders the Woche + Monat saldo tiles and a signed saldo. The two `TestWebSetTarget*` (preserve-map) tests still pass because `handleWebSetTarget` is unchanged here.

- [ ] **Step 8: Rebuild CSS + full CI**

Run: `make web && make ci`
Expected: all green (coverage ≥75%).

- [ ] **Step 9: Commit**

```bash
git add internal/adapter/webui/stats.templ internal/adapter/webui/stats_templ.go \
        internal/adapter/webui/stats_vm.go internal/adapter/webui/format.go \
        internal/adapter/webui/render_test.go \
        internal/adapter/httpserver/webui_stats.go \
        internal/adapter/httpserver/webui_stats_internal_test.go \
        internal/i18n/catalog_de.go internal/i18n/catalog_en.go \
        internal/adapter/webui/static/app.css
git commit -m "feat(webui): rewrite Stats page onto AppShell (saldo tiles + burndown banner + target form)"
```

---

## Task 3: Extend `handleWebSetTarget` for per-weekday targets

Make the target form the authoritative source of **both** the default and the five Mon–Fri overrides. Empty weekday inputs omit that key (inherit the default). Add the pure, unit-testable `parseWeekdayTargets` mapping, and rewrite the two existing SetTarget handler tests that encoded the old "preserve the untouched map" semantics — which this task deliberately overturns (per the spec: "the form is the source of truth for both").

**Files:**
- Modify: `internal/adapter/httpserver/webui_stats.go` (rewrite `handleWebSetTarget`, add `parseWeekdayTargets`)
- Modify: `internal/adapter/httpserver/webui_stats_internal_test.go` (add `parseWeekdayTargets` unit tests)
- Modify: `internal/adapter/httpserver/webui_stats_test.go` (rewrite `TestWebSetTarget` + `TestWebSetTargetWithWeekdayOverrides` for authoritative semantics)

**Interfaces:**
- Consumes: `s.SetTarget.Execute(ctx, ownerID string, defaultMin int, weekday map[time.Weekday]int) error` (returns `domain.ErrInvalidTarget` on a negative value), `s.GetSettings`, `renderStatsFragment` (from Task 2), `clampPct` (unused here).
- Produces: `parseWeekdayTargets(form url.Values) (map[time.Weekday]int, error)` returning `domain.ErrInvalidTarget` on a non-numeric/negative weekday value.

- [ ] **Step 1: Write the failing unit test for `parseWeekdayTargets`**

Append to `internal/adapter/httpserver/webui_stats_internal_test.go`:

```go
func TestParseWeekdayTargets(t *testing.T) {
	form := url.Values{
		"mon": {"480"},
		"tue": {""}, // empty → omitted (inherit default)
		"fri": {"240"},
	}
	got, err := parseWeekdayTargets(form)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 keys, got %d (%v)", len(got), got)
	}
	if got[time.Monday] != 480 || got[time.Friday] != 240 {
		t.Errorf("want Mon=480 Fri=240, got %v", got)
	}
	if _, ok := got[time.Tuesday]; ok {
		t.Errorf("empty Tuesday should be omitted, got %v", got)
	}
}

func TestParseWeekdayTargets_Invalid(t *testing.T) {
	if _, err := parseWeekdayTargets(url.Values{"wed": {"-5"}}); err == nil {
		t.Error("negative value should error")
	}
	if _, err := parseWeekdayTargets(url.Values{"thu": {"abc"}}); err == nil {
		t.Error("non-numeric value should error")
	}
}
```

Add `"net/url"` to that test file's imports.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/adapter/httpserver/ -run TestParseWeekdayTargets -v`
Expected: FAIL — `undefined: parseWeekdayTargets`.

- [ ] **Step 3: Implement `parseWeekdayTargets` + rewrite `handleWebSetTarget`**

In `internal/adapter/httpserver/webui_stats.go`, add the helper and rewrite the handler:

```go
// parseWeekdayTargets reads the five optional Mon–Fri target inputs. An empty
// input omits that weekday (inherit the default); a non-numeric or negative
// value is rejected with domain.ErrInvalidTarget.
func parseWeekdayTargets(form url.Values) (map[time.Weekday]int, error) {
	fields := []struct {
		name string
		wd   time.Weekday
	}{
		{"mon", time.Monday},
		{"tue", time.Tuesday},
		{"wed", time.Wednesday},
		{"thu", time.Thursday},
		{"fri", time.Friday},
	}
	out := make(map[time.Weekday]int, len(fields))
	for _, f := range fields {
		v := strings.TrimSpace(form.Get(f.name))
		if v == "" {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return nil, domain.ErrInvalidTarget
		}
		out[f.wd] = n
	}
	return out, nil
}

func (s *Server) handleWebSetTarget(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	defaultMin, err := strconv.Atoi(r.FormValue("defaultTargetMin"))
	if err != nil || defaultMin < 0 {
		http.Error(w, "invalid defaultTargetMin", http.StatusBadRequest)
		return
	}
	weekday, err := parseWeekdayTargets(r.Form)
	if err != nil {
		http.Error(w, "invalid weekday target", http.StatusBadRequest)
		return
	}
	// The form is now the authoritative source of BOTH the default and the
	// per-weekday overrides (empty inputs omit a weekday → inherit the default).
	if err := s.SetTarget.Execute(r.Context(), u.ID, defaultMin, weekday); err != nil {
		if errors.Is(err, domain.ErrInvalidTarget) {
			http.Error(w, "invalid target", http.StatusBadRequest)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventSettingsChanged, UserID: u.ID})
	s.renderStatsFragment(w, r, u)
}
```

Add the imports this needs: `"errors"`, `"net/url"`, `"strings"` (alongside the existing `"strconv"`, `"time"`). The old `GetSettings` load to "preserve" the map is gone (the form is authoritative now).

- [ ] **Step 4: Rewrite the two SetTarget handler tests for authoritative semantics**

In `internal/adapter/httpserver/webui_stats_test.go`, replace `TestWebSetTarget` and `TestWebSetTargetWithWeekdayOverrides` with tests that POST the full form and assert the form is authoritative:

```go
func TestWebSetTarget(t *testing.T) {
	srv, codec, settingsStore := newWebStatsServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")
	ctx := context.Background()

	// POST default + a Friday override → both persist exactly as posted.
	form := url.Values{
		"defaultTargetMin": {"360"},
		"fri":              {"300"},
	}.Encode()
	req, _ := http.NewRequest("POST", ts.URL+"/ui/stats/target", strings.NewReader(form))
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	body := string(b)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /ui/stats/target status=%d body=%.200s", res.StatusCode, body)
	}
	if !strings.Contains(body, "Heute") { // fragment marker (saldo tile label)
		t.Fatalf("expected 'Heute' (fragment marker) in body, got: %.200s", body)
	}
	stored, err := settingsStore.Get(ctx, "u1")
	if err != nil {
		t.Fatalf("reading stored settings: %v", err)
	}
	if stored.DefaultTargetMin != 360 {
		t.Errorf("want DefaultTargetMin=360, got %d", stored.DefaultTargetMin)
	}
	if v, ok := stored.WeekdayTargetMin[time.Friday]; !ok || v != 300 {
		t.Errorf("Friday override should be 300, got map=%v", stored.WeekdayTargetMin)
	}
}

func TestWebSetTarget_EmptyWeekdayClearsOverride(t *testing.T) {
	srv, codec, settingsStore := newWebStatsServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")
	ctx := context.Background()

	// Seed a Friday override, then POST WITHOUT any weekday field → the form is
	// authoritative, so the override is cleared (empty = inherit default).
	_ = settingsStore.SetTargetConfig(ctx, "u1", 480, map[time.Weekday]int{time.Friday: 240})

	form := url.Values{"defaultTargetMin": {"420"}}.Encode()
	req, _ := http.NewRequest("POST", ts.URL+"/ui/stats/target", strings.NewReader(form))
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", res.StatusCode)
	}
	stored, _ := settingsStore.Get(ctx, "u1")
	if stored.DefaultTargetMin != 420 {
		t.Errorf("want default 420, got %d", stored.DefaultTargetMin)
	}
	if _, ok := stored.WeekdayTargetMin[time.Friday]; ok {
		t.Errorf("Friday override should be cleared, got map=%v", stored.WeekdayTargetMin)
	}
}

func TestWebSetTarget_InvalidWeekday(t *testing.T) {
	srv, codec, _ := newWebStatsServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	form := url.Values{"defaultTargetMin": {"480"}, "mon": {"-5"}}.Encode()
	req, _ := http.NewRequest("POST", ts.URL+"/ui/stats/target", strings.NewReader(form))
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for negative weekday, got %d", res.StatusCode)
	}
}
```

(`TestWebSetTarget_InvalidDefaultMin` in the same file stays as-is — still valid. `TestWebStatsSetTarget_HappyPath`/`_InvalidInput` in `webui_coverage_test.go` POST only `defaultTargetMin` and still pass.)

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/adapter/httpserver/ -run 'TestParseWeekdayTargets|TestWebSetTarget' -v`
Expected: PASS.

- [ ] **Step 6: Full CI**

Run: `make ci`
Expected: all green. (No new templ/CSS in this task; no `make web` needed.)

- [ ] **Step 7: Commit**

```bash
git add internal/adapter/httpserver/webui_stats.go \
        internal/adapter/httpserver/webui_stats_internal_test.go \
        internal/adapter/httpserver/webui_stats_test.go
git commit -m "feat(webui): per-weekday targets — POST /ui/stats/target now authoritative for default + Mo–Fr"
```

---

# SLICE 2 — Frei page

## Task 4: Frei page (add / list+holidays / delete / ICS / bundesland select)

Create `frei.templ` + `frei_vm.go`, rewrite the day-off handlers onto `FreiVM`, add the German-state option list, and the tiny clipboard helper. Delete the old `dayoffs.templ`. The bundesland `<select>` posts to `POST /ui/dayoffs/bundesland` — that route + handler land in Task 5 (the select renders here; its POST is wired next).

**Files:**
- Create: `internal/adapter/webui/frei.templ`
- Create: `internal/adapter/webui/frei_vm.go`
- Create: `internal/adapter/webui/static/js/clipboard.js`
- Modify: `internal/adapter/httpserver/webui_dayoffs.go` (rewrite `dayOffData` + render/home/fragment; add `bundeslandOptions`, `bundeslandName`)
- Modify: `internal/i18n/catalog_de.go` + `internal/i18n/catalog_en.go`
- Modify: `internal/adapter/httpserver/webui_dayoffs_test.go` (update assertions for the new markup)
- Delete: `internal/adapter/webui/dayoffs.templ` + `internal/adapter/webui/dayoffs_templ.go`

**Interfaces:**
- Consumes: `s.ListDayOffs.Execute(ctx, ownerID, from, to time.Time) ([]domain.DayOff, error)` (already merges computed holidays; each `DayOff` has `.Date time.Time`, `.Kind domain.Kind`, `.Label string`), `s.GetSettings.Execute(...) (domain.Settings{Bundesland}, []domain.FeedToken, error)`, `dayOffHue(domain.Kind) string` (already in `webui_woche.go`), `domain.Kind.LabelDe()`, `domain.ValidBundesland(s) (string, bool)`, `firstFeedURL([]domain.FeedToken) string` (already in `webui_dayoffs.go`), `components.ConfirmDialog`, `components.EmptyState`, `components.Button`, `components.Tn`.
- Produces: `webui.FreiVM`, `templ webui.FreiPage(FreiVM)` / `webui.FreiFragment(FreiVM)`, and the unchanged add/delete/regen handlers now render `FreiFragment`. Consumed by Task 5 (`handleWebSetBundesland` calls `renderDayOffFragment`).

- [ ] **Step 1: Create `frei_vm.go`**

Create `internal/adapter/webui/frei_vm.go`:

```go
package webui

import (
	"fmt"

	"github.com/serverkraken/flow/internal/domain"
)

// FreiVM is the view model for the Frei page: a day-off capture form, the
// own-entries + holidays list for the year, and the settings card (ICS feed +
// Bundesland).
type FreiVM struct {
	User              string
	BundeslandCode    string // "NW" (drives the <select> selected option)
	BundeslandName    string // "Nordrhein-Westfalen" (list header)
	BundeslandOptions []FreiBundeslandOption
	Year              string
	IcsURL            string
	Rows              []FreiRowVM
	HasOwn            bool // at least one non-holiday entry (drives EmptyState)
	Err               string
}

// FreiBundeslandOption is one entry in the Bundesland <select>.
type FreiBundeslandOption struct{ Code, Name string }

// FreiRowVM is one row in the year list (own day-off or read-only holiday).
type FreiRowVM struct {
	DateLabel string // "15.06.2026"
	KindLabel string // domain.Kind.LabelDe()
	Hue       string // dayOffHue(kind)
	Label     string
	IsHoliday bool
	Day       string // yyyy-mm-dd for the delete form
}

// FreiKindOption is one selectable kind in the capture form.
type FreiKindOption struct {
	Value domain.Kind
	Label string
}

// freiKinds lists the six user-creatable day-off kinds (holiday is computed,
// never created here), each with its German label from the domain.
func freiKinds() []FreiKindOption {
	kinds := []domain.Kind{
		domain.KindVacation, domain.KindSick, domain.KindFlex,
		domain.KindSpecial, domain.KindChildSick, domain.KindTraining,
	}
	out := make([]FreiKindOption, 0, len(kinds))
	for _, k := range kinds {
		out = append(out, FreiKindOption{Value: k, Label: k.LabelDe()})
	}
	return out
}

// freiKindChip maps a day-off hue to its pill wash+text utility (whitelisted),
// mirroring the Woche day-off chip look.
func freiKindChip(hue string) string {
	switch hue {
	case "blue", "cyan", "green", "purple", "magenta", "yellow", "orange", "red", "teal":
		return "bg-" + hue + "/10 text-" + hue
	default:
		return "bg-blue/10 text-blue"
	}
}

// freiDeleteVals builds the hx-vals JSON for a day-off delete confirm action.
func freiDeleteVals(day string) string {
	return fmt.Sprintf(`{"day":%q}`, day)
}
```

- [ ] **Step 2: Create `frei.templ`**

Create `internal/adapter/webui/frei.templ`:

```templ
package webui

import "github.com/serverkraken/flow/internal/adapter/webui/components"

// FreiPage is the full Frei page on the Slice-0 AppShell. Worktime sub-tabs
// share the "Heute" top-tab, so AppShell active stays "today" and the worktime
// sub-tab strip marks "frei".
templ FreiPage(vm FreiVM) {
	@components.Base("frei", freiBody(vm))
}

templ freiBody(vm FreiVM) {
	@components.AppShell("today", nil, worktimeSubnav("frei"), freiOuter(vm))
}

// freiOuter is the SSE-swap container: it reloads FreiFragment on any day-off
// or settings mutation (Bundesland changes recompute holidays).
templ freiOuter(vm FreiVM) {
	<div id="content"
		hx-get="/ui/dayoffs"
		hx-trigger="sse:dayoff.changed, sse:settings.changed"
		hx-swap="innerHTML">
		@FreiFragment(vm)
	</div>
}

// FreiFragment is the inner content re-rendered live and after every mutation.
templ FreiFragment(vm FreiVM) {
	<div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between mb-5 md:mb-6">
		<div>
			<p class="eyebrow uppercase text-[.72rem] font-semibold text-blue mb-1">{ components.T(ctx, "frei.eyebrow") }</p>
			<h1 class="font-display text-[2rem] sm:text-[2.5rem] font-semibold leading-none tracking-tight">{ components.T(ctx, "nav.dayoffs") }</h1>
		</div>
	</div>
	if vm.Err != "" {
		<p class="mb-5 rounded-2xl bg-red/10 px-4 py-3 text-[.9rem] font-medium text-red" role="alert">{ vm.Err }</p>
	}
	<section class="grid grid-cols-1 lg:grid-cols-3 gap-5 md:gap-6">
		<div class="lg:col-span-2 flex flex-col gap-5 md:gap-6">
			@freiAddCard(vm)
			@freiListCard(vm)
		</div>
		@freiSettingsCard(vm)
	</section>
}

// freiAddCard captures a new day-off (von–bis, kind, optional label, skip-weekends).
templ freiAddCard(vm FreiVM) {
	<article class="rounded-3xl bg-surface border border-line shadow-soft p-6">
		<h2 class="font-display text-lg font-semibold mb-4">{ components.T(ctx, "frei.addTitle") }</h2>
		<form hx-post="/ui/dayoffs/add" hx-target="#content" hx-swap="innerHTML" class="space-y-4">
			<div class="grid grid-cols-2 gap-3">
				<label class="block">
					<span class="eyebrow uppercase text-[.68rem] font-semibold text-muted">{ components.T(ctx, "frei.from") }</span>
					<input type="date" name="from" required class="mt-1 w-full rounded-lg border border-line bg-sunken/60 px-3 py-2 text-[.9rem] focus:border-blue/40 transition-colors"/>
				</label>
				<label class="block">
					<span class="eyebrow uppercase text-[.68rem] font-semibold text-muted">{ components.T(ctx, "frei.to") }</span>
					<input type="date" name="to" required class="mt-1 w-full rounded-lg border border-line bg-sunken/60 px-3 py-2 text-[.9rem] focus:border-blue/40 transition-colors"/>
				</label>
			</div>
			<label class="block">
				<span class="eyebrow uppercase text-[.68rem] font-semibold text-muted">{ components.T(ctx, "frei.kind") }</span>
				<select name="kind" class="mt-1 w-full rounded-lg border border-line bg-sunken/60 px-3 py-2 text-[.9rem] focus:border-blue/40 transition-colors">
					for _, k := range freiKinds() {
						<option value={ string(k.Value) }>{ k.Label }</option>
					}
				</select>
			</label>
			<label class="block">
				<span class="eyebrow uppercase text-[.68rem] font-semibold text-muted">{ components.T(ctx, "frei.label") }</span>
				<input name="label" class="mt-1 w-full rounded-lg border border-line bg-sunken/60 px-3 py-2 text-[.9rem] focus:border-blue/40 transition-colors"/>
			</label>
			<label class="flex items-center gap-2 text-[.86rem] text-body">
				<input type="checkbox" name="skipWeekends" value="true" checked/> { components.T(ctx, "frei.skipWeekends") }
			</label>
			<div class="flex justify-end">
				@components.Button(components.BtnPrimary, components.T(ctx, "frei.addButton"), "✚", templ.Attributes{"type": "submit"})
			</div>
		</form>
	</article>
}

// freiListCard lists own day-offs + computed holidays for the year, sorted by date.
templ freiListCard(vm FreiVM) {
	<article class="rounded-3xl bg-surface border border-line shadow-soft p-6">
		<div class="flex items-center justify-between mb-4">
			<h2 class="font-display text-lg font-semibold">{ vm.BundeslandName } · { vm.Year }</h2>
			<span class="text-[.8rem] text-muted">{ components.Tn(ctx, "list.entries", len(vm.Rows)) }</span>
		</div>
		if !vm.HasOwn {
			@components.EmptyState("○", "frei.emptyTitle", "frei.empty")
		}
		if len(vm.Rows) > 0 {
			<ul class="divide-y divide-line2">
				for _, row := range vm.Rows {
					@freiRow(row)
				}
			</ul>
		}
	</article>
}

// freiRow is one list row. Own entries carry a Löschen control → ConfirmDialog;
// holidays are read-only (dimmed, no delete).
templ freiRow(row FreiRowVM) {
	<li class={ "flex items-center gap-3 py-2.5", templ.KV("opacity-60", row.IsHoliday) }>
		<span class="w-24 shrink-0 font-mono text-[.82rem] tnum text-muted">{ row.DateLabel }</span>
		<span class={ "inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[.74rem] font-medium", freiKindChip(row.Hue) }>
			<span aria-hidden="true">○</span> { row.KindLabel }
		</span>
		<span class="flex-1 min-w-0 truncate text-[.88rem] text-body">{ row.Label }</span>
		if !row.IsHoliday {
			<button type="button" aria-label={ components.T(ctx, "common.delete") }
				data-dialog-open={ "frei-del-" + row.Day }
				class="grid place-items-center h-8 w-8 rounded-lg text-faint hover:text-red hover:bg-red/10 transition-colors">
				<span aria-hidden="true">✗</span>
			</button>
			@components.ConfirmDialog(components.ConfirmSpec{
				ID: "frei-del-" + row.Day,
				ConfirmAttrs: templ.Attributes{
					"hx-post":   "/ui/dayoffs/delete",
					"hx-target": "#content",
					"hx-swap":   "innerHTML",
					"hx-vals":   freiDeleteVals(row.Day),
					"type":      "button",
				},
			})
		}
	</li>
}

// freiSettingsCard holds the Bundesland select + the ICS feed (copy + regenerate).
templ freiSettingsCard(vm FreiVM) {
	<article class="rounded-3xl bg-surface border border-line shadow-soft p-6 self-start">
		<h2 class="font-display text-lg font-semibold mb-4">{ components.T(ctx, "frei.settings") }</h2>
		<label class="block mb-5">
			<span class="eyebrow uppercase text-[.68rem] font-semibold text-muted">{ components.T(ctx, "frei.bundesland") }</span>
			<select name="bundesland"
				hx-post="/ui/dayoffs/bundesland" hx-target="#content" hx-swap="innerHTML" hx-trigger="change"
				class="mt-1 w-full rounded-lg border border-line bg-sunken/60 px-3 py-2 text-[.9rem] focus:border-blue/40 transition-colors">
				for _, o := range vm.BundeslandOptions {
					if o.Code == vm.BundeslandCode {
						<option value={ o.Code } selected>{ o.Name }</option>
					} else {
						<option value={ o.Code }>{ o.Name }</option>
					}
				}
			</select>
		</label>
		<div>
			<span class="eyebrow uppercase text-[.68rem] font-semibold text-muted">{ components.T(ctx, "frei.icsFeed") }</span>
			<p class="mt-1 text-[.82rem] text-muted">{ components.T(ctx, "frei.icsHint") }</p>
			<div class="mt-2 flex items-center gap-2">
				<code class="flex-1 min-w-0 truncate rounded-lg bg-sunken px-3 py-2 text-[.78rem] font-mono text-body">{ vm.IcsURL }</code>
				<button type="button" data-copy={ vm.IcsURL } data-copied-label={ components.T(ctx, "frei.copied") }
					class="shrink-0 rounded-lg border border-line bg-surface px-3 py-2 text-[.8rem] font-medium text-body hover:text-blue hover:border-blue/40 transition-colors">
					{ components.T(ctx, "frei.copy") }
				</button>
			</div>
			<button type="button" data-dialog-open="frei-regen"
				class="mt-3 text-[.8rem] text-muted underline hover:text-red transition-colors">
				{ components.T(ctx, "frei.regenToken") }
			</button>
			@components.ConfirmDialog(components.ConfirmSpec{
				ID:              "frei-regen",
				BodyKey:         "frei.regenBody",
				ConfirmLabelKey: "frei.regenToken",
				ConfirmAttrs: templ.Attributes{
					"hx-post":   "/ui/dayoffs/regen-token",
					"hx-target": "#content",
					"hx-swap":   "innerHTML",
					"type":      "button",
				},
			})
		</div>
		<script src="/static/js/clipboard.js" defer></script>
	</article>
}
```

- [ ] **Step 3: Create `clipboard.js`**

Create `internal/adapter/webui/static/js/clipboard.js`:

```js
/* clipboard.js — dependency-free copy-to-clipboard for the Frei ICS feed URL.
 * A [data-copy] button writes its value to the clipboard via the async
 * Clipboard API and briefly swaps its label to data-copied-label. No native
 * browser popups (alert/confirm/prompt are banned by verify-no-popups).
 * Idempotent: a single delegated click listener survives htmx swaps. */
(function () {
  "use strict";
  if (window.__flowClipboardBound) { return; }
  window.__flowClipboardBound = true;

  document.addEventListener("click", function (e) {
    var btn = e.target.closest("[data-copy]");
    if (!btn) { return; }
    e.preventDefault();
    var text = btn.getAttribute("data-copy") || "";
    var done = btn.getAttribute("data-copied-label") || "✓";
    var orig = btn.textContent;
    function flash() {
      btn.textContent = done;
      setTimeout(function () { btn.textContent = orig; }, 1500);
    }
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(flash, flash);
    } else {
      flash();
    }
  });
})();
```

- [ ] **Step 4: Add the i18n keys (both catalogs)**

In `internal/i18n/catalog_de.go`:

```go
"frei.eyebrow":      "Abwesenheit",
"frei.addTitle":     "Frei-Tag erfassen",
"frei.from":         "Von",
"frei.to":           "Bis",
"frei.kind":         "Art",
"frei.label":        "Label (optional)",
"frei.skipWeekends": "Wochenenden überspringen",
"frei.addButton":    "Hinzufügen",
"frei.emptyTitle":   "Keine Einträge",
"frei.empty":        "Erfasse deinen ersten Frei-Tag oben.",
"frei.settings":     "Einstellungen",
"frei.icsFeed":      "Kalender-Abo (ICS)",
"frei.icsHint":      "Abonniere diesen Link in deinem Kalender.",
"frei.copy":         "Kopieren",
"frei.copied":       "Kopiert ✓",
"frei.regenToken":   "Token regenerieren",
"frei.regenBody":    "Der alte Abo-Link wird ungültig.",
"frei.bundesland":   "Bundesland",
```

In `internal/i18n/catalog_en.go`:

```go
"frei.eyebrow":      "Time off",
"frei.addTitle":     "Add a day off",
"frei.from":         "From",
"frei.to":           "To",
"frei.kind":         "Type",
"frei.label":        "Label (optional)",
"frei.skipWeekends": "Skip weekends",
"frei.addButton":    "Add",
"frei.emptyTitle":   "No entries",
"frei.empty":        "Add your first day off above.",
"frei.settings":     "Settings",
"frei.icsFeed":      "Calendar feed (ICS)",
"frei.icsHint":      "Subscribe to this link in your calendar.",
"frei.copy":         "Copy",
"frei.copied":       "Copied ✓",
"frei.regenToken":   "Regenerate token",
"frei.regenBody":    "The old subscription link will stop working.",
"frei.bundesland":   "Federal state",
```

- [ ] **Step 5: Rewrite the day-off handlers onto `FreiVM`**

Replace `internal/adapter/httpserver/webui_dayoffs.go` with (keeping the unchanged `handleWebDayOffAdd`, `handleWebDayOffDelete`, `handleWebRegenToken` bodies — only the data builder + render + home/fragment change, plus the new state-option helpers):

```go
package httpserver

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

// germanStates is the Bundesland <select> source (DE = bundesweit only).
var germanStates = []webui.FreiBundeslandOption{
	{Code: "DE", Name: "Bundesweit"},
	{Code: "BW", Name: "Baden-Württemberg"},
	{Code: "BY", Name: "Bayern"},
	{Code: "BE", Name: "Berlin"},
	{Code: "BB", Name: "Brandenburg"},
	{Code: "HB", Name: "Bremen"},
	{Code: "HH", Name: "Hamburg"},
	{Code: "HE", Name: "Hessen"},
	{Code: "MV", Name: "Mecklenburg-Vorpommern"},
	{Code: "NI", Name: "Niedersachsen"},
	{Code: "NW", Name: "Nordrhein-Westfalen"},
	{Code: "RP", Name: "Rheinland-Pfalz"},
	{Code: "SL", Name: "Saarland"},
	{Code: "SN", Name: "Sachsen"},
	{Code: "ST", Name: "Sachsen-Anhalt"},
	{Code: "SH", Name: "Schleswig-Holstein"},
	{Code: "TH", Name: "Thüringen"},
}

func bundeslandOptions() []webui.FreiBundeslandOption { return germanStates }

func bundeslandName(code string) string {
	for _, o := range germanStates {
		if o.Code == code {
			return o.Name
		}
	}
	return code
}

func (s *Server) dayOffData(ctx context.Context, u domain.User) (webui.FreiVM, error) {
	now := s.Clock.Now()
	loc := now.Location()
	year := now.Year()
	from := time.Date(year, 1, 1, 0, 0, 0, 0, loc)
	to := time.Date(year, 12, 31, 0, 0, 0, 0, loc)

	list, err := s.ListDayOffs.Execute(ctx, u.ID, from, to)
	if err != nil {
		return webui.FreiVM{}, err
	}
	set, toks, err := s.GetSettings.Execute(ctx, u.ID)
	if err != nil {
		return webui.FreiVM{}, err
	}

	sort.Slice(list, func(i, j int) bool { return list[i].Date.Before(list[j].Date) })
	rows := make([]webui.FreiRowVM, 0, len(list))
	hasOwn := false
	for _, d := range list {
		isHol := d.Kind == domain.KindHoliday
		if !isHol {
			hasOwn = true
		}
		rows = append(rows, webui.FreiRowVM{
			DateLabel: d.Date.In(loc).Format("02.01.2006"),
			KindLabel: d.Kind.LabelDe(),
			Hue:       dayOffHue(d.Kind),
			Label:     d.Label,
			IsHoliday: isHol,
			Day:       d.Date.In(loc).Format("2006-01-02"),
		})
	}

	code, _ := domain.ValidBundesland(set.Bundesland)
	if code == "" {
		code = "DE"
	}
	return webui.FreiVM{
		User:              u.Username,
		BundeslandCode:    code,
		BundeslandName:    bundeslandName(code),
		BundeslandOptions: bundeslandOptions(),
		Year:              strconv.Itoa(year),
		IcsURL:            firstFeedURL(toks),
		Rows:              rows,
		HasOwn:            hasOwn,
	}, nil
}

func firstFeedURL(toks []domain.FeedToken) string {
	if len(toks) == 0 {
		return "(none — regenerate below)"
	}
	return "/ics/" + toks[0].Token + ".ics"
}

func (s *Server) renderDayOffFragment(w http.ResponseWriter, r *http.Request, u domain.User) {
	vm, err := s.dayOffData(r.Context(), u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.FreiFragment(vm).Render(r.Context(), w)
}

func (s *Server) handleWebDayOffHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	vm, err := s.dayOffData(r.Context(), u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.FreiPage(vm).Render(r.Context(), w)
}

func (s *Server) handleWebDayOffFragment(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	s.renderDayOffFragment(w, r, u)
}

func (s *Server) handleWebDayOffAdd(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	kind, ok := domain.ParseKind(r.FormValue("kind"))
	if ok {
		from, err1 := time.ParseInLocation("2006-01-02", r.FormValue("from"), time.Local)
		to, err2 := time.ParseInLocation("2006-01-02", r.FormValue("to"), time.Local)
		if err1 == nil && err2 == nil {
			_ = s.AddDayOffs.Execute(r.Context(), u.ID, from, to, kind, r.FormValue("label"), 0, r.FormValue("skipWeekends") == "true")
		}
	}
	s.renderDayOffFragment(w, r, u)
}

func (s *Server) handleWebDayOffDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	if day, err := time.ParseInLocation("2006-01-02", r.FormValue("day"), time.Local); err == nil {
		_ = s.DeleteDayOff.Execute(r.Context(), u.ID, day)
	}
	s.renderDayOffFragment(w, r, u)
}

func (s *Server) handleWebRegenToken(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_, _ = s.RegenIcsToken.Execute(r.Context(), u.ID)
	s.renderDayOffFragment(w, r, u)
}
```

(`AddDayOffs.Execute`'s 7th arg `targetPerDay` is `time.Duration`; the literal `0` is fine. The `apiclient` import is gone — `FreiVM` carries `FreiRowVM`, not `apiclient.DayOff`.)

- [ ] **Step 6: Delete the old day-offs template**

```bash
git rm internal/adapter/webui/dayoffs.templ internal/adapter/webui/dayoffs_templ.go
```

(`DayOffPage`/`DayOffFragment`/`DayOffData` are now unreferenced. Confirm with `rg "DayOffPage|DayOffFragment|DayOffData" internal --glob '!*_templ.go'` → no hits.)

- [ ] **Step 7: Update the day-off handler tests**

In `internal/adapter/httpserver/webui_dayoffs_test.go`, update `TestWebDayOffPageAndMutations`'s assertions to the new `FreiVM` markup (the page title is now `flow · frei`; the ICS feed is identified by the `/ics/` URL and the bundesland select, not the old literal "Calendar feed"):

- In the "Full page renders" check, replace `strings.Contains(body, "flow · dayoffs")` with `strings.Contains(body, "flow · frei")`.
- In the "Fragment endpoint renders standalone" check, replace `strings.Contains(body, "Calendar feed")` with `strings.Contains(body, "name=\"bundesland\"")`.
- The add (`2026-06-15`) and regen (`/ics/`) assertions stay — but note the date now renders as `15.06.2026`, so change the add assertion from `strings.Contains(body, "2026-06-15")` to `strings.Contains(body, "15.06.2026")`.

`TestWebDayOffPage_ListsAllManualKinds` is unchanged — the add-form kind `<select>` still renders `Urlaub/Krank/Gleittag/Sonderurlaub/Kind krank/Fortbildung` via `freiKinds()` → `Kind.LabelDe()`.

- [ ] **Step 8: Generate, build, test**

Run: `go tool templ generate && go test ./internal/adapter/webui/... ./internal/adapter/httpserver/... -v`
Expected: PASS. (The bundesland `<select>` POSTs to a route not yet registered — no test exercises that POST until Task 5.)

- [ ] **Step 9: Rebuild CSS + verify-no-popups + full CI**

Run: `make web && make ci`
Expected: all green — including `verify-no-popups` (clipboard.js uses only `navigator.clipboard`).

- [ ] **Step 10: Commit**

```bash
git add internal/adapter/webui/frei.templ internal/adapter/webui/frei_templ.go \
        internal/adapter/webui/frei_vm.go \
        internal/adapter/webui/static/js/clipboard.js \
        internal/adapter/httpserver/webui_dayoffs.go \
        internal/adapter/httpserver/webui_dayoffs_test.go \
        internal/i18n/catalog_de.go internal/i18n/catalog_en.go \
        internal/adapter/webui/static/app.css
git add -u  # stage the dayoffs.templ deletions
git commit -m "feat(webui): Frei page on AppShell (add/list+holidays/delete + ICS copy + Bundesland select)"
```

---

## Task 5: Bundesland web handler + route

Add the web variant of `handleSetBundesland`: validate, persist, publish `settings.changed` (so other tabs reload + holidays recompute), and re-render the Frei fragment (instead of the REST handler's JSON).

**Files:**
- Modify: `internal/adapter/httpserver/webui_dayoffs.go` (add `handleWebSetBundesland`)
- Modify: `internal/adapter/httpserver/server.go` (register the route)
- Modify: `internal/adapter/httpserver/webui_dayoffs_test.go` (add the bundesland test)

**Interfaces:**
- Consumes: `s.SetBundesland.Execute(ctx, ownerID, land string) error` (returns `domain.ErrInvalidDayOff` on an invalid code), `s.Bus.Publish(domain.Event{Type: domain.EventSettingsChanged, UserID})`, `renderDayOffFragment` (from Task 4).
- Produces: `POST /ui/dayoffs/bundesland` → `handleWebSetBundesland`, behind `s.webAuth`.

- [ ] **Step 1: Write the failing test**

Add to `internal/adapter/httpserver/webui_dayoffs_test.go`:

```go
func TestWebSetBundesland(t *testing.T) {
	srv, codec := newWebDayOffServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	do := func(body string) (int, string) {
		req, _ := http.NewRequest("POST", ts.URL+"/ui/dayoffs/bundesland", strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		return res.StatusCode, string(b)
	}

	// Switch to NW → 200, fragment header names the state + NW holidays appear.
	code, body := do(url.Values{"bundesland": {"NW"}}.Encode())
	if code != http.StatusOK {
		t.Fatalf("set bundesland status=%d body=%.200s", code, body)
	}
	if !strings.Contains(body, "Nordrhein-Westfalen") {
		t.Fatalf("fragment should name the new state, got: %.300s", body)
	}
	if !strings.Contains(body, "Fronleichnam") { // NW-specific holiday → recomputed
		t.Fatalf("NW holidays should recompute (expected Fronleichnam), got: %.300s", body)
	}

	// Invalid code → 400.
	code, _ = do(url.Values{"bundesland": {"XX"}}.Encode())
	if code != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid bundesland, got %d", code)
	}
}
```

(The harness's clock is 2026; `ListDayOffs` uses `Loc: time.UTC` and merges computed holidays, so Fronleichnam 2026 appears for NW.)

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/adapter/httpserver/ -run TestWebSetBundesland -v`
Expected: FAIL — the route 404s (or the handler is undefined).

- [ ] **Step 3: Add `handleWebSetBundesland`**

In `internal/adapter/httpserver/webui_dayoffs.go`, add (and add `"errors"` to the imports):

```go
func (s *Server) handleWebSetBundesland(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	if err := s.SetBundesland.Execute(r.Context(), u.ID, r.FormValue("bundesland")); err != nil {
		if errors.Is(err, domain.ErrInvalidDayOff) {
			http.Error(w, "invalid bundesland", http.StatusBadRequest)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	// Holidays are derived from the Bundesland → notify other tabs to reload.
	s.Bus.Publish(domain.Event{Type: domain.EventSettingsChanged, UserID: u.ID})
	s.renderDayOffFragment(w, r, u)
}
```

- [ ] **Step 4: Register the route**

In `internal/adapter/httpserver/server.go`, after the `POST /ui/dayoffs/regen-token` line (~166), add:

```go
	mux.Handle("POST /ui/dayoffs/bundesland", s.webAuth(http.HandlerFunc(s.handleWebSetBundesland)))
```

- [ ] **Step 5: Run the test**

Run: `go test ./internal/adapter/httpserver/ -run TestWebSetBundesland -v`
Expected: PASS.

- [ ] **Step 6: Full CI**

Run: `make ci`
Expected: all green. (No new templ/CSS.)

- [ ] **Step 7: Commit**

```bash
git add internal/adapter/httpserver/webui_dayoffs.go \
        internal/adapter/httpserver/server.go \
        internal/adapter/httpserver/webui_dayoffs_test.go
git commit -m "feat(webui): POST /ui/dayoffs/bundesland — web handler recomputes holidays + reloads"
```

---

# SLICE 3 — Wiring verification & cleanup

## Task 6: Main-wiring verification, i18n parity, full CI + curl-smoke

A dedicated verification pass (per the "plans need a main-wiring task" rule): confirm every new route is reachable from the composition root, no dead symbols remain, the two catalogs agree, and the whole thing builds + serves live.

**Files:**
- Possibly modify: `internal/i18n/catalog_de.go` / `catalog_en.go` (only if a parity gap is found)
- No new production code expected.

- [ ] **Step 1: Route audit**

Run: `rg -n "/ui/stats|/ui/dayoffs|GET /stats|GET /dayoffs" internal/adapter/httpserver/server.go`
Confirm all of these are registered behind `s.webAuth`: `GET /stats`, `GET /ui/stats/fragment`, `POST /ui/stats/target`, `GET /dayoffs`, `GET /ui/dayoffs`, `POST /ui/dayoffs/add`, `POST /ui/dayoffs/delete`, `POST /ui/dayoffs/regen-token`, `POST /ui/dayoffs/bundesland`. Confirm each handler method exists (`rg "func \(s \*Server\) handleWeb(Stats|SetTarget|DayOff|RegenToken|SetBundesland)"`).

- [ ] **Step 2: Dead-symbol audit**

Run:
```bash
rg -n "DayOffPage|DayOffFragment|DayOffData|StatsData|StatsWeekRow|StatsRange|fmtMin|fmtSaldo|fmtInt|monthBarStyle|weekBarStyle|monthBarPct" internal --glob '!*_templ.go'
```
Expected: no hits (all removed). If any remain, remove them. Confirm `internal/adapter/webui/dayoffs.templ` and `dayoffs_templ.go` are gone (`fd dayoffs internal/adapter/webui`).

- [ ] **Step 3: i18n parity check**

Confirm every `stats.*` and `frei.*` key added to `catalog_de.go` exists in `catalog_en.go` and vice-versa:
```bash
rg -o '"(stats|frei)\.[a-zA-Z]+"' internal/i18n/catalog_de.go | sort -u > /tmp/de.keys
rg -o '"(stats|frei)\.[a-zA-Z]+"' internal/i18n/catalog_en.go | sort -u > /tmp/en.keys
diff /tmp/de.keys /tmp/en.keys && echo "i18n parity OK"
```
Expected: no diff. Add any missing stub.

- [ ] **Step 4: Full CI**

Run: `make web && make ci`
Expected: all green — `lint`, `verify-generate`, `verify-css`, `verify-no-popups`, `cover` (≥75%), `build`.

- [ ] **Step 5: Live curl-smoke (controller, vs the dev Postgres+Dex stack)**

Bring up the dev stack and obtain a token/cookie per `make dev-up` / `make dev-token` (see the dev-env reference). Then verify:
- `GET /stats` → 200, HTML contains the three saldo tile labels (`Heute`/`Woche`/`Monat`) + the burndown banner + the five weekday inputs (`name="mon"` … `name="fri"`).
- `POST /ui/stats/target` with `defaultTargetMin=480&fri=300` → 200; re-fetch `/stats` and confirm the Friday input shows `300` and the default shows `480`.
- `GET /dayoffs` → 200, contains the add-form, the bundesland `<select>`, and an `/ics/…` URL.
- `POST /ui/dayoffs/bundesland` with `bundesland=BY` → 200; fragment header now reads `Bayern · <year>`.
- `GET /ics/<token>.ics` → 200, body begins `BEGIN:VCALENDAR`.

Capture the status codes + key body excerpts in the task report.

- [ ] **Step 6: Hand off for browser dogfood**

Summarize for Soenne the manual checklist (do NOT mark the milestone done until Soenne confirms): both pages on the AppShell in dark/light + mobile; target save (default + a weekday); add + delete a day-off (ConfirmDialog, no popup); change Bundesland → holidays update; regenerate the ICS token (ConfirmDialog) → old link dies; copy the feed URL (label flips to "Kopiert ✓").

- [ ] **Step 7: Commit (only if Step 3 required a fix)**

```bash
git add internal/i18n/catalog_de.go internal/i18n/catalog_en.go
git commit -m "chore(webui): i18n parity for Slice 2 (Stats & Frei) keys"
```

---

## Self-Review (completed by plan author)

**Spec coverage:**
- Stats §1 saldo tiles (Heute/Woche/Monat) → Task 2 (`statsSaldoTile` ×3, `statsData` computes today/week-Mon–Fri/month). ✓
- Stats §2 burndown banner + pace marker → Task 1 (component) + Task 2 (`burndownBannerVM` math, unit-tested). ✓
- Stats §3 target config (default + Mon–Fri overrides), extended `handleWebSetTarget` → Task 2 (form rendering) + Task 3 (authoritative parse + `parseWeekdayTargets`). ✓
- Stats SSE wrapper triggers → Task 2 `statsOuter`. ✓
- Frei §1 capture form (von–bis/kind/label/skipWeekends) → Task 4 `freiAddCard`. ✓
- Frei §2 list + holidays + delete + EmptyState → Task 4 `freiListCard`/`freiRow`. ✓
- Frei §3 ICS copy + regenerate (ConfirmDialog) + Bundesland select → Task 4 (markup, clipboard.js) + Task 5 (handler/route). ✓
- New route `POST /ui/dayoffs/bundesland` → Task 5. ✓
- Delete old `stats.templ` content (rewrite in place) + `dayoffs.templ` → Task 2 / Task 4. ✓
- Testing & done-gate (handler tests, pure-VM unit tests, `make ci`, curl-smoke, dogfood) → distributed across tasks + Task 6. ✓

**Placeholder scan:** every code step shows complete code; no "TBD"/"add validation"/"similar to". ✓

**Type consistency:** `BurndownVM` fields (`Total, Target, Pct, PacePct, Variant, OnTrack`) are identical in the component (Task 1) and `burndownBannerVM` (Task 2). `StatsVM`/`StatsWeekdayVMs`/`renderStatsFragment` defined in Task 2 are reused (not redefined) in Task 3. `FreiVM`/`FreiRowVM`/`renderDayOffFragment` defined in Task 4 are reused in Task 5. Field names match across templ ↔ Go. ✓
