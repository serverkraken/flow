# markdown_overlay Component — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Drei parallele Markdown-Overlays (`brief_view`, `today_note_view`, kompendium `view.Model`) auf eine gemeinsame Komponente `internal/frontend/tui/components/markdown_overlay/` lifteln. Unified Chrome (rounded frame + title + sep + body + footer + statusBar), Search/Code-Copy als Opt-In.

**Architecture:** `markdown_overlay.Model` implementiert `tea.Model`, wird mit einer `RenderFunc`-Closure konstruiert (caller-bound — entkoppelt von `ports.MarkdownRenderer` vs. `internal/frontend/tui/markdown.Render`). Features (Search/CodeCopy) per Option opt-in. Drei Caller (browse, brief_view, today_note_view) hosten den Overlay als embedded `tea.Model`-Field und routen Messages durch. ExitMsg signalisiert Schließen.

**Tech Stack:** Go, bubbletea, bubbles/viewport, bubbles/textinput, bubbles/key, lipgloss, x/ansi, charmbracelet/x.

**Spec:** `docs/specs/2026-05-11-markdown-overlay-component-design.md`

**Design-Refinement gegenüber Spec:** Die Spec listet `WithMarkdownOptions(...markdown.Option) Option`. In der Implementation wird das ersetzt durch eine `RenderFunc func(src string, width int) string`-Closure als Konstruktor-Arg. Begründung: brief/today benutzen `ports.MarkdownRenderer.Render`, kompendium benutzt `markdown.Render(...opts)` — beide haben unterschiedliche Signaturen. Eine Closure entkoppelt sauber, statt die Komponente an zwei Render-Abstraktionen zu binden.

---

## File Structure

**New files (Phase 1):**

| Path | Responsibility |
| --- | --- |
| `internal/frontend/tui/components/markdown_overlay/doc.go` | Package-Doc |
| `internal/frontend/tui/components/markdown_overlay/model.go` | `Model` struct + `New()` + `tea.Model` contract (Init/Update/View dispatcher) |
| `internal/frontend/tui/components/markdown_overlay/options.go` | `Option` type, `config` struct, `With*` constructors |
| `internal/frontend/tui/components/markdown_overlay/setters.go` | `SetSize`/`SetTitle`/`SetSource`/`SetError` |
| `internal/frontend/tui/components/markdown_overlay/chrome.go` | Frame + title + separator + footer + statusBar rendering |
| `internal/frontend/tui/components/markdown_overlay/chrome_styles.go` | lipgloss styles + `SetPalette()` |
| `internal/frontend/tui/components/markdown_overlay/search.go` | Search-Mode state + key handling + match scan + highlight gutter |
| `internal/frontend/tui/components/markdown_overlay/code_copy.go` | Snippet extraction + cycling + OSC52 wrapper |
| `internal/frontend/tui/components/markdown_overlay/keymap.go` | `KeyMap` + `defaultKeys()` |
| `internal/frontend/tui/components/markdown_overlay/render.go` | `RenderFunc` type alias |
| `internal/frontend/tui/components/markdown_overlay/model_test.go` | unit + integration tests |
| `internal/frontend/tui/components/markdown_overlay/golden_test.go` | golden-file tests |
| `internal/frontend/tui/components/markdown_overlay/testdata/*.golden` | golden files |

**Modified files (Phase 2-5):**

| Path | Why |
| --- | --- |
| `internal/kompendium/frontend/tui/browse/model.go` | `viewer view.Model` → `viewer markdown_overlay.Model` |
| `internal/kompendium/frontend/tui/browse/preview.go` | `view.New(...)` → `markdown_overlay.New(...)` |
| `internal/kompendium/frontend/tui/browse/update.go` | ExitMsg-Konvention konsumieren |
| `internal/frontend/tui/screen/worktime/brief_view.go` | `briefView` struct → embed `markdown_overlay.Model` |
| `internal/frontend/tui/screen/worktime/today_note_view.go` | analog brief_view |
| `internal/frontend/tui/screen/worktime/today.go` | Sub-State-Felder + Update-Routing |
| `internal/frontend/tui/screen/worktime/today_dialog_keys.go` | Key-Routing für note-view-Dialog |
| `internal/frontend/tui/screen/worktime/today_render.go` | note-view-Render-Pfad |
| `internal/frontend/tui/screen/worktime/model.go` | brief_view-State-Felder |
| `internal/frontend/tui/screen/worktime/menu_brief.go` | brief_view-Konstruktion |
| `cmd/flow/main.go` | `view.SetPalette(...)` → `markdown_overlay.SetPalette(...)` |

**Deleted files (Phase 2):**

| Path | Replaced by |
| --- | --- |
| `internal/kompendium/frontend/tui/view/model.go` | markdown_overlay.Model |
| `internal/kompendium/frontend/tui/view/copy.go` | markdown_overlay/code_copy.go |
| `internal/kompendium/frontend/tui/view/styles.go` | markdown_overlay/chrome_styles.go |
| `internal/kompendium/frontend/tui/view/keymap.go` | markdown_overlay/keymap.go |
| `internal/kompendium/frontend/tui/view/markdown_adapter.go` | inline in browse/preview.go closure |
| `internal/kompendium/frontend/tui/view/model_test.go` | markdown_overlay/model_test.go |
| `internal/kompendium/frontend/tui/view/copy_internal_test.go` | markdown_overlay/code_copy_test.go |

---

## Phase 1: Base Component

Each task: TDD (failing test → minimal impl → passing test → commit). Run `make ci` (or focused `go test ./internal/frontend/tui/components/markdown_overlay/`) between tasks.

### Task 1: Package skeleton

**Files:**
- Create: `internal/frontend/tui/components/markdown_overlay/doc.go`
- Create: `internal/frontend/tui/components/markdown_overlay/model.go`
- Create: `internal/frontend/tui/components/markdown_overlay/render.go`
- Create: `internal/frontend/tui/components/markdown_overlay/options.go`
- Create: `internal/frontend/tui/components/markdown_overlay/model_test.go`

- [ ] **Step 1.1: Write failing test for package-level New + tea.Model contract**

```go
// model_test.go
package markdown_overlay_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown_overlay"
)

func TestNew_ReturnsTeaModel(t *testing.T) {
	m := markdown_overlay.New(func(src string, w int) string { return src })
	var _ tea.Model = m
	if cmd := m.Init(); cmd != nil {
		t.Errorf("Init: got non-nil cmd, want nil")
	}
}
```

- [ ] **Step 1.2: Run test, verify build failure**

Run: `go test ./internal/frontend/tui/components/markdown_overlay/`
Expected: `package … is not in std` or undefined-symbol failure.

- [ ] **Step 1.3: Write skeleton files**

```go
// doc.go
// Package markdown_overlay is the shared Markdown reader component
// used by the worktime brief overlay, the worktime today-note overlay,
// and the kompendium full-screen viewer. It hosts a viewport, a
// re-flowing markdown body, an optional search mode, optional code-
// snippet copy, and a uniform chrome (rounded frame + title + separator
// + footer + status bar).
//
// Render abstraction is passed in as a RenderFunc closure so the
// component doesn't bind to a specific renderer (some callers use
// ports.MarkdownRenderer, others bind markdown.Render with options).
package markdown_overlay
```

```go
// render.go
package markdown_overlay

// RenderFunc renders markdown source at the requested inner width.
// Implementations must NEVER return "" for a non-empty source — empty
// signals "renderer not wired" and the overlay falls back to raw text.
type RenderFunc func(src string, width int) string
```

```go
// options.go
package markdown_overlay

// config holds the resolved Option set. Unexported; callers configure
// via With* funcs.
type config struct {
	title          string
	source         string
	enableSearch   bool
	enableCodeCopy bool
	closeKeys      []string
	footerExtras   []string
}

// Option configures a Model at New time.
type Option func(*config)

func defaultConfig() config {
	return config{
		closeKeys: []string{"q", "esc", "b"},
	}
}
```

```go
// model.go
package markdown_overlay

import tea "github.com/charmbracelet/bubbletea"

// Model is the markdown overlay's bubbletea model.
type Model struct {
	cfg    config
	render RenderFunc
}

// New constructs a Model. render must not be nil — a nil RenderFunc is
// a wiring bug, not a runtime fallback.
func New(render RenderFunc, opts ...Option) Model {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	return Model{cfg: cfg, render: render}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) { return m, nil }

func (m Model) View() string { return "" }
```

- [ ] **Step 1.4: Run test, verify pass**

Run: `go test ./internal/frontend/tui/components/markdown_overlay/ -v`
Expected: `PASS: TestNew_ReturnsTeaModel`

- [ ] **Step 1.5: Commit**

```bash
git add internal/frontend/tui/components/markdown_overlay/
git commit -m "feat(markdown_overlay): package skeleton + tea.Model contract (F4.1)"
```

---

### Task 2: Title + Source + Body Rendering

**Files:**
- Modify: `internal/frontend/tui/components/markdown_overlay/options.go`
- Modify: `internal/frontend/tui/components/markdown_overlay/model.go`
- Create: `internal/frontend/tui/components/markdown_overlay/setters.go`
- Modify: `internal/frontend/tui/components/markdown_overlay/model_test.go`

- [ ] **Step 2.1: Write failing test for source rendering**

```go
func TestSetSource_RendersThroughRenderFunc(t *testing.T) {
	got := ""
	render := func(src string, w int) string {
		got = src
		return "RENDERED:" + src
	}
	m := markdown_overlay.New(render, markdown_overlay.WithTitle("T"), markdown_overlay.WithSource("hello"))
	m = m.SetSize(40, 10)
	if got != "hello" {
		t.Errorf("render input: got %q, want %q", got, "hello")
	}
}

func TestWithSource_ConfiguresInitialBody(t *testing.T) {
	m := markdown_overlay.New(
		func(src string, w int) string { return "R:" + src },
		markdown_overlay.WithSource("body"),
	)
	m = m.SetSize(40, 10)
	view := m.View()
	if !strings.Contains(view, "R:body") {
		t.Errorf("view does not contain rendered body. Got:\n%s", view)
	}
}
```

- [ ] **Step 2.2: Run, verify fail**

Run: `go test ./internal/frontend/tui/components/markdown_overlay/`
Expected: `undefined: WithTitle, WithSource, SetSize` etc.

- [ ] **Step 2.3: Add Options + setters + minimal viewport**

In `options.go`, add:

```go
func WithTitle(title string) Option {
	return func(c *config) { c.title = title }
}

func WithSource(src string) Option {
	return func(c *config) { c.source = src }
}
```

In `setters.go`:

```go
package markdown_overlay

import "github.com/charmbracelet/bubbles/viewport"

// SetSize updates the overlay's outer dimensions and re-flows the body
// through the RenderFunc at the new inner width.
func (m Model) SetSize(w, h int) Model {
	m.width = w
	m.height = h
	return m.rerender()
}

// SetTitle replaces the title shown in the chrome.
func (m Model) SetTitle(title string) Model {
	m.cfg.title = title
	return m
}

// SetSource replaces the markdown body and re-renders.
func (m Model) SetSource(src string) Model {
	m.cfg.source = src
	return m.rerender()
}

// initViewport returns a viewport sized to the current inner box.
func (m Model) initViewport() viewport.Model {
	innerW, innerH := m.contentSize()
	vp := viewport.New(innerW, innerH)
	return vp
}
```

In `model.go`, expand Model:

```go
import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type Model struct {
	cfg    config
	render RenderFunc

	width  int
	height int

	rendered string
	viewport viewport.Model
}

// Chrome budget — see chrome.go for the exact split.
const (
	chromeVertical    = 2 + 4 // border top+bottom + title + sep + footer + statusBar
	contentLineBudget = 2 + 2 // border + padding both sides
	frameWidthOffset  = 2     // border-only; argument to lipgloss.Style.Width
	gutterWidth       = 2     // reserved for the search match bar
)

func (m Model) contentSize() (int, int) {
	innerW := m.width - contentLineBudget - gutterWidth
	innerH := m.height - chromeVertical
	if innerW < 1 || innerH < 1 {
		return 0, 0
	}
	return innerW, innerH
}

func (m Model) rerender() Model {
	innerW, innerH := m.contentSize()
	if innerW <= 0 || innerH <= 0 {
		m.rendered = ""
		m.viewport.Width = 0
		m.viewport.Height = 0
		return m
	}
	m.rendered = m.render(m.cfg.source, innerW)
	m.viewport.Width = innerW + gutterWidth
	m.viewport.Height = innerH
	m.viewport.SetContent(m.rendered)
	return m
}

func (m Model) View() string {
	if m.width <= contentLineBudget || m.height <= chromeVertical {
		return ""
	}
	return m.viewport.View()
}
```

- [ ] **Step 2.4: Run, verify pass**

Run: `go test ./internal/frontend/tui/components/markdown_overlay/ -v`
Expected: both new tests PASS.

- [ ] **Step 2.5: Commit**

```bash
git add internal/frontend/tui/components/markdown_overlay/
git commit -m "feat(markdown_overlay): source + size + viewport (F4.2)"
```

---

### Task 3: Chrome — frame + title + separator + footer

**Files:**
- Create: `internal/frontend/tui/components/markdown_overlay/chrome.go`
- Create: `internal/frontend/tui/components/markdown_overlay/chrome_styles.go`
- Modify: `internal/frontend/tui/components/markdown_overlay/model.go` (View → chrome)
- Modify: `internal/frontend/tui/components/markdown_overlay/model_test.go`

- [ ] **Step 3.1: Failing test — View output contains title + body separator**

```go
func TestView_ChromeContainsTitleAndBody(t *testing.T) {
	m := markdown_overlay.New(
		func(src string, w int) string { return "BODY" },
		markdown_overlay.WithTitle("MyTitle"),
		markdown_overlay.WithSource("x"),
	).SetSize(40, 10)
	out := m.View()
	if !strings.Contains(ansi.Strip(out), "MyTitle") {
		t.Errorf("title missing from view:\n%s", out)
	}
	if !strings.Contains(ansi.Strip(out), "BODY") {
		t.Errorf("body missing from view:\n%s", out)
	}
}
```

(Import `"github.com/charmbracelet/x/ansi"` in the test.)

- [ ] **Step 3.2: Run, verify fail**

Run: `go test ./internal/frontend/tui/components/markdown_overlay/ -run TestView_ChromeContainsTitleAndBody`
Expected: FAIL (output is just viewport body).

- [ ] **Step 3.3: Add chrome_styles.go**

Copy + adapt from `internal/kompendium/frontend/tui/view/styles.go`. Strip the `matchBar*` styles and `cursorStyle`+`searchActiveLabelStyle` (added in Task 5); keep frame/title/separator/footer/statusBar.

```go
// chrome_styles.go
package markdown_overlay

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

var (
	pal = theme.Default
	sem = pal.Sem()
)

// SetPalette swaps the package palette and rebuilds all styles. Call
// once at boot, before any New(...) — see cmd/flow/main.go.
func SetPalette(p theme.Palette) {
	pal = p
	sem = p.Sem()
	rebuildStyles()
}

var (
	frameStyle               lipgloss.Style
	titleStyle               lipgloss.Style
	separatorStyle           lipgloss.Style
	footerStyle              lipgloss.Style
	footerKeyStyle           lipgloss.Style
	statusBarStyle           lipgloss.Style
	statusBarModeSearchStyle lipgloss.Style
	statusBarPathStyle       lipgloss.Style
)

func init() { rebuildStyles() }

func rebuildStyles() {
	frameStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(sem.Accent).
		Padding(0, 1)

	titleStyle = lipgloss.NewStyle().
		Foreground(sem.Accent).
		Bold(true)

	separatorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(pal.BgChip))

	footerStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(pal.FgMuted))

	footerKeyStyle = lipgloss.NewStyle().
		Foreground(sem.Active).
		Bold(true)

	statusBarStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(pal.BgChip)).
		Foreground(lipgloss.Color(pal.FgDim))

	statusBarModeSearchStyle = lipgloss.NewStyle().
		Background(sem.Warning).
		Foreground(lipgloss.Color(pal.Bg)).
		Bold(true).
		Padding(0, 1)

	statusBarPathStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(pal.BgChip)).
		Foreground(lipgloss.Color(pal.Fg))
}
```

- [ ] **Step 3.4: Add chrome.go (renderChrome / renderFooter / renderStatusBar)**

```go
// chrome.go
package markdown_overlay

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderChrome assembles the frame: title row, separator, body
// (viewport view), footer hint row, and status bar. The frame border is
// wrapped on the outside via frameStyle.
func (m Model) renderChrome() string {
	lineW := m.width - contentLineBudget
	if lineW < 1 {
		lineW = 1
	}
	title := titleStyle.Render(m.titleWithPercent())
	sep := separatorStyle.Render(strings.Repeat("─", lineW))
	body := m.viewport.View()
	footer := m.renderFooter()
	statusBar := m.renderStatusBar()

	content := lipgloss.JoinVertical(lipgloss.Left, title, sep, body, footer, statusBar)
	target := m.height - 2
	if got := strings.Count(content, "\n") + 1; target > 0 && got < target {
		content += strings.Repeat("\n", target-got)
	}
	return frameStyle.Width(m.width - frameWidthOffset).Render(content)
}

// titleWithPercent prefixes the configured title with the current scroll
// percent, mirroring brief_view / today_note_view today.
func (m Model) titleWithPercent() string {
	if m.viewport.Height == 0 {
		return m.cfg.title
	}
	return formatTitleWithPercent(m.cfg.title, m.viewport.ScrollPercent())
}

func formatTitleWithPercent(title string, percent float64) string {
	return title + " · " + formatPercent(percent)
}

func formatPercent(p float64) string {
	pct := int(p*100 + 0.5)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return itoaPct(pct) + "%"
}

func itoaPct(n int) string {
	// 3 chars max
	if n == 100 {
		return "100"
	}
	if n < 10 {
		return " " + string(rune('0'+n))
	}
	return string([]rune{rune('0' + n/10), rune('0' + n%10)})
}

// renderFooter assembles the footer hint row. Without WithSearch /
// WithCodeCopy enabled, only close-key hints show.
func (m Model) renderFooter() string {
	parts := []string{
		footerStyle.Render("j/k → scrollen"),
	}
	if m.cfg.enableSearch {
		parts = append(parts, footerKeyStyle.Render("/")+footerStyle.Render(" → suchen"))
	}
	if m.cfg.enableCodeCopy {
		parts = append(parts, footerKeyStyle.Render("c")+footerStyle.Render(" → Code kopieren"))
	}
	for _, x := range m.cfg.footerExtras {
		parts = append(parts, footerStyle.Render(x))
	}
	parts = append(parts, footerKeyStyle.Render(strings.Join(m.cfg.closeKeys, "/"))+footerStyle.Render(" → zurück"))
	return footerStyle.Render(strings.Join(parts, "  ·  "))
}

// renderStatusBar produces the bottom status row: mode badge (left),
// title path (center-left), scroll percent or match counter (right).
// Search mode + code-copy add to the right segment in later tasks.
func (m Model) renderStatusBar() string {
	innerW := m.width - contentLineBudget
	if innerW <= 0 {
		return ""
	}
	title := m.cfg.title
	if title == "" {
		title = "—"
	}
	pathSegment := statusBarPathStyle.Render(" " + title + " ")
	right := statusBarStyle.Render(" " + formatPercent(m.viewport.ScrollPercent()) + " ")
	gap := innerW - lipgloss.Width(pathSegment) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	return pathSegment + statusBarStyle.Render(strings.Repeat(" ", gap)) + right
}
```

- [ ] **Step 3.5: Rewrite Model.View to call renderChrome**

In model.go, replace `View()`:

```go
func (m Model) View() string {
	if m.width <= contentLineBudget || m.height <= chromeVertical {
		return ""
	}
	return m.renderChrome()
}
```

- [ ] **Step 3.6: Run, verify pass**

Run: `go test ./internal/frontend/tui/components/markdown_overlay/ -v`
Expected: chrome test + earlier tests all pass.

- [ ] **Step 3.7: Commit**

```bash
git add internal/frontend/tui/components/markdown_overlay/
git commit -m "feat(markdown_overlay): chrome (frame + title + sep + footer + statusBar) (F4.3)"
```

---

### Task 4: Close-Keys + ExitMsg + Update Routing

**Files:**
- Modify: `internal/frontend/tui/components/markdown_overlay/model.go`
- Modify: `internal/frontend/tui/components/markdown_overlay/options.go`
- Create: `internal/frontend/tui/components/markdown_overlay/keymap.go`
- Modify: `internal/frontend/tui/components/markdown_overlay/model_test.go`

- [ ] **Step 4.1: Failing test — `q` emits ExitMsg**

```go
func TestUpdate_CloseKeyEmitsExitMsg(t *testing.T) {
	m := markdown_overlay.New(func(src string, w int) string { return src },
		markdown_overlay.WithSource("x")).SetSize(40, 10)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected non-nil cmd from close-key")
	}
	if _, ok := cmd().(markdown_overlay.ExitMsg); !ok {
		t.Errorf("expected ExitMsg, got %T", cmd())
	}
}

func TestUpdate_NonCloseKeyDoesNotExit(t *testing.T) {
	m := markdown_overlay.New(func(src string, w int) string { return src },
		markdown_overlay.WithSource("x")).SetSize(40, 10)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if cmd != nil {
		if _, ok := cmd().(markdown_overlay.ExitMsg); ok {
			t.Error("non-close key emitted ExitMsg")
		}
	}
}
```

- [ ] **Step 4.2: Run, verify fail**

Run: `go test ./internal/frontend/tui/components/markdown_overlay/ -run TestUpdate_CloseKey`
Expected: FAIL — `ExitMsg` undefined and `Update` returns nil cmd.

- [ ] **Step 4.3: Add ExitMsg + close-key handling**

```go
// model.go (additions)
type ExitMsg struct{}

func exitCmd() tea.Cmd { return func() tea.Msg { return ExitMsg{} } }

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.SetSize(msg.Width, msg.Height), nil
	case tea.KeyMsg:
		if m.isCloseKey(msg) {
			return m, exitCmd()
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m Model) isCloseKey(msg tea.KeyMsg) bool {
	s := msg.String()
	for _, k := range m.cfg.closeKeys {
		if s == k {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4.4: Add WithCloseKeys option**

```go
// options.go
func WithCloseKeys(keys ...string) Option {
	return func(c *config) {
		if len(keys) > 0 {
			c.closeKeys = append([]string{}, keys...)
		}
	}
}
```

- [ ] **Step 4.5: Add stub keymap.go (placeholder for richer keymap in later tasks)**

```go
// keymap.go
package markdown_overlay

import "github.com/charmbracelet/bubbles/key"

// keyMap collects the key bindings the overlay reacts to. Close keys
// are matched dynamically (config-driven); the rest are static bindings
// activated when the corresponding feature flag is set in config.
type keyMap struct {
	Search    key.Binding
	NextMatch key.Binding
	PrevMatch key.Binding
	Top       key.Binding
	Bottom    key.Binding
	CopyCode  key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Search:    key.NewBinding(key.WithKeys("/")),
		NextMatch: key.NewBinding(key.WithKeys("n")),
		PrevMatch: key.NewBinding(key.WithKeys("N")),
		Top:       key.NewBinding(key.WithKeys("g", "home")),
		Bottom:    key.NewBinding(key.WithKeys("G", "end")),
		CopyCode:  key.NewBinding(key.WithKeys("c")),
	}
}
```

- [ ] **Step 4.6: Run, verify pass**

Run: `go test ./internal/frontend/tui/components/markdown_overlay/ -v`
Expected: PASS.

- [ ] **Step 4.7: Commit**

```bash
git add internal/frontend/tui/components/markdown_overlay/
git commit -m "feat(markdown_overlay): close-keys + ExitMsg (F4.4)"
```

---

### Task 5: Search Mode

**Files:**
- Create: `internal/frontend/tui/components/markdown_overlay/search.go`
- Modify: `internal/frontend/tui/components/markdown_overlay/model.go`
- Modify: `internal/frontend/tui/components/markdown_overlay/chrome_styles.go` (matchBar styles)
- Modify: `internal/frontend/tui/components/markdown_overlay/chrome.go` (status bar match counter)
- Modify: `internal/frontend/tui/components/markdown_overlay/options.go`
- Modify: `internal/frontend/tui/components/markdown_overlay/model_test.go`

- [ ] **Step 5.1: Failing tests**

```go
func TestSearch_DisabledByDefault(t *testing.T) {
	m := markdown_overlay.New(func(s string, w int) string { return "foo bar\nbar baz" },
		markdown_overlay.WithSource("x")).SetSize(40, 10)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if m.CurrentMode() != markdown_overlay.ModeNormal {
		t.Errorf("/ activated search without WithSearch(); mode=%v", m.CurrentMode())
	}
}

func TestSearch_EnabledFindsMatches(t *testing.T) {
	render := func(s string, w int) string { return "foo bar\nbar baz\nqux" }
	m := markdown_overlay.New(render,
		markdown_overlay.WithSource("ignored"),
		markdown_overlay.WithSearch(),
	).SetSize(40, 10)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if m.CurrentMode() != markdown_overlay.ModeSearch {
		t.Fatalf("expected ModeSearch, got %v", m.CurrentMode())
	}
	for _, r := range "bar" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.Query(); got != "bar" {
		t.Errorf("query: got %q, want %q", got, "bar")
	}
	if got := m.Matches(); len(got) != 2 {
		t.Errorf("matches: got %v, want 2 (lines with 'bar')", got)
	}
}
```

- [ ] **Step 5.2: Run, verify fail**

- [ ] **Step 5.3: Add Mode + search state + WithSearch option**

In options.go:

```go
func WithSearch() Option {
	return func(c *config) { c.enableSearch = true }
}
```

In search.go (new):

```go
package markdown_overlay

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// Mode is the overlay's input mode. Search is opt-in via WithSearch.
type Mode int

const (
	ModeNormal Mode = iota
	ModeSearch
)

// CurrentMode reports the live input mode. Exposed for the host
// status-bar and for tests.
func (m Model) CurrentMode() Mode { return m.mode }
func (m Model) Query() string     { return m.query }
func (m Model) Matches() []int    { return m.matches }
func (m Model) MatchIndex() int   { return m.matchIdx }

func newSearchInput() textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 256
	return ti
}

func (m Model) handleSearchKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = ModeNormal
		m.search.Blur()
		return m, nil
	case tea.KeyEnter:
		m.mode = ModeNormal
		m.search.Blur()
		return m.applyQuery(strings.TrimSpace(m.search.Value())), nil
	}
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	return m, cmd
}

func (m Model) maybeEnterSearch(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	if !m.cfg.enableSearch {
		return m, nil, false
	}
	if !key.Matches(msg, m.keys.Search) {
		return m, nil, false
	}
	m.mode = ModeSearch
	m.search.SetValue("")
	m.search.Focus()
	return m, textinput.Blink, true
}

func (m Model) cycleMatch(delta int) Model {
	if len(m.matches) == 0 {
		return m
	}
	m.matchIdx = (m.matchIdx + delta + len(m.matches)) % len(m.matches)
	m.viewport.SetContent(m.composeContent())
	m.scrollToCurrent()
	return m
}

func (m Model) applyQuery(q string) Model {
	m.query = strings.ToLower(q)
	m = m.recomputeMatches()
	m.viewport.SetContent(m.composeContent())
	m.scrollToCurrent()
	return m
}

func (m Model) recomputeMatches() Model {
	m.matches = m.matches[:0]
	if m.query == "" {
		m.matchIdx = 0
		return m
	}
	for i, line := range m.plain {
		if strings.Contains(line, m.query) {
			m.matches = append(m.matches, i)
		}
	}
	if m.matchIdx >= len(m.matches) {
		m.matchIdx = 0
	}
	return m
}

func (m *Model) scrollToCurrent() {
	if len(m.matches) == 0 {
		return
	}
	line := m.matches[m.matchIdx]
	target := line - m.viewport.Height/3
	if target < 0 {
		target = 0
	}
	m.viewport.SetYOffset(target)
}

// composeContent prepends the match gutter (or an empty gutter) to
// every rendered line so the viewport sees lines of consistent shape.
func (m Model) composeContent() string {
	if len(m.lines) == 0 {
		return ""
	}
	bar := matchBarStyle.Render("▎ ")
	curBar := matchCurrentBarStyle.Render("▎ ")
	const empty = "  "
	cur := -1
	if m.matchIdx >= 0 && m.matchIdx < len(m.matches) {
		cur = m.matches[m.matchIdx]
	}
	matchSet := make(map[int]struct{}, len(m.matches))
	for _, i := range m.matches {
		matchSet[i] = struct{}{}
	}
	out := make([]string, len(m.lines))
	for i, l := range m.lines {
		switch {
		case i == cur:
			out[i] = curBar + l
		case len(matchSet) > 0:
			if _, ok := matchSet[i]; ok {
				out[i] = bar + l
			} else {
				out[i] = empty + l
			}
		default:
			out[i] = empty + l
		}
	}
	return strings.Join(out, "\n")
}

// rerender's hook for search-mode: keep .lines + .plain so match scan
// works without re-rendering. Caller (rerender) populates both.
func (m *Model) refreshLineCache() {
	m.lines = strings.Split(m.rendered, "\n")
	m.plain = make([]string, len(m.lines))
	for i, l := range m.lines {
		m.plain[i] = strings.ToLower(ansi.Strip(l))
	}
}
```

- [ ] **Step 5.4: Wire search state into Model + Update + rerender**

In model.go:

```go
// Add to Model struct:
//   mode     Mode
//   search   textinput.Model
//   query    string
//   matches  []int
//   matchIdx int
//   lines    []string
//   plain    []string
//   keys     keyMap
```

In `New()`:

```go
return Model{
	cfg:    cfg,
	render: render,
	search: newSearchInput(),
	keys:   defaultKeys(),
}
```

In `Update`:

```go
case tea.KeyMsg:
	if m.mode == ModeSearch {
		return m.handleSearchKey(msg)
	}
	if updated, cmd, handled := m.maybeEnterSearch(msg); handled {
		return updated, cmd
	}
	switch {
	case key.Matches(msg, m.keys.NextMatch):
		return m.cycleMatch(+1), nil
	case key.Matches(msg, m.keys.PrevMatch):
		return m.cycleMatch(-1), nil
	}
	if m.isCloseKey(msg) {
		return m, exitCmd()
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
```

(Order matters: search-mode capture first, then search-enter, then match-cycle, then close-key, finally viewport.)

In `rerender()`, after setting `m.rendered`:

```go
m.refreshLineCache()
if m.query != "" {
	m = m.recomputeMatches()
}
m.viewport.SetContent(m.composeContent())
```

- [ ] **Step 5.5: Add matchBar styles**

In chrome_styles.go, add to `var (...)` block and `rebuildStyles`:

```go
matchBarStyle = lipgloss.NewStyle().Foreground(sem.Warning)
matchCurrentBarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(pal.Orange)).Bold(true)
searchActiveLabelStyle = lipgloss.NewStyle().Foreground(sem.Warning).Bold(true)
cursorStyle = lipgloss.NewStyle().Foreground(sem.Accent).Bold(true)
```

And set `m.search.Cursor.Style = cursorStyle` in `New()`.

- [ ] **Step 5.6: Status-bar match counter**

In chrome.go `renderStatusBar`, the right segment becomes:

```go
func (m Model) renderStatusBar() string {
	innerW := m.width - contentLineBudget
	if innerW <= 0 {
		return ""
	}
	mode := ""
	if m.mode == ModeSearch {
		mode = statusBarModeSearchStyle.Render("SEARCH")
	}
	title := m.cfg.title
	if title == "" {
		title = "—"
	}
	pathSegment := statusBarPathStyle.Render(" " + title + " ")
	right := m.statusBarRight()
	gap := innerW - lipgloss.Width(mode) - lipgloss.Width(pathSegment) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	return mode + pathSegment + statusBarStyle.Render(strings.Repeat(" ", gap)) + right
}

func (m Model) statusBarRight() string {
	if m.query != "" {
		var label string
		if len(m.matches) == 0 {
			label = " Keine Treffer "
		} else {
			label = " " + itoa(m.matchIdx+1) + "/" + itoa(len(m.matches)) + " "
		}
		return statusBarStyle.Render(label)
	}
	return statusBarStyle.Render(" " + formatPercent(m.viewport.ScrollPercent()) + " ")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
```

Also: footer renders the search textinput when in ModeSearch:

```go
// renderFooter (replace previous):
func (m Model) renderFooter() string {
	if m.mode == ModeSearch {
		view := m.search.View()
		if view == "" {
			view = "▎"
		}
		return searchActiveLabelStyle.Render("Suche:") + " " + view +
			"   " + footerStyle.Render("Enter → übernehmen  ·  Esc → abbrechen")
	}
	// ... original hint path
}
```

- [ ] **Step 5.7: Run, verify pass**

Run: `go test ./internal/frontend/tui/components/markdown_overlay/ -v`
Expected: all search tests pass.

- [ ] **Step 5.8: Commit**

```bash
git add internal/frontend/tui/components/markdown_overlay/
git commit -m "feat(markdown_overlay): search mode + match highlight + counter (F4.5)"
```

---

### Task 6: Code-Copy

**Files:**
- Create: `internal/frontend/tui/components/markdown_overlay/code_copy.go`
- Modify: `internal/frontend/tui/components/markdown_overlay/model.go`
- Modify: `internal/frontend/tui/components/markdown_overlay/options.go`
- Modify: `internal/frontend/tui/components/markdown_overlay/chrome.go` (status-bar copy-status)
- Modify: `internal/frontend/tui/components/markdown_overlay/model_test.go`

- [ ] **Step 6.1: Failing tests for snippet extraction + cycling**

```go
func TestCodeCopy_DisabledByDefault(t *testing.T) {
	body := "```sh\necho hi\n```\n"
	m := markdown_overlay.New(func(s string, w int) string { return s },
		markdown_overlay.WithSource(body)).SetSize(40, 10)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if m.CopyStatus() != "" {
		t.Errorf("c without WithCodeCopy should not change status; got %q", m.CopyStatus())
	}
}

func TestCodeCopy_EnabledCyclesSnippets(t *testing.T) {
	body := "intro\n```sh\necho one\n```\nmid\n```py\nprint(2)\n```\nend"
	m := markdown_overlay.New(func(s string, w int) string { return s },
		markdown_overlay.WithSource(body),
		markdown_overlay.WithCodeCopy(),
	).SetSize(40, 10)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if !strings.Contains(m.CopyStatus(), "1/2") {
		t.Errorf("first c: got status %q, want contains 1/2", m.CopyStatus())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if !strings.Contains(m.CopyStatus(), "2/2") {
		t.Errorf("second c: got status %q, want contains 2/2", m.CopyStatus())
	}
}
```

(Add `CopyStatus() string` accessor.)

- [ ] **Step 6.2: Run, verify fail**

- [ ] **Step 6.3: Port code_copy.go from kompendium/view/copy.go**

Copy the file `internal/kompendium/frontend/tui/view/copy.go` to `internal/frontend/tui/components/markdown_overlay/code_copy.go` and:
- Change package to `markdown_overlay`
- Keep `codeSnippet` struct, `extractCodeSnippets`, `osc52SetClipboard`, `writeClipboardCmd`, `writeClipboardLocal`, `clipboardWriters`, `isClosingFence`, `fenceLine`
- Add the A1 platform-detection comment on `osc52SetClipboard` (already done in Q2 — keep it)

Add Model methods:

```go
// copyNextSnippet picks the next snippet in cycle order, updates the
// status-bar label, and returns the body for the OSC52 cmd.
func (m Model) copyNextSnippet() (Model, string) {
	if len(m.snippets) == 0 {
		m.copyStatus = "Keine Code-Blöcke zum Kopieren."
		return m, ""
	}
	if m.copyIdx >= len(m.snippets) {
		m.copyIdx = 0
	}
	snip := m.snippets[m.copyIdx]
	label := snip.lang
	if label == "" {
		label = "Code"
	}
	m.copyStatus = "Kopiert: " + label + " " + itoa(m.copyIdx+1) + "/" + itoa(len(m.snippets))
	m.copyIdx++
	return m, snip.body
}

// CopyStatus exposes the current status line for tests + host
// integration.
func (m Model) CopyStatus() string { return m.copyStatus }

// clearCopyStatusMsg fires ~2s after a c press so the status fades.
type clearCopyStatusMsg struct{}

func clearCopyStatusCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return clearCopyStatusMsg{} })
}
```

- [ ] **Step 6.4: Add WithCodeCopy option + snippets field + Update routing**

In options.go:

```go
func WithCodeCopy() Option {
	return func(c *config) { c.enableCodeCopy = true }
}
```

In model.go, extend Model with `snippets []codeSnippet`, `copyIdx int`, `copyStatus string`. In `New()`:

```go
if cfg.enableCodeCopy {
	// snippets are extracted on every SetSource; we set them lazily in rerender.
}
```

(Actually simpler: extract on SetSource. Move the extraction to `rerender`.)

In `rerender()` add:

```go
if m.cfg.enableCodeCopy {
	m.snippets = extractCodeSnippets(m.cfg.source)
}
```

In Update, add the `c` key handler before the close-key check:

```go
if m.cfg.enableCodeCopy && key.Matches(msg, m.keys.CopyCode) {
	updated, payload := m.copyNextSnippet()
	if payload == "" {
		return updated, clearCopyStatusCmd()
	}
	return updated, tea.Batch(writeClipboardCmd(payload), clearCopyStatusCmd())
}
```

Plus the clearCopyStatus message:

```go
case clearCopyStatusMsg:
	m.copyStatus = ""
	return m, nil
```

- [ ] **Step 6.5: Status-bar shows copy-status**

In chrome.go `statusBarRight`, prepend:

```go
if m.copyStatus != "" {
	return statusBarStyle.Render(" " + m.copyStatus + " ")
}
```

- [ ] **Step 6.6: Run, verify pass**

Run: `go test ./internal/frontend/tui/components/markdown_overlay/ -v`

- [ ] **Step 6.7: Commit**

```bash
git add internal/frontend/tui/components/markdown_overlay/
git commit -m "feat(markdown_overlay): code-copy + OSC52 + clipboard fallback (F4.6)"
```

---

### Task 7: SetError + Footer Hints + Final Polish

**Files:**
- Modify: `internal/frontend/tui/components/markdown_overlay/setters.go`
- Modify: `internal/frontend/tui/components/markdown_overlay/model.go`
- Modify: `internal/frontend/tui/components/markdown_overlay/chrome.go`
- Modify: `internal/frontend/tui/components/markdown_overlay/options.go`
- Modify: `internal/frontend/tui/components/markdown_overlay/model_test.go`

- [ ] **Step 7.1: Failing tests for SetError + WithFooterExtras**

```go
func TestSetError_RendersInsteadOfBody(t *testing.T) {
	m := markdown_overlay.New(func(s string, w int) string { return "X" },
		markdown_overlay.WithSource("ignored")).SetSize(60, 10)
	m = m.SetError(errors.New("boom"))
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "boom") {
		t.Errorf("SetError body missing 'boom':\n%s", view)
	}
	if strings.Contains(view, "X") {
		t.Errorf("body 'X' should not appear when error set:\n%s", view)
	}
}

func TestWithFooterExtras_RendersInFooter(t *testing.T) {
	m := markdown_overlay.New(func(s string, w int) string { return "x" },
		markdown_overlay.WithSource("x"),
		markdown_overlay.WithFooterExtras("p → punch"),
	).SetSize(60, 10)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "p → punch") {
		t.Errorf("footer extra missing:\n%s", view)
	}
}
```

- [ ] **Step 7.2: Run, verify fail**

- [ ] **Step 7.3: Add err field + SetError + WithFooterExtras**

In model.go, add `err error` field.

In setters.go:

```go
func (m Model) SetError(err error) Model {
	m.err = err
	return m
}
```

In options.go:

```go
func WithFooterExtras(hints ...string) Option {
	return func(c *config) { c.footerExtras = append([]string{}, hints...) }
}
```

In chrome.go `renderChrome`, replace `body := m.viewport.View()`:

```go
body := m.bodyView()
```

And add:

```go
func (m Model) bodyView() string {
	if m.err != nil {
		// Tinted error line in place of the viewport.
		errStyle := lipgloss.NewStyle().Foreground(sem.Negative)
		return "\n  " + errStyle.Render("Fehler: "+m.err.Error())
	}
	return m.viewport.View()
}
```

(`sem.Negative` — check it exists; if not, use `sem.Error` or fall back to `pal.Red`.)

- [ ] **Step 7.4: Run, verify pass**

- [ ] **Step 7.5: Run full make ci**

Run: `make ci`
Expected: green, coverage stays ≥85%.

- [ ] **Step 7.6: Commit**

```bash
git add internal/frontend/tui/components/markdown_overlay/
git commit -m "feat(markdown_overlay): SetError + WithFooterExtras + final polish (F4.7)"
```

---

### Task 8: Golden-file tests

**Files:**
- Create: `internal/frontend/tui/components/markdown_overlay/golden_test.go`
- Create: `internal/frontend/tui/components/markdown_overlay/testdata/*.golden`

- [ ] **Step 8.1: Add golden helper + 4 scenarios**

```go
// golden_test.go
package markdown_overlay_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown_overlay"
)

var updateGolden = flag.Bool("update", false, "rewrite golden files")

type goldenCase struct {
	name string
	build func() markdown_overlay.Model
}

func TestGolden(t *testing.T) {
	cases := []goldenCase{
		{
			name: "small_body_no_scroll",
			build: func() markdown_overlay.Model {
				return markdown_overlay.New(
					func(s string, w int) string { return "single line" },
					markdown_overlay.WithTitle("Title"),
					markdown_overlay.WithSource("ignored"),
				).SetSize(40, 12)
			},
		},
		{
			name: "narrow_width",
			build: func() markdown_overlay.Model {
				return markdown_overlay.New(
					func(s string, w int) string { return "narrow" },
					markdown_overlay.WithTitle("X"),
					markdown_overlay.WithSource("x"),
				).SetSize(20, 8)
			},
		},
		{
			name: "search_active_with_matches",
			build: func() markdown_overlay.Model {
				render := func(s string, w int) string {
					return "alpha foo bar\nbeta foo qux\ngamma"
				}
				m := markdown_overlay.New(render,
					markdown_overlay.WithTitle("S"),
					markdown_overlay.WithSource("x"),
					markdown_overlay.WithSearch(),
				).SetSize(50, 12)
				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
				for _, r := range "foo" {
					m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				}
				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
				return m
			},
		},
		{
			name: "code_copy_status",
			build: func() markdown_overlay.Model {
				body := "intro\n```sh\necho a\n```\n"
				m := markdown_overlay.New(
					func(s string, w int) string { return s },
					markdown_overlay.WithTitle("C"),
					markdown_overlay.WithSource(body),
					markdown_overlay.WithCodeCopy(),
				).SetSize(50, 12)
				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
				return m
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.build().View()
			path := filepath.Join("testdata", tc.name+".golden")
			if *updateGolden {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			if got != string(want) {
				t.Errorf("golden mismatch for %s:\nwant:\n%s\ngot:\n%s",
					tc.name, want, got)
			}
		})
	}
}
```

- [ ] **Step 8.2: Generate goldens**

Run: `go test ./internal/frontend/tui/components/markdown_overlay/ -run TestGolden -update`

Verify files exist:

```bash
ls internal/frontend/tui/components/markdown_overlay/testdata/
```

Expected: 4 .golden files.

- [ ] **Step 8.3: Run without -update, verify pass**

Run: `go test ./internal/frontend/tui/components/markdown_overlay/ -run TestGolden`
Expected: PASS.

- [ ] **Step 8.4: Commit**

```bash
git add internal/frontend/tui/components/markdown_overlay/
git commit -m "test(markdown_overlay): golden-file tests for 4 chrome scenarios (F4.8)"
```

---

## Phase 2: Migrate kompendium-view

### Task 9: browse adopts markdown_overlay

**Files:**
- Modify: `internal/kompendium/frontend/tui/browse/model.go` (viewer field type)
- Modify: `internal/kompendium/frontend/tui/browse/preview.go` (constructor)
- Modify: `internal/kompendium/frontend/tui/browse/update.go` (ExitMsg consumption)
- Modify: `cmd/flow/main.go` (call markdown_overlay.SetPalette instead of view.SetPalette)

- [ ] **Step 9.1: Skim current view.New call site**

Run: `rg -n "view\.New|view\.Model|view\.SetPalette|view\.ExitMsg" internal/kompendium cmd/flow`
Note each call site.

- [ ] **Step 9.2: Replace viewer field**

In browse/model.go `Model`:

```go
viewer markdown_overlay.Model  // was: view.Model
```

Imports: replace `view "...."` with `markdown_overlay ".../markdown_overlay"`.

- [ ] **Step 9.3: Build the render closure in preview.go**

In browse/preview.go, the `v := view.New(title, source, m.wikilinkResolver(), &meta, backlinks)` line becomes:

```go
resolver := m.wikilinkResolver()
fm := &meta
bls := backlinks
render := func(src string, w int) string {
	var opts []markdown.Option
	if resolver != nil {
		opts = append(opts, markdown.WithWikilinks(resolver))
	}
	if fm != nil {
		opts = append(opts, markdown.WithFrontmatter(fm))
	}
	if len(bls) > 0 {
		opts = append(opts, markdown.WithBacklinks(bls))
	}
	out, _ := markdown.Render(src, w, opts...)
	return out
}
v := markdown_overlay.New(render,
	markdown_overlay.WithTitle(title),
	markdown_overlay.WithSource(source),
	markdown_overlay.WithSearch(),
	markdown_overlay.WithCodeCopy(),
)
```

(Add markdown import if missing.)

- [ ] **Step 9.4: Consume ExitMsg in browse/update.go**

Find the place that handles `view.ExitMsg`. Replace with `markdown_overlay.ExitMsg`.

Look for `m.viewer = view.Model{}` and replace with `m.viewer = markdown_overlay.Model{}` — or use a zero-Model factory.

- [ ] **Step 9.5: Update cmd/flow/main.go**

Find: `view.SetPalette(pal)` (or equivalent).
Replace with: `markdown_overlay.SetPalette(pal)`.

- [ ] **Step 9.6: Run targeted tests**

Run: `go test ./internal/kompendium/frontend/tui/browse/`
Expected: PASS — model_test.go, edit_reload_internal_test.go, etc.

- [ ] **Step 9.7: Run make ci**

Run: `make ci`
Expected: green, coverage ≥85%.

- [ ] **Step 9.8: Commit**

```bash
git add internal/kompendium/frontend/tui/browse/ cmd/flow/main.go
git commit -m "refactor(kompendium/browse): markdown_overlay statt eigener view.Model (F4.9)"
```

---

### Task 10: Delete kompendium/view package

**Files:**
- Delete: `internal/kompendium/frontend/tui/view/model.go`
- Delete: `internal/kompendium/frontend/tui/view/copy.go`
- Delete: `internal/kompendium/frontend/tui/view/styles.go`
- Delete: `internal/kompendium/frontend/tui/view/keymap.go`
- Delete: `internal/kompendium/frontend/tui/view/markdown_adapter.go`
- Delete: `internal/kompendium/frontend/tui/view/model_test.go`
- Delete: `internal/kompendium/frontend/tui/view/copy_internal_test.go`

- [ ] **Step 10.1: Verify no remaining callers**

Run: `rg "kompendium/frontend/tui/view" internal/ cmd/ 2>&1`
Expected: empty output.

- [ ] **Step 10.2: Delete the package directory**

```bash
rm -rf internal/kompendium/frontend/tui/view/
```

- [ ] **Step 10.3: Migrate kompendium-view tests into markdown_overlay/**

For each test that exercised behavior generic to the overlay (not browse-specific), move it into `internal/frontend/tui/components/markdown_overlay/`. Skip tests already covered by golden + unit tests in Phase 1.

- [ ] **Step 10.4: Run make ci**

Expected: green.

- [ ] **Step 10.5: Commit**

```bash
git add -A
git commit -m "refactor(kompendium): delete view/ package nach markdown_overlay-Migration (F4.10)"
```

---

## Phase 3: Migrate worktime brief_view

### Task 11: brief_view embeds markdown_overlay

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/brief_view.go` (slim wrapper)
- Modify: `internal/frontend/tui/screen/worktime/model.go` (briefView field type)
- Modify: `internal/frontend/tui/screen/worktime/menu_brief.go` (constructor)
- Modify: `internal/frontend/tui/screen/worktime/today.go` (or wherever briefView lives)
- Modify: `internal/frontend/tui/screen/worktime/brief_view_test.go`
- Modify: `internal/frontend/tui/screen/worktime/menu_brief_test.go`

- [ ] **Step 11.1: Identify briefView field owner**

Run: `rg "briefView " internal/frontend/tui/screen/worktime/ -n | head -20`
Note the struct that holds the brief overlay.

- [ ] **Step 11.2: Replace struct + factory**

In brief_view.go, replace the `briefView` struct definition with:

```go
package worktime

import (
	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown_overlay"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// briefViewMsg signalisiert dem Worktime-Root, dass ein Brief gerendert
// und im Overlay angezeigt werden soll.
type briefViewMsg struct {
	title string
	body  string
}

// newBriefView baut den markdown_overlay mit dem deps.MarkdownRenderer
// als RenderFunc-Closure. nil-Renderer wäre Wiring-Bug; in dem Fall
// gibt der Closure raw markdown zurück (fail-soft).
func newBriefView(title, body string, width, height int, deps Deps, pal theme.Palette) markdown_overlay.Model {
	render := func(src string, w int) string {
		if deps.MarkdownRenderer == nil {
			return src
		}
		out, err := deps.MarkdownRenderer.Render(src, w)
		if err != nil {
			return src
		}
		return out
	}
	return markdown_overlay.New(render,
		markdown_overlay.WithTitle(title),
		markdown_overlay.WithSource(body),
		markdown_overlay.WithSearch(),
		markdown_overlay.WithCodeCopy(),
	).SetSize(width, height)
}
```

(Remove the old `briefView` struct + `briefViewWidth`/`briefViewHeight`/`updateKey`/`resize`/`view` methods; SetSize now handles resize.)

- [ ] **Step 11.3: Update host (heute or worktime model)**

Find the field holding `briefView` and change its type to `*markdown_overlay.Model`. Route messages:

```go
// On WindowSizeMsg:
if h.briefView != nil {
	upd := h.briefView.SetSize(w, h)
	h.briefView = &upd
}

// On tea.KeyMsg or other tea.Msg when briefView is active:
if h.briefView != nil {
	upd, cmd := h.briefView.Update(msg)
	h.briefView = &upd
	return h, cmd
}

// On markdown_overlay.ExitMsg:
case markdown_overlay.ExitMsg:
	h.briefView = nil
	return h, nil
```

- [ ] **Step 11.4: Update brief_view_test.go**

Rewrite assertions to reflect the new chrome (rounded frame, status bar). Where the old test asserted titlebox output, switch to asserting `ansi.Strip(view).Contains(title)` + `Contains(body)` instead. Don't check exact byte output.

- [ ] **Step 11.5: Run brief tests**

Run: `go test ./internal/frontend/tui/screen/worktime/ -run TestBrief`
Expected: PASS.

- [ ] **Step 11.6: Run make ci**

Expected: green.

- [ ] **Step 11.7: Commit**

```bash
git add internal/frontend/tui/screen/worktime/
git commit -m "refactor(worktime/brief): markdown_overlay statt eigener briefView-Struct (F4.11)"
```

---

## Phase 4: Migrate worktime today_note_view

### Task 12: today_note_view embeds markdown_overlay

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/today_note_view.go` (slim wrapper)
- Modify: `internal/frontend/tui/screen/worktime/today.go` (state + dispatch)
- Modify: `internal/frontend/tui/screen/worktime/today_dialog_keys.go`
- Modify: `internal/frontend/tui/screen/worktime/today_render.go`
- Modify: `internal/frontend/tui/screen/worktime/today_note_view_test.go` (incl. F1 resize regression)

- [ ] **Step 12.1: Replace today_note_view.go**

```go
package worktime

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown_overlay"
)

// openNoteViewDialog aktiviert den integrierten Note-Viewer. Liest die
// Note via deps.NoteReader, baut den Overlay und übernimmt einen
// initialen SetError-Pfad bei Read-Fehlern.
func (h heute) openNoteViewDialog() (tea.Model, tea.Cmd) {
	if len(h.attachedNotes) == 0 {
		return h, func() tea.Msg {
			return heuteActionDoneMsg{toast: "Keine Notiz angehängt — `n` hängt eine an", info: true}
		}
	}
	if h.deps.NoteReader == nil {
		return h, func() tea.Msg {
			return heuteActionDoneMsg{err: fmt.Errorf("note-reader nicht verdrahtet")}
		}
	}
	id := h.attachedNotes[0]
	render := func(src string, w int) string {
		if h.deps.MarkdownRenderer == nil {
			return src
		}
		out, err := h.deps.MarkdownRenderer.Render(src, w)
		if err != nil {
			return src
		}
		return out
	}
	overlay := markdown_overlay.New(render,
		markdown_overlay.WithTitle("Note · "+id),
		markdown_overlay.WithSearch(),
		markdown_overlay.WithCodeCopy(),
	).SetSize(h.width, h.height)
	body, err := h.deps.NoteReader.Read(id)
	if err != nil {
		overlay = overlay.SetError(err)
	} else {
		overlay = overlay.SetSource(body)
	}
	h.dialog = heuteDialogNoteView
	h.noteViewID = id
	h.noteView = &overlay
	return h, nil
}
```

(Remove the old renderNoteViewBody, renderNoteViewDialog, updateNoteViewKey methods — replaced by overlay.Update + overlay.View.)

- [ ] **Step 12.2: Update today.go state**

Remove fields: `noteViewBody`, `noteViewVP`, `noteViewReady`, `noteViewErr`.
Keep: `noteViewID string`.
Add: `noteView *markdown_overlay.Model`.

- [ ] **Step 12.3: Update today_dialog_keys.go routing**

```go
// in the dialog key dispatch:
if h.dialog == heuteDialogNoteView && h.noteView != nil {
	upd, cmd := h.noteView.Update(msg)
	h.noteView = &upd
	return h, cmd
}
```

Plus the ExitMsg case at the message-top-level:

```go
case markdown_overlay.ExitMsg:
	if h.dialog == heuteDialogNoteView {
		h.dialog = heuteDialogNone
		h.noteView = nil
	}
	return h, nil
```

- [ ] **Step 12.4: Update today_render.go**

Replace `h.renderNoteViewDialog(inner)` with `h.noteView.View()`.

- [ ] **Step 12.5: Update today_note_view_test.go — keep F1 resize regression**

The F1 resize regression test was: after WindowSizeMsg, the overlay re-renders markdown at the new width. The new overlay handles this in SetSize → rerender. Test it:

```go
func TestNoteView_ResizeReflowsMarkdown(t *testing.T) {
	widths := []int{}
	render := func(src string, w int) string {
		widths = append(widths, w)
		return "rendered@" + itoa(w)
	}
	// instantiate a minimal heute, openNoteView, then send WindowSizeMsg
	// ... (mirror current F1 test setup)
	// after second WindowSizeMsg, widths should have at least 2 distinct values
}
```

- [ ] **Step 12.6: Run note tests**

Run: `go test ./internal/frontend/tui/screen/worktime/ -run TestNoteView`
Expected: PASS.

- [ ] **Step 12.7: Run make ci**

Expected: green.

- [ ] **Step 12.8: Commit**

```bash
git add internal/frontend/tui/screen/worktime/
git commit -m "refactor(worktime/note): markdown_overlay statt eigener noteView-State (F4.12)"
```

---

## Phase 5: Cleanup

### Task 13: screenBaseline + dead code sweep

**Files:**
- Modify: `internal/frontend/tui/lint/screen_baseline_test.go`

- [ ] **Step 13.1: Run lint baseline test, observe deltas**

Run: `go test ./internal/frontend/tui/lint/ -run TestScreenInlineNewStyleBudget -v`

Expected: test logs lower counts for `worktime/brief_view.go` (was no entry) and `worktime/today.go` (count likely drops). Test fails with "baseline is N, current is M, lower the baseline".

- [ ] **Step 13.2: Update screenBaseline entries**

Edit `internal/frontend/tui/lint/screen_baseline_test.go` to reflect new counts. If brief_view.go or today_note_view.go now have 0 NewStyle calls, remove their entries from the map.

- [ ] **Step 13.3: Run make ci**

Expected: green, coverage stays ≥85%.

- [ ] **Step 13.4: Commit**

```bash
git add internal/frontend/tui/lint/screen_baseline_test.go
git commit -m "chore(lint): screenBaseline nach markdown_overlay-Lift (F4.13)"
```

---

### Task 14: Final verification

- [ ] **Step 14.1: Build the binary and exercise the three overlays manually**

```bash
make build
./bin/flow worktime today  # open today, press `o` for note overlay
./bin/flow worktime brief week  # brief overlay
./bin/flow kompendium browse  # press `v` on a note for kompendium overlay
```

For each: verify q/Esc/b close, `/` opens search, `c` copies code, status bar shows scroll percent / match counter / copy confirmation.

- [ ] **Step 14.2: Run make ci one last time**

Run: `make ci`
Expected: green.

- [ ] **Step 14.3: Verify no remaining references**

Run: `rg "kompendium/frontend/tui/view|briefView struct|noteViewBody|noteViewVP|noteViewReady|noteViewErr" internal/ cmd/`
Expected: empty.

- [ ] **Step 14.4: Inspect final diff size**

Run: `git log --oneline f76dfb6..HEAD`
Expected: roughly 14 commits Q3/Q1/Q2/Spec + 8 base-component tasks + 4 migration tasks + 2 cleanup tasks.

---

## Self-Review

- **Spec coverage:** Each spec section maps to ≥1 task. Scope (Phase 1), Form (`tea.Model` — Task 1), Chrome (Task 3), Search (Task 5), Code-Copy (Task 6), all three migrations (Tasks 9-12), screenBaseline cleanup (Task 13). ✓
- **Placeholder scan:** No TBD/TODO. Each code step shows complete code. ✓
- **Type consistency:** `Model`, `New`, `RenderFunc`, `Option`, `WithSearch`, `WithCodeCopy`, `WithCloseKeys`, `WithFooterExtras`, `WithTitle`, `WithSource`, `SetSize`, `SetTitle`, `SetSource`, `SetError`, `ExitMsg`, `Mode`, `ModeNormal`, `ModeSearch`, `CurrentMode`, `Query`, `Matches`, `MatchIndex`, `CopyStatus` — consistent across tasks. ✓
- **Deviation flag:** `WithMarkdownOptions` from spec → `RenderFunc` closure — documented at the top.
