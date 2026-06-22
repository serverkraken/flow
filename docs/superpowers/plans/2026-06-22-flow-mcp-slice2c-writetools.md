# flow-mcp Slice 2c (Write Tools + Guard + Resources) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the three Kompendium **write** tools — `flow_create_doc`, `flow_update_doc`, `flow_delete_doc` — with the anti-clobber **write guard** (spec §6), plus the document **resources** layer (spec §7.1): one `flow://doc/{id}` resource per document of the resolved project, registered at boot and kept live as documents are created / updated / deleted.

**Architecture:** A **pure client-side** MCP layer over `cmd/flow-mcp`, thin over the existing `apiclient` write methods, reusing the 2a spine (`handlers`, `textResult`/`errorResult`/`loginRequired`, always-registered + degraded-mode invariants) and the 2b support layer (`resolveScope`, `projectName`). Two genuinely new pieces: (1) the **write guard** — `flow_update_doc`/`flow_delete_doc` fetch the target first (also enabling partial-update merge) and refuse to mutate a **human-owned** type (`daily`/`project`/`free`) unless `confirm=true`; `flow_create_doc` only adds, so it is unguarded. (2) the **resources** layer — `newServer` now keeps a back-reference to the `*mcp.Server` so the write handlers can `AddResource`/`RemoveResources` live; resource bodies are always read fresh via `GetDocument` so they never go stale.

**Tech Stack:** Go 1.26.x, `github.com/modelcontextprotocol/go-sdk/mcp` v1.6.1 (already in go.mod), existing `internal/adapter/apiclient`, `internal/domain`.

## Global Constraints

- Go module `github.com/serverkraken/flow`.
- **stdout is reserved for the JSON-RPC stream. ALL logging goes to stderr.** This slice adds no logging; never introduce a `fmt.Println`/`log.*`/default-logger write that could reach stdout.
- **No interactive auth / never crash.** Every tool short-circuits when unauthenticated: `if !h.authed { return h.loginRequired(), nil, nil }`. The tools are always registered so `tools/list` is stable regardless of auth.
- **No backend / server / pgstore / apiclient / migration change.** 2c is additive over `cmd/flow-mcp` only, except a single-source `HumanOwned()` method in `internal/domain/document.go` (Task 1). The four agent-owned document types (`memory`/`instruction`/`skill`/`plan`) already exist in `domain` (added before 2a); **no domain type constants and no migration are added here.**
- **CI gate is `golangci-lint run`** (+ `verify-generate` + `cover` + `build`), NOT plain gofmt. Verify with `golangci-lint run`, not `gofmt -l` (local gofmt is go1.26.4 and reports pre-existing committed files as false positives).
- **pgstore/other Docker tests** in the full suite need `export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"` and `export TESTCONTAINERS_RYUK_DISABLED=true`. This slice adds no Docker tests, but `make ci`/`cover` runs the whole suite, so set this env when running the full gate.
- Result text is concise plain text (the model reads it); errors set `IsError: true` with an actionable message — never a silent success/empty.
- Commit message trailers (every commit):
  ```
  Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
  ```

**As-built interfaces this slice consumes (verified against the codebase — do NOT re-derive):**
- `apiclient.Client.CreateDocument(ctx, apiclient.CreateDocumentInput{Type string, ProjectID *string, Path, Title, Body string}) (domain.Document, error)` → `POST /api/v1/documents`
- `apiclient.Client.UpdateDocument(ctx, id string, apiclient.UpdateDocumentInput{Title, Body string}) (domain.Document, error)` → `PUT /api/v1/documents/{id}` (**both fields required** — hence fetch-then-merge)
- `apiclient.Client.DeleteDocument(ctx, id string) error` → `DELETE /api/v1/documents/{id}`
- `apiclient.Client.GetDocument(ctx, id string) (domain.Document, error)` → `GET /api/v1/documents/{id}`
- `apiclient.Client.ListDocumentsScoped(ctx, projectID *string, tags ...string) ([]domain.Document, error)` → used to enumerate the resolved project's docs at boot (resources)
- `apiclient.New(base, token string) *Client`
- `domain.Document{ ID, OwnerID string; ProjectID *string; Type DocumentType; Path, Title, Body string; Tags []string; Role *string; ... }`
- `domain.DocumentTypes() []DocumentType` (canonical type set, single-source from 2b); `domain.DocDaily/DocProject/DocFree` (human-owned), all others agent-owned.
- 2a spine (`cmd/flow-mcp`): `handlers{ client *apiclient.Client; authed bool; proj domain.Project; matched bool; projMu sync.Mutex; projects []domain.Project; projFetched bool; listProjects func(...)... }`; `newServer(client, authed, proj, matched) *mcp.Server`; `textResult`/`errorResult`/`(*handlers).loginRequired()`.
- 2b support: `(*handlers).resolveScope(ctx, project string) (scope, error)` where `scope{ projectID *string; label string }`; `(*handlers).projectName(ctx, id *string) string`. **`resolveScope` returns `&"none"` for the `"none"` sentinel and `nil` for `"global"`/unmatched-default — for `create` both `&"none"` and `nil` mean "no project assigned", so map `&"none"` → `nil` before building `CreateDocumentInput`.**
- go-sdk: `mcp.AddTool(s, &mcp.Tool{Name, Description}, handler)`; handler signature `func(ctx, *mcp.CallToolRequest, In) (*mcp.CallToolResult, any, error)`; a field is **required** unless its `json` tag carries `,omitempty`.
- go-sdk resources: `(*mcp.Server).AddResource(r *mcp.Resource, h mcp.ResourceHandler)`; `(*mcp.Server).RemoveResources(uris ...string)`; `mcp.Resource{ URI, Name, Description, MIMEType string }`; `mcp.ResourceHandler = func(ctx, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error)`; `mcp.ReadResourceResult{ Contents []*mcp.ResourceContents }`; `mcp.ResourceContents{ URI, MIMEType, Text string }`. The SDK emits `listChanged` on add/remove. Loopback client: `mcp.ClientSession.ListResources(ctx, nil)` → `{Resources []*mcp.Resource}`; `mcp.ClientSession.ReadResource(ctx, &mcp.ReadResourceParams{URI})` → `{Contents []*mcp.ResourceContents}`.

---

### Task 1: Ownership helper + write-guard / partial-merge support layer

The pure, unit-tested foundation: a single-source `HumanOwned()` classifier in `domain`, plus the create-input validators and the partial-update merge helper in `cmd/flow-mcp`. No tools or resources registered in this task.

**Files:**
- Modify: `internal/domain/document.go` (add `func (t DocumentType) HumanOwned() bool`)
- Create: `internal/domain/document_owned_test.go`
- Create: `cmd/flow-mcp/write.go` (`requireType`, `mergeUpdate`, `guardMutation`)
- Create: `cmd/flow-mcp/write_test.go`

**Interfaces:**
- Consumes: `domain.Document/DocumentType/DocumentTypes`; `apiclient.UpdateDocumentInput`.
- Produces (consumed by Task 2):
  - `domain.DocumentType.HumanOwned() bool` — `true` for `daily`/`project`/`free`, `false` for everything else (type-agnostic: the agent-owned set is "not human-owned", so future agent types fall on the free side automatically).
  - `requireType(typ string) (domain.DocumentType, error)` — like 2b's `checkType` but `""` is an error ("type is required"); reuses `domain.DocumentTypes()` for the valid set + message.
  - `mergeUpdate(cur domain.Document, title, body *string) (apiclient.UpdateDocumentInput, error)` — carries missing fields from `cur`; errors when both `title` and `body` are nil ("nothing to update: pass title and/or body").
  - `guardMutation(d domain.Document, confirm bool) error` — nil when allowed; an actionable error when `d.Type.HumanOwned() && !confirm`.

- [ ] **Step 1: Write the ownership test first (RED)**

Create `internal/domain/document_owned_test.go`:

```go
package domain

import "testing"

func TestHumanOwned(t *testing.T) {
	human := []DocumentType{DocDaily, DocProject, DocFree}
	agent := []DocumentType{DocAgent, DocMemory, DocInstruction, DocSkill, DocPlan}
	for _, dt := range human {
		if !dt.HumanOwned() {
			t.Errorf("%q should be human-owned", dt)
		}
	}
	for _, dt := range agent {
		if dt.HumanOwned() {
			t.Errorf("%q should be agent-owned", dt)
		}
	}
	// A future / unknown type defaults to agent-owned (not guarded).
	if DocumentType("future-kind").HumanOwned() {
		t.Error("an unknown type should default to agent-owned (not human-owned)")
	}
}
```

Run: `go test ./internal/domain/ -run TestHumanOwned` → FAIL (`undefined: HumanOwned`).

- [ ] **Step 2: Add `HumanOwned()`**

In `internal/domain/document.go`, after `valid()`:

```go
// HumanOwned reports whether documents of this type are authored by the human
// (daily / project / free notes) rather than the agent. It drives flow-mcp's
// write guard: mutating a human-owned document needs explicit confirmation.
// Expressed as a positive set so any future (agent) type is unguarded by default.
func (t DocumentType) HumanOwned() bool {
	switch t {
	case DocDaily, DocProject, DocFree:
		return true
	default:
		return false
	}
}
```

Run: `go test ./internal/domain/` → PASS.

- [ ] **Step 3: Write `write_test.go` (RED)**

Create `cmd/flow-mcp/write_test.go`:

```go
package main

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func sp(s string) *string { return &s }

func TestRequireType(t *testing.T) {
	if got, err := requireType("memory"); err != nil || got != domain.DocMemory {
		t.Fatalf("requireType(memory) = (%q,%v), want (memory,nil)", got, err)
	}
	if _, err := requireType(""); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("requireType(\"\") err = %v, want a 'required' error", err)
	}
	if _, err := requireType("bogus"); err == nil || !strings.Contains(err.Error(), "memory") {
		t.Fatalf("requireType(bogus) err = %v, want it to list valid types", err)
	}
}

func TestMergeUpdate(t *testing.T) {
	cur := domain.Document{Title: "Old", Body: "old body"}
	// title only → body carried over
	got, err := mergeUpdate(cur, sp("New"), nil)
	if err != nil || got.Title != "New" || got.Body != "old body" {
		t.Fatalf("title-only merge = (%+v,%v), want New/old body", got, err)
	}
	// body only → title carried over
	got, err = mergeUpdate(cur, nil, sp("new body"))
	if err != nil || got.Title != "Old" || got.Body != "new body" {
		t.Fatalf("body-only merge = (%+v,%v), want Old/new body", got, err)
	}
	// both nil → error
	if _, err := mergeUpdate(cur, nil, nil); err == nil {
		t.Fatal("merge with no fields should error")
	}
}

func TestGuardMutation(t *testing.T) {
	human := domain.Document{ID: "d1", Type: domain.DocFree}
	agent := domain.Document{ID: "d2", Type: domain.DocMemory}
	if err := guardMutation(agent, false); err != nil {
		t.Fatalf("agent-owned without confirm should pass, got %v", err)
	}
	if err := guardMutation(human, true); err != nil {
		t.Fatalf("human-owned WITH confirm should pass, got %v", err)
	}
	err := guardMutation(human, false)
	if err == nil || !strings.Contains(err.Error(), "confirm") || !strings.Contains(err.Error(), "free") {
		t.Fatalf("human-owned without confirm = %v, want an error naming confirm + the type", err)
	}
}
```

Run: `go test ./cmd/flow-mcp/ -run 'RequireType|MergeUpdate|GuardMutation'` → FAIL (compile: undefined).

- [ ] **Step 4: Write `cmd/flow-mcp/write.go`**

```go
package main

import (
	"fmt"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// requireType validates a required `type` argument for create against the
// canonical document-type set. Unlike the read tools' optional checkType, an
// empty value is an error.
func requireType(typ string) (domain.DocumentType, error) {
	t := strings.TrimSpace(typ)
	if t == "" {
		return "", fmt.Errorf("type is required. Valid types: %s", typeList())
	}
	for _, v := range domain.DocumentTypes() {
		if domain.DocumentType(t) == v {
			return v, nil
		}
	}
	return "", fmt.Errorf("invalid type %q. Valid types: %s", t, typeList())
}

// mergeUpdate builds the apiclient update payload from the current document and
// the optionally-supplied fields, carrying over whatever the caller omitted.
// UpdateDocumentInput requires both Title and Body, so a partial MCP update is
// realized as fetch-current-then-merge. At least one field must be supplied.
func mergeUpdate(cur domain.Document, title, body *string) (apiclient.UpdateDocumentInput, error) {
	if title == nil && body == nil {
		return apiclient.UpdateDocumentInput{}, fmt.Errorf("nothing to update: pass title and/or body")
	}
	out := apiclient.UpdateDocumentInput{Title: cur.Title, Body: cur.Body}
	if title != nil {
		out.Title = *title
	}
	if body != nil {
		out.Body = *body
	}
	return out, nil
}

// guardMutation enforces the anti-clobber write guard: a human-owned document
// (daily / project / free) may only be modified or deleted with confirm=true.
func guardMutation(d domain.Document, confirm bool) error {
	if d.Type.HumanOwned() && !confirm {
		return fmt.Errorf("%s is a human-owned note (type=%s). Pass confirm=true to modify it.", d.ID, d.Type)
	}
	return nil
}
```

Run: `go test ./cmd/flow-mcp/ -run 'RequireType|MergeUpdate|GuardMutation' -v` → PASS.

- [ ] **Step 5: Build + lint + commit**

```
go test ./internal/domain/ ./cmd/flow-mcp/
go build ./...
golangci-lint run ./cmd/flow-mcp/... ./internal/domain/...
```
Expected: all PASS; build OK; lint 0 issues (the unchanged 2a/2b tests still compile and pass).

```bash
git add internal/domain/document.go internal/domain/document_owned_test.go cmd/flow-mcp/write.go cmd/flow-mcp/write_test.go
git commit -m "feat(flow-mcp): write-guard support layer (HumanOwned + requireType/mergeUpdate/guardMutation)" -m "Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: The three write tools + guard (loopback)

Register `flow_create_doc`, `flow_update_doc`, `flow_delete_doc`, each a thin handler over apiclient using the Task 1 support layer + the 2b `resolveScope`, and prove them end-to-end with a writable loopback backend. (Resource side-effects are deliberately **not** wired here — they land in Task 3, where the resource layer is introduced; keeping them apart keeps each task's loopback backend small. The write handlers gain a single resource-sync call in Task 3.)

**Files:**
- Create: `cmd/flow-mcp/tools_write.go` (param structs + the 3 handlers)
- Modify: `cmd/flow-mcp/server.go` (register the 3 tools in `newServer`)
- Create: `cmd/flow-mcp/loopback_write_test.go` (writable fixture backend + assertions; reuses `connect`/`callText`/`text`/`hasTool` from the existing test files — same package)

**Interfaces:**
- Consumes: Task 1 (`requireType`/`mergeUpdate`/`guardMutation`); 2b `resolveScope`/`projectName`; `apiclient.{CreateDocument,UpdateDocument,DeleteDocument,GetDocument}`; 2a `textResult`/`errorResult`/`loginRequired`.
- Produces: registered tools `flow_create_doc`/`flow_update_doc`/`flow_delete_doc` (consumed by Task 3's resource sync + the live done-gate).

- [ ] **Step 1: Write `loopback_write_test.go` (RED)**

Create `cmd/flow-mcp/loopback_write_test.go`. The fixture backend keeps an in-memory doc map and serves create/get/update/delete; it starts with one human-owned doc (`d-human`, type `free`) so the guard can be exercised.

```go
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// fakeWriteBackend serves the CRUD endpoints the write tools touch, backed by an
// in-memory map. p1 = the resolved project (Alpha).
func fakeWriteBackend(t *testing.T) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	p1 := "p1"
	docs := map[string]domain.Document{
		"d-human": {ID: "d-human", OwnerID: "u1", ProjectID: &p1, Type: domain.DocFree, Path: "notes/keep", Title: "Keep", Body: "human note"},
	}
	seq := 0
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/projects", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Project{{ID: "p1", Name: "Alpha", Slug: "alpha"}})
	})
	mux.HandleFunc("POST /api/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		var in apiclient.CreateDocumentInput
		_ = json.NewDecoder(r.Body).Decode(&in)
		mu.Lock()
		defer mu.Unlock()
		seq++
		id := "new" + string(rune('0'+seq))
		d := domain.Document{ID: id, OwnerID: "u1", ProjectID: in.ProjectID, Type: domain.DocumentType(in.Type), Path: in.Path, Title: in.Title, Body: in.Body}
		docs[id] = d
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(d)
	})
	mux.HandleFunc("GET /api/v1/documents/{id}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		d, ok := docs[r.PathValue("id")]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(d)
	})
	mux.HandleFunc("PUT /api/v1/documents/{id}", func(w http.ResponseWriter, r *http.Request) {
		var in apiclient.UpdateDocumentInput
		_ = json.NewDecoder(r.Body).Decode(&in)
		mu.Lock()
		defer mu.Unlock()
		d, ok := docs[r.PathValue("id")]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		d.Title, d.Body = in.Title, in.Body
		docs[d.ID] = d
		_ = json.NewEncoder(w).Encode(d)
	})
	mux.HandleFunc("DELETE /api/v1/documents/{id}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		delete(docs, r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	})
	// list (used by resolveScope's nothing here, but harmless to provide)
	mux.HandleFunc("GET /api/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		var out []domain.Document
		pid, has := r.URL.Query().Get("projectId"), r.URL.Query().Has("projectId")
		for _, d := range docs {
			if !has || (d.ProjectID != nil && *d.ProjectID == pid) {
				out = append(out, d)
			}
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	return httptest.NewServer(mux)
}

func authedWriteServer(t *testing.T) *mcp.ClientSession {
	t.Helper()
	be := fakeWriteBackend(t)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	return connect(t, newServer(client, true, domain.Project{ID: "p1", Name: "Alpha", Slug: "alpha"}, true))
}

func TestLoopback_WriteTools_Advertised(t *testing.T) {
	sess := authedWriteServer(t)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"flow_create_doc", "flow_update_doc", "flow_delete_doc"} {
		if !hasTool(tools.Tools, name) {
			t.Fatalf("%s not advertised; got %v", name, toolNames(tools.Tools))
		}
	}
}

func TestLoopback_CreateThenGet(t *testing.T) {
	sess := authedWriteServer(t)
	res, out := callText(t, sess, "flow_create_doc", map[string]any{
		"type": "memory", "path": "notes/new", "title": "New", "body": "fresh body",
	})
	if res.IsError {
		t.Fatalf("create errored: %s", out)
	}
	if !strings.Contains(out, "new1") || !strings.Contains(out, "memory") {
		t.Fatalf("create result = %q, want it to name the new id + type", out)
	}
	_, got := callText(t, sess, "flow_get_doc", map[string]any{"id": "new1"})
	if !strings.Contains(got, "fresh body") {
		t.Fatalf("get after create = %q, want the body", got)
	}
}

func TestLoopback_CreateInvalidType(t *testing.T) {
	sess := authedWriteServer(t)
	res, out := callText(t, sess, "flow_create_doc", map[string]any{"type": "bogus", "path": "p", "title": "T", "body": "B"})
	if !res.IsError || !strings.Contains(out, "memory") {
		t.Fatalf("create bad type = (IsError=%v, %q), want IsError listing valid types", res.IsError, out)
	}
}

func TestLoopback_UpdateGuard(t *testing.T) {
	sess := authedWriteServer(t)
	// human-owned (free) without confirm → refused
	res, out := callText(t, sess, "flow_update_doc", map[string]any{"id": "d-human", "title": "Hacked"})
	if !res.IsError || !strings.Contains(out, "confirm") {
		t.Fatalf("guarded update = (IsError=%v, %q), want refusal naming confirm", res.IsError, out)
	}
	// with confirm → allowed, body carried over (partial merge)
	res, out = callText(t, sess, "flow_update_doc", map[string]any{"id": "d-human", "title": "Edited", "confirm": true})
	if res.IsError {
		t.Fatalf("confirmed update errored: %s", out)
	}
	_, got := callText(t, sess, "flow_get_doc", map[string]any{"id": "d-human"})
	if !strings.Contains(got, "Edited") || !strings.Contains(got, "human note") {
		t.Fatalf("after confirmed partial update = %q, want new title + carried-over body", got)
	}
}

func TestLoopback_UpdateAgentOwnedNoConfirm(t *testing.T) {
	sess := authedWriteServer(t)
	_, _ = callText(t, sess, "flow_create_doc", map[string]any{"type": "memory", "path": "notes/a", "title": "A", "body": "B"})
	res, out := callText(t, sess, "flow_update_doc", map[string]any{"id": "new1", "body": "B2"})
	if res.IsError {
		t.Fatalf("agent-owned update without confirm should succeed, got error: %s", out)
	}
}

func TestLoopback_DeleteGuard(t *testing.T) {
	sess := authedWriteServer(t)
	// human-owned delete without confirm → refused
	res, out := callText(t, sess, "flow_delete_doc", map[string]any{"id": "d-human"})
	if !res.IsError || !strings.Contains(out, "confirm") {
		t.Fatalf("guarded delete = (IsError=%v, %q), want refusal", res.IsError, out)
	}
	// with confirm → gone
	res, _ = callText(t, sess, "flow_delete_doc", map[string]any{"id": "d-human", "confirm": true})
	if res.IsError {
		t.Fatal("confirmed delete should succeed")
	}
	res, _ = callText(t, sess, "flow_get_doc", map[string]any{"id": "d-human"})
	if !res.IsError {
		t.Fatal("get after delete should error (not found)")
	}
}

func TestLoopback_WriteTools_DegradedRequireLogin(t *testing.T) {
	sess := connect(t, newServer(nil, false, domain.Project{}, false))
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"flow_create_doc", map[string]any{"type": "memory", "path": "p", "title": "T", "body": "B"}},
		{"flow_update_doc", map[string]any{"id": "d1", "title": "X"}},
		{"flow_delete_doc", map[string]any{"id": "d1"}},
	} {
		res, got := callText(t, sess, tc.name, tc.args)
		if !res.IsError || !strings.Contains(got, "Login required") {
			t.Fatalf("%s degraded = (IsError=%v, %q), want Login required", tc.name, res.IsError, got)
		}
	}
}
```

> Note: `loopback_write_test.go` imports `mcp` only via `*mcp.ClientSession` in `authedWriteServer`; add `"github.com/modelcontextprotocol/go-sdk/mcp"` to its imports. `callText` is defined in the existing `loopback_test.go` (2b) — same package, do not redefine it.

Run: `go test ./cmd/flow-mcp/ -run Loopback` → FAIL (compile: `newServer` doesn't register the write tools; handlers undefined).

- [ ] **Step 2: Write `cmd/flow-mcp/tools_write.go`**

```go
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

type createDocIn struct {
	Path    string `json:"path" jsonschema:"the document path (hierarchical slug, e.g. notes/architecture)"`
	Title   string `json:"title" jsonschema:"the document title"`
	Body    string `json:"body" jsonschema:"the markdown body; tags are set via YAML frontmatter in the body"`
	Type    string `json:"type" jsonschema:"the document type: daily, project, free, agent, memory, instruction, skill, or plan"`
	Project string `json:"project,omitempty" jsonschema:"project slug, name, or id to create in; 'global'/'none' for an unassigned document; omit to use the current directory's project"`
}

func (h *handlers) createDoc(ctx context.Context, _ *mcp.CallToolRequest, in createDocIn) (*mcp.CallToolResult, any, error) {
	if !h.authed {
		return h.loginRequired(), nil, nil
	}
	typ, err := requireType(in.Type)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	if strings.TrimSpace(in.Path) == "" || strings.TrimSpace(in.Title) == "" {
		return errorResult("path and title are required"), nil, nil
	}
	sc, err := h.resolveScope(ctx, in.Project)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	pid := sc.projectID
	if pid != nil && *pid == "none" { // "none"/"global" both mean unassigned for create
		pid = nil
	}
	d, err := h.client.CreateDocument(ctx, apiclient.CreateDocumentInput{
		Type: string(typ), ProjectID: pid, Path: in.Path, Title: in.Title, Body: in.Body,
	})
	if err != nil {
		return errorResult(fmt.Sprintf("flow server error: %v", err)), nil, nil
	}
	return textResult(fmt.Sprintf("Created %s [%s] %s · %s.", d.Type, d.ID, d.Title, d.Path)), nil, nil
}

type updateDocIn struct {
	ID      string  `json:"id" jsonschema:"the document id to update"`
	Title   *string `json:"title,omitempty" jsonschema:"new title; omit to keep the current title"`
	Body    *string `json:"body,omitempty" jsonschema:"new markdown body; omit to keep the current body"`
	Confirm bool    `json:"confirm,omitempty" jsonschema:"required (true) to modify a human-owned note (daily/project/free)"`
}

func (h *handlers) updateDoc(ctx context.Context, _ *mcp.CallToolRequest, in updateDocIn) (*mcp.CallToolResult, any, error) {
	if !h.authed {
		return h.loginRequired(), nil, nil
	}
	if strings.TrimSpace(in.ID) == "" {
		return errorResult("id is required"), nil, nil
	}
	cur, err := h.client.GetDocument(ctx, in.ID)
	if err != nil {
		return errorResult(fmt.Sprintf("flow server error: %v", err)), nil, nil
	}
	if err := guardMutation(cur, in.Confirm); err != nil {
		return errorResult(err.Error()), nil, nil
	}
	payload, err := mergeUpdate(cur, in.Title, in.Body)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	d, err := h.client.UpdateDocument(ctx, in.ID, payload)
	if err != nil {
		return errorResult(fmt.Sprintf("flow server error: %v", err)), nil, nil
	}
	return textResult(fmt.Sprintf("Updated [%s] %s · %s.", d.ID, d.Title, d.Path)), nil, nil
}

type deleteDocIn struct {
	ID      string `json:"id" jsonschema:"the document id to delete"`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"required (true) to delete a human-owned note (daily/project/free)"`
}

func (h *handlers) deleteDoc(ctx context.Context, _ *mcp.CallToolRequest, in deleteDocIn) (*mcp.CallToolResult, any, error) {
	if !h.authed {
		return h.loginRequired(), nil, nil
	}
	if strings.TrimSpace(in.ID) == "" {
		return errorResult("id is required"), nil, nil
	}
	cur, err := h.client.GetDocument(ctx, in.ID)
	if err != nil {
		return errorResult(fmt.Sprintf("flow server error: %v", err)), nil, nil
	}
	if err := guardMutation(cur, in.Confirm); err != nil {
		return errorResult(err.Error()), nil, nil
	}
	if err := h.client.DeleteDocument(ctx, in.ID); err != nil {
		return errorResult(fmt.Sprintf("flow server error: %v", err)), nil, nil
	}
	return textResult(fmt.Sprintf("Deleted [%s] %s.", cur.ID, cur.Title)), nil, nil
}
```

- [ ] **Step 3: Register the three tools in `newServer`**

In `cmd/flow-mcp/server.go`, after the `flow_backlinks` registration, before `return s`:

```go
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_create_doc",
		Description: "Create a Kompendium document in the current project by default. Tags are set via YAML frontmatter in the body. Type must be one of: daily, project, free, agent, memory, instruction, skill, plan.",
	}, h.createDoc)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_update_doc",
		Description: "Update a document's title and/or body by id (partial: omit a field to keep it). Modifying a human-owned note (daily/project/free) requires confirm=true.",
	}, h.updateDoc)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_delete_doc",
		Description: "Delete a document by id. Deleting a human-owned note (daily/project/free) requires confirm=true.",
	}, h.deleteDoc)
```

- [ ] **Step 4: Loopback GREEN + build + lint**

```
go test ./cmd/flow-mcp/ -run Loopback -v
go build ./...
golangci-lint run ./cmd/flow-mcp/...
```
Expected: all 2a/2b/2c loopback tests PASS; build OK; lint 0 issues.

- [ ] **Step 5: Commit**

```bash
git add cmd/flow-mcp/tools_write.go cmd/flow-mcp/server.go cmd/flow-mcp/loopback_write_test.go
git commit -m "feat(flow-mcp): three Kompendium write tools (create/update/delete) with anti-clobber guard" -m "Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Document resources (boot registration + live sync on write)

Add the `flow://doc/{id}` resource layer (spec §7.1): a back-reference to the `*mcp.Server` on `handlers`, a boot-time registration of the resolved project's documents, fresh-on-read bodies, and live add/remove/refresh as the Task 2 write tools mutate documents.

**Files:**
- Modify: `cmd/flow-mcp/server.go` (add `srv *mcp.Server` to `handlers`; set `h.srv = s` in `newServer`)
- Create: `cmd/flow-mcp/resources.go` (`docURI`, `resourceFor`, `(*handlers).registerResources`, `(*handlers).addResource`, `(*handlers).removeResource`)
- Modify: `cmd/flow-mcp/tools_write.go` (call the resource-sync helpers after a successful create/update/delete)
- Modify: `cmd/flow-mcp/main.go` (call `registerResources` when authed && matched)
- Modify: `cmd/flow-mcp/loopback_write_test.go` (add resource-lifecycle assertions)

**Interfaces:**
- Consumes: `(*mcp.Server).AddResource/RemoveResources`; `apiclient.{ListDocumentsScoped,GetDocument}`; `domain.Document`.
- Produces: `docURI(id string) string` (= `"flow://doc/"+id`); `(*handlers).registerResources(ctx) error`; `(*handlers).addResource(d domain.Document)`; `(*handlers).removeResource(id string)`; `(*handlers).inScope(d domain.Document) bool`.

- [ ] **Step 1: Add the resource-lifecycle assertions to `loopback_write_test.go` (RED)**

Append (the helper `authedWriteServer` already builds an authed+matched server; call `registerResources` via a small exported-to-package seam — register at the start of each resource test through a new helper `authedWriteServerWithResources`):

```go
func authedWriteServerWithResources(t *testing.T) (*mcp.ClientSession, *handlers) {
	t.Helper()
	be := fakeWriteBackend(t)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	proj := domain.Project{ID: "p1", Name: "Alpha", Slug: "alpha"}
	h := &handlers{client: client, authed: true, proj: proj, matched: true, listProjects: client.ListProjects}
	srv := newServerForHandlers(h) // see Step 2 note
	if err := h.registerResources(context.Background()); err != nil {
		t.Fatalf("registerResources: %v", err)
	}
	return connect(t, srv), h
}

func TestLoopback_Resources_BootAndLiveSync(t *testing.T) {
	sess, _ := authedWriteServerWithResources(t)
	ctx := context.Background()

	// boot: the one seeded project doc (d-human) is a resource
	rs, err := sess.ListResources(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasResource(rs.Resources, "flow://doc/d-human") {
		t.Fatalf("boot resources = %v, want d-human", resourceURIs(rs.Resources))
	}
	// read returns the (fresh) body
	rr, err := sess.ReadResource(ctx, &mcp.ReadResourceParams{URI: "flow://doc/d-human"})
	if err != nil || len(rr.Contents) == 0 || !strings.Contains(rr.Contents[0].Text, "human note") {
		t.Fatalf("read d-human = (%+v,%v), want the body", rr, err)
	}
	// create in-project → a new resource appears
	_, _ = callText(t, sess, "flow_create_doc", map[string]any{"type": "memory", "path": "notes/r", "title": "R", "body": "rbody"})
	rs, _ = sess.ListResources(ctx, nil)
	if !hasResource(rs.Resources, "flow://doc/new1") {
		t.Fatalf("after create resources = %v, want new1", resourceURIs(rs.Resources))
	}
	// delete (agent-owned, no confirm needed) → resource removed
	_, _ = callText(t, sess, "flow_delete_doc", map[string]any{"id": "new1"})
	rs, _ = sess.ListResources(ctx, nil)
	if hasResource(rs.Resources, "flow://doc/new1") {
		t.Fatalf("after delete resources still has new1: %v", resourceURIs(rs.Resources))
	}
}

func hasResource(rs []*mcp.Resource, uri string) bool {
	for _, r := range rs {
		if r.URI == uri {
			return true
		}
	}
	return false
}

func resourceURIs(rs []*mcp.Resource) []string {
	var u []string
	for _, r := range rs {
		u = append(u, r.URI)
	}
	return u
}
```

> Implementation note for the worker: rather than a separate `newServerForHandlers`, prefer to **refactor `newServer` to build `h` first, set `h.srv = s`, register tools, and return `s`** (Step 2), then have the test construct its own `handlers`, call a tiny `newServerFromHandlers(h)` that does the AddTool wiring + sets `h.srv`. Pick whichever keeps `newServer(client, authed, proj, matched)` as the production entry point unchanged in signature. The simplest path: keep `newServer` as-is for production, and in the test build the server via `newServer(client, true, proj, true)` then obtain the `*handlers` it created. Since `newServer` does not currently return the handlers, **add an internal constructor** `newServerH(client, authed, proj, matched) (*mcp.Server, *handlers)` that `newServer` delegates to; the test uses `newServerH`. Update this step's helper to use `newServerH`.

- [ ] **Step 2: Add the server back-reference + internal constructor in `server.go`**

Add `srv *mcp.Server` to the `handlers` struct. Refactor `newServer` to delegate to a new `newServerH` that also returns the handlers, and sets `h.srv`:

```go
func newServer(client *apiclient.Client, authed bool, proj domain.Project, matched bool) *mcp.Server {
	s, _ := newServerH(client, authed, proj, matched)
	return s
}

// newServerH is newServer but also returns the handlers it wired — used by tests
// (and main, for resource registration) that need the live *handlers.
func newServerH(client *apiclient.Client, authed bool, proj domain.Project, matched bool) (*mcp.Server, *handlers) {
	h := &handlers{client: client, authed: authed, proj: proj, matched: matched}
	if client != nil {
		h.listProjects = client.ListProjects
	}
	s := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: serverVersion}, nil)
	h.srv = s
	// ... all existing mcp.AddTool calls (read + write tools) ...
	return s, h
}
```

Update the test helper from Step 1 to `srv, h := newServerH(client, true, proj, true)`.

- [ ] **Step 3: Write `cmd/flow-mcp/resources.go`**

```go
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/domain"
)

const docURIPrefix = "flow://doc/"

func docURI(id string) string { return docURIPrefix + id }

// inScope reports whether a document belongs to the resolved project — only such
// documents are exposed as resources.
func (h *handlers) inScope(d domain.Document) bool {
	return h.matched && d.ProjectID != nil && *d.ProjectID == h.proj.ID
}

// resourceFor builds the resource descriptor for a document.
func resourceFor(d domain.Document) *mcp.Resource {
	desc := fmt.Sprintf("%s · %s", d.Path, d.Type)
	if len(d.Tags) > 0 {
		desc += " · " + strings.Join(d.Tags, ", ")
	}
	return &mcp.Resource{
		URI:         docURI(d.ID),
		Name:        d.Title,
		Description: desc,
		MIMEType:    "text/markdown",
	}
}

// registerResources lists the resolved project's documents and registers a
// resource per document. No-op when unauthenticated or no project is bound.
func (h *handlers) registerResources(ctx context.Context) error {
	if !h.authed || !h.matched {
		return nil
	}
	docs, err := h.client.ListDocumentsScoped(ctx, &h.proj.ID)
	if err != nil {
		return err
	}
	for _, d := range docs {
		h.addResource(d)
	}
	return nil
}

// addResource registers (or refreshes) a document's resource. The read handler
// fetches the body fresh via GetDocument so content never goes stale.
func (h *handlers) addResource(d domain.Document) {
	if h.srv == nil || !h.inScope(d) {
		return
	}
	id := d.ID
	h.srv.AddResource(resourceFor(d), func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		doc, err := h.client.GetDocument(ctx, id)
		if err != nil {
			return nil, err
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI: docURI(id), MIMEType: "text/markdown", Text: doc.Body,
		}}}, nil
	})
}

// removeResource unregisters a document's resource (safe if it was never added).
func (h *handlers) removeResource(id string) {
	if h.srv == nil {
		return
	}
	h.srv.RemoveResources(docURI(id))
}
```

- [ ] **Step 4: Wire resource sync into the write handlers (`tools_write.go`)**

- In `createDoc`, after a successful create: `h.addResource(d)`.
- In `updateDoc`, after a successful update: `h.removeResource(d.ID); h.addResource(d)` (refreshes the title/description; `addResource` no-ops if out of scope).
- In `deleteDoc`, after a successful delete: `h.removeResource(cur.ID)`.

(`addResource` already guards on scope + nil server, so these calls are unconditional and safe.)

- [ ] **Step 5: Register resources at boot in `main.go`**

In `main`, replace `srv := newServer(...)` with the handler-returning constructor and register resources before `Run`:

```go
	srv, h := newServerH(client, authed, proj, matched)
	if err := h.registerResources(ctx); err != nil {
		log.Warn("could not register document resources", "err", err)
	}
	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
		...
	}
```

(Boot must never crash on a resource-listing failure — log to stderr and start anyway.)

- [ ] **Step 6: Loopback GREEN + build + lint**

```
go test ./cmd/flow-mcp/ -run Loopback -v
go build ./...
golangci-lint run ./cmd/flow-mcp/...
```
Expected: resource-lifecycle test + all prior tests PASS; build OK; lint 0.

- [ ] **Step 7: Commit**

```bash
git add cmd/flow-mcp/server.go cmd/flow-mcp/resources.go cmd/flow-mcp/tools_write.go cmd/flow-mcp/main.go cmd/flow-mcp/loopback_write_test.go
git commit -m "feat(flow-mcp): flow://doc resources with boot registration + live write sync" -m "Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Final verification

- [ ] Full gate with the podman env (so unrelated pgstore Docker tests run, not skip):
  ```
  export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"
  export TESTCONTAINERS_RYUK_DISABLED=true
  make ci
  ```
  Expected: golangci-lint 0, coverage ≥ 80% gate, build OK.
- [ ] 9-tool surface intact: `go test ./cmd/flow-mcp/ -v` green (2a context + 2b 5 read tools + 2c 3 write tools = `flow_project_context`, `flow_search_docs`, `flow_list_docs`, `flow_get_doc`, `flow_list_tags`, `flow_backlinks`, `flow_create_doc`, `flow_update_doc`, `flow_delete_doc`). (`flow_bind_project`/`flow_list_projects` remain for 2d → total reaches the spec's 11 there.)
- [ ] **Main-wiring verification** (per `feedback_plan_main_wiring_task`): confirm `cmd/flow-mcp/main.go` builds via `newServerH`, calls `registerResources`, and `newServerH` registers all nine tools + the resource handler path. Stdout-hygiene smoke (no tool/resource added a stdout write):
  ```
  go build -o bin/flow-mcp ./cmd/flow-mcp
  printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}' | ./bin/flow-mcp 2>/dev/null
  ```
  Expected: a single JSON-RPC line on stdout, nothing else. (`bin/flow-mcp` is gitignored — do not commit it.)
- [ ] **Live done-gate** (deferred to the slice's manual dogfood, against the dev stack per `reference_flow_dev_env`): after `flow login`, from a Claude Code session with flow-mcp registered — `flow_create_doc` writes a `memory` doc to this repo's project; `flow_get_doc` reads it back; `flow_update_doc` on it succeeds (agent-owned, no confirm); `flow_update_doc`/`flow_delete_doc` on a human-owned `project`/`free` doc is refused without `confirm` and accepted with; the created doc shows up as a `flow://doc/{id}` resource. (Capable clients only — tools are the reliable path.)

## Self-review notes (spec coverage for 2c)

- Spec §7.2 write tools (`flow_create_doc`/`flow_update_doc`/`flow_delete_doc`) → Task 2. Params match the spec table; create is unguarded and tags via body frontmatter (no tags write field); update is partial via fetch-then-merge; type/project/path immutable on update (only Title/Body sent).
- Spec §6 write guard → Task 1 `guardMutation` (type-agnostic via `domain.HumanOwned()`), enforced in update/delete after the fetch; create unguarded. Asserted by `TestLoopback_UpdateGuard`/`DeleteGuard`/`UpdateAgentOwnedNoConfirm`.
- Spec §5 ownership classes → single-source `domain.DocumentType.HumanOwned()` (positive set {daily,project,free}; future agent types unguarded by default). The four agent-owned type constants already exist in `domain` — **no constant additions, no migration** here.
- Spec §7.1 resources → Task 3: `flow://doc/{id}`, Name=title, Description=`<path> · <type> · <tags>`, MIMEType `text/markdown`, body read **fresh** via `GetDocument`; registered at boot from the resolved project; live `AddResource`/`RemoveResources` on create/delete and refresh on update; skipped when unauthed/unresolved. Asserted by `TestLoopback_Resources_BootAndLiveSync`.
- Spec §4.4 degraded mode → every write handler's `!h.authed` short-circuit (`TestLoopback_WriteTools_DegradedRequireLogin`); `registerResources` no-ops when unauthed/unmatched.
- Spec §4.5 auto-scope on create → `resolveScope` reused; the create-specific `&"none"`→`nil` mapping documented and applied.
- **DEFERRED to 2d (its own just-in-time plan):** `flow_bind_project` + `flow_list_projects` (reusing the 2b project-ref cache), the `docs/runbook/flow-mcp-setup.md` registration runbook, and the live `.mcp.json` registration done-gate (§11). 2d brings the surface to the spec's 11 tools.
