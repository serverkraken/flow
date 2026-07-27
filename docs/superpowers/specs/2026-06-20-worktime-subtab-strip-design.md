# Worktime Sub-Tab Strip — Design

**Date:** 2026-06-20
**Branch:** `rebuild` (unmerged)
**Status:** approved

## Problem

The rebuild's Worktime screen exposes five routes — **Heute, Woche, Stats, Frei,
Export** — but only Heute is visible. The siblings are reachable solely through
undocumented, non-mnemonic key codes (`w`=Woche, `t`=Stats, `d`=Frei,
`e`=Export). From Heute the siblings are neither shown nor advertised (Today's
footer is capped at 4 hints and lists none of them). The old Worktime had a
visible sub-tab strip (`Heute · Woche · Verlauf · Frei`) at the top of the panel,
which was more discoverable. Restore that discoverability.

## Goal

A persistent, visible Worktime sub-tab strip showing all five routes with the
active one highlighted, navigable with `←/→`, while keeping the existing
mnemonic accelerators and the established back/quit grammar.

## Decisions (from brainstorming)

1. **Strip contents:** all five routes, in order `Heute · Woche · Stats · Frei ·
   Export`. Label `Stats` is kept (not renamed to `Verlauf`).
2. **Navigation:** `←/→` moves along the strip (clamp, no wrap). The mnemonic
   accelerators `w/t/d/e` stay as direct shortcuts on every route.
3. **Model — Heute = home-base (unchanged):** Heute is the nav-stack root;
   Woche/Stats/Frei/Export drill one level deeper. `Esc`/`q` on a sibling returns
   to Heute; `Esc`/`q` on Heute leaves Worktime. `←` from a sibling to Heute is a
   `PopRouteMsg` (so Heute's live clock resumes via the existing pop-re-Init).
4. **Export exception:** Export already binds `←/→` for its date-segment and
   field navigation (and `Tab`/`Shift+Tab` for field focus). On Export, `←/→`
   stays date/field navigation; switching to/from Export uses `w/t/d/e` or `Esc`.
   The strip still renders with `Export` active, so position stays visible.
5. **Breadcrumb:** suppressed on Worktime sub-tabs — the strip already shows the
   position, so the `Worktime › Woche` breadcrumb would be redundant.

## Architecture

### Single source of truth: `wtnav`

`internal/tui/screen/worktime/wtnav` is the only package every leaf may import
(it currently imports only `shell`+`tea`; adding `tabstrip`+`theme` introduces no
cycle — both are leaves). It gains:

- `SubTabs` — an ordered slice, the single source for order + labels +
  accelerator keys:
  `[{Label:"Heute", Key:""}, {Label:"Woche", Key:"w"}, {Label:"Stats", Key:"t"},
  {Label:"Frei", Key:"d"}, {Label:"Export", Key:"e"}]`.
- `Strip(activeIdx, width int, pal theme.Palette) string` — renders the strip via
  `tabstrip.Render(labels, activeIdx, width, pal)` (the same component as the top
  tabs, so the active tab is highlighted identically one level down).
- `Lateral(reg Registry, currentIdx int, k tea.KeyPressMsg) tea.Cmd` — maps
  `←/→` to a navigation command:
  - `←` → target `currentIdx-1`; `→` → target `currentIdx+1`; clamp to
    `[0, len(SubTabs)-1]`, no wrap. Target == current → `nil`.
  - target index 0 (Heute) from a sibling → `func() tea.Msg { return
    shell.PopRouteMsg{} }`.
  - target index > 0 → `reg.Nav(SubTabs[target].Key)` (emits `SwitchRouteMsg`;
    the shell pushes from Heute / replaces sibling↔sibling via existing logic).
  - any non-arrow key → `nil` (caller keeps handling it).

### Routes (Heute, Woche, Stats, Frei, Export)

Each route:
- declares its own sub-tab index constant (Heute=0 … Export=4);
- prepends `wtnav.Strip(idx, f.Width, f.Pal)` + a blank line as the first lines
  of `View()`;
- handles `←/→` via `wtnav.Lateral(r.reg, idx, k)` **except Export**, which keeps
  its existing `←/→` date/field handling and switches only via `w/t/d/e`/`Esc`;
- implements `HideBreadcrumb() bool { return true }`;
- updates `KeyHints()` to advertise `←/→ → Bereich` (Export: `w/t/d/e →
  Bereich`); the full accelerator list lives in the `?`-help overlay (footer is
  capped at 4). Existing per-sibling `t/d/e` footer hints are consolidated to the
  uniform `←/→ Bereich` + that route's own verbs.

`↑/↓` (Heute's session cursor via `listnav`) is independent of `←/→` and stays
unchanged. The `←/→` handling runs alongside, not instead of, the cursor.

### Shell

- `internal/tui/shell/route.go`: new capability
  `type BreadcrumbHider interface { HideBreadcrumb() bool }`.
- `internal/tui/shell/shell.go`: when the active top route implements
  `BreadcrumbHider` and returns `true`, render an empty breadcrumb line
  (`crumbs = ""`). Keeps the chrome generic; only worktime routes opt in.

## Testing

- `wtnav`:
  - `Strip` output contains all five labels and highlights the active index.
  - `Lateral`: `→` from Heute → `SwitchRouteMsg` whose route Title is "Woche";
    `←` from Woche → `PopRouteMsg`; `→` from Stats → `SwitchRouteMsg` ("Frei");
    `←`/`→` clamp at the ends (no-op, `nil`); a non-arrow key → `nil`.
- Per route: `View()` contains the strip (all five labels); `←/→` emits the
  correct command (Export: `←/→` still performs date/field nav, strip present,
  switch via a letter key); `HideBreadcrumb() == true`.
- `shell`: breadcrumb renders empty when the top route is a `BreadcrumbHider`
  stub returning true, even at nav-stack depth 2.

## Constraints

- `make ci` green (lint + templ + build + tests; coverage gate ≥80%).
- Structural keys via grammar/`listnav`/`wtnav` (no new raw key checks for the
  strip nav beyond what `Lateral` centralizes). `←/→` are `tea.KeyLeft`/
  `tea.KeyRight`.
- German UI, English code/comments; hints use ` → ` connector and `  ·  `
  separator; no emoji/hex; glyphs only from `ui/glyphs` (strip via `tabstrip`).

## Out of scope

- Mouse/click on the strip.
- Renaming routes (Stats stays Stats).
- Changing Export's internal `←/→` semantics.
- The deferred markdown-viewer `g`/`G` scroll keys (separate `tui-usability`
  follow-up).
