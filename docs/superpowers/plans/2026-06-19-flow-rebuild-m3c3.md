# flow rebuild M3c3 — Home Dashboard + Docs Tab + 3-Tab Shell Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the placeholder Home with a live, responsive dashboard; wrap the existing `DocsModel` as a shell Route; wire `flow ui` to three tabs (Home · Worktime · Docs) with a `flow ui [tab]` deep-link.

**Architecture:** Two small shell navigation primitives first (`SwitchTabMsg{Title}` for in-app drill, `WithActiveTab(i)` for the deep-link start tab). Then a Frame-driven dashboard `HomeRoute` (in `package shell`, rewriting the placeholder) that loads `GetToday`/`GetWeek`/`ListDocuments`/`ListProjects` and renders two responsive columns, reloading on SSE `EventMsg`. Then a thin `screen/docs` adapter wrapping the unchanged `tui.DocsModel` (`View() tea.View` → `View(Frame) string` via `tea.View.Content`). Finally the composition root wires 3 tabs + the deep-link.

**Tech Stack:** Go, charm.land/bubbletea/v2 + lipgloss/v2, existing `internal/tui/{shell,theme,ui}` design system, `internal/adapter/apiclient`, the legacy `internal/tui` monolith (for `DocsModel`).

**Spec:** `docs/superpowers/specs/2026-06-16-flow-rebuild-m3c-home-worktime-design.md` (M3c3 row + Home-Dashboard / Docs-als-Route / NavHost sections).

## Global Constraints

- charm.land v2 import paths only (`charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`).
- Key presses are `tea.KeyPressMsg` (`.Code`, `.Text`, `.Mod`); routes are **Frame-driven** — read size from `View(f shell.Frame)` via `f.Width`/`f.Height`, never store it from `tea.WindowSizeMsg`.
- `shell.Route` contract: `Title() string · Init() tea.Cmd · Update(tea.Msg) (Route, tea.Cmd) · View(Frame) string · KeyHints() []keyhint.Hint`. Navigation via `PushRouteMsg`/`PopRouteMsg`/`SwitchRouteMsg`/`tabSwitchMsg`.
- The shell broadcasts `shell.EventMsg{Ev apiclient.ClientEvent}` (with `Ev.Type string`) to every tab's top route on each SSE event; domain event-type constants live in `internal/domain` (`EventSessionStarted/Stopped/Updated/Deleted`, `EventDayOffChanged`, `EventSettingsChanged`, `EventDocument*`).
- Data types (verified): `apiclient.Today{Date string, LoggedMin, TargetMin, SaldoMin int, Running bool}`; `apiclient.WeekDay{Date string, LoggedMin, TargetMin int, IsToday, Workday bool}`; `domain.Document{ID, Path, Title string, UpdatedAt time.Time, …}` (slug is `Path`, there is NO `Slug`); `domain.Project{Name, Slug string, …}`.
- `tea.View` is a struct with field `Content string` (set by `tea.NewView`); read `.Content` to get a model's rendered body.
- `make ci` must stay green: lint (`golangci-lint`, watch SA4006), `verify-generate`, coverage ≥ 80%, build. The pre-existing `.gitignore`/`flow` working-tree changes must never be staged.
- `DocsModel` stays functionally unchanged (no edits to `internal/tui/docs.go`); the Markdown-viewer upgrade is M3d. The wrapped Docs renders its own header/footer inside the shell content area — a known M3c3 cosmetic (double chrome), accepted and deferred to M3d.

## File Structure

**Modify (shell primitives):**
- `internal/tui/shell/route.go` — add `SwitchTabMsg{Title string}`.
- `internal/tui/shell/shell.go` — handle `SwitchTabMsg`; add `WithActiveTab(i int) Shell`.
- `internal/tui/shell/navstack.go` — add `Root() Route` if absent.
- `internal/tui/shell/shell_test.go` — tests for the primitives.

**Rewrite (Home dashboard, in place):**
- `internal/tui/shell/home.go` — placeholder → live dashboard `HomeRoute`.
- `internal/tui/shell/home_test.go` — dashboard tests (replaces placeholder test).
- Delete: `internal/tui/shell/about.go` + `internal/tui/shell/about_test.go` (orphaned once Home stops drilling into AboutRoute).

**New (Docs adapter):**
- `internal/tui/screen/docs/route.go` — `Route` wrapping `tui.DocsModel`.
- `internal/tui/screen/docs/route_test.go`.

**Composition root:**
- `cmd/flow/ui.go` — 3 tabs + `flow ui [tab]` deep-link.

**Dependency direction:** `shell` already imports `apiclient`; the dashboard adds `domain` + `lipgloss`. `screen/docs` imports `shell` + the `tui` monolith + `adapter/editor` + `adapter/opener` (same as `cmd/flow/docs.go`). `cmd/flow` imports `shell`, `screen/worktime`, `screen/docs`. No new cycles (the `tui` monolith imports neither `shell` nor `screen/*`).

---

## Task 1: Shell nav primitives — `SwitchTabMsg{Title}` + `WithActiveTab(i)`

**Files:**
- Modify: `internal/tui/shell/route.go` (add message near `SwitchRouteMsg`)
- Modify: `internal/tui/shell/shell.go` (`Update` case + `WithActiveTab`)
- Modify: `internal/tui/shell/navstack.go` (add `Root()` if missing)
- Test: `internal/tui/shell/shell_test.go`

**Interfaces:**
- Produces: `type SwitchTabMsg struct{ Title string }` — emit as a cmd from a Route to jump to the tab whose ROOT route title equals `Title` (no-op if none). `func (s Shell) WithActiveTab(i int) Shell` — sets the initial active tab (clamped to range), so `flow ui [tab]` can start on Worktime/Docs.

- [ ] **Step 1: Write the failing test**

Add to `internal/tui/shell/shell_test.go`:

```go
func TestShell_switchTabByTitle(t *testing.T) {
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{
		stubRoute{title: "Home"},
		stubRoute{title: "Worktime"},
		stubRoute{title: "Docs"},
	})
	next, _ := s.Update(shell.SwitchTabMsg{Title: "Docs"})
	if next.(shell.Shell).ActiveTab() != 2 {
		t.Fatalf("SwitchTabMsg{Docs} should activate tab 2, got %d", next.(shell.Shell).ActiveTab())
	}
	// Unknown title is a no-op (stays put).
	again, _ := next.(shell.Shell).Update(shell.SwitchTabMsg{Title: "Nope"})
	if again.(shell.Shell).ActiveTab() != 2 {
		t.Fatalf("unknown SwitchTabMsg should be a no-op, got %d", again.(shell.Shell).ActiveTab())
	}
}

func TestShell_withActiveTabStartsThere(t *testing.T) {
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{
		stubRoute{title: "Home"},
		stubRoute{title: "Worktime"},
		stubRoute{title: "Docs"},
	}).WithActiveTab(1)
	if s.ActiveTab() != 1 {
		t.Fatalf("WithActiveTab(1) => ActiveTab %d, want 1", s.ActiveTab())
	}
	// Out of range clamps to 0.
	if shell.New(nil, "a", theme.Default).WithActiveTab(9).ActiveTab() != 0 {
		t.Fatal("WithActiveTab(out-of-range) should clamp to 0")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/shell/ -run 'TestShell_(switchTabByTitle|withActiveTab)' -v`
Expected: FAIL — `SwitchTabMsg` / `WithActiveTab` undefined.

- [ ] **Step 3: Add `SwitchTabMsg`**

In `internal/tui/shell/route.go`, after `SwitchRouteMsg`:

```go
// SwitchTabMsg asks the Shell to activate the tab whose nav-stack ROOT route
// title equals Title (a no-op if no tab matches). Emit it from a Route to drill
// laterally into another tab (e.g. the Home dashboard jumping to Worktime).
type SwitchTabMsg struct{ Title string }
```

- [ ] **Step 4: Add `Root()` to NavStack if absent**

Read `internal/tui/shell/navstack.go`. If there is no accessor for the bottom (root) route, add:

```go
// Root returns the bottom (tab-root) route of the stack.
func (n *NavStack) Root() Route { return n.stack[0] }
```

(Use the actual backing-slice field name. The stack always has ≥1 route, so `stack[0]` is safe.)

- [ ] **Step 5: Handle `SwitchTabMsg` + add `WithActiveTab`**

In `internal/tui/shell/shell.go` `Update`, after the `tabSwitchMsg` case:

```go
	case SwitchTabMsg:
		for i, ns := range s.tabs {
			if ns.Root().Title() == msg.Title {
				return s, s.switchTo(i)
			}
		}
		return s, nil
```

Add the accessor near `WithTabs`:

```go
// WithActiveTab sets the initially-visible tab (clamped to range). Used by the
// `flow ui [tab]` deep-link to start on Worktime/Docs instead of Home.
func (s Shell) WithActiveTab(i int) Shell {
	if i < 0 || i >= len(s.tabs) {
		i = 0
	}
	s.activeTab = i
	return s
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/tui/shell/ -v`
Expected: PASS (all shell tests).

- [ ] **Step 7: Commit**

```bash
git add internal/tui/shell/route.go internal/tui/shell/shell.go internal/tui/shell/navstack.go internal/tui/shell/shell_test.go
git commit -m "feat(m3c3): shell SwitchTabMsg{Title} + WithActiveTab(i)"
```

---

## Task 2: Home dashboard route

**Files:**
- Rewrite: `internal/tui/shell/home.go`
- Rewrite: `internal/tui/shell/home_test.go`
- Delete: `internal/tui/shell/about.go`, `internal/tui/shell/about_test.go`
- Modify: `cmd/flow/ui.go` (update the `NewHomeRoute` call to the new signature — full 3-tab wiring is Task 4)

**Interfaces:**
- Consumes: `SwitchTabMsg` (Task 1); `apiclient.Today/WeekDay`, `domain.Document/Project`; `EventMsg` (existing).
- Produces: `func NewHomeRoute(api DashboardAPI, pal theme.Palette, user string) HomeRoute` where `DashboardAPI` is the narrow interface below. `*apiclient.Client` satisfies `DashboardAPI`.

The dashboard loads four endpoints, renders two responsive columns (side-by-side when `f.Width >= 80`, else stacked), reloads on relevant SSE events, and maps `w`/`d` to a tab drill via `SwitchTabMsg`.

- [ ] **Step 1: Write the failing test**

Replace the contents of `internal/tui/shell/home_test.go`:

```go
package shell_test

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

type fakeDash struct{}

func (fakeDash) GetToday(context.Context) (apiclient.Today, error) {
	return apiclient.Today{Date: "2026-06-18", LoggedMin: 300, TargetMin: 480, Running: true}, nil
}
func (fakeDash) GetWeek(context.Context, string) ([]apiclient.WeekDay, error) {
	return []apiclient.WeekDay{{Date: "2026-06-18", LoggedMin: 300, TargetMin: 480, IsToday: true, Workday: true}}, nil
}
func (fakeDash) ListDocuments(context.Context, ...string) ([]domain.Document, error) {
	return []domain.Document{{ID: "d1", Path: "notes/x", Title: "Mein Dokument", UpdatedAt: time.Now()}}, nil
}
func (fakeDash) ListProjects(context.Context) ([]domain.Project, error) {
	return []domain.Project{{ID: "p1", Name: "ProjektA"}}, nil
}

func drainHome(r shell.Route, cmd tea.Cmd) shell.Route {
	for i := 0; cmd != nil && i < 20; i++ {
		msg := cmd()
		if msg == nil {
			break
		}
		r, cmd = r.Update(msg)
	}
	return r
}

func TestHomeRoute_rendersDashboard(t *testing.T) {
	var r shell.Route = shell.NewHomeRoute(fakeDash{}, theme.Default, "alice")
	r = drainHome(r, r.Init())
	body := r.View(shell.Frame{Width: 100, Height: 30, Pal: theme.Default})
	for _, want := range []string{"Arbeit", "Wissen", "Mein Dokument", "ProjektA"} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard missing %q:\n%s", want, body)
		}
	}
	if r.Title() != "Home" {
		t.Fatalf("title = %q, want Home", r.Title())
	}
}

func TestHomeRoute_wDrillsToWorktime(t *testing.T) {
	r := shell.NewHomeRoute(fakeDash{}, theme.Default, "alice")
	_, cmd := r.Update(tea.KeyPressMsg{Text: "w"})
	if cmd == nil {
		t.Fatal("w should emit a drill cmd")
	}
	msg, ok := cmd().(shell.SwitchTabMsg)
	if !ok || msg.Title != "Worktime" {
		t.Fatalf("w should emit SwitchTabMsg{Worktime}, got %#v", cmd())
	}
}

func TestHomeRoute_reloadsOnSessionEvent(t *testing.T) {
	r := shell.NewHomeRoute(fakeDash{}, theme.Default, "alice")
	_, cmd := r.Update(shell.EventMsg{Ev: apiclient.ClientEvent{Type: string(domain.EventSessionStarted)}})
	if cmd == nil {
		t.Fatal("a session event should trigger a reload cmd")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/shell/ -run TestHomeRoute -v`
Expected: FAIL — `NewHomeRoute` arity/shape mismatch.

- [ ] **Step 3: Rewrite `home.go`**

Replace `internal/tui/shell/home.go` entirely:

```go
package shell

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
	"github.com/serverkraken/flow/internal/tui/ui/statusbar"
)

// DashboardAPI is the narrow client surface the Home dashboard needs.
// *apiclient.Client satisfies it.
type DashboardAPI interface {
	GetToday(ctx context.Context) (apiclient.Today, error)
	GetWeek(ctx context.Context, ref string) ([]apiclient.WeekDay, error)
	ListDocuments(ctx context.Context, tags ...string) ([]domain.Document, error)
	ListProjects(ctx context.Context) ([]domain.Project, error)
}

type homeLoadedMsg struct {
	today    apiclient.Today
	week     []apiclient.WeekDay
	docs     []domain.Document
	projects []domain.Project
	err      error
}

// HomeRoute is the live dashboard: left column "Arbeit" (running session +
// Tagesziel bar + week pace), right column "Wissen" (recent docs + projects).
// Read-only; w/d drill into the Worktime/Docs tabs. Reloads on SSE events.
type HomeRoute struct {
	api      DashboardAPI
	pal      theme.Palette
	user     string
	today    apiclient.Today
	week     []apiclient.WeekDay
	docs     []domain.Document
	projects []domain.Project
	loaded   bool
	err      error
}

// NewHomeRoute builds the dashboard. api may be nil only in tests that never load.
func NewHomeRoute(api DashboardAPI, pal theme.Palette, user string) HomeRoute {
	return HomeRoute{api: api, pal: pal, user: user}
}

func (h HomeRoute) Title() string { return "Home" }

func (h HomeRoute) Init() tea.Cmd { return h.loadCmd() }

func (h HomeRoute) loadCmd() tea.Cmd {
	api := h.api
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		today, err := api.GetToday(ctx)
		if err != nil {
			return homeLoadedMsg{err: err}
		}
		week, _ := api.GetWeek(ctx, "")
		docs, _ := api.ListDocuments(ctx)
		projects, _ := api.ListProjects(ctx)
		return homeLoadedMsg{today: today, week: week, docs: docs, projects: projects}
	}
}

func (h HomeRoute) Update(msg tea.Msg) (Route, tea.Cmd) {
	switch m := msg.(type) {
	case homeLoadedMsg:
		h.loaded, h.err = true, m.err
		if m.err == nil {
			h.today, h.week, h.docs, h.projects = m.today, m.week, m.docs, m.projects
		}
		return h, nil
	case EventMsg:
		if homeRelevantEvent(m.Ev.Type) {
			return h, h.loadCmd()
		}
		return h, nil
	case tea.KeyPressMsg:
		switch m.Text {
		case "w":
			return h, func() tea.Msg { return SwitchTabMsg{Title: "Worktime"} }
		case "d":
			return h, func() tea.Msg { return SwitchTabMsg{Title: "Docs"} }
		}
	}
	return h, nil
}

func (h HomeRoute) View(f Frame) string {
	if !h.loaded {
		return theme.Dim("  Dashboard lädt …", f.Pal)
	}
	if h.err != nil {
		return theme.Dim("  Fehler: "+h.err.Error(), f.Pal)
	}
	left := h.workColumn(f.Pal)
	right := h.knowledgeColumn(f.Pal)
	if f.Width >= 80 {
		colW := f.Width/2 - 2
		l := lipgloss.NewStyle().Width(colW).Render(left)
		r := lipgloss.NewStyle().Width(colW).Render(right)
		return "\n" + lipgloss.JoinHorizontal(lipgloss.Top, "  ", l, "  ", r)
	}
	return "\n" + left + "\n\n" + right
}

func (h HomeRoute) workColumn(pal theme.Palette) string {
	var b strings.Builder
	b.WriteString(theme.Heading("Arbeit", pal) + "\n\n")
	state := theme.Dim("○ gestoppt", pal)
	if h.today.Running {
		state = theme.Active("● läuft", pal)
	}
	b.WriteString("  " + state + "\n")
	b.WriteString(fmt.Sprintf("  Heute: %s / %s\n", fmtMin(h.today.LoggedMin), fmtMin(h.today.TargetMin)))
	pct := 0
	if h.today.TargetMin > 0 {
		pct = h.today.LoggedMin * 100 / h.today.TargetMin
	}
	b.WriteString("  " + statusbar.Bar(pct, 16, pal) + "\n\n")
	b.WriteString(theme.Dim("  Woche", pal) + "\n")
	for _, d := range h.week {
		marker := "  "
		if d.IsToday {
			marker = theme.Active("▶ ", pal)
		}
		b.WriteString(fmt.Sprintf("  %s%s  %s / %s\n", marker, d.Date, fmtMin(d.LoggedMin), fmtMin(d.TargetMin)))
	}
	return b.String()
}

func (h HomeRoute) knowledgeColumn(pal theme.Palette) string {
	var b strings.Builder
	b.WriteString(theme.Heading("Wissen", pal) + "\n\n")
	b.WriteString(theme.Dim("  Zuletzt bearbeitet", pal) + "\n")
	recent := append([]domain.Document(nil), h.docs...)
	sort.Slice(recent, func(i, j int) bool { return recent[i].UpdatedAt.After(recent[j].UpdatedAt) })
	if len(recent) > 5 {
		recent = recent[:5]
	}
	if len(recent) == 0 {
		b.WriteString(theme.Dim("  keine Dokumente", pal) + "\n")
	}
	for _, d := range recent {
		title := d.Title
		if title == "" {
			title = d.Path
		}
		b.WriteString("  • " + title + theme.Dim("  ("+d.Path+")", pal) + "\n")
	}
	b.WriteString("\n" + theme.Dim("  Projekte", pal) + "\n")
	if len(h.projects) == 0 {
		b.WriteString(theme.Dim("  keine Projekte", pal) + "\n")
	}
	for _, p := range h.projects {
		b.WriteString("  • " + p.Name + "\n")
	}
	return b.String()
}

func (h HomeRoute) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "w", Desc: "Worktime"},
		{Key: "d", Desc: "Docs"},
		{Key: "tab", Desc: "Tab"},
		{Key: ":", Desc: "Palette"},
		{Key: "?", Desc: "Hilfe"},
	}
}

// fmtMin renders a non-negative minute count as "Xh YYm" (local to avoid
// coupling the shell to a worktime sub-package).
func fmtMin(min int) string {
	if min < 0 {
		min = 0
	}
	return fmt.Sprintf("%dh %02dm", min/60, min%60)
}

func homeRelevantEvent(t string) bool {
	switch domain.EventType(t) {
	case domain.EventSessionStarted, domain.EventSessionStopped,
		domain.EventSessionUpdated, domain.EventSessionDeleted,
		domain.EventDayOffChanged, domain.EventSettingsChanged:
		return true
	}
	// Any document.* event also refreshes the knowledge column.
	return strings.HasPrefix(t, "document.")
}
```

> Verify the `domain.Event*` constant names against `internal/domain` (the worktime/dayoffs routes use the session/dayoff/settings ones; confirm the document event constant prefix is `document.`). If `theme.Heading`/`theme.Body` differ, use the real builders (`internal/tui/theme/builders.go`).

- [ ] **Step 4: Delete the orphaned AboutRoute**

```bash
git rm internal/tui/shell/about.go internal/tui/shell/about_test.go
```

Confirm nothing else references `NewAboutRoute`/`AboutRoute`: `rg -n 'AboutRoute' internal/ cmd/` must return nothing after the delete.

- [ ] **Step 5: Keep the build green — update `cmd/flow/ui.go` call site**

In `cmd/flow/ui.go` `runUI`, change the Home construction to the new signature (the third tab + deep-link arrive in Task 4):

```go
	m := shell.New(client, os.Getenv("USER"), pal).
		WithTabs([]shell.Route{
			shell.NewHomeRoute(client, pal, os.Getenv("USER")),
			worktime.NewTodayRoute(client, time.Now, pal, worktime.BuildRegistry(client, pal)),
		})
```

Also update any other `shell.NewHomeRoute(...)` call site the compiler flags (e.g. `newShell` in `shell_test.go` → `shell.NewHomeRoute(nil, theme.Default, "alice")`).

- [ ] **Step 6: Run tests + build**

Run: `go test ./internal/tui/shell/ -v` then `go build ./...`
Expected: PASS; build clean (no `AboutRoute`/arity errors).

- [ ] **Step 7: Lint + commit**

Run: `golangci-lint run ./internal/tui/shell/ ./cmd/flow/`
Expected: 0 issues.

```bash
git add internal/tui/shell/home.go internal/tui/shell/home_test.go internal/tui/shell/about.go internal/tui/shell/about_test.go internal/tui/shell/shell_test.go cmd/flow/ui.go
git commit -m "feat(m3c3): Home dashboard (responsive Arbeit|Wissen, SSE reload, w/d drill)"
```

> `shell_test.go` is staged because `newShell` (Step 5) now passes the new `NewHomeRoute` signature. `git add` of the deleted `about*` files records the deletion alongside `git rm`. Stage only these files.

---

## Task 3: Docs-as-route adapter

**Files:**
- Create: `internal/tui/screen/docs/route.go`
- Test: `internal/tui/screen/docs/route_test.go`

**Interfaces:**
- Consumes: `shell.Route`/`shell.Frame`; `tui.DocsModel` (`NewDocs(client *apiclient.Client, ed docEditor, op urlOpener, user string) DocsModel`, value-receiver `Init`/`Update`/`View() tea.View`); `tea.View.Content`.
- Produces: `func NewRoute(client *apiclient.Client, ed Editor, op Opener, pal theme.Palette, user string) *Route` — a `shell.Route` wrapping `DocsModel`. `Editor`/`Opener` are re-declared local interfaces matching what `editor.New()`/`opener.New()` return (so `cmd/flow` can pass them).

The adapter is a thin wrapper: forward `Init`/`Update` to `DocsModel`, render `DocsModel.View().Content` from `View(Frame)`, and expose the list-mode keys as `KeyHints`. `DocsModel` is width-agnostic and self-subscribes to SSE in its `Init`, so the adapter needs no size sync (and the shell's `EventMsg` broadcast is harmlessly ignored by `DocsModel.Update`).

- [ ] **Step 1: Write the failing test**

`internal/tui/screen/docs/route_test.go`:

```go
package docs_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/screen/docs"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

func TestDocsRoute_titleAndRenders(t *testing.T) {
	// client nil + nil editor/opener: DocsModel renders its list chrome without
	// touching the network until Init's cmd runs (which we don't drain here).
	var r shell.Route = docs.NewRoute(nil, nil, nil, theme.Default, "alice")
	if r.Title() != "Docs" {
		t.Fatalf("title = %q, want Docs", r.Title())
	}
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "docs") { // DocsModel renders a "flow · docs" header
		t.Fatalf("docs body should contain the docs header:\n%s", body)
	}
	if len(r.KeyHints()) == 0 {
		t.Fatal("docs route should expose key hints")
	}
}

func TestDocsRoute_updateReturnsRoute(t *testing.T) {
	var r shell.Route = docs.NewRoute(nil, nil, nil, theme.Default, "alice")
	r2, _ := r.Update(tea.KeyPressMsg{Text: "j"})
	if r2 == nil {
		t.Fatal("Update must return a Route")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/docs/`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write the adapter**

`internal/tui/screen/docs/route.go`:

```go
// Package docs wraps the legacy tui.DocsModel as a shell.Route so the compendium
// screen can live as a tab in `flow ui`. It is a thin adapter — DocsModel is
// unchanged (the Markdown-viewer upgrade is M3d). DocsModel renders its own
// header/footer inside the shell content area (a known M3c3 cosmetic).
package docs

import (
	"os/exec"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/tui"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// Editor is the editor adapter DocsModel needs (editor.New() satisfies it).
type Editor interface {
	Command(initial []byte) (*exec.Cmd, func() ([]byte, error), func(), error)
}

// Opener opens a URL in the OS browser (opener.New() satisfies it).
type Opener interface {
	Open(url string) error
}

// Route hosts a DocsModel under the shell Route contract.
type Route struct {
	m   tui.DocsModel
	pal theme.Palette
}

// NewRoute builds the Docs route. ed/op may be nil in tests (the $EDITOR/open
// paths are never hit there).
func NewRoute(client *apiclient.Client, ed Editor, op Opener, pal theme.Palette, user string) *Route {
	return &Route{m: tui.NewDocs(client, ed, op, user), pal: pal}
}

func (r *Route) Title() string  { return "Docs" }
func (r *Route) Init() tea.Cmd  { return r.m.Init() }

func (r *Route) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	nm, cmd := r.m.Update(msg)
	r.m = nm.(tui.DocsModel)
	return r, cmd
}

func (r *Route) View(shell.Frame) string { return r.m.View().Content }

func (r *Route) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "j/k", Desc: "wählen"},
		{Key: "enter", Desc: "öffnen"},
		{Key: "n", Desc: "neu"},
		{Key: "e", Desc: "edit"},
		{Key: "/", Desc: "suchen"},
		{Key: "f", Desc: "Filter"},
	}
}
```

> Add the missing `"os/exec"` import for the `Editor` interface signature. Verify `tui.NewDocs`'s parameter interface names: the params are unexported (`docEditor`/`urlOpener`) in `package tui`, but Go lets an external caller pass any value that structurally implements them — so passing your `Editor`/`Opener` values (or nil) compiles, exactly as `cmd/flow/docs.go` passes `editor.New()`/`opener.New()` today. If the compiler rejects the nil-typed args, pass the concrete `editor.New()`/`opener.New()` in production and a tiny local stub in tests instead.

- [ ] **Step 4: Run test to verify it passes + no cycle**

Run: `go test ./internal/tui/screen/docs/ -v`
Expected: PASS.
Run: `go build ./...` and `go list -deps github.com/serverkraken/flow/internal/tui/screen/docs | grep -E 'tui/shell$|internal/tui$'`
Expected: clean build; deps include `internal/tui` + `internal/tui/shell` but NO cycle (build succeeding proves acyclic).

- [ ] **Step 5: Lint + commit**

Run: `golangci-lint run ./internal/tui/screen/docs/`
Expected: 0 issues.

```bash
git add internal/tui/screen/docs/
git commit -m "feat(m3c3): Docs-as-route adapter (wraps tui.DocsModel under Route)"
```

---

## Task 4: Wire 3-tab shell + `flow ui [tab]` deep-link

**Files:**
- Modify: `cmd/flow/ui.go`

**Interfaces:**
- Consumes: `shell.NewHomeRoute` (Task 2), `worktime.NewTodayRoute`, `docs.NewRoute` (Task 3), `shell.WithActiveTab` (Task 1).

- [ ] **Step 1: Update `uiCmd` + `runUI`**

Replace `cmd/flow/ui.go` with the 3-tab wiring + deep-link (imports: add `"github.com/serverkraken/flow/internal/adapter/editor"`, `"github.com/serverkraken/flow/internal/adapter/opener"`, `docsscreen "github.com/serverkraken/flow/internal/tui/screen/docs"`):

```go
func uiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ui [tab]",
		Short: "Sidekick-Shell (TUI) — Home · Worktime · Docs in einem Programm",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runUI,
	}
}

// tabIndexForArg maps an optional positional arg to a start-tab index
// (0=Home, 1=Worktime, 2=Docs); unknown/empty → Home.
func tabIndexForArg(args []string) int {
	if len(args) == 0 {
		return 0
	}
	switch args[0] {
	case "worktime", "work", "w":
		return 1
	case "docs", "doc", "d":
		return 2
	default:
		return 0
	}
}

func runUI(cmd *cobra.Command, args []string) error {
	client, err := clientFromStore(cmd.Context())
	if err != nil {
		return err
	}
	logf, err := os.OpenFile(filepath.Join(os.TempDir(), "flow-tui.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err == nil {
		defer func() { _ = logf.Close() }()
		os.Stderr = logf
	}
	pal := theme.Load()
	user := os.Getenv("USER")
	m := shell.New(client, user, pal).
		WithTabs([]shell.Route{
			shell.NewHomeRoute(client, pal, user),
			worktime.NewTodayRoute(client, time.Now, pal, worktime.BuildRegistry(client, pal)),
			docsscreen.NewRoute(client, editor.New(), opener.New(), pal, user),
		}).
		WithActiveTab(tabIndexForArg(args))
	_, err = tea.NewProgram(m, tea.WithContext(cmd.Context())).Run()
	return err
}
```

> Confirm `editor.New()` and `opener.New()` constructors + import paths against `cmd/flow/docs.go` (it uses `github.com/serverkraken/flow/internal/adapter/editor` + `…/opener`). They satisfy the `docs.Editor`/`docs.Opener` interfaces.

- [ ] **Step 2: Build + vet**

Run: `go build ./...` and `go vet ./cmd/flow/...`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add cmd/flow/ui.go
git commit -m "feat(m3c3): wire Home·Worktime·Docs tabs + flow ui [tab] deep-link"
```

---

## Task 5: Done-gate — full CI + live verification

**Files:** none (verification only).

- [ ] **Step 1: Run the full CI gate**

Run: `make ci`
Expected: lint + verify-generate + build + tests green; coverage ≥ 80%.
If coverage dips below 80%, add render/reducer tests to the thinnest new surface (the Home dashboard's stacked-vs-2-col branch, or the docs adapter) until the gate passes — do not lower the threshold.

- [ ] **Step 2: Live done-gate against the dev stack**

Start the dev stack ([[reference_flow_dev_env]]): `make dev-up && make dev-run` (separate shells), ensure a token. Then verify:

- [ ] `flow ui` opens on **Home** showing the real Tagesstand: running/stopped state, Heute total + Tagesziel bar, the week pace, recent docs, and projects.
- [ ] Resize narrow (≤ 79 cols) → columns stack; wide → side by side.
- [ ] `w` jumps to the **Worktime** tab; `d` jumps to the **Docs** tab; Tab/digits/palette still switch tabs.
- [ ] **Docs** tab lists documents and is navigable (j/k, enter to view, `/` search) — functionally as standalone `flow docs`.
- [ ] Start/stop a session elsewhere → Home reloads live (SSE).
- [ ] `flow ui worktime` starts directly on the Worktime tab; `flow ui docs` starts on Docs; `flow ui bogus` falls back to Home.

- [ ] **Step 3: Final commit (if any coverage tests were added)**

```bash
git add -A -- internal/ cmd/
git commit -m "test(m3c3): coverage top-up for dashboard/docs route"
```

---

## Notes / known cosmetics (out of scope, do not fix here)

- **Docs double chrome:** the wrapped `DocsModel` renders its own "flow · docs" header + footer inside the shell content area (below the shell's header/tabstrip/breadcrumb and above the shell's footer). Accepted for M3c3 — the Docs surface is functionally unchanged; the Markdown-viewer + chrome cleanup is M3d.
- **Two SSE subscriptions:** `DocsModel.Init` opens its own SSE stream in addition to the shell's broadcast. Harmless (the server allows it); unifying onto the shell's `EventMsg` is M3d.
- **Tagesziel bar is `statusbar.Bar` (neutral), not threshold-colored:** the `totalThresholdColor`/`status_adapter` promotion is deferred to M3d (per the M3c spec); the dashboard uses the plain bar until then.
- **Standalone `flow worktime` still uses the monolith** (`tui.New`) until M3c4 (NavHost). This plan only touches `flow ui`.
