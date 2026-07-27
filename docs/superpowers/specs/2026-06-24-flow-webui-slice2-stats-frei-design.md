# WebUI Slice 2 — Stats & Frei Design Spec

**Goal:** Port the two remaining Worktime sub-tabs — **Stats** and **Frei** (day-offs) — from the old pre-Slice-0 markup onto the new Slice-0 AppShell + Slice-1 component system, completing the Worktime sub-tab strip (Heute · Woche · Historie · **Stats** · **Frei**). No new usecases; faithful feature parity plus two small, explicitly-chosen enhancements (per-weekday targets editable; Bundesland editable).

**Non-goals:** charts/charting libraries (offline, no CDN); trends/longer-range history (deferred); redesigning the day-off domain model or holiday computation; touching the TUI/CLI.

## Architecture

Pure composition on the existing foundation — no new mechanism:

- Two new page packages-of-templ in `internal/adapter/webui/`: `stats.templ` (rewrite) + `frei.templ` (new; old `dayoffs.templ` deleted). Each renders `components.Base("…", …) → components.AppShell("stats"|"frei", nil, worktimeSubnav("stats"|"frei"), content)` with an SSE-swap `#content` wrapper, exactly like `heute.templ`/`woche.templ`.
- `worktimeSubnav` already carries the `stats`→`/stats` and `frei`→`/dayoffs` tabs (heute.templ) — reused unchanged.
- New/rewritten handlers in `internal/adapter/httpserver/webui_stats.go` + `webui_dayoffs.go`, wired to the **existing** usecases: `Stats.Today/Week/Burndown`, `SetTarget`, `GetSettings`, `ListDayOffs/AddDayOffs/DeleteDayOff`, `RegenIcsToken`, `SetBundesland`. No usecase changes.
- Routes (`server.go`): repoint `GET /stats` + `GET /dayoffs` to the new home handlers; keep the `/ui/stats/*` + `/ui/dayoffs/*` fragment/action routes (extend two, add two — see below).
- **Delete** old `internal/adapter/webui/stats.templ` (rewrite in place) and `internal/adapter/webui/dayoffs.templ` (replaced by `frei.templ`) once nothing references the old `StatsPage`/`StatsFragment`/`DayOffPage`/`DayOffFragment` symbols — mirroring the Slice-1 worktime.templ deletion.
- **Reuse:** `StatTile`, `ProgressBar`, `Card`, `badge`/`chip`, `ConfirmDialog`, `EmptyState`, and the `dayOffHue(kind)`/`Kind.LabelDe()` mapping (currently in httpserver — keep there or lift to a shared helper, implementer's call). One small new component if cleaner: a burndown banner with a pace marker (see Stats §2).

## Global Constraints (same as Slice 0/1)

- Offline: every asset local under `/static/`; pages MUST use `components.Base` via `AppShell`, never a hand-rolled `<head>`. No `unpkg/googleapis/gstatic/fontshare/cdn.tailwindcss`.
- No browser popups: every destructive action (delete a day-off, regenerate the ICS token) goes through `components.ConfirmDialog`; never `window.alert/confirm/prompt`.
- i18n: no hardcoded display strings in templates — all via `T(ctx,"key")`/`Tn`; every new key added to BOTH `internal/i18n/catalog_de.go` (full) and `catalog_en.go` (stub). German primary.
- Owner-scoping: every store/usecase call scoped to `u.ID`.
- CI: `make ci` green (lint incl. gofumpt/staticcheck, verify-generate, verify-css, verify-no-popups, coverage gate **75%**, build). `.templ` files must NOT `import "github.com/a-h/templ"` (auto-injected).
- Glyph whitelist (monospace): `▶ ■ ‖ ✓ ✗ ▲ ▼ ● ○ ★ ☼ ✚ ▎ ▰▱ › · ◆ ▤ ▾ ▴`. No colored emoji.
- Module path `github.com/serverkraken/flow`.

---

## Stats page (`/stats`, tab "Stats")

Role: the **aggregate & goal-config board**. The per-day week breakdown is intentionally NOT here (Woche owns it). Three blocks:

**§1 — Saldo tiles (top).** Three `StatTile`s side by side: **Heute · Woche · Monat**. Each shows the saldo (`+2h 18m` green / `−1h 05m` red via the balance hue) with a sub-line `geloggt / Ziel`. Data: `Stats.Today` (today saldo/logged/target), `Stats.Week` summed Mon–Fri (week saldo), `Stats.Burndown` (month total/target/saldo).

**§2 — Monats-Burndown banner.** A `Card` in the `WeekTotalBanner` style: "Monat gesamt **{total}** / **{target}**" + a `ProgressBar` (month `Total/Target`) + a **pace marker** — a thin vertical tick at the position where one *should* stand by today, plus a green "auf Kurs" / yellow "Rückstand" label from `report.OnTrack`. `MonthBurndownReport` gives `Total`, `Target`, `Saldo` (= Total − expected-by-now), `OnTrack`, `WorkdaysAll/Due`; the **expected-by-now = `Total − Saldo`**, so the marker position = `clamp((Total−Saldo)/Target × 100, 0, 100)%`. No charting lib — a positioned `<span>` over the bar. May be a small new component `BurndownBanner(vm)` or an extension of the existing banner; implementer's call.

**§3 — Tagesziel-Konfig card.** Form posting to `POST /ui/stats/target`:
- **Standard-Tagesziel** input (hours+minutes, or minutes — match the existing `defaultTargetMin` field).
- **Per-weekday overrides Mon–Fri**: five inputs, each optional (empty → inherits the default). Pre-filled from `Settings.WeekdayTargetMin`.
- The handler `handleWebSetTarget` is **extended**: parse `defaultTargetMin` + the five weekday values into a `map[time.Weekday]int`, call `SetTarget.Execute(ctx, u.ID, defaultMin, weekdayMap)`. (Today it only reads `defaultTargetMin` and preserves the existing map; now the form is the source of truth for both.) Empty weekday inputs omit that key (inherit default).
- Validation: invalid/negative → 400 or an inline error banner (reuse the fragment's error slot).

**SSE wrapper:** `#content` reloads on `sse:session.started/stopped/updated/deleted`, `sse:settings.changed`, `sse:dayoff.changed` (holidays change targets).

`StatsVM` (sketch): `TodaySaldo/TodaySub`, `WeekSaldo/WeekSub`, `MonthSaldo/MonthSub` (strings + pos bools); `MonthTotal/MonthTarget` + `MonthPct int` + `MonthVariant` + `PacePct int` + `OnTrack bool`; `DefaultTarget string` + `Weekdays []WeekdayTargetVM{Label, Name (form field), Value string}`; `Err string`.

---

## Frei page (`/dayoffs`, tab "Frei")

Role: manage day-offs + see holidays + the ICS subscription. Three cards:

**§1 — Frei-Tag erfassen card.** Form posting `POST /ui/dayoffs/add` (existing handler, unchanged):
- **von–bis** two `<input type="date">` (a single day = von==bis).
- **Art** select of the six kinds (Urlaub/Krank/Gleittag/Sonderurlaub/Kind-krank/Fortbildung), each option rendered with its `dayOffHue` color cue + `Kind.LabelDe()`.
- optional **Label** text input.
- **„Wochenenden überspringen"** checkbox (`skipWeekends`, default on).
- `AddDayOffs.Execute(ctx, u.ID, from, to, kind, label, targetPerDay=0, skipWeekends)`.

**§2 — Frei-Liste (year) card.** Own day-offs **+ Feiertage** for the current year, sorted by date. Header "**{Bundesland} · {year}**". Each row: date · kind badge (`dayOffHue` + `○` glyph, same look as the Slice-1 calendar day-off badges) · label. Own entries get a **Löschen** control → `components.ConfirmDialog` → `POST /ui/dayoffs/delete`. Holidays are read-only (distinct dimmed styling, the `Feiertag` kind, no delete). `EmptyState` when there are zero own day-offs (holidays still listed). Data: `ListDayOffs.Execute(ctx, u.ID, jan1, dec31)` (which already merges computed holidays).

**§3 — Einstellungen card.**
- **ICS-Feed**: show the subscription URL `…/ics/{token}` (read from settings) with a "Kopieren"-to-clipboard button (tiny vanilla JS, dependency-free, no popup) and a **„Token regenerieren"** action behind a `ConfirmDialog` (invalidates the old link) → `POST /ui/dayoffs/regen-token` (existing handler).
- **Bundesland**: a `<select>` of the 16 German states (codes from `holidays_de.go`), current value pre-selected. Change posts to a **new** web handler `POST /ui/dayoffs/bundesland` → `SetBundesland.Execute` → re-render the fragment (holidays recompute). Mirrors the existing REST `handleSetBundesland`; the web variant renders the Frei fragment instead of JSON.

**SSE wrapper:** `#content` reloads on `sse:dayoff.changed`, `sse:settings.changed`.

`FreiVM` (sketch): `Bundesland string` + `BundeslandOptions []{Code,Name}` ; `Year string`; `IcsURL string`; `Rows []FreiRowVM{DateLabel, Kind, KindLabel, Hue, Label, IsHoliday bool, DeleteAttrs}`; `Err string`.

---

## New / changed routes (server.go)

- `GET /stats` → `handleWebStatsHome` (rewritten) ; `GET /ui/stats/fragment` → `handleWebStatsFragment` (rewritten) ; `POST /ui/stats/target` → `handleWebSetTarget` (**extended** for per-weekday).
- `GET /dayoffs` → `handleWebDayOffHome` (rewritten) ; `GET /ui/dayoffs` → `handleWebDayOffFragment` (rewritten) ; `POST /ui/dayoffs/add` (unchanged) ; `POST /ui/dayoffs/delete` (unchanged) ; `POST /ui/dayoffs/regen-token` (unchanged) ; **NEW** `POST /ui/dayoffs/bundesland` → `handleWebSetBundesland` (web variant, re-renders the fragment).

All `s.webAuth`. No new Server struct fields (all usecases already wired).

## Testing & done-gate

- Handler tests (cookie harness `newWorktimeTestServer` / the dayoff harness): `/stats` 200 renders the three saldo tiles + the burndown banner + the target form (incl. the five weekday inputs); `POST /ui/stats/target` with default + a Friday override persists both (assert via the settings store); `/dayoffs` 200 renders the add-form + a seeded day-off badge + a holiday row; `POST /ui/dayoffs/add` adds; `POST /ui/dayoffs/delete` removes; `POST /ui/dayoffs/bundesland` changes the state + recomputes holidays. Pure-VM unit tests for the pace-marker math + the weekday-target form mapping.
- `make ci` green (lint, verify-generate, verify-css after `make web`, verify-no-popups incl. any new JS, coverage ≥75%, build).
- Live curl-smoke (controller, vs Postgres+Dex): `/stats` + `/dayoffs` 200; the new bundesland POST works; the ICS URL is present and `/ics/{token}` serves a calendar.
- Browser dogfood (Soenne): both pages on the AppShell, dark/light, mobile; target save (default + weekday); add/delete a day-off; change Bundesland → holidays update; regenerate the ICS token (ConfirmDialog) → old link dies; copy the feed URL.

## File structure

**Create:** `internal/adapter/webui/frei.templ` (+ `frei_vm.go` if cleaner); `internal/adapter/httpserver/webui_stats_test.go` / `webui_dayoffs_test.go` additions; optional `components/burndownbanner.templ`.
**Modify:** `internal/adapter/webui/stats.templ` (rewrite onto AppShell) ; `internal/adapter/httpserver/webui_stats.go` (rewrite home/fragment + extend SetTarget) ; `internal/adapter/httpserver/webui_dayoffs.go` (rewrite home/fragment + add `handleWebSetBundesland`) ; `internal/adapter/httpserver/server.go` (new bundesland route) ; `internal/i18n/catalog_de.go` + `catalog_en.go` (new keys) ; `internal/adapter/webui/static/app.css` (rebuilt) ; possibly `internal/adapter/webui/static/js/` (tiny copy-to-clipboard helper).
**Delete:** `internal/adapter/webui/dayoffs.templ` (+ `_templ.go`) once unreferenced; the old `stats.templ` content is replaced in place.

## Slices (suggested for the plan)

1. Stats page (saldo tiles + burndown banner + extended target form/handler) + tests.
2. Frei page (add/list/holidays/delete + ICS + bundesland web handler + tiny clipboard JS) + tests.
3. Main-wiring verification + cleanup (delete old templ, i18n completeness, `make ci`, curl-smoke). Each slice its own subagent-driven task set.
