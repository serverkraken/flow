# Worktime Day-Off Categories — Design

**Date:** 2026-06-20
**Status:** Approved (pending spec review)
**Branch:** `rebuild`

## Problem

When adding a day-off in the TUI "Frei" route, the user cannot choose a
category. The add dialog only has **Von · Bis · Label**, and on submit the kind
is hardcoded to `urlaub` (`dayoffs/dialogs.go:194`). The list also does not
visually distinguish categories — vacation and sick both render as a plain `○`;
only Feiertag is ad-hoc dimmed.

The domain already models a day-off `Kind` (holiday / vacation / sick) end to
end (domain → usecase → REST → store → WebUI select), but the TUI never exposes
it for selection and no surface colors the new kinds consistently.

## Goal

1. Expand the category set from 3 to **7**.
2. Let the user pick a category when adding a day-off in the TUI (separate
   picker list).
3. Show the category — colored glyph + German label — consistently across all
   surfaces that render day-offs.
4. Keep the new categories available in every host where a day-off is created
   (TUI, WebUI, CLI).

Out of scope (explicitly deferred):
- Overtime accounting for Gleittag — all categories behave identically for the
  target/saldo (a full day off: Target 0, no deficit). Categories are
  label + color + future stats grouping only.
- A Stats breakdown by category ("Urlaub: 12 Tage, Krank: 3"). Natural
  follow-up, separate scope.

## No migration needed

The `day_offs.kind` column is free-text; the server validates the value via
`domain.ParseKind` (`usecase/add_dayoffs.go:27`, `httpserver/dayoffs.go:78`,
`httpserver/webui_dayoffs.go:72`). Adding new `Kind` constants + `ParseKind`
cases is sufficient — existing rows are untouched.

## The 7 categories

| Kind const      | stored value | `LabelDe()`   | selectable when adding? |
|-----------------|--------------|---------------|-------------------------|
| KindHoliday     | `holiday`    | Feiertag      | no (computed from Bundesland) |
| KindVacation    | `vacation`   | Urlaub        | yes |
| KindSick        | `sick`       | Krank         | yes |
| **KindFlex**        | `flex`       | Gleittag      | yes |
| **KindSpecial**     | `special`    | Sonderurlaub  | yes |
| **KindChildSick**   | `childsick`  | Kind krank    | yes |
| **KindTraining**    | `training`   | Fortbildung   | yes |

`KindHoliday` is rejected by `AddDayOffs` (`ErrHolidayNotManual`) — holidays are
computed, never stored. So the add picker offers the 6 manual kinds.

## Component design

### 1. Domain (`internal/domain/dayoff.go`)

- Add the 4 new `Kind` constants with the canonical single-token values above.
- Extend `AllKinds` to enumerate all 7 in display order.
- Add a new `SelectableKinds` var = the manual kinds (all except Holiday), used
  by the TUI picker so callers don't re-derive the list.
- Extend `LabelDe()` with the 4 new cases.
- Extend `ParseKind()` to accept German + canonical + a couple of short forms:
  - `flex`, `gleittag`, `gleit` → KindFlex
  - `special`, `sonderurlaub`, `sonder` → KindSpecial
  - `childsick`, `kindkrank`, `kind krank` → KindChildSick
  - `training`, `fortbildung`, `schulung` → KindTraining
  - Existing single-letter forms (`h`/`v`/`s`) are unchanged; no new
    single-letter forms (avoid ambiguity).
- Fix the stale doc comment that says kinds are persisted in
  `worktime-dayoffs.tsv` — it is the Postgres `kind` column now.

### 2. Single-source color helper (`internal/tui/kindcolor/dayoff.go`)

New file in the existing `kindcolor` package (which already owns
DocumentType → color). Add:

```go
func DayOffColor(k domain.Kind, p theme.Palette) theme.Color
```

so the kind→color mapping has one home and the Frei list, Woche pace dots, and
any future surface can never drift. Mapping (all from the semantic palette):

| Kind         | Semantic slot | hue    |
|--------------|---------------|--------|
| Feiertag     | Schedule      | blue   |
| Urlaub       | Highlight     | purple |
| Krank        | Notice        | orange |
| Gleittag     | Success       | green  |
| Sonderurlaub | Warning       | yellow |
| Kind krank   | Danger        | red    |
| Fortbildung  | Info          | cyan   |

Unknown kind → `p.FgMuted` (defensive default). Color is a **secondary** cue —
every surface also shows the German label text, so 7 hues stay A11y-safe.

### 3. TUI add dialog — category picker (`dayoffs/dialogs.go`)

- Add a `kind domain.Kind` field to `dialogState`, default `KindVacation`.
- Add a **Kategorie** field to the add form. Field order / cursor:
  `0 Von · 1 Bis · 2 Kategorie · 3 Label` (Tab cycles, mod 4).
- The Kategorie field renders the currently-selected kind as its **colored
  German label** (via `DayOffColor` + `LabelDe`).
- Pressing **Enter** while the Kategorie field is focused opens a separate
  picker dialog (new `dialogKindPick`) — a `↑/↓` list of `SelectableKinds`
  rendered as colored labels, mirroring the existing `dialogBundesland`
  pattern (`picker.Row`). Enter sets `r.dlg.kind` and returns to the add form
  (cursor back on Kategorie); Esc returns without change.
- `submitAdd` sends `string(r.dlg.kind)` instead of the hardcoded `"urlaub"`.
- Free-text Label stays optional (e.g. "Mallorca").
- Update the add-dialog key hints to mention the category step.

### 4. Frei list rendering (`dayoffs/route.go`)

- Color the `○` glyph via `kindcolor.DayOffColor(domain.Kind(d.Kind), pal)`,
  replacing the ad-hoc `dayOffGlyph(holiday)` dim logic (Feiertag now reads as
  blue, consistent with Woche).
- Always show the German category label, plus the free-text label when present:
  `○ 2026-06-22  Urlaub` or `○ 2026-06-22  Urlaub — Mallorca`. Use
  `domain.Kind(d.Kind).LabelDe()` instead of the raw kind string.

### 5. Woche pace dots (`week/pacedot.go`)

- Replace the inline 3-case `switch` in `paceColor` with a call to
  `kindcolor.DayOffColor`, so the 4 new kinds get colored dots too. Behaviour
  for the existing triad is unchanged (same Schedule/Highlight/Notice hues).

### 6. Other hosts

- **WebUI** (`internal/adapter/webui/dayoffs.templ`): add the 4 new `<option>`s
  to the existing `<select name="kind">` (vacation, sick, flex, special,
  childsick, training — no holiday). Regenerate `dayoffs_templ.go`.
- **CLI** (`cmd/flow/dayoff.go`): update the `--kind` flag help text to list the
  manual kinds. The value passes through to `AddDayOffs`, validated server-side
  via `ParseKind`, so no other CLI change is needed.

## Data flow (add a Gleittag in the TUI)

```
Frei route: a → add form
  Tab to Kategorie → Enter → kind picker (↑/↓) → "Gleittag" → Enter → back to form
  Enter on Label → submitAdd
    apiclient.AddDayOffs(from,to,"flex",label,0,skipWeekends)
      POST /api/v1/dayoffs
        httpserver: ParseKind("flex") → KindFlex
          AddDayOffs.Execute → ExpandRange → DayOffStore.Add (kind="flex")
            EventDayOffChanged published → SSE
  Frei + Woche reload → DayOffColor(KindFlex) → green ● / ○ + "Gleittag" label
```

## Testing

- **domain/dayoff_test.go**: `ParseKind` accepts all new German + canonical
  forms and rejects garbage; `LabelDe` round-trips; `SelectableKinds` excludes
  Holiday and contains all 6 manual kinds.
- **kindcolor**: a table test asserting each of the 7 kinds maps to its
  documented semantic slot, and unknown → FgMuted.
- **dayoffs/route_test.go + dialogs**: add-flow sends the picked kind (not a
  hardcoded `urlaub`); the kind picker sets state and returns to the add form;
  list rows render the German label.
- **week/pacedot_test.go**: existing triad assertions stay green; add cases for
  the 4 new kinds.
- `make ci` green (lint + templ + build + tests + coverage gate).

## Done-gate

- `make ci` green.
- Live dogfood vs the dev stack (Postgres + Dex): add one day-off of each new
  category in the TUI, confirm the colored label appears in Frei and the Woche
  pace dots; confirm the WebUI select offers all 6 manual kinds and a
  round-trip add shows the right label.
