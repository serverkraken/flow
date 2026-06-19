# flow TUI — Input-Capture-Fix + Date-Picker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop the sidekick shell from swallowing keystrokes meant for focused fields, then add a reusable segment-stepper date picker (with weekday label + read-only month grid) and wire it into the Frei and Export routes.

**Architecture:** Three independently shippable phases. **Phase A** adds an optional `shell.InputCapturer` interface; when the active top route reports it is capturing input, the shell forwards *all* keys to it instead of consuming global shortcuts (digits/Tab/Esc/q/:/?). **Phase B** builds a clock-free `internal/tui/ui/datepicker` component. **Phase C** wires the picker into Frei (Von/Bis) and Export (von/bis).

**Tech Stack:** Go, charm.land/bubbletea/v2 + bubbles/v2 + lipgloss/v2, existing `internal/tui/{shell,theme,ui}` design system.

**Spec:** `docs/superpowers/specs/2026-06-19-flow-tui-input-capture-datepicker-design.md`

## Global Constraints

- Go module `github.com/serverkraken/flow`; charm.land v2 import paths only (`charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, `charm.land/lipgloss/v2`).
- Key presses are `tea.KeyPressMsg` with fields `.Code` (e.g. `tea.KeyLeft/KeyRight/KeyUp/KeyDown/KeyEsc/KeyEnter/KeyTab`), `.Text` (typed rune as string), `.Mod` (use `.Mod.Contains(tea.ModShift)`).
- Theme helpers: `theme.Active(s string, pal theme.Palette) string` (accent), `theme.Dim(s string, pal) string`. Palette type `theme.Palette`; test palette `theme.Default`.
- `make ci` must stay green: lint (`golangci-lint`, watch SA4006 unused-var in tests), `verify-generate`, coverage ≥ 80%, build. Run `golangci-lint run <pkg>` per package before committing.
- The pre-existing working-tree changes (`.gitignore`, `flow`) must never be staged.
- No `git commit --amend` on commits you did not create in your own task.

## File Structure

**New:**
- `internal/tui/ui/datepicker/datepicker.go` — the picker component (model, keys, stepper View, Calendar grid).
- `internal/tui/ui/datepicker/datepicker_test.go` — component unit tests.

**Modified:**
- `internal/tui/shell/route.go` — add `InputCapturer` interface.
- `internal/tui/shell/shell.go` — capture gate in `handleKey`.
- `internal/tui/shell/shell_test.go` — capture-gate tests.
- `internal/tui/screen/worktime/route.go` — `Today.CapturesInput()`.
- `internal/tui/screen/worktime/dayoffs/route.go` — `CapturesInput()`; `NewRoute` gains `now func() time.Time`.
- `internal/tui/screen/worktime/dayoffs/dialogs.go` — Von/Bis datepickers; `t`=today; drop empty-Bis path; Calendar render.
- `internal/tui/screen/worktime/nav.go` — `BuildRegistry` passes `time.Now` to `dayoffs.NewRoute`.
- `internal/tui/screen/worktime/export/route.go` — `CapturesInput()`, von/bis datepickers, `t`=today, `Esc`→pop, Calendar render, simplified validation.

**Dependency direction:** `datepicker` imports only `theme` + bubbletea (leaf-free, like other `ui/` components). Routes import `datepicker`. Shell gains no new imports for the interface.

---

## Task 1: `shell.InputCapturer` interface + capture gate

**Files:**
- Modify: `internal/tui/shell/route.go` (add interface near `SwitchRouteMsg`)
- Modify: `internal/tui/shell/shell.go` (`handleKey`)
- Test: `internal/tui/shell/shell_test.go`

**Interfaces:**
- Produces: `type InputCapturer interface{ CapturesInput() bool }`. Routes implementing it and returning `true` receive every key while their top is active.

- [ ] **Step 1: Write the failing test**

Add to `internal/tui/shell/shell_test.go`. (Reuses existing `stubRoute`; defines a capturing stub that records the last key text.)

```go
// captureRoute is a stubRoute that captures input and records keys it receives.
type captureRoute struct {
	stubRoute
	capturing bool
	gotKeys   *[]string
}

func (r captureRoute) CapturesInput() bool { return r.capturing }
func (r captureRoute) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		*r.gotKeys = append(*r.gotKeys, k.Text)
	}
	return r, nil
}

func TestShell_capturingRouteReceivesDigitInsteadOfTabSwitch(t *testing.T) {
	var keys []string
	cap := captureRoute{stubRoute{title: "Form"}, true, &keys}
	other := stubRoute{title: "Other"}
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{cap, other})

	// Active tab 0 captures: "2" must reach the route, NOT switch to tab 1.
	next, _ := s.Update(tea.KeyPressMsg{Text: "2"})
	sh := next.(shell.Shell)
	if sh.ActiveTab() != 0 {
		t.Fatalf("capturing route: digit must not switch tab (activeTab=%d)", sh.ActiveTab())
	}
	if len(keys) != 1 || keys[0] != "2" {
		t.Fatalf("capturing route should receive '2', got %v", keys)
	}
}

func TestShell_nonCapturingRouteStillSwitchesTabOnDigit(t *testing.T) {
	var keys []string
	cap := captureRoute{stubRoute{title: "Form"}, false, &keys}
	other := stubRoute{title: "Other"}
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{cap, other})

	next, _ := s.Update(tea.KeyPressMsg{Text: "2"})
	if next.(shell.Shell).ActiveTab() != 1 {
		t.Fatal("non-capturing route: digit '2' should switch to tab 1")
	}
}
```

If `shell.Shell` has no `ActiveTab()` accessor, add one in Step 3 next to `ActiveDepth()` (`func (s Shell) ActiveTab() int { return s.activeTab }`).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/shell/ -run 'TestShell_(capturingRoute|nonCapturing)' -v`
Expected: FAIL — `CapturesInput`/`InputCapturer` undefined and/or no capture gate (digit switches tab even when capturing).

- [ ] **Step 3: Add the interface**

In `internal/tui/shell/route.go`, after the `SwitchRouteMsg` declaration:

```go
// InputCapturer lets a route signal it is in text-entry mode. While the active
// tab's top route reports CapturesInput()==true, the Shell forwards every key
// to it instead of consuming digits/Tab/Esc/q/:/? as global shortcuts. It is an
// optional interface — routes that don't implement it keep the global shortcuts.
type InputCapturer interface{ CapturesInput() bool }
```

- [ ] **Step 4: Add the gate (and `ActiveTab` if missing)**

In `internal/tui/shell/shell.go`, in `handleKey`, immediately after the `if s.paletteOpen { … }` block and before the `switch`:

```go
	if ic, ok := s.tabs[s.activeTab].Top().(InputCapturer); ok && ic.CapturesInput() && !s.helpOpen {
		return s, s.tabs[s.activeTab].UpdateTop(k)
	}
```

If `ActiveTab()` does not already exist, add next to `ActiveDepth()`:

```go
func (s Shell) ActiveTab() int { return s.activeTab }
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/tui/shell/ -v`
Expected: PASS (all shell tests).

- [ ] **Step 6: Commit**

```bash
git add internal/tui/shell/route.go internal/tui/shell/shell.go internal/tui/shell/shell_test.go
git commit -m "feat(input): shell.InputCapturer gate (capturing route gets all keys)"
```

---

## Task 2: `Today.CapturesInput()`

**Files:**
- Modify: `internal/tui/screen/worktime/route.go`
- Test: `internal/tui/screen/worktime/route_test.go`

**Interfaces:**
- Consumes: `shell.InputCapturer` (Task 1).
- Produces: `Today` now captures input while any dialog is open.

Today already has a `dialog dialogKind` field and `dialogNone` constant (used by `r.dialog != dialogNone`).

- [ ] **Step 1: Write the failing test**

Add to `internal/tui/screen/worktime/route_test.go` (white-box `package worktime`, reuses `newTestRoute`/`keyPress`):

```go
func TestTodayRoute_capturesInputWhileDialogOpen(t *testing.T) {
	r := newTestRoute(&fakeAPI{})
	if r.CapturesInput() {
		t.Fatal("Today should not capture input in the list state")
	}
	// Open the booking dialog (start a session prompts project pick); 's' opens it.
	r.Update(keyPress("s"))
	if !r.CapturesInput() {
		t.Fatal("Today should capture input while a dialog is open")
	}
}
```

If `s` is not the key that opens a dialog in the current Today route, use whichever key opens one (check `handleKey`); the assertion is: list state → false, dialog open → true.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/ -run TestTodayRoute_capturesInputWhileDialogOpen -v`
Expected: FAIL — `CapturesInput` undefined.

- [ ] **Step 3: Implement**

In `internal/tui/screen/worktime/route.go`, add the method (near `KeyHints`):

```go
// CapturesInput reports that Today owns the keyboard while a dialog is open, so
// the Shell forwards digits/Tab/Esc/etc. to the dialog instead of treating them
// as global shortcuts. Implements shell.InputCapturer.
func (r *TodayRoute) CapturesInput() bool { return r.dialog != dialogNone }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/screen/worktime/ -run TestTodayRoute_capturesInputWhileDialogOpen -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen/worktime/route.go internal/tui/screen/worktime/route_test.go
git commit -m "feat(input): Today captures input while a dialog is open"
```

---

## Task 3: `dayoffs.CapturesInput()`

**Files:**
- Modify: `internal/tui/screen/worktime/dayoffs/route.go`
- Test: `internal/tui/screen/worktime/dayoffs/route_test.go`

**Interfaces:**
- Produces: `dayoffs` captures input while any dialog is open.

`dayoffs.Route` has `dialog dialogKind` + `dialogNone` (existing).

- [ ] **Step 1: Write the failing test**

Add to `internal/tui/screen/worktime/dayoffs/route_test.go`:

```go
func TestDayOffsRoute_capturesInputWhileDialogOpen(t *testing.T) {
	api := &fakeAPI{}
	r := dayoffs.NewRoute(api, theme.Default, wtnav.Registry{})
	r2 := drain(r, r.Init())
	if r2.(interface{ CapturesInput() bool }).CapturesInput() {
		t.Fatal("dayoffs should not capture in the list state")
	}
	// 'a' opens the add dialog.
	r3, _ := r2.Update(tea.KeyPressMsg{Text: "a"})
	if !r3.(interface{ CapturesInput() bool }).CapturesInput() {
		t.Fatal("dayoffs should capture while the add dialog is open")
	}
}
```

(If `dayoffs.NewRoute` already gained a `now` param from a later task being applied out of order, pass `time.Now`; per this plan order it has not yet — keep the 3-arg call.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/dayoffs/ -run TestDayOffsRoute_capturesInputWhileDialogOpen -v`
Expected: FAIL — `CapturesInput` undefined.

- [ ] **Step 3: Implement**

In `internal/tui/screen/worktime/dayoffs/route.go`, add:

```go
// CapturesInput reports that the route owns the keyboard while a dialog is open.
// Implements shell.InputCapturer.
func (r *Route) CapturesInput() bool { return r.dialog != dialogNone }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/screen/worktime/dayoffs/ -run TestDayOffsRoute_capturesInputWhileDialogOpen -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen/worktime/dayoffs/route.go internal/tui/screen/worktime/dayoffs/route_test.go
git commit -m "feat(input): dayoffs captures input while a dialog is open"
```

---

## Task 4: `export.CapturesInput()` + `Esc` → pop

**Files:**
- Modify: `internal/tui/screen/worktime/export/route.go`
- Test: `internal/tui/screen/worktime/export/route_test.go`

**Interfaces:**
- Consumes: `shell.PopRouteMsg` (existing).
- Produces: `export` captures input while a text/picker field (von/bis/Pfad) is focused; `Esc` on the route emits `shell.PopRouteMsg`.

`export.Route` has `focus int` (0=Range,1=von,2=bis,3=Format,4=Pfad).

- [ ] **Step 1: Write the failing test**

Add to `internal/tui/screen/worktime/export/route_test.go`:

```go
func TestExportRoute_capturesOnTextFieldsNotChoiceFields(t *testing.T) {
	r := export.NewRoute(fakeAPI{}, fixedNow, theme.Default, wtnav.Registry{})
	// focus 0 = Range (choice) → not capturing
	if r.CapturesInput() {
		t.Fatal("Range field must not capture (globals stay reachable)")
	}
	// Tab to focus 1 = von (text/picker) → capturing
	r.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if !r.CapturesInput() {
		t.Fatal("von field must capture input")
	}
}

func TestExportRoute_escEmitsPop(t *testing.T) {
	r := export.NewRoute(fakeAPI{}, fixedNow, theme.Default, wtnav.Registry{})
	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("Esc should emit a command")
	}
	if _, ok := cmd().(shell.PopRouteMsg); !ok {
		t.Fatalf("Esc should emit shell.PopRouteMsg, got %T", cmd())
	}
}
```

`r.Update(...)` mutates the pointer receiver, so `r.CapturesInput()` after a Tab reflects the new focus. Confirm `export.NewRoute` returns `*Route` and `Update` has a pointer receiver (it does).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/export/ -run 'TestExportRoute_(captures|escEmitsPop)' -v`
Expected: FAIL — `CapturesInput` undefined and Esc returns nil.

- [ ] **Step 3: Implement**

In `internal/tui/screen/worktime/export/route.go`:

Add the method:

```go
// CapturesInput reports input capture while a text/picker field (von/bis/Pfad)
// is focused, so digits/Tab/Esc reach the field. On the Range/Format choice
// fields it returns false, leaving global shortcuts + lateral nav reachable.
// Implements shell.InputCapturer.
func (r *Route) CapturesInput() bool {
	return r.focus == 1 || r.focus == 2 || r.focus == 4
}
```

Add an `Esc` case at the top of `handleKey` (before the existing focus/tab logic), importing `"github.com/serverkraken/flow/internal/tui/shell"` if not already imported:

```go
	if k.Code == tea.KeyEsc {
		return r, func() tea.Msg { return shell.PopRouteMsg{} }
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/screen/worktime/export/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen/worktime/export/route.go internal/tui/screen/worktime/export/route_test.go
git commit -m "feat(input): export captures text fields + Esc pops route"
```

> **Milestone after Task 4:** Phase A complete — every typed field under the shell is usable (dates can be typed as raw text in Frei/Export, minutes in Tagesziel, HH:MM in Today). Phases B/C replace the date text fields with the picker.

---

## Task 5: `datepicker` model — value, View (stepper + weekday)

**Files:**
- Create: `internal/tui/ui/datepicker/datepicker.go`
- Test: `internal/tui/ui/datepicker/datepicker_test.go`

**Interfaces:**
- Produces: `datepicker.New(initial time.Time, pal theme.Palette) Model`; `Value() string` (`YYYY-MM-DD`); `SetValue(string) error`; `Focus()`, `Blur()`, `Focused() bool`; `View() string`. (`Update` and `Calendar` arrive in Tasks 6/7.)

- [ ] **Step 1: Write the failing test**

`internal/tui/ui/datepicker/datepicker_test.go`:

```go
package datepicker_test

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/datepicker"
)

func TestDatepicker_valueRoundTrip(t *testing.T) {
	m := datepicker.New(time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC), theme.Default)
	if m.Value() != "2026-07-20" {
		t.Fatalf("Value = %q, want 2026-07-20", m.Value())
	}
	if err := m.SetValue("2024-02-29"); err != nil {
		t.Fatalf("SetValue err: %v", err)
	}
	if m.Value() != "2024-02-29" {
		t.Fatalf("after SetValue, Value = %q", m.Value())
	}
	if err := m.SetValue("not-a-date"); err == nil {
		t.Fatal("SetValue should reject bad input")
	}
}

func TestDatepicker_viewShowsSegmentsAndWeekday(t *testing.T) {
	m := datepicker.New(time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC), theme.Default)
	v := m.View()
	if !strings.Contains(v, "2026") || !strings.Contains(v, "07") || !strings.Contains(v, "20") {
		t.Fatalf("view missing date segments: %q", v)
	}
	if !strings.Contains(v, "Mo") { // 2026-07-20 is a Monday
		t.Fatalf("view should show weekday Mo: %q", v)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ui/datepicker/`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement**

`internal/tui/ui/datepicker/datepicker.go`:

```go
// Package datepicker is a clock-free segment-stepper date picker: ‹YYYY›-‹MM›-‹DD›
// with a weekday label and an optional read-only month grid (Calendar). It never
// reads the host time; "today" for the grid is passed in. Drop it into a dialog
// row like a textinput; the embedding route routes keys to Update.
package datepicker

import (
	"fmt"
	"time"

	"github.com/serverkraken/flow/internal/tui/theme"
)

// Model holds the selected date and which segment is active. Zero value is not
// valid; use New.
type Model struct {
	y, mo, d int
	seg      int // 0=year, 1=month, 2=day
	typed    int // digits entered into the active segment since it became active
	focused  bool
	pal      theme.Palette
}

// New builds a picker initialised to initial's date.
func New(initial time.Time, pal theme.Palette) Model {
	return Model{y: initial.Year(), mo: int(initial.Month()), d: initial.Day(), pal: pal}
}

// Value returns the selected date as "YYYY-MM-DD".
func (m Model) Value() string { return fmt.Sprintf("%04d-%02d-%02d", m.y, m.mo, m.d) }

// SetValue parses "YYYY-MM-DD" and replaces the selection; bad input is rejected.
func (m *Model) SetValue(s string) error {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return err
	}
	m.y, m.mo, m.d, m.typed = t.Year(), int(t.Month()), t.Day(), 0
	return nil
}

func (m *Model) Focus()       { m.focused = true }
func (m *Model) Blur()        { m.focused = false; m.typed = 0 }
func (m Model) Focused() bool { return m.focused }

// weekdayShort renders a Go weekday as the German two-letter abbreviation.
func weekdayShort(w time.Weekday) string {
	return [...]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}[int(w)]
}

// daysIn returns the number of days in month mo of year y.
func daysIn(y, mo int) int {
	return time.Date(y, time.Month(mo)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// View renders the one-line stepper "‹2026›-‹07›-‹20›  (Mo)"; the active segment
// is accent-highlighted only while focused.
func (m Model) View() string {
	segs := [3]string{fmt.Sprintf("%04d", m.y), fmt.Sprintf("%02d", m.mo), fmt.Sprintf("%02d", m.d)}
	for i := range segs {
		if m.focused && i == m.seg {
			segs[i] = theme.Active("‹"+segs[i]+"›", m.pal)
		} else {
			segs[i] = " " + segs[i] + " "
		}
	}
	wd := weekdayShort(time.Date(m.y, time.Month(m.mo), m.d, 0, 0, 0, 0, time.UTC).Weekday())
	return segs[0] + "-" + segs[1] + "-" + segs[2] + "  (" + wd + ")"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ui/datepicker/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/ui/datepicker/
git commit -m "feat(datepicker): model + value + stepper View with weekday"
```

---

## Task 6: `datepicker.Update` — segment nav, ±rollover, digit entry

**Files:**
- Modify: `internal/tui/ui/datepicker/datepicker.go`
- Test: `internal/tui/ui/datepicker/datepicker_test.go`

**Interfaces:**
- Produces: `func (m Model) Update(k tea.KeyPressMsg) Model` — `←/→` move segment, `↑/↓` step the active segment (month/day roll over; day clamps to month length), digit keys fill the active segment and auto-advance.

- [ ] **Step 1: Write the failing test**

Add to `internal/tui/ui/datepicker/datepicker_test.go` (add imports `tea "charm.land/bubbletea/v2"`):

```go
func key(code tea.KeyCode) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code} }
func digit(s string) tea.KeyPressMsg       { return tea.KeyPressMsg{Text: s} }

func TestDatepicker_arrowStepsAndRollsOver(t *testing.T) {
	m := datepicker.New(time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), theme.Default)
	// active segment defaults to year (0). Move to month (seg 1) and roll 12 -> 1.
	m = m.Update(key(tea.KeyRight)) // seg=month
	m = m.Update(key(tea.KeyUp))    // 12 -> 1
	if m.Value() != "2026-01-31" {
		t.Fatalf("month rollover: %q, want 2026-01-31", m.Value())
	}
}

func TestDatepicker_dayClampsOnMonthChange(t *testing.T) {
	m := datepicker.New(time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC), theme.Default)
	m = m.Update(key(tea.KeyRight)) // seg=month
	m = m.Update(key(tea.KeyUp))    // Jan -> Feb; day 31 must clamp to 28 (2026 not leap)
	if m.Value() != "2026-02-28" {
		t.Fatalf("day clamp: %q, want 2026-02-28", m.Value())
	}
}

func TestDatepicker_dayRollsWithinMonth(t *testing.T) {
	m := datepicker.New(time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), theme.Default)
	m = m.Update(key(tea.KeyRight)) // seg=month
	m = m.Update(key(tea.KeyRight)) // seg=day
	m = m.Update(key(tea.KeyUp))    // 28 -> rolls to 1 (Feb has 28)
	if m.Value() != "2026-02-01" {
		t.Fatalf("day rollover: %q, want 2026-02-01", m.Value())
	}
}

func TestDatepicker_digitEntryFillsSegments(t *testing.T) {
	m := datepicker.New(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), theme.Default)
	// year: 2026
	for _, c := range []string{"2", "0", "2", "6"} {
		m = m.Update(digit(c))
	}
	// auto-advanced to month: "07"
	m = m.Update(digit("0"))
	m = m.Update(digit("7"))
	// auto-advanced to day: "2","0"
	m = m.Update(digit("2"))
	m = m.Update(digit("0"))
	if m.Value() != "2026-07-20" {
		t.Fatalf("digit entry: %q, want 2026-07-20", m.Value())
	}
}

func TestDatepicker_singleDigitMonthAutoCommits(t *testing.T) {
	m := datepicker.New(time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), theme.Default)
	m = m.Update(key(tea.KeyRight)) // seg=month
	m = m.Update(digit("7"))        // 7 cannot start a 2-digit month -> commit 07, advance to day
	if m.Value() != "2026-07-15" {
		t.Fatalf("single-digit month: %q, want 2026-07-15", m.Value())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ui/datepicker/ -run TestDatepicker_ -v`
Expected: FAIL — `Update` undefined.

- [ ] **Step 3: Implement**

Add to `internal/tui/ui/datepicker/datepicker.go` (add import `tea "charm.land/bubbletea/v2"`):

```go
// Update applies one key: ←/→ move the active segment, ↑/↓ step it, digits fill
// it. It never returns a command and never reads the clock.
func (m Model) Update(k tea.KeyPressMsg) Model {
	switch k.Code {
	case tea.KeyLeft:
		m.commit()
		if m.seg > 0 {
			m.seg--
		}
	case tea.KeyRight:
		m.commit()
		if m.seg < 2 {
			m.seg++
		}
	case tea.KeyUp:
		m.commit()
		m.step(+1)
	case tea.KeyDown:
		m.commit()
		m.step(-1)
	default:
		if len(k.Text) == 1 && k.Text[0] >= '0' && k.Text[0] <= '9' {
			m.typeDigit(int(k.Text[0] - '0'))
		}
	}
	return m
}

// commit clamps the current fields to a valid date and resets the typing buffer.
func (m *Model) commit() {
	if m.y < 1 {
		m.y = 1
	}
	if m.mo < 1 {
		m.mo = 1
	} else if m.mo > 12 {
		m.mo = 12
	}
	if m.d < 1 {
		m.d = 1
	}
	if n := daysIn(m.y, m.mo); m.d > n {
		m.d = n
	}
	m.typed = 0
}

// step increments (delta +1) or decrements the active segment with rollover for
// month and day; the day is clamped when year/month change.
func (m *Model) step(delta int) {
	switch m.seg {
	case 0:
		m.y += delta
		if m.y < 1 {
			m.y = 1
		}
		if n := daysIn(m.y, m.mo); m.d > n {
			m.d = n
		}
	case 1:
		m.mo += delta
		if m.mo > 12 {
			m.mo = 1
		} else if m.mo < 1 {
			m.mo = 12
		}
		if n := daysIn(m.y, m.mo); m.d > n {
			m.d = n
		}
	case 2:
		n := daysIn(m.y, m.mo)
		m.d += delta
		if m.d > n {
			m.d = 1
		} else if m.d < 1 {
			m.d = n
		}
	}
}

// advance commits the active segment and moves focus to the next one.
func (m *Model) advance() {
	m.commit()
	if m.seg < 2 {
		m.seg++
	}
}

// typeDigit accumulates a typed digit into the active segment, auto-advancing
// when the segment is full or cannot take another digit.
func (m *Model) typeDigit(dg int) {
	switch m.seg {
	case 0: // year: 4 digits
		if m.typed == 0 {
			m.y = dg
		} else {
			m.y = (m.y*10 + dg) % 10000
		}
		m.typed++
		if m.typed >= 4 {
			m.advance()
		}
	case 1: // month: 1-2 digits
		if m.typed == 0 {
			m.mo = dg
			m.typed = 1
			if dg >= 2 { // 2..9 can't start a valid 2-digit month
				m.advance()
			}
		} else {
			m.mo = m.mo*10 + dg
			m.advance()
		}
	case 2: // day: 1-2 digits
		if m.typed == 0 {
			m.d = dg
			m.typed = 1
			if dg >= 4 { // 4..9 can't start a valid 2-digit day
				m.advance()
			}
		} else {
			m.d = m.d*10 + dg
			m.advance()
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/ui/datepicker/ -v`
Expected: PASS (all datepicker tests).

- [ ] **Step 5: Lint + commit**

Run: `golangci-lint run ./internal/tui/ui/datepicker/`
Expected: 0 issues.

```bash
git add internal/tui/ui/datepicker/
git commit -m "feat(datepicker): Update — segment nav, ±rollover, day clamp, digit entry"
```

---

## Task 7: `datepicker.Calendar(today)` — read-only month grid

**Files:**
- Modify: `internal/tui/ui/datepicker/datepicker.go`
- Test: `internal/tui/ui/datepicker/datepicker_test.go`

**Interfaces:**
- Produces: `func (m Model) Calendar(today time.Time) string` — Monday-first month grid of the selected date's month; selected day and (if in-month) today are highlighted. Pass the zero `time.Time` to skip the today highlight.

- [ ] **Step 1: Write the failing test**

Add to `internal/tui/ui/datepicker/datepicker_test.go`:

```go
func TestDatepicker_calendarStructure(t *testing.T) {
	m := datepicker.New(time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC), theme.Default)
	cal := m.Calendar(time.Time{})
	if !strings.Contains(cal, "Juli 2026") {
		t.Fatalf("calendar missing month header: %q", cal)
	}
	if !strings.Contains(cal, "Mo Di Mi Do Fr Sa So") {
		t.Fatalf("calendar missing weekday header: %q", cal)
	}
	for _, day := range []string{" 1", "15", "31"} {
		if !strings.Contains(cal, day) {
			t.Fatalf("calendar missing day %q:\n%s", day, cal)
		}
	}
}

func TestDatepicker_calendarMarksSelectedAndToday(t *testing.T) {
	m20 := datepicker.New(time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC), theme.Default)
	m21 := datepicker.New(time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC), theme.Default)
	// Changing the selected day changes the rendered grid (selection is marked).
	if m20.Calendar(time.Time{}) == m21.Calendar(time.Time{}) {
		t.Fatal("selected-day marking should differ between day 20 and 21")
	}
	// today in the shown month changes the grid; today in another month does not.
	todayIn := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
	todayOut := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	if m20.Calendar(todayIn) == m20.Calendar(time.Time{}) {
		t.Fatal("today-in-month should change the grid")
	}
	if m20.Calendar(todayOut) != m20.Calendar(time.Time{}) {
		t.Fatal("today outside the shown month should not change the grid")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ui/datepicker/ -run TestDatepicker_calendar -v`
Expected: FAIL — `Calendar` undefined.

- [ ] **Step 3: Implement**

Add to `internal/tui/ui/datepicker/datepicker.go` (add import `"strings"`):

```go
// monthNames are the German month names indexed 1..12.
var monthNames = [...]string{"", "Januar", "Februar", "März", "April", "Mai", "Juni",
	"Juli", "August", "September", "Oktober", "November", "Dezember"}

// Calendar renders a read-only Monday-first month grid of the selected date's
// month. The selected day is accent-highlighted; today (when it falls in the
// shown month) is dimmed. Pass the zero time to omit the today highlight. It is
// pure rendering — it does not change the selection.
func (m Model) Calendar(today time.Time) string {
	first := time.Date(m.y, time.Month(m.mo), 1, 0, 0, 0, 0, time.UTC)
	lead := (int(first.Weekday()) + 6) % 7 // Monday=0
	n := daysIn(m.y, m.mo)
	todayInMonth := !today.IsZero() && today.Year() == m.y && int(today.Month()) == m.mo

	var b strings.Builder
	fmt.Fprintf(&b, "  %s %d\n", monthNames[m.mo], m.y)
	b.WriteString("  Mo Di Mi Do Fr Sa So\n  ")
	for i := 0; i < lead; i++ {
		b.WriteString("   ")
	}
	for day := 1; day <= n; day++ {
		cell := fmt.Sprintf("%2d", day)
		switch {
		case day == m.d:
			cell = theme.Active(cell, m.pal)
		case todayInMonth && today.Day() == day:
			cell = theme.Dim(cell, m.pal)
		}
		b.WriteString(cell + " ")
		if (lead+day)%7 == 0 {
			b.WriteString("\n  ")
		}
	}
	return b.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/ui/datepicker/ -v`
Expected: PASS.

- [ ] **Step 5: Lint + commit**

Run: `golangci-lint run ./internal/tui/ui/datepicker/`
Expected: 0 issues.

```bash
git add internal/tui/ui/datepicker/
git commit -m "feat(datepicker): Calendar — read-only month grid, marks selected + today"
```

---

## Task 8: Wire datepicker into Frei (`dayoffs`) add dialog

**Files:**
- Modify: `internal/tui/screen/worktime/dayoffs/dialogs.go`
- Modify: `internal/tui/screen/worktime/dayoffs/route.go` (`NewRoute` gains `now`)
- Modify: `internal/tui/screen/worktime/nav.go` (`BuildRegistry` passes `time.Now`)
- Test: `internal/tui/screen/worktime/dayoffs/route_test.go`

**Interfaces:**
- Consumes: `datepicker.New/Update/View/Calendar/Value/SetValue/Focus/Blur` (Tasks 5-7); `r.now func() time.Time`.
- Produces: `dayoffs.NewRoute(api API, pal theme.Palette, reg wtnav.Registry, now func() time.Time) *Route`.

The current add dialog uses three `textinput.Model` (Von/Bis/Label). This task replaces Von/Bis with `datepicker.Model`, keeps Label as text, defaults Bis to Von, validates Bis ≥ Von, adds `t`=today, and renders the focused picker's month grid.

- [ ] **Step 1: Write the failing test**

Update `internal/tui/screen/worktime/dayoffs/route_test.go`. First, every `dayoffs.NewRoute(...)` call gains a `now` arg — define a fixed now and a small helper, and update existing call sites:

```go
func fixedNow() time.Time { return time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC) }

func newRoute(api dayoffs.API) shell.Route {
	return dayoffs.NewRoute(api, theme.Default, wtnav.Registry{}, fixedNow)
}
```

Replace existing `dayoffs.NewRoute(api, theme.Default, wtnav.Registry{})` calls with `newRoute(api)`. Then add the add-flow test driving the pickers:

```go
func TestDayOffsRoute_addViaDatepicker(t *testing.T) {
	api := &fakeAPI{}
	r := drain(newRoute(api), nil)
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "a"}) // open add dialog; Von focused, defaults to today
	// Von defaults to fixedNow (2026-06-18); step year segment +0, set day to 20 via digits.
	// seg starts at year; move to day and type 20.
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyRight}) // -> month
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyRight}) // -> day
	r, _ = r.Update(tea.KeyPressMsg{Text: "2"})
	r, _ = r.Update(tea.KeyPressMsg{Text: "0"}) // Von day = 20
	// Tab to Bis (defaults to Von), Tab to Label, type a label, Tab back? Submit via enter on last field.
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Bis
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Label
	for _, c := range []string{"U", "r", "l", "a", "u", "b"} {
		r, _ = r.Update(tea.KeyPressMsg{Text: c})
	}
	r2, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // submit on last field
	_ = drain(r2, cmd)
	if api.addedFrom != "2026-06-20" {
		t.Fatalf("addedFrom = %q, want 2026-06-20", api.addedFrom)
	}
}

func TestDayOffsRoute_addRejectsBisBeforeVon(t *testing.T) {
	api := &fakeAPI{}
	r := drain(newRoute(api), nil)
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "a"})
	// Set Von to day 20, leave Bis defaulting to Von (=20), then lower Bis to 10.
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	r, _ = r.Update(tea.KeyPressMsg{Text: "2"})
	r, _ = r.Update(tea.KeyPressMsg{Text: "0"})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // Bis (=20)
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	r, _ = r.Update(tea.KeyPressMsg{Text: "1"})
	r, _ = r.Update(tea.KeyPressMsg{Text: "0"}) // Bis day = 10 (< Von 20)
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab})  // Label
	r2, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_ = drain(r2, cmd)
	if api.addedFrom != "" {
		t.Fatalf("submit with Bis<Von must not call AddDayOffs (addedFrom=%q)", api.addedFrom)
	}
}
```

If the existing `fakeAPI.AddDayOffs` does not record `to`, extend it to also record `addedTo` so the range is checked; otherwise asserting `addedFrom` is enough for these tests.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/screen/worktime/dayoffs/`
Expected: FAIL — `NewRoute` arity mismatch and datepicker-driven add not implemented.

- [ ] **Step 3: Add `now` to `NewRoute`**

In `internal/tui/screen/worktime/dayoffs/route.go`, add a `now func() time.Time` field to `Route` and a param to `NewRoute` (default to `time.Now` when nil); import `"time"` if needed:

```go
// in the Route struct, add:
	now func() time.Time

// constructor:
func NewRoute(api API, pal theme.Palette, reg wtnav.Registry, now func() time.Time) *Route {
	if now == nil {
		now = time.Now
	}
	return &Route{api: api, pal: pal, reg: reg, now: now}
}
```

- [ ] **Step 4: Replace Von/Bis textinputs with datepickers in `dialogs.go`**

In `internal/tui/screen/worktime/dayoffs/dialogs.go`:

Change the `dialogState` so the add form holds two pickers + a label input:

```go
type dialogState struct {
	target    string
	vonDP     datepicker.Model
	bisDP     datepicker.Model
	bisEdited bool // once true, Bis stops tracking Von
	label     textinput.Model
	addCur    int // 0=Von, 1=Bis, 2=Label
	confirm   confirm.Model
	blSel     int
}
```

> **Bis tracks Von until edited.** While `bisEdited` is false, any change to Von mirrors into Bis (so a single-day off only needs Von set). Editing Bis latches `bisEdited`, enabling a real range. This mirrors the existing `pathEdited` latch in the export route.

Add imports `"github.com/serverkraken/flow/internal/tui/ui/datepicker"` and keep `textinput`/`time`.

Replace `openAdd`:

```go
func (r *Route) openAdd() (shell.Route, tea.Cmd) {
	today := r.now()
	r.dlg.vonDP = datepicker.New(today, r.pal)
	r.dlg.bisDP = datepicker.New(today, r.pal)
	r.dlg.bisEdited = false
	r.dlg.label = form.NewTextInput("z.B. Urlaub", r.pal)
	r.dlg.vonDP.Focus()
	r.dlg.addCur = 0
	r.dialog = dialogAdd
	return r, nil
}
```

Replace `handleAddKey` to route keys to the focused widget, handle `t`=today on a picker, Tab/↑/↓ to move between widgets, and Enter on Label to submit:

```go
func (r *Route) handleAddKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch k.Code {
	case tea.KeyEsc:
		r.dialog = dialogNone
		return r, nil
	case tea.KeyTab:
		r.addFocus(+1)
		return r, nil
	case tea.KeyEnter:
		if r.dlg.addCur == 2 {
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
	case 2:
		var cmd tea.Cmd
		r.dlg.label, cmd = r.dlg.label.Update(k)
		return r, cmd
	}
	return r, nil
}
```

Replace `addFocus` to cycle the three widgets and manage picker/label focus:

```go
func (r *Route) addFocus(delta int) {
	r.dlg.addCur = (r.dlg.addCur + delta + 3) % 3
	r.dlg.vonDP.Blur()
	r.dlg.bisDP.Blur()
	r.dlg.label.Blur()
	switch r.dlg.addCur {
	case 0:
		r.dlg.vonDP.Focus()
	case 1:
		r.dlg.bisDP.Focus()
	case 2:
		_ = r.dlg.label.Focus()
	}
}
```

Replace `submitAdd` to read picker values, validate Bis ≥ Von, and submit:

```go
func (r *Route) submitAdd() tea.Cmd {
	from := r.dlg.vonDP.Value()
	to := r.dlg.bisDP.Value()
	if to < from { // ISO yyyy-mm-dd compares lexically
		return nil // keep dialog open; invalid range
	}
	label := strings.TrimSpace(r.dlg.label.Value())
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
```

Replace the `dialogAdd` branch of `renderDialog` to show the two pickers, the label, and the focused picker's calendar:

```go
	case dialogAdd:
		var b strings.Builder
		b.WriteString("\n  Frei-Tag anlegen (tab wechselt · t heute · enter speichert · esc ab)\n\n")
		fmt.Fprintf(&b, "  Von    %s\n", r.dlg.vonDP.View())
		fmt.Fprintf(&b, "  Bis    %s\n", r.dlg.bisDP.View())
		fmt.Fprintf(&b, "  Label  %s\n", r.dlg.label.View())
		switch r.dlg.addCur {
		case 0:
			b.WriteString("\n" + r.dlg.vonDP.Calendar(r.now()) + "\n")
		case 1:
			b.WriteString("\n" + r.dlg.bisDP.Calendar(r.now()) + "\n")
		}
		return b.String()
```

Update the add-dialog hints in `dialogHints` if they reference the old field model (keep `{Key: "tab", Desc: "Feld"}, {Key: "t", Desc: "heute"}, {Key: "enter", Desc: "speichern"}, {Key: "esc", Desc: "abbrechen"}`).

- [ ] **Step 5: Update `BuildRegistry`**

In `internal/tui/screen/worktime/nav.go`, pass `time.Now` to `dayoffs.NewRoute` (add `"time"` import):

```go
		"d": func() shell.Route { return dayoffs.NewRoute(client, pal, reg, time.Now) },
```

- [ ] **Step 6: Run tests + build**

Run: `go test ./internal/tui/screen/worktime/... -v`
Expected: PASS (dayoffs add via picker, Bis<Von rejected, existing dayoffs/nav tests with updated call sites).
Run: `go build ./...`
Expected: clean.

- [ ] **Step 7: Lint + commit**

Run: `golangci-lint run ./internal/tui/screen/worktime/...`
Expected: 0 issues.

```bash
git add internal/tui/screen/worktime/dayoffs/ internal/tui/screen/worktime/nav.go internal/tui/screen/worktime/route_test.go
git commit -m "feat(datepicker): Frei add dialog uses Von/Bis pickers + month grid"
```

> If `route_test.go` (the Today white-box test) does not reference dayoffs, omit it from the `git add`; only stage files you changed.

---

## Task 9: Wire datepicker into Export

**Files:**
- Modify: `internal/tui/screen/worktime/export/route.go`
- Test: `internal/tui/screen/worktime/export/route_test.go`

**Interfaces:**
- Consumes: `datepicker` (Tasks 5-7); existing `r.now func() time.Time`.
- Produces: Export von/bis are pickers; `Range` preset fills them via `SetValue`; `t`=today on a focused picker; focused picker's calendar rendered.

Export currently keeps `from, to string` and edits them as text in `editField`. Replace with `vonDP, bisDP datepicker.Model`; `Value()` feeds `submit`/`refreshPath`.

- [ ] **Step 1: Write the failing test**

Add to `internal/tui/screen/worktime/export/route_test.go`:

```go
func TestExportRoute_presetFillsPickersAndExports(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.md")
	r := export.NewRoute(fakeAPI{payload: []byte("# hi")}, fixedNow, theme.Default, wtnav.Registry{})
	// Default preset "monat" with fixedNow (2026-06-17) → from 2026-06-01.
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "2026") || !strings.Contains(body, "06") {
		t.Fatalf("view should show the preset-filled date pickers:\n%s", body)
	}
	r = export.WithPathForTest(r, path)
	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			break
		}
		var c tea.Cmd
		var nr shell.Route
		nr, c = r.Update(msg)
		r, cmd = nr.(*export.Route), c
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "# hi" {
		t.Fatalf("export not written: err=%v content=%q", err, string(b))
	}
}

func TestExportRoute_calendarShownWhenDateFocused(t *testing.T) {
	r := export.NewRoute(fakeAPI{}, fixedNow, theme.Default, wtnav.Registry{})
	r.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // focus von
	body := r.View(shell.Frame{Width: 80, Height: 30, Pal: theme.Default})
	if !strings.Contains(body, "Mo Di Mi Do Fr Sa So") {
		t.Fatalf("calendar grid should show when a date field is focused:\n%s", body)
	}
}
```

(`fixedNow` already exists in this test file from Task 7 of the M3c2 work; reuse it.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/screen/worktime/export/`
Expected: FAIL — pickers not wired (no calendar, date fields still text).

- [ ] **Step 3: Replace from/to text with pickers**

In `internal/tui/screen/worktime/export/route.go`:

Add import `"github.com/serverkraken/flow/internal/tui/ui/datepicker"`. Replace the `from, to string` fields with:

```go
	vonDP, bisDP datepicker.Model
```

In `NewRoute`, after computing the default `from, to` strings from `presetRange("monat", now())`, build the pickers (parse the strings into the pickers):

```go
	von := mustDate(from)
	bis := mustDate(to)
	r := &Route{ api: api, now: now, pal: pal, reg: reg,
		preset: "monat", format: "md",
		vonDP: datepicker.New(von, pal), bisDP: datepicker.New(bis, pal),
		path: defaultPath(from, to, "md"),
	}
	r.vonDP.Focus() // von is the first text/picker field; focus tracked by r.focus
	return r
```

Add a helper:

```go
// mustDate parses a yyyy-mm-dd produced by presetRange; presetRange always emits
// valid dates, so a parse failure is a programming error.
func mustDate(s string) time.Time {
	t, err := time.Parse(dayFmt, s)
	if err != nil {
		panic("export: bad preset date " + s)
	}
	return t
}
```

Replace `from`/`to` reads. `cycleField` for the preset case sets the pickers:

```go
	case 0:
		r.preset = cyclePreset(r.preset, dir)
		if r.preset != "custom" {
			f, to := presetRange(r.preset, r.now())
			_ = r.vonDP.SetValue(f)
			_ = r.bisDP.SetValue(to)
		}
		r.refreshPath()
```

In `handleKey`, route keys to the focused picker when `focus==1` (von) or `focus==2` (bis), handle `t`=today there, and set `preset="custom"` on a picker edit. Insert before the existing tab/enter/left-right/text handling (after the Esc case from Task 4):

```go
	if r.focus == 1 || r.focus == 2 {
		if k.Text == "t" {
			r.setFocusedDate(r.now().Format(dayFmt))
			return r, nil
		}
		switch k.Code {
		case tea.KeyLeft, tea.KeyRight, tea.KeyUp, tea.KeyDown:
			r.editFocusedPicker(k)
			r.preset = "custom"
			r.refreshPath()
			return r, nil
		default:
			if len(k.Text) == 1 && k.Text[0] >= '0' && k.Text[0] <= '9' {
				r.editFocusedPicker(k)
				r.preset = "custom"
				r.refreshPath()
				return r, nil
			}
		}
	}
```

Helpers:

```go
func (r *Route) editFocusedPicker(k tea.KeyPressMsg) {
	if r.focus == 1 {
		r.vonDP = r.vonDP.Update(k)
	} else {
		r.bisDP = r.bisDP.Update(k)
	}
}

func (r *Route) setFocusedDate(s string) {
	if r.focus == 1 {
		_ = r.vonDP.SetValue(s)
	} else {
		_ = r.bisDP.SetValue(s)
	}
	r.preset = "custom"
	r.refreshPath()
}
```

Update `refreshPath` and `submit` to read picker values via `r.vonDP.Value()` / `r.bisDP.Value()` instead of `r.from`/`r.to`. Simplify `submit`'s validation to a range check (pickers guarantee valid dates):

```go
func (r *Route) submit() tea.Cmd {
	from := r.vonDP.Value()
	to := r.bisDP.Value()
	if to < from {
		r.status = "bis muss >= von sein"
		return nil
	}
	r.status = "exportiere…"
	api, format, path := r.api, r.format, expandHome(r.path)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		b, err := api.Export(ctx, from, to, format, "")
		if err != nil {
			return errMsg{err}
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return errMsg{err}
		}
		return doneMsg{path: path}
	}
}
```

In `refreshPath`:

```go
func (r *Route) refreshPath() {
	if !r.pathEdited {
		r.path = defaultPath(r.vonDP.Value(), r.bisDP.Value(), r.format)
	}
}
```

In `View`, render the von/bis rows with `r.vonDP.View()` / `r.bisDP.View()` (instead of the old text values) and append the focused picker's calendar:

```go
	field(0, "Range", r.preset)
	fieldRaw(1, "von", r.vonDP.View())
	fieldRaw(2, "bis", r.bisDP.View())
	field(3, "Format", r.format)
	field(4, "Pfad", r.path)
	if r.focus == 1 {
		b.WriteString("\n" + r.vonDP.Calendar(r.now()))
	} else if r.focus == 2 {
		b.WriteString("\n" + r.bisDP.Calendar(r.now()))
	}
```

Where `fieldRaw` is `field` without re-applying the active value styling (the picker already accents its focused segment). If the existing `field` closure applies `theme.Active` to `val` when focused, add a sibling that skips that for picker rows:

```go
	fieldRaw := func(idx int, label, val string) {
		cur := "  "
		if r.focus == idx {
			cur = theme.Active("▸", f.Pal) + " "
		}
		fmt.Fprintf(&b, "%s%-8s %s\n", cur, label, val)
	}
```

Ensure focus changes (Tab) call `Focus()/Blur()` on the pickers so `View` highlights the active segment. In the Tab handler, after updating `r.focus`, sync picker focus:

```go
	r.syncPickerFocus()
```

with:

```go
func (r *Route) syncPickerFocus() {
	r.vonDP.Blur()
	r.bisDP.Blur()
	switch r.focus {
	case 1:
		r.vonDP.Focus()
	case 2:
		r.bisDP.Focus()
	}
}
```

Call `r.syncPickerFocus()` wherever `r.focus` changes (the Tab / Shift-Tab cases).

- [ ] **Step 4: Run tests + build**

Run: `go test ./internal/tui/screen/worktime/export/ -v`
Expected: PASS (preset fills pickers + exports; calendar shows when date focused; existing export tests).
Run: `go build ./...`
Expected: clean.

- [ ] **Step 5: Lint + commit**

Run: `golangci-lint run ./internal/tui/screen/worktime/export/`
Expected: 0 issues.

```bash
git add internal/tui/screen/worktime/export/
git commit -m "feat(datepicker): Export von/bis use date pickers + month grid"
```

---

## Task 10: Done-gate — full CI + live verification

**Files:** none (verification only).

- [ ] **Step 1: Run the full CI gate**

Run: `make ci`
Expected: lint + verify-generate + build + tests green; coverage ≥ 80%.
If coverage dips below 80%, add render/reducer tests to the thinnest new surface (the `datepicker` View/Calendar branches or the export/dayoffs picker paths) until the gate passes — do not lower the threshold.

- [ ] **Step 2: Live done-gate against the dev stack**

Start the dev stack ([[reference_flow_dev_env]]): `make dev-up && make dev-run` (separate shells), ensure a token. Run `flow ui` and verify:

- [ ] In any field, typing digits/`Tab`/`q`/`:` now goes INTO the field (no tab-switch/quit/palette).
- [ ] Frei → `a`: Von/Bis show as `‹YYYY›-‹MM›-‹DD› (Wd)` with a month grid below; `←/→` moves segment, `↑/↓` changes value, digits type, `t` jumps to today; the grid highlights the selected day + today.
- [ ] Frei add with Bis < Von is rejected (dialog stays open); a valid add appears live.
- [ ] `Esc` in the Frei dialog closes the dialog (does NOT pop the route).
- [ ] Export → `e`: von/bis are pickers; choosing a Range preset fills them; the grid shows when a date field is focused; `Esc` returns to Today.
- [ ] Export writes a file with the picked range.

- [ ] **Step 3: Final commit (if any coverage tests were added)**

```bash
git add -A -- internal/
git commit -m "test(datepicker): coverage top-up"
```

---

## Notes

- Phase A (Tasks 1-4) is independently shippable and immediately unblocks all typed fields; Phase B (5-7) builds the component in isolation; Phase C (8-9) swaps the date text fields for the picker.
- The `datepicker` is clock-free: `t`=today is handled by the routes (which inject `now`), and `Calendar` takes `today` as a parameter.
- Time (`HH:MM`) and project-name fields keep plain text input — Phase A alone fixes them; they are out of scope for the picker.
