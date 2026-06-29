# Querschnitt A — Memory-Lifecycle (Archivierung) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a boolean `archived` lifecycle flag to documents — out of bootstrap/compose + default lists/search, but findable + reversible — mirroring the existing `pinned` flag, plus the import-manifest disposition and an idempotent bulk-archive pass over existing done-milestones.

**Architecture:** `archived` (+ `archived_at`) is a column on `documents`, exactly like `pinned`. The read-path exclusion lives in SQL (one `AND NOT archived` per query) so the pure `Compose()` ranking is untouched. A dedicated `SetArchived` setter mirrors `SetPinned` through every layer (domain → ports → pgstore → usecase → REST → apiclient → MCP → CLI). `SetArchived(true)` also clears `pinned` (archived dominates).

**Tech Stack:** Go (hexagonal), pgx/Postgres (goose migrations), net/http (stdlib mux), modelcontextprotocol/go-sdk (MCP), cobra (CLI). Tests: standard `testing`; pgstore tests run against Docker Postgres.

**Spec:** `docs/superpowers/specs/2026-06-29-flow-kontext-querschnitt-a-lifecycle-design.md`

## Global Constraints

- **Mirror `pinned` exactly** where a `pinned` analog exists; reference the cited `pinned` line as the template.
- `make ci` must stay green: `gofumpt`, `staticcheck`, `templ`, build, tests, coverage gate. Run `make ci` before each commit where practical; run at minimum at the Task 12 done-gate.
- **goose migrations need `-- +goose Up` / `-- +goose Down` annotations** — bare SQL fails at apply-time, the build does NOT catch it; only pgstore Docker tests do.
- **pgstore tests need Docker/Postgres.** On this machine use the podman socket: `DOCKER_HOST=unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')` if `make ci`'s pgstore step needs it.
- **`archived` dominates `pinned`:** `SetArchived(true)` clears `pinned`; un-archiving does NOT restore `pinned`.
- **docCols grows from 13 → 15 columns** (`… , pinned, archived, archived_at`). EVERY scan (`scanDocument`, `scanSearchHit`, `scanSemanticHit`) and `Create` must be updated in the SAME task (Task 1), or column-count mismatches break unrelated tests.
- **Scope trim vs spec:** `superseded_by` (the "ersetzt durch [[x]]" note in `extra`, spec §6/A-7) is **deferred to a follow-up within A** — it needs an `extra`-write path that the bool-only `SetArchived` doesn't carry. The MCP/REST archive surface ships `{id, archived}` only. (Flagged for approval; easy to add later without schema change since `extra` JSONB already exists.)
- UI (badges, Aufräum-Ansicht, Inspektor) is **Querschnitt B — not in this plan.**

---

### Task 1: `archived` column round-trips through pgstore

**Files:**
- Create: `internal/adapter/pgstore/migrations/0022_documents_archived.sql`
- Modify: `internal/domain/document.go:73` (after `Pinned`)
- Modify: `internal/adapter/pgstore/documents.go:30-32` (docCols), `:91` (Create), `:502-520` (scanDocument), `:362` (scanSearchHit), `:477` (scanSemanticHit)
- Modify: `internal/testutil/fakes.go:612` (FakeDocumentStore.Create stores it automatically via struct — no change; add nothing)
- Test: `internal/adapter/pgstore/documents_test.go`

**Interfaces:**
- Produces: `domain.Document.Archived bool`, `domain.Document.ArchivedAt *time.Time`; the `documents.archived` / `documents.archived_at` columns; all reads hydrate both fields.

- [ ] **Step 1: Write the migration**

`internal/adapter/pgstore/migrations/0022_documents_archived.sql`:
```sql
-- +goose Up
ALTER TABLE documents ADD COLUMN archived    BOOLEAN     NOT NULL DEFAULT false;
ALTER TABLE documents ADD COLUMN archived_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE documents DROP COLUMN archived_at;
ALTER TABLE documents DROP COLUMN archived;
```

- [ ] **Step 2: Add the domain fields**

`internal/domain/document.go`, immediately after `Pinned bool \`json:"pinned"\`` (line 73):
```go
	Archived   bool       `json:"archived"`
	ArchivedAt *time.Time `json:"archivedAt,omitempty"`
```
(`time` is already imported — `CreatedAt`/`UpdatedAt` use it.)

- [ ] **Step 3: Write the failing test**

In `internal/adapter/pgstore/documents_test.go`, mirror the setup of `TestDocumentStore_SetPinned` (line 529):
```go
func TestDocumentStore_ArchivedRoundTrip(t *testing.T) {
	ds, cleanup := newTestDocumentStore(t) // same helper TestDocumentStore_SetPinned uses
	defer cleanup()
	ctx := context.Background()

	seedUser(t, ds, "u1") // same user-seed the neighbouring tests use
	d := domain.Document{
		ID: "d1", OwnerID: "u1", Type: domain.DocMemory, Path: "m1",
		Title: "M1", Body: "b", Archived: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if _, err := ds.Create(ctx, d); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := ds.Get(ctx, "u1", "d1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.Archived {
		t.Fatalf("archived not persisted: %+v", got)
	}
}
```
(Use the exact store/user-seed helpers from the adjacent tests in that file.)

- [ ] **Step 4: Run it — expect FAIL**

Run: `go test ./internal/adapter/pgstore/ -run TestDocumentStore_ArchivedRoundTrip` → FAIL (column does not exist / scan count mismatch).

- [ ] **Step 5: Wire the columns**

`documents.go:30-32` — append to both constants:
```go
const docCols = `id, owner_id, node_id, type, path, title, body, doc_date, role, extra, created_at, updated_at, pinned, archived, archived_at`
const prefixedDocCols = `d.id, d.owner_id, d.node_id, d.type, d.path, d.title, d.body, d.doc_date, d.role, d.extra, d.created_at, d.updated_at, d.pinned, d.archived, d.archived_at`
```
`scanDocument` (line 506-507) — add the two targets after `&d.Pinned`:
```go
	if err := r.Scan(&d.ID, &d.OwnerID, &d.NodeID, &typ, &d.Path, &d.Title, &d.Body,
		&d.Date, &d.Role, &extra, &d.CreatedAt, &d.UpdatedAt, &d.Pinned, &d.Archived, &d.ArchivedAt); err != nil {
```
`scanSearchHit` (line 362) and `scanSemanticHit` (line 477) — add `&d.Archived, &d.ArchivedAt` after their `&…Pinned` target, in the same column order (before any trailing snippet/distance column).
`Create` (line 91) — add `archived, archived_at` to the INSERT column list, `$14, $15` to `VALUES`, and append `d.Archived, d.ArchivedAt` to the exec args (mirroring how `$13`/`d.Pinned` are passed).

- [ ] **Step 6: Run it — expect PASS**

Run: `go test ./internal/adapter/pgstore/ -run TestDocumentStore_ArchivedRoundTrip` → PASS. Also `go build ./...` to confirm scan-count changes compile everywhere.

- [ ] **Step 7: Commit**

```bash
git add internal/adapter/pgstore/migrations/0022_documents_archived.sql internal/domain/document.go internal/adapter/pgstore/documents.go internal/adapter/pgstore/documents_test.go
git commit -m "feat(querschnitt-a): archived+archived_at column on documents (migr 0022)"
```

---

### Task 2: `SetArchived` setter (sets archived_at, clears pinned)

**Files:**
- Modify: `internal/ports/ports.go:199` (after `SetPinned`)
- Modify: `internal/adapter/pgstore/documents.go:200` (after `SetPinned`)
- Modify: `internal/testutil/fakes.go:925` (after `FakeDocumentStore.SetPinned`)
- Test: `internal/adapter/pgstore/documents_test.go`

**Interfaces:**
- Produces: `ports.DocumentStore.SetArchived(ctx, ownerID, id string, archived bool) error`.

- [ ] **Step 1: Add the port method**

`internal/ports/ports.go`, after line 199:
```go
	// SetArchived sets (archived=true) or clears (archived=false) the archived
	// flag. Archiving also clears pinned (archived dominates) and stamps
	// archived_at; un-archiving nulls archived_at and leaves pinned untouched.
	SetArchived(ctx context.Context, ownerID, id string, archived bool) error
```

- [ ] **Step 2: Add the fake (mirror SetPinned)**

`internal/testutil/fakes.go`, after `SetPinned` (line 935):
```go
func (s *FakeDocumentStore) SetArchived(_ context.Context, ownerID, id string, archived bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.m[id]
	if !ok || d.OwnerID != ownerID {
		return ports.ErrDocumentNotFound
	}
	d.Archived = archived
	if archived {
		now := time.Now()
		d.ArchivedAt = &now
		d.Pinned = false
	} else {
		d.ArchivedAt = nil
	}
	s.m[id] = d
	return nil
}
```

- [ ] **Step 3: Write the failing test**

`internal/adapter/pgstore/documents_test.go` (mirror `TestDocumentStore_SetPinned`):
```go
func TestDocumentStore_SetArchived(t *testing.T) {
	ds, cleanup := newTestDocumentStore(t)
	defer cleanup()
	ctx := context.Background()
	seedUser(t, ds, "u1")
	if _, err := ds.Create(ctx, domain.Document{
		ID: "d1", OwnerID: "u1", Type: domain.DocMemory, Path: "m1", Title: "M1",
		Pinned: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := ds.SetArchived(ctx, "u1", "d1", true); err != nil {
		t.Fatal(err)
	}
	got, _ := ds.Get(ctx, "u1", "d1")
	if !got.Archived || got.ArchivedAt == nil {
		t.Fatalf("archived/archived_at not set: %+v", got)
	}
	if got.Pinned {
		t.Fatalf("archiving must clear pinned: %+v", got)
	}
	if err := ds.SetArchived(ctx, "u1", "d1", false); err != nil {
		t.Fatal(err)
	}
	got, _ = ds.Get(ctx, "u1", "d1")
	if got.Archived || got.ArchivedAt != nil {
		t.Fatalf("un-archive must clear: %+v", got)
	}
}
```

- [ ] **Step 4: Run it — expect FAIL** (`SetArchived` undefined on `*DocumentStore`).

- [ ] **Step 5: Implement pgstore (mirror SetPinned at :200)**

`internal/adapter/pgstore/documents.go`, after `SetPinned` (line 209):
```go
func (s *DocumentStore) SetArchived(ctx context.Context, ownerID, id string, archived bool) error {
	ct, err := s.pool.Exec(ctx, `UPDATE documents SET archived=$1, archived_at = CASE WHEN $1 THEN now() ELSE NULL END, pinned = CASE WHEN $1 THEN false ELSE pinned END, updated_at=now() WHERE owner_id=$2 AND id=$3`, archived, ownerID, id)
	if err != nil {
		return fmt.Errorf("pgstore: set archived: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ports.ErrDocumentNotFound
	}
	return nil
}
```

- [ ] **Step 6: Run it — expect PASS.** Run: `go test ./internal/adapter/pgstore/ -run TestDocumentStore_SetArchived`.

- [ ] **Step 7: Commit**
```bash
git add internal/ports/ports.go internal/adapter/pgstore/documents.go internal/testutil/fakes.go internal/adapter/pgstore/documents_test.go
git commit -m "feat(querschnitt-a): SetArchived store method (stamps archived_at, clears pinned)"
```

---

### Task 3: Read-path exclusion (5 queries + compose)

**Files:**
- Modify: `internal/adapter/pgstore/documents.go` — `List` (:125), `ListPage` (:143-144), `Search` (:317), `SemanticSearch` (:432), `ListForContext` (:234)
- Modify: `internal/testutil/fakes.go` — `List` (:640), `Search` (:783), `SemanticSearch` (:901), `ListForContext` (:973-984)
- Test: `internal/adapter/pgstore/documents_test.go`, `internal/usecase/compose_context_test.go`

**Interfaces:**
- Consumes: `domain.Document.Archived` (Task 1).
- Produces: archived docs are invisible to `List`/`ListPage`/`Search`/`SemanticSearch`/`ListForContext` (and thus `ComposeContext`).

- [ ] **Step 1: Write the failing tests**

pgstore — `internal/adapter/pgstore/documents_test.go`:
```go
func TestDocumentStore_ArchivedExcludedFromReads(t *testing.T) {
	ds, cleanup := newTestDocumentStore(t)
	defer cleanup()
	ctx := context.Background()
	seedUser(t, ds, "u1")
	mk := func(id string, archived bool) {
		if _, err := ds.Create(ctx, domain.Document{
			ID: id, OwnerID: "u1", Type: domain.DocMemory, Path: id, Title: "needle " + id,
			Body: "needle body", Archived: archived, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	mk("live", false)
	mk("arch", true)

	list, _ := ds.List(ctx, "u1", nil)
	if containsID(list, "arch") || !containsID(list, "live") {
		t.Fatalf("List must exclude archived: %v", ids(list))
	}
	hits, _ := ds.Search(ctx, "u1", "needle", nil, nil)
	for _, h := range hits {
		if h.Document.ID == "arch" {
			t.Fatalf("Search must exclude archived")
		}
	}
	ctxDocs, _ := ds.ListForContext(ctx, "u1", nil, true, []domain.DocumentType{domain.DocMemory})
	if containsID(ctxDocs, "arch") {
		t.Fatalf("ListForContext must exclude archived")
	}
}
```
(`containsID`/`ids` helpers: add small local helpers if not present in the test file.)

compose — `internal/usecase/compose_context_test.go` (mirror an existing ComposeContext test's setup):
```go
func TestComposeContext_ExcludesArchived(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	// seed one archived global memory + one live one, then compose for any repo.
	_, _, _ = docs.UpsertByPath(context.Background(), "u1", nil, domain.DocMemory, "live", "Live", "x", false, false)
	id, _, _ := docs.UpsertByPath(context.Background(), "u1", nil, domain.DocMemory, "old", "Old", "x", false, false)
	_ = docs.SetArchived(context.Background(), "u1", id, true)
	uc := /* construct ComposeContext as the existing tests do */
	res, err := uc.Execute(/* ownerID u1, no repo */)
	if err != nil { t.Fatal(err) }
	if composeMentions(res, "Old") {
		t.Fatalf("archived memory must not appear in compose")
	}
}
```
(Adapt constructor + result-introspection to the existing `compose_context_test.go` style. `UpsertByPath` here takes the new `archived` param from Task 6 — if Task 6 not yet merged, seed via `Create` + `SetArchived` instead.)

- [ ] **Step 2: Run them — expect FAIL** (archived doc leaks into reads).

- [ ] **Step 3: pgstore — add the filter to each query**

- `List` (line 125): change `WHERE owner_id=$1` → `WHERE owner_id=$1 AND NOT archived`.
- `ListPage` (line 143-144): same `AND NOT archived` on its `WHERE owner_id=$1`.
- `Search` (line 317): `WHERE d.owner_id = $1` → `… AND NOT d.archived`.
- `SemanticSearch` (line 432): add `AND NOT d.archived` to the OUTER select's WHERE (the one that joins back to `documents d`).
- `ListForContext` (line 234): `q := \`SELECT \` + docCols + \` FROM documents WHERE owner_id=$1 AND type = ANY($2) AND NOT archived\``.

- [ ] **Step 4: fake — skip archived in each read**

`internal/testutil/fakes.go`:
- `List` (line 640): `if d.OwnerID == ownerID && !d.Archived && matchesNode(...) && hasAllTags(...)`.
- `Search` (line 783): add `|| d.Archived` to the skip `continue` condition.
- `SemanticSearch` (line 901): add `|| d.Archived` to the skip `continue` condition.
- `ListForContext` (line 974): change `if d.OwnerID != ownerID || !inTypes[d.Type] {` → `if d.OwnerID != ownerID || d.Archived || !inTypes[d.Type] {`.
(`ListPage` delegates to `List`, so it's covered.)

- [ ] **Step 5: Run them — expect PASS.** Run: `go test ./internal/adapter/pgstore/ ./internal/usecase/ -run 'Archived|ExcludesArchived'`.

- [ ] **Step 6: Commit**
```bash
git add internal/adapter/pgstore/documents.go internal/testutil/fakes.go internal/adapter/pgstore/documents_test.go internal/usecase/compose_context_test.go
git commit -m "feat(querschnitt-a): exclude archived from list/search/compose (SQL-level)"
```

---

### Task 4: `ListArchived` findability

**Files:**
- Modify: `internal/ports/ports.go` (DocumentStore)
- Modify: `internal/adapter/pgstore/documents.go`
- Modify: `internal/testutil/fakes.go`
- Test: `internal/adapter/pgstore/documents_test.go`

**Interfaces:**
- Produces: `ports.DocumentStore.ListArchived(ctx, ownerID string) ([]domain.Document, error)` — archived docs only, newest `archived_at` first.

- [ ] **Step 1: Add the port method** (after `SetArchived`):
```go
	// ListArchived returns the owner's archived documents, newest archived_at first.
	ListArchived(ctx context.Context, ownerID string) ([]domain.Document, error)
```

- [ ] **Step 2: Fake**:
```go
func (s *FakeDocumentStore) ListArchived(_ context.Context, ownerID string) ([]domain.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Document
	for _, d := range s.m {
		if d.OwnerID == ownerID && d.Archived {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		var ai, aj time.Time
		if out[i].ArchivedAt != nil {
			ai = *out[i].ArchivedAt
		}
		if out[j].ArchivedAt != nil {
			aj = *out[j].ArchivedAt
		}
		return ai.After(aj)
	})
	return out, nil
}
```

- [ ] **Step 3: Failing test**:
```go
func TestDocumentStore_ListArchived(t *testing.T) {
	ds, cleanup := newTestDocumentStore(t)
	defer cleanup()
	ctx := context.Background()
	seedUser(t, ds, "u1")
	_, _ = ds.Create(ctx, domain.Document{ID: "live", OwnerID: "u1", Type: domain.DocMemory, Path: "live", Title: "L", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	_, _ = ds.Create(ctx, domain.Document{ID: "arch", OwnerID: "u1", Type: domain.DocMemory, Path: "arch", Title: "A", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	_ = ds.SetArchived(ctx, "u1", "arch", true)
	got, err := ds.ListArchived(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "arch" {
		t.Fatalf("want [arch], got %v", ids(got))
	}
}
```

- [ ] **Step 4: Run — expect FAIL.**

- [ ] **Step 5: pgstore impl**:
```go
func (s *DocumentStore) ListArchived(ctx context.Context, ownerID string) ([]domain.Document, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+docCols+` FROM documents WHERE owner_id=$1 AND archived ORDER BY archived_at DESC NULLS LAST`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list archived: %w", err)
	}
	defer rows.Close()
	return scanDocuments(rows)
}
```

- [ ] **Step 6: Run — expect PASS.**

- [ ] **Step 7: Commit**
```bash
git add internal/ports/ports.go internal/adapter/pgstore/documents.go internal/testutil/fakes.go internal/adapter/pgstore/documents_test.go
git commit -m "feat(querschnitt-a): ListArchived store query (findability)"
```

---

### Task 5: `SetArchived` usecase delegator

**Files:**
- Create: `internal/usecase/set_archived.go`
- Test: `internal/usecase/set_archived_test.go`

**Interfaces:**
- Produces: `usecase.SetArchived{Docs ports.DocumentStore}` with `Execute(ctx, ownerID, id string, archived bool) error`.

- [ ] **Step 1: Failing test** (mirror `set_pinned_test.go`):
```go
package usecase_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestSetArchived(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	_, _ = docs.Create(context.Background(), domain.Document{ID: "d1", OwnerID: "u1", Type: domain.DocMemory, Path: "p", Title: "T"})
	uc := usecase.SetArchived{Docs: docs}
	if err := uc.Execute(context.Background(), "u1", "d1", true); err != nil {
		t.Fatal(err)
	}
	got, _ := docs.Get(context.Background(), "u1", "d1")
	if !got.Archived {
		t.Fatalf("not archived: %+v", got)
	}
}
```

- [ ] **Step 2: Run — expect FAIL.**

- [ ] **Step 3: Implement** `internal/usecase/set_archived.go` (verbatim mirror of `set_pinned.go`):
```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/ports"
)

// SetArchived archives or un-archives a document.
type SetArchived struct{ Docs ports.DocumentStore }

func (uc SetArchived) Execute(ctx context.Context, ownerID, id string, archived bool) error {
	return uc.Docs.SetArchived(ctx, ownerID, id, archived)
}
```

- [ ] **Step 4: Run — expect PASS.**

- [ ] **Step 5: Commit**
```bash
git add internal/usecase/set_archived.go internal/usecase/set_archived_test.go
git commit -m "feat(querschnitt-a): SetArchived usecase"
```

---

### Task 6: `UpsertByPath` archived param + idempotent reclassify

**Files:**
- Modify: `internal/ports/ports.go:200-205` (`UpsertByPath` signature)
- Modify: `internal/adapter/pgstore/documents.go:211-226` (`UpsertByPath`)
- Modify: `internal/testutil/fakes.go:937-959` (`UpsertByPath`)
- Modify: `internal/usecase/upsert_document_by_path.go` (`UpsertByPathInput`, `Execute`)
- Test: `internal/usecase/upsert_document_by_path_test.go`

**Interfaces:**
- Consumes: `SetArchived` (Task 2/5).
- Produces: `UpsertByPath(..., pinned, archived bool)`; `usecase.UpsertByPathInput.Archived bool`. Upsert sets `archived` on INSERT and re-applies it via `SetArchived` after (so re-runs reclassify, mirroring `pinned`).

- [ ] **Step 1: Failing test** (`internal/usecase/upsert_document_by_path_test.go`, mirror the existing pinned assertion):
```go
func TestUpsertDocumentByPath_Archived(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	uc := usecase.UpsertDocumentByPath{Docs: docs /*, + same deps the existing test wires */}
	id, err := uc.Execute(context.Background(), "u1", usecase.UpsertByPathInput{
		Type: string(domain.DocMemory), Path: "m1", Title: "M1", Body: "b", Archived: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := docs.Get(context.Background(), "u1", id)
	if !got.Archived {
		t.Fatalf("upsert did not set archived: %+v", got)
	}
	// re-run un-archived → reclassifies
	if _, err := uc.Execute(context.Background(), "u1", usecase.UpsertByPathInput{
		Type: string(domain.DocMemory), Path: "m1", Title: "M1", Body: "b", Archived: false,
	}); err != nil {
		t.Fatal(err)
	}
	got, _ = docs.Get(context.Background(), "u1", id)
	if got.Archived {
		t.Fatalf("re-run did not reclassify: %+v", got)
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`UpsertByPathInput` has no `Archived`).

- [ ] **Step 3: Port signature** — `internal/ports/ports.go`, add `archived bool` after `pinned bool` in `UpsertByPath`.

- [ ] **Step 4: pgstore `UpsertByPath`** — add the column + param:
```go
func (s *DocumentStore) UpsertByPath(ctx context.Context, ownerID string, nodeID *string, typ domain.DocumentType, path, title, body string, pinned, archived bool) (string, time.Time, error) {
	id := s.ids.NewID()
	const q = `
INSERT INTO documents (id, owner_id, node_id, type, path, title, body, extra, pinned, archived, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,'{}',$8,$9,now(),now())
ON CONFLICT (owner_id, coalesce(node_id, ''), path)
DO UPDATE SET title = EXCLUDED.title, body = EXCLUDED.body, type = EXCLUDED.type, updated_at = now()
RETURNING id, updated_at`
	var gotID string
	var updated time.Time
	err := s.pool.QueryRow(ctx, q, id, ownerID, nodeID, string(typ), path, title, body, pinned, archived).Scan(&gotID, &updated)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("pgstore: upsert by path: %w", err)
	}
	return gotID, updated, nil
}
```
(`ON CONFLICT` still does NOT touch `archived` — like `pinned`; the usecase re-applies it.)

- [ ] **Step 5: Fake `UpsertByPath`** — update signature `(…, pinned, archived bool)`; on the conflict branch leave `Archived` untouched (preserve, like pinned); on the new-insert branch set `Archived: archived` in the `domain.Document{…}` literal (line 957).

- [ ] **Step 6: Usecase** — `internal/usecase/upsert_document_by_path.go`:
  - add `Archived bool` to `UpsertByPathInput` (after `Pinned`; no json tag — the REST handler maps fields explicitly).
  - pass `in.Archived` as the new last arg to `uc.Docs.UpsertByPath(...)` (line 38).
  - after the existing explicit `SetPinned` block (line 51-53), add:
```go
	if err := uc.Docs.SetArchived(ctx, ownerID, id, in.Archived); err != nil {
		return "", fmt.Errorf("upsert by path: set archived: %w", err)
	}
```
(match the exact return shape of the neighbouring `SetPinned` error path.)

- [ ] **Step 7: REST request mapping** — the server decodes the upsert body into its own `upsertByPathReq` struct, so `archived` must be threaded there too. `internal/adapter/httpserver/documents.go`:
  - add `Archived bool \`json:"archived"\`` to `upsertByPathReq` (line 214, after `Pinned`).
  - in `handleUpsertByPath` (line 224-227), add `Archived: req.Archived,` to the `usecase.UpsertByPathInput{...}` literal.

- [ ] **Step 8: Run — expect PASS.** Run: `go test ./internal/usecase/ -run TestUpsertDocumentByPath_Archived` and `go build ./...`. Note: `cmd/flow/context_migrate.go` uses the **apiclient** `UpsertByPathInput` (unchanged until Task 8), so it does NOT break here. Any remaining **direct** callers of the store/fake `UpsertByPath` (e.g. existing tests) need the extra `archived` arg — add `false` where archived is irrelevant.

- [ ] **Step 9: Commit**
```bash
git add internal/ports/ports.go internal/adapter/pgstore/documents.go internal/testutil/fakes.go internal/usecase/upsert_document_by_path.go internal/usecase/upsert_document_by_path_test.go internal/adapter/httpserver/documents.go
git commit -m "feat(querschnitt-a): UpsertByPath carries archived (idempotent reclassify)"
```

---

### Task 7: REST `POST /documents/{id}/archive` + server/main wiring

**Files:**
- Modify: `internal/adapter/httpserver/documents.go` (after `handlePinDocument`, line 261)
- Modify: `internal/adapter/httpserver/server.go:87` (struct field), `:167` (route)
- Modify: `cmd/flow-server/main.go:174` (wire usecase)
- Modify: `internal/adapter/httpserver/documents_test.go:62` (test server wiring)
- Test: `internal/adapter/httpserver/documents_test.go`

**Interfaces:**
- Consumes: `usecase.SetArchived` (Task 5).
- Produces: `POST /api/v1/documents/{id}/archive` body `{"archived": bool}` → `204` + `document.updated` SSE; `404` on unknown id.

- [ ] **Step 1: Failing test** (mirror the pin handler test in `documents_test.go`):
```go
func TestServer_ArchiveDocument(t *testing.T) {
	srv, docs, bus := newTestServer(t) // same constructor the pin test uses
	_, _ = docs.Create(context.Background(), domain.Document{ID: "d1", OwnerID: testUserID, Type: domain.DocMemory, Path: "p", Title: "T"})
	req := httptest.NewRequest("POST", "/api/v1/documents/d1/archive", strings.NewReader(`{"archived":true}`))
	req = withTestUser(req) // same auth helper the pin test uses
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", rr.Code)
	}
	got, _ := docs.Get(context.Background(), testUserID, "d1")
	if !got.Archived {
		t.Fatalf("not archived")
	}
	assertPublished(t, bus, domain.EventDocumentUpdated) // same bus-assert helper neighbours use
}
```

- [ ] **Step 2: Run — expect FAIL.**

- [ ] **Step 3: Handler** — `internal/adapter/httpserver/documents.go`, after line 261 (mirror `handlePinDocument`):
```go
type archiveReq struct {
	Archived bool `json:"archived"`
}

func (s *Server) handleArchiveDocument(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req archiveReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")
	switch err := s.SetArchived.Execute(r.Context(), u.ID, id, req.Archived); {
	case errors.Is(err, ports.ErrDocumentNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		s.Bus.Publish(domain.Event{Type: domain.EventDocumentUpdated, UserID: u.ID, Data: map[string]any{"id": id}})
		w.WriteHeader(http.StatusNoContent)
	}
}
```

- [ ] **Step 4: Server struct + route** — `server.go`: add `SetArchived usecase.SetArchived` after line 87 (`SetPinned`); add after line 167:
```go
	mux.Handle("POST /api/v1/documents/{id}/archive", s.auth(http.HandlerFunc(s.handleArchiveDocument)))
```

- [ ] **Step 5: Wire usecase** — `cmd/flow-server/main.go`, after line 174:
```go
		SetArchived:   usecase.SetArchived{Docs: documentStore},
```
And `internal/adapter/httpserver/documents_test.go:62` test server literal: add `SetArchived: usecase.SetArchived{Docs: docs},`.

- [ ] **Step 6: Run — expect PASS.** Run: `go test ./internal/adapter/httpserver/ -run TestServer_ArchiveDocument`.

- [ ] **Step 7: Commit**
```bash
git add internal/adapter/httpserver/documents.go internal/adapter/httpserver/server.go cmd/flow-server/main.go internal/adapter/httpserver/documents_test.go
git commit -m "feat(querschnitt-a): POST /documents/{id}/archive + server/main wiring"
```

---

### Task 8: apiclient `SetArchived` + `UpsertByPathInput.Archived`

**Files:**
- Modify: `internal/adapter/apiclient/context.go:69-72` (after `SetPinned`), `:94` (`UpsertByPathInput`)
- Test: `internal/adapter/apiclient/context_test.go`

**Interfaces:**
- Produces: `(*apiclient.Client).SetArchived(ctx, id string, archived bool) error`; `apiclient.UpsertByPathInput.Archived bool`.

- [ ] **Step 1: Failing test** (mirror `TestClient_SetPinned`, line 69):
```go
func TestClient_SetArchived(t *testing.T) {
	var gotPath, gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	c := newTestClient(ts.URL) // same helper the pin test uses
	if err := c.SetArchived(context.Background(), "doc-42", true); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/documents/doc-42/archive" {
		t.Fatalf("path: %s", gotPath)
	}
	if !strings.Contains(gotBody, `"archived":true`) {
		t.Fatalf("body: %s", gotBody)
	}
}
```

- [ ] **Step 2: Run — expect FAIL.**

- [ ] **Step 3: Implement** — `internal/adapter/apiclient/context.go`, after line 72:
```go
// SetArchived calls POST /api/v1/documents/{id}/archive to archive or un-archive a document.
func (c *Client) SetArchived(ctx context.Context, id string, archived bool) error {
	return c.do(ctx, http.MethodPost, "/api/v1/documents/"+id+"/archive", map[string]bool{"archived": archived}, nil)
}
```
And add to `UpsertByPathInput` (line 94, after `Pinned`): `Archived bool \`json:"archived"\``.

- [ ] **Step 4: Run — expect PASS.**

- [ ] **Step 5: Commit**
```bash
git add internal/adapter/apiclient/context.go internal/adapter/apiclient/context_test.go
git commit -m "feat(querschnitt-a): apiclient SetArchived + UpsertByPathInput.Archived"
```

---

### Task 9: MCP tool `flow_archive_doc`

**Files:**
- Modify: `cmd/flow-mcp/tools_write.go` (new `archiveDocIn` + `archiveDoc` handler)
- Modify: `cmd/flow-mcp/server.go:104` (register the 14th tool, before `return s, h`)

**Interfaces:**
- Consumes: `apiclient.SetArchived` (Task 8).
- Produces: MCP tool `flow_archive_doc` `{id, archived?}` (archived defaults true).

> MCP tool handlers have no unit-test harness (they need a live authed client); this task is verified by the manual round-trip in Task 12. Keep the handler a thin mirror of `deleteDoc`.

- [ ] **Step 1: Input + handler** — `cmd/flow-mcp/tools_write.go`, after `deleteDoc` (line 131):
```go
type archiveDocIn struct {
	ID       string `json:"id" jsonschema:"the document id to archive or un-archive"`
	Archived *bool  `json:"archived,omitempty" jsonschema:"true (default) to archive — out of bootstrap + default lists/search but still findable; false to un-archive"`
}

func (h *handlers) archiveDoc(ctx context.Context, _ *mcp.CallToolRequest, in archiveDocIn) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.ID) == "" {
		return errorResult("id is required"), nil, nil
	}
	archived := true
	if in.Archived != nil {
		archived = *in.Archived
	}
	var out string
	err := h.mgr.Do(ctx, func(c *apiclient.Client) error {
		cur, err := c.GetDocument(ctx, in.ID)
		if err != nil {
			return err
		}
		if err := c.SetArchived(ctx, in.ID, archived); err != nil {
			return err
		}
		if archived {
			h.removeResource(cur.ID)
			out = fmt.Sprintf("Archived [%s] %s.", cur.ID, cur.Title)
		} else {
			h.addResource(ctx, cur)
			out = fmt.Sprintf("Un-archived [%s] %s.", cur.ID, cur.Title)
		}
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}
```

- [ ] **Step 2: Register the tool** — `cmd/flow-mcp/server.go`, after the `flow_set_active_context` block (line 104), before `return s, h`:
```go
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_archive_doc",
		Description: "Archive a context doc (out of bootstrap + default lists/search, but findable + reversible) or un-archive it. Safe, reversible — use this to retire done/historical memories instead of deleting them.",
	}, h.archiveDoc)
```

- [ ] **Step 3: Build** — `go build ./cmd/flow-mcp/` → PASS.

- [ ] **Step 4: Commit**
```bash
git add cmd/flow-mcp/tools_write.go cmd/flow-mcp/server.go
git commit -m "feat(querschnitt-a): MCP flow_archive_doc tool"
```

---

### Task 10: CLI — `flow context migrate memories` archived disposition

**Files:**
- Modify: `cmd/flow/context_migrate.go` (`manifestRow`, `parseManifest`, `memoryDoc`, `deriveMemoryDoc`, `runMigrateMemories`)
- Test: `cmd/flow/context_migrate_test.go`

**Interfaces:**
- Consumes: `apiclient.UpsertByPathInput.Archived` (Task 8).
- Produces: manifest gains optional 6th column `archived` (`y`/`-`); imported docs land archived when set.

- [ ] **Step 1: Failing test** (mirror the existing `parseManifest` test):
```go
func TestParseManifest_ArchivedColumn(t *testing.T) {
	in := "file\tscope\ttags\tpin\tkeep\tarchived\n" +
		"a_done.md\tglobal\t\t-\ty\ty\n" +
		"b.md\tglobal\t\t-\ty\t-\n" +
		"c.md\tglobal\t\t-\ty\n" // 5-col legacy row → archived defaults false
	rows, err := parseManifest(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if !rows[0].Archived || rows[1].Archived || rows[2].Archived {
		t.Fatalf("archived parse: %+v", rows)
	}
}
```

- [ ] **Step 2: Run — expect FAIL.**

- [ ] **Step 3: Implement** — `cmd/flow/context_migrate.go`:
  - `manifestRow`: add `Archived bool` after `Keep`.
  - `parseManifest` (line 93-99): after building the row, set archived from the optional 6th column (the `len(f) < 5` guard stays — 6th is optional):
```go
			row := manifestRow{
				File:  strings.TrimSpace(f[0]),
				Scope: strings.TrimSpace(f[1]),
				Tags:  tags,
				Pin:   strings.TrimSpace(f[3]) == "y",
				Keep:  strings.TrimSpace(f[4]) != "skip",
			}
			if len(f) >= 6 {
				row.Archived = strings.TrimSpace(f[5]) == "y"
			}
			rows = append(rows, row)
```
  - `memoryDoc`: add `Archived bool`.
  - `deriveMemoryDoc` (line 129): return `…, Archived: row.Pin /*→*/` — set `Archived: row.Archived`. (Guard: if both `Pin` and `Archived` are set, archived wins — clear pin: `pin := row.Pin && !row.Archived`, and return `Pinned: pin, Archived: row.Archived`.)
  - `runMigrateMemories`: the dry-run print (line 205) append ` archived=%v`, doc.Archived; the upsert call (line 209-211) add `Archived: doc.Archived`.

- [ ] **Step 4: Run — expect PASS.**

- [ ] **Step 5: Commit**
```bash
git add cmd/flow/context_migrate.go cmd/flow/context_migrate_test.go
git commit -m "feat(querschnitt-a): migrate-memories manifest archived disposition"
```

---

### Task 11: CLI — `flow context archive` + `flow context archived`

**Files:**
- Create: `cmd/flow/context_archive.go`
- Modify: `cmd/flow/context.go:35` (register subcommands)
- Test: `cmd/flow/context_archive_test.go`

**Interfaces:**
- Consumes: `apiclient.SetArchived` (Task 8); `apiclient` list/get for path→id resolution and the archived listing (the apiclient method backing `ListArchived` — add `(*Client).ListArchived(ctx) ([]domain.Document, error)` hitting a `GET /api/v1/documents?archived=1` style endpoint **OR**, to avoid a new endpoint in A-Kern, reuse the existing documents list with a client-side `Archived` filter; pick the existing-list reuse to keep scope tight and note it).
- Produces: `flow context archive --from <tsv> [--dry-run]` (bulk apply) and `flow context archive --candidates` (emit review TSV); `flow context archived` (list).

> **Decision for the implementer:** `flow context archived` needs to *read* archived docs, but every list endpoint excludes archived (Task 3). Rather than thread an `includeArchived` flag through every layer (no precedent — spec §7), add ONE narrow read path: `GET /api/v1/documents/archived` → `s.ListArchived` usecase → `Docs.ListArchived` (Task 4) → `apiclient.ListArchived`. This is the dedicated findability surface from the spec. Add this endpoint+usecase+apiclient method as Step 0 below (small, mirrors `handleListDocuments`).

- [ ] **Step 0: Add the archived read path**
  - usecase `internal/usecase/list_archived.go`: `ListArchived{Docs ports.DocumentStore}` with `Execute(ctx, ownerID) ([]domain.Document, error)` delegating to `Docs.ListArchived`.
  - REST: `handleListArchived` in `documents.go` (mirror `handleListDocuments` — auth, call `s.ListArchived.Execute`, `writeJSON` the slice); field `ListArchived usecase.ListArchived` on `Server`; route `mux.Handle("GET /api/v1/documents/archived", s.auth(http.HandlerFunc(s.handleListArchived)))` **before** the `GET /api/v1/documents/{id}` wildcard (line 163) so the static path wins; wire in `main.go` + test server.
  - apiclient: `(*Client).ListArchived(ctx) ([]domain.Document, error)` → `GET /api/v1/documents/archived`.

- [ ] **Step 1: Failing test** (`cmd/flow/context_archive_test.go`) — drive the apply path against a fake/stub client:
```go
func TestContextArchive_ApplyFromTSV(t *testing.T) {
	calls := map[string]bool{}
	stub := &stubArchiveClient{ // implements the small interface runArchive needs
		resolve: func(path string) (string, error) { return "id-" + path, nil },
		setArchived: func(id string, a bool) error { calls[id] = a; return nil },
	}
	tsv := "path\tarchive\nm_done\ty\nm_keep\tn\n"
	if err := runArchive(context.Background(), io.Discard, stub, strings.NewReader(tsv), false); err != nil {
		t.Fatal(err)
	}
	if !calls["id-m_done"] || calls["id-m_keep"] {
		t.Fatalf("apply: %+v", calls)
	}
}
```
(Define a tiny `archiveClient` interface in `context_archive.go` — `resolvePath`, `SetArchived` — so the command is unit-testable without a live server, matching how other CLI commands isolate the client.)

- [ ] **Step 2: Run — expect FAIL.**

- [ ] **Step 3: Implement** `cmd/flow/context_archive.go`:
  - `contextArchiveCmd()` (cobra): flags `--from <tsv>`, `--dry-run`, `--candidates`. With `--candidates`: fetch `c.ListDocuments` filtered to `type=memory` whose Title/Body contains "DONE" (case-insensitive) or whose path ends `_done`, and print TSV `path\ttitle\tarchive` with `archive=y`. With `--from`: parse `path\tarchive` rows, resolve each path → id (via list/get), call `SetArchived(id, archive=="y")`; `--dry-run` prints "would archive N" without writing.
  - `contextArchivedCmd()` (cobra): call `c.ListArchived(ctx)`, print `path · title · archived_at`.
  - `runArchive(ctx, out, client archiveClient, tsv io.Reader, dryRun bool) error`: the testable core.

- [ ] **Step 4: Register** — `cmd/flow/context.go:35`:
```go
	cmd.AddCommand(installHooksCmd(), flushCheckCmd(), contextMigrateCmd(), contextArchiveCmd(), contextArchivedCmd())
```

- [ ] **Step 5: Run — expect PASS.** Run: `go test ./cmd/flow/ -run TestContextArchive`.

- [ ] **Step 6: Commit**
```bash
git add cmd/flow/context_archive.go cmd/flow/context.go cmd/flow/context_archive_test.go internal/usecase/list_archived.go internal/adapter/httpserver/documents.go internal/adapter/httpserver/server.go internal/adapter/apiclient/context.go cmd/flow-server/main.go internal/adapter/httpserver/documents_test.go
git commit -m "feat(querschnitt-a): flow context archive/archived CLI + archived read path"
```

---

### Task 12: Wiring verification + done-gate

**Files:** none new — this task verifies end-to-end wiring (per the project rule that every multi-task plan ends with an explicit main-wiring + curl-smoke task).

- [ ] **Step 1: Full CI**

Run: `make ci`. Expect green (gofumpt, staticcheck, templ, build, all tests incl. pgstore Docker, coverage gate held). Fix anything red.

- [ ] **Step 2: Confirm composition root wiring**

Grep that the usecases reach the server and MCP:
```bash
rg -n "SetArchived|ListArchived|flow_archive_doc" cmd/flow-server/main.go cmd/flow-mcp/server.go internal/adapter/httpserver/server.go
```
Expect: `SetArchived` + `ListArchived` fields wired in `main.go` and registered on the `Server` struct + routes; `flow_archive_doc` registered in the MCP server. (If a field is declared but never assigned in `main.go`, that's the exact bug this task exists to catch.)

- [ ] **Step 3: curl-smoke vs Postgres+Dex**

Bring up dev (`FLOW_DEV=1`, `make dev-up` + `make dev-run`, token via `make dev-token`). Then:
- `POST /api/v1/documents/{id}/archive {"archived":true}` → `204`; the doc disappears from `GET /api/v1/documents`, from search (`?q=`), and from `flow context` (compose); appears in `GET /api/v1/documents/archived`.
- `POST …/archive {"archived":false}` → doc returns; verify a previously-pinned doc has `pinned=false` after the archive round-trip.
- Confirm `document.updated` arrives on `GET /api/v1/events` (SSE) for the archive call.

- [ ] **Step 4: MCP round-trip**

With the MCP server running against the dev backend, call `flow_archive_doc {id, archived:true}` then `{archived:false}`; confirm the textResult and that the doc leaves/returns to `flow_get_context`.

- [ ] **Step 5: Bulk-pass real (the headline win)**

```bash
flow context archive --candidates > /tmp/cand.tsv   # heuristic seed of done-milestones
# review: keep a few as archive=n
flow context archive --from /tmp/cand.tsv --dry-run  # "would archive N"
flow context archive --from /tmp/cand.tsv            # apply
flow context                                          # Used-token count drops; the previously
                                                      # silently-capped leaf memories are gone from candidates
```
Compare the `flow context` footer before/after against the cap+rank baseline (≈12k / "+~65 leaf not shown"): the archived done-milestones should no longer be candidates, so genuinely-active memories surface instead of being dropped.

- [ ] **Step 6: Final commit (if any fixups)**
```bash
git add -A
git commit -m "test(querschnitt-a): done-gate — wiring + curl-smoke + bulk-pass verified"
```

---

## Self-Review (completed during planning)

- **Spec coverage:** §1 migration → T1; §2 mechanism/exclusion → T1/T3; §3 archived×pinned → T2; §4 surfaces (REST/apiclient/MCP/CLI) → T7/T8/T9/T11; §5 import+bulk → T10/T11; §6 provenance `archived_at` → T1 (note: `superseded_by` deferred per Global Constraints); §7 findability → T4/T11; §8 tests → each task; §9 done-gate → T12; §10 touch-points → all tasks.
- **Deviation flagged:** `superseded_by` deferred (see Global Constraints) — surfaced for approval.
- **Type consistency:** `SetArchived(ctx, ownerID, id, archived bool)`, `ListArchived(ctx, ownerID)`, `UpsertByPath(…, pinned, archived bool)`, `UpsertByPathInput.Archived`, `domain.Document.Archived/ArchivedAt` — used identically across tasks.
