# flow rebuild M3b — Sidekick-Shell Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `flow ui` sidekick-shell — one bubbletea-v2 program hosting a top **tabstrip**, a `:`-**command-palette**, a per-tab **nav-stack router** with live **drill-down/back**, plus the chrome components (`header`, `breadcrumb`, `overlay`, `keyhint`-footer) the M3 spec lists. M3b ships placeholder routes (Home → About drill-down); real screens are M3c/M3d. `flow worktime`/`flow docs` keep working standalone.

**Architecture:** New `ui` chrome packages (`keyhint`, `tabstrip`, `header`, `breadcrumb`, `overlay`) on top of the M3a design-system, plus a new `internal/tui/shell/` package: `route.go` (the `Route` contract + `Frame` + nav messages), `navstack.go` (per-tab LIFO stack), `palette.go` (`:`-palette model), `home.go`/`about.go` (placeholder routes), `shell.go` (the root `tea.Model` wiring chrome + routing + SSE fan-out), and `host.go` (a chrome-less single-Route standalone host for M3c). Each screen implements `Route`; the shell switches the active **stack** on tab change, **pushes** on drill-down, **pops** on `esc`. Modals (palette, `?`-help) render as centered boxes via the `overlay` compositor.

**Tech Stack:** Go 1.25.7; `charm.land/bubbletea/v2 v2.0.6`, `charm.land/bubbles/v2 v2.1.0`, `charm.land/lipgloss/v2 v2.0.3` (all already in go.mod from M3a); the M3a packages `internal/tui/theme` + `internal/tui/ui/{statusbar,help,picker,titlebox,glyphs}`; `internal/adapter/apiclient`. No new third-party deps (palette uses `strings.Contains`, not fuzzy — see Note in Task 8).

---

## Context for the implementer — VERIFIED APIs (do not redefine; these were confirmed against the pinned deps)

### bubbletea v2 Model contract — THE most important detail
```go
import tea "charm.land/bubbletea/v2"

// Every tea.Model in this repo (see internal/tui/worktime.go):
func (m Model) Init() tea.Cmd
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd)
func (m Model) View() tea.View          // returns tea.View, NOT string

// AltScreen is set INSIDE View() on the tea.View value, never via a program option:
v := tea.NewView(someString)
v.AltScreen = true
return v
```

### bubbletea v2 KEY handling — the repo uses `tea.KeyPressMsg`, NOT `tea.KeyMsg`
```go
// The key message is tea.KeyPressMsg. Its fields:
//   k.Text string   // the printed characters, e.g. "q", ":", "?", "2"; EMPTY for special keys
//   k.Code rune      // tea.KeyEsc | tea.KeyEnter | tea.KeyTab | tea.KeyUp | tea.KeyDown |
//                    // tea.KeyBackspace | tea.KeySpace | a rune literal like 'c'
//   k.Mod  tea.KeyMod // bitmask: tea.ModCtrl | tea.ModShift | tea.ModAlt
//
// There is NO msg.String(), NO key.Matches(), NO tea.KeyCtrlC, NO tea.KeyShiftTab.
// Idioms used in this repo (internal/tui/worktime.go):
case k.Text == "q" || (k.Code == 'c' && k.Mod == tea.ModCtrl):   // quit
case k.Code == tea.KeyEsc:                                       // esc
case k.Code == tea.KeyEnter:                                     // enter
// Tab / Shift+Tab:
case k.Code == tea.KeyTab && k.Mod == tea.ModShift:              // shift+tab (best-effort per terminal)
case k.Code == tea.KeyTab:                                       // tab
// Construct in tests:
tea.KeyPressMsg{Text: "2"}                       // digit 2
tea.KeyPressMsg{Text: ":"}                       // colon
tea.KeyPressMsg{Code: tea.KeyTab}                // tab
tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}  // shift+tab
tea.KeyPressMsg{Code: tea.KeyEsc}                // esc
tea.KeyPressMsg{Code: tea.KeyEnter}              // enter
tea.KeyPressMsg{Code: tea.KeyDown}               // arrow down
```

### theme (M3a) — verified
```go
import "github.com/serverkraken/flow/internal/tui/theme"
var theme.Default theme.Palette        // = TokyonightNight; use in tests
func theme.Load() theme.Palette        // never errors; reads tmux overrides
func (p theme.Palette) Sem() theme.Semantic
// Semantic fields: Accent Active Success Warning Notice Danger Info Schedule Highlight SearchCurrent Border BorderSubtle BorderStrong
// Palette chrome fields: Bg BgPanel BgCode BgChip BgChipSoft BgBar BgDanger BgSuccess  Fg FgDim FgMuted  (+ named colors)
// Builders, all (s string, p theme.Palette) string:
//   theme.Heading (Accent+Bold)  theme.Body (Fg)  theme.Dim (FgMuted)  theme.Strong (Fg+Bold)
//   theme.Active (Cyan+Bold)  theme.Success  theme.Warning  theme.Danger  theme.Info  theme.Highlight
// Tokens (consts): theme.PadXS=1 theme.PadSM=2  theme.KeyHintWidth=12  theme.DefaultBox=60 theme.NarrowBox=40 theme.WideBox=80
```

### ported ui components (M3a) — verified signatures
```go
import "github.com/serverkraken/flow/internal/tui/ui/statusbar"
func statusbar.Hints(text string, p theme.Palette) string

import "github.com/serverkraken/flow/internal/tui/ui/help"
type help.Section struct { Title string; Keys [][2]string }
func help.Render(title string, sections []help.Section, keyWidth, boxWidth int, p theme.Palette) string

import "github.com/serverkraken/flow/internal/tui/ui/picker"
func picker.Row(selected bool, label, hint string, width int, p theme.Palette) string

import "github.com/serverkraken/flow/internal/tui/ui/titlebox"
func titlebox.Render(title, body string, width int, p theme.Palette) string
```

### lipgloss v2 — verified helpers
```go
import "charm.land/lipgloss/v2"
func lipgloss.Place(width, height int, hPos, vPos lipgloss.Position, str string, opts ...lipgloss.WhitespaceOption) string
const ( lipgloss.Top; lipgloss.Bottom; lipgloss.Center; lipgloss.Left; lipgloss.Right ) // Position values
func lipgloss.Width(string) int
func lipgloss.Height(string) int
lipgloss.NewStyle().Foreground(c).Background(c).Bold(true).Padding(0, n).Render(s)
```

### apiclient + cmd wiring — verified
```go
import "github.com/serverkraken/flow/internal/adapter/apiclient"
type apiclient.ClientEvent struct { Type string; Data map[string]any }
func (c *apiclient.Client) Events(ctx context.Context) (<-chan apiclient.ClientEvent, error)
// In cmd/flow (package main), auth.go:
func clientFromStore(ctx context.Context) (*apiclient.Client, error)
// cmd/flow/worktime.go launch pattern (copy verbatim):
//   client, err := clientFromStore(cmd.Context())
//   logf, _ := os.OpenFile(filepath.Join(os.TempDir(), "flow-tui.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); os.Stderr = logf
//   _, err = tea.NewProgram(m, tea.WithContext(cmd.Context())).Run()
// cmd/flow/main.go rootCmd(): root := &cobra.Command{Use:"flow", Short:"flow client"} ; root.AddCommand(...) ; NO RunE today.
```

### What NOT to touch
- `internal/tui/*.go` (worktime.go, docs.go, stats.go, export.go, dayoffs.go, styles.go, weblink.go) and their tests — the standalone screens stay as-is.
- `cmd/flow/worktime.go`, `cmd/flow/docs.go`, `cmd/flow/auth.go`, and the other existing subcommand files.
- Any M3a package implementation (only consume them).

---

## File map

| File | Responsibility | New/Modify |
|---|---|---|
| `internal/tui/ui/keyhint/keyhint.go` | `Hint{Key,Desc}` + footer renderer (max 4) | New |
| `internal/tui/ui/tabstrip/tabstrip.go` | tab row, active highlight, narrow-width overflow | New |
| `internal/tui/ui/header/header.go` | app title (left) + user (right) | New |
| `internal/tui/ui/breadcrumb/breadcrumb.go` | drill-down depth `A › B › C` | New |
| `internal/tui/ui/overlay/overlay.go` | center a box (lipgloss.Place) for modals | New |
| `internal/tui/shell/route.go` | `Route` interface, `Frame`, `PushRouteMsg`/`PopRouteMsg` | New |
| `internal/tui/shell/navstack.go` | per-tab LIFO `NavStack` | New |
| `internal/tui/shell/palette.go` | `:`-command-palette model | New |
| `internal/tui/shell/home.go` | `HomeRoute` (drill-down trigger) | New |
| `internal/tui/shell/about.go` | `AboutRoute` (drill-down target) | New |
| `internal/tui/shell/shell.go` | `Shell` root `tea.Model`: chrome + routing + SSE | New |
| `internal/tui/shell/host.go` | `RouteHost` — chrome-less single-Route host | New |
| `cmd/flow/ui.go` | `uiCmd()` cobra command | New |
| `cmd/flow/main.go` | add `uiCmd()` + root no-args `RunE` | Modify |
| (+ a `_test.go` beside each new `.go`) | unit tests | New |

Build order = leaves first (ui components), then shell contract, then routes, then the Shell that wires everything, then cmd.

---

## Task 1: `ui/keyhint` — footer hints

**Files:** Create `internal/tui/ui/keyhint/keyhint.go`, `internal/tui/ui/keyhint/keyhint_test.go`

- [ ] **Step 1: Write the failing test** — `internal/tui/ui/keyhint/keyhint_test.go`
```go
package keyhint_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

func TestRender_includesKeysAndDescs(t *testing.T) {
	got := keyhint.Render([]keyhint.Hint{{Key: "tab", Desc: "Tab wechseln"}, {Key: ":", Desc: "Palette"}}, theme.Default)
	for _, want := range []string{"tab", "Tab wechseln", ":", "Palette"} {
		if !strings.Contains(got, want) {
			t.Fatalf("hints %q missing %q", got, want)
		}
	}
}

func TestRender_capsAtFour(t *testing.T) {
	hints := []keyhint.Hint{{Key: "a"}, {Key: "b"}, {Key: "c"}, {Key: "d"}, {Key: "e"}, {Key: "f"}}
	got := keyhint.Render(hints, theme.Default)
	if strings.Contains(got, "f") || strings.Contains(got, "e") {
		t.Fatalf("expected only first 4 hints rendered, got %q", got)
	}
}

func TestRender_emptyIsEmpty(t *testing.T) {
	if keyhint.Render(nil, theme.Default) != "" {
		t.Fatal("nil hints should render empty string")
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`no required module provides package .../keyhint`)
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/ui/keyhint/ 2>&1 | head
```

- [ ] **Step 3: Implement** — `internal/tui/ui/keyhint/keyhint.go`
```go
// Package keyhint renders the contextual footer key-hint line. A Route
// returns []Hint from KeyHints(); the shell shows up to maxFooter of them in
// the footer and the rest in the ?-help overlay.
package keyhint

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// Hint is one footer key-hint: a key token and what it does.
type Hint struct {
	Key  string
	Desc string
}

// maxFooter is how many hints fit on the footer line; the rest live in ?-help.
const maxFooter = 4

// Render formats up to maxFooter hints as "key desc  ·  key desc". Returns ""
// for no hints. Keys are accented, descriptions dimmed.
func Render(hints []Hint, p theme.Palette) string {
	if len(hints) == 0 {
		return ""
	}
	n := len(hints)
	if n > maxFooter {
		n = maxFooter
	}
	parts := make([]string, 0, n)
	for _, h := range hints[:n] {
		seg := theme.Active(h.Key, p)
		if h.Desc != "" {
			seg += " " + theme.Dim(h.Desc, p)
		}
		parts = append(parts, seg)
	}
	line := strings.Join(parts, theme.Dim("  ·  ", p))
	return lipgloss.NewStyle().Padding(0, theme.PadXS).Render(line)
}
```

- [ ] **Step 4: Run — expect PASS**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/ui/keyhint/ -v 2>&1 | tail
```

- [ ] **Step 5: Commit**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add internal/tui/ui/keyhint && git commit -m "feat(m3b): keyhint footer component"
```

---

## Task 2: `ui/tabstrip` — tab row with overflow

**Files:** Create `internal/tui/ui/tabstrip/tabstrip.go`, `internal/tui/ui/tabstrip/tabstrip_test.go`

- [ ] **Step 1: Write the failing test**
```go
package tabstrip_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/tabstrip"
)

func TestRender_showsAllWhenWide(t *testing.T) {
	got := tabstrip.Render([]string{"Home", "Worktime", "Docs"}, 1, 200, theme.Default)
	for _, w := range []string{"Home", "Worktime", "Docs"} {
		if !strings.Contains(got, w) {
			t.Fatalf("wide strip %q missing %q", got, w)
		}
	}
}

func TestRender_empty(t *testing.T) {
	if tabstrip.Render(nil, 0, 80, theme.Default) != "" {
		t.Fatal("nil titles -> empty")
	}
}

func TestRender_overflowFitsWidthAndKeepsActive(t *testing.T) {
	titles := []string{"Home", "Worktime", "Docs", "Stats", "DayOffs", "Export", "Projects"}
	const width = 24
	got := tabstrip.Render(titles, 5, width, theme.Default) // active = "Export"
	if lipgloss.Width(got) > width {
		t.Fatalf("overflow strip width %d exceeds %d: %q", lipgloss.Width(got), width, got)
	}
	if !strings.Contains(got, "Export") {
		t.Fatalf("overflow strip must keep active tab, got %q", got)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/ui/tabstrip/ 2>&1 | head
```

- [ ] **Step 3: Implement** — `internal/tui/ui/tabstrip/tabstrip.go`
```go
// Package tabstrip renders the shell's top tab row. When the rendered tabs
// exceed the available width it shows a window around the active tab with
// "‹"/"›" overflow markers so the active tab is always visible.
package tabstrip

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// Render draws the tab row. active is the selected tab index; width is the
// usable terminal width. Returns "" for no titles.
func Render(titles []string, active, width int, p theme.Palette) string {
	if len(titles) == 0 {
		return ""
	}
	sem := p.Sem()
	cells := make([]string, len(titles))
	for i, t := range titles {
		label := " " + t + " "
		if i == active {
			cells[i] = lipgloss.NewStyle().Foreground(sem.Active).Bold(true).Background(p.BgChipSoft).Render(label)
		} else {
			cells[i] = lipgloss.NewStyle().Foreground(p.FgMuted).Render(label)
		}
	}
	full := strings.Join(cells, " ")
	if width <= 0 || lipgloss.Width(full) <= width {
		return full
	}

	// Overflow: grow a window outward from the active tab while it fits,
	// reserving 2 cols for the "‹ "/" ›" markers.
	if active < 0 || active >= len(cells) {
		active = 0
	}
	lo, hi := active, active
	used := lipgloss.Width(cells[active])
	budget := width - 2
	for {
		grew := false
		if lo > 0 && used+1+lipgloss.Width(cells[lo-1]) <= budget {
			lo--
			used += 1 + lipgloss.Width(cells[lo])
			grew = true
		}
		if hi < len(cells)-1 && used+1+lipgloss.Width(cells[hi+1]) <= budget {
			hi++
			used += 1 + lipgloss.Width(cells[hi])
			grew = true
		}
		if !grew {
			break
		}
	}
	var b strings.Builder
	if lo > 0 {
		b.WriteString(theme.Dim("‹ ", p))
	}
	b.WriteString(strings.Join(cells[lo:hi+1], " "))
	if hi < len(cells)-1 {
		b.WriteString(theme.Dim(" ›", p))
	}
	return b.String()
}
```

- [ ] **Step 4: Run — expect PASS**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/ui/tabstrip/ -v 2>&1 | tail
```

- [ ] **Step 5: Commit**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add internal/tui/ui/tabstrip && git commit -m "feat(m3b): tabstrip component with narrow-width overflow"
```

---

## Task 3: `ui/header` — app title + user

**Files:** Create `internal/tui/ui/header/header.go`, `internal/tui/ui/header/header_test.go`

- [ ] **Step 1: Write the failing test**
```go
package header_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/header"
)

func TestRender_containsTitleAndUser(t *testing.T) {
	got := header.Render("flow", "alice", 40, theme.Default)
	if !strings.Contains(got, "flow") || !strings.Contains(got, "alice") {
		t.Fatalf("header %q missing title or user", got)
	}
	if lipgloss.Width(got) > 40 {
		t.Fatalf("header width %d exceeds 40", lipgloss.Width(got))
	}
}

func TestRender_narrowDoesNotPanic(t *testing.T) {
	_ = header.Render("flow", "alice", 3, theme.Default)
}
```

- [ ] **Step 2: Run — expect FAIL**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/ui/header/ 2>&1 | head
```

- [ ] **Step 3: Implement** — `internal/tui/ui/header/header.go`
```go
// Package header renders the shell's top line: app title on the left, the
// current user on the right, separated by flexible space.
package header

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// Render draws "title ............ user" padded to width.
func Render(title, user string, width int, p theme.Palette) string {
	left := theme.Heading(title, p)
	right := theme.Dim(user, p)
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}
```

- [ ] **Step 4: Run — expect PASS**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/ui/header/ -v 2>&1 | tail
```

- [ ] **Step 5: Commit**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add internal/tui/ui/header && git commit -m "feat(m3b): header component (title + user)"
```

---

## Task 4: `ui/breadcrumb` — drill-down depth

**Files:** Create `internal/tui/ui/breadcrumb/breadcrumb.go`, `internal/tui/ui/breadcrumb/breadcrumb_test.go`

- [ ] **Step 1: Write the failing test**
```go
package breadcrumb_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/breadcrumb"
)

func TestRender_emptyForSingleCrumb(t *testing.T) {
	if breadcrumb.Render([]string{"Home"}, theme.Default) != "" {
		t.Fatal("a single crumb (no drill-down) renders empty")
	}
	if breadcrumb.Render(nil, theme.Default) != "" {
		t.Fatal("nil renders empty")
	}
}

func TestRender_joinsCrumbs(t *testing.T) {
	got := breadcrumb.Render([]string{"Docs", "Note", "Backlink"}, theme.Default)
	for _, w := range []string{"Docs", "Note", "Backlink", "›"} {
		if !strings.Contains(got, w) {
			t.Fatalf("breadcrumb %q missing %q", got, w)
		}
	}
}
```

- [ ] **Step 2: Run — expect FAIL**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/ui/breadcrumb/ 2>&1 | head
```

- [ ] **Step 3: Implement** — `internal/tui/ui/breadcrumb/breadcrumb.go`
```go
// Package breadcrumb renders the drill-down path of the active nav-stack,
// e.g. "Docs › Note › Backlink". Returns "" at depth 1 (no drill-down).
package breadcrumb

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// sep is the whitelisted info glyph "›" used between crumbs.
const sep = "›"

// Render joins crumbs with "›"; the last (current) crumb is emphasized.
func Render(crumbs []string, p theme.Palette) string {
	if len(crumbs) <= 1 {
		return ""
	}
	parts := make([]string, len(crumbs))
	for i, c := range crumbs {
		if i == len(crumbs)-1 {
			parts[i] = theme.Strong(c, p)
		} else {
			parts[i] = theme.Dim(c, p)
		}
	}
	joined := strings.Join(parts, theme.Dim(" "+sep+" ", p))
	return lipgloss.NewStyle().Padding(0, theme.PadXS).Render(joined)
}
```

- [ ] **Step 4: Run — expect PASS**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/ui/breadcrumb/ -v 2>&1 | tail
```

- [ ] **Step 5: Commit**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add internal/tui/ui/breadcrumb && git commit -m "feat(m3b): breadcrumb component for drill-down depth"
```

---

## Task 5: `ui/overlay` — centered modal compositor

**Files:** Create `internal/tui/ui/overlay/overlay.go`, `internal/tui/ui/overlay/overlay_test.go`

Note: M3b centers the modal box on a blank field of the content size (the body is hidden while a modal is open). True backdrop-dim-over-body compositing is deferred (a later refinement); the centered box already gives the modal look the spec calls for.

- [ ] **Step 1: Write the failing test**
```go
package overlay_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/tui/ui/overlay"
)

func TestRender_centersBoxWithinField(t *testing.T) {
	box := "PALETTE"
	got := overlay.Render(box, 40, 10)
	if !strings.Contains(got, "PALETTE") {
		t.Fatalf("overlay must contain the box content, got %q", got)
	}
	if lipgloss.Width(got) != 40 {
		t.Fatalf("overlay width = %d, want 40", lipgloss.Width(got))
	}
	if lipgloss.Height(got) != 10 {
		t.Fatalf("overlay height = %d, want 10", lipgloss.Height(got))
	}
}

func TestRender_zeroDimsFallBackToBox(t *testing.T) {
	got := overlay.Render("X", 0, 0)
	if !strings.Contains(got, "X") {
		t.Fatalf("zero dims should still render the box, got %q", got)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/ui/overlay/ 2>&1 | head
```

- [ ] **Step 3: Implement** — `internal/tui/ui/overlay/overlay.go`
```go
// Package overlay composites a modal box centered on a field of the given
// size, for the shell's :-palette and ?-help layers.
package overlay

import "charm.land/lipgloss/v2"

// Render centers box within a width×height field. Non-positive dims fall back
// to the box's own measured size.
func Render(box string, width, height int) string {
	if width <= 0 {
		width = lipgloss.Width(box)
	}
	if height <= 0 {
		height = lipgloss.Height(box)
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}
```

- [ ] **Step 4: Run — expect PASS**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/ui/overlay/ -v 2>&1 | tail
```

- [ ] **Step 5: Commit**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add internal/tui/ui/overlay && git commit -m "feat(m3b): overlay component (centered modal compositor)"
```

---

## Task 6: shell `Route` contract + `Frame` + nav messages

**Files:** Create `internal/tui/shell/route.go`, `internal/tui/shell/route_test.go`

- [ ] **Step 1: Write the failing test** — defines `stubRoute` reused by later shell tests.
```go
package shell_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// stubRoute is a minimal Route used across the shell tests.
type stubRoute struct {
	title string
	hints []keyhint.Hint
	push  shell.Route // if set, Update on Enter pushes this route
}

func (s stubRoute) Title() string  { return s.title }
func (s stubRoute) Init() tea.Cmd  { return nil }
func (s stubRoute) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok && k.Code == tea.KeyEnter && s.push != nil {
		next := s.push
		return s, func() tea.Msg { return shell.PushRouteMsg{Route: next} }
	}
	return s, nil
}
func (s stubRoute) View(f shell.Frame) string     { return s.title }
func (s stubRoute) KeyHints() []keyhint.Hint       { return s.hints }

func TestRoute_satisfiedByStub(t *testing.T) {
	var r shell.Route = stubRoute{title: "Home"}
	if r.Title() != "Home" {
		t.Fatalf("got %q", r.Title())
	}
	if r.View(shell.Frame{Width: 10, Height: 5}) != "Home" {
		t.Fatal("view")
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`undefined: shell.Route`)
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/shell/ 2>&1 | head
```

- [ ] **Step 3: Implement** — `internal/tui/shell/route.go`
```go
// Package shell is the flow sidekick-shell: a top tabstrip, a :-command
// palette, and a per-tab nav-stack router. Each screen implements Route and
// is hosted by the Shell tea.Model (or, chrome-less, by RouteHost).
package shell

import (
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// Frame is the usable content area handed to a Route's View, after the shell
// chrome (header, tabstrip, breadcrumb, footer) has been subtracted. It also
// carries the active palette so routes never reach for a global.
type Frame struct {
	Width  int
	Height int
	Pal    theme.Palette
}

// Route is the contract every hosted screen, drill-down, and modal implements.
// Update returns the (possibly swapped) Route so the nav-stack can replace it
// without type assertions; to navigate, a Route returns a command emitting
// PushRouteMsg/PopRouteMsg which the Shell applies to the active stack.
type Route interface {
	Title() string
	Init() tea.Cmd
	Update(msg tea.Msg) (Route, tea.Cmd)
	View(f Frame) string
	KeyHints() []keyhint.Hint
}

// PushRouteMsg asks the Shell to push Route onto the active tab's nav-stack
// (a drill-down). Emit it as a tea.Cmd from a Route's Update.
type PushRouteMsg struct{ Route Route }

// PopRouteMsg asks the Shell to pop the active tab's nav-stack (a back).
type PopRouteMsg struct{}
```

- [ ] **Step 4: Run — expect PASS**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/shell/ -run TestRoute -v 2>&1 | tail
```

- [ ] **Step 5: Commit**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add internal/tui/shell/route.go internal/tui/shell/route_test.go && git commit -m "feat(m3b): shell Route contract + Frame + nav messages"
```

---

## Task 7: shell `NavStack` — per-tab stack

**Files:** Create `internal/tui/shell/navstack.go`, `internal/tui/shell/navstack_test.go`

- [ ] **Step 1: Write the failing test**
```go
package shell_test

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/shell"
)

func TestNavStack_pushPopFloor(t *testing.T) {
	ns := shell.NewNavStack(stubRoute{title: "Home"})
	if ns.Top().Title() != "Home" || ns.Len() != 1 {
		t.Fatal("fresh stack")
	}
	ns.Pop() // floor: no-op at depth 1
	if ns.Len() != 1 {
		t.Fatal("pop floor")
	}
	ns.Push(stubRoute{title: "Detail"})
	if ns.Top().Title() != "Detail" || ns.Len() != 2 {
		t.Fatal("push")
	}
	ns.Pop()
	if ns.Top().Title() != "Home" || ns.Len() != 1 {
		t.Fatal("pop")
	}
}

func TestNavStack_crumbs(t *testing.T) {
	ns := shell.NewNavStack(stubRoute{title: "Docs"})
	ns.Push(stubRoute{title: "Note"})
	if got := ns.Crumbs(); !reflect.DeepEqual(got, []string{"Docs", "Note"}) {
		t.Fatalf("crumbs = %v", got)
	}
}

func TestNavStack_updateTopReplaces(t *testing.T) {
	ns := shell.NewNavStack(stubRoute{title: "Home"})
	_ = ns.UpdateTop(tea.KeyPressMsg{Code: tea.KeyDown}) // returns tea.Cmd; must not panic
	if ns.Top().Title() != "Home" {
		t.Fatal("update top kept identity")
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`undefined: shell.NewNavStack`)
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/shell/ -run TestNavStack 2>&1 | head
```

- [ ] **Step 3: Implement** — `internal/tui/shell/navstack.go`
```go
package shell

import tea "charm.land/bubbletea/v2"

// NavStack is one tab's LIFO stack of Routes. Index 0 is the permanent root:
// Pop() is a no-op at depth 1.
type NavStack struct {
	stack []Route
}

// NewNavStack returns a stack whose only (permanent) entry is root.
func NewNavStack(root Route) *NavStack { return &NavStack{stack: []Route{root}} }

// Top returns the visible Route.
func (n *NavStack) Top() Route { return n.stack[len(n.stack)-1] }

// Len returns the stack depth.
func (n *NavStack) Len() int { return len(n.stack) }

// Push adds r as the new top (drill-down).
func (n *NavStack) Push(r Route) { n.stack = append(n.stack, r) }

// Pop removes the top Route; no-op at depth 1.
func (n *NavStack) Pop() {
	if len(n.stack) > 1 {
		n.stack = n.stack[:len(n.stack)-1]
	}
}

// ReplaceTop swaps the top Route without changing depth.
func (n *NavStack) ReplaceTop(r Route) { n.stack[len(n.stack)-1] = r }

// UpdateTop forwards msg to the top Route, stores the returned Route, and
// returns its command.
func (n *NavStack) UpdateTop(msg tea.Msg) tea.Cmd {
	next, cmd := n.Top().Update(msg)
	n.ReplaceTop(next)
	return cmd
}

// Crumbs returns the titles from root to top, for the breadcrumb.
func (n *NavStack) Crumbs() []string {
	out := make([]string, len(n.stack))
	for i, r := range n.stack {
		out[i] = r.Title()
	}
	return out
}
```

- [ ] **Step 4: Run — expect PASS**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/shell/ -run TestNavStack -v 2>&1 | tail
```

- [ ] **Step 5: Commit**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add internal/tui/shell/navstack.go internal/tui/shell/navstack_test.go && git commit -m "feat(m3b): shell per-tab nav-stack (push/pop/crumbs)"
```

---

## Task 8: shell `Palette` — `:`-command palette model

**Files:** Create `internal/tui/shell/palette.go`, `internal/tui/shell/palette_test.go`

Note: M3b filters with case-insensitive `strings.Contains` (no new dep). The spec mentions fuzzy matching; with only ~7 tab entries Contains is adequate. Swapping in `github.com/sahilm/fuzzy` is a deliberate later enhancement once the palette grows to actions.

- [ ] **Step 1: Write the failing test**
```go
package shell_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

func entries() []shell.PaletteEntry {
	return []shell.PaletteEntry{
		{Label: "Home", Action: func() tea.Msg { return nil }},
		{Label: "Worktime", Action: func() tea.Msg { return nil }},
		{Label: "Docs", Action: func() tea.Msg { return nil }},
	}
}

func TestPalette_filterContains(t *testing.T) {
	p := shell.NewPalette(entries()).SetQuery("work")
	f := p.Filtered()
	if len(f) != 1 || f[0].Label != "Worktime" {
		t.Fatalf("filtered = %v", f)
	}
}

func TestPalette_emptyShowsAll(t *testing.T) {
	if len(shell.NewPalette(entries()).Filtered()) != 3 {
		t.Fatal("empty query shows all")
	}
}

func TestPalette_typingFiltersAndEnterSelects(t *testing.T) {
	p := shell.NewPalette(entries())
	p, _ = p.Update(tea.KeyPressMsg{Text: "d"})
	p, _ = p.Update(tea.KeyPressMsg{Text: "o"})
	if len(p.Filtered()) != 1 || p.Filtered()[0].Label != "Docs" {
		t.Fatalf("after typing 'do': %v", p.Filtered())
	}
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should emit a command")
	}
	if _, ok := cmd().(shell.PaletteSelectedMsg); !ok {
		t.Fatal("enter should emit PaletteSelectedMsg")
	}
}

func TestPalette_escDismisses(t *testing.T) {
	p := shell.NewPalette(entries())
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc should emit a command")
	}
	if _, ok := cmd().(shell.PaletteDismissedMsg); !ok {
		t.Fatal("esc should emit PaletteDismissedMsg")
	}
}

func TestPalette_cursorClamp(t *testing.T) {
	p := shell.NewPalette(entries())
	for i := 0; i < 10; i++ {
		p, _ = p.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if p.Cursor() >= len(p.Filtered()) {
		t.Fatalf("cursor %d OOB for %d", p.Cursor(), len(p.Filtered()))
	}
}

func TestPalette_viewShowsQuery(t *testing.T) {
	p := shell.NewPalette(entries()).SetQuery("ho")
	if !strings.Contains(p.View(60, theme.Default), "ho") {
		t.Fatal("view should echo query")
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`undefined: shell.NewPalette`)
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/shell/ -run TestPalette 2>&1 | head
```

- [ ] **Step 3: Implement** — `internal/tui/shell/palette.go`
```go
package shell

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/picker"
)

// PaletteEntry is one selectable command/route in the palette.
type PaletteEntry struct {
	Label  string
	Action func() tea.Msg // returns the msg the Shell dispatches on select
}

// PaletteSelectedMsg is emitted when the user confirms an entry.
type PaletteSelectedMsg struct{ Entry PaletteEntry }

// PaletteDismissedMsg is emitted when the user presses Esc.
type PaletteDismissedMsg struct{}

// Palette is the :-command-palette model. Value type: the Shell holds it by
// value and reassigns on Update.
type Palette struct {
	entries  []PaletteEntry
	query    string
	cursor   int
	filtered []PaletteEntry
}

// NewPalette builds a palette over entries (empty query shows all).
func NewPalette(entries []PaletteEntry) Palette {
	return Palette{entries: entries}.refilter()
}

// SetQuery sets the filter text and resets the cursor.
func (p Palette) SetQuery(q string) Palette {
	p.query = q
	p.cursor = 0
	return p.refilter()
}

// Reset clears the query — call before opening.
func (p Palette) Reset() Palette { return p.SetQuery("") }

// Filtered returns the current matches.
func (p Palette) Filtered() []PaletteEntry { return p.filtered }

// Cursor returns the selected index.
func (p Palette) Cursor() int { return p.cursor }

// Update handles palette keys. On Enter emits PaletteSelectedMsg; on Esc
// emits PaletteDismissedMsg.
func (p Palette) Update(msg tea.Msg) (Palette, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil
	}
	switch {
	case k.Code == tea.KeyEsc:
		return p, func() tea.Msg { return PaletteDismissedMsg{} }
	case k.Code == tea.KeyEnter:
		if len(p.filtered) == 0 {
			return p, func() tea.Msg { return PaletteDismissedMsg{} }
		}
		entry := p.filtered[p.cursor]
		return p, func() tea.Msg { return PaletteSelectedMsg{Entry: entry} }
	case k.Code == tea.KeyDown:
		if p.cursor < len(p.filtered)-1 {
			p.cursor++
		}
	case k.Code == tea.KeyUp:
		if p.cursor > 0 {
			p.cursor--
		}
	case k.Code == tea.KeyBackspace:
		if p.query != "" {
			p.query = p.query[:len(p.query)-1]
			p = p.refilter()
		}
	case k.Text != "" && k.Mod&(tea.ModCtrl|tea.ModAlt) == 0:
		// printable (incl. Shift for capitals/symbols); ignore Ctrl/Alt combos
		p.query += k.Text
		p = p.refilter()
	}
	return p, nil
}

// View renders the palette inner content (query line + filtered rows). The
// Shell wraps this in a titlebox + overlay.
func (p Palette) View(width int, pal theme.Palette) string {
	var b strings.Builder
	b.WriteString(theme.Active(":", pal) + " " + theme.Body(p.query+"_", pal) + "\n")
	if len(p.filtered) == 0 {
		b.WriteString(theme.Dim("  keine Treffer", pal))
		return b.String()
	}
	for i, e := range p.filtered {
		b.WriteString(picker.Row(i == p.cursor, e.Label, "", width, pal))
		if i < len(p.filtered)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (p Palette) refilter() Palette {
	if p.query == "" {
		p.filtered = p.entries
	} else {
		q := strings.ToLower(p.query)
		out := make([]PaletteEntry, 0, len(p.entries))
		for _, e := range p.entries {
			if strings.Contains(strings.ToLower(e.Label), q) {
				out = append(out, e)
			}
		}
		p.filtered = out
	}
	if p.cursor >= len(p.filtered) {
		if len(p.filtered) > 0 {
			p.cursor = len(p.filtered) - 1
		} else {
			p.cursor = 0
		}
	}
	return p
}
```

- [ ] **Step 4: Run — expect PASS**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/shell/ -run TestPalette -v 2>&1 | tail
```

- [ ] **Step 5: Commit**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add internal/tui/shell/palette.go internal/tui/shell/palette_test.go && git commit -m "feat(m3b): shell :-command palette model"
```

---

## Task 9: placeholder routes — `HomeRoute` (drill-down trigger) + `AboutRoute`

**Files:** Create `internal/tui/shell/home.go`, `internal/tui/shell/about.go`, `internal/tui/shell/home_test.go`

`HomeRoute` is Tab 0's root; pressing Enter pushes `AboutRoute` (so drill-down/back is demonstrable in `flow ui`). `AboutRoute` is a static leaf; `esc` is handled by the Shell (pops the stack). Both are stateless in M3b — M3c replaces Home with a live dashboard.

- [ ] **Step 1: Write the failing test** — `internal/tui/shell/home_test.go`
```go
package shell_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/shell"
)

func TestHomeRoute_basics(t *testing.T) {
	r := shell.NewHomeRoute("alice")
	if r.Title() != "Home" {
		t.Fatalf("title %q", r.Title())
	}
	if !strings.Contains(r.View(shell.Frame{Width: 80, Height: 20}), "alice") {
		t.Fatal("home view should contain user")
	}
	if len(r.KeyHints()) == 0 {
		t.Fatal("home should expose key hints")
	}
}

func TestHomeRoute_enterPushesAbout(t *testing.T) {
	r := shell.NewHomeRoute("alice")
	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should emit a command")
	}
	msg, ok := cmd().(shell.PushRouteMsg)
	if !ok || msg.Route.Title() != "About" {
		t.Fatalf("enter should push AboutRoute, got %#v", cmd())
	}
}
```

- [ ] **Step 2: Run — expect FAIL**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/shell/ -run TestHomeRoute 2>&1 | head
```

- [ ] **Step 3a: Implement** — `internal/tui/shell/home.go`
```go
package shell

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// HomeRoute is the placeholder Home screen (M3b). Enter drills into AboutRoute
// to exercise the nav-stack. M3c replaces this with a live dashboard.
type HomeRoute struct{ user string }

// NewHomeRoute builds the Home route for user.
func NewHomeRoute(user string) HomeRoute { return HomeRoute{user: user} }

func (h HomeRoute) Title() string { return "Home" }
func (h HomeRoute) Init() tea.Cmd { return nil }

func (h HomeRoute) Update(msg tea.Msg) (Route, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok && k.Code == tea.KeyEnter {
		return h, func() tea.Msg { return PushRouteMsg{Route: NewAboutRoute()} }
	}
	return h, nil
}

func (h HomeRoute) View(f Frame) string {
	greeting := theme.Heading(fmt.Sprintf("Willkommen, %s", h.user), f.Pal)
	hint := theme.Body("Das Dashboard kommt in M3c.", f.Pal)
	drill := theme.Dim("Enter -> Details (Drill-down-Demo)", f.Pal)
	return fmt.Sprintf("\n  %s\n\n  %s\n  %s\n", greeting, hint, drill)
}

func (h HomeRoute) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "enter", Desc: "Details"},
		{Key: "tab", Desc: "Tab"},
		{Key: ":", Desc: "Palette"},
		{Key: "?", Desc: "Hilfe"},
	}
}
```

- [ ] **Step 3b: Implement** — `internal/tui/shell/about.go`
```go
package shell

import (
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// AboutRoute is a static drill-down leaf used to demonstrate push/pop in M3b.
type AboutRoute struct{}

// NewAboutRoute builds the About leaf.
func NewAboutRoute() AboutRoute { return AboutRoute{} }

func (AboutRoute) Title() string                       { return "About" }
func (AboutRoute) Init() tea.Cmd                        { return nil }
func (AboutRoute) Update(tea.Msg) (Route, tea.Cmd)      { return AboutRoute{}, nil }

func (AboutRoute) View(f Frame) string {
	return "\n  " + theme.Strong("flow sidekick-shell", f.Pal) +
		"\n  " + theme.Body("Eine Programm-Shell für alle Screens.", f.Pal) +
		"\n\n  " + theme.Dim("esc -> zurück", f.Pal) + "\n"
}

func (AboutRoute) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{{Key: "esc", Desc: "zurück"}, {Key: "tab", Desc: "Tab"}}
}
```

- [ ] **Step 4: Run — expect PASS**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/shell/ -run TestHomeRoute -v 2>&1 | tail
```

- [ ] **Step 5: Commit**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add internal/tui/shell/home.go internal/tui/shell/about.go internal/tui/shell/home_test.go && git commit -m "feat(m3b): Home + About placeholder routes (drill-down demo)"
```

---

## Task 10: `Shell` root model — chrome + routing + SSE

**Files:** Create `internal/tui/shell/shell.go`, `internal/tui/shell/shell_test.go`

The `Shell` owns `[]*NavStack` (one per tab), the active tab index, the palette, help/palette open flags, terminal size, the visual palette, and the apiclient (for SSE). Chrome = header (1) + tabstrip (1) + breadcrumb (0/1) + footer (1). Keys: `Tab`/`Shift+Tab`/digits switch tabs; `:` opens palette; `?` toggles help; `esc` closes a modal else pops the active stack; `q`/`Ctrl+C` quits; everything else forwards to the active route. `PushRouteMsg`/`PopRouteMsg` mutate the active stack. SSE events fan out to every tab's top route.

- [ ] **Step 1: Write the failing test**
```go
package shell_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

func newShell() shell.Shell {
	return shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{
		shell.NewHomeRoute("alice"),
		stubRoute{title: "Worktime", push: stubRoute{title: "Detail"}},
	})
}

func TestShell_windowSize(t *testing.T) {
	next, _ := newShell().Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	s := next.(shell.Shell)
	if s.Width() != 120 || s.Height() != 40 {
		t.Fatalf("size = %dx%d", s.Width(), s.Height())
	}
}

func TestShell_tabSwitchByDigitAndTab(t *testing.T) {
	next, _ := newShell().Update(tea.KeyPressMsg{Text: "2"})
	if next.(shell.Shell).ActiveTab() != 1 {
		t.Fatal("digit 2 -> tab 1")
	}
	next2, _ := newShell().Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if next2.(shell.Shell).ActiveTab() != 1 {
		t.Fatal("tab -> next")
	}
}

func TestShell_paletteOpenClose(t *testing.T) {
	next, _ := newShell().Update(tea.KeyPressMsg{Text: ":"})
	s := next.(shell.Shell)
	if !s.PaletteOpen() {
		t.Fatal("':' opens palette")
	}
	// Esc inside palette emits PaletteDismissedMsg; feed it back.
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	next2, _ := s.Update(cmd())
	if next2.(shell.Shell).PaletteOpen() {
		t.Fatal("dismiss closes palette")
	}
}

func TestShell_drillDownAndBack(t *testing.T) {
	// Switch to tab 1 (stub that pushes "Detail" on Enter).
	s, _ := newShell().Update(tea.KeyPressMsg{Text: "2"})
	sh := s.(shell.Shell)
	// Enter -> route emits PushRouteMsg; feed the produced msg back.
	_, cmd := sh.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should produce a push command")
	}
	pushed, _ := sh.Update(cmd())
	sh = pushed.(shell.Shell)
	if sh.ActiveDepth() != 2 {
		t.Fatalf("after push depth = %d want 2", sh.ActiveDepth())
	}
	// Esc pops back.
	back, _ := sh.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if back.(shell.Shell).ActiveDepth() != 1 {
		t.Fatal("esc should pop back to depth 1")
	}
}

func TestShell_quit(t *testing.T) {
	_, cmd := newShell().Update(tea.KeyPressMsg{Text: "q"})
	if cmd == nil {
		t.Fatal("q should quit")
	}
}

func TestShell_viewNoPanic(t *testing.T) {
	s, _ := newShell().Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("View panicked: %v", r)
		}
	}()
	_ = s.(shell.Shell).View()
}
```

- [ ] **Step 2: Run — expect FAIL** (`undefined: shell.New`)
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/shell/ -run TestShell 2>&1 | head
```

- [ ] **Step 3: Implement** — `internal/tui/shell/shell.go`
```go
package shell

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/breadcrumb"
	"github.com/serverkraken/flow/internal/tui/ui/header"
	"github.com/serverkraken/flow/internal/tui/ui/help"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
	"github.com/serverkraken/flow/internal/tui/ui/overlay"
	"github.com/serverkraken/flow/internal/tui/ui/tabstrip"
	"github.com/serverkraken/flow/internal/tui/ui/titlebox"
)

// Shell is the root tea.Model for `flow ui`.
type Shell struct {
	tabs      []*NavStack
	activeTab int

	palette     Palette
	paletteOpen bool
	helpOpen    bool

	width, height int
	user          string
	pal           theme.Palette

	client *apiclient.Client
	events <-chan apiclient.ClientEvent
}

type shellEventsReadyMsg struct{ ch <-chan apiclient.ClientEvent }
type shellEventMsg struct{ ev apiclient.ClientEvent }
type shellErrMsg struct{ err error }

// tabSwitchMsg requests a tab change (emitted by palette entries).
type tabSwitchMsg int

// New creates a Shell with a single Home tab. client may be nil (tests).
// pal is the visual palette (theme.Load()).
func New(client *apiclient.Client, user string, pal theme.Palette) Shell {
	s := Shell{user: user, pal: pal, client: client}
	return s.WithTabs([]Route{NewHomeRoute(user)})
}

// WithTabs (re)builds the tab set; each Route becomes a stack root and gets a
// palette entry. Used in New, tests, and future M3c wiring.
func (s Shell) WithTabs(routes []Route) Shell {
	s.tabs = make([]*NavStack, len(routes))
	entries := make([]PaletteEntry, len(routes))
	for i, r := range routes {
		s.tabs[i] = NewNavStack(r)
		idx := i
		entries[i] = PaletteEntry{Label: r.Title(), Action: func() tea.Msg { return tabSwitchMsg(idx) }}
	}
	s.palette = NewPalette(entries)
	if s.activeTab >= len(s.tabs) {
		s.activeTab = 0
	}
	return s
}

// Accessors (used by tests + cmd).
func (s Shell) Width() int       { return s.width }
func (s Shell) Height() int      { return s.height }
func (s Shell) ActiveTab() int   { return s.activeTab }
func (s Shell) PaletteOpen() bool { return s.paletteOpen }
func (s Shell) ActiveDepth() int { return s.tabs[s.activeTab].Len() }

// Init subscribes to SSE if a client is present.
func (s Shell) Init() tea.Cmd {
	if s.client == nil {
		return nil
	}
	cl := s.client
	return func() tea.Msg {
		ch, err := cl.Events(context.Background())
		if err != nil {
			return shellErrMsg{err}
		}
		return shellEventsReadyMsg{ch}
	}
}

// Update is the central dispatcher.
func (s Shell) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		s.width, s.height = msg.Width, msg.Height
		var cmds []tea.Cmd
		for _, ns := range s.tabs {
			if c := ns.UpdateTop(msg); c != nil {
				cmds = append(cmds, c)
			}
		}
		return s, tea.Batch(cmds...)

	case shellEventsReadyMsg:
		s.events = msg.ch
		return s, waitForShellEvent(msg.ch)
	case shellEventMsg:
		var cmds []tea.Cmd
		for _, ns := range s.tabs { // broadcast to all tabs
			if c := ns.UpdateTop(msg); c != nil {
				cmds = append(cmds, c)
			}
		}
		cmds = append(cmds, waitForShellEvent(s.events))
		return s, tea.Batch(cmds...)
	case shellErrMsg:
		return s, nil // swallow for M3b; M3c can toast

	case PushRouteMsg:
		s.tabs[s.activeTab].Push(msg.Route)
		return s, msg.Route.Init()
	case PopRouteMsg:
		s.tabs[s.activeTab].Pop()
		return s, nil

	case PaletteSelectedMsg:
		s.paletteOpen = false
		return s.Update(msg.Entry.Action())
	case PaletteDismissedMsg:
		s.paletteOpen = false
		return s, nil
	case tabSwitchMsg:
		if i := int(msg); i >= 0 && i < len(s.tabs) {
			s.activeTab = i
		}
		return s, nil

	case tea.KeyPressMsg:
		return s.handleKey(msg)
	}

	// default: forward to active route
	return s, s.tabs[s.activeTab].UpdateTop(msg)
}

func (s Shell) handleKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if s.paletteOpen {
		var cmd tea.Cmd
		s.palette, cmd = s.palette.Update(k)
		return s, cmd
	}
	switch {
	case k.Code == tea.KeyEsc:
		if s.helpOpen {
			s.helpOpen = false
			return s, nil
		}
		s.tabs[s.activeTab].Pop()
		return s, nil
	case k.Text == "q" || (k.Code == 'c' && k.Mod == tea.ModCtrl):
		return s, tea.Quit
	case k.Text == ":":
		s.paletteOpen = true
		s.palette = s.palette.Reset()
		return s, nil
	case k.Text == "?":
		s.helpOpen = !s.helpOpen
		return s, nil
	case k.Code == tea.KeyTab && k.Mod == tea.ModShift:
		s.activeTab = (s.activeTab - 1 + len(s.tabs)) % len(s.tabs)
		return s, nil
	case k.Code == tea.KeyTab:
		s.activeTab = (s.activeTab + 1) % len(s.tabs)
		return s, nil
	case len(k.Text) == 1 && k.Text[0] >= '1' && k.Text[0] <= '9':
		if i := int(k.Text[0] - '1'); i < len(s.tabs) {
			s.activeTab = i
		}
		return s, nil
	default:
		return s, s.tabs[s.activeTab].UpdateTop(k)
	}
}

// View renders header + tabstrip + breadcrumb + body + footer.
func (s Shell) View() tea.View {
	titles := make([]string, len(s.tabs))
	for i, ns := range s.tabs {
		titles[i] = ns.Top().Title()
	}
	head := header.Render("flow", s.user, max(s.width, 1), s.pal)
	tabs := tabstrip.Render(titles, s.activeTab, max(s.width, 1), s.pal)
	crumbs := breadcrumb.Render(s.tabs[s.activeTab].Crumbs(), s.pal)

	chrome := 2 // header + tabstrip
	if crumbs != "" {
		chrome++
	}
	chrome++ // footer
	contentH := s.height - chrome
	if contentH < 0 {
		contentH = 0
	}

	var body, footer string
	switch {
	case s.helpOpen:
		body = overlay.Render(s.renderHelp(), s.width, contentH)
		footer = keyhint.Render([]keyhint.Hint{{Key: "esc", Desc: "schließen"}}, s.pal)
	case s.paletteOpen:
		modalW := min(theme.DefaultBox, max(s.width-4, 10))
		modal := titlebox.Render("Palette", s.palette.View(modalW-2, s.pal), modalW, s.pal)
		body = overlay.Render(modal, s.width, contentH)
		footer = keyhint.Render([]keyhint.Hint{{Key: "enter", Desc: "wählen"}, {Key: "esc", Desc: "schließen"}}, s.pal)
	default:
		top := s.tabs[s.activeTab].Top()
		body = top.View(Frame{Width: s.width, Height: contentH, Pal: s.pal})
		footer = keyhint.Render(top.KeyHints(), s.pal)
	}

	parts := []string{head, tabs}
	if crumbs != "" {
		parts = append(parts, crumbs)
	}
	parts = append(parts, body, footer)
	v := tea.NewView(strings.Join(parts, "\n"))
	v.AltScreen = true
	return v
}

func (s Shell) renderHelp() string {
	top := s.tabs[s.activeTab].Top()
	keys := make([][2]string, 0, len(top.KeyHints())+5)
	for _, h := range top.KeyHints() {
		keys = append(keys, [2]string{h.Key, h.Desc})
	}
	sections := []help.Section{
		{Title: "Aktueller Screen", Keys: keys},
		{Title: "Global", Keys: [][2]string{
			{"Tab / Shift+Tab", "Tab wechseln"},
			{"1-9", "Tab direkt"},
			{":", "Palette"},
			{"esc", "zurück / schließen"},
			{"q", "Beenden"},
		}},
	}
	return help.Render("Tastatur", sections, theme.KeyHintWidth, theme.DefaultBox, s.pal)
}

func waitForShellEvent(ch <-chan apiclient.ClientEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return shellEventMsg{ev}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
```

- [ ] **Step 4: Run — expect PASS** (whole shell package, all tasks so far)
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/shell/ -v 2>&1 | tail -30
```

- [ ] **Step 5: Commit**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add internal/tui/shell/shell.go internal/tui/shell/shell_test.go && git commit -m "feat(m3b): Shell root model — chrome, tab/palette/drill-down routing, SSE fan-out"
```

---

## Task 11: `RouteHost` — chrome-less single-Route standalone host

**Files:** Create `internal/tui/shell/host.go`, `internal/tui/shell/host_test.go`

A thin `tea.Model` that runs ONE Route with just a footer (no tabstrip/palette), per the spec's "Standalone-Host (1 Route, kein Chrome)". M3c can host real leaf screens with it. `q`/`Ctrl+C`/`esc` quit.

- [ ] **Step 1: Write the failing test**
```go
package shell_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

func TestRouteHost_viewAndQuit(t *testing.T) {
	h := shell.NewRouteHost(stubRoute{title: "Solo"}, theme.Default)
	m, _ := h.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_ = m.(shell.RouteHost).View() // must not panic
	_, cmd := h.Update(tea.KeyPressMsg{Text: "q"})
	if cmd == nil {
		t.Fatal("q should quit the host")
	}
}
```

- [ ] **Step 2: Run — expect FAIL**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/shell/ -run TestRouteHost 2>&1 | head
```

- [ ] **Step 3: Implement** — `internal/tui/shell/host.go`
```go
package shell

import (
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// RouteHost runs a single Route chrome-less (footer only), for the standalone
// `flow <screen>` launch mode. Drill-down (PushRouteMsg) is ignored here — a
// standalone host shows one leaf screen.
type RouteHost struct {
	route         Route
	pal           theme.Palette
	width, height int
}

// NewRouteHost wraps route as a standalone program model.
func NewRouteHost(route Route, pal theme.Palette) RouteHost {
	return RouteHost{route: route, pal: pal}
}

func (h RouteHost) Init() tea.Cmd { return h.route.Init() }

func (h RouteHost) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.width, h.height = msg.Width, msg.Height
		var cmd tea.Cmd
		h.route, cmd = h.route.Update(msg)
		return h, cmd
	case tea.KeyPressMsg:
		if msg.Text == "q" || msg.Code == tea.KeyEsc || (msg.Code == 'c' && msg.Mod == tea.ModCtrl) {
			return h, tea.Quit
		}
	}
	var cmd tea.Cmd
	h.route, cmd = h.route.Update(msg)
	return h, cmd
}

func (h RouteHost) View() tea.View {
	contentH := h.height - 1
	if contentH < 0 {
		contentH = 0
	}
	body := h.route.View(Frame{Width: h.width, Height: contentH, Pal: h.pal})
	footer := keyhint.Render(h.route.KeyHints(), h.pal)
	v := tea.NewView(body + "\n" + footer)
	v.AltScreen = true
	return v
}
```

- [ ] **Step 4: Run — expect PASS**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/shell/ -run TestRouteHost -v 2>&1 | tail
```

- [ ] **Step 5: Commit**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add internal/tui/shell/host.go internal/tui/shell/host_test.go && git commit -m "feat(m3b): RouteHost chrome-less single-route standalone host"
```

---

## Task 12: `flow ui` command + root no-args dispatch

**Files:** Create `cmd/flow/ui.go`; Modify `cmd/flow/main.go`

- [ ] **Step 1: Confirm baseline builds**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go build ./cmd/flow/ 2>&1 && echo OK
```
Expected: `OK`.

- [ ] **Step 2: Create** `cmd/flow/ui.go`
```go
package main

import (
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/spf13/cobra"
)

func uiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ui",
		Short: "Sidekick-Shell (TUI) — alle Screens in einem Programm",
		RunE:  runUI,
	}
}

func runUI(cmd *cobra.Command, _ []string) error {
	client, err := clientFromStore(cmd.Context())
	if err != nil {
		return err
	}
	// slog/stderr must never corrupt the TUI: send logs to a file.
	logf, err := os.OpenFile(filepath.Join(os.TempDir(), "flow-tui.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err == nil {
		defer func() { _ = logf.Close() }()
		os.Stderr = logf
	}
	m := shell.New(client, os.Getenv("USER"), theme.Load())
	_, err = tea.NewProgram(m, tea.WithContext(cmd.Context())).Run()
	return err
}
```

- [ ] **Step 3: Modify** `cmd/flow/main.go` — add `RunE: runUI` to root and register `uiCmd()`. Locate `rootCmd()` and change:
```go
func rootCmd() *cobra.Command {
	root := &cobra.Command{Use: "flow", Short: "flow client"}
	root.AddCommand(whoamiCmd())
	root.AddCommand(worktimeCmd())
	...
}
```
to:
```go
func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "flow",
		Short: "flow client",
		RunE:  runUI, // bare `flow` launches the sidekick shell
	}
	root.AddCommand(whoamiCmd())
	root.AddCommand(worktimeCmd())
	root.AddCommand(uiCmd())
	...
}
```
(Add only the `RunE: runUI` line and the `root.AddCommand(uiCmd())` line; leave every other `AddCommand` exactly as-is.)

- [ ] **Step 4: Build**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go build ./cmd/flow/ 2>&1 && echo OK
```
Expected: `OK`.

- [ ] **Step 5: Commit**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add cmd/flow/ui.go cmd/flow/main.go && git commit -m "feat(m3b): flow ui command + root no-args dispatch to sidekick shell"
```

---

## Task 13: Full CI + done-gate smoke

**Files:** none (verification; commit only if lint fixups needed).

- [ ] **Step 1: Build everything + vet the new code**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go build ./... && go vet ./internal/tui/... ./cmd/flow/ && echo OK
```
Expected: `OK`.

- [ ] **Step 2: `make ci`**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && make ci 2>&1 | tail -25
```
Expected: lint + verify-generate + cover (≥ 80 %) + build green. The shell + ui packages bring their own tests; coverage should hold. Fix any lint nit in the new code minimally and re-run.

- [ ] **Step 3: CLI smoke — `flow ui` exists, standalone screens intact**
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go build -o /tmp/flow-m3b ./cmd/flow/ \
  && /tmp/flow-m3b ui --help 2>&1 | head -3 \
  && /tmp/flow-m3b worktime --help 2>&1 | head -1 \
  && /tmp/flow-m3b docs --help 2>&1 | head -1 \
  && /tmp/flow-m3b --help 2>&1 | rg -A2 'Available Commands' | head
```
Expected: `ui --help` shows the Sidekick-Shell short text; `worktime`/`docs` still show their own usage; root help lists `ui` among the commands.

- [ ] **Step 4: Live done-gate (manual, against the dev stack)** — document the result in the completion note:
```bash
# Terminal A: make dev-up && make dev-run   (Postgres + Dex + server)
# Terminal B: FLOW_DEV=1 ./flow login  (device-flow), then:  FLOW_DEV=1 ./flow ui
#   verify: Tab / Shift+Tab / digit keys switch tabs; ':' opens the palette and a
#   selection jumps tabs; on Home press Enter -> About drill-down, breadcrumb shows
#   "Home › About", esc pops back; '?' shows the help overlay; 'q' quits.
```
Expected: all interactions work; the standalone `flow worktime` still launches. (Real screens land in M3c/M3d; M3b's gate is shell navigation + router.)

- [ ] **Step 5: Commit any lint fixups** (skip if clean)
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add -A && git commit -m "chore(m3b): lint fixups for sidekick-shell"
```

---

## Self-review

### Spec coverage (M3 spec slice M3b row + architecture)
| Spec item | Task |
|---|---|
| Router / Nav-Stack (per-tab, push/pop, back-history) | 7 (NavStack) + 10 (Shell push/pop/esc) |
| `route.go` (`Route` interface + `Frame`) | 6 |
| `tabstrip` (active/inactive + overflow) | 2 |
| `:`-Palette (filter + dispatch) | 8 (model) + 10 (open/close/select) |
| globaler Status/Footer (`KeyHints`) | 1 (keyhint) + 10 (footer render) |
| `header` (App-Titel · User) | 3 |
| `breadcrumb` (drill-down depth) | 4 + 10 |
| `overlay` (centered box for modals) | 5 + 10 (palette/help via overlay) |
| `?`-help overlay | 10 (renderHelp + help.Render) |
| `flow ui` entry + root no-args | 12 |
| Standalone-Host (1 Route, kein Chrome) | 11 (RouteHost) + existing `flow worktime`/`docs` unaffected (13) |
| Hostet eine Placeholder-/Home-Route | 9 (Home + About) |
| SSE `ChangedMsg` broadcast to all stacks | 10 (`shellEventMsg` fan-out loop) |
| Done-gate: tabbt + Palette + Drill-down/Back; Router-Tests grün | 10 tests (`TestShell_*`) + 13 live gate |

### Placeholder scan
No TBD/TODO/"add error handling"/"similar to Task N" — every step has complete code. The only intentional deferrals are documented inline: palette uses Contains not fuzzy (Task 8 Note), and overlay centers without backdrop-dim (Task 5 Note).

### Type consistency
- `Route` (Task 6): `Title()`, `Init() tea.Cmd`, `Update(tea.Msg) (Route, tea.Cmd)`, `View(Frame) string`, `KeyHints() []keyhint.Hint` — implemented identically by `stubRoute` (6), `HomeRoute`/`AboutRoute` (9); consumed by `NavStack` (7), `Shell` (10), `RouteHost` (11).
- `Frame{Width, Height int; Pal theme.Palette}` (6) — constructed in Shell.View (10) and RouteHost.View (11), consumed in every route's `View`.
- `keyhint.Hint{Key, Desc string}` (1) — returned by `KeyHints()` (9), rendered by `keyhint.Render` (1) in Shell/RouteHost footers, converted to `[2]string` for `help.Section` (10).
- `PushRouteMsg{Route}` / `PopRouteMsg{}` (6) — emitted by HomeRoute (9) + stubRoute (6), handled in Shell.Update (10).
- `Palette` / `PaletteEntry` / `PaletteSelectedMsg` / `PaletteDismissedMsg` (8) — embedded + handled in Shell (10).
- `NavStack` methods `Top/Len/Push/Pop/ReplaceTop/UpdateTop/Crumbs` (7) — used in Shell (10).
- `shell.New(client *apiclient.Client, user string, pal theme.Palette) Shell` + `WithTabs([]Route) Shell` (10) — called in cmd `runUI` (12) and tests.
- bubbletea v2 key idiom (`tea.KeyPressMsg`, `k.Text`/`k.Code`/`k.Mod`, `View() tea.View` + `tea.NewView`) — used consistently in every Update/View; matches `internal/tui/worktime.go`.

### Notes for the executor
- [[feedback_subagent_git_commits_isolated]]: verify HEAD advances after each subagent commit; recover orphans via reflog.
- [[project_flow_rebuild_m3a]] shipped the consumed `theme` + `ui` packages; [[project_flow_rebuild_m3_planned]] is the parent spec. Next slices: M3c (Home dashboard + Worktime Today on apiclient+SSE), M3d (remaining screens + markdown viewer).
- [[feedback_long_lived_integration_branch]]: commit on `rebuild`; do not merge to main per milestone.
```
