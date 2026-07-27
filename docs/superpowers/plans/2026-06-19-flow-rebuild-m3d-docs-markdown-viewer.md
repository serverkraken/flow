# flow rebuild M3d-Docs — Fullscreen Markdown Document Viewer — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a document is opened in the `flow ui` Docs tab, show it **fullscreen** with the rich goldmark→ANSI markdown viewer (ported from `main`), including keyboard wikilink navigation with a visible focus highlight.

**Architecture:** Port three presentation packages from `main` verbatim with import rewrites (`internal/tui/markdown/theme`, `internal/tui/markdown`, `internal/tui/ui/markdown_overlay`), add a small render-contract extension for a focused wikilink, add a `shell.FullScreener` takeover mechanism, then wire the overlay into the legacy `tui.DocsModel`'s view mode with a heap-cell focus state and a Frame→SetSize bridge.

**Tech Stack:** Go, charm.land/bubbletea+lipgloss+bubbles v2, goldmark + chroma/v2, the rebuild `internal/tui/{theme,ui,shell}` design system.

**Spec:** `docs/superpowers/specs/2026-06-19-flow-rebuild-m3d-docs-markdown-viewer-design.md`

## Global Constraints

- charm.land v2 import paths only (`charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `charm.land/bubbles/v2`).
- **Import rewrite map** (applied to every ported file): `internal/frontend/tui/components/glyphs`→`internal/tui/ui/glyphs`; `internal/frontend/tui/components/strings`→`internal/tui/ui/strings`; `internal/frontend/tui/markdown`→`internal/tui/markdown` (covers `markdown` + `markdown/theme`); `internal/frontend/tui/theme`→`internal/tui/theme`. The `internal/ports` dependency is removed (replaced by a local interface).
- Routes are **Frame-driven**: size comes from `View(f shell.Frame)` via `f.Width`/`f.Height`, never from a stored `tea.WindowSizeMsg`.
- No glamour (per the M3 design principle). Renderer is custom goldmark→ANSI.
- `DocsModel` stays in `package tui`; new accessors on it are read-only or value-receiver, matching the existing `CapturesInput()` accessor.
- `make ci` must stay green: lint (`golangci-lint`, watch SA4006), `verify-generate`, build, coverage ≥ 80 %.
- **Never stage** the pre-existing working-tree noise: `.gitignore`, `flow`, `cover*.out`. Stage only each task's named files.
- Source worktree for ports (read-only): `/Users/msoent/SourceCode/serverkraken/flow` (checked out at `main`). All `git show main:<path>` commands below run from the rebuild worktree (`main` is reachable there).

## File Structure

**New (ported):**
- `internal/tui/markdown/theme/{markdown.go,theme.go,markdown_test.go}` — palette→role bundle.
- `internal/tui/markdown/*.go` (15 impl + ~13 test files) — goldmark→ANSI pipeline.
- `internal/tui/markdown/interfaces.go` — local `WikilinkResolver` (replaces `ports`).
- `internal/tui/ui/markdown_overlay/*.go` + `testdata/*.golden` — viewport viewer component.

**New (this feature):**
- focus support inside `internal/tui/markdown/` (option + role + counter).
- `Rerender()` + `CapturesInput()` on the overlay.
- `FullScreener` in `internal/tui/shell/route.go` + branch in `shell.go`.

**Modified:**
- `internal/tui/docs.go` — mount overlay in `modeView`, focus state, key routing, accessors.
- `internal/tui/screen/docs/route.go` — `FullScreen()` + Frame→SetSize bridge.
- `go.mod` / `go.sum` — add chroma/v2 + x/cellbuf.

---

## Task 1: Add dependencies (chroma/v2, x/cellbuf)

**Files:** `go.mod`, `go.sum`.

- [ ] **Step 1: Add the modules at the versions `main` uses**

Run:
```bash
go get github.com/alecthomas/chroma/v2@v2.20.0
go get github.com/charmbracelet/x/cellbuf@v0.0.15
go mod tidy
```
(`github.com/charmbracelet/x/ansi v0.11.7` and `github.com/yuin/goldmark v1.8.2` are already present.)

- [ ] **Step 2: Verify the build still compiles**

Run: `go build ./...`
Expected: clean (no new packages use the deps yet; this only proves they resolve).

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "build(m3d): add chroma/v2 + x/cellbuf for the markdown viewer"
```

---

## Task 2: Port `markdown/theme` sub-package

**Files:**
- Create: `internal/tui/markdown/theme/markdown.go`, `internal/tui/markdown/theme/theme.go`, `internal/tui/markdown/theme/markdown_test.go`

**Interfaces:**
- Produces: `theme.MarkdownRoles`, `theme.MarkdownRolesFor(palette)`, `theme.CalloutBadge(...)`, `theme.CalloutBar(...)` (exact API as on `main`). Consumed by Task 3.

- [ ] **Step 1: Copy the three files with the import rewrite**

Run from the rebuild worktree:
```bash
mkdir -p internal/tui/markdown/theme
for f in markdown.go theme.go markdown_test.go; do
  git show main:internal/frontend/tui/markdown/theme/$f \
  | sed -e 's#internal/frontend/tui/components/glyphs#internal/tui/ui/glyphs#g' \
        -e 's#internal/frontend/tui/components/strings#internal/tui/ui/strings#g' \
        -e 's#internal/frontend/tui/markdown#internal/tui/markdown#g' \
        -e 's#internal/frontend/tui/theme#internal/tui/theme#g' \
  > internal/tui/markdown/theme/$f
done
```

- [ ] **Step 2: Build the package**

Run: `go build ./internal/tui/markdown/theme/`
Expected: clean. If it references a `theme.Palette` field that does not exist, STOP and report (the spec claims zero gaps — a gap here is a real finding).

- [ ] **Step 3: Run the ported tests**

Run: `go test ./internal/tui/markdown/theme/ -v`
Expected: PASS. If a golden/string assertion fails, diff it: a mismatch means the rebuild palette differs from `main` — report rather than blindly editing fixtures.

- [ ] **Step 4: Lint + commit**

Run: `golangci-lint run ./internal/tui/markdown/theme/`
```bash
git add internal/tui/markdown/theme/
git commit -m "feat(m3d): port markdown/theme role bundle from main"
```

---

## Task 3: Port the `markdown` goldmark→ANSI renderer (verbatim)

**Files:**
- Create (impl): `internal/tui/markdown/{api.go,backlinks.go,blocks.go,code.go,doc.go,footnote.go,frontmatter.go,inline.go,osc8.go,render.go,renderer.go,table.go,types.go,width.go,wikilink.go}`
- Create (interface): `internal/tui/markdown/interfaces.go`
- Create (tests): `internal/tui/markdown/{api_test.go,backlinks_test.go,callout_test.go,code_test.go,footnote_test.go,frontmatter_test.go,list_test.go,osc8_test.go,render_test.go,strikethrough_test.go,table_test.go,wikilink_test.go}`

**Interfaces:**
- Consumes: `internal/tui/markdown/theme` (Task 2), `internal/tui/ui/glyphs`, `internal/tui/ui/strings`, `internal/tui/theme`.
- Produces: `markdown.Render(source string, width int, opts ...Option) (string, error)`; options `WithFrontmatter(*Frontmatter)`, `WithBacklinks([]BacklinkRef)`, `WithWikilinks(WikilinkResolver)`, `WithNoColor(bool)`, `WithNerdFont(bool)`, `WithPalette(theme.Palette)`; types `NoteType` (`TypeDaily`/`TypeProject`/`TypeFree` = `"daily"`/`"project"`/`"free"`), `Frontmatter{ID,Type NoteType,Project,Date,Title string,Tags []string}` (+ `IsEmpty()`), `BacklinkRef{ID,Title string}`; interface `WikilinkResolver { Resolve(target string) (uri string, title string, ok bool) }`.

- [ ] **Step 1: Copy all impl + test files with the import rewrite**

```bash
mkdir -p internal/tui/markdown
for f in api.go backlinks.go blocks.go code.go doc.go footnote.go frontmatter.go inline.go osc8.go render.go renderer.go table.go types.go width.go wikilink.go \
         api_test.go backlinks_test.go callout_test.go code_test.go footnote_test.go frontmatter_test.go list_test.go osc8_test.go render_test.go strikethrough_test.go table_test.go wikilink_test.go; do
  git show main:internal/frontend/tui/markdown/$f \
  | sed -e 's#internal/frontend/tui/components/glyphs#internal/tui/ui/glyphs#g' \
        -e 's#internal/frontend/tui/components/strings#internal/tui/ui/strings#g' \
        -e 's#internal/frontend/tui/markdown#internal/tui/markdown#g' \
        -e 's#internal/frontend/tui/theme#internal/tui/theme#g' \
  > internal/tui/markdown/$f
done
```

- [ ] **Step 2: Create the local `WikilinkResolver` interface**

Create `internal/tui/markdown/interfaces.go`:
```go
package markdown

// WikilinkResolver looks up `[[id]]` / `[[id|display]]` targets so the
// renderer can style them valid (OSC 8 hyperlink + accent) or broken (red
// marker). Returns ok=false when unknown. When ok=true, uri is the address
// the OSC 8 escape carries (the docs viewer uses flow://docs/<id>) and title
// is the fallback display when no `|display` override is given.
type WikilinkResolver interface {
	Resolve(target string) (uri string, title string, ok bool)
}
```

- [ ] **Step 3: Drop the `ports` dependency**

Replace the `ports.WikilinkResolver` references with the local type and remove the now-unused import.

In `internal/tui/markdown/render.go`:
```bash
sed -i '' -e 's#ports\.WikilinkResolver#WikilinkResolver#g' \
          -e '\#"github.com/serverkraken/flow/internal/ports"#d' \
  internal/tui/markdown/render.go
```
In `internal/tui/markdown/api.go` remove the interface assertion line and the import:
```bash
sed -i '' -e '/var _ ports\.MarkdownRenderer = Renderer{}/d' \
          -e '\#"github.com/serverkraken/flow/internal/ports"#d' \
  internal/tui/markdown/api.go
```
Then confirm no executable `ports.` reference remains (comments are fine):
```bash
rg -n 'ports\.' internal/tui/markdown/ | rg -v '^\S+:\s*//'
```
Expected: no output. (`doc.go`/`wikilink.go` mention `ports` only in comments — leave them or tidy the wording; they do not affect compilation.)

- [ ] **Step 4: Build the package**

Run: `go build ./internal/tui/markdown/`
Expected: clean. If `api.go`'s `Renderer` is left unused and lint later complains, that is handled in Step 6 (it is exported API, so it is not dead).

- [ ] **Step 5: Run the ported tests (golden ANSI)**

Run: `go test ./internal/tui/markdown/ -v`
Expected: PASS. If golden assertions fail, diff to confirm whether the delta is an expected theme difference; if the package supports `-update`, regenerate and eyeball the diff before committing. Do not weaken assertions.

- [ ] **Step 6: Lint + commit**

Run: `golangci-lint run ./internal/tui/markdown/`
```bash
git add internal/tui/markdown/
git commit -m "feat(m3d): port goldmark->ANSI markdown renderer from main (local WikilinkResolver)"
```

---

## Task 4: Add focused-wikilink rendering (`WithFocusedWikilink`)

**Files:**
- Modify: `internal/tui/markdown/theme/markdown.go` (add a `Focused` role)
- Modify: `internal/tui/markdown/render.go` (add the option + plumb the index)
- Modify: `internal/tui/markdown/wikilink.go` (apply focused style to the n-th valid wikilink)
- Test: `internal/tui/markdown/focus_test.go`

**Interfaces:**
- Produces: `markdown.WithFocusedWikilink(idx int) Option` (idx < 0 ⇒ none). Consumed by Task 8.

- [ ] **Step 1: Write the failing test**

Create `internal/tui/markdown/focus_test.go`:
```go
package markdown

import (
	"strings"
	"testing"
)

type stubResolver struct{}

func (stubResolver) Resolve(target string) (string, string, bool) {
	return "flow://docs/" + target, target, true
}

// Focusing the 2nd valid wikilink must style it differently from the 1st.
func TestWithFocusedWikilink_HighlightsNthValid(t *testing.T) {
	src := "see [[alpha]] and [[beta]] today"
	none, err := Render(src, 80, WithWikilinks(stubResolver{}), WithFocusedWikilink(-1))
	if err != nil {
		t.Fatal(err)
	}
	focused, err := Render(src, 80, WithWikilinks(stubResolver{}), WithFocusedWikilink(1))
	if err != nil {
		t.Fatal(err)
	}
	if none == focused {
		t.Fatal("focusing a wikilink should change the rendered output")
	}
	// Both still contain both link displays (ANSI-stripped).
	plain := ansiStrip(focused)
	if !strings.Contains(plain, "alpha") || !strings.Contains(plain, "beta") {
		t.Fatalf("both wikilinks should still render:\n%s", plain)
	}
}
```
> If the package already has an ANSI-strip helper in a `_test.go`, reuse it instead of `ansiStrip`; otherwise add a tiny local `ansiStrip` using `regexp.MustCompile("\x1b\\[[0-9;]*m")` in this test file.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/markdown/ -run TestWithFocusedWikilink -v`
Expected: FAIL — `WithFocusedWikilink` undefined.

- [ ] **Step 3: Add the `Focused` role**

In `internal/tui/markdown/theme/markdown.go`, add a field to the `MarkdownRoles` struct (near the existing wikilink valid/broken roles) and set it in `MarkdownRolesFor`:
```go
// WikilinkFocused styles the keyboard-focused wikilink (reverse video over the
// valid-link accent) so it stands out from the other links.
WikilinkFocused lipgloss.Style
```
and in the constructor (mirror the existing `WikilinkValid` line, adding reverse):
```go
WikilinkFocused: lipgloss.NewStyle().Foreground(p.Sem().Accent).Reverse(true),
```
> Use the real field/types as they appear in the ported file (the role bundle uses `lipgloss.Style` built from `p`/`p.Sem()`). Match the surrounding style of the valid-link role.

- [ ] **Step 4: Add the option + index plumbing in `render.go`**

In the `options` struct add `focusedWikilink int`; default it to `-1` in the options constructor/`Render` setup; add:
```go
// WithFocusedWikilink highlights the idx-th valid wikilink (0-based) as the
// keyboard-focused link. idx < 0 means no focus.
func WithFocusedWikilink(idx int) Option {
	return func(o *options) { o.focusedWikilink = idx }
}
```
Plumb `o.focusedWikilink` into the renderer struct (the `nodeRenderer` that `wikilink.go` uses), the same way `resolver` is threaded.

- [ ] **Step 5: Apply the focused style in `wikilink.go`**

In the wikilink render path, maintain a per-render counter of **valid** wikilinks; when the current valid wikilink's ordinal equals the focused index, render its display with `roles.WikilinkFocused` instead of `roles.WikilinkValid` (keep the OSC 8 wrap). Initialise the counter at render start (it lives on the renderer struct, reset per `Render` call).

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test ./internal/tui/markdown/ -run TestWithFocusedWikilink -v`
Expected: PASS. Then run the whole package: `go test ./internal/tui/markdown/` — Expected: PASS (no golden regressions; existing renders pass `focusedWikilink = -1`).

- [ ] **Step 7: Lint + commit**

Run: `golangci-lint run ./internal/tui/markdown/...`
```bash
git add internal/tui/markdown/
git commit -m "feat(m3d): renderer WithFocusedWikilink highlights the focused link"
```

---

## Task 5: Port the `markdown_overlay` component (verbatim)

**Files:**
- Create: `internal/tui/ui/markdown_overlay/{chrome.go,chrome_styles.go,code_copy.go,doc.go,keymap.go,model.go,options.go,render.go,search.go,setters.go}`
- Create (tests): `internal/tui/ui/markdown_overlay/{chrome_styles_test.go,code_copy_test.go,golden_test.go,keymap_test.go,model_test.go,setters_external_test.go}`
- Create: `internal/tui/ui/markdown_overlay/testdata/*.golden` (7 files)

**Interfaces:**
- Consumes: `internal/tui/ui/glyphs`, `internal/tui/ui/strings`, `internal/tui/theme`, `charm.land/bubbles/v2/{viewport,textinput,key}`, chroma, x/ansi, x/cellbuf.
- Produces: `markdown_overlay.New(render RenderFunc, opts ...Option) Model`; `RenderFunc func(src string, width int) string`; options `WithTitle`, `WithSource`, `WithCloseKeys(...string)`, `WithSearch()`, `WithCodeCopy()`, `WithFooterExtras(...string)`; methods `Model.Init() tea.Cmd`, `Model.Update(tea.Msg) (Model, tea.Cmd)`, `Model.View() string`, `Model.SetSize(w,h int) Model`; `ExitMsg struct{}`.

- [ ] **Step 1: Copy impl + test files with the import rewrite**

```bash
mkdir -p internal/tui/ui/markdown_overlay/testdata
for f in chrome.go chrome_styles.go code_copy.go doc.go keymap.go model.go options.go render.go search.go setters.go \
         chrome_styles_test.go code_copy_test.go golden_test.go keymap_test.go model_test.go setters_external_test.go; do
  git show main:internal/frontend/tui/components/markdown_overlay/$f \
  | sed -e 's#internal/frontend/tui/components/glyphs#internal/tui/ui/glyphs#g' \
        -e 's#internal/frontend/tui/components/strings#internal/tui/ui/strings#g' \
        -e 's#internal/frontend/tui/components/markdown_overlay#internal/tui/ui/markdown_overlay#g' \
        -e 's#internal/frontend/tui/theme#internal/tui/theme#g' \
  > internal/tui/ui/markdown_overlay/$f
done
```

- [ ] **Step 2: Copy the golden testdata verbatim (no rewrite)**

```bash
for g in code_copy_status error_display long_title_truncates narrow_width search_active_with_matches search_no_matches small_body_no_scroll; do
  git show main:internal/frontend/tui/components/markdown_overlay/testdata/$g.golden \
  > internal/tui/ui/markdown_overlay/testdata/$g.golden
done
```

- [ ] **Step 3: Build the package**

Run: `go build ./internal/tui/ui/markdown_overlay/`
Expected: clean.

- [ ] **Step 4: Run the ported tests (incl. goldens)**

Run: `go test ./internal/tui/ui/markdown_overlay/ -v`
Expected: PASS. If goldens differ and the test supports `-update`, confirm the diff is only an expected theme delta, regenerate, and eyeball before committing.

- [ ] **Step 5: Lint + commit**

Run: `golangci-lint run ./internal/tui/ui/markdown_overlay/`
```bash
git add internal/tui/ui/markdown_overlay/
git commit -m "feat(m3d): port markdown_overlay viewer component from main"
```

---

## Task 6: Overlay `Rerender()` + `CapturesInput()`

**Files:**
- Modify: `internal/tui/ui/markdown_overlay/setters.go` (add `Rerender`)
- Modify: `internal/tui/ui/markdown_overlay/model.go` (add `CapturesInput`)
- Test: `internal/tui/ui/markdown_overlay/rerender_test.go`

**Interfaces:**
- Produces: `func (m Model) Rerender() Model` (re-runs the RenderFunc at the current width and refreshes the viewport content, preserving scroll offset where possible); `func (m Model) CapturesInput() bool` (true while in `/`-search input mode). Consumed by Task 8.

- [ ] **Step 1: Write the failing test**

Create `internal/tui/ui/markdown_overlay/rerender_test.go`:
```go
package markdown_overlay

import "testing"

func TestCapturesInput_TrueOnlyInSearch(t *testing.T) {
	m := New(func(src string, w int) string { return src }, WithSource("hello"), WithSearch())
	m = m.SetSize(40, 10)
	if m.CapturesInput() {
		t.Fatal("not searching yet: CapturesInput must be false")
	}
	// '/' enters search mode (the overlay's search-launch key).
	m, _ = m.Update(keyPress("/"))
	if !m.CapturesInput() {
		t.Fatal("after '/' the overlay should capture input")
	}
}

func TestRerender_ReflectsRenderFuncChange(t *testing.T) {
	out := "first"
	m := New(func(src string, w int) string { return out }, WithSource("x"))
	m = m.SetSize(40, 10)
	if got := m.View(); !contains(got, "first") {
		t.Fatalf("expected first render:\n%s", got)
	}
	out = "second"
	m = m.Rerender()
	if got := m.View(); !contains(got, "second") {
		t.Fatalf("Rerender should pick up the new RenderFunc output:\n%s", got)
	}
}
```
> Add tiny local helpers in this test file: `keyPress(s string) tea.KeyPressMsg` building `tea.KeyPressMsg{Text: s}` (import `tea "charm.land/bubbletea/v2"`), and `contains` via `strings.Contains` on the ANSI-stripped string (reuse the package's existing strip helper if present).

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ui/markdown_overlay/ -run 'TestCapturesInput|TestRerender' -v`
Expected: FAIL — `CapturesInput`/`Rerender` undefined.

- [ ] **Step 3: Implement `CapturesInput`**

In `model.go`:
```go
// CapturesInput reports that the overlay owns the keyboard because its in-doc
// search input is active. The host (DocsModel) forwards every key while true.
func (m Model) CapturesInput() bool { return m.mode == ModeSearch }
```

- [ ] **Step 4: Implement `Rerender`**

In `setters.go`, add a method that re-runs the render closure at the current width and writes it back into the viewport, preserving the scroll offset. Reuse whatever the existing `SetSize`/`SetSource` path does to set content (call the same internal helper that re-renders + assigns `m.rendered`, `m.lines`, `m.plain`, and `m.viewport.SetContent(...)`); capture `m.viewport.YOffset()` before and restore it after (clamped). Example shape:
```go
// Rerender re-runs the RenderFunc at the current width (e.g. after the host
// changed focus state the closure reads) and refreshes the viewport, keeping
// the scroll position.
func (m Model) Rerender() Model {
	return m.SetSize(m.width, m.height)
}
```
> If `SetSize` already fully re-renders from the RenderFunc and preserves offset, `Rerender` can delegate to it as shown. If `SetSize` short-circuits when dimensions are unchanged, factor the re-render body into an unexported `reflow()` and call it from both. Verify against the ported `setters.go`.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/tui/ui/markdown_overlay/ -v`
Expected: PASS (new + all ported).

- [ ] **Step 6: Lint + commit**

Run: `golangci-lint run ./internal/tui/ui/markdown_overlay/`
```bash
git add internal/tui/ui/markdown_overlay/
git commit -m "feat(m3d): overlay Rerender() + CapturesInput() for host-driven focus"
```

---

## Task 7: `shell.FullScreener` takeover

**Files:**
- Modify: `internal/tui/shell/route.go` (add interface)
- Modify: `internal/tui/shell/shell.go` (`View` branch)
- Test: `internal/tui/shell/shell_test.go`

**Interfaces:**
- Produces: `type FullScreener interface{ FullScreen() bool }`. When the active tab's top route implements it and returns true (and no help/palette overlay is open), `Shell.View` renders only the route body over the full height, suppressing header/tabstrip/breadcrumb/footer.

- [ ] **Step 1: Write the failing test**

Add to `internal/tui/shell/shell_test.go`:
```go
type fullScreenRoute struct{ stubRoute }

func (fullScreenRoute) FullScreen() bool { return true }

func TestShell_fullScreenSuppressesChrome(t *testing.T) {
	normal := shell.New(nil, "alice", theme.Default).
		WithTabs([]shell.Route{stubRoute{title: "Docs"}})
	full := shell.New(nil, "alice", theme.Default).
		WithTabs([]shell.Route{fullScreenRoute{stubRoute{title: "Docs"}}})
	// Give both a size.
	n, _ := normal.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	f, _ := full.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	nv := n.(shell.Shell).View().Content
	fv := f.(shell.Shell).View().Content
	// The tab strip title appears in normal chrome but not in fullscreen.
	if !strings.Contains(nv, "Docs") {
		t.Fatalf("normal view should show the tabstrip:\n%s", nv)
	}
	if strings.Contains(fv, "Docs") {
		t.Fatalf("fullscreen view must suppress the tabstrip chrome:\n%s", fv)
	}
}
```
> Confirm the `stubRoute` helper and its `View(Frame) string` exist in `shell_test.go`; the embedded `fullScreenRoute` reuses it and adds `FullScreen()`. If `stubRoute.View` returns a fixed string, ensure it is not literally "Docs" so the assertion distinguishes chrome from body — if it is, give `fullScreenRoute` a `View` returning e.g. "BODY" and assert on that instead.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/shell/ -run TestShell_fullScreen -v`
Expected: FAIL — chrome still present (tabstrip shows "Docs").

- [ ] **Step 3: Add the interface**

In `internal/tui/shell/route.go`, after `InputCapturer`:
```go
// FullScreener lets the active tab's top route take over the whole terminal,
// suppressing the shell's header/tabstrip/breadcrumb/footer. Used by the Docs
// viewer for an immersive read (matches main's old ModeView takeover). Combine
// with InputCapturer so the shell also forwards every key to the route.
type FullScreener interface{ FullScreen() bool }
```

- [ ] **Step 4: Branch in `Shell.View`**

In `internal/tui/shell/shell.go` `View()`, before composing the chrome (after computing `s.width`/help/palette handling, but where the normal body path is chosen), add:
```go
	top := s.tabs[s.activeTab].Top()
	if fs, ok := top.(FullScreener); ok && fs.FullScreen() && !s.helpOpen && !s.paletteOpen {
		return tea.NewView(top.View(Frame{Width: max(s.width, 1), Height: max(s.height, 1), Pal: s.pal}))
	}
```
> Place this so it wins over the normal `head/tabs/crumbs/body/footer` assembly but not over an open help/palette overlay. Use the real field names from the existing `View` (`s.height`, `s.pal`, `Frame{...}`, `tea.NewView`). The full height is the terminal height, not `contentH`.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/tui/shell/ -v`
Expected: PASS (new + all existing shell tests).

- [ ] **Step 6: Lint + commit**

Run: `golangci-lint run ./internal/tui/shell/`
```bash
git add internal/tui/shell/route.go internal/tui/shell/shell.go internal/tui/shell/shell_test.go
git commit -m "feat(m3d): shell.FullScreener — route can take over the full screen"
```

---

## Task 8: Wire the overlay into `DocsModel` view mode

**Files:**
- Modify: `internal/tui/docs.go`
- Modify: `internal/tui/screen/docs/route.go`
- Test: `internal/tui/docs_test.go`, `internal/tui/screen/docs/route_test.go`

**Interfaces:**
- Consumes: `markdown.Render`/options + `WikilinkResolver` (T3/T4), `markdown_overlay.{New,Model,Rerender,CapturesInput,SetSize,WithSource,WithSearch,WithCodeCopy,WithCloseKeys}` (T5/T6), `shell.FullScreener` (T7), existing `domain.ResolveWikilink`, `apiclient.Backlinks`.
- Produces: `DocsModel.InViewMode() bool`; `(*docs.Route).FullScreen() bool`.

This is the heaviest task. Take it in small steps; build after each code step.

- [ ] **Step 1: Write the failing wiring tests**

Add to `internal/tui/docs_test.go`:
```go
func TestDocs_InViewModeTracksMode(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	if m.InViewMode() {
		t.Fatal("list mode: InViewMode must be false")
	}
	v, _ := m.Update(docViewMsg{doc: sampleDocs()[0]})
	if !v.(DocsModel).InViewMode() {
		t.Fatal("after docViewMsg: InViewMode must be true")
	}
}

func TestDocs_TabCyclesWikilinkFocus(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	// a doc whose body has two wikilinks resolvable within sampleDocs()
	doc := sampleDocs()[0]
	doc.Body = "see [[" + sampleDocs()[1].Path + "]] and [[" + sampleDocs()[1].Path + "]]"
	v, _ := m.Update(docViewMsg{doc: doc})
	m = v.(DocsModel)
	start := m.focusState()
	n, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if n.(DocsModel).focusState() == start {
		t.Fatal("Tab should advance the wikilink focus index")
	}
}
```
> `focusState()` is a tiny test-only accessor you add returning `m.viewer.focus` (the heap-cell value); if you prefer not to export internals, assert instead on the rendered overlay output changing after Tab. Adapt `sampleDocs()` usage to the real fixture; the key requirement is a body with ≥2 valid wikilinks. If `sampleDocs()` paths are not wikilink-resolvable, build a small inline `[]domain.Document` set and seed `m.docs` via `docsLoadedMsg` first.

Add to `internal/tui/screen/docs/route_test.go`:
```go
func TestDocsRoute_implementsFullScreenerListFalse(t *testing.T) {
	r := docs.NewRoute(nil, nil, nil, theme.Default, "alice")
	fs, ok := interface{}(r).(shell.FullScreener)
	if !ok {
		t.Fatal("docs.Route must implement shell.FullScreener")
	}
	if fs.FullScreen() {
		t.Fatal("list mode: FullScreen() must be false")
	}
}
```
> The `FullScreen()==true` case needs a `docViewMsg`, which is unexported in `package tui`; cover the true case in the `package tui` test (`TestDocs_InViewModeTracksMode`) and the live done-gate (Task 9). Here we only assert the interface is satisfied and list-mode returns false.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run 'TestDocs_InViewMode|TestDocs_TabCycles' -v` and `go test ./internal/tui/screen/docs/ -run TestDocsRoute_fullScreen -v`
Expected: FAIL — `InViewMode`/`focusState`/`FullScreen` undefined.

- [ ] **Step 3: Add the focus heap-cell + overlay fields to `DocsModel`**

In `internal/tui/docs.go`, add to the `DocsModel` struct:
```go
	viewer *viewerState        // heap cell so the RenderFunc closure sees focus updates across value-copies
	overlay markdown_overlay.Model
	overlayReady bool
```
and define the cell + accessors near the struct:
```go
type viewerState struct{ focus int } // -1 = none; index over valid wikilinks in the current doc

// InViewMode reports the docs screen is reading a document (drives shell fullscreen).
func (m DocsModel) InViewMode() bool { return m.mode == modeView }
```
Add the import: `markdown_overlay "github.com/serverkraken/flow/internal/tui/ui/markdown_overlay"` and `"github.com/serverkraken/flow/internal/tui/markdown"`.

- [ ] **Step 4: Add the wikilink adapter + render closure**

Add to `internal/tui/docs.go`:
```go
// wikiAdapter resolves [[links]] against the loaded doc set for the renderer,
// emitting flow://docs/<id> URIs so the viewer can detect in-TUI navigation.
type wikiAdapter struct {
	src  domain.Document
	all  []domain.Document
}

func (w wikiAdapter) Resolve(target string) (string, string, bool) {
	d, ok := domain.ResolveWikilink(w.src, target, w.all)
	if !ok {
		return "", "", false
	}
	return "flow://docs/" + d.ID, d.Title, true
}

// buildRenderFunc returns a RenderFunc closing over the doc + the focus cell, so
// re-rendering after a focus change highlights the right wikilink.
func (m DocsModel) buildRenderFunc(doc domain.Document, vs *viewerState) markdown_overlay.RenderFunc {
	adapter := wikiAdapter{src: doc, all: m.docs}
	fm := &markdown.Frontmatter{
		ID: doc.ID, Type: markdown.NoteType(string(doc.Type)),
		Title: doc.Title, Tags: doc.Tags,
	}
	if doc.ProjectID != nil {
		fm.Project = *doc.ProjectID
	}
	if doc.Date != nil {
		fm.Date = doc.Date.Format("2006-01-02")
	}
	bl := make([]markdown.BacklinkRef, 0, len(m.backlinks))
	for _, r := range m.backlinks {
		bl = append(bl, markdown.BacklinkRef{ID: r.ID, Title: r.Title})
	}
	return func(src string, width int) string {
		out, err := markdown.Render(src, width,
			markdown.WithWikilinks(adapter),
			markdown.WithFrontmatter(fm),
			markdown.WithBacklinks(bl),
			markdown.WithFocusedWikilink(vs.focus),
		)
		if err != nil {
			return src
		}
		return out
	}
}
```
> `markdown.NoteType("agent")` has no badge constant; the frontmatter card tolerates unknown types (renders without a badge) — acceptable. Confirm `Frontmatter` field names against the ported `types.go`.

- [ ] **Step 5: Initialise the overlay on `docViewMsg`**

In `Update`'s `docViewMsg` case (after `m.viewing = &d; m.mode = modeView`), add:
```go
	m.viewer = &viewerState{focus: -1}
	m.overlay = markdown_overlay.New(m.buildRenderFunc(d, m.viewer),
		markdown_overlay.WithSource(d.Body),
		markdown_overlay.WithSearch(),
		markdown_overlay.WithCodeCopy(),
		markdown_overlay.WithCloseKeys(), // DocsModel owns Esc/leaving
	)
	m.overlayReady = true
```
Keep the existing `m.loadBacklinks(d.ID)` return. In the `backlinksMsg` case, after setting `m.backlinks`, rebuild the render closure so the footer appears: if `m.overlayReady && m.viewing != nil { m.overlay = markdown_overlay.New(m.buildRenderFunc(*m.viewing, m.viewer), markdown_overlay.WithSource(m.viewing.Body), markdown_overlay.WithSearch(), markdown_overlay.WithCodeCopy(), markdown_overlay.WithCloseKeys()) }` then keep the existing viewLinks merge.

- [ ] **Step 6: Route keys in `modeView` (handleKey)**

Replace the `modeView` key handling in `handleKey` so DocsModel owns nav and forwards the rest. Find the `case modeView:` block (currently Esc/q/Tab/Enter/e link logic) and make it:
```go
	case modeView:
		if m.overlayReady && m.overlay.CapturesInput() {
			var cmd tea.Cmd
			m.overlay, cmd = m.overlay.Update(k)
			return m, cmd
		}
		switch {
		case k.Code == tea.KeyEsc:
			// existing pop-viewStack / leave-to-list logic stays here
			...
		case k.Code == tea.KeyTab && k.Mod == tea.ModShift:
			m.cycleWikiFocus(-1)
			return m, nil
		case k.Code == tea.KeyTab:
			m.cycleWikiFocus(+1)
			return m, nil
		case k.Code == tea.KeyEnter:
			return m.followFocusedWikilink()
		case k.Text == "e":
			if m.viewing == nil { return m, nil }
			return m, m.buildEditorCmd(m.viewing.ID)
		default:
			var cmd tea.Cmd
			m.overlay, cmd = m.overlay.Update(k)
			return m, cmd
		}
```
> Preserve the existing Esc body verbatim (viewStack pop → `loadDocNoPush`; empty stack → leave to list and clear `viewing`/`overlayReady`). The old `q`-quit in view mode is dropped (Esc leaves; `q` now scrolls/搜索 via the overlay only if bound — otherwise it falls through harmlessly).

- [ ] **Step 7: Add focus-cycle + follow helpers**

```go
// validWikiTargets returns the resolvable wikilink doc-ids in body order.
func (m DocsModel) validWikiTargets() []string {
	var ids []string
	if m.viewing == nil { return ids }
	for _, sp := range domain.FindWikilinks(m.viewing.Body) {
		if d, ok := domain.ResolveWikilink(*m.viewing, sp.Target, m.docs); ok {
			ids = append(ids, d.ID)
		}
	}
	return ids
}

func (m *DocsModel) cycleWikiFocus(delta int) {
	n := len(m.validWikiTargets())
	if n == 0 || m.viewer == nil { return }
	m.viewer.focus = (m.viewer.focus + delta + n) % n
	m.overlay = m.overlay.Rerender()
}

func (m DocsModel) followFocusedWikilink() (tea.Model, tea.Cmd) {
	ids := m.validWikiTargets()
	if m.viewer == nil || m.viewer.focus < 0 || m.viewer.focus >= len(ids) {
		return m, nil
	}
	if m.viewing != nil {
		m.viewStack = append(m.viewStack, m.viewing.ID)
	}
	return m, m.loadDoc(ids[m.viewer.focus], false)
}
```
> `domain.WikilinkSpan` exposes the link target as `.Target` (confirm the field name in `internal/domain/wikilink.go`); adapt if it differs. `cycleWikiFocus` has a pointer receiver — call sites in Step 6 use `m.cycleWikiFocus(...)` on the value `m`, which is addressable there. Add `focusState() int { return m.viewer.focus }` (test-only, guard nil) if the Step-1 test uses it.

- [ ] **Step 8: Render the overlay in `View` + size bridge**

In `DocsModel.View()`'s `modeView` case, replace `m.renderView(&b)` with the overlay body when ready:
```go
	case modeView:
		if m.overlayReady {
			b.WriteString(m.overlay.View())
		} else {
			m.renderView(&b)
		}
```
The overlay needs a size. Add a value-receiver helper and call it from the docs route adapter (the route is Frame-driven):
```go
// SetViewport sizes the in-view overlay from the host frame.
func (m DocsModel) SetViewport(w, h int) DocsModel {
	if m.overlayReady {
		m.overlay = m.overlay.SetSize(w, h)
	}
	return m
}
```
> Note: the legacy `DocsModel.View()` sets `v.AltScreen = true`; in `flow ui` the adapter discards everything but `.Content`, so the shell controls AltScreen — leave the line as-is (harmless; standalone `flow docs` still benefits).

- [ ] **Step 9: Update the docs route adapter**

In `internal/tui/screen/docs/route.go`:
```go
// FullScreen reports fullscreen while the docs screen is reading a document, so
// the shell suppresses its chrome. Implements shell.FullScreener.
func (r *Route) FullScreen() bool { return r.m.InViewMode() }
```
and bridge the frame size into the model in `View`:
```go
func (r *Route) View(f shell.Frame) string {
	r.m = r.m.SetViewport(f.Width, f.Height)
	return r.m.View().Content
}
```
> `View` now mutates `r.m` (pointer receiver already) to size the overlay before rendering — this is the Frame→SetSize bridge the spec requires (no `WindowSizeMsg`).

- [ ] **Step 10: Build, run tests**

Run: `go build ./...`
Run: `go test ./internal/tui/ ./internal/tui/screen/docs/ -v`
Expected: PASS. Fix compile/wiring issues until green.

- [ ] **Step 11: Lint + commit**

Run: `golangci-lint run ./internal/tui/ ./internal/tui/screen/docs/`
```bash
git add internal/tui/docs.go internal/tui/docs_test.go internal/tui/screen/docs/route.go internal/tui/screen/docs/route_test.go
git commit -m "feat(m3d): fullscreen markdown viewer in Docs (overlay + keyboard wikilink focus)"
```

---

## Task 9: Done-gate — full CI + live verification

**Files:** none (verification only).

- [ ] **Step 1: Full CI gate**

Run: `make ci`
Expected: lint + verify-generate + build + tests green; coverage ≥ 80 %. If coverage dipped, add focused render/reducer tests to the thinnest new surface (overlay `Rerender`, the focus helpers) until the gate passes — do not lower the threshold.

- [ ] **Step 2: Live done-gate against the dev stack**

Start the dev stack ([[reference_flow_dev_env]]): `make dev-up && make dev-run`, ensure a token. Create a doc with headings, a fenced code block, a GFM table, a callout, two `[[wikilinks]]` (one valid, one broken), and a bare URL. Then in `flow ui` → Docs → open it and verify:

- [ ] Opens **fullscreen** — no shell header/tabstrip/breadcrumb/footer around the document.
- [ ] Headings, code (syntax-highlighted), table, callout, task lists render richly.
- [ ] Frontmatter card (type/title/tags) on top; "↩ Referenced by" footer if backlinks exist.
- [ ] `Tab`/`⇧Tab` cycles the wikilinks with a **visible focus highlight** and scrolls the focused link into view; `Enter` drills into the valid target (and `Esc` returns to it); the broken link shows `⊘`.
- [ ] Weblink is an OSC-8 clickable hyperlink.
- [ ] `/` searches within the document (gutter match bars, `n`/`N` cycle); `c` copies the next code block; `j/k/g/G/ctrl-d/u` scroll.
- [ ] `Esc` at the root leaves view mode → back to the Docs list with the shell chrome restored.
- [ ] Standalone `flow docs` still opens and reads documents (regression check).

- [ ] **Step 3: Final commit (only if coverage top-up tests were added)**

```bash
git add -A -- internal/
git commit -m "test(m3d): coverage top-up for the markdown viewer"
```

---

## Notes / accepted scope boundaries

- Only the document **view** is fullscreen; the Docs **list** keeps its current in-tab chrome (separate cleanup).
- Weblinks remain OSC-8 click-only (as in old Kompendium); keyboard focus/follow is for wikilinks.
- The rest of M3d (Stats/Heatmap, DayOffs, Export, Projects as routes) is out of scope.
- `DocsModel.Init` still opens its own SSE stream in addition to the shell broadcast (pre-existing; unifying is out of scope).
