# Worktime Parity — Export Drill + Woche Summary — Design

**Date:** 2026-06-20
**Branch:** `rebuild` (unmerged)
**Status:** approved

## Problem

Two regressions of the rebuild's Worktime vs. the old design surfaced during dogfood:

1. **Export traps / navigation broken.** Export is the only multi-field form among
   the five Worktime routes. Its `CapturesInput()` only returns true for the
   von/bis/Pfad fields (focus 1/2/4); on Range/Format focus (0/3) the shell grabs
   `Tab` (switches top tab), and digits/`:`/`?` leak to the shell — so tabbing
   through the form derails. And because Export is a strip member with no working
   `←/→` (those edit dates), the only way out is `Esc` — a trap. The old design's
   sub-tab strip was `Heute · Woche · Verlauf · Frei` (four, **no Export**).

2. **Woche is missing its summary.** The old Woche view showed, below the per-day
   rows: **WOCHE GESAMT** (week total + bar), and **KENNZAHLEN** (Schnitt · Ziele
   N/N · Saldo, a Mon–Fri pace-dot row ○/●, and a `▲ auf Kurs` / `▼ im Rückstand`
   marker), with per-day day-off labels (Wochenende/Feiertag/Urlaub/Krank). The
   rebuild's Woche shows only per-day rows. Day-off accounting (holidays/vacation/
   sick) must be reflected.

## Decisions (from brainstorming)

- **Part A — Export becomes a drill, out of the strip.** Strip returns to four
  tabs `Heute · Woche · Stats · Frei` (matches the old design). Export is opened
  with `e` from any strip route (existing `navKey` mapping), `Esc`/`q` returns;
  `CapturesInput()` becomes true for the whole form (fixes the Tab/digit/arrow
  leak); Export no longer renders the strip and no longer hides the breadcrumb
  (it shows `Worktime › Export`). The four strip routes advertise `e → Export`.
- **Part B — restore the Woche summary**, computed client-side from the already
  dayoff-netted per-day data plus `ListDayOffs` (no Stats/Burndown calls, no
  server change). Port the pace-dot classification from the old design.

## Part A — Export Drill

### wtnav
- `SubTabs` drops the Export entry → four: `{Heute,""},{Woche,"w"},{Stats,"t"},
  {Frei,"d"}`. Remove `IdxExport`. `Lateral` now clamps at `IdxFrei` (last index
  3). Strip/Lateral behaviour otherwise unchanged.
- `Registry`/`Nav` keep the `"e"` factory (Export still opens via `e`) — only the
  strip membership changes, not the open mechanism.

### export route
- `CapturesInput() bool { return true }` (the whole form captures input). This
  makes the shell forward `Tab`/`Shift+Tab` (field cycling), digits, and `←/→`
  to Export at every focus; `Esc` still reaches the back chain (returns to Heute).
- Remove the strip from `View()` (revert to the pre-strip body) and remove
  `HideBreadcrumb()` — Export shows the normal `Worktime › Export` breadcrumb as
  a drilled route.
- Remove the `{w/t/d/e, Bereich}` KeyHint added earlier. KeyHints stay
  `[{tab,Feld},{←/→,wählen},{enter,export},{esc,zurück}]` — all now functional.

### the four strip routes (Heute, Woche, Stats, Frei)
- Add an `{e, Export}` KeyHint so the drill is discoverable now that Export is not
  in the strip. (Footer caps at 4; overflow surfaces in `?`-help.) The existing
  `e` accelerator (via `navKey`/the `w/t/d/e` case) already opens Export.

## Part B — Woche Summary

### Pace-dot logic (ported, Worktime-local)
Port the kind classification from the old `domain/pace_dot.go`, adapted to the
rebuild's DTO (`apiclient.WeekDay`: `LoggedMin`, `TargetMin`, `IsToday`,
`Workday` ints/bools — no live `Active`). New file in the worktime layer
(e.g. `internal/tui/screen/worktime/week/pacedot.go`):

- `type PaceDotKind int` with `PaceDotMissed=0, PaceDotHit, PaceDotRunning,
  PaceDotDayOff`.
- `PaceDotGlyph(k) string` → `○` for Missed, `●` otherwise.
- `paceDotKind(d apiclient.WeekDay, off *apiclient.DayOff) PaceDotKind`:
  - `off != nil` → `PaceDotDayOff`
  - `TargetMin > 0 && LoggedMin >= TargetMin` → `PaceDotHit`
  - `IsToday` → `PaceDotRunning` (today, target not yet met)
  - else → `PaceDotMissed`
- Colour mapping (TUI): Hit → `Sem().Success`, Running → `Sem().Active`, Missed →
  `Sem().Border`/`FgMuted`. DayOff → by kind, using existing Sem slots (no
  day-off-kind colour helper exists in the rebuild — `kindcolor` is docs-only —
  so define a small local map in the pacedot file): `KindHoliday → Sem().Info`,
  `KindVacation → Sem().Accent`, `KindSick → Sem().Warning`, unknown → `FgMuted`.
  (The old design's Blue/Purple/Orange map onto Info/Accent/Warning; the rebuild
  theme has no Purple/Orange semantic slots.)

### week route data
- `API` interface gains `ListDayOffs(ctx, from, to string) ([]apiclient.DayOff, error)`
  (the client already implements it). `*apiclient.Client` still satisfies the
  interface.
- `loadCmd` additionally fetches `ListDayOffs` for the ISO-week range
  (Monday..Sunday of the loaded week) and stores the result; build a
  `map[string]apiclient.DayOff` keyed by `DayOff.Day` (date string).
- The week range is derived from the returned `days` (first/last `Date`), so no
  extra date math diverges from what `GetWeek` returned.

### week View (added below the per-day rows)
Keep the existing per-day rows but enrich them and append two sections:

- **Per-day rows:** for a weekend (Sat/Sun by date) or a day-off day (in the map),
  show the label (`Wochenende`, or the day-off `Label`/kind) instead of the
  progress bar. Working days keep the existing `bar + logged/target`.
- **WOCHE GESAMT:** a header line, then `Σloggedh / Σtargeth` and a full-width bar
  at `Σlogged/Σtarget` percent (Σtarget already day-off-netted server-side).
- **KENNZAHLEN:** a header line, then `Schnitt <avg>  ·  Ziele <hits>/<workdays>  ·
  Saldo <signed>`, then the Mon–Fri pace-dot row + `<hits>/<workdays> Ziele` +
  `▲ auf Kurs` / `▼ im Rückstand` (or neutral dot when nothing is due yet).

Client-side aggregation (all from `days` + day-off map):
- `workdays` = count of non-weekend, non-day-off days.
- `Σlogged` = sum `LoggedMin`; `Σtarget` = sum `TargetMin`.
- `avg` = `Σlogged / workdays` (0 when no workdays).
- `saldo` = `Σlogged − Σtarget` (signed, formatted via `wtfmt`).
- `hits` = pace-dot `PaceDotHit` count over workdays; `expected` = past workdays
  plus today-if-hit; track marker: `expected==0` → neutral, `hits>=expected` →
  `▲ auf Kurs` (Success), else `▼ im Rückstand` (Warning).

Use existing helpers: `statusbar.Bar`/`BarColored`, `wtfmt.FormatMin`/`FormatSaldo`,
`glyphs.*` (`▲`/`▼`/`○`/`●` — all already in the whitelist), section-header
styling consistent with the other routes. The strip stays at the top (Woche is
still a strip member).

## Testing

- **wtnav:** `SubTabs` has four entries (no Export); `Lateral` from Frei `→`
  clamps to nil (Frei is last); `Lateral` still pops to Heute and switches
  Woche/Stats/Frei correctly.
- **export route:** `CapturesInput()` is true at every focus (0..4); `View()` no
  longer contains the other tab labels (no strip); Export does not implement
  `HideBreadcrumb` (or it returns nothing / is removed) so the breadcrumb shows;
  `e`/`navKey` still opens Export from the strip routes.
- **pacedot:** table test of `paceDotKind` for each branch (dayoff, hit, today-
  running, missed) and `PaceDotGlyph` (○ only for Missed).
- **week route:** `View()` contains `WOCHE GESAMT` and `KENNZAHLEN`; a weekend/
  day-off day renders its label not a bar; totals/avg/saldo/hits computed
  correctly for a constructed week (with a day-off in the map) — assert the
  rendered numbers; `ListDayOffs` is requested for the week range.

## Constraints

- `make ci` green (lint + templ + build + tests; coverage gate ≥80%). Run it.
- German UI (Wochenende/Feiertag/Urlaub/Krank, Schnitt/Ziele/Saldo, auf Kurs/im
  Rückstand); English code/comments. Hints use ` → `/`  ·  `, never `=`.
- No emoji; glyphs only from `ui/glyphs`. No raw hex; colours via `theme.Sem()`/
  `kindcolor`. Day-off netting stays server-side (do not re-net client-side).

## Out of scope

- Server changes (week summary is composed client-side).
- The progress-bar colour tweak (separate, deferred).
- Live per-day running-session distinction (today-not-yet-hit renders as Running).
- Mouse/click.
