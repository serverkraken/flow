# flow Kontext B3d — DocType-Redesign + Ist-Migration + Seed-Split Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Finish the flow Kontext-Store by redesigning the document types (`agent`→`spec`/`plan`, `activeContext` as its own type, slim paths), then migrating the real corpus (116 flow memories + global CLAUDE.md) into flow via a reusable, idempotent CLI tool, and splitting CLAUDE.md into a minimal local seed + a flow-global instruction.

**Architecture:** Code-level type changes (Go enum, no DB enum) + two CLI-driven transforms. The deterministic **doctypes** transform is a server-side maintenance endpoint (mirrors the existing `StripFrontmatter` op). The **memory import** reads client-side files, parses/strips frontmatter with `domain.ParseFrontmatterMap`, resolves a manifest scope to a node, and upserts each via a new idempotent `PUT /documents/by-path` endpoint. Both are exposed under `flow context migrate {doctypes|memories}` with `--dry-run`. Hexagonal throughout: cobra → apiclient → REST → usecase → store.

**Tech Stack:** Go, cobra (CLI), net/http (`mux.Handle` method-prefixed routes), `gopkg.in/yaml.v3` (already a dep, via `domain.ParseFrontmatterMap`), goose (no new migration — `documents.type` is `TEXT`, Go-validated), pgstore (Docker tests), `make ci`.

## Global Constraints

- **No DB migration for type values** — `documents.type` is `TEXT` validated by `domain.valid()`; new types are a Go change only.
- **`agent` stays valid-but-deprecated** (B3d-6) — do NOT remove `DocAgent` from `DocumentTypes()`/`valid()`; only mark it deprecated. Full removal is a later cleanup after prod is confirmed 0-`agent`.
- **Body = pure content** (B2) — strip leading YAML frontmatter before persisting; tags are a parameter, never in the body.
- **Path = filename slug, verbatim** — memory docs keep their underscore filenames as `path` (so `[[feedback_no_icons]]` can resolve and re-runs are idempotent). This requires relaxing `SlugOK` to allow `_`.
- **Idempotent + `--dry-run`** on every transform; re-runs must not duplicate or drift.
- **No emoji** in any output ([[feedback_no_icons]]); monospace glyphs only.
- **Small focused files** ([[feedback_no_monoliths]]) — one usecase per file, CLI subcommands in their own file.
- **TDD**: failing test → run-fail → minimal impl → run-pass → commit. Verify `make ci` before claiming done ([[feedback_plan_main_wiring_task]]); check `git rev-parse HEAD` after each subagent ([[feedback_subagent_git_commits_isolated]]).
- **Spec:** `docs/superpowers/specs/2026-06-29-flow-kontext-b3d-doctype-migration-design.md`.

## File Structure

**Create:**
- `internal/domain/doctype_redesign.go` — pure `RedesignedDocType(path)` + `domain.RedesignReport`.
- `internal/usecase/redesign_doctypes.go` — `RedesignDocTypes` usecase (lists `agent` docs, applies transform).
- `internal/usecase/upsert_document_by_path.go` — `UpsertDocumentByPath` usecase (idempotent upsert + links + tags + pin).
- `internal/adapter/httpserver/maintenance_redesign.go` — `handleRedesignDocTypes`.
- `cmd/flow/context_migrate.go` — `flow context migrate {doctypes|memories}` + manifest reader + frontmatter→doc derivation.
- `cmd/flow/context_migrate_test.go` — manifest parse + derivation + CLI fake-server tests.

**Modify:**
- `internal/domain/document.go` — `DocSpec`, `DocActiveContext`, deprecate `DocAgent`, relax `slugRe`.
- `internal/usecase/compose_context.go` — `Compose` handles `DocActiveContext`; `bootstrapTypes` includes it; `SetActiveContext` writes it.
- `internal/adapter/httpserver/server.go` — `RedesignDocTypes` + `UpsertDocumentByPath` fields + 2 routes.
- `internal/adapter/apiclient/context.go` — `RedesignDocTypes` + `UpsertDocumentByPath` client methods.
- `cmd/flow/context.go` — register `contextMigrateCmd()`.
- `cmd/flow-server/main.go` — construct the two new usecases.
- `cmd/flow-mcp/*` — type enumerations in tool descriptions (add `spec`/`activecontext`, mark `agent` deprecated).

---

## Task 1: Domain types + SlugOK relaxation

**Files:**
- Modify: `internal/domain/document.go`
- Test: `internal/domain/documenttype_test.go`, `internal/domain/document_types_test.go`

**Interfaces:**
- Produces: `domain.DocSpec = "spec"`, `domain.DocActiveContext = "activecontext"` (both agent-owned); `SlugOK` accepts `_`.

- [ ] **Step 1: Write failing tests**

Append to `internal/domain/documenttype_test.go`:

```go
func TestDocumentType_SpecAndActiveContextValid(t *testing.T) {
	for _, ty := range []domain.DocumentType{domain.DocSpec, domain.DocActiveContext} {
		d := domain.Document{Type: ty, Path: "x"}
		if err := d.Validate(); err != nil {
			t.Errorf("type %q should be valid: %v", ty, err)
		}
		if ty.HumanOwned() {
			t.Errorf("type %q must be agent-owned, not human-owned", ty)
		}
	}
}

func TestDocumentType_AgentStillValid(t *testing.T) {
	// B3d-6: agent is deprecated but must remain valid so unconverted rows load.
	if err := (domain.Document{Type: domain.DocAgent, Path: "x"}).Validate(); err != nil {
		t.Errorf("DocAgent must stay valid during B3d: %v", err)
	}
}

func TestSlugOK_AllowsUnderscores(t *testing.T) {
	for _, s := range []string{"feedback_no_icons", "project_flow_rebuild_m1a", "active-context", "2026-06-23-flow-webui-overhaul-design"} {
		if !domain.SlugOK(s) {
			t.Errorf("SlugOK(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"Bad Upper", "trailing_", "/lead", "a//b"} {
		if domain.SlugOK(s) {
			t.Errorf("SlugOK(%q) = true, want false", s)
		}
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/domain/ -run 'SpecAndActiveContext|AgentStillValid|SlugOK_AllowsUnderscores' -v`
Expected: FAIL (undefined `DocSpec`/`DocActiveContext`; `SlugOK` rejects underscores).

- [ ] **Step 3: Implement**

In `internal/domain/document.go`, extend the const block and the canonical list:

```go
const (
	DocDaily       DocumentType = "daily"
	DocProject     DocumentType = "project"
	DocFree        DocumentType = "free"
	DocAgent       DocumentType = "agent" // DEPRECATED (B3d): split into DocSpec/DocPlan; kept valid until prod 0-agent
	DocMemory      DocumentType = "memory"        // agent-owned
	DocInstruction DocumentType = "instruction"   // agent-owned (CLAUDE.md)
	DocSkill       DocumentType = "skill"         // agent-owned
	DocPlan        DocumentType = "plan"          // agent-owned
	DocSpec        DocumentType = "spec"          // agent-owned (B3d: was agent)
	DocActiveContext DocumentType = "activecontext" // agent-owned (B3d: per-repo active context)
)
```

```go
func DocumentTypes() []DocumentType {
	return []DocumentType{
		DocDaily, DocProject, DocFree, DocAgent,
		DocMemory, DocInstruction, DocSkill, DocPlan,
		DocSpec, DocActiveContext,
	}
}
```

Relax the slug regex to allow `_` as an in-segment separator (alongside `-`):

```go
var slugRe = regexp.MustCompile(`^[a-z0-9]+(?:[-_][a-z0-9]+)*(?:/[a-z0-9]+(?:[-_][a-z0-9]+)*)*$`)
```

`HumanOwned()` is unchanged (positive set of daily/project/free → spec/activecontext are agent-owned automatically).

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/domain/ -v`
Expected: PASS (including the existing `TestDocumentTypesAllValid`, which counts via `DocumentTypes()` — update its expected length if it hard-codes a number).

Note: `internal/domain/document_types_test.go` asserts `len(ts)` — change the expected count from `8` to `10` and the message accordingly.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/document.go internal/domain/documenttype_test.go internal/domain/document_types_test.go
git commit -m "feat(domain): add spec+activecontext types, deprecate agent, allow _ in slugs (B3d-1)"
```

---

## Task 2: activeContext as its own type (Compose + SetActiveContext)

**Files:**
- Modify: `internal/usecase/compose_context.go`
- Test: `internal/usecase/compose_context_test.go` (existing Compose table test), `internal/usecase/set_active_context_test.go` (if present; else add to the compose test file)

**Interfaces:**
- Consumes: `domain.DocActiveContext` (Task 1).
- Produces: `Compose` routes `DocActiveContext`@leaf into `ComposedContext.ActiveContext`; `bootstrapTypes` fetches it; `SetActiveContext` writes `DocActiveContext`.

- [ ] **Step 1: Write failing test**

Add to the Compose table test (`internal/usecase/compose_context_test.go`) a case proving activeContext is detected by **type**, not by `memory@active-context`:

```go
func TestCompose_ActiveContextByType(t *testing.T) {
	leaf := domain.Node{ID: "n1", Kind: domain.KindRepo, Name: "flow"}
	chain := []domain.Node{leaf}
	docs := []domain.Document{
		{ID: "ac", NodeID: &leaf.ID, Type: domain.DocActiveContext, Path: "active-context", Body: "where I was"},
		{ID: "m1", NodeID: &leaf.ID, Type: domain.DocMemory, Path: "some-note", Body: "a memory"},
	}
	out := usecase.Compose(chain, docs, map[string]bool{}, 6000)
	if out.ActiveContext == nil || out.ActiveContext.ID != "ac" {
		t.Fatalf("activeContext not picked up by type: %+v", out.ActiveContext)
	}
	if len(out.Memories["leaf"]) != 1 || out.Memories["leaf"][0].ID != "m1" {
		t.Fatalf("leaf memory misrouted: %+v", out.Memories["leaf"])
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/usecase/ -run TestCompose_ActiveContextByType -v`
Expected: FAIL (`activeContext nil` — current code only matches `DocMemory && path==active-context`).

- [ ] **Step 3: Implement**

In `internal/usecase/compose_context.go`:

(a) add the type to the gather set:

```go
var bootstrapTypes = []domain.DocumentType{domain.DocInstruction, domain.DocMemory, domain.DocActiveContext}
```

(b) in `Compose`, add a dedicated case and remove the `d.Path == ActiveContextPath` special-case from the `DocMemory` branch:

```go
	for _, d := range docs {
		switch d.Type {
		case domain.DocInstruction:
			lbl := "global"
			if d.NodeID != nil {
				lbl = label[*d.NodeID]
			}
			out.Instructions = append(out.Instructions, itemOf(d, lbl))
		case domain.DocActiveContext:
			if d.NodeID != nil && tier[*d.NodeID] == "leaf" {
				it := itemOf(d, label[*d.NodeID])
				out.ActiveContext = &it
			}
		case domain.DocMemory:
			if d.NodeID == nil {
				if globalAllowed[d.ID] {
					it := itemOf(d, "global")
					relevance = append(relevance, ranked{it, "global", d.Pinned, it.UpdatedAt})
				}
				continue
			}
			nid := *d.NodeID
			switch tier[nid] {
			case "leaf":
				out.Memories["leaf"] = append(out.Memories["leaf"], itemOf(d, label[nid]))
			case "vorhaben":
				out.Memories["vorhaben"] = append(out.Memories["vorhaben"], itemOf(d, label[nid]))
			case "engagement":
				it := itemOf(d, label[nid])
				relevance = append(relevance, ranked{it, "engagement", d.Pinned, it.UpdatedAt})
			}
		}
	}
```

(c) in `SetActiveContext.Execute`, write the new type (the only line that changes):

```go
	id, updated, err := uc.Docs.UpsertByPath(ctx, ownerID, &leaf.ID, domain.DocActiveContext, ActiveContextPath, title, body, false)
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/usecase/ -run 'TestCompose|SetActiveContext' -v`
Expected: PASS. Fix any existing Compose test that seeded activeContext as `DocMemory@active-context` — re-seed it as `DocActiveContext`.

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/compose_context.go internal/usecase/compose_context_test.go
git commit -m "feat(usecase): activeContext is its own type in Compose + SetActiveContext (B3d-1)"
```

---

## Task 3: flow-mcp type enumerations

**Files:**
- Modify: the flow-mcp tool description(s) that enumerate document types.
- Test: any existing flow-mcp type-validation test (run the package).

- [ ] **Step 1: Locate the enumerations**

Run: `rg -n "daily, project, free, agent|daily.*instruction.*skill" cmd/flow-mcp`
These are the `flow_create_doc` / `flow_list_docs` tool `type` descriptions.

- [ ] **Step 2: Update the strings**

Change each enumeration to include the new types and mark `agent` deprecated, e.g.:

```
Type must be one of: daily, project, free, memory, instruction, skill, plan, spec, activecontext (agent: deprecated).
```

- [ ] **Step 3: Build + test the package**

Run: `go build ./cmd/flow-mcp/... && go test ./cmd/flow-mcp/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/flow-mcp
git commit -m "docs(mcp): list spec+activecontext types, mark agent deprecated (B3d-1)"
```

---

## Task 4: RedesignDocTypes — pure transform + usecase

**Files:**
- Create: `internal/domain/doctype_redesign.go`
- Create: `internal/usecase/redesign_doctypes.go`
- Test: `internal/domain/doctype_redesign_test.go`, `internal/usecase/redesign_doctypes_test.go`

**Interfaces:**
- Produces: `domain.RedesignedDocType(path) (DocumentType, string)`; `domain.RedesignReport{Scanned, Converted int}`; `usecase.RedesignDocTypes{Docs ports.DocumentStore; Clock ports.Clock}` with `Execute(ctx, ownerID string, dryRun bool) (domain.RedesignReport, error)`.

- [ ] **Step 1: Write failing domain test**

`internal/domain/doctype_redesign_test.go`:

```go
package domain_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestRedesignedDocType(t *testing.T) {
	cases := []struct {
		in       string
		wantType domain.DocumentType
		wantPath string
	}{
		{"plans/2026-06-25-foo", domain.DocPlan, "2026-06-25-foo"},
		{"specs/2026-06-25-foo-design", domain.DocSpec, "2026-06-25-foo-design"},
		{"loose-doc", domain.DocSpec, "loose-doc"}, // no prefix → spec, path unchanged
	}
	for _, c := range cases {
		gotT, gotP := domain.RedesignedDocType(c.in)
		if gotT != c.wantType || gotP != c.wantPath {
			t.Errorf("RedesignedDocType(%q) = (%q,%q), want (%q,%q)", c.in, gotT, gotP, c.wantType, c.wantPath)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/domain/ -run TestRedesignedDocType -v`
Expected: FAIL (undefined).

- [ ] **Step 3: Implement the pure transform**

`internal/domain/doctype_redesign.go`:

```go
package domain

import "strings"

// RedesignReport summarises a RedesignDocTypes maintenance run.
type RedesignReport struct {
	Scanned   int `json:"scanned"`   // legacy agent docs seen
	Converted int `json:"converted"` // docs rewritten (== Scanned outside dry-run)
}

// RedesignedDocType maps a legacy `agent` document's path to its new (type, path):
// a `plans/` prefix → DocPlan, anything else → DocSpec; the leading
// `specs/`|`plans/` segment is stripped so path becomes the slim node-local slug.
func RedesignedDocType(path string) (DocumentType, string) {
	switch {
	case strings.HasPrefix(path, "plans/"):
		return DocPlan, strings.TrimPrefix(path, "plans/")
	case strings.HasPrefix(path, "specs/"):
		return DocSpec, strings.TrimPrefix(path, "specs/")
	default:
		return DocSpec, path
	}
}
```

- [ ] **Step 4: Write failing usecase test**

`internal/usecase/redesign_doctypes_test.go` (use the existing fake `DocumentStore` test helper in the `usecase` test package; mirror how `strip_frontmatter_test.go` constructs one):

```go
func TestRedesignDocTypes_ConvertsAgentDocs(t *testing.T) {
	store := newFakeDocStore(
		domain.Document{ID: "1", Type: domain.DocAgent, Path: "plans/2026-x"},
		domain.Document{ID: "2", Type: domain.DocAgent, Path: "specs/2026-y-design"},
		domain.Document{ID: "3", Type: domain.DocMemory, Path: "untouched"},
	)
	uc := usecase.RedesignDocTypes{Docs: store, Clock: fixedClock{}}

	// dry-run mutates nothing
	rep, err := uc.Execute(context.Background(), "owner", true)
	if err != nil || rep.Scanned != 2 || rep.Converted != 2 {
		t.Fatalf("dry-run rep=%+v err=%v", rep, err)
	}
	if store.byID["1"].Type != domain.DocAgent {
		t.Fatalf("dry-run must not mutate")
	}

	// real run
	if _, err := uc.Execute(context.Background(), "owner", false); err != nil {
		t.Fatal(err)
	}
	if d := store.byID["1"]; d.Type != domain.DocPlan || d.Path != "2026-x" {
		t.Errorf("doc 1 = %+v, want plan/2026-x", d)
	}
	if d := store.byID["2"]; d.Type != domain.DocSpec || d.Path != "2026-y-design" {
		t.Errorf("doc 2 = %+v, want spec/2026-y-design", d)
	}
	if store.byID["3"].Type != domain.DocMemory {
		t.Errorf("non-agent doc must be untouched")
	}

	// idempotent: second run finds nothing
	rep2, _ := uc.Execute(context.Background(), "owner", false)
	if rep2.Scanned != 0 {
		t.Errorf("second run should find 0 agent docs, got %d", rep2.Scanned)
	}
}
```

> Note: if no shared fake `DocumentStore` exists in the `usecase` test package, add a minimal one (List returns all; Update replaces `byID`). Reuse it in Task 6.

- [ ] **Step 5: Run to verify it fails**

Run: `go test ./internal/usecase/ -run TestRedesignDocTypes -v`
Expected: FAIL (undefined `RedesignDocTypes`).

- [ ] **Step 6: Implement the usecase**

`internal/usecase/redesign_doctypes.go`:

```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// RedesignDocTypes is an idempotent maintenance op that rewrites every legacy
// `agent` document to its new type (spec|plan) and slim path (see
// domain.RedesignedDocType). Run with dryRun=true to audit without mutating.
type RedesignDocTypes struct {
	Docs  ports.DocumentStore
	Clock ports.Clock
}

func (uc RedesignDocTypes) Execute(ctx context.Context, ownerID string, dryRun bool) (domain.RedesignReport, error) {
	docs, err := uc.Docs.List(ctx, ownerID, nil)
	if err != nil {
		return domain.RedesignReport{}, err
	}
	var rep domain.RedesignReport
	for _, d := range docs {
		if d.Type != domain.DocAgent {
			continue
		}
		rep.Scanned++
		if dryRun {
			continue
		}
		d.Type, d.Path = domain.RedesignedDocType(d.Path)
		d.UpdatedAt = uc.Clock.Now()
		if _, err := uc.Docs.Update(ctx, d); err != nil {
			return rep, err
		}
		rep.Converted++
	}
	if dryRun {
		rep.Converted = rep.Scanned
	}
	return rep, nil
}
```

- [ ] **Step 7: Run to verify pass**

Run: `go test ./internal/domain/ ./internal/usecase/ -run 'Redesign' -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/domain/doctype_redesign.go internal/domain/doctype_redesign_test.go internal/usecase/redesign_doctypes.go internal/usecase/redesign_doctypes_test.go
git commit -m "feat(usecase): RedesignDocTypes maintenance op (agent→spec/plan + path-strip) (B3d-2)"
```

---

## Task 5: doctypes endpoint + apiclient + CLI

**Files:**
- Create: `internal/adapter/httpserver/maintenance_redesign.go`
- Modify: `internal/adapter/httpserver/server.go` (field + route)
- Modify: `internal/adapter/apiclient/context.go`
- Create: `cmd/flow/context_migrate.go` (the `migrate` parent + `doctypes` subcommand)
- Modify: `cmd/flow/context.go` (register `contextMigrateCmd()`)
- Test: `internal/adapter/httpserver/maintenance_redesign_test.go`

**Interfaces:**
- Consumes: `usecase.RedesignDocTypes` (Task 4).
- Produces: `POST /api/v1/maintenance/redesign-doctypes?dry_run=`; `apiclient.(*Client).RedesignDocTypes(ctx, dryRun) (domain.RedesignReport, error)`; `flow context migrate doctypes [--dry-run]`.

- [ ] **Step 1: Add the server field + handler + route**

In `internal/adapter/httpserver/server.go`, next to `StripFrontmatter usecase.StripFrontmatter` (~line 90):

```go
	StripFrontmatter usecase.StripFrontmatter
	RedesignDocTypes usecase.RedesignDocTypes
```

Add the route next to the strip-frontmatter route (~line 219):

```go
	mux.Handle("POST /api/v1/maintenance/strip-frontmatter", s.authAny(http.HandlerFunc(s.handleStripFrontmatter)))
	mux.Handle("POST /api/v1/maintenance/redesign-doctypes", s.authAny(http.HandlerFunc(s.handleRedesignDocTypes)))
```

`internal/adapter/httpserver/maintenance_redesign.go`:

```go
package httpserver

import "net/http"

func (s *Server) handleRedesignDocTypes(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	dryRun := r.URL.Query().Get("dry_run") == "true"
	rep, err := s.RedesignDocTypes.Execute(r.Context(), u.ID, dryRun)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
```

- [ ] **Step 2: Write failing httptest**

`internal/adapter/httpserver/maintenance_redesign_test.go` (mirror `documents_test.go` test server setup; seed two agent docs via the in-memory store):

```go
func TestHandleRedesignDocTypes(t *testing.T) {
	ts, _ := newTestServerWithDocs(t,
		domain.Document{ID: "1", Type: domain.DocAgent, Path: "plans/p"},
		domain.Document{ID: "2", Type: domain.DocAgent, Path: "specs/s-design"},
	)
	defer ts.Close()

	res := doDoc(t, ts, "POST", "/api/v1/maintenance/redesign-doctypes?dry_run=true", "")
	if res.Code != 200 {
		t.Fatalf("status %d", res.Code)
	}
	var rep domain.RedesignReport
	_ = json.Unmarshal(res.Body.Bytes(), &rep)
	if rep.Scanned != 2 {
		t.Fatalf("scanned=%d want 2", rep.Scanned)
	}
}
```

> Use whatever test-server constructor the existing `documents_test.go` uses; if it doesn't expose a doc-seeding helper, add `RedesignDocTypes` to the `Server{}` it already builds and seed via the same fake store.

- [ ] **Step 3: Run to verify it fails, then passes after wiring the test server**

Run: `go test ./internal/adapter/httpserver/ -run TestHandleRedesignDocTypes -v`
Expected: first FAIL (route/handler), then PASS once the handler + test-server field are in.

- [ ] **Step 4: apiclient method**

Append to `internal/adapter/apiclient/context.go`:

```go
// RedesignDocTypes triggers the server-side maintenance op that rewrites legacy
// `agent` docs to spec/plan with slim paths. dryRun audits without mutating.
func (c *Client) RedesignDocTypes(ctx context.Context, dryRun bool) (domain.RedesignReport, error) {
	path := "/api/v1/maintenance/redesign-doctypes"
	if dryRun {
		path += "?dry_run=true"
	}
	var out domain.RedesignReport
	err := c.do(ctx, http.MethodPost, path, nil, &out)
	return out, err
}
```

(Ensure `domain` and `net/http` are imported in `context.go`.)

- [ ] **Step 5: CLI — `flow context migrate doctypes`**

`cmd/flow/context_migrate.go`:

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func contextMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate the document corpus into the B3d type system",
	}
	cmd.AddCommand(migrateDocTypesCmd(), migrateMemoriesCmd())
	return cmd
}

func migrateDocTypesCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "doctypes",
		Short: "Rewrite legacy `agent` docs to spec/plan with slim paths",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			rep, err := c.RedesignDocTypes(cmd.Context(), dryRun)
			if err != nil {
				return err
			}
			mode := ""
			if dryRun {
				mode = " (dry-run)"
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "scanned %d agent docs, converted %d%s\n",
				rep.Scanned, rep.Converted, mode)
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report without mutating")
	return cmd
}
```

In `cmd/flow/context.go`, register it:

```go
	cmd.AddCommand(installHooksCmd(), flushCheckCmd(), contextMigrateCmd())
```

> `migrateMemoriesCmd()` is defined in Task 8; this file compiles only after Task 8. To keep tasks independently buildable, add a temporary stub `func migrateMemoriesCmd() *cobra.Command { return &cobra.Command{Use: "memories", Hidden: true} }` here and replace its body in Task 8 — OR implement Tasks 5 and 8 in one branch. (Recommended: implement Task 8's file in the same task-branch; the stub keeps `go build` green between commits.)

- [ ] **Step 6: Build + test**

Run: `go build ./... && go test ./internal/adapter/httpserver/ ./internal/adapter/apiclient/ -run 'Redesign' -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/adapter/httpserver/maintenance_redesign.go internal/adapter/httpserver/server.go internal/adapter/httpserver/maintenance_redesign_test.go internal/adapter/apiclient/context.go cmd/flow/context_migrate.go cmd/flow/context.go
git commit -m "feat: redesign-doctypes endpoint + apiclient + flow context migrate doctypes (B3d-2)"
```

---

## Task 6: UpsertDocumentByPath usecase

**Files:**
- Create: `internal/usecase/upsert_document_by_path.go`
- Test: `internal/usecase/upsert_document_by_path_test.go`

**Interfaces:**
- Consumes: `ports.DocumentStore.{UpsertByPath, ReplaceLinks, SetPinned}`, `ports.TagStore.SetTags`, `ports.DocChangeNotifier`.
- Produces: `usecase.UpsertDocumentByPath` with `Execute(ctx, ownerID string, in UpsertByPathInput) (id string, updatedAt time.Time, err error)` and `UpsertByPathInput{Type domain.DocumentType; NodeID *string; Path, Title, Body string; Tags []string; Pinned bool}`.

- [ ] **Step 1: Write failing test**

`internal/usecase/upsert_document_by_path_test.go`:

```go
func TestUpsertDocumentByPath_Idempotent(t *testing.T) {
	store := newFakeDocStore()
	tags := newFakeTagStore()
	uc := usecase.UpsertDocumentByPath{Docs: store, Tags: tags}

	in := usecase.UpsertByPathInput{
		Type: domain.DocMemory, NodeID: nil, Path: "feedback_no_icons",
		Title: "No emoji", Body: "avoid colored emoji [[feedback_no_monoliths]]",
		Tags: []string{"feedback"}, Pinned: true,
	}
	id1, _, err := uc.Execute(context.Background(), "owner", in)
	if err != nil {
		t.Fatal(err)
	}
	// re-run with same path → same id (upsert, not duplicate)
	id2, _, err := uc.Execute(context.Background(), "owner", in)
	if err != nil || id1 != id2 {
		t.Fatalf("not idempotent: id1=%s id2=%s err=%v", id1, id2, err)
	}
	if !store.byID[id1].Pinned {
		t.Errorf("pinned not applied")
	}
	if got := tags.tagsFor(id1); len(got) != 1 || got[0] != "feedback" {
		t.Errorf("tags = %v, want [feedback]", got)
	}
	if got := store.linksFor(id1); len(got) != 1 || got[0] != "feedback_no_monoliths" {
		t.Errorf("wikilinks = %v, want [feedback_no_monoliths]", got)
	}
}

func TestUpsertDocumentByPath_RejectsBadType(t *testing.T) {
	uc := usecase.UpsertDocumentByPath{Docs: newFakeDocStore(), Tags: newFakeTagStore()}
	_, _, err := uc.Execute(context.Background(), "owner",
		usecase.UpsertByPathInput{Type: domain.DocumentType("bogus"), Path: "x", Body: "y"})
	if !errors.Is(err, domain.ErrInvalidDocument) {
		t.Fatalf("err = %v, want ErrInvalidDocument", err)
	}
}
```

> The fake `DocumentStore` must implement `UpsertByPath` (insert-or-update keyed by `(nodeID,path)` returning a stable id), `ReplaceLinks` (record targets), `SetPinned`. The fake `TagStore` records `SetTags`. Extend the fakes from Task 4 as needed.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/usecase/ -run TestUpsertDocumentByPath -v`
Expected: FAIL (undefined).

- [ ] **Step 3: Implement**

`internal/usecase/upsert_document_by_path.go`:

```go
package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// UpsertByPathInput is the caller-supplied shape for an idempotent upsert keyed
// by (owner, node, path). Used by the memory-migration importer.
type UpsertByPathInput struct {
	Type   domain.DocumentType
	NodeID *string
	Path   string
	Title  string
	Body   string
	Tags   []string // explicit tag set; nil → leave tags untouched
	Pinned bool
}

// UpsertDocumentByPath inserts or updates a document at (owner, node, path),
// re-extracts wikilinks, applies the tag set, and enforces the pinned flag on
// every run (so re-imports stay in sync). It is the general idempotent write
// behind `flow context migrate memories`.
type UpsertDocumentByPath struct {
	Docs     ports.DocumentStore
	Tags     ports.TagStore
	Notifier ports.DocChangeNotifier // optional; nil → no notification
}

func (uc UpsertDocumentByPath) Execute(ctx context.Context, ownerID string, in UpsertByPathInput) (string, time.Time, error) {
	// Validate via a domain.Document (type set, slug form, project rule).
	if err := (domain.Document{Type: in.Type, NodeID: in.NodeID, Path: in.Path, Title: in.Title, Body: in.Body}).Validate(); err != nil {
		return "", time.Time{}, err
	}
	id, updated, err := uc.Docs.UpsertByPath(ctx, ownerID, in.NodeID, in.Type, in.Path, in.Title, in.Body, in.Pinned)
	if err != nil {
		return "", time.Time{}, err
	}
	if err := uc.Docs.ReplaceLinks(ctx, id, ownerID, domain.WikilinkTargets(in.Body)); err != nil {
		return "", time.Time{}, err
	}
	if uc.Tags != nil && in.Tags != nil {
		if _, err := uc.Tags.SetTags(ctx, ownerID, domain.TaggableDocument, id, in.Tags); err != nil {
			return id, updated, err
		}
	}
	// UpsertByPath's ON CONFLICT path does not touch pinned; enforce it explicitly.
	if err := uc.Docs.SetPinned(ctx, ownerID, id, in.Pinned); err != nil {
		return id, updated, err
	}
	if uc.Notifier != nil {
		uc.Notifier.DocumentChanged()
	}
	return id, updated, nil
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/usecase/ -run TestUpsertDocumentByPath -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/upsert_document_by_path.go internal/usecase/upsert_document_by_path_test.go
git commit -m "feat(usecase): UpsertDocumentByPath (idempotent upsert+links+tags+pin) (B3d-2)"
```

---

## Task 7: by-path endpoint + apiclient

**Files:**
- Modify: `internal/adapter/httpserver/documents.go` (handler) + `server.go` (field + route)
- Modify: `internal/adapter/apiclient/context.go`
- Test: `internal/adapter/httpserver/documents_test.go`

**Interfaces:**
- Consumes: `usecase.UpsertDocumentByPath` (Task 6).
- Produces: `PUT /api/v1/documents/by-path`; `apiclient.(*Client).UpsertDocumentByPath(ctx, in apiclient.UpsertByPathInput) (apiclient.UpsertByPathResult, error)`.

- [ ] **Step 1: Server field + route**

In `server.go` (near the other document usecases):

```go
	UpsertDocumentByPath usecase.UpsertDocumentByPath
```

Route (near `POST /api/v1/documents/import`):

```go
	mux.Handle("PUT /api/v1/documents/by-path", s.authAny(http.HandlerFunc(s.handleUpsertByPath)))
```

- [ ] **Step 2: Handler**

Append to `internal/adapter/httpserver/documents.go`:

```go
type upsertByPathReq struct {
	Type   string   `json:"type"`
	NodeID *string  `json:"projectId,omitempty"`
	Path   string   `json:"path"`
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Tags   []string `json:"tags,omitempty"`
	Pinned bool     `json:"pinned"`
}

func (s *Server) handleUpsertByPath(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req upsertByPathReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	id, updated, err := s.UpsertDocumentByPath.Execute(r.Context(), u.ID, usecase.UpsertByPathInput{
		Type: domain.DocumentType(req.Type), NodeID: req.NodeID, Path: req.Path,
		Title: req.Title, Body: req.Body, Tags: req.Tags, Pinned: req.Pinned,
	})
	if err != nil {
		if errors.Is(err, domain.ErrInvalidDocument) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "updatedAt": updated})
}
```

(Confirm `errors`, `domain`, `usecase`, `json` are imported in `documents.go`.)

- [ ] **Step 3: Write failing httptest**

In `documents_test.go`:

```go
func TestUpsertByPath_InsertThenUpdate(t *testing.T) {
	ts := newTestServer(t) // the existing constructor that builds Server{} with fakes
	defer ts.Close()

	body := `{"type":"memory","path":"feedback_no_icons","title":"No emoji","body":"x","tags":["feedback"],"pinned":true}`
	res := doDoc(t, ts, "PUT", "/api/v1/documents/by-path", body)
	if res.Code != 200 {
		t.Fatalf("insert status %d: %s", res.Code, res.Body)
	}
	res2 := doDoc(t, ts, "PUT", "/api/v1/documents/by-path", body)
	if res2.Code != 200 {
		t.Fatalf("update status %d", res2.Code)
	}

	bad := `{"type":"bogus","path":"x","body":"y"}`
	if r := doDoc(t, ts, "PUT", "/api/v1/documents/by-path", bad); r.Code != 400 {
		t.Fatalf("bad type status %d want 400", r.Code)
	}
}
```

> Add `UpsertDocumentByPath: usecase.UpsertDocumentByPath{Docs: ..., Tags: ...}` to the test-server's `Server{}` construction so the route is live.

- [ ] **Step 4: Run to verify it fails then passes**

Run: `go test ./internal/adapter/httpserver/ -run TestUpsertByPath -v`
Expected: PASS after handler + route + test-server wiring.

- [ ] **Step 5: apiclient method**

Append to `internal/adapter/apiclient/context.go`:

```go
// UpsertByPathInput mirrors the by-path upsert payload.
type UpsertByPathInput struct {
	Type   string   `json:"type"`
	NodeID *string  `json:"projectId,omitempty"`
	Path   string   `json:"path"`
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Tags   []string `json:"tags,omitempty"`
	Pinned bool     `json:"pinned"`
}

type UpsertByPathResult struct {
	ID        string    `json:"id"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// UpsertDocumentByPath inserts or updates a document at (node, path) idempotently.
func (c *Client) UpsertDocumentByPath(ctx context.Context, in UpsertByPathInput) (UpsertByPathResult, error) {
	var out UpsertByPathResult
	err := c.do(ctx, http.MethodPut, "/api/v1/documents/by-path", in, &out)
	return out, err
}
```

(Ensure `time` is imported in `context.go`.)

- [ ] **Step 6: Build + test**

Run: `go build ./... && go test ./internal/adapter/httpserver/ ./internal/adapter/apiclient/ -run 'UpsertByPath' -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/adapter/httpserver/documents.go internal/adapter/httpserver/server.go internal/adapter/httpserver/documents_test.go internal/adapter/apiclient/context.go
git commit -m "feat: PUT /documents/by-path idempotent upsert endpoint + apiclient (B3d-2)"
```

---

## Task 8: memory-import CLI

**Files:**
- Modify/replace: `cmd/flow/context_migrate.go` (replace the `migrateMemoriesCmd` stub with the real implementation + helpers)
- Test: `cmd/flow/context_migrate_test.go`

**Interfaces:**
- Consumes: `apiclient.(*Client).ListNodes`, `apiclient.(*Client).UpsertDocumentByPath` (Task 7), `domain.ParseFrontmatterMap`.
- Produces: `flow context migrate memories --dir <dir> --manifest <m.tsv> [--dry-run]`.

**Manifest format** (TSV, `#` comments + a header line ignored): `file⇥scope⇥tags⇥pin⇥keep`
- `scope`: a node slug (e.g. `github-com-serverkraken-flow`, `privat`) or `global`.
- `tags`: comma-separated extra tags (the `metadata.type` frontmatter value is auto-added as a tag).
- `pin`: `y` → pinned; anything else → not.
- `keep`: `skip` → not imported; anything else → imported.

- [ ] **Step 1: Write failing tests for the pure helpers**

`cmd/flow/context_migrate_test.go`:

```go
func TestParseManifest(t *testing.T) {
	in := strings.NewReader("# comment\nfile\tscope\ttags\tpin\tkeep\n" +
		"feedback_no_icons.md\tglobal\tux,style\ty\ty\n" +
		"project_x.md\tgithub-com-serverkraken-flow\t\t\ty\n" +
		"dead.md\tglobal\t\t\tskip\n")
	rows, err := parseManifest(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows=%d want 3", len(rows))
	}
	if rows[0].Scope != "global" || rows[0].Pin != true || !rows[0].Keep {
		t.Errorf("row0 = %+v", rows[0])
	}
	if rows[1].Tags != nil && len(rows[1].Tags) != 0 {
		t.Errorf("row1 tags = %v want empty", rows[1].Tags)
	}
	if rows[2].Keep {
		t.Errorf("dead.md should be skip")
	}
}

func TestDeriveMemoryDoc(t *testing.T) {
	body := "---\nname: feedback_no_icons\ndescription: avoid colored emoji\nmetadata:\n  type: feedback\n---\nUse monospace glyphs. See [[feedback_no_monoliths]].\n"
	row := manifestRow{File: "feedback_no_icons.md", Scope: "global", Tags: []string{"ux"}, Pin: true, Keep: true}
	doc := deriveMemoryDoc(body, row)
	if doc.Path != "feedback_no_icons" {
		t.Errorf("path = %q", doc.Path)
	}
	if doc.Title != "avoid colored emoji" {
		t.Errorf("title = %q", doc.Title)
	}
	if !contains(doc.Tags, "feedback") || !contains(doc.Tags, "ux") {
		t.Errorf("tags = %v want feedback+ux", doc.Tags)
	}
	if strings.Contains(doc.Body, "---") || !strings.HasPrefix(doc.Body, "Use monospace") {
		t.Errorf("frontmatter not stripped: %q", doc.Body)
	}
	if !doc.Pinned {
		t.Errorf("pinned should be true")
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./cmd/flow/ -run 'ParseManifest|DeriveMemoryDoc' -v`
Expected: FAIL (undefined).

- [ ] **Step 3: Implement the helpers + command**

Replace the stub in `cmd/flow/context_migrate.go`:

```go
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/spf13/cobra"
)

type manifestRow struct {
	File  string
	Scope string // node slug or "global"
	Tags  []string
	Pin   bool
	Keep  bool
}

type memoryDoc struct {
	Path   string
	Title  string
	Body   string
	Tags   []string
	Pinned bool
}

// parseManifest reads a TSV manifest: file<TAB>scope<TAB>tags<TAB>pin<TAB>keep.
// Blank lines, `#` comments, and a leading `file<TAB>...` header are ignored.
func parseManifest(r io.Reader) ([]manifestRow, error) {
	var rows []manifestRow
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 5 {
			return nil, fmt.Errorf("manifest line needs 5 tab-separated columns: %q", line)
		}
		if f[0] == "file" { // header
			continue
		}
		var tags []string
		for _, t := range strings.Split(f[2], ",") {
			if t = strings.TrimSpace(t); t != "" {
				tags = append(tags, t)
			}
		}
		rows = append(rows, manifestRow{
			File:  strings.TrimSpace(f[0]),
			Scope: strings.TrimSpace(f[1]),
			Tags:  tags,
			Pin:   strings.TrimSpace(f[3]) == "y",
			Keep:  strings.TrimSpace(f[4]) != "skip",
		})
	}
	return rows, sc.Err()
}

// deriveMemoryDoc turns a raw memory file body + its manifest row into the
// document to upsert: path = filename slug, title from frontmatter description
// (fallback name, fallback slug), body = content after the frontmatter, tags =
// manifest tags ∪ frontmatter metadata.type.
func deriveMemoryDoc(body string, row manifestRow) memoryDoc {
	stem := strings.TrimSuffix(row.File, filepath.Ext(row.File))
	fm, start := domain.ParseFrontmatterMap(body)
	content := body
	if start > 0 {
		content = strings.TrimLeft(body[start:], "\n")
	}
	title := stem
	tags := append([]string{}, row.Tags...)
	if fm != nil {
		if d, ok := fm["description"].(string); ok && strings.TrimSpace(d) != "" {
			title = strings.TrimSpace(d)
		} else if n, ok := fm["name"].(string); ok && strings.TrimSpace(n) != "" {
			title = strings.TrimSpace(n)
		}
		if meta, ok := fm["metadata"].(map[string]any); ok {
			if mt, ok := meta["type"].(string); ok && mt != "" {
				tags = appendUnique(tags, mt)
			}
		}
	}
	return memoryDoc{Path: stem, Title: title, Body: content, Tags: tags, Pinned: row.Pin}
}

func appendUnique(ss []string, s string) []string {
	for _, x := range ss {
		if x == s {
			return ss
		}
	}
	return append(ss, s)
}

func migrateMemoriesCmd() *cobra.Command {
	var dir, manifest string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "memories",
		Short: "Import classified memory files into flow (idempotent)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runMigrateMemories(cmd.Context(), cmd.OutOrStdout(), dir, manifest, dryRun)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "memory directory (the source files)")
	cmd.Flags().StringVar(&manifest, "manifest", "", "reviewed TSV manifest")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report without writing")
	_ = cmd.MarkFlagRequired("dir")
	_ = cmd.MarkFlagRequired("manifest")
	return cmd
}

func runMigrateMemories(ctx context.Context, out io.Writer, dir, manifest string, dryRun bool) error {
	mf, err := os.Open(manifest)
	if err != nil {
		return err
	}
	defer func() { _ = mf.Close() }()
	rows, err := parseManifest(mf)
	if err != nil {
		return err
	}

	c, err := clientFromStore(ctx)
	if err != nil {
		return err
	}
	nodes, err := c.ListNodes(ctx)
	if err != nil {
		return err
	}
	slugToID := map[string]string{}
	for _, n := range nodes {
		slugToID[n.Slug] = n.ID
	}

	var imported, skipped int
	for _, row := range rows {
		if !row.Keep || row.File == "MEMORY.md" {
			skipped++
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, row.File))
		if err != nil {
			return fmt.Errorf("read %s: %w", row.File, err)
		}
		doc := deriveMemoryDoc(string(raw), row)

		var nodeID *string
		if row.Scope != "global" {
			id, ok := slugToID[row.Scope]
			if !ok {
				return fmt.Errorf("%s: unknown scope slug %q (not a node)", row.File, row.Scope)
			}
			nodeID = &id
		}

		if dryRun {
			fmt.Fprintf(out, "UPSERT %-45s → %-30s tags=%v pin=%v\n", doc.Path, row.Scope, doc.Tags, doc.Pinned)
			imported++
			continue
		}
		if _, err := c.UpsertDocumentByPath(ctx, apiclient.UpsertByPathInput{
			Type: string(domain.DocMemory), NodeID: nodeID, Path: doc.Path,
			Title: doc.Title, Body: doc.Body, Tags: doc.Tags, Pinned: doc.Pinned,
		}); err != nil {
			return fmt.Errorf("upsert %s: %w", doc.Path, err)
		}
		imported++
	}
	mode := ""
	if dryRun {
		mode = " (dry-run)"
	}
	fmt.Fprintf(out, "\nimported %d, skipped %d%s\n", imported, skipped, mode)
	return nil
}
```

Add a tiny `contains` test helper in the test file if not already present.

- [ ] **Step 4: Write the CLI integration test (fake server)**

In `cmd/flow/context_migrate_test.go`, mirror `docs_import_test.go`'s `httptest.NewServer` pattern: serve `GET /api/v1/nodes` (return one node with a known slug+id) and `PUT /api/v1/documents/by-path` (record bodies); write a temp memory file + manifest; run `runMigrateMemories` non-dry; assert the upsert body carries the right `projectId`, `path`, `tags`, `pinned`. Also assert `--dry-run` writes nothing.

- [ ] **Step 5: Run to verify pass**

Run: `go test ./cmd/flow/ -run 'Manifest|DeriveMemoryDoc|MigrateMemories' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/flow/context_migrate.go cmd/flow/context_migrate_test.go
git commit -m "feat(cli): flow context migrate memories — manifest-driven idempotent import (B3d-2)"
```

---

## Task 9: Composition-root wiring + curl-smoke

**Files:**
- Modify: `cmd/flow-server/main.go`

**Interfaces:**
- Consumes: `usecase.RedesignDocTypes` (Task 4), `usecase.UpsertDocumentByPath` (Task 6); existing `documentStore`, `tagStore`, `clock`, `embedWorker`.

- [ ] **Step 1: Construct the two usecases in the `Server{}` literal**

In `cmd/flow-server/main.go`, next to `StripFrontmatter` (~line 163) and the document usecases:

```go
		StripFrontmatter:    usecase.StripFrontmatter{Docs: documentStore, Clock: clock},
		RedesignDocTypes:    usecase.RedesignDocTypes{Docs: documentStore, Clock: clock},
		UpsertDocumentByPath: usecase.UpsertDocumentByPath{Docs: documentStore, Tags: tagStore, Notifier: embedWorker},
```

- [ ] **Step 2: Build everything**

Run: `go build ./...`
Expected: no errors (the `Server{}` now has every field its routes reference).

- [ ] **Step 3: Curl-smoke each new route against the dev stack** ([[reference_flow_dev_env]])

```bash
make dev-up && make dev-run   # in one shell; export TOKEN=$(make dev-token) in another
# doctypes (dry-run is safe even on a fresh DB)
curl -fsS -X POST -H "Authorization: Bearer $TOKEN" \
  "$FLOW_API/api/v1/maintenance/redesign-doctypes?dry_run=true"      # → {"scanned":N,"converted":N}
# by-path upsert
curl -fsS -X PUT -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"type":"memory","path":"smoke_note","title":"S","body":"hi","tags":["smoke"],"pinned":false}' \
  "$FLOW_API/api/v1/documents/by-path"                               # → {"id":"...","updatedAt":"..."}
```

Expected: both return 200 with the shapes shown. Clean up `smoke_note` afterward (`flow` delete or leave; it is a global memory).

- [ ] **Step 4: Commit**

```bash
git add cmd/flow-server/main.go
git commit -m "feat(server): wire RedesignDocTypes + UpsertDocumentByPath usecases (B3d-5)"
```

- [ ] **Step 5: Run the full CI gate**

Run: `make ci`
Expected: lint 0, all tests pass (incl. pgstore Docker), coverage gate held, binaries build.

---

## Task 10: Classify the 116 memories → manifest (interactive)

**Files:**
- Create: `docs/superpowers/specs/assets/2026-06-29-b3d-manifest/manifest.tsv` (the reviewed artifact; keep it in-repo for provenance)

- [ ] **Step 1: Dispatch a classification subagent**

Dispatch a `code-searcher`/`general-purpose` subagent with this brief (give it full context — it sees none of this conversation):

> Read every `*.md` file in `/Users/msoent/.claude/projects/-Users-msoent-Sourcecode-serverkraken-flow/memory/` **except `MEMORY.md`**. For each, output one TSV row `file⇥scope⇥tags⇥pin⇥keep`:
> - **scope** = one of: `global` (general working-style/cross-project), `github-com-serverkraken-flow` (flow product + flow-tech feedback), `github-com-serverkraken-homelab-study` (deploy/authentik/homelab), `privat` (worktime workflow / personal). Default unsure → `github-com-serverkraken-flow`.
> - **tags** = 0–3 comma-separated topical tags (the file's `metadata.type` is auto-added by the importer, do NOT repeat it).
> - **pin** = `y` for the handful of always-relevant working-style rules (e.g. no_icons, no_monoliths, plan_main_wiring); else blank.
> - **keep** = `skip` for clearly dead/duplicate done-milestone records; else `y`. When in doubt, `y`.
> Emit ONLY the TSV (a `file⇥scope⇥tags⇥pin⇥keep` header then one row per file).

- [ ] **Step 2: Human review (Soenne)**

Save the agent output to `docs/superpowers/specs/assets/2026-06-29-b3d-manifest/manifest.tsv`. Review every row — fix scopes, pins, and skips. This is the one curation pass.

- [ ] **Step 3: Commit the manifest**

```bash
git add docs/superpowers/specs/assets/2026-06-29-b3d-manifest/manifest.tsv
git commit -m "docs(b3d): reviewed memory-migration manifest (116 files) (B3d-3)"
```

---

## Task 11: Run the doctypes redesign live (interactive)

- [ ] **Step 1: Dry-run against the target flow instance**

Run: `flow context migrate doctypes --dry-run`
Expected: `scanned 87 agent docs, converted 87 (dry-run)` (the count the live corpus reports).

- [ ] **Step 2: Apply**

Run: `flow context migrate doctypes`
Expected: `scanned 87 agent docs, converted 87`.

- [ ] **Step 3: Verify**

- `flow_list_docs type=spec` and `type=plan` now return the corpus; `type=agent` returns 0.
- Paths no longer carry `specs/`/`plans/`.
- A previously-dangling cross-spec wikilink resolves: open the WebUI/TUI for the doc that contains `[[2026-06-23-flow-webui-overhaul-design]]` and confirm the backlink shows on the target.

---

## Task 12: Run the memory import live (interactive)

- [ ] **Step 1: Dry-run**

Run: `flow context migrate memories --dir /Users/msoent/.claude/projects/-Users-msoent-Sourcecode-serverkraken-flow/memory --manifest docs/superpowers/specs/assets/2026-06-29-b3d-manifest/manifest.tsv --dry-run`
Expected: one `UPSERT …` line per kept file, ending `imported N, skipped M (dry-run)`. Sanity-check scopes/tags.

- [ ] **Step 2: Apply, then re-run to prove idempotency**

Run the same command without `--dry-run`, then run it a **second** time.
Expected: both succeed; the second run produces the same counts (upsert, no duplicates).

- [ ] **Step 3: Verify via compose**

Run: `flow_get_context` (or `flow context`) for the flow repo.
Expected: the composed block now shows real **Instructions** (after Task 13), **Memories — Leaf** (flow repo memories), and global memories that pass the D7 tag-gate, with a plausible token budget + dropped footer. Spot-check that a same-scope `[[feedback_…]]` link resolves.

---

## Task 13: Seed-split + one-step-write + hook dogfood (interactive)

> CLAUDE.md is **never committed** ([[global rule]]); these are local edits + a flow global doc + a settings change.

- [ ] **Step 1: Create the flow global instruction**

Via `flow_create_doc` (`type: instruction`, `project: none`, `path: working-agreement`): move the non-HARD-RULES sections of `~/.claude/CLAUDE.md` — "How Soenne & Claude Work Together", "Subagent Routing", "CLI Quick Reference" — and **rewrite** "Memory Bank System" + "Flow as cross-device knowledge store" to the post-B3d reality: flow is the memory; write typed/scoped via `flow_create_doc`; activeContext via `flow_set_active_context`; recall via `flow_get_context` / the SessionStart hook.

- [ ] **Step 2: Shrink the local seed**

Edit `~/.claude/CLAUDE.md` down to **only** the `HARD RULES — NEVER VIOLATE` block (banned tools, banned/required behaviors). Leave nothing else.

- [ ] **Step 3: Turn native auto-memory off**

In `~/.claude/settings.json`, disable the native auto-memory feature so flow is the single write target (B3-Kern §9). (Do not touch CLAUDE.md from the installer.)

- [ ] **Step 4: SessionStart hook dogfood** (closes B3-Kern's deferred gate)

Ensure the hook is installed (`flow context install-hooks`). Start a fresh Claude Code session in the flow repo and confirm the injected `# flow context` block shows the migrated Instructions + Active Context + Memories. Stop after doing real work and confirm the Stop reminder fires only when `active-context` is stale. Kill the network and confirm SessionStart still serves the offline cache without hard-failing.

---

## Task 14: Final gate + plan/spec mirror

- [ ] **Step 1: Full CI**

Run: `make ci`
Expected: green (lint 0, tests pass, coverage gate held, binaries build).

- [ ] **Step 2: Mirror this plan + the (updated) spec to flow**

Per the cross-device convention: `flow_create_doc` (`type: agent`, current project = flow) at `plans/2026-06-29-flow-kontext-b3d-doctype-migration`; update the spec doc if it changed during execution.

- [ ] **Step 3: Done-gate sign-off**

Confirm: 87 docs reclassified (0 `agent`); 116 memories imported (minus skips/MEMORY.md), idempotent re-run clean; seed split + auto-memory off; SessionStart loads real composed context. Record the result in the spec's status line.

---

## Self-Review

**Spec coverage:**
- DocType-Redesign enum + `agent` deprecation + `activecontext` type → Tasks 1, 2, 3. ✓
- Path-strip / `agent`→spec/plan transform → Tasks 4, 5, 11. ✓
- SlugOK relaxation (needed for underscore memory slugs) → Task 1. ✓
- Migration tool (CLI, idempotent, dry-run, reusable) → Tasks 5, 6, 7, 8. ✓
- Classification (subagent → manifest → review) → Task 10. ✓
- Ist-Migration apply → Task 12. ✓
- Seed-split + one-step-write (auto-memory off) → Task 13. ✓
- Wiring + done-gate + hook dogfood → Tasks 9, 13, 14. ✓
- Out-of-scope respected: no lifecycle/`veraltet` schema; no cross-scope wikilink resolution; `agent` kept (not removed); only flow+global sources. ✓

**Placeholder scan:** No TBD/TODO; every code step carries real code; the one cross-task dependency (Task 5 ↔ Task 8 `migrateMemoriesCmd`) is called out with a concrete stub strategy.

**Type consistency:** `RedesignDocTypes`/`domain.RedesignReport`/`RedesignedDocType`, `UpsertDocumentByPath`/`UpsertByPathInput`/`UpsertByPathResult`, `DocSpec`/`DocActiveContext`, route paths (`/maintenance/redesign-doctypes`, `/documents/by-path`) are used identically across tasks. The apiclient `UpsertByPathInput.Type` is `string` (JSON); the usecase `UpsertByPathInput.Type` is `domain.DocumentType` (the handler converts) — intentional, matching the existing `CreateDocumentInput` pattern.
