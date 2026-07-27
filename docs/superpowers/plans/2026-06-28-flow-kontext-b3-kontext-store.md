---
type: agent
project: github.com/serverkraken/flow
---
# flow Kontext-Redesign · B3 — Kontext-Store (Kern) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the load/save loop of the Kontext-Store — a read-only **compose** endpoint that walks the B1 node ancestor chain ∪ B2 global tag-matches into a typed, token-budgeted bundle; an **activeContext path-upsert**; and the two Claude Code hooks (`SessionStart` loads, `Stop` conditionally reminds to flush) plus a `flow context` CLI with an offline cache.

**Architecture:** A **pure** `usecase.Compose` function does all ranking/budgeting (deterministic, no DB — the test focus). A thin `usecase.ComposeContext.Execute` does the I/O (B1 `ResolveNode` → `NodeStore.Ancestors` → new `DocumentStore.ListForContext` → B2 `TagStore.FilterIDs`/`TagsForMany`) then calls `Compose`. One new column (`documents.pinned`, migration `0021`) is the only schema change; the activeContext write is a path-upsert on the existing B1 unique index `documents_owner_node_path`. The endpoint returns structured JSON; the `flow context` CLI renders Markdown for the hook; MCP returns the JSON. Hexagonal: domain → ports → usecase → adapters (pgstore/httpserver/apiclient/mcp) → cmd wiring.

**Tech Stack:** Go, Postgres + pgx/v5 + goose migrations (applied on server boot via `pgstore.Migrate`), testcontainers (`pgvector/pgvector:pg16`) for pgstore tests, stdlib `net/http` ServeMux (Go 1.22 method+pattern routing, `r.PathValue`), cobra (CLI), `github.com/modelcontextprotocol/go-sdk/mcp` (MCP). Claude Code hooks via `~/.claude/settings.json`.

**Spec:** `docs/superpowers/specs/2026-06-28-flow-kontext-b3-kontext-store-design.md` (implements §1–§13 + B3-1…B3-11). **Übersicht:** `docs/superpowers/specs/2026-06-27-flow-kontext-redesign-design.md` (D1–D11, esp. D5/D7/D8). **Prereqs landed:** B1 (recursive `nodes`, `NodeStore.Ancestors` leaf→root, `usecase.ResolveNode`, unique index `documents_owner_node_path`, migrations 0015–0018), B2 (`tags`/`taggings`, `TagStore.FilterIDs`/`TagsForMany`, frontmatter abolished, migrations 0019–0020).

## Global Constraints

- Module path `github.com/serverkraken/flow`. Hexagonal — one responsibility per file, no monoliths ([[feedback_no_monoliths]]).
- **Canonical names (obey verbatim):** `domain.Document.Pinned bool` (new field, JSON `pinned`); `usecase.Compose` (pure), `usecase.ComposeContext`, `usecase.ContextItem`, `usecase.ComposedContext`, `usecase.ContextResolution`, `usecase.ContextBudget`, `usecase.DroppedCount`, `usecase.ContextResolveInput`, `usecase.ActiveContextPath = "active-context"`; ports `DocumentStore.ListForContext`/`UpsertByPath`/`SetPinned`; migration `0021_documents_pinned.sql`; REST `GET /api/v1/context`, `PUT /api/v1/context/active`, `POST /api/v1/documents/{id}/pin`; apiclient `ComposeContext`/`SetActiveContext`/`SetPinned`; CLI `flow context` (+ `install-hooks`, `flush-check`); MCP `flow_get_context`, `flow_set_active_context`; env `FLOW_CONTEXT_BUDGET` (default `6000`).
- **New `DocumentStore` methods (PINNED — every slice consumes exactly these signatures):**
  ```go
  // ListForContext returns docs WHERE (node_id = ANY(nodeIDs) [OR node_id IS NULL if includeGlobal])
  //   AND type = ANY(types); Tags hydrated from taggings (B2). nodeIDs may be empty.
  ListForContext(ctx context.Context, ownerID string, nodeIDs []string, includeGlobal bool, types []domain.DocumentType) ([]domain.Document, error)
  // UpsertByPath inserts or updates the doc identified by (ownerID, nodeID, path) — the B1
  //   unique index documents_owner_node_path. On conflict it updates title/body/updated_at and
  //   LEAVES pinned untouched. Returns the row id + updated_at.
  UpsertByPath(ctx context.Context, ownerID string, nodeID *string, typ domain.DocumentType, path, title, body string, pinned bool) (id string, updatedAt time.Time, err error)
  // SetPinned flips the pinned flag on one owned doc.
  SetPinned(ctx context.Context, ownerID, id string, pinned bool) error
  ```
- **`ComposeContext` usecase contract (PINNED):**
  ```go
  type ContextResolveInput struct{ RemoteSlug, MachineID, Cwd, NodeOverride string }
  type ComposeContext struct {
      Resolve ResolveNode      // {Bindings, Nodes} — reused from B1
      Nodes   ports.NodeStore
      Docs    ports.DocumentStore
      Tags    ports.TagStore
  }
  func (uc ComposeContext) Execute(ctx context.Context, ownerID string, in ContextResolveInput, cap int) (ComposedContext, error)
  ```
- **Pure `Compose` contract (PINNED — the test focus):**
  ```go
  // chain is leaf→root (NodeStore.Ancestors order). docs are all gathered instruction+memory
  // docs (chain nodes + global). globalAllowed is the set of global *memory* doc-ids that passed
  // the D7 tag-gate. Compose classifies, ranks (pinned → updated_at desc), budgets, counts dropped.
  func Compose(chain []domain.Node, docs []domain.Document, globalAllowed map[string]bool, cap int) ComposedContext
  ```
- **Tier rules (D5/D8 — encode exactly):** bootstrap types = `domain.DocInstruction` + `domain.DocMemory` only.
  - **Always (uncapped):** every `instruction` (chain nodes ∪ global); the `activeContext` (the `memory` doc whose `node_id == chain[0].ID` AND `path == ActiveContextPath`); every `memory` whose `node_id` is the **leaf** (`chain[0]`) or an **intermediate vorhaben** (a chain node that is neither `chain[0]` nor the engagement root).
  - **Relevance (capped, ranked `pinned` desc then `UpdatedAt` desc):** `memory` whose `node_id` is the **engagement root** (`chain[last]`, only when `last != 0`); `memory` whose `node_id IS NULL` (global) **and** id ∈ `globalAllowed`.
  - Always-tier always counts into `Budget.Used`. Relevance items are added in rank order while `Used+EstTokens ≤ cap`; the remainder increments `Budget.Dropped.{Engagement,Global}`.
- **`EstTokens` heuristic:** `EstTokens(body) = (len(body)+3)/4` (integer ceil of len/4). No tokenizer dependency. `cap` default `6000`, overridable via `FLOW_CONTEXT_BUDGET` (server reads env; CLI `--cap` overrides per call).
- **`global ≠ none` (B3-10):** global = `node_id IS NULL`. `ComposeContext` passes `includeGlobal=true`; never leaks the `"none"` sentinel outward.
- **Kind-agnostic (B3-11):** the resolved leaf may be `repo` **or** `vorhaben` (no-git-upstream path-binding); everything downstream keys on node-id/position, never on `kind`. Unresolved/unbound → `ComposedContext{Resolution:{Unresolved:true}}` + global-only; **never an error**.
- **Migrations:** goose-annotated (`-- +goose Up`/`Down`), embedded via `//go:embed migrations/*.sql`, applied on boot by `pgstore.Migrate` ([[feedback_pgstore_goose_migrations]] — bare SQL fails at apply; only Docker pgstore tests catch it). Highest existing = `0020`; this plan adds **`0021`**. Test a migration via `pgstore.MigrateUpTo(ctx, pool, 20)` (note: `version int64`) → seed → `pgstore.Migrate` → assert.
- **Hooks (verified vs official docs — do not re-derive):** `SessionStart` emits `hookSpecificOutput.additionalContext` (string, injected before first prompt), **cannot block**. `Stop` fires **once per turn** ("when Claude finishes responding"); emitting `hookSpecificOutput.additionalContext` makes "the conversation continue so Claude can act on the feedback"; the hook input JSON carries `stop_hook_active` (loop guard) + `transcript_path`. `SessionEnd` is **cleanup-only** (output never reaches context) — not used for the reminder.
- **Tests:** `package <pkg>_test`, `t.Parallel()`, table-driven where natural; fakes via `testutil.New*`; struct-literal usecase injection; stdlib `errors.Is` + `t.Fatalf`/`t.Errorf` (no assert lib). HTTP: `newDocServer(t)` / `doDoc(t, ts, method, path, body)` / `primeUser(t, ts.URL)` helpers; `FakeVerifier` hardwired identity, `FakeIDGen` deterministic (`id-1`, `id-2`, …), Bearer `x`. Docker Postgres via `startPG(t)` (skips when no container runtime; `DOCKER_HOST` may need the podman socket [[feedback_tailwind_v4_templ_gotchas]]).
- **CI:** `make ci` (gofumpt + staticcheck incl. QF1002, verify-generate/css/no-popups, coverage gate, build) must stay green and the coverage gate must not regress — run `make ci` (lint), not just `go test` ([[project_flow_rebuild_phase2_planned]]).
- **Owner-scoping** on every store/usecase call; foreign ids never leak or mutate. The `flow context` SessionStart hook **must never hard-fail** (offline cache fallback).
- **Subagent hygiene:** verify `git HEAD` advanced after each task ([[feedback_subagent_git_commits_isolated]]); recover orphan commits via reflog.
- Every `git commit` message ends with the trailer `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

## Slice map (execution order; each slice ends `make ci`-green + independently testable)

| Slice | Deliverable |
|---|---|
| **A — Compose core + store + migration** | `Document.Pinned` + migration `0021` · `DocumentStore.{ListForContext,UpsertByPath,SetPinned}` (+ pgstore + fakes) · pure `usecase.Compose` + types + exhaustive table test · `usecase.ComposeContext.Execute` orchestration. → pgstore-Docker gate. |
| **B — REST + apiclient** | `GET /api/v1/context` · `PUT /api/v1/context/active` · `POST /api/v1/documents/{id}/pin` (handlers + routes + httptest) · apiclient `ComposeContext`/`SetActiveContext`/`SetPinned`. → httptest gate. |
| **C — CLI + MCP** | `flow context` (client-side resolve + render markdown/JSON + offline cache) · `flow context install-hooks` · `flow context flush-check` · MCP `flow_get_context` + `flow_set_active_context`. → CLI/MCP test + curl-smoke. |
| **D — Wiring + Einschritt-Write + Done-Gate** | composition-root wiring (`ComposeContext` in `cmd/flow-server/main.go` + `Server` field) · curl-smoke every route · native-auto-memory-off + flow-only-memory convention (documented in `install-hooks` output) · live dogfood incl. real Claude Code hook test · `make ci` + memory/spec status. |

> **Cross-slice reconciliation (Slice A is authoritative — obey over any divergent later "Consumes"):**
> 1. **`ComposedContext.Memories` keys:** exactly `"leaf"`, `"vorhaben"`, `"engagement"`, `"global"` (strings). The spec's prose says "repo" for the leaf group; the code key is **`"leaf"`** because the leaf may be a non-code `vorhaben` (B3-11). `ScopeLabel` carries the human form (`"repo:flow"`, `"vorhaben:Buch"`, `"engagement:Privat"`, `"global"`).
> 2. **activeContext identification:** the `memory` doc with `node_id == chain[0].ID` **and** `path == usecase.ActiveContextPath`. It is pulled OUT of the memories tiers into `ComposedContext.ActiveContext` and never double-counted in `Memories["leaf"]`.
> 3. **pinned write semantics:** `Create` inserts `pinned=false` (default); `Update` never modifies `pinned`; only `SetPinned` and `UpsertByPath`(on insert) set it; `UpsertByPath` on-conflict **preserves** the existing `pinned`. So a flush never clears a pin.
> 4. **Type filter:** `ListForContext` is the only typed multi-node read; the existing `List(ownerID, nodeID, tags...)` is unchanged. Global is via `includeGlobal bool`, never the `"none"` string at the usecase layer.
> 5. **Resolve reuse:** `ComposeContext.Resolve` is the B1 `usecase.ResolveNode{Bindings,Nodes}`; `Execute` calls `Resolve.Execute(ctx, owner, in.RemoteSlug, in.MachineID, in.Cwd)` unless `in.NodeOverride != ""` (then `Nodes`-lookup by slug). `Nodes.Ancestors(ctx, owner, leaf.ID)` gives the chain.

---

## Slice A — Compose core + store + migration

> Pure-logic-first: A1 adds the `pinned` column + plumbing, A2/A3 add the two new store reads/writes (Docker-tested), A4 is the pure `Compose` (the bulk of the tests), A5 wires the orchestrating usecase. Nothing user-facing yet.

### Task A1: `Document.Pinned` + migration `0021` + pgstore plumbing

**Files:**
- Modify: `internal/domain/document.go` (add `Pinned bool` field)
- Create: `internal/adapter/pgstore/migrations/0021_documents_pinned.sql`
- Modify: `internal/adapter/pgstore/documents.go` (`docCols`/`prefixedDocCols` + `scanDocument` + `Create` insert + add `SetPinned`)
- Modify: `internal/testutil/fakes.go` (`FakeDocumentStore`: store `Pinned`, add `SetPinned`)
- Test: `internal/adapter/pgstore/documents_test.go` (append a pinned round-trip test)

**Interfaces:**
- Produces: `domain.Document.Pinned bool`; `DocumentStore.SetPinned(ctx, ownerID, id string, pinned bool) error` (port added in A3's interface edit — here only the pgstore + fake methods; A3 pins the interface). To keep `make ci` green now, add the interface method in this task too (see Step 4).

- [ ] **Step 1: Add the field** — `internal/domain/document.go`, inside `Document` (after `Body string` / near `Tags`):
```go
	Pinned    bool           `json:"pinned"`
```

- [ ] **Step 2: Write the migration** — `internal/adapter/pgstore/migrations/0021_documents_pinned.sql`:
```sql
-- +goose Up
ALTER TABLE documents ADD COLUMN pinned BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE documents DROP COLUMN pinned;
```

- [ ] **Step 3: Write the failing pgstore test** — append to `internal/adapter/pgstore/documents_test.go`:
```go
func TestDocumentStore_SetPinned(t *testing.T) {
	t.Parallel()
	ds, us, done := newDocStore(t) // existing helper in documents_test.go
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")

	d, err := ds.Create(ctx, domain.Document{
		ID: "d1", OwnerID: "u1", Type: domain.DocMemory, Path: "p1", Title: "t", Body: "b",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Pinned {
		t.Fatalf("new doc should default pinned=false")
	}
	if err := ds.SetPinned(ctx, "u1", "d1", true); err != nil {
		t.Fatal(err)
	}
	got, _ := ds.Get(ctx, "u1", "d1")
	if !got.Pinned {
		t.Fatalf("SetPinned(true) not reflected: %+v", got)
	}
}
```
> Mirror the `newDocStore`/`seedUser` helpers already in `documents_test.go`/`tags_test.go`. If `newDocStore` is named differently, use the existing one.

- [ ] **Step 4: Run to verify it fails**

Run: `go test ./internal/adapter/pgstore/ -run TestDocumentStore_SetPinned -v`
Expected: FAIL — `documents.Pinned undefined` / `ds.SetPinned undefined`.

- [ ] **Step 5: Plumb `pinned` through pgstore** — in `internal/adapter/pgstore/documents.go`:
  1. Extend the column constants (`documents.go:24-26`) to include `pinned` as the **last** column:
```go
const docCols = `id, owner_id, node_id, type, path, title, body, doc_date, role, extra, created_at, updated_at, pinned`
const prefixedDocCols = `d.id, d.owner_id, d.node_id, d.type, d.path, d.title, d.body, d.doc_date, d.role, d.extra, d.created_at, d.updated_at, d.pinned`
```
  2. In `scanDocument` (`documents.go:435`), add `&d.Pinned` as the final scan target (same order as `docCols`).
  3. In `Create`'s INSERT, add `pinned` to the column list + `$N` placeholder bound to `d.Pinned` (so a caller-set pin persists; default false otherwise). Keep `RETURNING ` + `docCols`.
  4. Add the method:
```go
func (s *DocumentStore) SetPinned(ctx context.Context, ownerID, id string, pinned bool) error {
	ct, err := s.pool.Exec(ctx, `UPDATE documents SET pinned=$1, updated_at=now() WHERE owner_id=$2 AND id=$3`, pinned, ownerID, id)
	if err != nil {
		return fmt.Errorf("pgstore: set pinned: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ports.ErrDocumentNotFound
	}
	return nil
}
```
  5. Confirm `Update` does **not** touch `pinned` (leave its SET list as-is).

- [ ] **Step 6: Add `SetPinned` to the port + fake** so the build stays green:
  - `internal/ports/ports.go` — inside `DocumentStore`, add: `SetPinned(ctx context.Context, ownerID, id string, pinned bool) error`.
  - `internal/testutil/fakes.go` — `FakeDocumentStore`: ensure stored docs keep `Pinned` (they store `domain.Document`, so `Create` already retains it); add:
```go
func (s *FakeDocumentStore) SetPinned(_ context.Context, ownerID, id string, pinned bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.docs[id]
	if !ok || d.OwnerID != ownerID {
		return ports.ErrDocumentNotFound
	}
	d.Pinned = pinned
	s.docs[id] = d
	return nil
}
```
> Match the fake's actual field names (`s.docs`/`s.mu`). If the fake stores by a composite key, adapt.

- [ ] **Step 7: Run to verify pass**

Run: `go test ./internal/adapter/pgstore/ -run TestDocumentStore_SetPinned -v && go build ./...`
Expected: PASS + clean build.

- [ ] **Step 8: Commit**
```bash
git add internal/domain/document.go internal/adapter/pgstore/migrations/0021_documents_pinned.sql internal/adapter/pgstore/documents.go internal/ports/ports.go internal/testutil/fakes.go internal/adapter/pgstore/documents_test.go
git commit -m "feat(docs): documents.pinned column + SetPinned (B3 A1)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task A2: `DocumentStore.UpsertByPath` (path-upsert)

**Files:**
- Modify: `internal/ports/ports.go` (`DocumentStore.UpsertByPath`)
- Modify: `internal/adapter/pgstore/documents.go` (`UpsertByPath`)
- Modify: `internal/testutil/fakes.go` (`FakeDocumentStore.UpsertByPath`)
- Test: `internal/adapter/pgstore/documents_test.go`

**Interfaces:**
- Produces: `DocumentStore.UpsertByPath(ctx, ownerID, nodeID *string, typ domain.DocumentType, path, title, body string, pinned bool) (id string, updatedAt time.Time, err error)`.
- Consumes: B1 unique index `documents_owner_node_path` on `(owner_id, coalesce(node_id,''), path)`; `ports.IDGen` (for the insert id).

- [ ] **Step 1: Write the failing test** — append to `internal/adapter/pgstore/documents_test.go`:
```go
func TestDocumentStore_UpsertByPath_InsertThenUpdate(t *testing.T) {
	t.Parallel()
	ds, us, done := newDocStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	nid := "n1"

	id1, _, err := ds.UpsertByPath(ctx, "u1", &nid, domain.DocMemory, "active-context", "AC", "v1 body", false)
	if err != nil {
		t.Fatal(err)
	}
	id2, _, err := ds.UpsertByPath(ctx, "u1", &nid, domain.DocMemory, "active-context", "AC", "v2 body", false)
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("upsert must reuse the same row: %q vs %q", id1, id2)
	}
	got, _ := ds.Get(ctx, "u1", id1)
	if got.Body != "v2 body" {
		t.Fatalf("body not updated: %q", got.Body)
	}
}

func TestDocumentStore_UpsertByPath_GlobalNodeNull(t *testing.T) {
	t.Parallel()
	ds, us, done := newDocStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")

	id1, _, err := ds.UpsertByPath(ctx, "u1", nil, domain.DocMemory, "active-context", "G", "g1", false)
	if err != nil {
		t.Fatal(err)
	}
	id2, _, _ := ds.UpsertByPath(ctx, "u1", nil, domain.DocMemory, "active-context", "G", "g2", false)
	if id1 != id2 {
		t.Fatalf("global (node_id NULL) upsert must hit the coalesce('') index, got %q vs %q", id1, id2)
	}
}

func TestDocumentStore_UpsertByPath_PreservesPin(t *testing.T) {
	t.Parallel()
	ds, us, done := newDocStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	nid := "n1"
	id, _, _ := ds.UpsertByPath(ctx, "u1", &nid, domain.DocMemory, "active-context", "AC", "v1", true)
	_, _, _ = ds.UpsertByPath(ctx, "u1", &nid, domain.DocMemory, "active-context", "AC", "v2", false) // flush, pinned arg false
	got, _ := ds.Get(ctx, "u1", id)
	if !got.Pinned {
		t.Fatalf("upsert-on-conflict must PRESERVE the existing pin")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/adapter/pgstore/ -run TestDocumentStore_UpsertByPath -v`
Expected: FAIL — `ds.UpsertByPath undefined`.

- [ ] **Step 3: Implement** — add to `internal/adapter/pgstore/documents.go`:
```go
func (s *DocumentStore) UpsertByPath(ctx context.Context, ownerID string, nodeID *string, typ domain.DocumentType, path, title, body string, pinned bool) (string, time.Time, error) {
	id := s.ids.NewID()
	const q = `
INSERT INTO documents (id, owner_id, node_id, type, path, title, body, extra, pinned, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,'{}',$8,now(),now())
ON CONFLICT (owner_id, coalesce(node_id, ''), path)
DO UPDATE SET title = EXCLUDED.title, body = EXCLUDED.body, updated_at = now()
RETURNING id, updated_at`
	var gotID string
	var updated time.Time
	err := s.pool.QueryRow(ctx, q, id, ownerID, nodeID, string(typ), path, title, body, pinned).Scan(&gotID, &updated)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("pgstore: upsert by path: %w", err)
	}
	return gotID, updated, nil
}
```
> **Note:** this requires `DocumentStore` to hold an `ids ports.IDGen`. If `NewDocumentStore(pool)` does not yet inject one, change it to `NewDocumentStore(pool *pgxpool.Pool, ids ports.IDGen) *DocumentStore` and add the `ids` field — and update the single call site `cmd/flow-server/main.go:74` (`documentStore := pgstore.NewDocumentStore(pool, ids)`) and any test constructors. (The `ids` value `uuidgen.Gen{}` already exists at `main.go`.) Do this in THIS task so the build stays green.

- [ ] **Step 4: Add the port method + fake** — `internal/ports/ports.go` `DocumentStore`:
```go
	UpsertByPath(ctx context.Context, ownerID string, nodeID *string, typ domain.DocumentType, path, title, body string, pinned bool) (id string, updatedAt time.Time, err error)
```
`internal/testutil/fakes.go` — `FakeDocumentStore` (key on `owner|coalesce(node,'')|path`):
```go
func (s *FakeDocumentStore) UpsertByPath(_ context.Context, ownerID string, nodeID *string, typ domain.DocumentType, path, title, body string, pinned bool) (string, time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	nv := ""
	if nodeID != nil {
		nv = *nodeID
	}
	for _, d := range s.docs { // find existing by (owner, node, path)
		dn := ""
		if d.NodeID != nil {
			dn = *d.NodeID
		}
		if d.OwnerID == ownerID && dn == nv && d.Path == path {
			d.Title, d.Body = title, body // preserve pinned, type, id
			s.docs[d.ID] = d
			return d.ID, s.clock(), nil
		}
	}
	id := s.nextID() // mirror the fake's existing id scheme
	s.docs[id] = domain.Document{ID: id, OwnerID: ownerID, NodeID: nodeID, Type: typ, Path: path, Title: title, Body: body, Pinned: pinned}
	return id, s.clock(), nil
}
```
> Adapt `s.nextID()`/`s.clock()` to the fake's real helpers (it already generates ids + timestamps for `Create`). If it has none, return a counter-based id and `time.Time{}`.

- [ ] **Step 5: Run to verify pass**

Run: `go test ./internal/adapter/pgstore/ -run TestDocumentStore_UpsertByPath -v && go build ./...`
Expected: PASS + clean build.

- [ ] **Step 6: Commit**
```bash
git add internal/ports/ports.go internal/adapter/pgstore/documents.go internal/testutil/fakes.go internal/adapter/pgstore/documents_test.go cmd/flow-server/main.go
git commit -m "feat(docs): UpsertByPath path-upsert on documents_owner_node_path (B3 A2)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task A3: `DocumentStore.ListForContext` (typed multi-node read)

**Files:**
- Modify: `internal/ports/ports.go` (`DocumentStore.ListForContext`)
- Modify: `internal/adapter/pgstore/documents.go` (`ListForContext` + reuse the B2 tag-hydration helper)
- Modify: `internal/testutil/fakes.go` (`FakeDocumentStore.ListForContext`)
- Test: `internal/adapter/pgstore/documents_test.go`

**Interfaces:**
- Produces: `DocumentStore.ListForContext(ctx, ownerID string, nodeIDs []string, includeGlobal bool, types []domain.DocumentType) ([]domain.Document, error)` — `Tags` hydrated.
- Consumes: the existing tag-hydration path used by `List` (the `hydrateTags` join on `taggings`).

- [ ] **Step 1: Write the failing test** — append to `internal/adapter/pgstore/documents_test.go`:
```go
func TestDocumentStore_ListForContext(t *testing.T) {
	t.Parallel()
	ds, us, done := newDocStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	leaf, eng := "leafN", "engN"

	mk := func(id, typ string, node *string) {
		if _, err := ds.Create(ctx, domain.Document{ID: id, OwnerID: "u1", NodeID: node, Type: domain.DocumentType(typ), Path: id, Title: id, Body: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	mk("i-leaf", "instruction", &leaf)
	mk("m-leaf", "memory", &leaf)
	mk("m-eng", "memory", &eng)
	mk("i-glob", "instruction", nil)
	mk("daily-leaf", "daily", &leaf) // must be excluded by type filter

	got, err := ds.ListForContext(ctx, "u1", []string{leaf, eng}, true,
		[]domain.DocumentType{domain.DocInstruction, domain.DocMemory})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, d := range got {
		ids[d.ID] = true
	}
	for _, want := range []string{"i-leaf", "m-leaf", "m-eng", "i-glob"} {
		if !ids[want] {
			t.Errorf("ListForContext missing %s; got %v", want, ids)
		}
	}
	if ids["daily-leaf"] {
		t.Errorf("type filter leaked a daily doc")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/adapter/pgstore/ -run TestDocumentStore_ListForContext -v`
Expected: FAIL — `ds.ListForContext undefined`.

- [ ] **Step 3: Implement** — add to `internal/adapter/pgstore/documents.go` (mirror how `List` builds args + hydrates tags):
```go
func (s *DocumentStore) ListForContext(ctx context.Context, ownerID string, nodeIDs []string, includeGlobal bool, types []domain.DocumentType) ([]domain.Document, error) {
	ts := make([]string, len(types))
	for i, t := range types {
		ts[i] = string(t)
	}
	args := []any{ownerID, ts}
	q := `SELECT ` + docCols + ` FROM documents WHERE owner_id=$1 AND type = ANY($2)`
	switch {
	case len(nodeIDs) > 0 && includeGlobal:
		args = append(args, nodeIDs)
		q += ` AND (node_id = ANY($3) OR node_id IS NULL)`
	case len(nodeIDs) > 0:
		args = append(args, nodeIDs)
		q += ` AND node_id = ANY($3)`
	case includeGlobal:
		q += ` AND node_id IS NULL`
	default:
		return nil, nil
	}
	q += ` ORDER BY updated_at DESC`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list for context: %w", err)
	}
	docs, err := collectDocuments(rows) // the existing scan-all helper used by List; if named differently, match it
	if err != nil {
		return nil, err
	}
	return s.hydrateTags(ctx, ownerID, docs) // the existing B2 tag-hydration helper used by List
}
```
> **Note:** reuse the exact scan-collector + `hydrateTags` helper that `List` uses (look at `List` in `documents.go`). If `List` inlines the row loop, factor a small `collectDocuments(rows)` helper or inline the same loop. Do not introduce a second hydration path.

- [ ] **Step 4: Add the port method + fake** — `internal/ports/ports.go` `DocumentStore`:
```go
	ListForContext(ctx context.Context, ownerID string, nodeIDs []string, includeGlobal bool, types []domain.DocumentType) ([]domain.Document, error)
```
`internal/testutil/fakes.go`:
```go
func (s *FakeDocumentStore) ListForContext(_ context.Context, ownerID string, nodeIDs []string, includeGlobal bool, types []domain.DocumentType) ([]domain.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inNodes := map[string]bool{}
	for _, n := range nodeIDs {
		inNodes[n] = true
	}
	inTypes := map[domain.DocumentType]bool{}
	for _, t := range types {
		inTypes[t] = true
	}
	var out []domain.Document
	for _, d := range s.docs {
		if d.OwnerID != ownerID || !inTypes[d.Type] {
			continue
		}
		switch {
		case d.NodeID == nil:
			if includeGlobal {
				out = append(out, d)
			}
		case inNodes[*d.NodeID]:
			out = append(out, d)
		}
	}
	return out, nil
}
```

- [ ] **Step 5: Run to verify pass**

Run: `go test ./internal/adapter/pgstore/ -run TestDocumentStore_ListForContext -v && go build ./...`
Expected: PASS + clean build.

- [ ] **Step 6: Commit**
```bash
git add internal/ports/ports.go internal/adapter/pgstore/documents.go internal/testutil/fakes.go internal/adapter/pgstore/documents_test.go
git commit -m "feat(docs): ListForContext typed multi-node read (chain + global) (B3 A3)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task A4: Pure `usecase.Compose` + types (the test focus)

**Files:**
- Create: `internal/usecase/compose_context.go` (types + pure `Compose`)
- Test: `internal/usecase/compose_context_test.go`

**Interfaces:**
- Produces: `usecase.ContextItem`, `usecase.DroppedCount`, `usecase.ContextResolution`, `usecase.ContextBudget`, `usecase.ComposedContext`, `usecase.ActiveContextPath`, and the pure `usecase.Compose(chain []domain.Node, docs []domain.Document, globalAllowed map[string]bool, cap int) ComposedContext`.
- Consumes: `domain.Node`, `domain.Document`, `domain.DocMemory`, `domain.DocInstruction`, `domain.KindEngagement`.

- [ ] **Step 1: Write the failing test** — `internal/usecase/compose_context_test.go`:
```go
package usecase_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

func node(id, name string, k domain.NodeKind) domain.Node {
	return domain.Node{ID: id, Name: name, Slug: name, Kind: k}
}
func doc(id string, node *string, typ domain.DocumentType, path string, pinned bool, updated time.Time, body string) domain.Document {
	return domain.Document{ID: id, NodeID: node, Type: typ, Path: path, Pinned: pinned, UpdatedAt: updated, Body: body}
}

func TestCompose_TiersAndActiveContext(t *testing.T) {
	t.Parallel()
	leaf, eng := "L", "E"
	chain := []domain.Node{node(leaf, "flow", domain.KindRepo), node(eng, "Privat", domain.KindEngagement)}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	docs := []domain.Document{
		doc("i1", &leaf, domain.DocInstruction, "claude", false, t0, "rules"),
		doc("i0", nil, domain.DocInstruction, "claude", false, t0, "global rules"),
		doc("ac", &leaf, domain.DocMemory, usecase.ActiveContextPath, false, t0, "where I was"),
		doc("ml", &leaf, domain.DocMemory, "m-leaf", false, t0, "leaf mem"),
		doc("me", &eng, domain.DocMemory, "m-eng", false, t0, "eng mem"),
	}
	got := usecase.Compose(chain, docs, map[string]bool{}, 100000)

	if len(got.Instructions) != 2 {
		t.Errorf("want 2 instructions (chain+global), got %d", len(got.Instructions))
	}
	if got.ActiveContext == nil || got.ActiveContext.ID != "ac" {
		t.Fatalf("activeContext not extracted: %+v", got.ActiveContext)
	}
	if len(got.Memories["leaf"]) != 1 || got.Memories["leaf"][0].ID != "ml" {
		t.Errorf("leaf memory tier wrong: %+v", got.Memories["leaf"])
	}
	if len(got.Memories["engagement"]) != 1 || got.Memories["engagement"][0].ID != "me" {
		t.Errorf("engagement memory tier wrong: %+v", got.Memories["engagement"])
	}
	// activeContext must NOT also appear in Memories["leaf"].
	for _, m := range got.Memories["leaf"] {
		if m.ID == "ac" {
			t.Errorf("activeContext double-counted in leaf memories")
		}
	}
}

func TestCompose_BudgetDropsRelevanceByRank(t *testing.T) {
	t.Parallel()
	leaf, eng := "L", "E"
	chain := []domain.Node{node(leaf, "flow", domain.KindRepo), node(eng, "Privat", domain.KindEngagement)}
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mid := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	body := func(n int) string { return string(make([]byte, n)) } // n bytes → EstTokens = ceil(n/4)
	docs := []domain.Document{
		// three engagement memories; each EstTokens=100 (400 bytes). cap=250 → only 2 fit.
		doc("pinnedOld", &eng, domain.DocMemory, "a", true, old, body(400)),
		doc("freshUnpinned", &eng, domain.DocMemory, "b", false, mid, body(400)),
		doc("olderUnpinned", &eng, domain.DocMemory, "c", false, old, body(400)),
	}
	got := usecase.Compose(chain, docs, map[string]bool{}, 250)
	kept := got.Memories["engagement"]
	if len(kept) != 2 {
		t.Fatalf("cap=250 with 3×100-token items should keep 2, got %d", len(kept))
	}
	// rank: pinned first, then newer. So pinnedOld + freshUnpinned kept; olderUnpinned dropped.
	if kept[0].ID != "pinnedOld" || kept[1].ID != "freshUnpinned" {
		t.Errorf("rank wrong: %s,%s", kept[0].ID, kept[1].ID)
	}
	if got.Budget.Dropped.Engagement != 1 {
		t.Errorf("want 1 dropped engagement, got %d", got.Budget.Dropped.Engagement)
	}
}

func TestCompose_GlobalGatedByTag(t *testing.T) {
	t.Parallel()
	leaf, eng := "L", "E"
	chain := []domain.Node{node(leaf, "flow", domain.KindRepo), node(eng, "Privat", domain.KindEngagement)}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	docs := []domain.Document{
		doc("gAllowed", nil, domain.DocMemory, "g1", false, t0, "x"),
		doc("gBlocked", nil, domain.DocMemory, "g2", false, t0, "y"),
	}
	got := usecase.Compose(chain, docs, map[string]bool{"gAllowed": true}, 100000)
	if len(got.Memories["global"]) != 1 || got.Memories["global"][0].ID != "gAllowed" {
		t.Fatalf("only tag-allowed global memory should pass: %+v", got.Memories["global"])
	}
}

func TestCompose_UnresolvedNotHandledHere(t *testing.T) {
	t.Parallel()
	// Compose with an empty chain treats everything as global candidates only.
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	docs := []domain.Document{doc("g", nil, domain.DocMemory, "g", false, t0, "x")}
	got := usecase.Compose(nil, docs, map[string]bool{"g": true}, 100000)
	if len(got.Memories["global"]) != 1 {
		t.Fatalf("empty chain should still surface gated global memories")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/usecase/ -run TestCompose -v`
Expected: FAIL — `undefined: usecase.Compose`.

- [ ] **Step 3: Implement** — `internal/usecase/compose_context.go`:
```go
package usecase

import (
	"sort"

	"github.com/serverkraken/flow/internal/domain"
)

// ActiveContextPath is the fixed path of the per-leaf activeContext memory doc.
const ActiveContextPath = "active-context"

type ContextItem struct {
	ID         string              `json:"id"`
	NodeID     *string             `json:"nodeId"`
	ScopeLabel string              `json:"scope"`
	Type       domain.DocumentType `json:"type"`
	Tags       []string            `json:"tags,omitempty"`
	UpdatedAt  string              `json:"updatedAt"`
	Pinned     bool                `json:"pinned"`
	EstTokens  int                 `json:"estTokens"`
	Body       string              `json:"body"`
}

type DroppedCount struct {
	Engagement int `json:"engagement"`
	Global     int `json:"global"`
}

type ContextResolution struct {
	Repo       *domain.Node  `json:"repo"`
	Chain      []domain.Node `json:"chain"`
	Unresolved bool          `json:"unresolved"`
}

type ContextBudget struct {
	Used    int          `json:"used"`
	Cap     int          `json:"cap"`
	Dropped DroppedCount `json:"dropped"`
}

type ComposedContext struct {
	Resolution    ContextResolution        `json:"resolution"`
	Instructions  []ContextItem            `json:"instructions"`
	ActiveContext *ContextItem             `json:"activeContext"`
	Memories      map[string][]ContextItem `json:"memories"`
	Budget        ContextBudget            `json:"budget"`
}

func estTokens(body string) int { return (len(body) + 3) / 4 }

func itemOf(d domain.Document, label string) ContextItem {
	return ContextItem{
		ID: d.ID, NodeID: d.NodeID, ScopeLabel: label, Type: d.Type, Tags: d.Tags,
		UpdatedAt: d.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"), Pinned: d.Pinned,
		EstTokens: estTokens(d.Body), Body: d.Body,
	}
}

// Compose classifies docs into tiers, ranks the relevance tier (pinned → newest),
// and fills until the token cap, counting dropped relevance items. Pure: no I/O.
func Compose(chain []domain.Node, docs []domain.Document, globalAllowed map[string]bool, cap int) ComposedContext {
	out := ComposedContext{Memories: map[string][]ContextItem{}}
	out.Budget.Cap = cap
	if len(chain) > 0 {
		repo := chain[0]
		out.Resolution.Repo = &repo
		out.Resolution.Chain = chain
	} else {
		out.Resolution.Unresolved = true
	}

	// node-id → scope label + tier classification from the chain.
	label := map[string]string{}
	tier := map[string]string{} // "leaf" | "vorhaben" | "engagement"
	for i, n := range chain {
		label[n.ID] = string(n.Kind) + ":" + n.Name
		switch {
		case i == 0:
			tier[n.ID] = "leaf"
		case i == len(chain)-1 && n.Kind == domain.KindEngagement:
			tier[n.ID] = "engagement"
		default:
			tier[n.ID] = "vorhaben"
		}
	}

	type ranked struct {
		item   ContextItem
		group  string
		pinned bool
		upd    string
	}
	var relevance []ranked

	for _, d := range docs {
		switch d.Type {
		case domain.DocInstruction:
			lbl := "global"
			if d.NodeID != nil {
				lbl = label[*d.NodeID]
			}
			out.Instructions = append(out.Instructions, itemOf(d, lbl))
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
				if d.Path == ActiveContextPath {
					it := itemOf(d, label[nid])
					out.ActiveContext = &it
				} else {
					out.Memories["leaf"] = append(out.Memories["leaf"], itemOf(d, label[nid]))
				}
			case "vorhaben":
				out.Memories["vorhaben"] = append(out.Memories["vorhaben"], itemOf(d, label[nid]))
			case "engagement":
				it := itemOf(d, label[nid])
				relevance = append(relevance, ranked{it, "engagement", d.Pinned, it.UpdatedAt})
			}
		}
	}

	// Always-tier into Used.
	for _, it := range out.Instructions {
		out.Budget.Used += it.EstTokens
	}
	if out.ActiveContext != nil {
		out.Budget.Used += out.ActiveContext.EstTokens
	}
	for _, g := range []string{"leaf", "vorhaben"} {
		for _, it := range out.Memories[g] {
			out.Budget.Used += it.EstTokens
		}
	}

	// Rank relevance: pinned first, then newest (UpdatedAt RFC3339 sorts lexicographically).
	sort.SliceStable(relevance, func(i, j int) bool {
		if relevance[i].pinned != relevance[j].pinned {
			return relevance[i].pinned
		}
		return relevance[i].upd > relevance[j].upd
	})
	for _, r := range relevance {
		if out.Budget.Used+r.item.EstTokens <= cap {
			out.Budget.Used += r.item.EstTokens
			out.Memories[r.group] = append(out.Memories[r.group], r.item)
		} else if r.group == "engagement" {
			out.Budget.Dropped.Engagement++
		} else {
			out.Budget.Dropped.Global++
		}
	}
	return out
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/usecase/ -run TestCompose -v`
Expected: PASS (all four tests).

- [ ] **Step 5: Commit**
```bash
git add internal/usecase/compose_context.go internal/usecase/compose_context_test.go
git commit -m "feat(usecase): pure Compose — tiers, pin>recency rank, budget+dropped (B3 A4)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task A5: `ComposeContext.Execute` orchestration

**Files:**
- Modify: `internal/usecase/compose_context.go` (add the `ComposeContext` struct + `Execute`)
- Test: `internal/usecase/compose_context_exec_test.go`

**Interfaces:**
- Produces: `usecase.ContextResolveInput`, `usecase.ComposeContext{Resolve,Nodes,Docs,Tags}`, `(uc ComposeContext) Execute(ctx, ownerID, in ContextResolveInput, cap int) (ComposedContext, error)`.
- Consumes: `usecase.ResolveNode` (B1), `ports.NodeStore.Ancestors`, `ports.DocumentStore.ListForContext`, `ports.TagStore.FilterIDs`/`TagsForMany`, the pure `Compose` (A4).

- [ ] **Step 1: Write the failing test** — `internal/usecase/compose_context_exec_test.go`:
```go
package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestComposeContext_Execute_ResolvesChainAndGates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	binds := testutil.NewFakeProjectBindingStore()

	// hierarchy: engagement E ← repo L; bind remote slug "flow" → L.
	eng, _ := nodes.Create(ctx, domain.Node{ID: "E", OwnerID: "u1", Kind: domain.KindEngagement, Name: "Privat", Slug: "privat"})
	leaf, _ := nodes.Create(ctx, domain.Node{ID: "L", OwnerID: "u1", Kind: domain.KindRepo, Name: "flow", Slug: "flow", ParentID: &eng.ID, OriginSlug: "flow"})
	_ = binds.BindRemote(ctx, "u1", "flow", leaf.ID) // mirror the fake's real bind method

	t0 := time.Now()
	_, _ = docs.Create(ctx, domain.Document{ID: "ac", OwnerID: "u1", NodeID: &leaf.ID, Type: domain.DocMemory, Path: usecase.ActiveContextPath, Body: "where", UpdatedAt: t0})
	_, _ = docs.Create(ctx, domain.Document{ID: "gmem", OwnerID: "u1", NodeID: nil, Type: domain.DocMemory, Path: "g", Body: "global", UpdatedAt: t0})
	// tag both the leaf node and the global memory with "go" so the D7 gate lets gmem cross.
	_, _ = tags.SetTags(ctx, "u1", domain.TaggableNode, leaf.ID, []string{"go"})
	_, _ = tags.SetTags(ctx, "u1", domain.TaggableDocument, "gmem", []string{"go"})

	uc := usecase.ComposeContext{
		Resolve: usecase.ResolveNode{Bindings: binds, Nodes: nodes},
		Nodes:   nodes, Docs: docs, Tags: tags,
	}
	got, err := uc.Execute(ctx, "u1", usecase.ContextResolveInput{RemoteSlug: "flow"}, 100000)
	if err != nil {
		t.Fatal(err)
	}
	if got.Resolution.Unresolved {
		t.Fatalf("should resolve via remote binding")
	}
	if got.ActiveContext == nil || got.ActiveContext.ID != "ac" {
		t.Errorf("activeContext missing: %+v", got.ActiveContext)
	}
	if len(got.Memories["global"]) != 1 || got.Memories["global"][0].ID != "gmem" {
		t.Errorf("D7 tag-gate should admit gmem: %+v", got.Memories["global"])
	}
}

func TestComposeContext_Execute_UnresolvedGivesGlobalOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	binds := testutil.NewFakeProjectBindingStore()
	_, _ = docs.Create(ctx, domain.Document{ID: "gi", OwnerID: "u1", NodeID: nil, Type: domain.DocInstruction, Path: "claude", Body: "rule"})

	uc := usecase.ComposeContext{Resolve: usecase.ResolveNode{Bindings: binds, Nodes: nodes}, Nodes: nodes, Docs: docs, Tags: tags}
	got, err := uc.Execute(ctx, "u1", usecase.ContextResolveInput{RemoteSlug: "unknown", Cwd: "/tmp/x"}, 100000)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Resolution.Unresolved {
		t.Fatalf("unknown repo must be Unresolved")
	}
	if len(got.Instructions) != 1 {
		t.Errorf("global instruction should still load when unresolved: %+v", got.Instructions)
	}
}
```
> **Note:** match the real fake constructors/method names (`NewFakeNodeStore`, `NewFakeProjectBindingStore`, and the bind method). If `FakeNodeStore` lacks `Ancestors` (it was added in B1), confirm it exists; if not, add a recursive-parent walk to the fake in this task.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/usecase/ -run TestComposeContext_Execute -v`
Expected: FAIL — `unknown field Resolve` / `Execute undefined`.

- [ ] **Step 3: Implement** — add to `internal/usecase/compose_context.go`:
```go
type ContextResolveInput struct {
	RemoteSlug   string
	MachineID    string
	Cwd          string
	NodeOverride string // explicit node slug; bypasses binding resolution
}

type ComposeContext struct {
	Resolve ResolveNode
	Nodes   ports.NodeStore
	Docs    ports.DocumentStore
	Tags    ports.TagStore
}

var bootstrapTypes = []domain.DocumentType{domain.DocInstruction, domain.DocMemory}

func (uc ComposeContext) Execute(ctx context.Context, ownerID string, in ContextResolveInput, cap int) (ComposedContext, error) {
	leaf, ok, err := uc.resolveLeaf(ctx, ownerID, in)
	if err != nil {
		return ComposedContext{}, err
	}
	if !ok {
		// Unresolved: global instructions + tag-less global memories never cross (no active tags).
		docs, err := uc.Docs.ListForContext(ctx, ownerID, nil, true, bootstrapTypes)
		if err != nil {
			return ComposedContext{}, err
		}
		return Compose(nil, docs, map[string]bool{}, cap), nil
	}

	chain, err := uc.Nodes.Ancestors(ctx, ownerID, leaf.ID)
	if err != nil {
		return ComposedContext{}, err
	}
	chainIDs := make([]string, len(chain))
	for i, n := range chain {
		chainIDs[i] = n.ID
	}
	docs, err := uc.Docs.ListForContext(ctx, ownerID, chainIDs, true, bootstrapTypes)
	if err != nil {
		return ComposedContext{}, err
	}

	// D7 tag-gate: global memories cross only if they carry one of the chain's node tags.
	allowed, err := uc.globalAllowed(ctx, ownerID, chainIDs)
	if err != nil {
		return ComposedContext{}, err
	}
	return Compose(chain, docs, allowed, cap), nil
}

func (uc ComposeContext) resolveLeaf(ctx context.Context, ownerID string, in ContextResolveInput) (domain.Node, bool, error) {
	if in.NodeOverride != "" {
		all, err := uc.Nodes.List(ctx, ownerID)
		if err != nil {
			return domain.Node{}, false, err
		}
		for _, n := range all {
			if n.Slug == in.NodeOverride {
				return n, true, nil
			}
		}
		return domain.Node{}, false, nil
	}
	return uc.Resolve.Execute(ctx, ownerID, in.RemoteSlug, in.MachineID, in.Cwd)
}

func (uc ComposeContext) globalAllowed(ctx context.Context, ownerID string, chainIDs []string) (map[string]bool, error) {
	tagsByNode, err := uc.Tags.TagsForMany(ctx, ownerID, domain.TaggableNode, chainIDs)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var slugs []string
	for _, ts := range tagsByNode {
		for _, t := range ts {
			if !seen[t.Slug] {
				seen[t.Slug] = true
				slugs = append(slugs, t.Slug)
			}
		}
	}
	allowed := map[string]bool{}
	if len(slugs) == 0 {
		return allowed, nil
	}
	ids, err := uc.Tags.FilterIDs(ctx, ownerID, domain.TaggableDocument, slugs, domain.TagMatchAny)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		allowed[id] = true
	}
	return allowed, nil
}
```
> Add the `import` of `context` + `ports` to the file's import block.

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/usecase/ -run 'TestCompose' -v`
Expected: PASS (pure tests + the two Execute tests).

- [ ] **Step 5: Commit**
```bash
git add internal/usecase/compose_context.go internal/usecase/compose_context_exec_test.go
git commit -m "feat(usecase): ComposeContext.Execute — resolve→ancestors→gather→D7-gate→Compose (B3 A5)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

- [ ] **Step 6: Slice-A gate** — `make ci` green.
```bash
make ci
```
Expected: PASS, coverage gate held (the pure `Compose` table tests carry the coverage).

---

## Slice B — REST + apiclient

> Three flat routes. The `Server` is a plain struct literal (no constructor), so each task adds a field + a handler + a route + an httptest. `newDocServer(t)` must learn to wire the new usecases with fakes (do it once in B1, reuse in B2/B3).

### Task B1: `GET /api/v1/context` handler

**Files:**
- Modify: `internal/adapter/httpserver/server.go` (`Server.ComposeContext usecase.ComposeContext` + `Server.ContextBudget int` fields; route)
- Create: `internal/adapter/httpserver/context.go` (`handleGetContext`)
- Modify: `internal/adapter/httpserver/documents_test.go` (`newDocServer` wires `ComposeContext` + `ContextBudget`)
- Test: `internal/adapter/httpserver/context_test.go`

**Interfaces:**
- Consumes: `usecase.ComposeContext` (A5), `usecase.ContextResolveInput`, `userFrom`, `writeJSON`.
- Produces: `GET /api/v1/context` → `200` with `usecase.ComposedContext` JSON.

- [ ] **Step 1: Add the `Server` fields** — `internal/adapter/httpserver/server.go` (in the struct):
```go
	ComposeContext usecase.ComposeContext
	ContextBudget  int // default cap when ?cap= absent; 0 → fall back to 6000
```

- [ ] **Step 2: Write the failing test** — `internal/adapter/httpserver/context_test.go`:
```go
package httpserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/usecase"
)

func TestHandleGetContext_UnresolvedReturns200Global(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	primeUser(t, ts.URL)

	res := doDoc(t, ts, "GET", "/api/v1/context?remote=does-not-exist", "")
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("unresolved context must be 200, got %d", res.StatusCode)
	}
	var cc usecase.ComposedContext
	if err := json.NewDecoder(res.Body).Decode(&cc); err != nil {
		t.Fatal(err)
	}
	if !cc.Resolution.Unresolved {
		t.Errorf("want Unresolved=true for unknown repo")
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./internal/adapter/httpserver/ -run TestHandleGetContext -v`
Expected: FAIL — route 404 / `ComposeContext` zero-value panics or `newDocServer` doesn't wire it.

- [ ] **Step 4: Wire `newDocServer`** — in `documents_test.go`'s `newDocServer`, build the fakes once and set:
```go
	nodes := testutil.NewFakeNodeStore()
	binds := testutil.NewFakeProjectBindingStore()
	// docs, tags already built in the helper
	srv := &httpserver.Server{
		// ...existing fields...
		ComposeContext: usecase.ComposeContext{
			Resolve: usecase.ResolveNode{Bindings: binds, Nodes: nodes},
			Nodes:   nodes, Docs: docs, Tags: tags,
		},
		ContextBudget: 6000,
	}
```

- [ ] **Step 5: Write the handler** — `internal/adapter/httpserver/context.go`:
```go
package httpserver

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/serverkraken/flow/internal/usecase"
)

func (s *Server) handleGetContext(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	q := r.URL.Query()
	in := usecase.ContextResolveInput{
		RemoteSlug:   strings.TrimSpace(q.Get("remote")),
		MachineID:    strings.TrimSpace(q.Get("machine")),
		Cwd:          strings.TrimSpace(q.Get("path")),
		NodeOverride: strings.TrimSpace(q.Get("node")),
	}
	budget := s.ContextBudget
	if budget <= 0 {
		budget = 6000
	}
	if v := strings.TrimSpace(q.Get("cap")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			budget = n
		}
	}
	cc, err := s.ComposeContext.Execute(r.Context(), u.ID, in, budget)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, cc)
}
```

- [ ] **Step 6: Register the route** — `server.go` `Routes()` (near the documents block):
```go
	mux.Handle("GET /api/v1/context", s.auth(http.HandlerFunc(s.handleGetContext)))
```

- [ ] **Step 7: Run to verify pass**

Run: `go test ./internal/adapter/httpserver/ -run TestHandleGetContext -v && go build ./...`
Expected: PASS + clean build.

- [ ] **Step 8: Commit**
```bash
git add internal/adapter/httpserver/server.go internal/adapter/httpserver/context.go internal/adapter/httpserver/context_test.go internal/adapter/httpserver/documents_test.go
git commit -m "feat(httpserver): GET /api/v1/context compose endpoint (B3 B1)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task B2: `PUT /api/v1/context/active` + `SetActiveContext` usecase

**Files:**
- Modify: `internal/usecase/compose_context.go` (add `SetActiveContext` usecase + `ErrContextUnresolved`)
- Modify: `internal/adapter/httpserver/server.go` (`Server.SetActiveContext` field + route)
- Modify: `internal/adapter/httpserver/context.go` (`handlePutContextActive`)
- Modify: `internal/adapter/httpserver/documents_test.go` (`newDocServer` wires `SetActiveContext`)
- Test: `internal/usecase/set_active_context_test.go`, `internal/adapter/httpserver/context_test.go`

**Interfaces:**
- Produces: `usecase.SetActiveContext{Resolve,Nodes,Docs,Tags}`, `(uc SetActiveContext) Execute(ctx, ownerID string, in ContextResolveInput, title, body string, tags []string) (id string, updatedAt time.Time, err error)`, `usecase.ErrContextUnresolved`; `PUT /api/v1/context/active`.

- [ ] **Step 1: Write the failing usecase test** — `internal/usecase/set_active_context_test.go`:
```go
package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestSetActiveContext_UpsertsAtLeaf(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	binds := testutil.NewFakeProjectBindingStore()
	eng, _ := nodes.Create(ctx, domain.Node{ID: "E", OwnerID: "u1", Kind: domain.KindEngagement, Name: "Privat", Slug: "privat"})
	leaf, _ := nodes.Create(ctx, domain.Node{ID: "L", OwnerID: "u1", Kind: domain.KindRepo, Name: "flow", Slug: "flow", ParentID: &eng.ID, OriginSlug: "flow"})
	_ = binds.BindRemote(ctx, "u1", "flow", leaf.ID)

	uc := usecase.SetActiveContext{Resolve: usecase.ResolveNode{Bindings: binds, Nodes: nodes}, Nodes: nodes, Docs: docs, Tags: tags}
	id, _, err := uc.Execute(ctx, "u1", usecase.ContextResolveInput{RemoteSlug: "flow"}, "", "where I was", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := docs.Get(ctx, "u1", id)
	if got.Path != usecase.ActiveContextPath || got.NodeID == nil || *got.NodeID != "L" || got.Type != domain.DocMemory {
		t.Fatalf("activeContext not written at leaf as memory: %+v", got)
	}
	// idempotent: a second flush reuses the same row.
	id2, _, _ := uc.Execute(ctx, "u1", usecase.ContextResolveInput{RemoteSlug: "flow"}, "", "v2", nil)
	if id2 != id {
		t.Fatalf("flush must reuse the row: %q vs %q", id, id2)
	}
}

func TestSetActiveContext_UnresolvedErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	uc := usecase.SetActiveContext{Resolve: usecase.ResolveNode{Bindings: testutil.NewFakeProjectBindingStore(), Nodes: nodes}, Nodes: nodes, Docs: testutil.NewFakeDocumentStore(), Tags: testutil.NewFakeTagStore()}
	_, _, err := uc.Execute(ctx, "u1", usecase.ContextResolveInput{RemoteSlug: "nope", Cwd: "/x"}, "", "body", nil)
	if !errors.Is(err, usecase.ErrContextUnresolved) {
		t.Fatalf("want ErrContextUnresolved, got %v", err)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/usecase/ -run TestSetActiveContext -v`
Expected: FAIL — `undefined: usecase.SetActiveContext`.

- [ ] **Step 3: Implement** — add to `internal/usecase/compose_context.go`:
```go
// ErrContextUnresolved means the cwd/remote did not resolve to a bound node, so there
// is nowhere to write the activeContext (the human must `flow node bind` first).
var ErrContextUnresolved = errors.New("usecase: context not resolved (bind the repo first)")

type SetActiveContext struct {
	Resolve ResolveNode
	Nodes   ports.NodeStore
	Docs    ports.DocumentStore
	Tags    ports.TagStore
}

func (uc SetActiveContext) Execute(ctx context.Context, ownerID string, in ContextResolveInput, title, body string, tags []string) (string, time.Time, error) {
	var leaf domain.Node
	var ok bool
	var err error
	if in.NodeOverride != "" {
		all, e := uc.Nodes.List(ctx, ownerID)
		if e != nil {
			return "", time.Time{}, e
		}
		for _, n := range all {
			if n.Slug == in.NodeOverride {
				leaf, ok = n, true
				break
			}
		}
	} else {
		leaf, ok, err = uc.Resolve.Execute(ctx, ownerID, in.RemoteSlug, in.MachineID, in.Cwd)
	}
	if err != nil {
		return "", time.Time{}, err
	}
	if !ok {
		return "", time.Time{}, ErrContextUnresolved
	}
	if strings.TrimSpace(title) == "" {
		title = "Active Context"
	}
	id, updated, err := uc.Docs.UpsertByPath(ctx, ownerID, &leaf.ID, domain.DocMemory, ActiveContextPath, title, body, false)
	if err != nil {
		return "", time.Time{}, err
	}
	if tags != nil {
		// tag write after the entity write; a tag failure must not orphan the upsert.
		_, _ = uc.Tags.SetTags(ctx, ownerID, domain.TaggableDocument, id, tags)
	}
	return id, updated, nil
}
```
> Add `"errors"`, `"strings"`, `"time"` to the file's imports if not present.

- [ ] **Step 4: Add the handler + field + route + test-wiring:**
  - `server.go` struct: `SetActiveContext usecase.SetActiveContext`.
  - `server.go` `Routes()`: `mux.Handle("PUT /api/v1/context/active", s.auth(http.HandlerFunc(s.handlePutContextActive)))`.
  - `context.go`:
```go
type putActiveReq struct {
	Remote  string   `json:"remote"`
	Machine string   `json:"machine"`
	Path    string   `json:"path"`
	Node    string   `json:"node"`
	Title   string   `json:"title"`
	Body    string   `json:"body"`
	Tags    []string `json:"tags"`
}

func (s *Server) handlePutContextActive(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req putActiveReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	id, updated, err := s.SetActiveContext.Execute(r.Context(), u.ID,
		usecase.ContextResolveInput{RemoteSlug: req.Remote, MachineID: req.Machine, Cwd: req.Path, NodeOverride: req.Node},
		req.Title, req.Body, req.Tags)
	switch {
	case errors.Is(err, usecase.ErrContextUnresolved):
		http.Error(w, "repo not bound", http.StatusConflict)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "updatedAt": updated})
	}
}
```
> Add `"encoding/json"`, `"errors"` to `context.go` imports. Wire `SetActiveContext` in `newDocServer` (same fakes as B1).
  - `context_test.go` — add a happy-path PUT test asserting `200` + a follow-up `GET /api/v1/context` shows the activeContext (resolve a bound node via a seeded binding/node in `newDocServer`, or assert the 409 path with an unbound remote).

- [ ] **Step 5: Run to verify pass**

Run: `go test ./internal/usecase/ ./internal/adapter/httpserver/ -run 'SetActiveContext|Context' -v && go build ./...`
Expected: PASS + clean build.

- [ ] **Step 6: Commit**
```bash
git add internal/usecase/compose_context.go internal/usecase/set_active_context_test.go internal/adapter/httpserver/server.go internal/adapter/httpserver/context.go internal/adapter/httpserver/context_test.go internal/adapter/httpserver/documents_test.go
git commit -m "feat(httpserver): PUT /context/active + SetActiveContext usecase (B3 B2)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task B3: `POST /api/v1/documents/{id}/pin` + `SetPinned` usecase

**Files:**
- Create: `internal/usecase/set_pinned.go` + test
- Modify: `internal/adapter/httpserver/server.go` (field + route), `internal/adapter/httpserver/documents.go` (handler) , `documents_test.go` (wire + test)

**Interfaces:**
- Produces: `usecase.SetPinned{Docs ports.DocumentStore}`, `(uc SetPinned) Execute(ctx, ownerID, id string, pinned bool) error`; `POST /api/v1/documents/{id}/pin`.

- [ ] **Step 1: Write the failing usecase test** — `internal/usecase/set_pinned_test.go`:
```go
package usecase_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestSetPinned(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	_, _ = docs.Create(ctx, domain.Document{ID: "d1", OwnerID: "u1", Type: domain.DocMemory, Path: "p"})
	uc := usecase.SetPinned{Docs: docs}
	if err := uc.Execute(ctx, "u1", "d1", true); err != nil {
		t.Fatal(err)
	}
	got, _ := docs.Get(ctx, "u1", "d1")
	if !got.Pinned {
		t.Fatalf("pin not set")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `go test ./internal/usecase/ -run TestSetPinned -v` → FAIL (`undefined: usecase.SetPinned`).

- [ ] **Step 3: Implement** — `internal/usecase/set_pinned.go`:
```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/ports"
)

type SetPinned struct{ Docs ports.DocumentStore }

func (uc SetPinned) Execute(ctx context.Context, ownerID, id string, pinned bool) error {
	return uc.Docs.SetPinned(ctx, ownerID, id, pinned)
}
```

- [ ] **Step 4: Handler + field + route + wiring:**
  - `server.go`: field `SetPinned usecase.SetPinned`; route `mux.Handle("POST /api/v1/documents/{id}/pin", s.auth(http.HandlerFunc(s.handlePinDocument)))`.
  - `documents.go`:
```go
type pinReq struct {
	Pinned bool `json:"pinned"`
}

func (s *Server) handlePinDocument(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req pinReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	switch err := s.SetPinned.Execute(r.Context(), u.ID, r.PathValue("id"), req.Pinned); {
	case errors.Is(err, ports.ErrDocumentNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
```
  - `documents_test.go`: wire `SetPinned: usecase.SetPinned{Docs: docs}` in `newDocServer`; add a 204 happy-path test + a 404 unknown-id test.

- [ ] **Step 5: Run to verify pass** — `go test ./internal/usecase/ ./internal/adapter/httpserver/ -run 'SetPinned|PinDocument' -v && go build ./...` → PASS.

- [ ] **Step 6: Commit**
```bash
git add internal/usecase/set_pinned.go internal/usecase/set_pinned_test.go internal/adapter/httpserver/server.go internal/adapter/httpserver/documents.go internal/adapter/httpserver/documents_test.go
git commit -m "feat(httpserver): POST /documents/{id}/pin + SetPinned usecase (B3 B3)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task B4: apiclient — `ComposeContext` / `SetActiveContext` / `SetPinned`

**Files:**
- Create: `internal/adapter/apiclient/context.go` + test
- (apiclient may import `usecase` for the `ComposedContext` read-model — an inward adapter→core dependency, which is correct; no cycle since `usecase` never imports `apiclient`.)

**Interfaces:**
- Produces: `(c *Client) ComposeContext(ctx, in ContextQuery) (usecase.ComposedContext, error)`, `(c *Client) SetActiveContext(ctx, in SetActiveContextInput) (SetActiveContextResult, error)`, `(c *Client) SetPinned(ctx, id string, pinned bool) error`, input types `ContextQuery`/`SetActiveContextInput`.

- [ ] **Step 1: Write the failing test** — `internal/adapter/apiclient/context_test.go` (spin a stub `httptest` server returning canned JSON; mirror existing apiclient tests):
```go
package apiclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

func TestClient_ComposeContext(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/context" || r.URL.Query().Get("remote") != "flow" {
			t.Errorf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"resolution":{"unresolved":false},"instructions":[],"memories":{},"budget":{"used":10,"cap":6000}}`))
	}))
	defer ts.Close()
	c := apiclient.New(ts.URL, "tok")
	cc, err := c.ComposeContext(context.Background(), apiclient.ContextQuery{Remote: "flow"})
	if err != nil {
		t.Fatal(err)
	}
	if cc.Budget.Used != 10 {
		t.Fatalf("decode mismatch: %+v", cc)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `go test ./internal/adapter/apiclient/ -run TestClient_ComposeContext -v` → FAIL (`undefined: apiclient.ContextQuery`).

- [ ] **Step 3: Implement** — `internal/adapter/apiclient/context.go`:
```go
package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/serverkraken/flow/internal/usecase"
)

type ContextQuery struct {
	Remote, Machine, Path, Node string
	Cap                         int
}

func (q ContextQuery) values() url.Values {
	v := url.Values{}
	set := func(k, s string) {
		if s != "" {
			v.Set(k, s)
		}
	}
	set("remote", q.Remote)
	set("machine", q.Machine)
	set("path", q.Path)
	set("node", q.Node)
	if q.Cap > 0 {
		v.Set("cap", strconv.Itoa(q.Cap))
	}
	return v
}

func (c *Client) ComposeContext(ctx context.Context, in ContextQuery) (usecase.ComposedContext, error) {
	var out usecase.ComposedContext
	err := c.do(ctx, http.MethodGet, "/api/v1/context?"+in.values().Encode(), nil, &out)
	return out, err
}

type SetActiveContextInput struct {
	Remote  string   `json:"remote,omitempty"`
	Machine string   `json:"machine,omitempty"`
	Path    string   `json:"path,omitempty"`
	Node    string   `json:"node,omitempty"`
	Title   string   `json:"title,omitempty"`
	Body    string   `json:"body"`
	Tags    []string `json:"tags,omitempty"`
}

type SetActiveContextResult struct {
	ID        string `json:"id"`
	UpdatedAt string `json:"updatedAt"`
}

func (c *Client) SetActiveContext(ctx context.Context, in SetActiveContextInput) (SetActiveContextResult, error) {
	var out SetActiveContextResult
	err := c.do(ctx, http.MethodPut, "/api/v1/context/active", in, &out)
	return out, err
}

func (c *Client) SetPinned(ctx context.Context, id string, pinned bool) error {
	return c.do(ctx, http.MethodPost, "/api/v1/documents/"+id+"/pin", map[string]bool{"pinned": pinned}, nil)
}
```

- [ ] **Step 4: Run to verify pass** — `go test ./internal/adapter/apiclient/ -run 'TestClient_ComposeContext' -v && go build ./...` → PASS.

- [ ] **Step 5: Commit**
```bash
git add internal/adapter/apiclient/context.go internal/adapter/apiclient/context_test.go
git commit -m "feat(apiclient): ComposeContext/SetActiveContext/SetPinned (B3 B4)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

- [ ] **Step 6: Slice-B gate** — `make ci` green.

---

## Slice C — CLI + MCP

> The render, the offline cache, and the flush decision are extracted as **pure functions** so they unit-test without git/network. The cobra/MCP wiring around them mirrors `cmd/flow/node.go` and `cmd/flow-mcp/tools_docs.go`.

### Task C1: `flow context` — resolve, render, offline cache

**Files:**
- Create: `cmd/flow/context.go` (`contextCmd()` + `runContext` + pure `renderContext` + cache helpers)
- Test: `cmd/flow/context_test.go`

**Interfaces:**
- Consumes: `apiclient.Client.ComposeContext`, `apiclient.ContextQuery`, `gitremote.OriginSlug`, `clientmachine.Load`, `clientFromStore`, `usecase.ComposedContext`.
- Produces: `flow context [--path <dir>] [--repo <slug>] [--cap <n>] [--json]`; pure `renderContext(cc usecase.ComposedContext, offline bool, stamp string) string`.

- [ ] **Step 1: Write the failing test** — `cmd/flow/context_test.go` (test the pure render + cache round-trip):
```go
package main

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/usecase"
)

func TestRenderContext_BodiesAndFooter(t *testing.T) {
	cc := usecase.ComposedContext{
		Instructions:  []usecase.ContextItem{{ScopeLabel: "repo:flow", Body: "RULE A"}},
		ActiveContext: &usecase.ContextItem{ScopeLabel: "repo:flow", Body: "where I was"},
		Memories: map[string][]usecase.ContextItem{
			"leaf": {{ScopeLabel: "repo:flow", Body: "leaf mem"}},
		},
	}
	cc.Budget.Used = 1200
	cc.Budget.Cap = 6000
	cc.Budget.Dropped.Engagement = 2
	out := renderContext(cc, false, "")
	for _, want := range []string{"RULE A", "where I was", "leaf mem", "1200/6000", "+2 engagement"} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q\n---\n%s", want, out)
		}
	}
}

func TestRenderContext_UnboundHintAndOffline(t *testing.T) {
	cc := usecase.ComposedContext{}
	cc.Resolution.Unresolved = true
	out := renderContext(cc, true, "2026-06-28T10:00:00Z")
	if !strings.Contains(out, "flow node bind") {
		t.Errorf("unbound render must hint at `flow node bind`:\n%s", out)
	}
	if !strings.Contains(out, "offline") || !strings.Contains(out, "2026-06-28T10:00:00Z") {
		t.Errorf("offline render must carry the stale marker:\n%s", out)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `go test ./cmd/flow/ -run TestRenderContext -v` → FAIL (`undefined: renderContext`).

- [ ] **Step 3: Implement** — `cmd/flow/context.go`:
```go
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/clientmachine"
	"github.com/serverkraken/flow/internal/gitremote"
	"github.com/serverkraken/flow/internal/usecase"
	"github.com/spf13/cobra"
)

func contextCmd() *cobra.Command {
	var path, repo string
	var capN int
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Show the composed start-context for this repo (SessionStart hook source)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runContext(cmd.Context(), cmd.OutOrStdout(), path, repo, capN, asJSON)
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "directory to resolve (default: cwd)")
	cmd.Flags().StringVar(&repo, "repo", "", "explicit node slug override")
	cmd.Flags().IntVar(&capN, "cap", 0, "token budget override")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the raw compose JSON")
	cmd.AddCommand(installHooksCmd(), flushCheckCmd())
	return cmd
}

func runContext(ctx context.Context, out interface{ Write([]byte) (int, error) }, path, repo string, capN int, asJSON bool) error {
	dir := path
	if dir == "" {
		dir, _ = os.Getwd()
	}
	dir = filepath.Clean(dir)
	remote, _, _ := gitremote.OriginSlug(dir)
	m, _ := clientmachine.Load()
	q := apiclient.ContextQuery{Remote: remote, Machine: m.ID, Path: dir, Node: repo, Cap: capN}

	c, err := clientFromStore(ctx)
	if err == nil {
		cc, cerr := c.ComposeContext(ctx, q)
		if cerr == nil {
			_ = writeContextCache(q, cc)
			return emit(out, cc, false, "", asJSON)
		}
		err = cerr
	}
	// Network/auth failure → offline cache (SessionStart must never hard-fail).
	if cc, stamp, ok := readContextCache(q); ok {
		return emit(out, cc, true, stamp, asJSON)
	}
	fmt.Fprintf(out, "# flow context\n\n_(offline — no cached context for this repo; %v)_\n", err)
	return nil // exit 0: do not break the hook
}

func emit(out interface{ Write([]byte) (int, error) }, cc usecase.ComposedContext, offline bool, stamp string, asJSON bool) error {
	if asJSON {
		b, _ := json.MarshalIndent(cc, "", "  ")
		_, err := out.Write(append(b, '\n'))
		return err
	}
	_, err := out.Write([]byte(renderContext(cc, offline, stamp)))
	return err
}

// renderContext is pure: ComposedContext → the Markdown block the SessionStart hook injects.
func renderContext(cc usecase.ComposedContext, offline bool, stamp string) string {
	var b strings.Builder
	b.WriteString("# flow context\n")
	if cc.Resolution.Unresolved {
		b.WriteString("\n_(repo not bound — run `flow node bind` to attach this directory)_\n")
	}
	if len(cc.Instructions) > 0 {
		b.WriteString("\n## Instructions\n")
		for _, it := range cc.Instructions {
			fmt.Fprintf(&b, "\n### [%s]\n%s\n", it.ScopeLabel, it.Body)
		}
	}
	b.WriteString("\n## Active Context\n")
	if cc.ActiveContext != nil {
		fmt.Fprintf(&b, "%s\n", cc.ActiveContext.Body)
	} else {
		b.WriteString("_(none yet — flush with `flow_set_active_context`)_\n")
	}
	groups := []struct{ key, label string }{
		{"leaf", "Leaf"}, {"vorhaben", "Vorhaben"}, {"engagement", "Engagement"}, {"global", "Global"},
	}
	wrote := false
	for _, g := range groups {
		items := cc.Memories[g.key]
		if len(items) == 0 {
			continue
		}
		if !wrote {
			b.WriteString("\n## Memories\n")
			wrote = true
		}
		for _, it := range items {
			fmt.Fprintf(&b, "\n### [%s] %s\n%s\n", g.label, it.ScopeLabel, it.Body)
		}
	}
	b.WriteString("\n---\n")
	fmt.Fprintf(&b, "%d/%d tokens", cc.Budget.Used, cc.Budget.Cap)
	if cc.Budget.Dropped.Engagement > 0 {
		fmt.Fprintf(&b, " · +%d engagement not shown", cc.Budget.Dropped.Engagement)
	}
	if cc.Budget.Dropped.Global > 0 {
		fmt.Fprintf(&b, " · +%d global not shown", cc.Budget.Dropped.Global)
	}
	if offline {
		fmt.Fprintf(&b, " · ⚠ offline — Stand %s", stamp)
	}
	b.WriteString("\n")
	return b.String()
}

type cachedContext struct {
	Stamp string                  `json:"stamp"`
	CC    usecase.ComposedContext `json:"cc"`
}

func contextCacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".flow", "context-cache")
}

func cacheKey(q apiclient.ContextQuery) string {
	sum := sha256.Sum256([]byte(q.Remote + "|" + q.Node + "|" + q.Path))
	return fmt.Sprintf("%x", sum[:8])
}

func writeContextCache(q apiclient.ContextQuery, cc usecase.ComposedContext) error {
	dir := contextCacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(cachedContext{Stamp: nowStamp(), CC: cc})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, cacheKey(q)+".json"), b, 0o644)
}

func readContextCache(q apiclient.ContextQuery) (usecase.ComposedContext, string, bool) {
	b, err := os.ReadFile(filepath.Join(contextCacheDir(), cacheKey(q)+".json"))
	if err != nil {
		return usecase.ComposedContext{}, "", false
	}
	var cached cachedContext
	if err := json.Unmarshal(b, &cached); err != nil {
		return usecase.ComposedContext{}, "", false
	}
	return cached.CC, cached.Stamp, true
}
```
> **Note:** `nowStamp()` returns `time.Now().UTC().Format(time.RFC3339)` — add it as a tiny helper (or inline). `clientFromStore` is `cmd/flow/auth.go:13`. Register `contextCmd()` in `rootCmd()` (Task D1).

- [ ] **Step 4: Run to verify pass** — `go test ./cmd/flow/ -run TestRenderContext -v && go build ./...` → PASS.

- [ ] **Step 5: Commit**
```bash
git add cmd/flow/context.go cmd/flow/context_test.go
git commit -m "feat(cli): flow context — resolve, render Markdown/JSON, offline cache (B3 C1)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task C2: `flow context install-hooks`

**Files:**
- Create: `cmd/flow/context_hooks.go` (`installHooksCmd()` + pure `mergeHooks`)
- Test: `cmd/flow/context_hooks_test.go`

**Interfaces:**
- Produces: `flow context install-hooks [--print]`; pure `mergeHooks(settings map[string]any) (map[string]any, bool)` (returns the merged settings + whether it changed).

- [ ] **Step 1: Write the failing test** — `cmd/flow/context_hooks_test.go`:
```go
package main

import "testing"

func TestMergeHooks_AddsThenIdempotent(t *testing.T) {
	got, changed := mergeHooks(map[string]any{})
	if !changed {
		t.Fatal("first merge must report a change")
	}
	hooks, _ := got["hooks"].(map[string]any)
	if hooks["SessionStart"] == nil || hooks["Stop"] == nil {
		t.Fatalf("both hooks must be installed: %+v", hooks)
	}
	_, changed2 := mergeHooks(got)
	if changed2 {
		t.Fatal("second merge must be idempotent (no change)")
	}
}

func TestMergeHooks_PreservesForeignHooks(t *testing.T) {
	in := map[string]any{"hooks": map[string]any{
		"SessionStart": []any{map[string]any{"hooks": []any{
			map[string]any{"type": "command", "command": "some-other-tool"},
		}}},
	}}
	got, _ := mergeHooks(in)
	ss, _ := got["hooks"].(map[string]any)["SessionStart"].([]any)
	// the foreign entry must survive AND our flow-context entry must be added.
	var foreign, ours bool
	for _, group := range ss {
		for _, h := range group.(map[string]any)["hooks"].([]any) {
			cmd, _ := h.(map[string]any)["command"].(string)
			if cmd == "some-other-tool" {
				foreign = true
			}
			if cmd == sessionStartCommand {
				ours = true
			}
		}
	}
	if !foreign || !ours {
		t.Fatalf("foreign=%v ours=%v", foreign, ours)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `go test ./cmd/flow/ -run TestMergeHooks -v` → FAIL.

- [ ] **Step 3: Implement** — `cmd/flow/context_hooks.go`:
```go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const (
	sessionStartCommand = `flow context --path "$CLAUDE_PROJECT_DIR"`
	stopCommand         = `flow context flush-check --path "$CLAUDE_PROJECT_DIR"`
)

func installHooksCmd() *cobra.Command {
	var printOnly bool
	cmd := &cobra.Command{
		Use:   "install-hooks",
		Short: "Install the SessionStart+Stop hooks into ~/.claude/settings.json (idempotent)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, _ := os.UserHomeDir()
			p := filepath.Join(home, ".claude", "settings.json")
			settings := map[string]any{}
			if b, err := os.ReadFile(p); err == nil {
				_ = json.Unmarshal(b, &settings)
			}
			merged, changed := mergeHooks(settings)
			b, _ := json.MarshalIndent(merged, "", "  ")
			if printOnly {
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
				return nil
			}
			if !changed {
				fmt.Fprintln(cmd.OutOrStdout(), "hooks already installed")
				return nil
			}
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(p, append(b, '\n'), 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "installed SessionStart+Stop hooks into %s\n", p)
			fmt.Fprintln(cmd.OutOrStdout(), "tip: turn OFF native auto-memory and write memory only via flow (flow_create_doc/update_doc).")
			return nil
		},
	}
	cmd.Flags().BoolVar(&printOnly, "print", false, "print the merged settings without writing")
	return cmd
}

// mergeHooks adds our two command entries to hooks.SessionStart and hooks.Stop,
// preserving any existing/foreign entries. Returns (merged, changed).
func mergeHooks(settings map[string]any) (map[string]any, bool) {
	if settings == nil {
		settings = map[string]any{}
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	changed := false
	ensure := func(event, command string) {
		groups, _ := hooks[event].([]any)
		for _, g := range groups { // already present?
			gm, _ := g.(map[string]any)
			hs, _ := gm["hooks"].([]any)
			for _, h := range hs {
				if hm, _ := h.(map[string]any); hm != nil {
					if c, _ := hm["command"].(string); c == command {
						return
					}
				}
			}
		}
		groups = append(groups, map[string]any{
			"hooks": []any{map[string]any{"type": "command", "command": command}},
		})
		hooks[event] = groups
		changed = true
	}
	ensure("SessionStart", sessionStartCommand)
	ensure("Stop", stopCommand)
	settings["hooks"] = hooks
	return settings, changed
}
```

- [ ] **Step 4: Run to verify pass** — `go test ./cmd/flow/ -run TestMergeHooks -v && go build ./...` → PASS.

- [ ] **Step 5: Commit**
```bash
git add cmd/flow/context_hooks.go cmd/flow/context_hooks_test.go
git commit -m "feat(cli): flow context install-hooks — idempotent settings.json merge (B3 C2)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task C3: `flow context flush-check` (Stop-hook decision)

**Files:**
- Create: `cmd/flow/context_flush.go` (`flushCheckCmd()` + pure `flushDecision` + transcript scan)
- Test: `cmd/flow/context_flush_test.go`

**Interfaces:**
- Produces: `flow context flush-check [--path <dir>]`; pure `flushDecision(in flushInput) (remind bool)`; `flushInput{StopHookActive bool; MutatingToolUses int; ActiveStale bool}`.

- [ ] **Step 1: Write the failing test** — `cmd/flow/context_flush_test.go`:
```go
package main

import "testing"

func TestFlushDecision(t *testing.T) {
	cases := []struct {
		name string
		in   flushInput
		want bool
	}{
		{"loop guard", flushInput{StopHookActive: true, MutatingToolUses: 5, ActiveStale: true}, false},
		{"no work", flushInput{MutatingToolUses: 0, ActiveStale: true}, false},
		{"fresh already flushed", flushInput{MutatingToolUses: 3, ActiveStale: false}, false},
		{"work + stale → remind", flushInput{MutatingToolUses: 3, ActiveStale: true}, true},
	}
	for _, c := range cases {
		if got := flushDecision(c.in); got != c.want {
			t.Errorf("%s: flushDecision=%v want %v", c.name, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `go test ./cmd/flow/ -run TestFlushDecision -v` → FAIL.

- [ ] **Step 3: Implement** — `cmd/flow/context_flush.go`:
```go
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/clientmachine"
	"github.com/serverkraken/flow/internal/gitremote"
	"github.com/spf13/cobra"
)

const flushReminder = "Du hast in dieser Session gearbeitet, aber active-context (wo war ich / was offen / nächster Schritt) nicht aktualisiert — flush jetzt via flow_set_active_context, bevor du stoppst."

type stopHookInput struct {
	StopHookActive bool   `json:"stop_hook_active"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
}

type flushInput struct {
	StopHookActive   bool
	MutatingToolUses int
	ActiveStale      bool
}

// flushDecision: remind only when real work happened AND activeContext is stale,
// never while a stop-hook continuation is already in flight.
func flushDecision(in flushInput) bool {
	if in.StopHookActive {
		return false
	}
	return in.MutatingToolUses > 0 && in.ActiveStale
}

func flushCheckCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:    "flush-check",
		Short:  "Stop-hook: conditionally remind to flush active-context",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var hin stopHookInput
			_ = json.NewDecoder(cmd.InOrStdin()).Decode(&hin) // best-effort; missing fields default
			uses, sessionStart := scanTranscript(hin.TranscriptPath)
			stale := activeContextStale(cmd.Context(), path, hin.Cwd, sessionStart)
			if !flushDecision(flushInput{StopHookActive: hin.StopHookActive, MutatingToolUses: uses, ActiveStale: stale}) {
				return nil // silent, exit 0
			}
			out := map[string]any{"hookSpecificOutput": map[string]any{
				"hookEventName":     "Stop",
				"additionalContext": flushReminder,
			}}
			b, _ := json.Marshal(out)
			fmt.Fprintln(cmd.OutOrStdout(), string(b))
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "directory to resolve (default: cwd / hook cwd)")
	return cmd
}

// scanTranscript counts mutating tool_use entries and returns the first timestamp seen.
// Heuristic per spec §13 — refine at dogfood. Returns (0,"") on any read error.
func scanTranscript(p string) (int, string) {
	f, err := os.Open(p)
	if err != nil {
		return 0, ""
	}
	defer func() { _ = f.Close() }()
	mutating := map[string]bool{"Edit": true, "Write": true, "Bash": true, "NotebookEdit": true}
	uses, first := 0, ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)
	for sc.Scan() {
		var row struct {
			Timestamp string `json:"timestamp"`
			Message   struct {
				Content []struct {
					Type string `json:"type"`
					Name string `json:"name"`
				} `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &row) != nil {
			continue
		}
		if first == "" && row.Timestamp != "" {
			first = row.Timestamp
		}
		for _, c := range row.Message.Content {
			if c.Type == "tool_use" && (mutating[c.Name] || strings.HasPrefix(c.Name, "mcp__flow__flow_set") || strings.HasPrefix(c.Name, "mcp__flow__flow_create") || strings.HasPrefix(c.Name, "mcp__flow__flow_update")) {
				uses++
			}
		}
	}
	return uses, first
}

// activeContextStale asks the server for the current activeContext updatedAt and compares
// it to the session start. Any error → treat as NOT stale (stay silent, never nag wrongly).
func activeContextStale(ctx context.Context, path, hookCwd, sessionStart string) bool {
	dir := path
	if dir == "" {
		dir = hookCwd
	}
	remote, _, _ := gitremote.OriginSlug(dir)
	m, _ := clientmachine.Load()
	c, err := clientFromStore(ctx)
	if err != nil {
		return false
	}
	cc, err := c.ComposeContext(ctx, apiclient.ContextQuery{Remote: remote, Machine: m.ID, Path: dir})
	if err != nil || cc.ActiveContext == nil {
		return cc.ActiveContext == nil && err == nil // no activeContext yet + work done → stale
	}
	return sessionStart != "" && cc.ActiveContext.UpdatedAt < sessionStart
}

var _ = io.EOF // keep io imported if unused after edits
```
> **Note:** the transcript schema + the mutating-tool set are the §13 calibration knobs — adjust at dogfood. Drop the `io` import if unused.

- [ ] **Step 4: Run to verify pass** — `go test ./cmd/flow/ -run TestFlushDecision -v && go build ./...` → PASS.

- [ ] **Step 5: Commit**
```bash
git add cmd/flow/context_flush.go cmd/flow/context_flush_test.go
git commit -m "feat(cli): flow context flush-check — conditional Stop-hook reminder (B3 C3)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task C4: MCP `flow_get_context` + `flow_set_active_context`

**Files:**
- Create: `cmd/flow-mcp/tools_context.go` (two tools + input structs + handlers)
- Modify: `cmd/flow-mcp/server.go` (register both via `mcp.AddTool`)
- Test: `cmd/flow-mcp/tools_context_test.go` (if the package has handler tests; else assert registration compiles)

**Interfaces:**
- Consumes: `apiclient.Client.ComposeContext`/`SetActiveContext`, `h.mgr.Do`, `h.resolveScope`/`h.resolved` (project resolution), `textResult`, `resultErr`.
- Produces: tools `flow_get_context`, `flow_set_active_context`.

- [ ] **Step 1: Write `cmd/flow-mcp/tools_context.go`** (mirror `tools_docs.go` handler shape):
```go
package main

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

type getContextIn struct {
	Repo string `json:"repo,omitempty" jsonschema:"explicit node slug override; default = the current directory's resolved repo"`
	Cap  int    `json:"cap,omitempty"  jsonschema:"token budget override"`
}

func (h *handlers) getContext(ctx context.Context, _ *mcp.CallToolRequest, in getContextIn) (*mcp.CallToolResult, any, error) {
	var out string
	err := h.mgr.Do(ctx, func(c *apiclient.Client) error {
		q := apiclient.ContextQuery{Node: in.Repo, Cap: in.Cap}
		if in.Repo == "" {
			q.Remote, q.Machine, q.Path = h.resolveTriple(ctx) // remote/machine/cwd from the MCP host (see note)
		}
		cc, err := c.ComposeContext(ctx, q)
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(cc, "", "  ")
		out = string(b)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}

type setActiveContextIn struct {
	Repo string   `json:"repo,omitempty"  jsonschema:"explicit node slug override; default = the current directory's resolved repo"`
	Body string   `json:"body"            jsonschema:"the activeContext markdown (where I was / what's open / next step)"`
	Tags []string `json:"tags,omitempty"  jsonschema:"tags as a flat list; replaces the whole set"`
}

func (h *handlers) setActiveContext(ctx context.Context, _ *mcp.CallToolRequest, in setActiveContextIn) (*mcp.CallToolResult, any, error) {
	err := h.mgr.Do(ctx, func(c *apiclient.Client) error {
		input := apiclient.SetActiveContextInput{Node: in.Repo, Body: in.Body, Tags: in.Tags}
		if in.Repo == "" {
			input.Remote, input.Machine, input.Path = h.resolveTriple(ctx)
		}
		_, err := c.SetActiveContext(ctx, input)
		return err
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult("active-context updated"), nil, nil
}
```
> **Note:** `h.resolveTriple(ctx)` is a tiny helper returning `(remoteSlug, machineID, cwd)` via `gitremote.OriginSlug(os.Getwd())` + `clientmachine.Load()` — add it next to `resolve.go` (the MCP already imports both, see `tools_project.go`). If the MCP prefers its cached `h.resolved()` node, you may instead set `q.Node = resolvedNode.Slug`; either path works since the server resolves. Match `textResult`/`resultErr` exact names from `server.go`.

- [ ] **Step 2: Register** — in `cmd/flow-mcp/server.go` (alongside the other `mcp.AddTool` calls):
```go
	mcp.AddTool(s, &mcp.Tool{Name: "flow_get_context", Description: "Compose the cross-device start-context (instructions + activeContext + memories) for the current repo, token-budgeted."}, h.getContext)
	mcp.AddTool(s, &mcp.Tool{Name: "flow_set_active_context", Description: "Upsert this repo's activeContext memory (where I was / what's open / next step)."}, h.setActiveContext)
```

- [ ] **Step 3: Build + (if present) test** — `go build ./... && go test ./cmd/flow-mcp/ -v` → PASS.

- [ ] **Step 4: Commit**
```bash
git add cmd/flow-mcp/tools_context.go cmd/flow-mcp/server.go cmd/flow-mcp/tools_context_test.go
git commit -m "feat(mcp): flow_get_context + flow_set_active_context tools (B3 C4)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

- [ ] **Step 5: Slice-C gate** — `make ci` green.

---

## Slice D — Wiring + Einschritt-Write + Done-Gate

### Task D1: Composition-root wiring + curl-smoke

**Files:**
- Modify: `cmd/flow/main.go` (register `contextCmd()`)
- Modify: `cmd/flow-server/main.go` (wire `ComposeContext`/`SetActiveContext`/`SetPinned`/`ContextBudget`; `FLOW_CONTEXT_BUDGET`)
- Create: `scripts/smoke-context.sh` (curl every new route against the dev stack)

**Interfaces:**
- Consumes: every usecase/handler from Slices A–C. This is the [[feedback_plan_main_wiring_task]] task — the composition root must actually call the new constructors, and every route must answer.

- [ ] **Step 1: Register the CLI command** — `cmd/flow/main.go` `rootCmd()`: add `root.AddCommand(contextCmd())`.

- [ ] **Step 2: Wire the server** — `cmd/flow-server/main.go`, inside the `srv := &httpserver.Server{...}` literal, add:
```go
		ComposeContext: usecase.ComposeContext{
			Resolve: usecase.ResolveNode{Bindings: bindingStore, Nodes: nodeStore},
			Nodes:   nodeStore, Docs: documentStore, Tags: tagStore,
		},
		SetActiveContext: usecase.SetActiveContext{
			Resolve: usecase.ResolveNode{Bindings: bindingStore, Nodes: nodeStore},
			Nodes:   nodeStore, Docs: documentStore, Tags: tagStore,
		},
		SetPinned:     usecase.SetPinned{Docs: documentStore},
		ContextBudget: contextBudget(os.Getenv),
```
And add the helper (near the top-level funcs in `main.go`):
```go
func contextBudget(getenv func(string) string) int {
	if v := getenv("FLOW_CONTEXT_BUDGET"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 6000
}
```
> Confirm `documentStore := pgstore.NewDocumentStore(pool, ids)` (the `ids` arg was added in A2). Add `"strconv"` to imports if needed.

- [ ] **Step 3: Build + boot** — `go build ./... && make dev-up && make dev-run` (Postgres + Dex; `FLOW_DEV=1`; migrations 0001–0021 apply on boot). Confirm the log shows migration `0021` applied.

- [ ] **Step 4: Write `scripts/smoke-context.sh`** (mirror any existing smoke script; token via `make dev-token`):
```bash
#!/usr/bin/env bash
set -euo pipefail
BASE="${FLOW_SERVER_URL:-https://localhost:8080}"
TOK="$(make -s dev-token)"
curl() { command curl -ks -H "Authorization: Bearer $TOK" "$@"; }

echo "== GET /context (unresolved → 200, unresolved=true) =="
curl "$BASE/api/v1/context?remote=does-not-exist" | tee /dev/stderr | grep -q '"unresolved":true'

echo "== PUT /context/active (bound repo via ?node override) =="
# replace <slug> with a real bound engagement/repo slug from `flow node list`
curl -X PUT "$BASE/api/v1/context/active" -H 'Content-Type: application/json' \
  -d '{"node":"<slug>","title":"AC","body":"smoke where-I-was","tags":["smoke"]}' | grep -q '"id"'

echo "== GET /context?node=<slug> shows the activeContext =="
curl "$BASE/api/v1/context?node=<slug>" | grep -q 'smoke where-I-was'

echo "== POST /documents/{id}/pin (use an id from the response above) =="
# curl -X POST "$BASE/api/v1/documents/<id>/pin" -d '{"pinned":true}' -w '%{http_code}\n'

echo "smoke OK"
```

- [ ] **Step 5: Run the smoke** — `bash scripts/smoke-context.sh` against the running dev server. All assertions pass.

- [ ] **Step 6: Commit**
```bash
git add cmd/flow/main.go cmd/flow-server/main.go scripts/smoke-context.sh
git commit -m "feat: wire ComposeContext/SetActiveContext/SetPinned + context CLI + curl-smoke (B3 D1)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task D2: Einschritt-Write convention + Done-Gate (dogfood)

**Files:**
- Modify: `docs/superpowers/specs/2026-06-28-flow-kontext-b3-kontext-store-design.md` (flip Status → implemented + record the done-gate)
- Memory: add/update a `project_flow_rebuild_*` memory entry

This task is a **gate**, not code. Complete every checkbox before declaring B3-Kern done.

- [ ] **Step 1: `make ci` green** (lint incl. gofumpt/staticcheck/QF1002, verify-generate/css/no-popups, coverage gate held, build). The pure `Compose` + render + flushDecision + mergeHooks table tests carry the new coverage.

- [ ] **Step 2: Live smoke vs Postgres+Dex** — with `make dev-run` up:
  - `flow context --json` in a bound repo → resolves the chain, returns instructions+activeContext+memories, budget footer present.
  - `flow_set_active_context` (or `PUT /context/active`) → `flow context` now shows the activeContext; a second flush keeps the same doc id (path-upsert idempotent).
  - Pin an engagement memory (`POST /documents/{id}/pin`), shrink `--cap` below the tier size → the pinned item survives while an unpinned newer one drops; footer shows `+N engagement not shown`.
  - Tag a global memory + a chain node with the same tag → it crosses into `Memories.global`; remove the tag → it disappears (D7 gate proven).

- [ ] **Step 3: Real Claude Code hook dogfood** —
  - `flow context install-hooks` → inspect `~/.claude/settings.json` (both hooks present, foreign hooks preserved).
  - Open a Claude Code session in a bound repo → the SessionStart block is injected (visible context: instructions + activeContext + memories + footer).
  - Do mutating work (edit a file) **without** flushing → at Stop, the reminder fires once; flush via `flow_set_active_context` → next Stop is **silent** (freshness debounce).
  - Stop flow (`make dev-down` or kill the server), open a session → SessionStart serves the **offline cache** with the stale marker and **does not break** startup.

- [ ] **Step 4: Einschritt-Write convention** — document (in the `install-hooks` tip output, already wired in C2, and in the spec): turn **off** native Claude Code auto-memory and write memory **only** via flow (`flow_create_doc`/`flow_update_doc`; activeContext via `flow_set_active_context`). Verify a memory write lands in flow and not in a local `MEMORY.md`. (No code — operational convention; the loaded context is the single source.)

- [ ] **Step 5: Flip the spec status** — edit the B3-Kern spec header `**Status:**` → `implemented (Slices A–D)` + a one-line done-gate record (ci %, live-verified, hook-dogfood). Commit:
```bash
git add docs/superpowers/specs/2026-06-28-flow-kontext-b3-kontext-store-design.md
git commit -m "docs(b3): flip spec to implemented + record done-gate (B3 D2)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

- [ ] **Step 6: Update memory** — add a `project_flow_rebuild_b3_kontext_store` memory (DONE, commit range, ci %, what landed: compose endpoint + path-upsert + hooks + CLI/MCP; what's deferred: B3c branch + auto-create, B3d doctype+migration, Quer A/B). Update `MEMORY.md` index line. Re-sync the spec + this plan to flow (`flow_update_doc`).

- [ ] **Step 7: Final whole-branch review (recommended)** — an Opus holistic review of the slice branch before integrating to `rebuild`, per the B1/B2 precedent (catches cross-task bugs per-task reviews miss, e.g. the SSE/live-sync class of issues in M2b, the taggings-cleanup miss in B2-9).

---

## Plan self-review

**Spec coverage (§ → task):** Compose-Read §1/§2 ranking+budget → A4 (pure) + A5 (orchestration); `global≠none` §B3-10 → A3 `includeGlobal` + A5; D7 tag-gate §1.4 → A5 `globalAllowed`; D5 bootstrap-types §B3-5 → A5 `bootstrapTypes`; `pinned` §2/§B3-4 → A1; path-upsert §2/§3-write → A2 + B2; kind-agnostic/no-upstream §B3-11 → A4 tier-by-position + A5 (resolution returns leaf of any kind); token heuristic §3 → A4 `estTokens`; REST §5 → B1/B2/B3; MCP §6 → C4; CLI §7 + offline-cache §B3-9 → C1; hooks §8 (SessionStart/Stop/install) → C1 command + C2 install + C3 flush-check; einschritt-write §9 → C2 tip + D2; wiring §10 → D1; testing §11 → every task's TDD steps + D2 gate; slicing §12 → slice map; calibration §13 → A4 `estTokens` note + C3 transcript-scan note + `FLOW_CONTEXT_BUDGET`.

**No-placeholder scan:** every code step shows complete code; the genuinely deferred calibration points (`estTokens` factor, flush "real-work" signal, budget number) are explicitly flagged as dogfood knobs (§13), not as missing implementation. The `<slug>`/`<id>` tokens in the smoke script are runtime values the operator fills, not plan gaps.

**Type consistency:** `usecase.ComposeContext{Resolve,Nodes,Docs,Tags}` identical across A5/B1/B2/D1; `ContextResolveInput{RemoteSlug,MachineID,Cwd,NodeOverride}` identical across A5/B1/B2/C/D; `DocumentStore.{ListForContext,UpsertByPath,SetPinned}` signatures identical across ports (A1–A3), pgstore, fakes, and callers; `Compose(chain,docs,globalAllowed,cap)` identical A4↔A5; `ComposedContext.Memories` keys `"leaf"/"vorhaben"/"engagement"/"global"` identical A4↔C1 render; `ActiveContextPath="active-context"` single source (A4) used by A4/A5/B2.

**Known seams the implementer must reconcile against real code (flagged inline):** `NewDocumentStore` gains an `ids` arg (A2); `FakeNodeStore.Ancestors` must exist (B1 added it — verify); fake helper names (`s.docs`/`s.nextID`/bind method) match the real fakes; `collectDocuments`/`hydrateTags` reuse the exact existing `List` helpers (A3); `textResult`/`resultErr`/`h.resolveTriple` match MCP `server.go` (C4).

