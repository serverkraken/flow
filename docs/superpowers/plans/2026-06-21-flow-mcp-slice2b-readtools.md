# flow-mcp Slice 2b (Kompendium Read Tools) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the five Kompendium **read** tools — `flow_search_docs`, `flow_list_docs`, `flow_get_doc`, `flow_list_tags`, `flow_backlinks` — to the flow-mcp spine, each a thin client-side wrapper over an existing `apiclient` method, project-scoped through a robust ref-resolver.

**Architecture:** A **pure client-side** MCP tool layer over `cmd/flow-mcp`. Slice 1 already shipped the server-side `projectId` filtering (spec §8), so 2b touches **only `cmd/flow-mcp`** (plus one tiny single-source helper in `domain`). The one genuinely new piece is a **scope resolver** that maps each tool's optional `project` argument (`""` → cwd-resolved default · `"global"` → all · `"none"` → unassigned · else an id/slug/name looked up against a cached `ListProjects`, refreshed once on a miss) to the `projectID *string` the apiclient wants — never silently returning 0 rows for an unknown ref. Every tool reuses the 2a `handlers` struct, `textResult`/`errorResult`/`loginRequired` helpers, and the always-registered + degraded-mode invariants.

**Tech Stack:** Go 1.26.x, `github.com/modelcontextprotocol/go-sdk/mcp` v1.6.1 (already in go.mod from 2a), existing `internal/adapter/apiclient`, `internal/domain`.

## Global Constraints

- Go module `github.com/serverkraken/flow`.
- **stdout is reserved for the JSON-RPC stream. ALL logging goes to stderr.** This slice adds no logging, but never introduce a `fmt.Println`/`log.*`/default-logger write that could reach stdout.
- **No interactive auth / never crash.** Every tool short-circuits when unauthenticated: `if !h.authed { return h.loginRequired(), nil, nil }` (text `"Login required: run 'flow login' in a terminal on this device."`). The 11/6 tools are always registered so `tools/list` is stable regardless of auth.
- **`projectID *string` convention** (Slice 1): `nil` → all; `"none"` → unassigned (`project_id IS NULL`); else a project **id** (equality). **The MCP `"global"` sentinel must be mapped to `nil` BEFORE calling apiclient** — never pass `"global"` (or a slug) through as a project id, or the backend returns 0 rows. The resolver is the single place this mapping happens.
- **No backend / server / pgstore / apiclient / migration change.** Slice 1 delivered spec §8 (`ListDocumentsScoped`/`SearchScoped`). 2b is additive over `cmd/flow-mcp` only, except the single-source `domain.DocumentTypes()` helper in Task 1.
- **CI gate is `golangci-lint run`** (+ `verify-generate` + `cover` + `build`), NOT plain gofmt. `make fmt` is manual/ungated. Verify with `golangci-lint run`, not `gofmt -l` (local gofmt is go1.26.4 and reports pre-existing committed files as false positives).
- **pgstore/other Docker tests** in the full suite need `export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"` and `export TESTCONTAINERS_RYUK_DISABLED=true`. This slice adds no Docker tests, but `make ci`/`cover` runs the whole suite, so set this env when running the full gate.
- Result text is concise plain text (the model reads it); errors set `IsError: true` with an actionable message.
- Commit message trailers (every commit):
  ```
  Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01GTHf7tSzKearihWVzL6i4n
  ```

**As-built interfaces this slice consumes (verified against the codebase — do NOT re-derive):**
- `apiclient.Client.ListDocumentsScoped(ctx, projectID *string, tags ...string) ([]domain.Document, error)` → `GET /api/v1/documents?projectId=&tag=`
- `apiclient.Client.SearchScoped(ctx, q string, projectID *string, tags ...string) ([]domain.SearchHit, error)` → `GET /api/v1/documents?q=&projectId=&tag=`
- `apiclient.Client.GetDocument(ctx, id string) (domain.Document, error)` → `GET /api/v1/documents/{id}`
- `apiclient.Client.Tags(ctx) ([]domain.TagCount, error)` → `GET /api/v1/documents/tags` (global, owner-wide)
- `apiclient.Client.Backlinks(ctx, id string) ([]domain.BacklinkRef, error)` → `GET /api/v1/documents/{id}/backlinks`
- `apiclient.Client.ListProjects(ctx) ([]domain.Project, error)` → `GET /api/v1/projects`
- `domain.SearchHit{ Document; Snippet string }` (embeds Document → `.ID/.Title/.Path/.Type/.Tags`)
- `domain.TagCount{ Tag string; Count int }`; `domain.BacklinkRef{ ID, Path, Title string; Type DocumentType }`
- `domain.Document{ ID, OwnerID string; ProjectID *string; Type DocumentType; Path, Title, Body string; Tags []string; Role *string; ... }`
- `domain.Project{ ID, Name, Slug string; ... }`
- `domain.CollectTags(docs []domain.Document) []domain.TagCount` (aggregates frontmatter tags)
- 2a spine (in `cmd/flow-mcp`): `handlers{ client *apiclient.Client; authed bool; proj domain.Project; matched bool }`; `newServer(client *apiclient.Client, authed bool, proj domain.Project, matched bool) *mcp.Server`; `textResult(s)`, `errorResult(s)`, `(*handlers).loginRequired()`; tools registered via `mcp.AddTool(s, &mcp.Tool{Name, Description}, h.handler)` with handler signature `func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error)`.
- go-sdk typed params: fields are inferred from the `In` struct; descriptions from the `jsonschema:"..."` tag; a field is **required** unless its `json` tag carries `,omitempty`.

---

### Task 1: Read-tool support layer (scope resolver, type filter, result formatters)

The pure, unit-tested foundation the five tools stand on: the project-ref **scope resolver** (the slice's one real algorithm), the `type`-filter validator, and the result-text formatters. No tools are registered in this task.

**Files:**
- Modify: `internal/domain/document.go` (add `DocumentTypes()`, refactor `valid()` to use it — single-source the type set)
- Create: `internal/domain/document_types_test.go`
- Modify: `cmd/flow-mcp/server.go` (extend `handlers` with the project-ref cache + `listProjects` seam; wire the seam in `newServer`)
- Create: `cmd/flow-mcp/scope.go` (`scope`, `resolveScope`, `projectList`, `matchProject`, `slugList`, `projectName`, `checkType`, `typeList`)
- Create: `cmd/flow-mcp/scope_test.go`
- Create: `cmd/flow-mcp/format.go` (`formatDocLine`, `formatDocList`, `formatSearchHits`, `formatDoc`, `formatTags`, `formatBacklinks`)
- Create: `cmd/flow-mcp/format_test.go`

**Interfaces:**
- Consumes: `apiclient.Client.ListProjects`; `domain.Project/Document/SearchHit/TagCount/BacklinkRef/CollectTags`; the 2a `handlers` struct.
- Produces (consumed by Task 2):
  - `type scope struct { projectID *string; label string }`
  - `(*handlers).resolveScope(ctx context.Context, project string) (scope, error)`
  - `(*handlers).projectList(ctx context.Context, refresh bool) ([]domain.Project, error)`
  - `(*handlers).projectName(ctx context.Context, id *string) string`
  - `checkType(typ string) (domain.DocumentType, error)`
  - `formatDocList(docs []domain.Document, sc scope) string`, `formatSearchHits(hits []domain.SearchHit, query string, sc scope) string`, `formatDoc(d domain.Document, projectName string) string`, `formatTags(tags []domain.TagCount, sc scope) string`, `formatBacklinks(refs []domain.BacklinkRef, label string) string`
  - `handlers` gains fields: `projMu sync.Mutex`, `projects []domain.Project`, `projFetched bool`, `listProjects func(ctx context.Context) ([]domain.Project, error)`.

- [ ] **Step 1: Single-source the document-type set in `domain` (write the test first)**

Create `internal/domain/document_types_test.go`:

```go
package domain

import "testing"

func TestDocumentTypesAllValid(t *testing.T) {
	ts := DocumentTypes()
	if len(ts) != 8 {
		t.Fatalf("DocumentTypes() returned %d types, want 8", len(ts))
	}
	for _, dt := range ts {
		if !dt.valid() {
			t.Errorf("DocumentTypes() includes %q but valid() rejects it", dt)
		}
	}
	if DocumentType("bogus").valid() {
		t.Error("valid() accepted a bogus type")
	}
}
```

- [ ] **Step 2: Run the test — it fails to compile (RED)**

Run: `go test ./internal/domain/ -run TestDocumentTypesAllValid`
Expected: FAIL — `undefined: DocumentTypes`.

- [ ] **Step 3: Add `DocumentTypes()` and refactor `valid()`**

In `internal/domain/document.go`, replace the existing `valid()` method:

```go
func (t DocumentType) valid() bool {
	switch t {
	case DocDaily, DocProject, DocFree, DocAgent, DocMemory, DocInstruction, DocSkill, DocPlan:
		return true
	}
	return false
}
```

with a list-backed version plus the new exported helper:

```go
// DocumentTypes returns every valid document type in canonical order. It is the
// single source of truth for the type set; valid() and external validators
// (flow-mcp's type filter) both derive from it, so a new type is added here once.
func DocumentTypes() []DocumentType {
	return []DocumentType{
		DocDaily, DocProject, DocFree, DocAgent,
		DocMemory, DocInstruction, DocSkill, DocPlan,
	}
}

func (t DocumentType) valid() bool {
	for _, v := range DocumentTypes() {
		if t == v {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run the domain tests (GREEN)**

Run: `go test ./internal/domain/`
Expected: PASS — the new test passes and every pre-existing `valid()`-based test still passes (behavior is identical).

- [ ] **Step 5: Extend the `handlers` struct + wire the `listProjects` seam**

In `cmd/flow-mcp/server.go`, add `"context"` and `"sync"` to the imports, replace the `handlers` struct, and set the seam in `newServer`.

Struct (replaces the existing `handlers` definition):

```go
// handlers carries the dependencies every tool needs: the authenticated client,
// whether auth succeeded at boot, the cwd-resolved project, and a lazily-fetched
// project-ref cache used to resolve explicit `project` arguments (slug/name/id).
type handlers struct {
	client  *apiclient.Client
	authed  bool
	proj    domain.Project
	matched bool

	// project-ref cache, guarded by projMu. listProjects is the fetch seam
	// (defaults to client.ListProjects; overridable in unit tests).
	projMu       sync.Mutex
	projects     []domain.Project
	projFetched  bool
	listProjects func(ctx context.Context) ([]domain.Project, error)
}
```

In `newServer`, after constructing `h`, wire the seam (guard against a nil client in degraded mode — the seam is only ever called on the authed path, but forming the value defensively keeps it obvious):

```go
func newServer(client *apiclient.Client, authed bool, proj domain.Project, matched bool) *mcp.Server {
	h := &handlers{client: client, authed: authed, proj: proj, matched: matched}
	if client != nil {
		h.listProjects = client.ListProjects
	}
	s := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: serverVersion}, nil)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_project_context",
		Description: "Report which flow project the current working directory resolves to, and how many Kompendium documents are in scope. Call this first to orient.",
	}, h.projectContext)
	return s
}
```

- [ ] **Step 6: Write `scope_test.go` (failing) for the resolver, type filter, and project cache**

Create `cmd/flow-mcp/scope_test.go`:

```go
package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func fakeProjects() []domain.Project {
	return []domain.Project{
		{ID: "p1", Name: "Alpha", Slug: "alpha"},
		{ID: "p2", Name: "Beta", Slug: "beta"},
	}
}

func TestResolveScope_DefaultUsesMatchedProject(t *testing.T) {
	h := &handlers{matched: true, proj: domain.Project{ID: "p1", Name: "Alpha"}}
	sc, err := h.resolveScope(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if sc.projectID == nil || *sc.projectID != "p1" {
		t.Fatalf("projectID = %v, want &\"p1\"", sc.projectID)
	}
	if !strings.Contains(sc.label, "Alpha") {
		t.Fatalf("label = %q, want it to mention Alpha", sc.label)
	}
}

func TestResolveScope_DefaultUnmatchedIsGlobal(t *testing.T) {
	h := &handlers{matched: false}
	sc, err := h.resolveScope(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if sc.projectID != nil {
		t.Fatalf("projectID = %v, want nil (global)", sc.projectID)
	}
}

func TestResolveScope_GlobalAndNoneSentinels(t *testing.T) {
	h := &handlers{matched: true, proj: domain.Project{ID: "p1"}}
	g, _ := h.resolveScope(context.Background(), "global")
	if g.projectID != nil {
		t.Fatalf("global projectID = %v, want nil", g.projectID)
	}
	n, _ := h.resolveScope(context.Background(), "none")
	if n.projectID == nil || *n.projectID != "none" {
		t.Fatalf("none projectID = %v, want &\"none\"", n.projectID)
	}
}

func TestResolveScope_ExplicitBySlugAndName(t *testing.T) {
	calls := 0
	h := &handlers{listProjects: func(context.Context) ([]domain.Project, error) {
		calls++
		return fakeProjects(), nil
	}}
	bySlug, err := h.resolveScope(context.Background(), "beta")
	if err != nil {
		t.Fatal(err)
	}
	if bySlug.projectID == nil || *bySlug.projectID != "p2" {
		t.Fatalf("by slug = %v, want &\"p2\"", bySlug.projectID)
	}
	byName, err := h.resolveScope(context.Background(), "Alpha")
	if err != nil {
		t.Fatal(err)
	}
	if byName.projectID == nil || *byName.projectID != "p1" {
		t.Fatalf("by name = %v, want &\"p1\"", byName.projectID)
	}
	if calls != 1 {
		t.Fatalf("listProjects called %d times, want 1 (cached after first fetch)", calls)
	}
}

func TestResolveScope_UnknownRefreshesOnceThenErrors(t *testing.T) {
	calls := 0
	h := &handlers{listProjects: func(context.Context) ([]domain.Project, error) {
		calls++
		return fakeProjects(), nil // never contains "gamma"
	}}
	_, err := h.resolveScope(context.Background(), "gamma")
	if err == nil {
		t.Fatal("expected an error for an unknown project")
	}
	if !strings.Contains(err.Error(), "gamma") || !strings.Contains(err.Error(), "alpha") {
		t.Fatalf("error %q should name the bad ref and list known slugs", err)
	}
	if calls != 2 {
		t.Fatalf("listProjects called %d times, want 2 (initial + one refresh on miss)", calls)
	}
}

func TestResolveScope_NewlyCreatedFoundAfterRefresh(t *testing.T) {
	calls := 0
	h := &handlers{listProjects: func(context.Context) ([]domain.Project, error) {
		calls++
		if calls == 1 {
			return fakeProjects(), nil // gamma not yet visible
		}
		return append(fakeProjects(), domain.Project{ID: "p3", Name: "Gamma", Slug: "gamma"}), nil
	}}
	sc, err := h.resolveScope(context.Background(), "gamma")
	if err != nil {
		t.Fatal(err)
	}
	if sc.projectID == nil || *sc.projectID != "p3" {
		t.Fatalf("projectID = %v, want &\"p3\" after refresh", sc.projectID)
	}
}

func TestResolveScope_ListProjectsError(t *testing.T) {
	h := &handlers{listProjects: func(context.Context) ([]domain.Project, error) {
		return nil, errors.New("boom")
	}}
	_, err := h.resolveScope(context.Background(), "beta")
	if err == nil {
		t.Fatal("expected the underlying list error to surface")
	}
}

func TestProjectName(t *testing.T) {
	h := &handlers{listProjects: func(context.Context) ([]domain.Project, error) {
		return fakeProjects(), nil
	}}
	p1 := "p1"
	if got := h.projectName(context.Background(), &p1); got != "Alpha" {
		t.Fatalf("projectName(&p1) = %q, want Alpha", got)
	}
	if got := h.projectName(context.Background(), nil); got != "" {
		t.Fatalf("projectName(nil) = %q, want \"\"", got)
	}
	unknown := "pX"
	if got := h.projectName(context.Background(), &unknown); got != "" {
		t.Fatalf("projectName(unknown) = %q, want \"\"", got)
	}
}

func TestCheckType(t *testing.T) {
	if got, err := checkType(""); err != nil || got != "" {
		t.Fatalf("checkType(\"\") = (%q,%v), want (\"\",nil)", got, err)
	}
	if got, err := checkType("memory"); err != nil || got != domain.DocMemory {
		t.Fatalf("checkType(\"memory\") = (%q,%v), want (memory,nil)", got, err)
	}
	_, err := checkType("bogus")
	if err == nil || !strings.Contains(err.Error(), "memory") {
		t.Fatalf("checkType(\"bogus\") err = %v, want it to list valid types", err)
	}
}
```

- [ ] **Step 7: Run scope tests to verify they fail (RED)**

Run: `go test ./cmd/flow-mcp/ -run 'ResolveScope|CheckType'`
Expected: FAIL — `undefined: resolveScope` / `undefined: checkType` (compile error).

- [ ] **Step 8: Write `cmd/flow-mcp/scope.go`**

```go
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
)

// scope is a tool call's resolved project filter: the apiclient projectID pointer
// (nil → all projects, "none" → unassigned, &id → one project) plus a human label
// for result text.
type scope struct {
	projectID *string
	label     string
}

// resolveScope maps a tool's optional `project` argument to a scope. Accepts:
// "" (use the cwd-resolved project, or global if none is bound), "global" (all
// projects), "none" (unassigned documents), or a project id / slug / name (looked
// up against the cached project list, refreshed once on a miss). An unknown
// reference returns an error whose message is shown to the model — never a silent
// empty result.
func (h *handlers) resolveScope(ctx context.Context, project string) (scope, error) {
	switch p := strings.TrimSpace(project); p {
	case "":
		if h.matched {
			id := h.proj.ID
			return scope{projectID: &id, label: "in project " + h.proj.Name}, nil
		}
		return scope{projectID: nil, label: "across all projects (no project is bound to this directory — use flow_bind_project)"}, nil
	case "global":
		return scope{projectID: nil, label: "across all projects"}, nil
	case "none":
		none := "none"
		return scope{projectID: &none, label: "among unassigned documents"}, nil
	default:
		proj, err := h.lookupProject(ctx, p)
		if err != nil {
			return scope{}, err
		}
		id := proj.ID
		return scope{projectID: &id, label: "in project " + proj.Name}, nil
	}
}

// lookupProject finds a project by id, slug, or name (case-insensitive for slug
// and name). On a miss it refreshes the cache once — to catch a just-created
// project — then returns an actionable error listing the known slugs.
func (h *handlers) lookupProject(ctx context.Context, ref string) (domain.Project, error) {
	ps, err := h.projectList(ctx, false)
	if err != nil {
		return domain.Project{}, fmt.Errorf("flow server error listing projects: %w", err)
	}
	if p, ok := matchProject(ps, ref); ok {
		return p, nil
	}
	ps, err = h.projectList(ctx, true) // refresh once, then retry
	if err != nil {
		return domain.Project{}, fmt.Errorf("flow server error listing projects: %w", err)
	}
	if p, ok := matchProject(ps, ref); ok {
		return p, nil
	}
	return domain.Project{}, fmt.Errorf("unknown project %q. Use 'global' (all projects), 'none' (unassigned), or a known slug: %s", ref, slugList(ps))
}

// projectList returns the cached project list, fetching it once via the seam.
// refresh=true forces a re-fetch.
func (h *handlers) projectList(ctx context.Context, refresh bool) ([]domain.Project, error) {
	h.projMu.Lock()
	defer h.projMu.Unlock()
	if h.projFetched && !refresh {
		return h.projects, nil
	}
	ps, err := h.listProjects(ctx)
	if err != nil {
		return nil, err
	}
	h.projects = ps
	h.projFetched = true
	return ps, nil
}

// projectName best-effort resolves a project id to its name via the cache;
// returns "" when id is nil or unknown.
func (h *handlers) projectName(ctx context.Context, id *string) string {
	if id == nil {
		return ""
	}
	ps, err := h.projectList(ctx, false)
	if err != nil {
		return ""
	}
	for _, p := range ps {
		if p.ID == *id {
			return p.Name
		}
	}
	return ""
}

func matchProject(ps []domain.Project, ref string) (domain.Project, bool) {
	for _, p := range ps {
		if p.ID == ref || strings.EqualFold(p.Slug, ref) || strings.EqualFold(p.Name, ref) {
			return p, true
		}
	}
	return domain.Project{}, false
}

func slugList(ps []domain.Project) string {
	if len(ps) == 0 {
		return "(none)"
	}
	s := make([]string, len(ps))
	for i, p := range ps {
		s[i] = p.Slug
	}
	return strings.Join(s, ", ")
}

// checkType validates an optional `type` filter argument against the canonical
// document-type set. "" → no filter (returns ""). An invalid value is an error
// listing the valid types (not a silent empty result).
func checkType(typ string) (domain.DocumentType, error) {
	t := strings.TrimSpace(typ)
	if t == "" {
		return "", nil
	}
	for _, v := range domain.DocumentTypes() {
		if domain.DocumentType(t) == v {
			return v, nil
		}
	}
	return "", fmt.Errorf("invalid type %q. Valid types: %s", t, typeList())
}

func typeList() string {
	ts := domain.DocumentTypes()
	s := make([]string, len(ts))
	for i, t := range ts {
		s[i] = string(t)
	}
	return strings.Join(s, ", ")
}
```

- [ ] **Step 9: Run scope tests (GREEN)**

Run: `go test ./cmd/flow-mcp/ -run 'ResolveScope|CheckType' -v`
Expected: PASS — all scope/type tests pass.

- [ ] **Step 10: Write `format_test.go` (failing) for the result formatters**

Create `cmd/flow-mcp/format_test.go`:

```go
package main

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func strp(s string) *string { return &s }

func TestFormatDocList(t *testing.T) {
	docs := []domain.Document{
		{ID: "d1", Title: "Arch", Path: "notes/arch", Type: domain.DocMemory, Tags: []string{"go", "design"}},
	}
	out := formatDocList(docs, scope{label: "in project Alpha"})
	for _, want := range []string{"1 document", "Alpha", "d1", "Arch", "notes/arch", "memory", "go, design"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatDocList missing %q in:\n%s", want, out)
		}
	}
	if empty := formatDocList(nil, scope{label: "in project Alpha"}); !strings.HasPrefix(empty, "No documents") {
		t.Errorf("empty list = %q, want a 'No documents' message", empty)
	}
}

func TestFormatSearchHits(t *testing.T) {
	hits := []domain.SearchHit{
		{Document: domain.Document{ID: "d1", Title: "Arch", Path: "notes/arch", Type: domain.DocMemory}, Snippet: "the needle here"},
	}
	out := formatSearchHits(hits, "needle", scope{label: "in project Alpha"})
	for _, want := range []string{"1 match", "needle", "d1", "the needle here"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatSearchHits missing %q in:\n%s", want, out)
		}
	}
	if empty := formatSearchHits(nil, "needle", scope{label: "in project Alpha"}); !strings.Contains(empty, "No matches") {
		t.Errorf("empty search = %q, want a 'No matches' message", empty)
	}
}

func TestFormatDoc(t *testing.T) {
	d := domain.Document{ID: "d1", Title: "Arch", Path: "notes/arch", Type: domain.DocMemory, Body: "BODY", Tags: []string{"go"}, Role: strp("brief")}
	out := formatDoc(d, "Alpha")
	for _, want := range []string{"Arch", "notes/arch", "memory", "Alpha", "go", "brief", "d1", "BODY"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatDoc missing %q in:\n%s", want, out)
		}
	}
}

func TestFormatTags(t *testing.T) {
	tags := []domain.TagCount{{Tag: "go", Count: 1}, {Tag: "design", Count: 3}}
	out := formatTags(tags, scope{label: "in project Alpha"})
	// highest count first
	if strings.Index(out, "design") > strings.Index(out, "go") {
		t.Errorf("formatTags should sort by count desc:\n%s", out)
	}
	if empty := formatTags(nil, scope{label: "in project Alpha"}); !strings.Contains(empty, "No tags") {
		t.Errorf("empty tags = %q, want a 'No tags' message", empty)
	}
}

func TestFormatBacklinks(t *testing.T) {
	refs := []domain.BacklinkRef{{ID: "d2", Title: "Todo", Path: "notes/todo", Type: domain.DocFree}}
	out := formatBacklinks(refs, "notes/arch")
	for _, want := range []string{"1 document", "notes/arch", "d2", "Todo"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatBacklinks missing %q in:\n%s", want, out)
		}
	}
	if empty := formatBacklinks(nil, "notes/arch"); !strings.Contains(empty, "No documents link") {
		t.Errorf("empty backlinks = %q, want a 'No documents link' message", empty)
	}
}
```

- [ ] **Step 11: Run format tests to verify they fail (RED)**

Run: `go test ./cmd/flow-mcp/ -run Format`
Expected: FAIL — `undefined: formatDocList` etc. (compile error).

- [ ] **Step 12: Write `cmd/flow-mcp/format.go`**

```go
package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
)

// formatDocLine renders one document's metadata as a single line.
func formatDocLine(d domain.Document) string {
	line := fmt.Sprintf("- [%s] %s · %s · %s", d.ID, d.Title, d.Path, d.Type)
	if len(d.Tags) > 0 {
		line += " · tags: " + strings.Join(d.Tags, ", ")
	}
	return line
}

// formatDocList renders a metadata list with a scope-describing header.
func formatDocList(docs []domain.Document, sc scope) string {
	if len(docs) == 0 {
		return "No documents " + sc.label + "."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d document(s) %s:\n", len(docs), sc.label)
	for _, d := range docs {
		b.WriteString(formatDocLine(d))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// formatSearchHits renders search hits (metadata line + indented snippet).
func formatSearchHits(hits []domain.SearchHit, query string, sc scope) string {
	if len(hits) == 0 {
		return fmt.Sprintf("No matches for %q %s.", query, sc.label)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d match(es) for %q %s:\n", len(hits), query, sc.label)
	for _, h := range hits {
		b.WriteString(formatDocLine(h.Document))
		if s := strings.TrimSpace(h.Snippet); s != "" {
			fmt.Fprintf(&b, "\n    %s", s)
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// formatDoc renders a full document for flow_get_doc.
func formatDoc(d domain.Document, projectName string) string {
	proj := "—"
	if projectName != "" {
		proj = projectName
	}
	tags := "—"
	if len(d.Tags) > 0 {
		tags = strings.Join(d.Tags, ", ")
	}
	meta := fmt.Sprintf("%s · %s · project: %s · tags: %s", d.Path, d.Type, proj, tags)
	if d.Role != nil && *d.Role != "" {
		meta += " · role: " + *d.Role
	}
	return fmt.Sprintf("%s\n%s\nid: %s\n\n%s", d.Title, meta, d.ID, d.Body)
}

// formatTags renders tag counts, highest first.
func formatTags(tags []domain.TagCount, sc scope) string {
	if len(tags) == 0 {
		return "No tags " + sc.label + "."
	}
	sorted := make([]domain.TagCount, len(tags))
	copy(sorted, tags)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Count > sorted[j].Count })
	var b strings.Builder
	fmt.Fprintf(&b, "Tags %s:\n", sc.label)
	for _, t := range sorted {
		fmt.Fprintf(&b, "- %s (%d)\n", t.Tag, t.Count)
	}
	return strings.TrimRight(b.String(), "\n")
}

// formatBacklinks renders inbound wikilink references to a document.
func formatBacklinks(refs []domain.BacklinkRef, label string) string {
	if len(refs) == 0 {
		return "No documents link to " + label + "."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d document(s) link to %s:\n", len(refs), label)
	for _, r := range refs {
		fmt.Fprintf(&b, "- [%s] %s · %s · %s\n", r.ID, r.Title, r.Path, r.Type)
	}
	return strings.TrimRight(b.String(), "\n")
}
```

- [ ] **Step 13: Run the support-layer tests + build + lint (GREEN)**

Run:
```
go test ./internal/domain/ ./cmd/flow-mcp/
go build ./...
golangci-lint run ./cmd/flow-mcp/... ./internal/domain/...
```
Expected: all tests PASS (domain + the new scope/format tests + the unchanged 2a loopback tests still compile and pass — `newServer` still registers exactly the one 2a tool); build OK; golangci-lint 0 issues.

- [ ] **Step 14: Commit**

```bash
git add internal/domain/document.go internal/domain/document_types_test.go cmd/flow-mcp/server.go cmd/flow-mcp/scope.go cmd/flow-mcp/scope_test.go cmd/flow-mcp/format.go cmd/flow-mcp/format_test.go
git commit -m "feat(flow-mcp): read-tool support layer (scope resolver, type filter, formatters)" -m "Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01GTHf7tSzKearihWVzL6i4n"
```

---

### Task 2: The five read tools + loopback integration test

Register `flow_search_docs`, `flow_list_docs`, `flow_get_doc`, `flow_list_tags`, `flow_backlinks` on the server, each a thin handler over apiclient using the Task 1 support layer, and prove the 6-tool surface end-to-end with the in-memory loopback.

**Files:**
- Create: `cmd/flow-mcp/tools_docs.go` (param structs, `resolveDocRef`, `filterDocsByType`/`filterHitsByType`, the 5 handlers)
- Modify: `cmd/flow-mcp/server.go` (register the 5 tools in `newServer`)
- Modify: `cmd/flow-mcp/loopback_test.go` (add a fixture backend + 2b assertions)

**Interfaces:**
- Consumes: everything Task 1 produces; `apiclient.Client.{SearchScoped, ListDocumentsScoped, GetDocument, Tags, Backlinks}`; the 2a `textResult`/`errorResult`/`loginRequired`.
- Produces: the registered tools `flow_search_docs`/`flow_list_docs`/`flow_get_doc`/`flow_list_tags`/`flow_backlinks` (consumed by the 2c/2d slices and the live done-gate); `(*handlers).resolveDocRef(ctx, id, path string, sc scope) (string, error)`.

- [ ] **Step 1: Extend `loopback_test.go` with a fixture backend + the 2b assertions (RED)**

Append to `cmd/flow-mcp/loopback_test.go` (keep the existing 2a tests and helpers — `connect`, `hasTool`, `toolNames`, `text` — they are reused):

```go
// readFixture is the document set the 2b loopback backend serves.
func readFixture() []domain.Document {
	p1, p2 := "p1", "p2"
	return []domain.Document{
		{ID: "d1", OwnerID: "u1", ProjectID: &p1, Type: domain.DocMemory, Path: "notes/arch", Title: "Arch", Body: "the needle lives here", Tags: []string{"go", "design"}},
		{ID: "d2", OwnerID: "u1", ProjectID: &p1, Type: domain.DocFree, Path: "notes/todo", Title: "Todo", Body: "links [[notes/arch]]", Tags: []string{"go"}},
		{ID: "d3", OwnerID: "u1", ProjectID: &p2, Type: domain.DocMemory, Path: "notes/arch", Title: "Beta Arch", Body: "beta body", Tags: []string{"beta"}},
		{ID: "d4", OwnerID: "u1", Type: domain.DocFree, Path: "global-note", Title: "Global", Body: "no project", Tags: nil},
	}
}

// scopedMatch reports whether doc d is in the projectId filter (nil → all,
// "none" → unassigned, else equality) — mirroring the real backend.
func scopedMatch(d domain.Document, projectId string, hasProjectId bool) bool {
	if !hasProjectId {
		return true
	}
	switch projectId {
	case "none":
		return d.ProjectID == nil
	default:
		return d.ProjectID != nil && *d.ProjectID == projectId
	}
}

// fakeReadBackend serves the read endpoints flow-mcp's read tools touch.
func fakeReadBackend(t *testing.T) *httptest.Server {
	t.Helper()
	docs := readFixture()
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/projects", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Project{
			{ID: "p1", Name: "Alpha", Slug: "alpha"},
			{ID: "p2", Name: "Beta", Slug: "beta"},
		})
	})
	mux.HandleFunc("GET /api/v1/documents/tags", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.CollectTags(docs)) // global owner-wide counts
	})
	mux.HandleFunc("GET /api/v1/documents/{id}/backlinks", func(w http.ResponseWriter, r *http.Request) {
		var out []domain.BacklinkRef
		if r.PathValue("id") == "d1" { // d2 links to d1
			out = []domain.BacklinkRef{{ID: "d2", Path: "notes/todo", Title: "Todo", Type: domain.DocFree}}
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("GET /api/v1/documents/{id}", func(w http.ResponseWriter, r *http.Request) {
		for _, d := range docs {
			if d.ID == r.PathValue("id") {
				_ = json.NewEncoder(w).Encode(d)
				return
			}
		}
		http.Error(w, "not found", http.StatusNotFound)
	})
	mux.HandleFunc("GET /api/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		pid, hasPid := q.Get("projectId"), q.Has("projectId")
		query := q.Get("q")
		if query != "" { // search branch
			var hits []domain.SearchHit
			for _, d := range docs {
				if !scopedMatch(d, pid, hasPid) {
					continue
				}
				if strings.Contains(strings.ToLower(d.Title+" "+d.Body), strings.ToLower(query)) {
					hits = append(hits, domain.SearchHit{Document: d, Snippet: d.Body})
				}
			}
			_ = json.NewEncoder(w).Encode(hits)
			return
		}
		var out []domain.Document // list branch
		for _, d := range docs {
			if scopedMatch(d, pid, hasPid) {
				out = append(out, d)
			}
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	return httptest.NewServer(mux)
}

// authedReadServer builds an MCP server authed and scoped to project Alpha (p1).
func authedReadServer(t *testing.T) *mcp.ClientSession {
	t.Helper()
	be := fakeReadBackend(t)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	proj := domain.Project{ID: "p1", Name: "Alpha", Slug: "alpha"}
	return connect(t, newServer(client, true, proj, true))
}

func callText(t *testing.T, sess *mcp.ClientSession, name string, args map[string]any) (*mcp.CallToolResult, string) {
	t.Helper()
	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool %s: %v", name, err)
	}
	return res, text(res)
}

func TestLoopback_ReadTools_Advertised(t *testing.T) {
	sess := authedReadServer(t)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"flow_project_context", "flow_search_docs", "flow_list_docs", "flow_get_doc", "flow_list_tags", "flow_backlinks"} {
		if !hasTool(tools.Tools, name) {
			t.Fatalf("%s not advertised; got %v", name, toolNames(tools.Tools))
		}
	}
}

func TestLoopback_ListDocs_ScopeAndType(t *testing.T) {
	sess := authedReadServer(t)

	// default scope = Alpha (p1) → d1, d2
	_, def := callText(t, sess, "flow_list_docs", map[string]any{})
	if !strings.Contains(def, "d1") || !strings.Contains(def, "d2") || strings.Contains(def, "d3") {
		t.Fatalf("default list = %q, want d1+d2 only", def)
	}
	// type filter memory → d1 only (d2 is free)
	_, mem := callText(t, sess, "flow_list_docs", map[string]any{"type": "memory"})
	if !strings.Contains(mem, "d1") || strings.Contains(mem, "d2") {
		t.Fatalf("type=memory list = %q, want d1 only", mem)
	}
	// explicit project by slug → Beta (p2) → d3
	_, beta := callText(t, sess, "flow_list_docs", map[string]any{"project": "beta"})
	if !strings.Contains(beta, "d3") || strings.Contains(beta, "d1") {
		t.Fatalf("project=beta list = %q, want d3 only", beta)
	}
	// global → all four
	_, all := callText(t, sess, "flow_list_docs", map[string]any{"project": "global"})
	for _, id := range []string{"d1", "d2", "d3", "d4"} {
		if !strings.Contains(all, id) {
			t.Fatalf("global list = %q, missing %s", all, id)
		}
	}
	// none → unassigned → d4
	_, none := callText(t, sess, "flow_list_docs", map[string]any{"project": "none"})
	if !strings.Contains(none, "d4") || strings.Contains(none, "d1") {
		t.Fatalf("project=none list = %q, want d4 only", none)
	}
}

func TestLoopback_ListDocs_UnknownProjectErrors(t *testing.T) {
	sess := authedReadServer(t)
	res, got := callText(t, sess, "flow_list_docs", map[string]any{"project": "bogus"})
	if !res.IsError || !strings.Contains(got, "unknown project") {
		t.Fatalf("unknown project = (IsError=%v, %q), want IsError + 'unknown project' (never a silent empty)", res.IsError, got)
	}
}

func TestLoopback_Search_Scoped(t *testing.T) {
	sess := authedReadServer(t)
	_, hit := callText(t, sess, "flow_search_docs", map[string]any{"query": "needle"})
	if !strings.Contains(hit, "d1") {
		t.Fatalf("search 'needle' (Alpha) = %q, want d1", hit)
	}
	// invalid type → error listing valid types
	res, bad := callText(t, sess, "flow_search_docs", map[string]any{"query": "needle", "type": "bogus"})
	if !res.IsError || !strings.Contains(bad, "memory") {
		t.Fatalf("type=bogus = (IsError=%v, %q), want IsError listing valid types", res.IsError, bad)
	}
}

func TestLoopback_GetDoc_ByIDAndByPath(t *testing.T) {
	sess := authedReadServer(t)
	_, byID := callText(t, sess, "flow_get_doc", map[string]any{"id": "d1"})
	if !strings.Contains(byID, "the needle lives here") || !strings.Contains(byID, "Alpha") {
		t.Fatalf("get_doc id=d1 = %q, want body + project name", byID)
	}
	// by path in the default (Alpha) scope resolves to d1, NOT the same-path d3 in Beta
	_, byPath := callText(t, sess, "flow_get_doc", map[string]any{"path": "notes/arch"})
	if !strings.Contains(byPath, "id: d1") {
		t.Fatalf("get_doc path=notes/arch (Alpha) = %q, want d1 (scope-disambiguated from d3)", byPath)
	}
}

func TestLoopback_ListTags_And_Backlinks(t *testing.T) {
	sess := authedReadServer(t)
	_, tags := callText(t, sess, "flow_list_tags", map[string]any{"project": "global"})
	if !strings.Contains(tags, "go") {
		t.Fatalf("global tags = %q, want 'go'", tags)
	}
	_, bl := callText(t, sess, "flow_backlinks", map[string]any{"id": "d1"})
	if !strings.Contains(bl, "d2") {
		t.Fatalf("backlinks d1 = %q, want d2", bl)
	}
}

func TestLoopback_ReadTools_DegradedRequireLogin(t *testing.T) {
	sess := connect(t, newServer(nil, false, domain.Project{}, false))
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"flow_search_docs", map[string]any{"query": "x"}},
		{"flow_list_docs", map[string]any{}},
		{"flow_get_doc", map[string]any{"id": "d1"}},
		{"flow_list_tags", map[string]any{}},
		{"flow_backlinks", map[string]any{"id": "d1"}},
	} {
		res, got := callText(t, sess, tc.name, tc.args)
		if !res.IsError || !strings.Contains(got, "Login required") {
			t.Fatalf("%s degraded = (IsError=%v, %q), want IsError + 'Login required'", tc.name, res.IsError, got)
		}
	}
}
```

- [ ] **Step 2: Run the loopback to verify it fails (RED)**

Run: `go test ./cmd/flow-mcp/ -run Loopback`
Expected: FAIL — compile error (`newServer` does not register the new tools; the handlers `searchDocs`/`listDocs`/… are undefined).

- [ ] **Step 3: Write `cmd/flow-mcp/tools_docs.go`**

```go
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/domain"
)

const defaultSearchLimit = 20

type searchDocsIn struct {
	Query   string   `json:"query" jsonschema:"the search query (hybrid keyword + semantic)"`
	Project string   `json:"project,omitempty" jsonschema:"project slug, name, or id to scope to; 'global' for all projects, 'none' for unassigned; omit to use the current directory's project"`
	Tags    []string `json:"tags,omitempty" jsonschema:"only documents carrying ALL of these tags"`
	Type    string   `json:"type,omitempty" jsonschema:"only this document type: daily, project, free, agent, memory, instruction, skill, or plan"`
	Limit   int      `json:"limit,omitempty" jsonschema:"maximum number of results (default 20)"`
}

func (h *handlers) searchDocs(ctx context.Context, _ *mcp.CallToolRequest, in searchDocsIn) (*mcp.CallToolResult, any, error) {
	if !h.authed {
		return h.loginRequired(), nil, nil
	}
	if strings.TrimSpace(in.Query) == "" {
		return errorResult("query is required"), nil, nil
	}
	typ, err := checkType(in.Type)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	sc, err := h.resolveScope(ctx, in.Project)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	hits, err := h.client.SearchScoped(ctx, in.Query, sc.projectID, in.Tags...)
	if err != nil {
		return errorResult(fmt.Sprintf("flow server error: %v", err)), nil, nil
	}
	if typ != "" {
		hits = filterHitsByType(hits, typ)
	}
	limit := in.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return textResult(formatSearchHits(hits, in.Query, sc)), nil, nil
}

type listDocsIn struct {
	Project string   `json:"project,omitempty" jsonschema:"project slug, name, or id to scope to; 'global' for all projects, 'none' for unassigned; omit to use the current directory's project"`
	Tags    []string `json:"tags,omitempty" jsonschema:"only documents carrying ALL of these tags"`
	Type    string   `json:"type,omitempty" jsonschema:"only this document type: daily, project, free, agent, memory, instruction, skill, or plan"`
}

func (h *handlers) listDocs(ctx context.Context, _ *mcp.CallToolRequest, in listDocsIn) (*mcp.CallToolResult, any, error) {
	if !h.authed {
		return h.loginRequired(), nil, nil
	}
	typ, err := checkType(in.Type)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	sc, err := h.resolveScope(ctx, in.Project)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	docs, err := h.client.ListDocumentsScoped(ctx, sc.projectID, in.Tags...)
	if err != nil {
		return errorResult(fmt.Sprintf("flow server error: %v", err)), nil, nil
	}
	if typ != "" {
		docs = filterDocsByType(docs, typ)
	}
	return textResult(formatDocList(docs, sc)), nil, nil
}

type getDocIn struct {
	ID   string `json:"id,omitempty" jsonschema:"the document id (pass exactly one of id or path)"`
	Path string `json:"path,omitempty" jsonschema:"the document path within the current project (pass exactly one of id or path)"`
}

func (h *handlers) getDoc(ctx context.Context, _ *mcp.CallToolRequest, in getDocIn) (*mcp.CallToolResult, any, error) {
	if !h.authed {
		return h.loginRequired(), nil, nil
	}
	sc, _ := h.resolveScope(ctx, "") // path lookups use the cwd-resolved default scope
	id, err := h.resolveDocRef(ctx, in.ID, in.Path, sc)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	d, err := h.client.GetDocument(ctx, id)
	if err != nil {
		return errorResult(fmt.Sprintf("flow server error: %v", err)), nil, nil
	}
	return textResult(formatDoc(d, h.projectName(ctx, d.ProjectID))), nil, nil
}

type listTagsIn struct {
	Project string `json:"project,omitempty" jsonschema:"project slug, name, or id to scope to; 'global' for all projects, 'none' for unassigned; omit to use the current directory's project"`
}

func (h *handlers) listTags(ctx context.Context, _ *mcp.CallToolRequest, in listTagsIn) (*mcp.CallToolResult, any, error) {
	if !h.authed {
		return h.loginRequired(), nil, nil
	}
	sc, err := h.resolveScope(ctx, in.Project)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	var tags []domain.TagCount
	if sc.projectID == nil { // global → the efficient owner-wide tag-count endpoint
		tags, err = h.client.Tags(ctx)
	} else { // scoped (a project, or "none") → aggregate over the scoped documents
		var docs []domain.Document
		docs, err = h.client.ListDocumentsScoped(ctx, sc.projectID)
		if err == nil {
			tags = domain.CollectTags(docs)
		}
	}
	if err != nil {
		return errorResult(fmt.Sprintf("flow server error: %v", err)), nil, nil
	}
	return textResult(formatTags(tags, sc)), nil, nil
}

type backlinksIn struct {
	ID   string `json:"id,omitempty" jsonschema:"the document id (pass exactly one of id or path)"`
	Path string `json:"path,omitempty" jsonschema:"the document path within the current project (pass exactly one of id or path)"`
}

func (h *handlers) backlinks(ctx context.Context, _ *mcp.CallToolRequest, in backlinksIn) (*mcp.CallToolResult, any, error) {
	if !h.authed {
		return h.loginRequired(), nil, nil
	}
	sc, _ := h.resolveScope(ctx, "")
	id, err := h.resolveDocRef(ctx, in.ID, in.Path, sc)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	refs, err := h.client.Backlinks(ctx, id)
	if err != nil {
		return errorResult(fmt.Sprintf("flow server error: %v", err)), nil, nil
	}
	ref := strings.TrimSpace(in.ID)
	if ref == "" {
		ref = strings.TrimSpace(in.Path)
	}
	return textResult(formatBacklinks(refs, ref)), nil, nil
}

// resolveDocRef turns a tool's id/path arguments into a document id. Exactly one
// of id or path must be set. A path is looked up within the given scope; a path
// matching zero or multiple documents is an actionable error (never a silent miss).
func (h *handlers) resolveDocRef(ctx context.Context, id, path string, sc scope) (string, error) {
	id, path = strings.TrimSpace(id), strings.TrimSpace(path)
	switch {
	case id != "" && path != "":
		return "", fmt.Errorf("pass either id or path, not both")
	case id != "":
		return id, nil
	case path == "":
		return "", fmt.Errorf("pass either id or path")
	}
	docs, err := h.client.ListDocumentsScoped(ctx, sc.projectID)
	if err != nil {
		return "", fmt.Errorf("flow server error: %v", err)
	}
	var matches []domain.Document
	for _, d := range docs {
		if d.Path == path {
			matches = append(matches, d)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no document at path %q %s", path, sc.label)
	case 1:
		return matches[0].ID, nil
	default:
		return "", fmt.Errorf("path %q matches %d documents %s; use id instead", path, len(matches), sc.label)
	}
}

func filterDocsByType(docs []domain.Document, t domain.DocumentType) []domain.Document {
	out := make([]domain.Document, 0, len(docs))
	for _, d := range docs {
		if d.Type == t {
			out = append(out, d)
		}
	}
	return out
}

func filterHitsByType(hits []domain.SearchHit, t domain.DocumentType) []domain.SearchHit {
	out := make([]domain.SearchHit, 0, len(hits))
	for _, h := range hits {
		if h.Type == t {
			out = append(out, h)
		}
	}
	return out
}
```

- [ ] **Step 4: Register the five tools in `newServer`**

In `cmd/flow-mcp/server.go`, add the five `mcp.AddTool` calls after the existing `flow_project_context` registration, before `return s`:

```go
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_search_docs",
		Description: "Search the Kompendium (hybrid keyword + semantic). Scoped to the current project by default; pass project='global' to search everything. Returns matching documents with snippets.",
	}, h.searchDocs)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_list_docs",
		Description: "List Kompendium documents (metadata only) in the current project by default. Filter by project, tags, or type.",
	}, h.listDocs)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_get_doc",
		Description: "Fetch one document's full content by id, or by path within the current project.",
	}, h.getDoc)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_list_tags",
		Description: "List tag counts for filtering — across the current project by default, or project='global' for all.",
	}, h.listTags)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_backlinks",
		Description: "List documents that link (via wikilinks) to a given document, by id or path. Navigates the memory graph.",
	}, h.backlinks)
```

- [ ] **Step 5: Run the loopback to verify it passes (GREEN)**

Run: `go test ./cmd/flow-mcp/ -run Loopback -v`
Expected: PASS — all 2a tests plus the new 2b tests (`ReadTools_Advertised`, `ListDocs_ScopeAndType`, `ListDocs_UnknownProjectErrors`, `Search_Scoped`, `GetDoc_ByIDAndByPath`, `ListTags_And_Backlinks`, `ReadTools_DegradedRequireLogin`).

- [ ] **Step 6: Build, full gate, commit**

Run:
```
go build ./...
go test ./cmd/flow-mcp/ -v
golangci-lint run ./cmd/flow-mcp/...
```
Expected: build OK; all flow-mcp tests PASS; golangci-lint 0 issues.

Then commit:
```bash
git add cmd/flow-mcp/tools_docs.go cmd/flow-mcp/server.go cmd/flow-mcp/loopback_test.go
git commit -m "feat(flow-mcp): five Kompendium read tools (search/list/get/list_tags/backlinks)" -m "Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01GTHf7tSzKearihWVzL6i4n"
```

---

## Final verification

- [ ] Run the full gate with the podman env (so the unrelated pgstore Docker tests in the suite run, not skip):
  ```
  export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"
  export TESTCONTAINERS_RYUK_DISABLED=true
  make ci
  ```
  Expected: golangci-lint 0, coverage ≥ 80%, build OK.
- [ ] Confirm the 6-tool surface and no behavior regression in the 2a spine: `go test ./cmd/flow-mcp/ -v` green (both 2a context tests and all 2b read-tool tests).
- [ ] Stdout-hygiene re-check (the spine smoke from 2a still holds — no tool added a stdout write):
  ```
  go build -o bin/flow-mcp ./cmd/flow-mcp
  printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}' | ./bin/flow-mcp 2>/dev/null
  ```
  Expected: a single JSON-RPC line on stdout, nothing else. (`bin/flow-mcp` is gitignored — do not commit it.)

## Self-review notes (spec coverage for 2b)

- Spec §7.2 read tools (`flow_search_docs`, `flow_list_docs`, `flow_get_doc`, `flow_list_tags`, `flow_backlinks`) → Task 2. Params match the spec table; `type` filtered MCP-side (Task 2 `filterDocsByType`/`filterHitsByType` + Task 1 `checkType`); `path → id` resolved MCP-side via scoped list (`resolveDocRef`).
- Spec §4.5 auto-scope (default project, `"global"` escape, `"none"`) + the Slice-1 carry-forward "map `"global"` → nil before apiclient" → Task 1 `resolveScope` (the single mapping point; the brainstorm's chosen **full id/slug/name resolution with refresh-on-miss** is implemented and the centerpiece unit test).
- Spec §4.4 degraded mode → every handler's `!h.authed` short-circuit, asserted by `TestLoopback_ReadTools_DegradedRequireLogin`.
- Spec §8 backend slice → already delivered in Slice 1; **2b adds no backend/apiclient change** (only the single-source `domain.DocumentTypes()` helper, which prevents the type-filter set from drifting from `valid()`).
- **DEFERRED (each its own just-in-time plan):** §7.1 **Resources** move to **2c** (where create/delete provide the live add/remove hooks — a resource set without create/delete can't be exercised); 2c also brings the write tools (`flow_create_doc`/`flow_update_doc`/`flow_delete_doc`) + the write guard (§6); **2d** brings `flow_bind_project`/`flow_list_projects` + the runbook (§11) + the live `.mcp.json` registration done-gate (§11). The 2b project-ref cache (`projectList`/`matchProject`/`slugList`) is the foundation 2d's `flow_list_projects`/`flow_bind_project` reuse.
- Result format: concise plain text the model parses; ids are always shown (so the model can chain into get/update by id); empty results are explicit ("No documents …"), and unknown refs / invalid types are `IsError` with actionable text — never silent-empty.
