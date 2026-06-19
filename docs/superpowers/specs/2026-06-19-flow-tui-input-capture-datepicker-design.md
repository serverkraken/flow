# flow TUI — Input-Capture-Fix + Date-Picker — Design Spec

**Status:** approved (brainstorm 2026-06-19)
**Scope:** Fix the shell swallowing keystrokes meant for focused fields, then add a reusable segment-stepper date picker and wire it into the Frei and Export routes.

---

## Problem

The sidekick shell (`internal/tui/shell/shell.go`) applies single-key global shortcuts **unconditionally** in `handleKey`, before forwarding to the active route:

- digits `1`–`9` → switch tab
- `Tab` / `Shift+Tab` → switch tab
- `Esc` → pop the active nav-stack
- `q` → quit, `:` → palette, `?` → help

Because these always win, **no text field anywhere under the shell can receive those characters.** Concretely, typing a date `2026-07-20` into the Frei "Anlegen" dialog drops every `1`–`9` (only `0` survives); `Tab` switches the top tab instead of the dialog field; `Esc` pops the whole route instead of closing the dialog. This breaks every typed field under the shell:

| Route | Affected fields |
|---|---|
| Today | booking project filter; edit-session `HH:MM` start/stop, tag, note |
| Frei (`dayoffs`) | Anlegen Von/Bis/Label; Tagesziel (digit minutes) |
| Export | von / bis / Pfad |

The pure-command routes (Woche, Stats) have no free-text fields and must keep their shortcuts.

Separately, entering dates as raw `YYYY-MM-DD` text is error-prone; a guided picker is wanted. Bubbles v2 (v2.1.0) ships **no** date picker, and community pickers target Bubbles v1 — so it must be a small custom component.

## Goals

1. Keystrokes reach the focused field/dialog when a route is in input mode; shell shortcuts still work in list/command views.
2. A reusable `internal/tui/ui/datepicker` segment-stepper component.
3. Date entry in Frei (Von/Bis) and Export (von/bis) uses the picker.

## Non-Goals

- No calendar-grid or hybrid picker (segment-stepper chosen).
- No date picker for `HH:MM` time fields, project name, or the Export path — those stay text inputs (they only need Phase A).
- No new server surface; this is TUI-only.

---

## Architecture — three independently shippable phases

**Phase A — global input-capture fix.** Foundation; unblocks every field immediately.
**Phase B — `ui/datepicker` component.** Isolated + unit-tested, not yet wired.
**Phase C — wiring** into Frei and Export.

Phases A and B are independent; C depends on both.

---

## Phase A — `shell.InputCapturer`

A **separate, optional** interface — NOT added to the core `shell.Route` interface, so existing routes, stubs, and the Home route are unaffected:

```go
// InputCapturer lets a route tell the Shell it is in text-entry mode, so the
// Shell forwards all keys to it instead of consuming them as global shortcuts.
type InputCapturer interface{ CapturesInput() bool }
```

**Shell gate.** At the very top of `Shell.handleKey` (mirroring the existing `paletteOpen` early-return), before the global `switch`:

```go
if ic, ok := s.tabs[s.activeTab].Top().(InputCapturer); ok && ic.CapturesInput() {
    return s, s.tabs[s.activeTab].UpdateTop(k)
}
```

When capturing, **all** keys go to the route — digits, `Tab`, `Esc`, `q`, `:`, `?`. Only `Ctrl+C` remains a hard global (it is handled by Bubble Tea / a guard outside this gate; the palette early-return stays above the gate too).

**Esc contract.** Because `Esc` now reaches capturing routes, each capturing route owns `Esc`:
- `dayoffs`, `Today` — `Esc` closes the open dialog (already implemented in their dialog key handlers).
- `export` — has no list state and currently relies on the shell to pop on `Esc`; it **gains** an `Esc` handler that emits `shell.PopRouteMsg{}` to return to Today.

**Per-route `CapturesInput()`:**
- `dayoffs.CapturesInput()` → `r.dialog != dialogNone` (any open dialog, incl. the Bundesland `j/k` picker and the delete confirm — in all of them keys belong to the dialog, not the shell).
- `Today.CapturesInput()` → `r.dialog != dialogNone`.
- `export.CapturesInput()` → focus is on a text/picker field (`von`, `bis`, `Pfad`), i.e. NOT on the `Range`/`Format` choice fields. On the choice fields, globals + `w/t/d/e` lateral nav keep working (matches export's existing focus-based nav gating).

Routes that don't implement the interface (Home, Woche, Stats, all `ui` stubs) are unchanged — the type assertion simply fails and the global switch runs as today.

---

## Phase B — `internal/tui/ui/datepicker`

A standalone, theme-aware component with no SSE and pure local state. API parallels `form.NewTextInput` so routes embed it the same way:

```go
type Model struct{ /* y, m, d ints; seg (0=year,1=month,2=day); focused bool; pal */ }

func New(initial time.Time, pal theme.Palette) Model
func (m Model) Update(k tea.KeyPressMsg) Model   // pure value editor, no tea.Cmd, no clock
func (m Model) View() string            // e.g. "‹2026›-‹07›-‹20›", focused segment accented
func (m Model) Value() string           // "YYYY-MM-DD"
func (m *Model) SetValue(s string) error // parse "YYYY-MM-DD"; ignore/return err on bad input
func (m *Model) Focus()
func (m *Model) Blur()
func (m Model) Focused() bool
```

The component is **clock-free and deterministic** — it never reads the host time. (No `tea.Cmd` return; no `time.Now`.)

**Key behavior** (only consumed while focused; the embedding route routes keys to it):
- `←` / `→` — move the active segment (year → month → day), clamped at the ends (no wrap across segments).
- `↑` / `↓` — increment/decrement the active segment. Month and day **roll over** within their range (month 12→1; day wraps within the current month length). Year is unbounded (sane floor, e.g. ≥ 1970). After any month/year change, the day is **clamped to the valid length of the resulting month** (e.g. 31 Jan → switch to Feb ⇒ 28/29).
- digit keys — fill the focused segment left-to-right; once a segment is "full" (4 digits year, 2 digits month/day) or the next digit can't extend it, advance to the next segment. Invalid partials are clamped on blur/`Value()` (e.g. month `19` clamps to `12`).
- `t` (jump-to-today) is **NOT** in the component — it stays clock-free. The embedding route owns `t`: when a picker is focused and `t` is pressed, the route calls `picker.SetValue(r.now())` (routes already inject `now func() time.Time` for determinism — `export` and `Today` have it; `dayoffs` adopts the same).

**No empty state.** A stepper always holds a valid date. This removes the old "Bis leer = wie Von" affordance (see Phase C).

**Rendering.** Single line; the focused segment is wrapped in accent-colored guillemets/markers via `theme`. Non-focused segments plain. Width is fixed and small, so it drops into any dialog row.

**Edge cases to cover in tests:** segment clamping at ends, month/day rollover, day clamping on month/year change (Jan 31 → Feb), multi-digit entry + auto-advance, partial/invalid digit clamp, `Value()`/`SetValue()` round-trip, leap-year Feb 29. (The `t`=today shortcut is tested at the route level, not here.)

---

## Phase C — wiring

### Frei (`dayoffs`) Anlegen dialog
The add form becomes `[Von datepicker] [Bis datepicker] [Label textinput]`.
- `Tab` / `↑` / `↓` move focus **between** the three widgets (the existing `addCur`/`addFocus` mechanism), `←` / `→` move segments **within** the focused picker — no conflict.
- `Bis` defaults to `Von`'s value when the dialog opens; a single day-off = `Von == Bis`.
- On submit: validate `Bis ≥ Von` (else keep the dialog open with a hint); send `AddDayOffs(from=Von, to=Bis, kind="urlaub", label, …)`. The old "leave Bis empty" path is gone.
- `kind`/skip-weekends behavior unchanged.

### Export route
- `von` / `bis` text fields become datepickers; `Pfad` stays a textinput; `Range`/`Format` stay choice-cyclers.
- Selecting a `Range` preset (kw/monat/letzter) fills both pickers via `SetValue`; changing a picker manually sets `preset = "custom"` (same trigger as today).
- `←` / `→` act on the focused field: cycle on `Range`/`Format`, move segment on `von`/`bis`.
- Add the `Esc` → `shell.PopRouteMsg{}` handler (per Phase A Esc contract).
- `submit()` validation (`to ≥ from`, valid dates) is simplified — the picker guarantees well-formed dates, so the parse-error branch becomes a `to < from` check only.

---

## Testing

- **Phase A** (`shell_test.go`): a capturing stub route — when `CapturesInput()` is true, feeding `2`, `Tab`, `Esc` forwards them to the route (assert via the route receiving them / no tab-switch / no pop); when false, the shortcuts still fire. Cover that a non-`InputCapturer` route is unaffected.
- **Phase B** (`datepicker_test.go`): all edge cases listed above, driven through `Update(tea.KeyPressMsg)` and asserted via `Value()`.
- **Phase C** (`dayoffs`/`export` reducer tests): open Frei add → set Von/Bis via picker keys → submit calls `AddDayOffs` with the right range; `Bis < Von` blocks submit; Export `Range` preset fills both pickers; Export `Esc` emits `PopRouteMsg`.

CI gate: `make ci` green, coverage ≥ 80%.

---

## File structure

**New:**
- `internal/tui/ui/datepicker/datepicker.go` + `datepicker_test.go`

**Modified:**
- `internal/tui/shell/route.go` — add `InputCapturer` interface.
- `internal/tui/shell/shell.go` — capture gate in `handleKey`.
- `internal/tui/shell/shell_test.go` — capture-gate tests.
- `internal/tui/screen/worktime/route.go` — `Today.CapturesInput()`.
- `internal/tui/screen/worktime/dayoffs/route.go` — `CapturesInput()`; `NewRoute` gains a `now func() time.Time` param (defaults to `time.Now`) for the `t`=today shortcut.
- `internal/tui/screen/worktime/dayoffs/dialogs.go` — Von/Bis datepickers in the add dialog; `t`=today; drop empty-Bis path.
- `internal/tui/screen/worktime/nav.go` — `BuildRegistry` passes `time.Now` to `dayoffs.NewRoute` (constructor ripple).
- `internal/tui/screen/worktime/export/route.go` — `CapturesInput()`, von/bis datepickers, `t`=today, `Esc` → pop, simplified validation.
- Test ripple: `dayoffs` tests pass a fixed `now`; `worktime` `BuildRegistry` test unaffected (still constructs).

---

## Out of scope / accepted

- Time (`HH:MM`) and project-name fields keep plain text input (Phase A alone fixes them).
- Bundesland picker stays a `j/k` list (not a text field).
- The `dayoffs` "kind" is always `urlaub` from this dialog (unchanged); choosing a kind is a separate future enhancement.
