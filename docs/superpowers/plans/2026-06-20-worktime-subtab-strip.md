# Worktime Sub-Tab Strip Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the rebuild's Worktime screen a persistent, visible sub-tab strip (Heute · Woche · Stats · Frei · Export) with the active tab highlighted, navigable with ←/→, so the sibling pages are discoverable instead of hidden behind undocumented keys.

**Architecture:** A new single-source-of-truth in `wtnav` (ordered `SubTabs` + `Strip` renderer over the existing `tabstrip` component + a `Lateral` ←/→ → nav-command mapper). Each of the five Worktime routes prepends the strip to its `View()`, wires `←/→` through `Lateral` (except Export, which keeps ←/→ for its date fields), and opts out of the now-redundant breadcrumb via a new `shell.BreadcrumbHider` capability. Heute stays the nav-stack root; siblings drill one level; ← from a sibling to Heute pops.

**Tech Stack:** Go, charm.land/bubbletea/v2 (`tea.KeyPressMsg`, `tea.KeyLeft`/`tea.KeyRight`), charm.land/lipgloss/v2, existing `internal/tui/ui/{tabstrip,keyhint}`, `internal/tui/shell` (`SwitchRouteMsg`, `PopRouteMsg`, `Registry`).

## Global Constraints

- Branch: `rebuild` (unmerged). Do not merge to main.
- `make ci` must stay green (lint + templ + build + tests; coverage gate ≥80%). Run it, not just `go test`. Lint QF1002: a `switch { case k.Code == X }` that becomes all-`k.Code` must be a tagged `switch k.Code {}`.
- German UI strings, proper umlauts (ä/ö/ü/ß). Code/comments English.
- Hints use the ` → ` connector and `  ·  ` separator (never `=`). No emoji; glyphs only via existing components (`tabstrip`/`ui/glyphs`). No raw hex.
- Strip order/labels are fixed: `Heute · Woche · Stats · Frei · Export` (label "Stats" stays). Single source: `wtnav.SubTabs`.
- Heute = home-base model is unchanged: Esc/q on a sibling → Heute; on Heute → leaves Worktime. ← from a sibling to Heute is a `PopRouteMsg`.
- Export keeps its own `←/→` (date-segment + field nav); it switches sub-tabs only via `w/t/d/e`/`Esc`.

---

### Task 1: `wtnav` SubTabs + Strip + Lateral (single source of truth)

**Files:**
- Modify: `internal/tui/screen/worktime/wtnav/wtnav.go`
- Test: `internal/tui/screen/worktime/wtnav/wtnav_test.go` (create if absent)

**Interfaces:**
- Consumes: `tea.KeyPressMsg`, `shell.{Registry-equivalent via this package's Registry, SwitchRouteMsg, PopRouteMsg, Route}`, `tabstrip.Render`, `theme.Palette`.
- Produces:
  - consts `IdxHeute, IdxWoche, IdxStats, IdxFrei, IdxExport int`
  - `type SubTab struct{ Label, Key string }`; `var SubTabs []SubTab`
  - `func Strip(active, width int, pal theme.Palette) string`
  - `func Lateral(reg Registry, current int, k tea.KeyPressMsg) tea.Cmd`

- [ ] **Step 1: Write the failing test**

In `wtnav_test.go` (package `wtnav_test`):

```go
package wtnav_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// stubRoute is a minimal shell.Route for registry factories.
type stubRoute struct{ title string }

func (s stubRoute) Title() string                  { return s.title }
func (s stubRoute) Init() tea.Cmd                  { return nil }
func (s stubRoute) Update(tea.Msg) (shell.Route, tea.Cmd) { return s, nil }
func (s stubRoute) View(shell.Frame) string        { return "" }
func (s stubRoute) KeyHints() []keyhint.Hint        { return nil }

func key(c tea.KeyCode) tea.KeyPressMsg { return tea.KeyPressMsg{Code: c} }
func run(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func testReg() wtnav.Registry {
	return wtnav.Registry{
		"w": func() shell.Route { return stubRoute{title: "Woche"} },
		"t": func() shell.Route { return stubRoute{title: "Stats"} },
		"d": func() shell.Route { return stubRoute{title: "Frei"} },
		"e": func() shell.Route { return stubRoute{title: "Export"} },
	}
}

func TestStrip_ContainsAllLabels(t *testing.T) {
	out := wtnav.Strip(wtnav.IdxStats, 200, theme.Default)
	for _, l := range []string{"Heute", "Woche", "Stats", "Frei", "Export"} {
		if !strings.Contains(out, l) {
			t.Fatalf("strip missing %q: %q", l, out)
		}
	}
}

func TestLateral_RightFromHeutePushesWoche(t *testing.T) {
	m := run(wtnav.Lateral(testReg(), wtnav.IdxHeute, key(tea.KeyRight)))
	sw, ok := m.(shell.SwitchRouteMsg)
	if !ok || sw.Route.Title() != "Woche" {
		t.Fatalf("→ from Heute = %#v, want SwitchRouteMsg(Woche)", m)
	}
}

func TestLateral_LeftFromWochePopsToHeute(t *testing.T) {
	if _, ok := run(wtnav.Lateral(testReg(), wtnav.IdxWoche, key(tea.KeyLeft))).(shell.PopRouteMsg); !ok {
		t.Fatal("← from Woche must emit PopRouteMsg (back to Heute root)")
	}
}

func TestLateral_RightFromStatsSwitchesFrei(t *testing.T) {
	m := run(wtnav.Lateral(testReg(), wtnav.IdxStats, key(tea.KeyRight)))
	sw, ok := m.(shell.SwitchRouteMsg)
	if !ok || sw.Route.Title() != "Frei" {
		t.Fatalf("→ from Stats = %#v, want SwitchRouteMsg(Frei)", m)
	}
}

func TestLateral_ClampsAtEnds(t *testing.T) {
	if wtnav.Lateral(testReg(), wtnav.IdxHeute, key(tea.KeyLeft)) != nil {
		t.Fatal("← from Heute must clamp to nil")
	}
	if wtnav.Lateral(testReg(), wtnav.IdxExport, key(tea.KeyRight)) != nil {
		t.Fatal("→ from Export must clamp to nil")
	}
}

func TestLateral_NonArrowIsNil(t *testing.T) {
	if wtnav.Lateral(testReg(), wtnav.IdxWoche, tea.KeyPressMsg{Text: "x"}) != nil {
		t.Fatal("non-arrow key must return nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/wtnav/ -v`
Expected: FAIL — `Strip`, `Lateral`, `SubTabs`, `Idx*` undefined.

- [ ] **Step 3: Write the implementation**

Append to `internal/tui/screen/worktime/wtnav/wtnav.go` (and add imports `theme` and `tabstrip` to the existing import block):

```go
// Sub-tab indices into SubTabs; each Worktime route declares which one it is.
const (
	IdxHeute = iota
	IdxWoche
	IdxStats
	IdxFrei
	IdxExport
)

// SubTab is one Worktime sub-tab: the strip label and its accelerator key
// ("" for Heute, the nav-stack root reached by popping back).
type SubTab struct {
	Label string
	Key   string
}

// SubTabs is the single source of truth for Worktime sub-tab order, labels, and
// accelerator keys. Positions match the Idx* constants.
var SubTabs = []SubTab{
	{Label: "Heute", Key: ""},
	{Label: "Woche", Key: "w"},
	{Label: "Stats", Key: "t"},
	{Label: "Frei", Key: "d"},
	{Label: "Export", Key: "e"},
}

// Strip renders the Worktime sub-tab strip with active highlighted, reusing the
// shell's top-tab component so it looks identical one level down.
func Strip(active, width int, pal theme.Palette) string {
	labels := make([]string, len(SubTabs))
	for i, t := range SubTabs {
		labels[i] = t.Label
	}
	return tabstrip.Render(labels, active, width, pal)
}

// Lateral maps ←/→ to a sub-tab navigation command relative to current. ← / →
// step one tab (clamped, no wrap). Stepping to Heute from a sibling pops back to
// the root (Heute's live clock resumes via the shell's pop-re-Init); stepping to
// a sibling emits a SwitchRouteMsg through the registry. Returns nil for a
// non-arrow key or a no-op step, so the caller keeps handling the key.
func Lateral(reg Registry, current int, k tea.KeyPressMsg) tea.Cmd {
	var target int
	switch k.Code {
	case tea.KeyLeft:
		target = current - 1
	case tea.KeyRight:
		target = current + 1
	default:
		return nil
	}
	if target < 0 || target >= len(SubTabs) || target == current {
		return nil
	}
	if target == IdxHeute {
		return func() tea.Msg { return shell.PopRouteMsg{} }
	}
	return reg.Nav(SubTabs[target].Key)
}
```

Imports to add to the existing block in `wtnav.go`:

```go
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/tabstrip"
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/screen/worktime/wtnav/ -v`
Expected: PASS (all six tests).

- [ ] **Step 5: Run make ci**

Run: `make ci`
Expected: green (no cycle: `wtnav` → `tabstrip`/`theme`/`shell`; none import `wtnav`).

- [ ] **Step 6: Commit**

```bash
git add internal/tui/screen/worktime/wtnav/
git commit -m "feat(worktime): wtnav SubTabs + Strip + Lateral (sub-tab single source)"
```

---

### Task 2: `shell.BreadcrumbHider` capability + suppress redundant breadcrumb

**Files:**
- Modify: `internal/tui/shell/route.go` (add interface)
- Modify: `internal/tui/shell/shell.go` (`View()` render, after the `crumbs :=` line at ~`:277`)
- Test: `internal/tui/shell/shell_test.go`

**Interfaces:**
- Produces: `type BreadcrumbHider interface{ HideBreadcrumb() bool }`.
- Consumes: nothing new.

- [ ] **Step 1: Write the failing test**

Add to `internal/tui/shell/shell_test.go`. Mirror the existing Shell construction in that file (e.g. the depth-2 setup used by `TestShell_docsBackPopsExactlyOneLevel`): build a Shell whose active tab has a root plus one pushed route at depth 2, so `Crumbs()` would yield two entries. Use a stub route that also implements `BreadcrumbHider` returning true for the hider case, and a plain stub for the control.

```go
// hiderRoute is a depth-2 child that suppresses the breadcrumb.
type hiderRoute struct{ stubRoute } // stubRoute: existing minimal Route in shell_test
func (h hiderRoute) HideBreadcrumb() bool { return true }

func TestShell_BreadcrumbHiddenWhenRouteOptsOut(t *testing.T) {
	// Build a Shell at nav-stack depth 2 with a hiderRoute on top.
	// (Mirror the existing shell_test construction; push the child so Crumbs() has 2 entries.)
	s := newTestShellAtDepth2(t, hiderRoute{stubRoute{title: "Woche"}}) // adapt helper/inline to real construction
	out := s.View().String()
	if strings.Contains(out, "›") {
		t.Fatalf("breadcrumb separator must be absent when top hides it:\n%s", out)
	}
}

func TestShell_BreadcrumbShownForPlainRoute(t *testing.T) {
	s := newTestShellAtDepth2(t, stubRoute{title: "Woche"})
	out := s.View().String()
	if !strings.Contains(out, "›") {
		t.Fatalf("breadcrumb separator expected for a non-hider at depth 2:\n%s", out)
	}
}
```

Build `newTestShellAtDepth2` inline using the real Shell/NavStack constructors already used in `shell_test.go` (do not invent an API — read the file and reuse its construction; the helper just packages a root + one pushed `top`). `stubRoute` already exists in this test file; if its fields differ, adapt the `hiderRoute` embedding accordingly.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/shell/ -run 'Breadcrumb' -v`
Expected: FAIL — `HideBreadcrumb` undefined and breadcrumb still rendered for the hider.

- [ ] **Step 3: Add the interface**

Append to `internal/tui/shell/route.go` (near the other capability interfaces like `Backer`/`TextCapturer`):

```go
// BreadcrumbHider lets a route suppress the frame's drill-down breadcrumb when
// it renders its own location indicator (e.g. the Worktime sub-tab strip), so
// the position is not shown twice.
type BreadcrumbHider interface{ HideBreadcrumb() bool }
```

- [ ] **Step 4: Suppress the breadcrumb in shell.go**

In `internal/tui/shell/shell.go` `View()`, immediately after the existing line:

```go
	crumbs := breadcrumb.Render(s.tabs[s.activeTab].Crumbs(), s.pal)
```

add:

```go
	if bh, ok := s.tabs[s.activeTab].Top().(BreadcrumbHider); ok && bh.HideBreadcrumb() {
		crumbs = ""
	}
```

The downstream `chrome` count and `parts` append already special-case `crumbs == ""`, so no other change is needed.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/tui/shell/ -run 'Breadcrumb' -v`
Expected: PASS.

- [ ] **Step 6: Run make ci**

Run: `make ci`
Expected: green.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/shell/route.go internal/tui/shell/shell.go internal/tui/shell/shell_test.go
git commit -m "feat(shell): BreadcrumbHider capability suppresses redundant breadcrumb"
```

---

### Task 3: Heute (TodayRoute) — strip + ←/→ + HideBreadcrumb + hints

**Files:**
- Modify: `internal/tui/screen/worktime/route.go` (`View` `:197`, `handleKey` `:176`, `KeyHints` `:215`)
- Test: `internal/tui/screen/worktime/route_test.go`

**Interfaces:**
- Consumes: `wtnav.{Strip, Lateral, IdxHeute}` (Task 1), `shell.BreadcrumbHider` (Task 2).

- [ ] **Step 1: Write the failing test**

Add to `internal/tui/screen/worktime/route_test.go`. Construct a TodayRoute the way the existing tests do (the package can call `BuildRegistry(nil, theme.Default)` for a real registry):

```go
func TestToday_StripAndLateralAndHideCrumb(t *testing.T) {
	reg := BuildRegistry(nil, theme.Default)
	r := NewTodayRoute(nil, time.Now, theme.Default, reg) // adapt args to the real NewTodayRoute signature
	// Strip is visible even before load.
	out := r.View(shell.Frame{Width: 200, Height: 24, Pal: theme.Default})
	for _, l := range []string{"Heute", "Woche", "Stats", "Frei", "Export"} {
		if !strings.Contains(out, l) {
			t.Fatalf("Today View missing sub-tab %q", l)
		}
	}
	if !r.HideBreadcrumb() {
		t.Fatal("Today must hide the breadcrumb (strip shows position)")
	}
	// → from Heute navigates to Woche.
	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if cmd == nil {
		t.Fatal("→ on Heute must emit a navigation command")
	}
	if sw, ok := cmd().(shell.SwitchRouteMsg); !ok || sw.Route.Title() != "Woche" {
		t.Fatalf("→ on Heute must switch to Woche, got %#v", cmd())
	}
}
```

(If `NewTodayRoute` needs a non-nil `api`, reuse the existing test's fake `todayAPI`; the nav path does not call it.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/ -run TestToday_StripAndLateral -v`
Expected: FAIL — no strip in View, `HideBreadcrumb` undefined, → does nothing.

- [ ] **Step 3: Prepend the strip in View**

Replace the body of `func (r *TodayRoute) View(f shell.Frame) string` so every return path carries the strip:

```go
func (r *TodayRoute) View(f shell.Frame) string {
	strip := wtnav.Strip(wtnav.IdxHeute, f.Width, f.Pal) + "\n"
	if !r.loaded {
		return strip + theme.Dim("  Heute lädt …", f.Pal)
	}
	if r.err != nil {
		return strip + theme.Dim("  Fehler: "+r.err.Error(), f.Pal)
	}
	if r.dialog != dialogNone {
		return strip + r.renderDialog(f)
	}
	return strip + renderBody(r.st, r.cursor, f.Width, f.Height, r.now(), &r.toast, f.Pal)
}
```

- [ ] **Step 4: Wire ←/→ in handleKey**

In `func (r *TodayRoute) handleKey`, immediately after the `listnav` block (before the `switch {`), add:

```go
	if cmd := wtnav.Lateral(r.reg, wtnav.IdxHeute, k); cmd != nil {
		return r, cmd
	}
```

- [ ] **Step 5: Add HideBreadcrumb and update KeyHints**

Add the method (near the other `*TodayRoute` methods):

```go
// HideBreadcrumb implements shell.BreadcrumbHider — the sub-tab strip shows the
// position, so the frame breadcrumb would be redundant.
func (r *TodayRoute) HideBreadcrumb() bool { return true }
```

Replace `KeyHints` so it advertises the area-switch and drops the manual 4-cap (the footer renderer caps at 4 and sends the rest to `?`-help):

```go
func (r *TodayRoute) KeyHints() []keyhint.Hint {
	if r.dialog != dialogNone {
		return r.dialogHints()
	}
	hints := []keyhint.Hint{}
	if r.st.Running {
		hints = append(hints, keyhint.Hint{Key: "s", Desc: "stoppen"})
	} else {
		hints = append(hints, keyhint.Hint{Key: "s", Desc: "starten"})
	}
	hints = append(hints, grammar.MoveUp.Hint())
	if len(r.st.Completed) > 0 {
		hints = append(hints, keyhint.Hint{Key: "enter", Desc: "bearbeiten"})
	}
	hints = append(hints, keyhint.Hint{Key: "←/→", Desc: "Bereich"})
	hints = append(hints, keyhint.Hint{Key: "?", Desc: "Hilfe"})
	return hints
}
```

(`wtnav` is already imported in `route.go`. The existing `w/t/d/e` case in `handleKey` stays as direct accelerators.)

- [ ] **Step 6: Run tests**

Run: `go test ./internal/tui/screen/worktime/ -run TestToday -v`
Expected: PASS.

- [ ] **Step 7: Run make ci**

Run: `make ci`
Expected: green.

- [ ] **Step 8: Commit**

```bash
git add internal/tui/screen/worktime/route.go internal/tui/screen/worktime/route_test.go
git commit -m "feat(worktime): Heute renders sub-tab strip + ←/→ nav + hides breadcrumb"
```

---

### Task 4: Woche + Stats — strip + ←/→ + HideBreadcrumb + hints

**Files:**
- Modify: `internal/tui/screen/worktime/week/route.go` (`Update` `:62`, `View` `:80`, `KeyHints` `:107`)
- Modify: `internal/tui/screen/worktime/statsrange/route.go` (`Update` `:69`, `View` `:105`, `KeyHints` `:141`)
- Test: `internal/tui/screen/worktime/week/route_test.go`, `internal/tui/screen/worktime/statsrange/route_test.go`

**Interfaces:**
- Consumes: `wtnav.{Strip, Lateral, IdxWoche, IdxStats}`, `shell.BreadcrumbHider`.

- [ ] **Step 1: Write the failing tests**

In `week/route_test.go` (mirror the existing construction; `week.NewRoute(api, pal, reg)`):

```go
func TestWeek_StripAndLeftPopsAndHideCrumb(t *testing.T) {
	reg := wtnav.Registry{
		"w": func() shell.Route { return week.NewRoute(nil, theme.Default, nil) },
		"t": func() shell.Route { return statsStub{} }, // any shell.Route stub
	}
	r := week.NewRoute(nil, theme.Default, reg)
	out := r.View(shell.Frame{Width: 200, Height: 24, Pal: theme.Default})
	for _, l := range []string{"Heute", "Woche", "Stats", "Frei", "Export"} {
		if !strings.Contains(out, l) {
			t.Fatalf("Woche View missing sub-tab %q", l)
		}
	}
	if !r.HideBreadcrumb() {
		t.Fatal("Woche must hide breadcrumb")
	}
	// ← from Woche pops back to Heute (deterministic, no registry needed).
	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if cmd == nil {
		t.Fatal("← on Woche must emit a command")
	}
	if _, ok := cmd().(shell.PopRouteMsg); !ok {
		t.Fatalf("← on Woche must pop to Heute, got %#v", cmd())
	}
}
```

Use the simplest stub `shell.Route` available in the test (define a tiny local one if none exists). In `statsrange/route_test.go`, the analogous test on `statsrange.NewRoute(nil, theme.Default, reg)` asserting the strip labels, `HideBreadcrumb()`, and that `←` (from Stats, idx 2 → Woche) emits a non-nil command:

```go
func TestStats_StripAndLateralAndHideCrumb(t *testing.T) {
	reg := wtnav.Registry{"w": func() shell.Route { return wocheStub{} }}
	r := statsrange.NewRoute(nil, theme.Default, reg)
	out := r.View(shell.Frame{Width: 200, Height: 24, Pal: theme.Default})
	if !strings.Contains(out, "Stats") || !strings.Contains(out, "Export") {
		t.Fatalf("Stats View missing strip labels:\n%s", out)
	}
	if !r.HideBreadcrumb() {
		t.Fatal("Stats must hide breadcrumb")
	}
	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyLeft}) // → Woche via reg
	if cmd == nil {
		t.Fatal("← on Stats must emit a command")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/screen/worktime/week/ ./internal/tui/screen/worktime/statsrange/ -run 'Week_Strip|Stats_Strip' -v`
Expected: FAIL — no strip, `HideBreadcrumb` undefined, ← no-op.

- [ ] **Step 3: week — strip in View, Lateral in Update, HideBreadcrumb, hints**

In `week/route.go` `View`, capture the strip and prepend on every return:

```go
func (r *Route) View(f shell.Frame) string {
	strip := wtnav.Strip(wtnav.IdxWoche, f.Width, f.Pal) + "\n"
	if !r.loaded {
		return strip + theme.Dim("  Woche lädt …", f.Pal)
	}
	if r.err != nil {
		return strip + theme.Dim("  Fehler: "+r.err.Error(), f.Pal)
	}
	cells := 20
	var b strings.Builder
	b.WriteString("\n")
	for _, d := range r.days {
		marker := "  "
		if d.IsToday {
			marker = theme.Active(glyphs.Active, f.Pal) + " "
		}
		pct := 0
		if d.TargetMin > 0 {
			pct = d.LoggedMin * 100 / d.TargetMin
		}
		line := fmt.Sprintf("%s%s  %s  %s / %s",
			marker, d.Date, statusbar.Bar(pct, cells, f.Pal),
			wtfmt.FormatMin(d.LoggedMin), wtfmt.FormatMin(d.TargetMin))
		b.WriteString("  " + line + "\n")
	}
	return strip + b.String()
}
```

In `week/route.go` `Update`, the `tea.KeyPressMsg` case, add `Lateral` before `navKey`:

```go
	case tea.KeyPressMsg:
		if cmd := wtnav.Lateral(r.reg, wtnav.IdxWoche, m); cmd != nil {
			return r, cmd
		}
		if cmd := navKey(r.reg, m); cmd != nil {
			return r, cmd
		}
```

Add the method and replace `KeyHints`:

```go
// HideBreadcrumb implements shell.BreadcrumbHider.
func (r *Route) HideBreadcrumb() bool { return true }

func (r *Route) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "←/→", Desc: "Bereich"},
		{Key: "esc", Desc: "zurück"},
	}
}
```

Add the `shell` import to `week/route.go` if not present (it is — `View(f shell.Frame)`), and `wtnav` is already imported.

- [ ] **Step 4: statsrange — same pattern (IdxStats)**

In `statsrange/route.go` `View`, prepend `strip := wtnav.Strip(wtnav.IdxStats, f.Width, f.Pal) + "\n"` and return `strip + ...` on every path (the loading/error early returns and the final `return b.String()` → `return strip + b.String()`), exactly as in week.

In `statsrange/route.go` `Update`, the `case tea.KeyPressMsg:` block contains a nested `switch m.Text { case "m": … case "W": … default: navKey(...) }`. Add the `Lateral` check as the **first statements of the `case tea.KeyPressMsg:` block, before `switch m.Text {`** (←/→ are `Code`, so `m.Text` is "" and they would otherwise fall to `default`; checking first is cleanest and mirrors week):

```go
	case tea.KeyPressMsg:
		if cmd := wtnav.Lateral(r.reg, wtnav.IdxStats, m); cmd != nil {
			return r, cmd
		}
		switch m.Text {
		case "m":
			// ... existing m/W/default body unchanged ...
```

Add `HideBreadcrumb` and update `KeyHints` (keep the m/W verb):

```go
// HideBreadcrumb implements shell.BreadcrumbHider.
func (r *Route) HideBreadcrumb() bool { return true }

func (r *Route) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "m/W", Desc: "Monat/KW"},
		{Key: "←/→", Desc: "Bereich"},
		{Key: "esc", Desc: "zurück"},
	}
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/tui/screen/worktime/week/ ./internal/tui/screen/worktime/statsrange/ -v`
Expected: PASS.

- [ ] **Step 6: Run make ci**

Run: `make ci`
Expected: green.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/screen/worktime/week/ internal/tui/screen/worktime/statsrange/
git commit -m "feat(worktime): Woche+Stats render sub-tab strip + ←/→ nav + hide breadcrumb"
```

---

### Task 5: Frei (dayoffs) — strip + ←/→ + HideBreadcrumb + hints

**Files:**
- Modify: `internal/tui/screen/worktime/dayoffs/route.go` (`handleKey` `:126`, `View` `:149`, `KeyHints` `:198`)
- Test: `internal/tui/screen/worktime/dayoffs/route_test.go`

**Interfaces:**
- Consumes: `wtnav.{Strip, Lateral, IdxFrei}`, `shell.BreadcrumbHider`.

- [ ] **Step 1: Write the failing test**

In `dayoffs/route_test.go` (mirror existing construction; `dayoffs.NewRoute(api, pal, reg, now)`):

```go
func TestDayoffs_StripAndLeftPopsAndHideCrumb(t *testing.T) {
	reg := wtnav.Registry{"t": func() shell.Route { return statsStub{} }}
	r := dayoffs.NewRoute(nil, theme.Default, reg, time.Now)
	out := r.View(shell.Frame{Width: 200, Height: 24, Pal: theme.Default})
	for _, l := range []string{"Heute", "Woche", "Stats", "Frei", "Export"} {
		if !strings.Contains(out, l) {
			t.Fatalf("Frei View missing sub-tab %q", l)
		}
	}
	if !r.HideBreadcrumb() {
		t.Fatal("Frei must hide breadcrumb")
	}
	// ← from Frei (idx 3) → Stats via reg.
	r2, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	_ = r2
	if cmd == nil {
		t.Fatal("← on Frei must emit a command")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/dayoffs/ -run TestDayoffs_Strip -v`
Expected: FAIL — no strip, `HideBreadcrumb` undefined, ← no-op.

- [ ] **Step 3: Wire ←/→ in handleKey**

In `dayoffs/route.go` `handleKey`, immediately after the `listnav` block (before `switch k.Text {`), add:

```go
	if cmd := wtnav.Lateral(r.reg, wtnav.IdxFrei, k); cmd != nil {
		return r, cmd
	}
```

(The dialog guard at the top of `handleKey` returns before this, so ←/→ is inert while a dialog is open — correct.)

- [ ] **Step 4: Prepend the strip in View**

In `dayoffs/route.go` `View`, prepend the strip on every return path:

```go
func (r *Route) View(f shell.Frame) string {
	strip := wtnav.Strip(wtnav.IdxFrei, f.Width, f.Pal) + "\n"
	if !r.loaded {
		return strip + theme.Dim("  Frei lädt …", f.Pal)
	}
	if r.err != nil {
		return strip + theme.Dim("  Fehler: "+r.err.Error(), f.Pal)
	}
	if r.dialog != dialogNone {
		return strip + r.renderDialog(f)
	}
	var b strings.Builder
	// ... existing body building unchanged ...
	return strip + b.String()
}
```

(Keep the entire existing body-building block between `var b strings.Builder` and the final return; only the early returns gain `strip +` and the final `return b.String()` becomes `return strip + b.String()`.)

- [ ] **Step 5: Add HideBreadcrumb and update KeyHints**

```go
// HideBreadcrumb implements shell.BreadcrumbHider.
func (r *Route) HideBreadcrumb() bool { return true }

func (r *Route) KeyHints() []keyhint.Hint {
	if r.dialog != dialogNone {
		return r.dialogHints()
	}
	return []keyhint.Hint{
		{Key: "g/a/D", Desc: "Ziel/Add/Del"},
		{Key: "b", Desc: "Bundesland"},
		{Key: "←/→", Desc: "Bereich"},
		{Key: "esc", Desc: "zurück"},
	}
}
```

(`wtnav` already imported in `dayoffs/route.go`. The existing `w/t/d/e` case in `handleKey` stays as accelerators.)

- [ ] **Step 6: Run tests**

Run: `go test ./internal/tui/screen/worktime/dayoffs/ -v`
Expected: PASS.

- [ ] **Step 7: Run make ci**

Run: `make ci`
Expected: green.

- [ ] **Step 8: Commit**

```bash
git add internal/tui/screen/worktime/dayoffs/
git commit -m "feat(worktime): Frei renders sub-tab strip + ←/→ nav + hides breadcrumb"
```

---

### Task 6: Export — strip + HideBreadcrumb + hints (keeps its own ←/→)

**Files:**
- Modify: `internal/tui/screen/worktime/export/route.go` (`View` `:255`, `KeyHints` `:291`)
- Test: `internal/tui/screen/worktime/export/route_test.go`

**Interfaces:**
- Consumes: `wtnav.{Strip, IdxExport}`, `shell.BreadcrumbHider`. (No `Lateral` — Export keeps ←/→ for its date/field nav.)

- [ ] **Step 1: Write the failing test**

In `export/route_test.go` (mirror existing construction; `export.NewRoute(api, now, pal, reg)`):

```go
func TestExport_StripAndHideCrumbAndArrowsStillEditDate(t *testing.T) {
	r := export.NewRoute(nil, time.Now, theme.Default, nil)
	out := r.View(shell.Frame{Width: 200, Height: 24, Pal: theme.Default})
	for _, l := range []string{"Heute", "Woche", "Stats", "Frei", "Export"} {
		if !strings.Contains(out, l) {
			t.Fatalf("Export View missing sub-tab %q", l)
		}
	}
	if !r.HideBreadcrumb() {
		t.Fatal("Export must hide breadcrumb")
	}
	// ←/→ must NOT switch sub-tabs on Export (it edits the date/field). With a
	// nil registry, a sub-tab switch would have been a no-op anyway; assert the
	// route does not return a SwitchRouteMsg/PopRouteMsg for ←.
	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if cmd != nil {
		if m := cmd(); func() bool { _, a := m.(shell.SwitchRouteMsg); _, b := m.(shell.PopRouteMsg); return a || b }() {
			t.Fatalf("← on Export must not switch sub-tabs, got %#v", m)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/export/ -run TestExport_Strip -v`
Expected: FAIL — no strip, `HideBreadcrumb` undefined.

- [ ] **Step 3: Prepend the strip in View**

In `export/route.go` `View`, return the strip prepended to the built body. Change the final `return b.String()` to:

```go
	return wtnav.Strip(wtnav.IdxExport, f.Width, f.Pal) + "\n" + b.String()
```

- [ ] **Step 4: Add HideBreadcrumb and extend KeyHints**

```go
// HideBreadcrumb implements shell.BreadcrumbHider.
func (r *Route) HideBreadcrumb() bool { return true }

func (r *Route) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "tab", Desc: "Feld"},
		{Key: "←/→", Desc: "wählen"},
		{Key: "enter", Desc: "export"},
		{Key: "esc", Desc: "zurück"},
		{Key: "w/t/d/e", Desc: "Bereich"},
	}
}
```

(The 5th hint exceeds the 4-hint footer cap, so it surfaces in the `?`-help overlay — Export's discoverability comes primarily from the always-visible strip. `wtnav` is already imported in `export/route.go`.)

- [ ] **Step 5: Run tests**

Run: `go test ./internal/tui/screen/worktime/export/ -v`
Expected: PASS.

- [ ] **Step 6: Run make ci**

Run: `make ci`
Expected: green.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/screen/worktime/export/
git commit -m "feat(worktime): Export renders sub-tab strip + hides breadcrumb (keeps date ←/→)"
```

---

### Task 7: Wiring verification + manual key-walk

**Files:**
- None (verification task).

- [ ] **Step 1: Confirm all five routes carry the strip and capability**

Run: `rg -n 'wtnav.Strip|HideBreadcrumb' internal/tui/screen/worktime`
Expected: `Strip` appears in all five route `View`s (route.go, week, statsrange, dayoffs, export); `HideBreadcrumb` on all five.

- [ ] **Step 2: Confirm ←/→ wiring (Lateral on four, not Export)**

Run: `rg -n 'wtnav.Lateral' internal/tui/screen/worktime`
Expected: `Lateral` in Heute (route.go), week, statsrange, dayoffs — NOT export.

- [ ] **Step 3: `make ci` green**

Run: `make ci`
Expected: lint + templ + build + tests pass; coverage ≥ gate.

- [ ] **Step 4: Manual key-walk against the dev stack**

Start the dev stack and `flow ui worktime` (see `reference_flow_dev_env`: `make dev-up`, `make dev-run`). Verify:
- The strip `Heute · Woche · Stats · Frei · Export` is visible on every Worktime sub-tab with the current one highlighted; the breadcrumb no longer shows `Worktime › …`.
- On Heute/Woche/Stats/Frei: `→` advances along the strip, `←` goes back; `←` from Woche returns to Heute (live clock keeps running); `Esc`/`q` on a sibling returns to Heute, on Heute leaves Worktime.
- `w/t/d/e` still jump directly from any route.
- On Export: `←/→` still moves the date segments / field selection (does NOT switch sub-tabs); switching away uses `w/t/d/e` or `Esc`; the strip still shows `Export` active.
- Footers read the grammar (`←/→ → Bereich`, etc.); `?`-help lists the accelerators.

- [ ] **Step 5: Commit any final note (if needed)**

```bash
git add -A
git commit -m "docs: worktime sub-tab strip wiring verified" || true
```

---

## Self-Review notes

- **Spec coverage:** SubTabs/Strip/Lateral → Task 1; BreadcrumbHider + suppression → Task 2; the five routes (strip + ←/→ + HideBreadcrumb + hints, Export exception) → Tasks 3–6; verification + manual key-walk → Task 7. Decisions 1–5 from the spec are all covered.
- **Type consistency:** `wtnav.Strip(active, width int, pal theme.Palette) string`, `wtnav.Lateral(reg Registry, current int, k tea.KeyPressMsg) tea.Cmd`, `Idx{Heute,Woche,Stats,Frei,Export}`, `BreadcrumbHider.HideBreadcrumb() bool` are used identically across tasks.
- **Export exception** is explicit: Task 6 adds the strip + capability + hints but NOT `Lateral`, so ←/→ stays Export's date/field nav (asserted by `TestExport_...ArrowsStillEditDate` and the Task 7 `rg` check).
- **No-cycle note:** `wtnav` newly imports `tabstrip` + `theme` (both leaves); nothing imports `wtnav` except the worktime leaf routes, so no import cycle.
- **Out of scope:** strip mouse/click, route renames, Export's internal ←/→ semantics, the deferred markdown-viewer g/G scroll keys.
