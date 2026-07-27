# Worktime Day-Off Categories Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the user pick one of 7 day-off categories (Feiertag, Urlaub, Krank, Gleittag, Sonderurlaub, Kind krank, Fortbildung) when adding a day-off, and show each category as a colored glyph + German label consistently across Frei, Woche, WebUI, and CLI.

**Architecture:** The domain already models a day-off `Kind` end-to-end; the `day_offs.kind` column is free-text validated via `domain.ParseKind`, so **no DB migration**. We add 4 new `Kind` constants, a single-source `kindcolor.DayOffColor` helper, a TUI category-picker dialog, and extend the WebUI `<select>` + CLI flag help. All categories behave identically for the target/saldo (a full day off, Target 0) — categories are label + color only.

**Tech Stack:** Go, charm.land/bubbletea/v2 + bubbles/v2 + lipgloss/v2, templ (WebUI), cobra (CLI), Postgres (pgstore), TDD.

## Global Constraints

- Charm imports use `charm.land/...` v2 paths (`charm.land/lipgloss/v2`, `charm.land/bubbletea/v2`).
- Screen/TUI packages use semantic colors via `theme.Palette.Sem()` — never raw hues. `kindcolor.DayOffColor` reads `Sem()`, keeping consumers inside the Sem-only screen-hue rule.
- No DB migration — `day_offs.kind` is free-text; server validates via `domain.ParseKind`.
- `KindHoliday` is never user-selectable or stored manually (`AddDayOffs` returns `ErrHolidayNotManual`).
- Keep files focused (no monoliths); follow existing patterns in each package.
- `make ci` must stay green (lint + `go tool templ generate` drift check + build + tests + coverage gate ~83%).
- Generated `*_templ.go` must be regenerated with `make generate` after editing any `.templ`.
- The 7 kinds and their stored values / labels / colors are fixed:

  | Kind const | stored value | LabelDe() | semantic color |
  |---|---|---|---|
  | KindHoliday | `holiday` | Feiertag | Schedule (blue) |
  | KindVacation | `vacation` | Urlaub | Highlight (purple) |
  | KindSick | `sick` | Krank | Notice (orange) |
  | KindFlex | `flex` | Gleittag | Success (green) |
  | KindSpecial | `special` | Sonderurlaub | Warning (yellow) |
  | KindChildSick | `childsick` | Kind krank | Danger (red) |
  | KindTraining | `training` | Fortbildung | Info (cyan) |

- Selectable (manual) kinds, in picker order: `vacation, sick, flex, special, childsick, training` (everything except holiday).

---

## File Structure

- `internal/domain/dayoff.go` — add 4 Kind constants, `SelectableKinds`, extend `AllKinds` / `LabelDe` / `ParseKind` (Task 1)
- `internal/tui/kindcolor/dayoff.go` — new: `DayOffColor(domain.Kind, theme.Palette) theme.Color` (Task 2)
- `internal/tui/screen/worktime/week/pacedot.go` — delegate day-off color to `kindcolor.DayOffColor` (Task 3)
- `internal/tui/screen/worktime/dayoffs/dialogs.go` — Kategorie form field, `fgColor` helper, default kind, submit uses picked kind (Task 4); kind-picker dialog (Task 5)
- `internal/tui/screen/worktime/dayoffs/route.go` — colored glyph + German label in list (Task 6)
- `internal/adapter/webui/dayoffs.templ` (+ regenerated `dayoffs_templ.go`) — 4 new `<option>`s (Task 7)
- `cmd/flow/dayoff.go` — `--kind` flag help text (Task 7)

---

## Task 1: Domain — 4 new kinds, SelectableKinds, ParseKind, LabelDe

**Files:**
- Modify: `internal/domain/dayoff.go`
- Test: `internal/domain/dayoff_test.go`

**Interfaces:**
- Produces: `domain.KindFlex` (`"flex"`), `domain.KindSpecial` (`"special"`), `domain.KindChildSick` (`"childsick"`), `domain.KindTraining` (`"training"`); `domain.SelectableKinds []domain.Kind` (the 6 manual kinds, picker order); extended `AllKinds`, `LabelDe()`, `ParseKind()`.

- [ ] **Step 1: Write the failing tests**

In `internal/domain/dayoff_test.go`, extend `TestKind_LabelDe`'s table with:

```go
		{domain.KindFlex, "Gleittag"},
		{domain.KindSpecial, "Sonderurlaub"},
		{domain.KindChildSick, "Kind krank"},
		{domain.KindTraining, "Fortbildung"},
```

Extend `TestParseKind`'s table with:

```go
		// flex aliases
		{"flex", domain.KindFlex, true},
		{"gleittag", domain.KindFlex, true},
		{"Gleit", domain.KindFlex, true},
		// special aliases
		{"special", domain.KindSpecial, true},
		{"sonderurlaub", domain.KindSpecial, true},
		// child-sick aliases
		{"childsick", domain.KindChildSick, true},
		{"kindkrank", domain.KindChildSick, true},
		{"Kind krank", domain.KindChildSick, true},
		// training aliases
		{"training", domain.KindTraining, true},
		{"fortbildung", domain.KindTraining, true},
		{"schulung", domain.KindTraining, true},
```

Replace `TestAllKinds_CoversConstants`'s `want` map with all 7 kinds:

```go
	want := map[domain.Kind]bool{
		domain.KindHoliday:   false,
		domain.KindVacation:  false,
		domain.KindSick:      false,
		domain.KindFlex:      false,
		domain.KindSpecial:   false,
		domain.KindChildSick: false,
		domain.KindTraining:  false,
	}
```

Add a new test for `SelectableKinds`:

```go
func TestSelectableKinds_ExcludesHolidayCoversManual(t *testing.T) {
	want := map[domain.Kind]bool{
		domain.KindVacation:  false,
		domain.KindSick:      false,
		domain.KindFlex:      false,
		domain.KindSpecial:   false,
		domain.KindChildSick: false,
		domain.KindTraining:  false,
	}
	for _, k := range domain.SelectableKinds {
		if k == domain.KindHoliday {
			t.Fatal("SelectableKinds must not contain KindHoliday (computed, not manual)")
		}
		if _, ok := want[k]; !ok {
			t.Errorf("SelectableKinds contains unexpected kind %q", k)
		}
		want[k] = true
	}
	for k, seen := range want {
		if !seen {
			t.Errorf("SelectableKinds missing %q", k)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/domain/ -run 'Kind|ParseKind|AllKinds|SelectableKinds' -v`
Expected: FAIL (undefined: `domain.KindFlex`, `domain.SelectableKinds`, etc.)

- [ ] **Step 3: Implement in `internal/domain/dayoff.go`**

Replace the const block, `AllKinds`, and add `SelectableKinds` (and fix the stale `.tsv` comment):

```go
// Day-off categories. Persisted as the literal string in the day_offs.kind
// column, so renaming a value requires a migration. Adding a new value does
// not (the column is free-text, validated via ParseKind).
const (
	KindHoliday   Kind = "holiday"   // gesetzlicher Feiertag
	KindVacation  Kind = "vacation"  // Urlaub
	KindSick      Kind = "sick"      // Krankheit
	KindFlex      Kind = "flex"      // Gleittag / Überstundenabbau
	KindSpecial   Kind = "special"   // Sonderurlaub
	KindChildSick Kind = "childsick" // Kind krank
	KindTraining  Kind = "training"  // Fortbildung / Schulung
)

// AllKinds enumerates valid kinds in display order. Used by UI cycling and
// CLI validation so callers don't have to repeat the list.
var AllKinds = []Kind{
	KindHoliday, KindVacation, KindSick,
	KindFlex, KindSpecial, KindChildSick, KindTraining,
}

// SelectableKinds enumerates the kinds a user may pick when adding a day-off,
// in picker display order. Excludes KindHoliday (holidays are computed from the
// Bundesland, never stored manually — see AddDayOffs.ErrHolidayNotManual).
var SelectableKinds = []Kind{
	KindVacation, KindSick, KindFlex,
	KindSpecial, KindChildSick, KindTraining,
}
```

Extend `LabelDe()` with the 4 new cases (before the `return string(k)` fallthrough):

```go
	case KindFlex:
		return "Gleittag"
	case KindSpecial:
		return "Sonderurlaub"
	case KindChildSick:
		return "Kind krank"
	case KindTraining:
		return "Fortbildung"
```

Extend `ParseKind()` with the 4 new cases (before the final `return "", false`):

```go
	case "flex", "gleittag", "gleit":
		return KindFlex, true
	case "special", "sonderurlaub", "sonder":
		return KindSpecial, true
	case "childsick", "kindkrank", "kind krank":
		return KindChildSick, true
	case "training", "fortbildung", "schulung":
		return KindTraining, true
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/domain/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/dayoff.go internal/domain/dayoff_test.go
git commit -m "feat(domain): add 4 day-off kinds + SelectableKinds (Gleittag/Sonderurlaub/Kind krank/Fortbildung)"
```

---

## Task 2: kindcolor.DayOffColor single-source helper

**Files:**
- Create: `internal/tui/kindcolor/dayoff.go`
- Test: `internal/tui/kindcolor/dayoff_test.go`

**Interfaces:**
- Consumes: `domain.Kind` constants (Task 1), `theme.Palette.Sem()`.
- Produces: `kindcolor.DayOffColor(k domain.Kind, p theme.Palette) theme.Color`.

- [ ] **Step 1: Write the failing test**

Create `internal/tui/kindcolor/dayoff_test.go`:

```go
package kindcolor_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/kindcolor"
	"github.com/serverkraken/flow/internal/tui/theme"
)

func TestDayOffColor_PerKind(t *testing.T) {
	t.Parallel()
	p := theme.Default
	sem := p.Sem()
	cases := []struct {
		k    domain.Kind
		want theme.Color
	}{
		{domain.KindHoliday, sem.Schedule},
		{domain.KindVacation, sem.Highlight},
		{domain.KindSick, sem.Notice},
		{domain.KindFlex, sem.Success},
		{domain.KindSpecial, sem.Warning},
		{domain.KindChildSick, sem.Danger},
		{domain.KindTraining, sem.Info},
	}
	for _, c := range cases {
		if got := kindcolor.DayOffColor(c.k, p); got != c.want {
			t.Errorf("DayOffColor(%q) = %v, want %v", c.k, got, c.want)
		}
	}
}

func TestDayOffColor_UnknownFallsBackToMuted(t *testing.T) {
	t.Parallel()
	p := theme.Default
	if got := kindcolor.DayOffColor(domain.Kind("nonsense"), p); got != p.FgMuted {
		t.Errorf("DayOffColor(unknown) = %v, want FgMuted %v", got, p.FgMuted)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/kindcolor/ -run DayOffColor -v`
Expected: FAIL (undefined: `kindcolor.DayOffColor`)

- [ ] **Step 3: Implement `internal/tui/kindcolor/dayoff.go`**

```go
package kindcolor

import (
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// DayOffColor maps a day-off Kind to its semantic color. Single source of truth
// so the Frei list glyph and the Woche pace dots can never drift. Unknown kinds
// fall back to the muted foreground.
//
// Holiday=Schedule (blue, calendar event), Vacation=Highlight (purple,
// Urlaub-identity), Sick=Notice (orange, Krank-class) match the semantic.go
// role tokens; Flex/Special/ChildSick/Training take the remaining distinct hues.
func DayOffColor(k domain.Kind, p theme.Palette) theme.Color {
	sem := p.Sem()
	switch k {
	case domain.KindHoliday:
		return sem.Schedule
	case domain.KindVacation:
		return sem.Highlight
	case domain.KindSick:
		return sem.Notice
	case domain.KindFlex:
		return sem.Success
	case domain.KindSpecial:
		return sem.Warning
	case domain.KindChildSick:
		return sem.Danger
	case domain.KindTraining:
		return sem.Info
	}
	return p.FgMuted
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/kindcolor/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/kindcolor/dayoff.go internal/tui/kindcolor/dayoff_test.go
git commit -m "feat(tui): kindcolor.DayOffColor — single-source day-off kind→color map"
```

---

## Task 3: Woche pace dots delegate to DayOffColor

**Files:**
- Modify: `internal/tui/screen/worktime/week/pacedot.go`
- Test: `internal/tui/screen/worktime/week/pacedot_test.go`

**Interfaces:**
- Consumes: `kindcolor.DayOffColor` (Task 2).
- Produces: unchanged `paceColor` signature; day-off branch now covers all 7 kinds.

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/screen/worktime/week/pacedot_test.go` (note: this is the internal `week` package, so import theme/kindcolor/domain):

```go
func TestPaceColor_DayOffKindsMatchKindcolor(t *testing.T) {
	p := theme.Default
	for _, k := range []string{"holiday", "vacation", "sick", "flex", "special", "childsick", "training"} {
		off := &apiclient.DayOff{Kind: k}
		want := kindcolor.DayOffColor(domain.Kind(k), p)
		if got := paceColor(paceDotDayOff, off, p); got != want {
			t.Errorf("paceColor(dayoff %q) = %v, want %v", k, got, want)
		}
	}
}
```

Add the needed imports to the test file's import block:

```go
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/kindcolor"
	"github.com/serverkraken/flow/internal/tui/theme"
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/week/ -run PaceColor -v`
Expected: FAIL (new kinds map to `p.FgMuted` from the old default branch, not the kindcolor hues)

- [ ] **Step 3: Implement — replace the day-off branch in `paceColor`**

In `internal/tui/screen/worktime/week/pacedot.go`, replace the `case paceDotDayOff:` block inside `paceColor` with a delegation:

```go
	case paceDotDayOff:
		if off != nil {
			return kindcolor.DayOffColor(domain.Kind(off.Kind), p)
		}
		return p.FgMuted
```

Update the package import block to add `"github.com/serverkraken/flow/internal/tui/kindcolor"` (keep `domain`, `theme`, `glyphs`, `apiclient`). Update the doc comment above `paceColor` to say it delegates day-off hues to `kindcolor.DayOffColor`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/screen/worktime/week/ -v`
Expected: PASS (existing `TestClassifyPaceDot` / `TestPaceGlyph` stay green; new color test passes)

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen/worktime/week/pacedot.go internal/tui/screen/worktime/week/pacedot_test.go
git commit -m "refactor(worktime): Woche pace dots use kindcolor.DayOffColor (covers 4 new kinds)"
```

---

## Task 4: TUI add dialog — Kategorie field + submit picked kind

**Files:**
- Modify: `internal/tui/screen/worktime/dayoffs/dialogs.go`
- Test: `internal/tui/screen/worktime/dayoffs/route_test.go`

**Interfaces:**
- Consumes: `domain.SelectableKinds`, `domain.Kind.LabelDe()` (Task 1), `kindcolor.DayOffColor` (Task 2).
- Produces: `dialogState.kind domain.Kind` (default `KindVacation`); add form has 4 fields `0 Von · 1 Bis · 2 Kategorie · 3 Label`; `submitAdd` sends `string(r.dlg.kind)`; package-level `fgColor(s string, c theme.Color) string` helper (reused by Task 6).

- [ ] **Step 1: Update the fake API + adjust existing add tests to the new field order, and add a default-kind test**

In `internal/tui/screen/worktime/dayoffs/route_test.go`:

(a) Add a captured field to `fakeAPI` and record the kind:

```go
type fakeAPI struct {
	list       []apiclient.DayOff
	settings   apiclient.Settings
	deleted    string
	addedFrom  string
	addedKind  string
	bundesland string
	listErr    error
}
```

```go
func (f *fakeAPI) AddDayOffs(_ context.Context, from, _, kind, _ string, _ int, _ bool) error {
	f.addedFrom = from
	f.addedKind = kind
	return nil
}
```

(b) `TestDayOffsRoute_addViaDatepicker` now needs one extra Tab (Label is field 3). Replace the two `Tab` lines with three and assert the kind too:

```go
	// Tab Von→Bis→Kategorie→Label (Label is now field 3).
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Bis
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Kategorie
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Label
	for _, c := range []string{"U", "r", "l", "a", "u", "b"} {
		r, _ = r.Update(tea.KeyPressMsg{Text: c})
	}
	r2, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // submit on last field
	_ = drain(r2, cmd)
	if api.addedFrom != "2026-06-20" {
		t.Fatalf("addedFrom = %q, want 2026-06-20", api.addedFrom)
	}
	if api.addedKind != "vacation" {
		t.Fatalf("addedKind = %q, want vacation (default)", api.addedKind)
	}
```

(c) `TestDayOffsRoute_addRejectsBisBeforeVon` now needs one extra Tab between Bis and Label. Replace the single `Tab // Label` after setting Bis with:

```go
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Kategorie
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Label
```

(d) Add a focused default-kind test:

```go
// TestDayOffsRoute_addSubmitsDefaultKind verifies that submitting the add form
// without touching the category sends the default kind "vacation".
func TestDayOffsRoute_addSubmitsDefaultKind(t *testing.T) {
	api := &fakeAPI{}
	r := drain(newRoute(api), nil)
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "a"})   // open add; Von focused (today=2026-06-18)
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Bis
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Kategorie
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Label
	r2, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // submit
	_ = drain(r2, cmd)
	if api.addedKind != "vacation" {
		t.Fatalf("addedKind = %q, want vacation", api.addedKind)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/screen/worktime/dayoffs/ -run 'addViaDatepicker|addRejectsBisBeforeVon|addSubmitsDefaultKind' -v`
Expected: FAIL (`addedKind` always `"urlaub"` from the hardcoded submit; field-order tabs land wrong)

- [ ] **Step 3: Implement the Kategorie field in `internal/tui/screen/worktime/dayoffs/dialogs.go`**

(a) Add imports `"charm.land/lipgloss/v2"`, `"github.com/serverkraken/flow/internal/domain"`, `"github.com/serverkraken/flow/internal/tui/kindcolor"`, and `"github.com/serverkraken/flow/internal/tui/theme"` to the import block.

(b) Add a `kind` field to `dialogState`:

```go
type dialogState struct {
	target    string
	vonDP     datepicker.Model
	bisDP     datepicker.Model
	bisEdited bool
	label     textinput.Model
	kind      domain.Kind
	addCur    int // 0=Von, 1=Bis, 2=Kategorie, 3=Label
	confirm   confirm.Model
	blSel     int
}
```

(c) In `openAdd`, default the kind:

```go
	r.dlg.kind = domain.KindVacation
```

(d) Replace `handleAddKey`'s `KeyEnter` case and the trailing field switch to use 4 fields. The Enter handler:

```go
	case tea.KeyEnter:
		if r.dlg.addCur == 3 {
			return r, r.submitAdd()
		}
		r.addFocus(+1)
		return r, nil
	}
	switch r.dlg.addCur {
	case 0: // Von — mirror into Bis while Bis is untouched
		if k.Text == "t" {
			_ = r.dlg.vonDP.SetValue(r.now().Format("2006-01-02"))
		} else {
			r.dlg.vonDP = r.dlg.vonDP.Update(k)
		}
		if !r.dlg.bisEdited {
			_ = r.dlg.bisDP.SetValue(r.dlg.vonDP.Value())
		}
	case 1: // Bis — latch bisEdited when the value actually changes
		if k.Text == "t" {
			_ = r.dlg.bisDP.SetValue(r.now().Format("2006-01-02"))
			r.dlg.bisEdited = true
		} else {
			before := r.dlg.bisDP.Value()
			r.dlg.bisDP = r.dlg.bisDP.Update(k)
			if r.dlg.bisDP.Value() != before {
				r.dlg.bisEdited = true
			}
		}
	case 2: // Kategorie — picker only (Task 5); ignore typed keys here
	case 3: // Label
		var cmd tea.Cmd
		r.dlg.label, cmd = r.dlg.label.Update(k)
		return r, cmd
	}
	return r, nil
```

(e) Replace `addFocus` to cycle 4 fields:

```go
func (r *Route) addFocus(delta int) {
	r.dlg.addCur = (r.dlg.addCur + delta + 4) % 4
	r.dlg.vonDP.Blur()
	r.dlg.bisDP.Blur()
	r.dlg.label.Blur()
	switch r.dlg.addCur {
	case 0:
		r.dlg.vonDP.Focus()
	case 1:
		r.dlg.bisDP.Focus()
	case 3:
		_ = r.dlg.label.Focus()
	}
	// case 2 (Kategorie) has no focusable widget.
}
```

(f) In `submitAdd`, replace the hardcoded `"urlaub"` with the picked kind:

```go
	kindStr := string(r.dlg.kind)
	api := r.api
	r.dialog = dialogNone
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := api.AddDayOffs(ctx, from, to, kindStr, label, 0, true); err != nil {
			return loadedMsg{err: err}
		}
		return reloadMsg{}
	}
```

(g) In `renderDialog`'s `dialogAdd` case, render the Kategorie line between Bis and Label:

```go
		fmt.Fprintf(&b, "  Von       %s\n", r.dlg.vonDP.View())
		fmt.Fprintf(&b, "  Bis       %s\n", r.dlg.bisDP.View())
		catMarker := "  "
		if r.dlg.addCur == 2 {
			catMarker = "▸ "
		}
		catLabel := fgColor(r.dlg.kind.LabelDe(), kindcolor.DayOffColor(r.dlg.kind, f.Pal))
		fmt.Fprintf(&b, "  Kategorie %s%s\n", catMarker, catLabel)
		fmt.Fprintf(&b, "  Label     %s\n", r.dlg.label.View())
```

(Keep the existing `vonDP.Calendar` / `bisDP.Calendar` block below for `addCur` 0/1.)

(h) Add the package-level helper (used here and in Task 6):

```go
// fgColor renders s in an arbitrary theme.Color. theme exposes no generic Fg
// builder (builders cover named semantic roles only), so we use lipgloss here.
func fgColor(s string, c theme.Color) string {
	return lipgloss.NewStyle().Foreground(c).Render(s)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/screen/worktime/dayoffs/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen/worktime/dayoffs/dialogs.go internal/tui/screen/worktime/dayoffs/route_test.go
git commit -m "feat(worktime): add Kategorie field to Frei add dialog; submit picked kind (default Urlaub)"
```

---

## Task 5: TUI kind-picker dialog

**Files:**
- Modify: `internal/tui/screen/worktime/dayoffs/dialogs.go`
- Test: `internal/tui/screen/worktime/dayoffs/route_test.go`

**Interfaces:**
- Consumes: `dialogState.kind` (Task 4), `domain.SelectableKinds`, `kindcolor.DayOffColor`, `picker.Row`, `listnav`.
- Produces: new `dialogKindPick` dialog opened by Enter on the Kategorie field; `dialogState.kindSel int`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/tui/screen/worktime/dayoffs/route_test.go`:

```go
// TestDayOffsRoute_kindPickerSelectsKind opens the add form, enters the kind
// picker from the Kategorie field, moves down once (vacation→sick), confirms,
// then submits — addedKind must be "sick".
func TestDayOffsRoute_kindPickerSelectsKind(t *testing.T) {
	api := &fakeAPI{}
	r := drain(newRoute(api), nil)
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "a"})        // open add; Von focused
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Bis
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Kategorie
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // open kind picker
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyDown})  // vacation(0) -> sick(1)
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // confirm -> back to add form
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab})   // Kategorie -> Label
	r2, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // submit
	_ = drain(r2, cmd)
	if api.addedKind != "sick" {
		t.Fatalf("addedKind = %q, want sick", api.addedKind)
	}
}

// TestDayOffsRoute_kindPickerView verifies the picker lists German kind labels.
func TestDayOffsRoute_kindPickerView(t *testing.T) {
	r := newRoute(&fakeAPI{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "a"})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // open picker
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	for _, want := range []string{"Gleittag", "Sonderurlaub", "Kind krank", "Fortbildung"} {
		if !strings.Contains(body, want) {
			t.Fatalf("kind picker missing %q; got:\n%s", want, body)
		}
	}
}

// TestDayOffsRoute_kindPickerEsc returns to the add form without changing kind.
func TestDayOffsRoute_kindPickerEsc(t *testing.T) {
	r := newRoute(&fakeAPI{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "a"})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // open picker
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyEsc})   // back to add form
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Frei-Tag anlegen") {
		t.Fatalf("Esc from kind picker should return to add form; got:\n%s", body)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/screen/worktime/dayoffs/ -run kindPicker -v`
Expected: FAIL (Enter on Kategorie advances instead of opening a picker; no German labels rendered)

- [ ] **Step 3: Implement the kind picker in `internal/tui/screen/worktime/dayoffs/dialogs.go`**

(a) Add the dialog constant:

```go
const (
	dialogNone dialogKind = iota
	dialogTarget
	dialogAdd
	dialogDelete
	dialogBundesland
	dialogKindPick
)
```

(b) Add `kindSel int` to `dialogState`.

(c) In `handleAddKey`, change the Enter handler so Enter on the Kategorie field (field 2) opens the picker:

```go
	case tea.KeyEnter:
		switch r.dlg.addCur {
		case 2: // Kategorie — open the kind picker
			return r.openKindPick()
		case 3: // Label — submit
			return r, r.submitAdd()
		}
		r.addFocus(+1)
		return r, nil
	}
```

(d) Add the open + key handler:

```go
func (r *Route) openKindPick() (shell.Route, tea.Cmd) {
	r.dlg.kindSel = 0
	for i, k := range domain.SelectableKinds {
		if k == r.dlg.kind {
			r.dlg.kindSel = i
		}
	}
	r.dialog = dialogKindPick
	return r, nil
}

func (r *Route) handleKindPickKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	n := len(domain.SelectableKinds)
	if cur, ok := listnav.New().Set(r.dlg.kindSel, n).Handle(k, n, 6); ok {
		r.dlg.kindSel = cur.Index()
		return r, nil
	}
	switch k.Code {
	case tea.KeyEsc:
		r.dialog = dialogAdd
	case tea.KeyEnter:
		r.dlg.kind = domain.SelectableKinds[r.dlg.kindSel]
		r.dialog = dialogAdd
	}
	return r, nil
}
```

(e) Route it in `handleDialogKey`:

```go
	case dialogKindPick:
		return r.handleKindPickKey(k)
```

(f) Render it in `renderDialog`:

```go
	case dialogKindPick:
		var b strings.Builder
		b.WriteString("\n  Kategorie wählen (↑/↓ · enter · esc)\n\n")
		for i, k := range domain.SelectableKinds {
			label := fgColor(k.LabelDe(), kindcolor.DayOffColor(k, f.Pal))
			b.WriteString(picker.Row(i == r.dlg.kindSel, label, "", f.Width-4, f.Pal) + "\n")
		}
		return b.String()
```

(g) Add hints in `dialogHints`:

```go
	case dialogKindPick:
		return []keyhint.Hint{{Key: "↑/↓", Desc: "wählen"}, {Key: "enter", Desc: "setzen"}, {Key: "esc", Desc: "zurück"}}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/screen/worktime/dayoffs/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen/worktime/dayoffs/dialogs.go internal/tui/screen/worktime/dayoffs/route_test.go
git commit -m "feat(worktime): kind-picker dialog for Frei add (↑/↓ list of 6 categories)"
```

---

## Task 6: Frei list — colored glyph + German category label

**Files:**
- Modify: `internal/tui/screen/worktime/dayoffs/route.go`
- Test: `internal/tui/screen/worktime/dayoffs/route_test.go`

**Interfaces:**
- Consumes: `kindcolor.DayOffColor`, `domain.Kind.LabelDe()`, package-level `fgColor` (Task 4).
- Produces: list rows render `<colored ○> <day>  <Kategorie>[ — <label>]`.

- [ ] **Step 1: Write the failing test**

Add to `internal/tui/screen/worktime/dayoffs/route_test.go`:

```go
// TestDayOffsRoute_listShowsCategoryLabel verifies the German category label is
// rendered for a non-holiday entry (here a Gleittag), independent of free-text label.
func TestDayOffsRoute_listShowsCategoryLabel(t *testing.T) {
	api := &fakeAPI{
		list:     []apiclient.DayOff{{Day: "2026-09-10", Kind: "flex", Label: ""}},
		settings: apiclient.Settings{DefaultTargetMin: 480},
	}
	r := newRoute(api)
	r = drain(r, r.Init())
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Gleittag") {
		t.Fatalf("list should show category label 'Gleittag'; got:\n%s", body)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/dayoffs/ -run listShowsCategoryLabel -v`
Expected: FAIL (label falls back to raw `"flex"`, not `"Gleittag"`)

- [ ] **Step 3: Implement in `internal/tui/screen/worktime/dayoffs/route.go`**

(a) Add import `"github.com/serverkraken/flow/internal/tui/kindcolor"`.

(b) Replace the list-rendering loop in `View`:

```go
	for i, d := range r.list {
		k := domain.Kind(d.Kind)
		cat := k.LabelDe()
		text := cat
		if d.Label != "" && d.Label != cat {
			text += " — " + d.Label
		}
		glyph := fgColor("○", kindcolor.DayOffColor(k, f.Pal))
		row := fmt.Sprintf("  %s %s  %s", glyph, d.Day, text)
		if i == r.cursor {
			row = theme.Active(row, f.Pal)
		}
		b.WriteString(row + "\n")
	}
```

(c) Delete the now-unused `dayOffGlyph` function.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/screen/worktime/dayoffs/ -v`
Expected: PASS (existing list tests stay green: "Weihnachten", "Tag der Einheit", "Urlaub" still appear)

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen/worktime/dayoffs/route.go internal/tui/screen/worktime/dayoffs/route_test.go
git commit -m "feat(worktime): Frei list shows colored kind glyph + German category label"
```

---

## Task 7: WebUI select options + CLI flag help

**Files:**
- Modify: `internal/adapter/webui/dayoffs.templ` (+ regenerate `internal/adapter/webui/dayoffs_templ.go`)
- Modify: `cmd/flow/dayoff.go`
- Test: `internal/adapter/httpserver/webui_dayoffs_test.go`

**Interfaces:**
- Consumes: server-side `domain.ParseKind` already accepts the new values (Task 1).
- Produces: WebUI `<select name="kind">` offers all 6 manual kinds; CLI `--kind` help lists them.

- [ ] **Step 1: Write the failing test**

Add to `internal/adapter/httpserver/webui_dayoffs_test.go` a test that the rendered day-offs page lists all manual kind options. Reuse the existing `newWebDayOffServer(t)` helper and the same `httptest` + cookie pattern as `TestWebDayOffPageAndMutations`:

```go
func TestWebDayOffPage_ListsAllManualKinds(t *testing.T) {
	srv, codec := newWebDayOffServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	req, _ := http.NewRequest("GET", ts.URL+"/dayoffs", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	body := string(b)

	for _, want := range []string{"Urlaub", "Krank", "Gleittag", "Sonderurlaub", "Kind krank", "Fortbildung"} {
		if !strings.Contains(body, want) {
			t.Fatalf("day-offs page select missing %q;\n%.400s", want, body)
		}
	}
}
```

(`http`, `httptest`, `io`, `strings` are already imported in this test file.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapter/httpserver/ -run ListsAllManualKinds -v`
Expected: FAIL (only "Urlaub" and "Krank" options exist)

- [ ] **Step 3: Implement**

(a) In `internal/adapter/webui/dayoffs.templ`, extend the `<select name="kind">`:

```html
		<select name="kind" class="rounded border px-2 py-1 text-sm">
			<option value="vacation">Urlaub</option>
			<option value="sick">Krank</option>
			<option value="flex">Gleittag</option>
			<option value="special">Sonderurlaub</option>
			<option value="childsick">Kind krank</option>
			<option value="training">Fortbildung</option>
		</select>
```

(b) Regenerate the templ output:

```bash
make generate
```

(c) In `cmd/flow/dayoff.go`, update the `--kind` flag help text:

```go
	cmd.Flags().StringVar(&kind, "kind", "vacation", "vacation|sick|flex|special|childsick|training")
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapter/httpserver/ ./cmd/... -v`
Expected: PASS

Run: `git diff --quiet -- ':*_templ.go' && echo CLEAN || echo "regenerate needed"`
Expected: CLEAN (after `make generate` the generated file is committed)

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/webui/dayoffs.templ internal/adapter/webui/dayoffs_templ.go cmd/flow/dayoff.go internal/adapter/httpserver/webui_dayoffs_test.go
git commit -m "feat(webui,cli): offer all 6 manual day-off categories in select + --kind help"
```

---

## Task 8: Full CI + live done-gate

**Files:** none (verification only)

- [ ] **Step 1: Run the full CI**

Run: `make ci`
Expected: green — lint, `go tool templ generate` drift check, build, all tests, coverage gate (~83%).

- [ ] **Step 2: Live smoke vs the dev stack**

Start the dev stack and mint a token (see `reference_flow_dev_env`):

```bash
make dev-up
make dev-run    # in another shell
TOKEN=$(make -s dev-token)
```

Add one day-off per new category via REST and read it back:

```bash
for k in flex special childsick training; do
  curl -s -X POST localhost:8080/api/v1/dayoffs \
    -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
    -d "{\"from\":\"2026-09-0$((RANDOM%9+1))\",\"to\":\"2026-09-0$((RANDOM%9+1))\",\"kind\":\"$k\",\"label\":\"smoke\",\"targetMin\":0,\"skipWeekends\":true}" ;
done
curl -s "localhost:8080/api/v1/dayoffs?from=2026-09-01&to=2026-09-30" \
  -H "Authorization: Bearer $TOKEN" | grep -oE '"kind":"[a-z]+"'
```

Expected: the response contains `flex`, `special`, `childsick`, `training`.

- [ ] **Step 3: TUI + WebUI dogfood checklist (manual, by Soenne)**

- `flow ui` → Frei → `a` → Tab to Kategorie → Enter → pick "Gleittag" → fill dates → save. Confirm the new row shows a green `○` and "Gleittag".
- Frei → Woche: confirm the Gleittag day renders a green pace dot.
- WebUI `/dayoffs`: confirm the select offers all 6 manual kinds; add one and confirm it round-trips with the right label.

- [ ] **Step 4: Final confirmation**

No commit needed (verification task). Report `make ci` output and smoke results.

---

## Self-Review

**Spec coverage:**
- 7 kinds (domain) → Task 1 ✓
- No migration (free-text kind) → confirmed, Global Constraints ✓
- Single-source color helper → Task 2 ✓
- TUI add picker (separate list, per decision) → Tasks 4 + 5 ✓
- Frei list colored glyph + label → Task 6 ✓
- Woche pace dots cover new kinds → Task 3 ✓
- WebUI select + CLI help → Task 7 ✓
- Done-gate (make ci + live smoke + dogfood) → Task 8 ✓
- Deferred (Gleittag overtime, Stats breakdown) → not in plan ✓

**Placeholder scan:** none — Task 7's test now uses the real `newWebDayOffServer` helper + httptest pattern verbatim from the existing test file. All steps contain concrete code.

**Type consistency:** `dialogState.kind domain.Kind`, `kindSel int`, `addCur` 0–3 used consistently across Tasks 4–5; `kindcolor.DayOffColor(domain.Kind, theme.Palette) theme.Color` and `fgColor(string, theme.Color) string` signatures match across Tasks 2/3/4/6; `fakeAPI.addedKind` captured in Task 4 and asserted in Tasks 4/5.
