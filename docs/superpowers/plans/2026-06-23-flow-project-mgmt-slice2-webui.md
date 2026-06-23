# Project Management M4 — Slice 2 (WebUI) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Visual styling tasks SHOULD additionally invoke the **`frontend-design`** skill to refine Tailwind classes *within* the structure this plan fixes — the data bindings, component boundaries, and class strings given here are a working baseline (mirroring the existing `docs.templ`), not a ceiling.

**Goal:** Add a full WebUI "Projekte" surface — a `/projects` overview list (status filter), a `/projects/{id}` cockpit (header + description + git section + worktime panel + documents panel + bindings panel), and a create/edit form (name/slug/description/upstream/status + color/glyph pickers + optional rate) — on top of the Slice-1 backend, plus a shared color/glyph whitelist that the TUI (Slice 3) will also consume.

**Architecture:** Server-side-rendered templ + HTMX + Tailwind, inside `flow-server` (same process as REST). WebUI handlers are methods on `*httpserver.Server` and call usecases DIRECTLY (no apiclient), exactly like the docs handlers. A new `domain` whitelist (color names + glyphs) is the single source of valid identity values; the WebUI maps names→hex with a drift-guard test. The cockpit aggregates per-project worktime in the handler (no new backend usecase).

**Tech Stack:** Go, templ (`go tool templ generate`), HTMX, Tailwind (`make web` → committed `static/app.css`, no Node at binary build), goldmark (description rendering, reuse `webui.RenderDocument`), SSE bus (`project.created/updated/deleted`).

## Global Constraints

- Module path `github.com/serverkraken/flow`. Work on branch `rebuild`.
- Spec: `docs/superpowers/specs/2026-06-23-flow-project-management-design.md`. Slice-1 backend (done, HEAD `143d6d8`) provides: `usecase.GetProject`, `usecase.UpdateProject`+`UpdateProjectInput`, `usecase.CreateProject`, `usecase.ListProjects`, `usecase.DeleteProject`, `usecase.SetProjectRate`, all already wired on `httpserver.Server` (`server.go:25-50`). `domain.Project` has `Description`, `UpstreamGit`, `Status` (`active`/`paused`/`archived`), `Color`, `Glyph`, optional `Rate *domain.Money`. Events `domain.EventProjectCreated/Updated/Deleted` (`"project.created"`/`"project.updated"`/`"project.deleted"`).
- `make ci` must stay green; coverage gate ~80%. CI = `golangci-lint run` + `verify-generate` + `cover` + `build` (NOT gofmt). Run it before the final commit of each task that changes Go/templ.
- **templ discipline:** after editing ANY `.templ` file, run `go tool templ generate` and commit the regenerated `_templ.go`. `make verify-generate` (part of `make ci`) fails if generated output drifts.
- **Tailwind discipline:** new utility classes only become visible after `make web` recompiles `internal/adapter/webui/static/app.css` (the Tailwind scanner sources `internal/adapter/webui/**/*.templ`). The committed `app.css` must be regenerated + committed (Task 7). `make web` requires the Tailwind toolchain locally; if unavailable the implementer reports it (Task 7 handles fallback).
- WebUI handlers call usecases directly on `s` (e.g. `s.ListProjects.Execute(...)`); NEVER via apiclient. Authenticated user comes from `userFrom(r.Context())` behind `s.webAuth(...)` (session cookie), NOT the bearer `s.auth`.
- Reuse the established `docs.templ` look (Kompendium): list container `<ul class="divide-y divide-slate-100">`, rows `<li class="py-2">`, chips `rounded-full ... px-2 py-0.5 text-xs`, error banner `rounded bg-rose-50 px-4 py-2 text-sm text-rose-700`, success redirect = HTTP 303 (`http.StatusSeeOther`). Page background/text use the existing slate palette.
- No emoji. The monospace project glyphs (◆ ● ▶ ★ ☼ ✚ ▲ ■) are allowed (whitelisted UI glyphs, not emoji).
- A project needs only a name; `Description`, `UpstreamGit`, `Color`, `Glyph`, `Rate`, and a non-`active` status are all OPTIONAL. Empty rate → no rate (`SetProjectRate(nil)` clears).
- Every commit message ends with the trailer:
  `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`
- Do NOT change `CreateProject.Execute`'s signature or existing callers. Color/glyph/description/upstream/status at create-time are applied by composing `UpdateProject` after create (mirrors the REST `handleCreateProject`).

---

### Task 1: Domain — project color/glyph whitelist + `Validate()` extension

**Files:**
- Create: `internal/domain/projectstyle.go`
- Modify: `internal/domain/project.go` (extend `Validate()`)
- Test: `internal/domain/projectstyle_test.go`

**Interfaces:**
- Produces: `domain.ProjectColors []string` (ordered name whitelist), `domain.ProjectGlyphs []string` (glyph whitelist), `domain.ValidProjectColor(string) bool`, `domain.ValidProjectGlyph(string) bool`. `Project.Validate()` additionally rejects a non-empty `Color`/`Glyph` that is not in the whitelist (empty stays valid).

- [ ] **Step 1: Write the failing test**

Create `internal/domain/projectstyle_test.go`:

```go
package domain_test

import (
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestProjectColorGlyphWhitelist(t *testing.T) {
	if len(domain.ProjectColors) == 0 || len(domain.ProjectGlyphs) == 0 {
		t.Fatal("whitelists must be non-empty")
	}
	// "" (unset) is always valid; a whitelisted value is valid; junk is not.
	if !domain.ValidProjectColor("") || !domain.ValidProjectGlyph("") {
		t.Error("empty must be valid (optional fields)")
	}
	if !domain.ValidProjectColor(domain.ProjectColors[0]) {
		t.Errorf("first palette color %q must validate", domain.ProjectColors[0])
	}
	if !domain.ValidProjectGlyph(domain.ProjectGlyphs[0]) {
		t.Errorf("first glyph %q must validate", domain.ProjectGlyphs[0])
	}
	if domain.ValidProjectColor("chartreuse") || domain.ValidProjectGlyph("Z") {
		t.Error("junk color/glyph must not validate")
	}
}

func TestProjectValidateColorGlyph(t *testing.T) {
	base := domain.Project{Name: "Flow", Slug: "flow", Status: domain.ProjectActive}
	t.Run("empty color/glyph ok", func(t *testing.T) {
		if err := base.Validate(); err != nil {
			t.Errorf("unset color/glyph must be valid: %v", err)
		}
	})
	t.Run("whitelisted ok", func(t *testing.T) {
		p := base
		p.Color = domain.ProjectColors[0]
		p.Glyph = domain.ProjectGlyphs[0]
		if err := p.Validate(); err != nil {
			t.Errorf("whitelisted color/glyph must be valid: %v", err)
		}
	})
	t.Run("bad color rejected", func(t *testing.T) {
		p := base
		p.Color = "chartreuse"
		if !errors.Is(p.Validate(), domain.ErrInvalidProject) {
			t.Error("want ErrInvalidProject for non-whitelist color")
		}
	})
	t.Run("bad glyph rejected", func(t *testing.T) {
		p := base
		p.Glyph = "Z"
		if !errors.Is(p.Validate(), domain.ErrInvalidProject) {
			t.Error("want ErrInvalidProject for non-whitelist glyph")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/ -run 'TestProjectColorGlyph|TestProjectValidateColorGlyph'`
Expected: FAIL — `domain.ProjectColors` etc. undefined.

- [ ] **Step 3: Create the whitelist**

Create `internal/domain/projectstyle.go`:

```go
package domain

// ProjectColors is the single source of valid project color NAMES (the visual
// identity palette). Each surface maps a name to its own rendering (WebUI: a
// hex swatch; TUI: a theme color) — the NAME set lives here so the surfaces
// cannot drift on which colors exist. Names mirror the theme palette accents.
var ProjectColors = []string{
	"blue", "cyan", "green", "purple", "magenta", "yellow", "orange", "red", "teal",
}

// ProjectGlyphs is the whitelist of monospace identity glyphs a project may
// carry (a curated subset of the UI glyph set; not emoji).
var ProjectGlyphs = []string{"◆", "●", "▶", "★", "☼", "✚", "▲", "■"}

// ValidProjectColor reports whether c is unset ("") or a whitelisted name.
func ValidProjectColor(c string) bool { return c == "" || inList(ProjectColors, c) }

// ValidProjectGlyph reports whether g is unset ("") or a whitelisted glyph.
func ValidProjectGlyph(g string) bool { return g == "" || inList(ProjectGlyphs, g) }

func inList(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Extend `Validate()`**

In `internal/domain/project.go`, in `func (p Project) Validate() error`, after the name/slug checks and before (or after) the status switch, add color/glyph guards. The method currently is:

```go
func (p Project) Validate() error {
	switch {
	case p.Name == "":
		return fmt.Errorf("%w: name required", ErrInvalidProject)
	case p.Slug == "":
		return fmt.Errorf("%w: slug required", ErrInvalidProject)
	}
	switch p.Status {
	case ProjectActive, ProjectPaused, ProjectArchived:
		return nil
	default:
		return fmt.Errorf("%w: invalid status %q", ErrInvalidProject, p.Status)
	}
}
```

Replace it with (adds the two whitelist checks, keeps the existing logic):

```go
func (p Project) Validate() error {
	switch {
	case p.Name == "":
		return fmt.Errorf("%w: name required", ErrInvalidProject)
	case p.Slug == "":
		return fmt.Errorf("%w: slug required", ErrInvalidProject)
	case !ValidProjectColor(p.Color):
		return fmt.Errorf("%w: invalid color %q", ErrInvalidProject, p.Color)
	case !ValidProjectGlyph(p.Glyph):
		return fmt.Errorf("%w: invalid glyph %q", ErrInvalidProject, p.Glyph)
	}
	switch p.Status {
	case ProjectActive, ProjectPaused, ProjectArchived:
		return nil
	default:
		return fmt.Errorf("%w: invalid status %q", ErrInvalidProject, p.Status)
	}
}
```

- [ ] **Step 5: Run tests to verify they pass + no regression**

Run: `go test ./internal/domain/ ./internal/usecase/ ./internal/adapter/httpserver/`
Expected: PASS. (UpdateProject calls `Validate()`; confirm no existing test sets a non-whitelist color/glyph — none should, they use `""`.)

- [ ] **Step 6: Commit**

```bash
git add internal/domain/projectstyle.go internal/domain/project.go internal/domain/projectstyle_test.go
git commit -m "feat(project-mgmt): project color/glyph whitelist (domain single-source) + Validate"
```

---

### Task 2: WebUI shared nav component + "Projekte" link

**Files:**
- Create: `internal/adapter/webui/nav.templ`
- Modify: `internal/adapter/webui/worktime.templ`, `dayoffs.templ`, `stats.templ`, `export.templ`, `docs.templ` (replace the inline `<header>…nav…</header>` blocks with `@Nav(...)`)
- Regenerate: all affected `_templ.go`
- Test: `internal/adapter/webui/nav_test.go`

**Interfaces:**
- Produces: templ component `Nav(active, user string)` rendering the top-level links (`worktime`/`dayoffs`/`stats`/`export`/`docs`/`projekte`) with the `active` one styled as current, plus the logout form. Consumed by every page header.

**Context:** The nav is currently copy-pasted across 6 templ sites (`worktime.templ:43`, `dayoffs.templ:35`, `stats.templ:61`, `export.templ:41`, `docs.templ:129/230/308`). Extracting one component is the only place the "Projekte" link needs to be added, and removes the duplication (no-monoliths convention). Each call passes which page is active so the current link is de-emphasised/marked.

- [ ] **Step 1: Write the failing test**

Create `internal/adapter/webui/nav_test.go`:

```go
package webui_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui"
)

func TestNavRendersAllLinksIncludingProjects(t *testing.T) {
	var buf bytes.Buffer
	if err := webui.Nav("projekte", "msoent").Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{`href="/"`, `href="/dayoffs"`, `href="/stats"`, `href="/export"`, `href="/docs"`, `href="/projects"`, "projekte", "msoent", "/auth/logout"} {
		if !strings.Contains(out, want) {
			t.Errorf("nav missing %q\n%s", want, out)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapter/webui/ -run TestNavRendersAllLinksIncludingProjects`
Expected: FAIL — `webui.Nav` undefined.

- [ ] **Step 3: Create the shared nav component**

Create `internal/adapter/webui/nav.templ`:

```templ
package webui

// navLink is one top-level destination. active==key dims it as the current page.
type navLink struct {
	Key, Href, Label string
}

var navLinks = []navLink{
	{"worktime", "/", "worktime"},
	{"dayoffs", "/dayoffs", "dayoffs"},
	{"stats", "/stats", "stats"},
	{"export", "/export", "export"},
	{"docs", "/docs", "docs"},
	{"projekte", "/projects", "projekte"},
}

templ Nav(active, user string) {
	<header class="mb-4 flex items-center justify-between">
		<h1 class="text-lg font-semibold text-slate-900">flow</h1>
		<div class="flex gap-4 text-sm text-slate-500">
			for _, l := range navLinks {
				if l.Key == active {
					<span class="font-medium text-slate-900">{ l.Label }</span>
				} else {
					<a href={ templ.SafeURL(l.Href) } class="hover:text-slate-700">{ l.Label }</a>
				}
			}
			<form action="/auth/logout" method="post" hx-boost="false">
				<button class="hover:text-slate-700">logout { user }</button>
			</form>
		</div>
	</header>
}
```

- [ ] **Step 4: Replace the 6 inline nav blocks with `@Nav(...)`**

In each file, replace the inline `<header class="mb-4 flex items-center justify-between"> … </header>` block with a single call, passing the page's active key and the existing user value (the surrounding data var differs per file — use whatever the file already binds for the username, e.g. `d.User`):

- `worktime.templ` (WorktimeFragment, ~line 43): `@Nav("worktime", d.User)`
- `dayoffs.templ` (~line 35): `@Nav("dayoffs", d.User)`
- `stats.templ` (~line 61): `@Nav("stats", d.User)`
- `export.templ` (~line 41): `@Nav("export", d.User)`
- `docs.templ` (DocsFragment ~129, DocView ~230, DocForm ~308): `@Nav("docs", d.User)` at each of the three sites.

(If a particular page's data struct names the user field differently, match it. Confirm each `.templ` still binds a username string in scope at the header site.)

- [ ] **Step 5: Regenerate templ + run tests**

Run: `go tool templ generate && go test ./internal/adapter/webui/ ./internal/adapter/httpserver/`
Expected: builds; the new nav test passes; existing webui render tests + httpserver webui handler tests still pass (they assert on body substrings like link hrefs — the hrefs are unchanged, and the username still renders).

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/webui/nav.templ internal/adapter/webui/nav_templ.go internal/adapter/webui/worktime.templ internal/adapter/webui/worktime_templ.go internal/adapter/webui/dayoffs.templ internal/adapter/webui/dayoffs_templ.go internal/adapter/webui/stats.templ internal/adapter/webui/stats_templ.go internal/adapter/webui/export.templ internal/adapter/webui/export_templ.go internal/adapter/webui/docs.templ internal/adapter/webui/docs_templ.go
git commit -m "feat(project-mgmt): shared webui Nav component + Projekte link"
```

---

### Task 3: WebUI render helpers — color hex, status badge, glyph (+ drift-guard)

**Files:**
- Create: `internal/adapter/webui/projectstyle.go`
- Test: `internal/adapter/webui/projectstyle_test.go`

**Interfaces:**
- Produces: `webui.ColorHex(name string) string` (whitelisted name → Tokyonight hex, "" → ""), `webui.StatusBadge(status domain.ProjectStatus) (label, classes string)` (de/active/paused/archived → German label + Tailwind chip classes; paused dimmed), `webui.ProjectGlyph(g string) string` (g, or a neutral default `domain.ProjectGlyphs` fallback when empty? NO — return g as-is; empty renders nothing). These are pure helpers the templ calls.

- [ ] **Step 1: Write the failing test (incl. drift-guard)**

Create `internal/adapter/webui/projectstyle_test.go`:

```go
package webui_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

// Drift guard: every domain palette color MUST have a WebUI hex, else a project
// could carry a color the WebUI renders as blank.
func TestColorHexCoversWholePalette(t *testing.T) {
	for _, name := range domain.ProjectColors {
		hex := webui.ColorHex(name)
		if !strings.HasPrefix(hex, "#") || len(hex) != 7 {
			t.Errorf("color %q → %q, want a #rrggbb hex", name, hex)
		}
	}
	if webui.ColorHex("") != "" {
		t.Error("empty color → empty hex")
	}
	if webui.ColorHex("chartreuse") != "" {
		t.Error("unknown color → empty hex (not a guess)")
	}
}

func TestStatusBadge(t *testing.T) {
	for _, st := range []domain.ProjectStatus{domain.ProjectActive, domain.ProjectPaused, domain.ProjectArchived} {
		label, classes := webui.StatusBadge(st)
		if label == "" || classes == "" {
			t.Errorf("status %q → empty label/classes", st)
		}
	}
	if l, _ := webui.StatusBadge(domain.ProjectActive); l != "aktiv" {
		t.Errorf("active label = %q, want aktiv", l)
	}
	if l, _ := webui.StatusBadge(domain.ProjectPaused); l != "pausiert" {
		t.Errorf("paused label = %q, want pausiert", l)
	}
	if l, _ := webui.StatusBadge(domain.ProjectArchived); l != "archiviert" {
		t.Errorf("archived label = %q, want archiviert", l)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapter/webui/ -run 'TestColorHexCoversWholePalette|TestStatusBadge'`
Expected: FAIL — helpers undefined.

- [ ] **Step 3: Implement**

Create `internal/adapter/webui/projectstyle.go`:

```go
package webui

import "github.com/serverkraken/flow/internal/domain"

// colorHex maps each domain.ProjectColors name to its Tokyonight-Night hex.
// MUST cover every name in domain.ProjectColors (enforced by a drift-guard test).
var colorHex = map[string]string{
	"blue":    "#7aa2f7",
	"cyan":    "#7dcfff",
	"green":   "#9ece6a",
	"purple":  "#bb9af7",
	"magenta": "#ff007c",
	"yellow":  "#e0af68",
	"orange":  "#ff9e64",
	"red":     "#f7768e",
	"teal":    "#73daca",
}

// ColorHex returns the swatch hex for a whitelisted color name, or "" for unset
// or unknown (the caller renders no swatch rather than guessing).
func ColorHex(name string) string { return colorHex[name] }

// StatusBadge returns a German label and Tailwind chip classes for a project
// status. Paused is dimmed; archived is muted; active is green-ish.
func StatusBadge(s domain.ProjectStatus) (label, classes string) {
	switch s {
	case domain.ProjectPaused:
		return "pausiert", "rounded-full bg-amber-100 px-2 py-0.5 text-xs text-amber-700 opacity-70"
	case domain.ProjectArchived:
		return "archiviert", "rounded-full bg-slate-100 px-2 py-0.5 text-xs text-slate-400"
	default: // active
		return "aktiv", "rounded-full bg-emerald-100 px-2 py-0.5 text-xs text-emerald-700"
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapter/webui/ -run 'TestColorHexCoversWholePalette|TestStatusBadge'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/webui/projectstyle.go internal/adapter/webui/projectstyle_test.go
git commit -m "feat(project-mgmt): webui color-hex + status-badge helpers (palette drift-guard)"
```

---

### Task 4: Project list page (`/projects` + SSE fragment + status filter)

**Files:**
- Create: `internal/adapter/webui/projects.templ` (list components only this task; detail+form added in Tasks 5–6)
- Create: `internal/adapter/httpserver/webui_projects.go` (list handlers + data builder)
- Modify: `internal/adapter/httpserver/server.go` (register `GET /projects`, `GET /ui/projects/list`)
- Test: `internal/adapter/httpserver/webui_projects_test.go`

**Interfaces:**
- Consumes: `s.ListProjects.Execute(ctx, ownerID) ([]domain.Project, error)`; `webui.ColorHex`, `webui.StatusBadge` (Task 3); `webui.Nav` (Task 2).
- Produces: view model `webui.ProjectsPageData{User string; Status string; Projects []domain.Project}`; templ `webui.ProjectsPage(d)` + `webui.ProjectsFragment(d)`; handlers `(*Server).handleWebProjectsHome`, `(*Server).handleWebProjectsList`; package-private `(*Server).projectsListData(r, u) ProjectsPageData`.

**Context:** Mirror the docs list (`webui_docs.go:96-199` + `docs.templ`). Default view shows `active`+`paused`; `?status=archived` (or `?status=all`) reveals archived. SSE re-renders the fragment on `project.created/updated/deleted`. Filtering is a full-page `GET /projects?status=…` (like docs' tag filter), the SSE div re-fetches `/ui/projects/list`.

- [ ] **Step 1: Write the failing test**

Add `internal/adapter/httpserver/webui_projects_test.go`. Mirror `webui_docs_test.go`'s helper style: build a `*httpserver.Server` with fakes + a `websession` codec, seed a user, forge the `flow_session` cookie, hit `srv.Routes()`. (Reuse/adapt the existing `newWebDocsServer`-style helper; if none is shared, write a local `newWebProjectsServer(t)`.)

```go
package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/adapter/websession"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// newWebProjectsServer builds a webui-capable server with the project usecases
// wired and a seeded user; returns the test server, a session cookie, and the
// fake project store for seeding.
func newWebProjectsServer(t *testing.T) (*httptest.Server, *http.Cookie, *testutil.FakeProjectStore) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 23, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	ps := testutil.NewFakeProjectStore()
	bs := testutil.NewFakeProjectBindingStore()
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x.de", "M")
	_, _ = users.UpsertBySub(nil, u) // adapt to the fake's signature if it needs ctx
	codec := websession.NewCodec([]byte("test-secret-test-secret-test-1234"))

	srv := &httpserver.Server{
		Users:         users,
		SessionCodec:  codec,
		Bus:           sse.NewBus(),
		Clock:         clk,
		CreateProject: usecase.CreateProject{Projects: ps, IDs: ids, Clock: clk},
		ListProjects:  usecase.ListProjects{Projects: ps},
		GetProject:    usecase.GetProject{Projects: ps},
		UpdateProject: usecase.UpdateProject{Projects: ps, Bindings: bs, IDs: ids, Clock: clk},
		DeleteProject: usecase.DeleteProject{Projects: ps},
		SetProjectRate: usecase.SetProjectRate{Projects: ps},
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cookie := &http.Cookie{Name: "flow_session", Value: codec.Issue("u1")}
	return ts, cookie, ps
}

func seedProject(t *testing.T, ps *testutil.FakeProjectStore, id, name string, status domain.ProjectStatus) {
	t.Helper()
	now := time.Date(2026, 6, 23, 9, 0, 0, 0, time.UTC)
	p, _ := domain.NewProject(id, "u1", name, name, now)
	p.Status = status
	_, _ = ps.Create(nil, p) // adapt ctx if needed
}

func getWeb(t *testing.T, ts *httptest.Server, c *http.Cookie, path string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest("GET", ts.URL+path, nil)
	req.AddCookie(c)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b := make([]byte, 0)
	buf := make([]byte, 4096)
	for {
		n, e := res.Body.Read(buf)
		b = append(b, buf[:n]...)
		if e != nil {
			break
		}
	}
	return res.StatusCode, string(b)
}

func TestWebProjectsListAndFilter(t *testing.T) {
	ts, c, ps := newWebProjectsServer(t)
	seedProject(t, ps, "p1", "Aaa", domain.ProjectActive)
	seedProject(t, ps, "p2", "Bbb", domain.ProjectPaused)
	seedProject(t, ps, "p3", "Ccc", domain.ProjectArchived)

	// default: active+paused → Aaa, Bbb shown, Ccc (archived) hidden
	code, body := getWeb(t, ts, c, "/projects")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	if !strings.Contains(body, "Aaa") || !strings.Contains(body, "Bbb") {
		t.Errorf("default should list active+paused")
	}
	if strings.Contains(body, "Ccc") {
		t.Errorf("default must hide archived")
	}
	// archived filter reveals Ccc
	_, arch := getWeb(t, ts, c, "/projects?status=archived")
	if !strings.Contains(arch, "Ccc") {
		t.Errorf("archived filter should show Ccc")
	}
	// status badge label present
	if !strings.Contains(body, "pausiert") {
		t.Errorf("paused badge label expected")
	}
	// SSE fragment route works
	codeF, _ := getWeb(t, ts, c, "/ui/projects/list")
	if codeF != 200 {
		t.Errorf("fragment status %d", codeF)
	}
}
```

(Adapt the fake constructors' context arguments to the real signatures — check `testutil` if `Create`/`UpsertBySub` need a non-nil `context.Context`; use `context.Background()`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapter/httpserver/ -run TestWebProjectsListAndFilter`
Expected: FAIL — routes 404 / `ProjectsPage` undefined.

- [ ] **Step 3: Add the list templ**

Create `internal/adapter/webui/projects.templ`:

```templ
package webui

import "github.com/serverkraken/flow/internal/domain"

// ProjectsPageData is the list view model.
type ProjectsPageData struct {
	User     string
	Status   string // active filter token: "" (=active+paused default), "archived", "all"
	Projects []domain.Project
}

templ ProjectsPage(d ProjectsPageData) {
	<!DOCTYPE html>
	<html lang="de">
		<head>
			<meta charset="utf-8"/>
			<meta name="viewport" content="width=device-width, initial-scale=1"/>
			<title>flow · projekte</title>
			<link rel="stylesheet" href="/static/app.css"/>
			<script src="https://unpkg.com/htmx.org@1.9.12"></script>
			<script src="https://unpkg.com/htmx.org@1.9.12/dist/ext/sse.js"></script>
		</head>
		<body class="mx-auto max-w-3xl p-6 text-slate-900" hx-ext="sse" sse-connect="/sse">
			@ProjectsFragment(d)
		</body>
	</html>
}

templ ProjectsFragment(d ProjectsPageData) {
	@Nav("projekte", d.User)
	<div class="mb-4 flex items-center justify-between">
		<div class="flex gap-2 text-sm">
			<a href="/projects" class={ filterChip(d.Status == "") }>aktiv + pausiert</a>
			<a href="/projects?status=archived" class={ filterChip(d.Status == "archived") }>archiviert</a>
			<a href="/projects?status=all" class={ filterChip(d.Status == "all") }>alle</a>
		</div>
		<a href="/projects/new" class="rounded bg-slate-900 px-3 py-1.5 text-sm text-white hover:bg-slate-700">Neues Projekt</a>
	</div>
	<div
		id="pc"
		hx-get="/ui/projects/list"
		hx-trigger="sse:project.created, sse:project.updated, sse:project.deleted"
		hx-swap="innerHTML"
		hx-include="[name='status']"
	>
		<input type="hidden" name="status" value={ d.Status }/>
		@projectsList(d.Projects)
	</div>
}

templ projectsList(ps []domain.Project) {
	if len(ps) == 0 {
		<p class="py-8 text-center text-sm text-slate-400">Keine Projekte.</p>
	} else {
		<ul class="divide-y divide-slate-100">
			for _, p := range ps {
				<li class="py-2">
					<a href={ templ.SafeURL("/projects/" + p.ID) } class="block hover:bg-slate-50">
						<div class="flex items-center justify-between text-sm">
							<span class="flex items-center gap-2 font-medium text-slate-800">
								@glyphSwatch(p)
								{ p.Name }
							</span>
							@statusBadge(p.Status)
						</div>
						if p.UpstreamGit != "" {
							<div class="mt-0.5 font-mono text-xs text-slate-500">{ p.UpstreamGit }</div>
						}
					</a>
				</li>
			}
		</ul>
	}
}

// glyphSwatch renders the project's color dot + glyph (both optional).
templ glyphSwatch(p domain.Project) {
	if ColorHex(p.Color) != "" {
		<span class="inline-block h-2.5 w-2.5 rounded-full" style={ "background-color:" + ColorHex(p.Color) }></span>
	}
	if p.Glyph != "" {
		<span class="font-mono text-slate-500">{ p.Glyph }</span>
	}
}

templ statusBadge(s domain.ProjectStatus) {
	{{ label, classes := StatusBadge(s) }}
	<span class={ classes }>{ label }</span>
}

func filterChip(active bool) string {
	if active {
		return "rounded-full bg-slate-900 px-2 py-0.5 text-xs text-white"
	}
	return "rounded-full bg-slate-100 px-2 py-0.5 text-xs text-slate-600 hover:bg-slate-200"
}
```

- [ ] **Step 4: Add the list handlers + data builder**

Create `internal/adapter/httpserver/webui_projects.go`:

```go
package httpserver

import (
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

// projectsListData loads the owner's projects and applies the status filter.
// "" → active+paused (default view); "archived" → archived only; "all" → every status.
func (s *Server) projectsListData(r *http.Request, u domain.User) webui.ProjectsPageData {
	status := r.URL.Query().Get("status")
	all, _ := s.ListProjects.Execute(r.Context(), u.ID)
	var filtered []domain.Project
	for _, p := range all {
		switch status {
		case "all":
			filtered = append(filtered, p)
		case "archived":
			if p.Status == domain.ProjectArchived {
				filtered = append(filtered, p)
			}
		default: // active + paused
			if p.Status == domain.ProjectActive || p.Status == domain.ProjectPaused {
				filtered = append(filtered, p)
			}
		}
	}
	return webui.ProjectsPageData{User: u.DisplayName, Status: status, Projects: filtered}
}

func (s *Server) handleWebProjectsHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d := s.projectsListData(r, u)
	_ = webui.ProjectsPage(d).Render(r.Context(), w)
}

func (s *Server) handleWebProjectsList(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d := s.projectsListData(r, u)
	_ = webui.ProjectsFragment(d).Render(r.Context(), w)
}
```

(Match the exact `userFrom` return shape and the `User.DisplayName` field name to the codebase; the docs handlers show the canonical usage.)

- [ ] **Step 5: Register the routes**

In `internal/adapter/httpserver/server.go`, in `Routes()`, near the other webui registrations (after the docs block ~line 164), add:

```go
	mux.Handle("GET /projects", s.webAuth(http.HandlerFunc(s.handleWebProjectsHome)))
	mux.Handle("GET /ui/projects/list", s.webAuth(http.HandlerFunc(s.handleWebProjectsList)))
```

NOTE: these are WEBUI routes (session-cookie auth). They do NOT collide with the REST `GET /api/v1/projects` routes (different path prefix). The detail route `GET /projects/{id}` is added in Task 5.

- [ ] **Step 6: Generate templ, run tests + build**

Run: `go tool templ generate && go test ./internal/adapter/httpserver/ -run TestWebProjectsListAndFilter && go build ./...`
Expected: PASS; builds. Then `golangci-lint run` clean on touched files.

- [ ] **Step 7: Commit**

```bash
git add internal/adapter/webui/projects.templ internal/adapter/webui/projects_templ.go internal/adapter/httpserver/webui_projects.go internal/adapter/httpserver/server.go internal/adapter/httpserver/webui_projects_test.go
git commit -m "feat(project-mgmt): webui /projects list + status filter + SSE fragment"
```

---

### Task 5: Project detail cockpit (`/projects/{id}`)

**Files:**
- Modify: `internal/adapter/webui/projects.templ` (add cockpit view model + `ProjectView`)
- Modify: `internal/adapter/httpserver/webui_projects.go` (add cockpit builder + `handleWebProjectView`)
- Modify: `internal/adapter/httpserver/server.go` (register `GET /projects/{id}`)
- Test: `internal/adapter/httpserver/webui_projects_test.go` (add cockpit test)

**Interfaces:**
- Consumes: `s.GetProject.Execute(ctx, ownerID, id)`; per-project worktime via `s.ListSessionsRange`/the existing session-list usecase (whatever the worktime webui builder uses — check `webui_worktime.go`'s `worktimeDataFor`); project-scoped docs via `s.ListDocumentsScoped.Execute(ctx, ownerID, &projectID)` (the projectID-filtered list from the flow-mcp slice); bindings via `s.ListProjectBindings`/`ListProjectBindingsByProject` (check `projectbindings.go` for the exact usecase/method); `webui.RenderDocument` (markdown), `webui.ColorHex`, `webui.StatusBadge`.
- Produces: view model `webui.ProjectCockpit{User string; P domain.Project; DescriptionHTML template.HTML; WeekHours, MonthHours, TotalHours float64; Earnings string; Docs []domain.Document; Bindings []domain.ProjectBinding}`; templ `webui.ProjectView(d)`; handler `(*Server).handleWebProjectView`; builder `(*Server).projectCockpitData(r, u, id) (webui.ProjectCockpit, error)`.

**Context:** This is the cockpit from the spec. The git section renders ONLY when `P.UpstreamGit != ""`. The worktime panel aggregates the project's sessions in the handler (sum durations: total, current week, current month) and computes `Σh × rate` when `P.Rate != nil`. The documents panel lists project-scoped docs (links). The bindings panel is read-only. SSE swaps the whole view on `project.updated`.

> **Implementer note:** Before writing the builder, READ `internal/adapter/httpserver/webui_worktime.go` (`worktimeDataFor`) to copy the exact session-listing usecase + duration-summing idiom already in use, and `internal/adapter/httpserver/projectbindings.go` + the flow-mcp `ListDocumentsScoped` apiclient/usecase to copy the exact signatures. Do NOT invent new usecases — Slice 1/earlier slices already expose these reads. If a per-project session range read is missing, sum over `s.ListSessions`/`ListSessionsRange` filtered by `ProjectID` in the builder.

- [ ] **Step 1: Write the failing test**

Add to `webui_projects_test.go`:

```go
func TestWebProjectCockpit(t *testing.T) {
	ts, c, ps := newWebProjectsServer(t)
	now := time.Date(2026, 6, 23, 9, 0, 0, 0, time.UTC)
	p, _ := domain.NewProject("p1", "u1", "Flow", "flow", now)
	p.Description = "# Notiz\nhallo"
	p.UpstreamGit = "git@github.com:serverkraken/flow.git"
	p.Status = domain.ProjectPaused
	p.Color = domain.ProjectColors[0]
	_, _ = ps.Create(context.Background(), p)

	code, body := getWeb(t, ts, c, "/projects/p1")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	for _, want := range []string{"Flow", "pausiert", "github.com/serverkraken/flow", "Bearbeiten"} {
		if !strings.Contains(body, want) {
			t.Errorf("cockpit missing %q", want)
		}
	}
	// rendered markdown description (goldmark → <h1>)
	if !strings.Contains(body, "Notiz") {
		t.Errorf("description should render")
	}
	// unknown id → 404
	code404, _ := getWeb(t, ts, c, "/projects/nope")
	if code404 != http.StatusNotFound {
		t.Errorf("unknown id status %d, want 404", code404)
	}
}
```

(Add `"context"` to the test imports if not present.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/adapter/httpserver/ -run TestWebProjectCockpit`
Expected: FAIL — route 404 / `ProjectView` undefined.

- [ ] **Step 3: Add the cockpit view model + templ**

Append to `internal/adapter/webui/projects.templ`:

```templ
import "html/template"

type ProjectCockpit struct {
	User           string
	P              domain.Project
	DescriptionHTML template.HTML
	TotalHours     float64
	WeekHours      float64
	MonthHours     float64
	Earnings       string // formatted "€" string, "" when no rate
	Docs           []domain.Document
	Bindings       []domain.ProjectBinding
}

templ ProjectView(d ProjectCockpit) {
	<!DOCTYPE html>
	<html lang="de">
		<head>
			<meta charset="utf-8"/>
			<meta name="viewport" content="width=device-width, initial-scale=1"/>
			<title>flow · { d.P.Name }</title>
			<link rel="stylesheet" href="/static/app.css"/>
			<script src="https://unpkg.com/htmx.org@1.9.12"></script>
			<script src="https://unpkg.com/htmx.org@1.9.12/dist/ext/sse.js"></script>
		</head>
		<body class="mx-auto max-w-3xl p-6 text-slate-900" hx-ext="sse" sse-connect="/sse">
			@Nav("projekte", d.User)
			<div id="pv" hx-get={ "/projects/" + d.P.ID } hx-trigger="sse:project.updated" hx-swap="outerHTML">
				@projectCockpitBody(d)
			</div>
		</body>
	</html>
}

templ projectCockpitBody(d ProjectCockpit) {
	<div class="mb-4 flex items-center justify-between">
		<h2 class="flex items-center gap-2 text-xl font-semibold">
			@glyphSwatch(d.P)
			{ d.P.Name }
			@statusBadge(d.P.Status)
		</h2>
		<div class="flex gap-2 text-sm">
			<a href={ templ.SafeURL("/projects/" + d.P.ID + "/edit") } class="rounded bg-slate-900 px-3 py-1.5 text-white hover:bg-slate-700">Bearbeiten</a>
			@statusActionButton(d.P)
		</div>
	</div>
	if d.DescriptionHTML != "" {
		<div class="prose prose-slate mb-6 max-w-none text-sm">@templ.Raw(string(d.DescriptionHTML))</div>
	} else {
		<p class="mb-6 text-sm text-slate-400">—</p>
	}
	if d.P.UpstreamGit != "" {
		<section class="mb-6">
			<h3 class="mb-1 text-sm font-medium text-slate-700">Git</h3>
			<code class="rounded bg-slate-100 px-2 py-1 text-xs">{ d.P.UpstreamGit }</code>
		</section>
	}
	<section class="mb-6">
		<h3 class="mb-1 text-sm font-medium text-slate-700">Worktime</h3>
		<div class="flex gap-6 text-sm text-slate-600">
			<span>Σ { fmtHours(d.TotalHours) }</span>
			<span>Woche { fmtHours(d.WeekHours) }</span>
			<span>Monat { fmtHours(d.MonthHours) }</span>
			if d.Earnings != "" {
				<span class="font-medium text-slate-800">{ d.Earnings }</span>
			}
		</div>
	</section>
	<section class="mb-6">
		<h3 class="mb-1 text-sm font-medium text-slate-700">Dokumente ({ fmtCount(len(d.Docs)) })</h3>
		if len(d.Docs) == 0 {
			<p class="text-sm text-slate-400">—</p>
		} else {
			<ul class="divide-y divide-slate-100">
				for _, doc := range d.Docs {
					<li class="py-1 text-sm">
						<a href={ templ.SafeURL("/docs/" + doc.ID) } class="hover:text-slate-700">{ doc.Title }</a>
					</li>
				}
			</ul>
		}
	</section>
	<section>
		<h3 class="mb-1 text-sm font-medium text-slate-700">Bindings</h3>
		if len(d.Bindings) == 0 {
			<p class="text-sm text-slate-400">—</p>
		} else {
			<ul class="text-xs text-slate-500">
				for _, b := range d.Bindings {
					<li class="font-mono">{ string(b.Kind) }: { bindingTarget(b) }</li>
				}
			</ul>
		}
	</section>
}

// statusActionButton offers the single most relevant lifecycle transition.
templ statusActionButton(p domain.Project) {
	switch p.Status {
		case domain.ProjectActive:
			<form action={ templ.SafeURL("/projects/" + p.ID + "/status") } method="post" hx-boost="false">
				<input type="hidden" name="status" value="paused"/>
				<button class="rounded border border-slate-300 px-3 py-1.5 hover:bg-slate-100">Pausieren</button>
			</form>
		case domain.ProjectPaused:
			<form action={ templ.SafeURL("/projects/" + p.ID + "/status") } method="post" hx-boost="false">
				<input type="hidden" name="status" value="active"/>
				<button class="rounded border border-slate-300 px-3 py-1.5 hover:bg-slate-100">Fortsetzen</button>
			</form>
		case domain.ProjectArchived:
			<form action={ templ.SafeURL("/projects/" + p.ID + "/status") } method="post" hx-boost="false">
				<input type="hidden" name="status" value="active"/>
				<button class="rounded border border-slate-300 px-3 py-1.5 hover:bg-slate-100">Reaktivieren</button>
			</form>
	}
	if p.Status != domain.ProjectArchived {
		<form action={ templ.SafeURL("/projects/" + p.ID + "/status") } method="post" hx-boost="false">
			<input type="hidden" name="status" value="archived"/>
			<button class="rounded border border-slate-300 px-3 py-1.5 text-slate-500 hover:bg-slate-100">Archivieren</button>
		</form>
	}
}

func bindingTarget(b domain.ProjectBinding) string {
	if b.RemoteSlug != "" {
		return b.RemoteSlug
	}
	return b.Path
}

func fmtHours(h float64) string { return fmtHoursImpl(h) }
func fmtCount(n int) string     { return fmtCountImpl(n) }
```

(Helpers `fmtHoursImpl`/`fmtCountImpl` — implement in `webui_projects.go` or `format.go`; reuse the existing `fmtDur`/`fmtHM`/`fmtInt` in `format.go` if they fit, in which case call those directly and drop the wrappers. Prefer reuse.)

- [ ] **Step 4: Add the cockpit builder + handler**

Add to `internal/adapter/httpserver/webui_projects.go`:

```go
// projectCockpitData assembles the full cockpit: project, rendered description,
// per-project worktime aggregate (total/week/month + earnings), scoped docs,
// and bindings. Read-only; aggregation is done here (no new backend usecase).
func (s *Server) projectCockpitData(r *http.Request, u domain.User, id string) (webui.ProjectCockpit, error) {
	p, err := s.GetProject.Execute(r.Context(), u.ID, id)
	if err != nil {
		return webui.ProjectCockpit{}, err
	}
	d := webui.ProjectCockpit{User: u.DisplayName, P: p}
	// description markdown (reuse the docs renderer; wikilinks resolve to nothing here)
	if p.Description != "" {
		d.DescriptionHTML = webui.RenderDocument(p.Description, func(string) (string, bool) { return "", false })
	}
	// worktime aggregate — copy the session-listing + duration idiom from
	// webui_worktime.go's worktimeDataFor; sum sessions whose ProjectID == p.ID.
	// total/week/month in hours; earnings = total * rate when p.Rate != nil.
	d.TotalHours, d.WeekHours, d.MonthHours, d.Earnings = s.projectWorktime(r, u, p)
	// scoped docs
	pid := p.ID
	d.Docs, _ = s.ListDocumentsScoped.Execute(r.Context(), u.ID, &pid)
	// bindings (read-only)
	d.Bindings, _ = s.ListProjectBindings.Execute(r.Context(), u.ID, p.ID) // adapt to real method
	return d, nil
}

func (s *Server) handleWebProjectView(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d, err := s.projectCockpitData(r, u, r.PathValue("id"))
	if errors.Is(err, ports.ErrProjectNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	_ = webui.ProjectView(d).Render(r.Context(), w)
}
```

> The `s.projectWorktime(...)` helper and the exact `ListDocumentsScoped`/`ListProjectBindings` field names/signatures are codebase-specific — the implementer fills them from the existing worktime/docs/bindings webui+usecase code (READ those files first, per the implementer note). Add `"errors"` and the `ports` import if missing.

- [ ] **Step 5: Register the route**

In `server.go` after the list routes:

```go
	mux.Handle("GET /projects/{id}", s.webAuth(http.HandlerFunc(s.handleWebProjectView)))
```

NOTE: `GET /projects/new` (Task 6) is a LITERAL segment and wins over `{id}` under Go 1.22 specificity — no conflict. Register `/projects/new` in Task 6.

- [ ] **Step 6: Generate, test, build, lint**

Run: `go tool templ generate && go test ./internal/adapter/httpserver/ -run 'TestWebProject' && go build ./... && golangci-lint run`
Expected: PASS; clean.

- [ ] **Step 7: Commit**

```bash
git add internal/adapter/webui/projects.templ internal/adapter/webui/projects_templ.go internal/adapter/httpserver/webui_projects.go internal/adapter/httpserver/server.go internal/adapter/httpserver/webui_projects_test.go
git commit -m "feat(project-mgmt): webui /projects/{id} cockpit (description, git, worktime, docs, bindings)"
```

---

### Task 6: Create/Edit form + create/update/delete/status/rate handlers

**Files:**
- Modify: `internal/adapter/webui/projects.templ` (add form view model + `ProjectForm` + color/glyph pickers)
- Modify: `internal/adapter/httpserver/webui_projects.go` (form + mutation handlers)
- Modify: `internal/adapter/httpserver/server.go` (register form/mutation routes)
- Test: `internal/adapter/httpserver/webui_projects_test.go` (form + create + edit + status + rate + delete)

**Interfaces:**
- Consumes: `s.CreateProject.Execute`, `s.UpdateProject.Execute(ctx, ownerID, id, usecase.UpdateProjectInput{...})`, `s.SetProjectRate.Execute` (signature: confirm — takes `(ctx, ownerID, id, *domain.Money)` per Slice-1; check `export.go:handleSetProjectRate`), `s.DeleteProject.Execute`, `s.GetProject.Execute`; `domain.ProjectColors`, `domain.ProjectGlyphs`.
- Produces: view model `webui.ProjectFormValues{Name, Slug, Description, UpstreamGit, Status, Color, Glyph, RateAmount, RateCurrency string}`; templ `webui.ProjectForm(d ProjectFormData, editing *domain.Project)`; handlers `handleWebProjectNew`, `handleWebProjectCreate`, `handleWebProjectEdit`, `handleWebProjectUpdate`, `handleWebProjectStatus`, `handleWebProjectDelete`.

**Context:** Single `ProjectForm` for create + edit (`editing == nil` → create). Create composes: `CreateProject` (name/slug/color/glyph) then `UpdateProject` (description/upstream/status + re-affirm color/glyph) then `SetProjectRate` if a rate was entered — mirroring the REST `handleCreateProject` compose. Edit: `UpdateProject` + `SetProjectRate` (nil clears when the amount field is blank). Rate is OPTIONAL: blank amount → `SetProjectRate(ctx, …, nil)`. Errors re-render the form with a rose banner + repopulated values + 400; success → 303 redirect to `/projects/{id}` (or `/projects` for delete). Color picker = radio swatches over `domain.ProjectColors`; glyph picker = radio buttons over `domain.ProjectGlyphs`; both allow "none".

- [ ] **Step 1: Write the failing tests**

Add to `webui_projects_test.go` (POST helper mirrors docs form tests; use `url.Values` form-encoding + a `CheckRedirect` client that stops at 303):

```go
func postWebForm(t *testing.T, ts *httptest.Server, c *http.Cookie, path string, form url.Values) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", ts.URL+path, strings.NewReader(form.Encode()))
	req.AddCookie(c)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestWebProjectCreateEditStatusDelete(t *testing.T) {
	ts, c, ps := newWebProjectsServer(t)

	// CREATE with upstream + color + rate
	res := postWebForm(t, ts, c, "/projects", url.Values{
		"name": {"PM Web"}, "slug": {"pm-web"}, "description": {"# Hi"},
		"upstreamGit": {"git@github.com:serverkraken/pmweb.git"}, "status": {"active"},
		"color": {domain.ProjectColors[0]}, "glyph": {domain.ProjectGlyphs[0]},
		"rateAmount": {"90.00"}, "rateCurrency": {"EUR"},
	})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("create status %d, want 303", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	_ = res.Body.Close()
	if !strings.HasPrefix(loc, "/projects/") {
		t.Fatalf("create redirect = %q", loc)
	}
	id := strings.TrimPrefix(loc, "/projects/")

	// the cockpit reflects the saved fields + rate earnings
	_, body := getWeb(t, ts, c, "/projects/"+id)
	if !strings.Contains(body, "PM Web") || !strings.Contains(body, "github.com/serverkraken/pmweb") {
		t.Errorf("created project not reflected: %s", body)
	}

	// EDIT → pause + change description
	res = postWebForm(t, ts, c, "/projects/"+id, url.Values{
		"name": {"PM Web"}, "slug": {"pm-web"}, "description": {"changed"},
		"upstreamGit": {"git@github.com:serverkraken/pmweb.git"}, "status": {"paused"},
		"color": {domain.ProjectColors[0]}, "glyph": {""}, "rateAmount": {""}, "rateCurrency": {"EUR"},
	})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("edit status %d, want 303", res.StatusCode)
	}
	_ = res.Body.Close()
	p, _ := ps.Get(context.Background(), "u1", id)
	if p.Status != domain.ProjectPaused {
		t.Errorf("edit did not pause: %s", p.Status)
	}
	if p.Rate != nil {
		t.Errorf("blank rateAmount must clear the rate, got %+v", p.Rate)
	}

	// STATUS action → archive
	res = postWebForm(t, ts, c, "/projects/"+id+"/status", url.Values{"status": {"archived"}})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("status action %d, want 303", res.StatusCode)
	}
	_ = res.Body.Close()
	p, _ = ps.Get(context.Background(), "u1", id)
	if p.Status != domain.ProjectArchived {
		t.Errorf("status action did not archive: %s", p.Status)
	}

	// CREATE with bad upstream → 400 + re-rendered form
	res = postWebForm(t, ts, c, "/projects", url.Values{"name": {"Bad"}, "upstreamGit": {"garbage"}, "status": {"active"}})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad upstream status %d, want 400", res.StatusCode)
	}
	_ = res.Body.Close()

	// DELETE
	res = postWebForm(t, ts, c, "/projects/"+id+"/delete", url.Values{})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete status %d, want 303", res.StatusCode)
	}
	_ = res.Body.Close()
	if _, err := ps.Get(context.Background(), "u1", id); err == nil {
		t.Errorf("project should be deleted")
	}
}
```

(Add `"net/url"` to the test imports.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/adapter/httpserver/ -run TestWebProjectCreateEditStatusDelete`
Expected: FAIL — routes 404 / `ProjectForm` undefined.

- [ ] **Step 3: Add the form view model + templ**

Append to `internal/adapter/webui/projects.templ`:

```templ
type ProjectFormData struct {
	User  string
	Error string
	Vals  ProjectFormValues
}

type ProjectFormValues struct {
	Name, Slug, Description, UpstreamGit, Status string
	Color, Glyph                                 string
	RateAmount, RateCurrency                     string
}

// ProjectForm renders create (editing==nil) or edit (editing!=nil) of a project.
templ ProjectForm(d ProjectFormData, editing *domain.Project) {
	<!DOCTYPE html>
	<html lang="de">
		<head>
			<meta charset="utf-8"/>
			<meta name="viewport" content="width=device-width, initial-scale=1"/>
			<title>flow · projekt</title>
			<link rel="stylesheet" href="/static/app.css"/>
		</head>
		<body class="mx-auto max-w-2xl p-6 text-slate-900">
			@Nav("projekte", d.User)
			<h2 class="mb-4 text-xl font-semibold">
				if editing != nil {
					Projekt bearbeiten
				} else {
					Neues Projekt
				}
			</h2>
			if d.Error != "" {
				<div class="mb-4 rounded bg-rose-50 px-4 py-2 text-sm text-rose-700">{ d.Error }</div>
			}
			<form method="post" action={ formAction(editing) } hx-boost="false" class="space-y-4 text-sm">
				<div>
					<label class="block text-slate-600">Name</label>
					<input name="name" value={ d.Vals.Name } required class="w-full rounded border border-slate-300 px-2 py-1"/>
				</div>
				<div>
					<label class="block text-slate-600">Slug</label>
					<input name="slug" value={ d.Vals.Slug } class="w-full rounded border border-slate-300 px-2 py-1"/>
				</div>
				<div>
					<label class="block text-slate-600">Beschreibung (Markdown)</label>
					<textarea name="description" rows="6" class="w-full rounded border border-slate-300 px-2 py-1 font-mono">{ d.Vals.Description }</textarea>
				</div>
				<div>
					<label class="block text-slate-600">Upstream Git</label>
					<input name="upstreamGit" value={ d.Vals.UpstreamGit } placeholder="git@github.com:org/repo.git" class="w-full rounded border border-slate-300 px-2 py-1 font-mono"/>
				</div>
				<div>
					<label class="block text-slate-600">Status</label>
					<select name="status" class="rounded border border-slate-300 px-2 py-1">
						@statusOption("active", "aktiv", d.Vals.Status)
						@statusOption("paused", "pausiert", d.Vals.Status)
						@statusOption("archived", "archiviert", d.Vals.Status)
					</select>
				</div>
				<div>
					<label class="block text-slate-600">Farbe</label>
					<div class="flex flex-wrap gap-2">
						@colorRadio("", d.Vals.Color)
						for _, name := range domain.ProjectColors {
							@colorRadio(name, d.Vals.Color)
						}
					</div>
				</div>
				<div>
					<label class="block text-slate-600">Glyph</label>
					<div class="flex flex-wrap gap-2">
						@glyphRadio("", d.Vals.Glyph)
						for _, g := range domain.ProjectGlyphs {
							@glyphRadio(g, d.Vals.Glyph)
						}
					</div>
				</div>
				<div class="flex gap-2">
					<div>
						<label class="block text-slate-600">Satz (optional)</label>
						<input name="rateAmount" value={ d.Vals.RateAmount } placeholder="z. B. 90.00" class="w-32 rounded border border-slate-300 px-2 py-1"/>
					</div>
					<div>
						<label class="block text-slate-600">Währung</label>
						<input name="rateCurrency" value={ orDefault(d.Vals.RateCurrency, "EUR") } class="w-20 rounded border border-slate-300 px-2 py-1"/>
					</div>
				</div>
				<div class="flex gap-2">
					<button class="rounded bg-slate-900 px-4 py-2 text-white hover:bg-slate-700">Speichern</button>
					<a href="/projects" class="rounded border border-slate-300 px-4 py-2 hover:bg-slate-100">Abbrechen</a>
					if editing != nil {
						<form method="post" action={ templ.SafeURL("/projects/" + editing.ID + "/delete") } hx-boost="false" class="ml-auto">
							<button class="rounded border border-rose-300 px-4 py-2 text-rose-700 hover:bg-rose-50">Löschen</button>
						</form>
					}
				</div>
			</form>
		</body>
	</html>
}

templ statusOption(val, label, current string) {
	if val == current || (current == "" && val == "active") {
		<option value={ val } selected>{ label }</option>
	} else {
		<option value={ val }>{ label }</option>
	}
}

templ colorRadio(name, current string) {
	<label class="cursor-pointer">
		if name == current {
			<input type="radio" name="color" value={ name } checked class="peer sr-only"/>
		} else {
			<input type="radio" name="color" value={ name } class="peer sr-only"/>
		}
		if name == "" {
			<span class="inline-flex h-6 w-6 items-center justify-center rounded-full border border-slate-300 text-xs text-slate-400 peer-checked:ring-2 peer-checked:ring-slate-900">∅</span>
		} else {
			<span class="inline-block h-6 w-6 rounded-full ring-offset-1 peer-checked:ring-2 peer-checked:ring-slate-900" style={ "background-color:" + ColorHex(name) }></span>
		}
	</label>
}

templ glyphRadio(g, current string) {
	<label class="cursor-pointer">
		if g == current {
			<input type="radio" name="glyph" value={ g } checked class="peer sr-only"/>
		} else {
			<input type="radio" name="glyph" value={ g } class="peer sr-only"/>
		}
		<span class="inline-flex h-6 w-6 items-center justify-center rounded border border-slate-300 font-mono peer-checked:ring-2 peer-checked:ring-slate-900">
			if g == "" {
				∅
			} else {
				{ g }
			}
		</span>
	</label>
}

func formAction(editing *domain.Project) templ.SafeURL {
	if editing != nil {
		return templ.SafeURL("/projects/" + editing.ID)
	}
	return templ.SafeURL("/projects")
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
```

- [ ] **Step 4: Add the handlers**

Add to `internal/adapter/httpserver/webui_projects.go` (parsing the optional rate, composing create, mapping errors). Confirm `SetProjectRate.Execute`'s signature against `export.go`/the usecase before writing:

```go
import (
	"strconv"
	// ... existing imports: errors, net/http, webui, domain, ports, usecase
)

// parseRate reads the optional rate form fields. Blank amount → (nil, nil) =
// "clear the rate". A malformed amount → error (re-rendered as a form error).
func parseRate(amount, currency string) (*domain.Money, error) {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return nil, nil
	}
	f, err := strconv.ParseFloat(amount, 64)
	if err != nil || f < 0 {
		return nil, fmt.Errorf("ungültiger Satz %q", amount)
	}
	cur := strings.TrimSpace(currency)
	if cur == "" {
		cur = "EUR"
	}
	return &domain.Money{Amount: int64(f*100 + 0.5), Currency: cur}, nil // domain.Money.Amount is minor units (cents): 90.00 -> 9000
}

func formValues(r *http.Request) webui.ProjectFormValues {
	return webui.ProjectFormValues{
		Name: r.FormValue("name"), Slug: r.FormValue("slug"), Description: r.FormValue("description"),
		UpstreamGit: r.FormValue("upstreamGit"), Status: r.FormValue("status"),
		Color: r.FormValue("color"), Glyph: r.FormValue("glyph"),
		RateAmount: r.FormValue("rateAmount"), RateCurrency: r.FormValue("rateCurrency"),
	}
}

func (s *Server) handleWebProjectNew(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = webui.ProjectForm(webui.ProjectFormData{User: u.DisplayName, Vals: webui.ProjectFormValues{Status: "active"}}, nil).Render(r.Context(), w)
}

func (s *Server) handleWebProjectCreate(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	vals := formValues(r)
	rate, rerr := parseRate(vals.RateAmount, vals.RateCurrency)
	reRender := func(msg string) {
		w.WriteHeader(http.StatusBadRequest)
		_ = webui.ProjectForm(webui.ProjectFormData{User: u.DisplayName, Error: msg, Vals: vals}, nil).Render(r.Context(), w)
	}
	if vals.Name == "" {
		reRender("Name erforderlich")
		return
	}
	if rerr != nil {
		reRender(rerr.Error())
		return
	}
	// create (name/slug/color/glyph) — same signature as REST
	p, err := s.CreateProject.Execute(r.Context(), u.ID, vals.Name, vals.Slug, vals.Color, vals.Glyph)
	if err != nil {
		reRender("Konnte Projekt nicht anlegen")
		return
	}
	// compose description/upstream/status (auto-syncs binding; validates)
	p, err = s.UpdateProject.Execute(r.Context(), u.ID, p.ID, usecase.UpdateProjectInput{
		Name: p.Name, Slug: p.Slug, Color: vals.Color, Glyph: vals.Glyph,
		Description: vals.Description, UpstreamGit: vals.UpstreamGit,
		Status: domain.ProjectStatus(orStatus(vals.Status)),
	})
	if err != nil {
		// surface validation/upstream errors as a 400 form (project exists but
		// metadata rejected — rare; user can retry the edit)
		reRender(err.Error())
		return
	}
	if rate != nil {
		_ = s.SetProjectRate.Execute(r.Context(), u.ID, p.ID, rate) // adapt signature
	}
	s.Bus.Publish(domain.Event{Type: domain.EventProjectCreated, UserID: u.ID, Data: map[string]any{"id": p.ID}})
	http.Redirect(w, r, "/projects/"+p.ID, http.StatusSeeOther)
}

func (s *Server) handleWebProjectEdit(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	p, err := s.GetProject.Execute(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	vals := webui.ProjectFormValues{
		Name: p.Name, Slug: p.Slug, Description: p.Description, UpstreamGit: p.UpstreamGit,
		Status: string(p.Status), Color: p.Color, Glyph: p.Glyph,
	}
	if p.Rate != nil {
		vals.RateAmount = fmt.Sprintf("%d.%02d", p.Rate.Amount/100, p.Rate.Amount%100) // cents -> "90.00" (Money.String appends currency, so don't use it here)
		vals.RateCurrency = p.Rate.Currency
	}
	_ = webui.ProjectForm(webui.ProjectFormData{User: u.DisplayName, Vals: vals}, &p).Render(r.Context(), w)
}

func (s *Server) handleWebProjectUpdate(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	vals := formValues(r)
	rate, rerr := parseRate(vals.RateAmount, vals.RateCurrency)
	cur, gerr := s.GetProject.Execute(r.Context(), u.ID, id)
	if gerr != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	reRender := func(msg string) {
		w.WriteHeader(http.StatusBadRequest)
		_ = webui.ProjectForm(webui.ProjectFormData{User: u.DisplayName, Error: msg, Vals: vals}, &cur).Render(r.Context(), w)
	}
	if rerr != nil {
		reRender(rerr.Error())
		return
	}
	p, err := s.UpdateProject.Execute(r.Context(), u.ID, id, usecase.UpdateProjectInput{
		Name: vals.Name, Slug: vals.Slug, Color: vals.Color, Glyph: vals.Glyph,
		Description: vals.Description, UpstreamGit: vals.UpstreamGit,
		Status: domain.ProjectStatus(orStatus(vals.Status)),
	})
	switch {
	case errors.Is(err, ports.ErrProjectNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case errors.Is(err, domain.ErrInvalidProject) || errors.Is(err, domain.ErrInvalidUpstream):
		reRender(err.Error())
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	_ = s.SetProjectRate.Execute(r.Context(), u.ID, id, rate) // rate==nil clears
	s.Bus.Publish(domain.Event{Type: domain.EventProjectUpdated, UserID: u.ID, Data: map[string]any{"id": p.ID}})
	http.Redirect(w, r, "/projects/"+id, http.StatusSeeOther)
}

// handleWebProjectStatus applies a single status transition (full-replace
// UpdateProject preserving current fields).
func (s *Server) handleWebProjectStatus(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	cur, err := s.GetProject.Execute(r.Context(), u.ID, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	_, err = s.UpdateProject.Execute(r.Context(), u.ID, id, usecase.UpdateProjectInput{
		Name: cur.Name, Slug: cur.Slug, Color: cur.Color, Glyph: cur.Glyph,
		Description: cur.Description, UpstreamGit: cur.UpstreamGit,
		Status: domain.ProjectStatus(r.FormValue("status")),
	})
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventProjectUpdated, UserID: u.ID, Data: map[string]any{"id": id}})
	http.Redirect(w, r, "/projects/"+id, http.StatusSeeOther)
}

func (s *Server) handleWebProjectDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	if err := s.DeleteProject.Execute(r.Context(), u.ID, id); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventProjectDeleted, UserID: u.ID, Data: map[string]any{"id": id}})
	http.Redirect(w, r, "/projects", http.StatusSeeOther)
}

// orStatus defaults an empty status form value to "active".
func orStatus(s string) string {
	if s == "" {
		return "active"
	}
	return s
}
```

> CONFIRMED (no need to re-verify): `domain.Money.Amount` is minor units (cents), int64 (`money.go:12`); `SetProjectRate.Execute(ctx, ownerID, projectID string, rate *domain.Money) error` (`set_project_rate.go:19`) — `rate == nil` clears. The implementer still confirms the `userFrom` return shape + `User.DisplayName` field name (copy from the docs webui handlers) and adds imports (`fmt`, `strconv`, `strings`, `usecase`, `errors`, `ports`).

- [ ] **Step 5: Register the routes**

In `server.go` (literal `/projects/new` BEFORE relying on `{id}`; method-specific so order is not required but keep grouped):

```go
	mux.Handle("GET /projects/new", s.webAuth(http.HandlerFunc(s.handleWebProjectNew)))
	mux.Handle("POST /projects", s.webAuth(http.HandlerFunc(s.handleWebProjectCreate)))
	mux.Handle("GET /projects/{id}/edit", s.webAuth(http.HandlerFunc(s.handleWebProjectEdit)))
	mux.Handle("POST /projects/{id}", s.webAuth(http.HandlerFunc(s.handleWebProjectUpdate)))
	mux.Handle("POST /projects/{id}/status", s.webAuth(http.HandlerFunc(s.handleWebProjectStatus)))
	mux.Handle("POST /projects/{id}/delete", s.webAuth(http.HandlerFunc(s.handleWebProjectDelete)))
```

- [ ] **Step 6: Generate, test, build, lint**

Run: `go tool templ generate && go test ./internal/adapter/httpserver/ -run 'TestWebProject' && go build ./... && golangci-lint run`
Expected: all project webui tests PASS; clean.

- [ ] **Step 7: Commit**

```bash
git add internal/adapter/webui/projects.templ internal/adapter/webui/projects_templ.go internal/adapter/httpserver/webui_projects.go internal/adapter/httpserver/server.go internal/adapter/httpserver/webui_projects_test.go
git commit -m "feat(project-mgmt): webui project create/edit form + status/delete/rate handlers"
```

---

### Task 7: Recompile Tailwind (`make web`) + commit `app.css`

**Files:**
- Modify: `internal/adapter/webui/static/app.css` (regenerated)

- [ ] **Step 1: Recompile the CSS**

Run: `make web`
Expected: Tailwind scans `internal/adapter/webui/**/*.templ` (now including `projects.templ` + `nav.templ`) and rewrites `internal/adapter/webui/static/app.css` with the utility classes the new templates use (e.g. `bg-emerald-100`, `text-amber-700`, `opacity-70`, `ring-2`, `peer-checked:*`, `prose`, etc.).

If `make web` fails because the Tailwind toolchain is not installed locally: STOP and report it — the implementer/controller resolves the toolchain (or the controller runs it). Do NOT hand-edit `app.css`.

- [ ] **Step 2: Verify the new classes are present**

Run: `rg -c "emerald-100|peer-checked|opacity-70" internal/adapter/webui/static/app.css`
Expected: non-zero counts (the new classes were compiled in).

- [ ] **Step 3: Build + the full gate**

Run: `make ci`
Expected: green (lint + verify-generate + cover + build). `verify-generate` confirms the committed `_templ.go` matches the `.templ` sources.

- [ ] **Step 4: Commit**

```bash
git add internal/adapter/webui/static/app.css
git commit -m "chore(project-mgmt): recompile Tailwind app.css for projects webui"
```

---

### Task 8: Wiring verification + live done-gate (controller)

**Files:** none (verification only). If a gap is found, fix it in the relevant task's files and re-commit.

- [ ] **Step 1: Full CI gate**

Run: `make ci` (with `DOCKER_HOST` pointed at the podman socket so pgstore tests run, as in Slice 1).
Expected: lint + verify-generate + cover (≥ ~80%) + build all green.

- [ ] **Step 2: Confirm the routes are registered**

Run: `rg -n "GET /projects|POST /projects|/ui/projects/list" internal/adapter/httpserver/server.go`
Expected: all 8 webui project routes present (`GET /projects`, `GET /ui/projects/list`, `GET /projects/new`, `POST /projects`, `GET /projects/{id}`, `GET /projects/{id}/edit`, `POST /projects/{id}`, `POST /projects/{id}/status`, `POST /projects/{id}/delete`).

- [ ] **Step 3: Bring up the dev stack + log in**

`make dev-up`; `make dev-run` (background); browse `https://localhost:8080/projects` (accept the self-signed cert; log in `msoent@dev.local` / `password`).

- [ ] **Step 4: Browser dogfood (the done-gate)**

Verify in the browser:
- `/projects` shows the list; default hides archived; the `archiviert` / `alle` filter chips work; "Neues Projekt" opens the form.
- Create a project with a color + glyph + upstream + rate → redirected to its cockpit; the cockpit shows glyph+color, status badge, rendered description, the Git section, the worktime panel (with `Σh × Rate = €` once a session is booked), and the auto-synced remote binding in the Bindings panel.
- Edit it: change description + status active→paused→archived; confirm the badge dims, it leaves the default list, and reappears under `archiviert`.
- Use the cockpit status buttons (Pausieren/Fortsetzen/Archivieren) → status changes, SSE updates the view.
- Clear the rate (blank amount) on edit → the `€` disappears from the cockpit.
- Bad upstream in the form → inline rose error, no redirect.
- Delete → back to the list, project gone.
- A name-only project shows no Git section and no `€`.

- [ ] **Step 5: Final commit (only if Steps 1–4 required fixes)**

```bash
git add -A
git commit -m "chore(project-mgmt): slice-2 done-gate fixes"
```

---

## Self-Review

**1. Spec coverage (Slice 2 = WebUI scope):**
- `/projects` overview, cards (glyph+color+name+status+upstream), status filter (active+paused default, archived toggle), "Neues Projekt" → Task 4. ✓
- `/projects/{slug}` cockpit: header + actions, rendered description, git section (only if upstream), worktime panel (Σh + week/month + Σh×Rate), documents panel, bindings panel → Task 5. ✓
- Create/Edit form: name·slug·description·upstream·color(palette)·glyph(whitelist)·rate·status → Task 6. ✓
- nav entry "Projekte" → Task 2. ✓
- Color/glyph as a shared single-source consumed by WebUI now + TUI later → Task 1 (domain whitelist) + Task 3 (webui hex map) + drift-guard. ✓
- frontend-design skill invoked for visual execution → noted in header + per-templ tasks. ✓

**2. Placeholder scan:** Load-bearing Go (domain, helpers, builders, handlers, routes, view models, tests) is complete and verbatim. The templ is complete and compiling (mirrors `docs.templ`); aesthetic refinement is explicitly delegated to `frontend-design` within the fixed structure — not a placeholder. Three implementer-verify points are flagged inline (NOT left as TODO): `domain.Money` unit + `SetProjectRate` signature, the worktime-aggregate idiom (copied from `webui_worktime.go`), and `ListDocumentsScoped`/`ListProjectBindings` exact names — each names the file to copy from.

**3. Type consistency:** `ProjectFormValues`/`ProjectFormData`/`ProjectsPageData`/`ProjectCockpit` defined in Task 4–6 and consumed by their handlers; `usecase.UpdateProjectInput` (Status `domain.ProjectStatus`) fed via `domain.ProjectStatus(req)` at the form boundary; `domain.ProjectColors`/`ProjectGlyphs` produced in Task 1 and consumed by Task 3 (hex map keys) + Task 6 (pickers); `webui.ColorHex`/`StatusBadge` produced in Task 3 and consumed by Tasks 4–5 templ. Routes registered in Task 4/5/6 match the handler names.

**Known limitation (carried from Slice 1 + repeated here):** create-compose is multiple writes (CreateProject → UpdateProject → optional SetProjectRate); an infra failure between them can leave a partially-configured project. Upstream is validated before create, so the common bad-input path fails fast; the residual is a rare, recoverable state (the user re-opens the edit form). Acceptable for the single-user homelab design.

**Deferred to later slices (not this plan):** TUI projects screen + `gitworktree`/`clientcheckout` (Slice 3 — will consume `domain.ProjectColors`/`ProjectGlyphs` via `theme`); CLI verbs (Slice 4); TUI session-edit project picker (Slice 5).
