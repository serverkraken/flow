# flow-mcp Slice 1 (Backend Foundation) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the four agent-owned document types and server-side project scoping (`projectId`) to the document list/search read path, so the upcoming flow-mcp server (Slice 2) can read/write project-scoped Kompendium documents.

**Architecture:** Thin, additive backend change. New `DocumentType` constants (no migration — `type` is a TEXT column). An optional `projectID *string` filter threaded through `ports.DocumentStore.{List,Search,SemanticSearch}` → pgstore SQL, the two read use cases, and the REST `GET /api/v1/documents` handler. The Go `apiclient` gains additive `*Scoped` variants so existing callers are untouched. Semantic search must scope server-side (post-filtering top-K vector hits client-side is broken), so the vector arm gets the same predicate.

**Tech Stack:** Go 1.25.7, pgx/v5 + Postgres (pgstore), testcontainers-go (pgstore Docker tests), net/http + stdlib mux (httpserver), `go test`.

## Global Constraints

- Go module `github.com/serverkraken/flow`; Go 1.25.7.
- **No database migration** — `documents.type` is a free TEXT column and `documents.project_id` (nullable, FK → projects ON DELETE SET NULL) already exists. Domain `valid()` is the only type gate.
- **`projectID *string` filter convention** (identical at every layer): `nil` → no filter (all owner docs); pointer to the literal `"none"` → `project_id IS NULL` (unassigned); any other value → `project_id = <value>` (the value is a project **ID/UUID**, never a slug — UUIDs never equal `"none"`).
- Existing behavior must not change: every pre-existing caller passes `nil`.
- `make ci` (build + lint + race tests + coverage) must stay green at/above the current gate (~83%). pgstore tests need Docker; they self-skip when Docker is unavailable (`t.Skipf`).
- Commit message trailers (every commit):
  ```
  Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01GTHf7tSzKearihWVzL6i4n
  ```

---

### Task 1: Agent-owned document types

**Files:**
- Modify: `internal/domain/document.go:12-25`
- Test: `internal/domain/documenttype_test.go` (create)

**Interfaces:**
- Consumes: nothing.
- Produces: `domain.DocMemory`, `domain.DocInstruction`, `domain.DocSkill`, `domain.DocPlan` (all `domain.DocumentType`); `(domain.Document).Validate()` accepts them with no project/date requirement.

- [ ] **Step 1: Write the failing test**

Create `internal/domain/documenttype_test.go`:

```go
package domain

import "testing"

func TestDocumentType_AgentOwnedValid(t *testing.T) {
	for _, ty := range []DocumentType{DocMemory, DocInstruction, DocSkill, DocPlan} {
		d := Document{Type: ty, Path: "agent/note"}
		if err := d.Validate(); err != nil {
			t.Fatalf("Validate(type=%q) = %v, want nil", ty, err)
		}
	}
}

func TestDocumentType_UnknownInvalid(t *testing.T) {
	d := Document{Type: DocumentType("bogus"), Path: "x"}
	if err := d.Validate(); err == nil {
		t.Fatal("Validate(type=bogus) = nil, want error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/ -run TestDocumentType -v`
Expected: FAIL — `undefined: DocMemory` (compile error).

- [ ] **Step 3: Add the constants and extend `valid()`**

In `internal/domain/document.go`, change the const block (lines 12-17) and `valid()` (lines 19-25) to:

```go
const (
	DocDaily       DocumentType = "daily"
	DocProject     DocumentType = "project"
	DocFree        DocumentType = "free"
	DocAgent       DocumentType = "agent"
	DocMemory      DocumentType = "memory"      // agent-owned
	DocInstruction DocumentType = "instruction" // agent-owned (CLAUDE.md)
	DocSkill       DocumentType = "skill"        // agent-owned
	DocPlan        DocumentType = "plan"         // agent-owned
)

func (t DocumentType) valid() bool {
	switch t {
	case DocDaily, DocProject, DocFree, DocAgent, DocMemory, DocInstruction, DocSkill, DocPlan:
		return true
	}
	return false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/ -run TestDocumentType -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/domain/document.go internal/domain/documenttype_test.go
git commit -m "feat(domain): agent-owned document types (memory/instruction/skill/plan)" -m "Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01GTHf7tSzKearihWVzL6i4n"
```

---

### Task 2: Plumb `projectID *string` through the read path (no behavior change)

This is one atomic signature change: the port, its implementations, the use cases, the REST handler, and all call sites move together so the build stays green. pgstore/fakes **accept** `projectID` but do not yet filter on it (filtering lands in Tasks 3-4). Every pre-existing caller passes `nil`, so behavior is unchanged.

**Files:**
- Modify: `internal/ports/ports.go:133,146,161`
- Modify: `internal/adapter/pgstore/documents.go:54,152,272` (signatures only)
- Modify: `internal/testutil/fakes.go:464,561,626` (signatures only)
- Modify: `internal/usecase/backlinks_test.go:29` (signature only)
- Modify: `internal/usecase/list_documents.go`, `internal/usecase/search_documents.go`, `internal/usecase/list_tags.go`, `internal/usecase/backlinks.go`
- Modify: `internal/adapter/httpserver/documents.go:50-74`
- Modify: `internal/adapter/httpserver/webui_docs.go:118,132,214`
- Modify: pgstore test call sites of `List`/`Search`/`SemanticSearch` (insert `nil`)

**Interfaces:**
- Consumes: Task 1 types (not directly used here).
- Produces:
  - `ports.DocumentStore.List(ctx, ownerID string, projectID *string, tags ...string) ([]domain.Document, error)`
  - `ports.DocumentStore.Search(ctx, ownerID, q string, projectID *string, tags []string) ([]domain.SearchHit, error)`
  - `ports.DocumentStore.SemanticSearch(ctx, ownerID string, query []float32, projectID *string, tags []string, limit int) ([]domain.SemanticHit, error)`
  - `usecase.ListDocuments.Execute(ctx, ownerID string, projectID *string, tags []string) ([]domain.Document, error)`
  - `usecase.SearchDocuments.Execute(ctx, ownerID, q string, projectID *string, tags []string) ([]domain.SearchHit, error)`
  - REST `GET /api/v1/documents?projectId=<id|none>` parsed into the `projectID` passed to both use cases.

- [ ] **Step 1: Change the three port signatures**

In `internal/ports/ports.go`, update the `DocumentStore` interface (keep the existing doc comments, append a sentence about the `projectID` convention to each):

```go
	List(ctx context.Context, ownerID string, projectID *string, tags ...string) ([]domain.Document, error)
	// ...
	Search(ctx context.Context, ownerID, q string, projectID *string, tags []string) ([]domain.SearchHit, error)
	// ...
	SemanticSearch(ctx context.Context, ownerID string, query []float32, projectID *string, tags []string, limit int) ([]domain.SemanticHit, error)
```

- [ ] **Step 2: Update pgstore signatures (accept, do not filter yet)**

In `internal/adapter/pgstore/documents.go`, change only the three function signatures to add `projectID *string` in the position above. Leave the bodies unchanged (the parameter is unused for now — Go permits unused function parameters):
- `:54` → `func (s *DocumentStore) List(ctx context.Context, ownerID string, projectID *string, tags ...string) (...)`
- `:152` → `func (s *DocumentStore) Search(ctx context.Context, ownerID, q string, projectID *string, tags []string) (...)`
- `:272` → `func (s *DocumentStore) SemanticSearch(ctx context.Context, ownerID string, query []float32, projectID *string, tags []string, limit int) (...)`

- [ ] **Step 3: Update the fakes (accept, do not filter yet)**

In `internal/testutil/fakes.go`, change only the signatures of `FakeDocumentStore.List` (:464), `.Search` (:561), `.SemanticSearch` (:626) to add `projectID *string` in the same positions (rename the receiver-side blank as needed). Bodies unchanged.

In `internal/usecase/backlinks_test.go:29`, change `partialErrDocStore.List` to:

```go
func (s *partialErrDocStore) List(ctx context.Context, ownerID string, projectID *string, tags ...string) ([]domain.Document, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.FakeDocumentStore.List(ctx, ownerID, projectID, tags...)
}
```

- [ ] **Step 4: Update the use cases**

`internal/usecase/list_documents.go:12-14`:

```go
func (uc ListDocuments) Execute(ctx context.Context, ownerID string, projectID *string, tags []string) ([]domain.Document, error) {
	return uc.Docs.List(ctx, ownerID, projectID, tags...)
}
```

`internal/usecase/search_documents.go` — `Execute` gains `projectID` and threads it to both arms:

```go
func (uc SearchDocuments) Execute(ctx context.Context, ownerID, q string, projectID *string, tags []string) ([]domain.SearchHit, error) {
	keyword, err := uc.Docs.Search(ctx, ownerID, q, projectID, tags)
	if err != nil {
		return nil, err
	}
	if uc.Embedder == nil {
		return keyword, nil
	}
	vecs, err := uc.Embedder.Embed(ctx, []string{q})
	if err != nil || len(vecs) == 0 {
		uc.warn("semantic search degraded; keyword-only", err)
		return keyword, nil
	}
	limit := uc.Limit
	if limit <= 0 {
		limit = 50
	}
	semantic, err := uc.Docs.SemanticSearch(ctx, ownerID, vecs[0], projectID, tags, limit)
	if err != nil {
		uc.warn("semantic search failed; keyword-only", err)
		return keyword, nil
	}
	return rrfFuse(keyword, semantic, rrfK), nil
}
```

`internal/usecase/list_tags.go:14` → `docs, err := uc.Docs.List(ctx, ownerID, nil)`
`internal/usecase/backlinks.go:26` → `all, err := uc.Docs.List(ctx, ownerID, nil)`

- [ ] **Step 5: Parse `?projectId=` in the REST handler**

Replace `internal/adapter/httpserver/documents.go:50-74` (`handleListDocuments`) with:

```go
func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	tags := r.URL.Query()["tag"]
	var projectID *string
	if v := strings.TrimSpace(r.URL.Query().Get("projectId")); v != "" {
		projectID = &v
	}
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		hits, err := s.SearchDocuments.Execute(r.Context(), u.ID, q, projectID, tags)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if hits == nil {
			hits = []domain.SearchHit{}
		}
		writeJSON(w, http.StatusOK, hits)
		return
	}
	list, err := s.ListDocuments.Execute(r.Context(), u.ID, projectID, tags)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []domain.Document{}
	}
	writeJSON(w, http.StatusOK, list)
}
```

- [ ] **Step 6: Update the WebUI handler call sites (pass `nil`)**

In `internal/adapter/httpserver/webui_docs.go`, pass `nil` for `projectID`:
- `:118` → `s.SearchDocuments.Execute(r.Context(), u.ID, q, nil, active)`
- `:132` → `s.ListDocuments.Execute(r.Context(), u.ID, nil, active)`
- `:214` → `s.ListDocuments.Execute(r.Context(), u.ID, nil, nil)`

- [ ] **Step 7: Update pgstore test call sites (insert `nil`)**

Run: `rg -n "\.List\(ctx, |\.Search\(ctx, |\.SemanticSearch\(ctx, " internal/adapter/pgstore/`
For each call to the document store's `List`/`Search`/`SemanticSearch`, insert `nil` as the new `projectID` argument. Example before → after:
- `st.List(ctx, owner)` → `st.List(ctx, owner, nil)`
- `st.List(ctx, owner, "tag")` → `st.List(ctx, owner, nil, "tag")`
- `st.Search(ctx, owner, "q", nil)` → `st.Search(ctx, owner, "q", nil, nil)` (q-arg projectID then tags)
- `st.SemanticSearch(ctx, owner, vec, tags, 10)` → `st.SemanticSearch(ctx, owner, vec, nil, tags, 10)`

(Ignore matches on `ProjectStore`/`SessionStore`/`ProjectBindingStore.List` — those are different interfaces and unchanged.)

- [ ] **Step 8: Build and run the full suite**

Run: `go build ./... && go test ./...`
Expected: build OK; all tests PASS (Docker tests run or skip). If any call site was missed, the compile error names the file:line — fix and re-run.

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "refactor(documents): thread projectID *string through list/search read path" -m "Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01GTHf7tSzKearihWVzL6i4n"
```

---

### Task 3: pgstore — filter list & search by project

**Files:**
- Modify: `internal/adapter/pgstore/documents.go` (add `appendProjectFilter` helper; use it in `List`, `Search`; rewrite `SemanticSearch` predicate assembly)
- Test: `internal/adapter/pgstore/documents_projectscope_test.go` (create)

**Interfaces:**
- Consumes: the Task 2 signatures.
- Produces: `appendProjectFilter(q, col string, args *[]any, projectID *string) string` (package-private). `List`/`Search`/`SemanticSearch` now honor the `projectID` convention.

- [ ] **Step 1: Write the failing Docker tests**

Create `internal/adapter/pgstore/documents_projectscope_test.go`. Seed users/projects exactly the way the existing tests in `documents_test.go` do (`pgstore.NewPool(ctx, startPG(t))` → `pgstore.Migrate` → `pgstore.NewUserStore(pool).UpsertBySub(...)`):

```go
package pgstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
)

func strptr(s string) *string { return &s }

// vec768 returns a 768-dim unit vector with 1.0 at index i (matches the
// migration 0010 vector(768) column).
func vec768(i int) []float32 {
	v := make([]float32, 768)
	v[i] = 1
	return v
}

func seedProjectScope(t *testing.T) (st *pgstore.DocumentStore, owner, pA, pB string) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	users := pgstore.NewUserStore(pool)
	u, _ := domain.NewUser("u-scope", "sub-scope", "scopeuser", "scope@x.de", "Scope User")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	owner = "u-scope"
	ps := pgstore.NewProjectStore(pool)
	now := time.Now()
	a, err := domain.NewProject("proj-a", owner, "Alpha", "alpha", now)
	if err != nil {
		t.Fatalf("new project a: %v", err)
	}
	if _, err := ps.Create(ctx, a); err != nil {
		t.Fatalf("create a: %v", err)
	}
	b, err := domain.NewProject("proj-b", owner, "Beta", "beta", now)
	if err != nil {
		t.Fatalf("new project b: %v", err)
	}
	if _, err := ps.Create(ctx, b); err != nil {
		t.Fatalf("create b: %v", err)
	}
	st = pgstore.NewDocumentStore(pool)
	mk := func(id, path string, proj *string, ty domain.DocumentType, body string) {
		_, err := st.Create(ctx, domain.Document{
			ID: id, OwnerID: owner, ProjectID: proj, Type: ty,
			Path: path, Title: id, Body: body, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			t.Fatalf("create doc %s: %v", id, err)
		}
	}
	mk("d-a", "alpha/note", strptr("proj-a"), domain.DocProject, "alpha widget design")
	mk("d-b", "beta/note", strptr("proj-b"), domain.DocProject, "beta widget design")
	mk("d-x", "free/note", nil, domain.DocFree, "unassigned widget design")
	return st, owner, "proj-a", "proj-b"
}

func ids(docs []domain.Document) map[string]bool {
	m := map[string]bool{}
	for _, d := range docs {
		m[d.ID] = true
	}
	return m
}

func TestDocumentStore_ListProjectScope(t *testing.T) {
	ctx := context.Background()
	st, owner, pA, _ := seedProjectScope(t)

	all, err := st.List(ctx, owner, nil)
	if err != nil || len(all) != 3 {
		t.Fatalf("List(nil) = %d docs, %v; want 3", len(all), err)
	}
	only, err := st.List(ctx, owner, &pA)
	if err != nil || len(only) != 1 || only[0].ID != "d-a" {
		t.Fatalf("List(proj-a) = %v, %v; want [d-a]", ids(only), err)
	}
	none, err := st.List(ctx, owner, strptr("none"))
	if err != nil || len(none) != 1 || none[0].ID != "d-x" {
		t.Fatalf("List(none) = %v, %v; want [d-x]", ids(none), err)
	}
}

func TestDocumentStore_SearchProjectScope(t *testing.T) {
	ctx := context.Background()
	st, owner, pA, _ := seedProjectScope(t)

	all, err := st.Search(ctx, owner, "widget", nil, nil)
	if err != nil || len(all) != 3 {
		t.Fatalf("Search(nil) = %d hits, %v; want 3", len(all), err)
	}
	only, err := st.Search(ctx, owner, "widget", &pA, nil)
	if err != nil || len(only) != 1 || only[0].ID != "d-a" {
		t.Fatalf("Search(proj-a) = %d hits, %v; want [d-a]", len(only), err)
	}
}

func TestDocumentStore_SemanticSearchProjectScope(t *testing.T) {
	ctx := context.Background()
	st, owner, pA, _ := seedProjectScope(t)
	// give each doc one chunk so SemanticSearch has candidates
	if err := st.ReplaceChunks(ctx, "d-a", owner, []string{"alpha"}, [][]float32{vec768(0)}); err != nil {
		t.Fatalf("chunks d-a: %v", err)
	}
	if err := st.ReplaceChunks(ctx, "d-b", owner, []string{"beta"}, [][]float32{vec768(1)}); err != nil {
		t.Fatalf("chunks d-b: %v", err)
	}

	all, err := st.SemanticSearch(ctx, owner, vec768(0), nil, nil, 10)
	if err != nil || len(all) != 2 {
		t.Fatalf("SemanticSearch(nil) = %d hits, %v; want 2", len(all), err)
	}
	only, err := st.SemanticSearch(ctx, owner, vec768(0), &pA, nil, 10)
	if err != nil || len(only) != 1 || only[0].ID != "d-a" {
		t.Fatalf("SemanticSearch(proj-a) = %d hits, %v; want [d-a]", len(only), err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapter/pgstore/ -run ProjectScope -v`
Expected: FAIL — scoped calls return all 3 docs (filter not implemented). (If Docker is unavailable the tests skip; ensure Docker is running for this task.)

- [ ] **Step 3: Add the helper and apply it in `List` and `Search`**

In `internal/adapter/pgstore/documents.go`, add near the top (after the `prefixedDocCols` const):

```go
// appendProjectFilter adds a project predicate to q, binding the next positional
// parameter when needed. projectID == nil → no filter; *projectID == "none" →
// IS NULL (unassigned); otherwise equality. col is the (possibly qualified)
// column, e.g. "project_id" or "d.project_id".
func appendProjectFilter(q, col string, args *[]any, projectID *string) string {
	if projectID == nil {
		return q
	}
	if *projectID == "none" {
		return q + ` AND ` + col + ` IS NULL`
	}
	*args = append(*args, *projectID)
	return q + fmt.Sprintf(` AND %s = $%d`, col, len(*args))
}
```

Rewrite `List` (currently :54-76) to use it and number `tags` dynamically:

```go
func (s *DocumentStore) List(ctx context.Context, ownerID string, projectID *string, tags ...string) ([]domain.Document, error) {
	q := `SELECT ` + docCols + ` FROM documents WHERE owner_id=$1`
	args := []any{ownerID}
	q = appendProjectFilter(q, "project_id", &args, projectID)
	if len(tags) > 0 {
		args = append(args, tags)
		q += fmt.Sprintf(` AND tags @> $%d`, len(args))
	}
	q += ` ORDER BY updated_at DESC`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list documents: %w", err)
	}
	defer rows.Close()
	var out []domain.Document
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
```

In `Search` (currently :152-190), insert the project filter right after `args := []any{ownerID, q, headlineOpts}` and number `tags` dynamically:

```go
	args := []any{ownerID, q, headlineOpts}
	sb = appendProjectFilter(sb, "d.project_id", &args, projectID)
	if len(tags) > 0 {
		args = append(args, tags)
		sb += fmt.Sprintf(` AND d.tags @> $%d`, len(args))
	}
```

(Replace the old `if len(tags) > 0 { sb += ` AND d.tags @> $4`; args = append(args, tags) }` block. The trailing `$2`/`$3` references in the SELECT/ORDER BY are unchanged.)

- [ ] **Step 4: Rewrite `SemanticSearch` predicate assembly**

Replace the args/branch block inside `SemanticSearch` (currently :282-294, from `args := []any{...}` through the `else { ... }`) with a uniform predicate builder:

```go
	args := []any{ownerID, vectorLiteral(query)}
	var preds []string
	if projectID != nil {
		if *projectID == "none" {
			preds = append(preds, "d.project_id IS NULL")
		} else {
			args = append(args, *projectID)
			preds = append(preds, fmt.Sprintf("d.project_id = $%d", len(args)))
		}
	}
	if len(tags) > 0 {
		args = append(args, tags)
		preds = append(preds, fmt.Sprintf("d.tags @> $%d", len(args)))
	}
	if len(preds) > 0 {
		q += "\nWHERE " + strings.Join(preds, " AND ")
	}
	args = append(args, limit)
	q += fmt.Sprintf("\nORDER BY x.dist\nLIMIT $%d", len(args))
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/adapter/pgstore/ -run ProjectScope -v`
Expected: PASS (all three).

- [ ] **Step 6: Run the whole pgstore suite (regression)**

Run: `go test ./internal/adapter/pgstore/`
Expected: PASS (existing List/Search/SemanticSearch tests still green — they pass `nil`).

- [ ] **Step 7: Commit**

```bash
git add internal/adapter/pgstore/documents.go internal/adapter/pgstore/documents_projectscope_test.go
git commit -m "feat(pgstore): project-scope filter on list/search/semantic" -m "Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01GTHf7tSzKearihWVzL6i4n"
```

---

### Task 4: Fake project filtering + use-case scoping tests

Mirror the store semantics in `FakeDocumentStore` so the use cases can be unit-tested without Docker, and prove `ListDocuments`/`SearchDocuments` thread `projectID` correctly (including to the semantic arm).

**Files:**
- Modify: `internal/testutil/fakes.go` (add `matchesProject`; apply in `List`, `Search`, `SemanticSearch`)
- Test: `internal/usecase/documents_scope_test.go` (create)

**Interfaces:**
- Consumes: Task 2 signatures; `domain` types from Task 1.
- Produces: `FakeDocumentStore` honoring the `projectID` convention; no new exported symbols.

- [ ] **Step 1: Write the failing use-case tests**

Create `internal/usecase/documents_scope_test.go`:

```go
package usecase_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func pa(s string) *string { return &s }

func seedScopeFake(t *testing.T) *testutil.FakeDocumentStore {
	t.Helper()
	fake := testutil.NewFakeDocumentStore()
	mk := func(id string, proj *string, body string) {
		if _, err := fake.Create(context.Background(), domain.Document{
			ID: id, OwnerID: "u1", ProjectID: proj, Type: domain.DocFree,
			Path: id, Title: id, Body: body,
		}); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	mk("d-a", pa("proj-a"), "alpha thing")
	mk("d-b", pa("proj-b"), "beta thing")
	mk("d-x", nil, "free thing")
	return fake
}

func TestListDocuments_ProjectScope(t *testing.T) {
	fake := seedScopeFake(t)
	uc := usecase.ListDocuments{Docs: fake}
	got, err := uc.Execute(context.Background(), "u1", pa("proj-a"), nil)
	if err != nil || len(got) != 1 || got[0].ID != "d-a" {
		t.Fatalf("ListDocuments(proj-a) = %d, %v; want [d-a]", len(got), err)
	}
	none, _ := uc.Execute(context.Background(), "u1", pa("none"), nil)
	if len(none) != 1 || none[0].ID != "d-x" {
		t.Fatalf("ListDocuments(none) = %d; want [d-x]", len(none))
	}
}

func TestSearchDocuments_ProjectScopeReachesSemanticArm(t *testing.T) {
	ctx := context.Background()
	fake := seedScopeFake(t)
	emb := testutil.NewFakeEmbedder()
	chunkVec := func(body string) []float32 {
		v, err := emb.Embed(ctx, []string{body})
		if err != nil {
			t.Fatalf("embed: %v", err)
		}
		return v[0]
	}
	// one chunk per doc so the semantic arm has candidates
	_ = fake.ReplaceChunks(ctx, "d-a", "u1", []string{"alpha thing"}, [][]float32{chunkVec("alpha thing")})
	_ = fake.ReplaceChunks(ctx, "d-b", "u1", []string{"beta thing"}, [][]float32{chunkVec("beta thing")})

	uc := usecase.SearchDocuments{Docs: fake, Embedder: emb}
	got, err := uc.Execute(ctx, "u1", "thing", pa("proj-a"), nil)
	if err != nil {
		t.Fatalf("Search err: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("Search(proj-a) returned no hits")
	}
	for _, h := range got {
		if h.ProjectID == nil || *h.ProjectID != "proj-a" {
			t.Fatalf("hit %s escaped project scope (projectID=%v)", h.ID, h.ProjectID)
		}
	}
}
```

> The `seedScopeFake` docs all contain the word "thing", so the keyword arm returns the scoped doc and the semantic arm (fed `NewFakeEmbedder` vectors) adds its own scoped candidates — both filtered to `proj-a`, proving the use case threads `projectID` to both arms.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/usecase/ -run "ProjectScope|SemanticArm" -v`
Expected: FAIL — fake ignores `projectID`, so scoped calls return all docs.

- [ ] **Step 3: Add `matchesProject` and apply it in the fake**

In `internal/testutil/fakes.go`, add:

```go
func matchesProject(docPID, filter *string) bool {
	if filter == nil {
		return true
	}
	if *filter == "none" {
		return docPID == nil
	}
	return docPID != nil && *docPID == *filter
}
```

Then add `projectID` filtering to the three methods' per-document guard:
- `List` (:464) loop guard → `if d.OwnerID == ownerID && matchesProject(d.ProjectID, projectID) && hasAllTags(d.Tags, tags) {`
- `Search` (:561) continue guard → `if d.OwnerID != ownerID || !matchesProject(d.ProjectID, projectID) || !hasAllTags(d.Tags, tags) { continue }`
- `SemanticSearch` (:626) continue guard → `if d.OwnerID != ownerID || !matchesProject(d.ProjectID, projectID) || !hasAllTags(d.Tags, tags) { continue }`

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/usecase/ -run "ProjectScope|SemanticArm" -v`
Expected: PASS.

- [ ] **Step 5: Run the full non-Docker suite (regression)**

Run: `go test ./internal/usecase/ ./internal/testutil/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/testutil/fakes.go internal/usecase/documents_scope_test.go
git commit -m "test(usecase): project-scope filtering via fake doc store" -m "Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01GTHf7tSzKearihWVzL6i4n"
```

---

### Task 5: apiclient — additive `*Scoped` document methods

The Slice-2 MCP server needs to request a specific project (or `"none"`, or all). Add `*Scoped` variants and make the existing `ListDocuments`/`Search` delegate with `nil`, so no current caller changes.

**Files:**
- Modify: `internal/adapter/apiclient/documents.go:27-39,70-79`
- Test: `internal/adapter/apiclient/documents_test.go` (append, or create if absent)

**Interfaces:**
- Consumes: REST `GET /api/v1/documents?projectId=` from Task 2.
- Produces:
  - `(*Client).ListDocumentsScoped(ctx, projectID *string, tags ...string) ([]domain.Document, error)`
  - `(*Client).SearchScoped(ctx, q string, projectID *string, tags ...string) ([]domain.SearchHit, error)`
  - `(*Client).ListDocuments` / `(*Client).Search` unchanged externally (now delegate with `nil`).

- [ ] **Step 1: Write the failing test**

Append to `internal/adapter/apiclient/documents_test.go` (create the file with `package apiclient_test` if it doesn't exist; mirror the `httptest` setup used by the other apiclient tests, e.g. `client_test.go`):

```go
func TestListDocumentsScoped_QueryParams(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	pid := "proj-a"
	if _, err := c.ListDocumentsScoped(context.Background(), &pid, "go"); err != nil {
		t.Fatalf("ListDocumentsScoped: %v", err)
	}
	if !strings.Contains(gotQuery, "projectId=proj-a") || !strings.Contains(gotQuery, "tag=go") {
		t.Fatalf("query = %q, want projectId=proj-a & tag=go", gotQuery)
	}

	if _, err := c.SearchScoped(context.Background(), "widget", strptrAC("none")); err != nil {
		t.Fatalf("SearchScoped: %v", err)
	}
	if !strings.Contains(gotQuery, "projectId=none") || !strings.Contains(gotQuery, "q=widget") {
		t.Fatalf("query = %q, want projectId=none & q=widget", gotQuery)
	}
}

func strptrAC(s string) *string { return &s }
```

(Ensure imports: `context`, `net/http`, `net/http/httptest`, `strings`, `testing`, and the `apiclient` package.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapter/apiclient/ -run Scoped -v`
Expected: FAIL — `c.ListDocumentsScoped undefined`.

- [ ] **Step 3: Implement the scoped methods**

In `internal/adapter/apiclient/documents.go`, replace `ListDocuments` (27-39) with:

```go
func (c *Client) ListDocuments(ctx context.Context, tags ...string) ([]domain.Document, error) {
	return c.ListDocumentsScoped(ctx, nil, tags...)
}

// ListDocumentsScoped lists documents, optionally scoped to a project.
// projectID: nil → all; "none" → unassigned; else a project ID.
func (c *Client) ListDocumentsScoped(ctx context.Context, projectID *string, tags ...string) ([]domain.Document, error) {
	q := url.Values{}
	if projectID != nil {
		q.Set("projectId", *projectID)
	}
	for _, t := range tags {
		q.Add("tag", t)
	}
	path := "/api/v1/documents"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	var out []domain.Document
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
```

And replace `Search` (70-79) with:

```go
// Search runs a server-side ranked search; tags AND-filter the results.
func (c *Client) Search(ctx context.Context, q string, tags ...string) ([]domain.SearchHit, error) {
	return c.SearchScoped(ctx, q, nil, tags...)
}

// SearchScoped is Search, optionally scoped to a project (see ListDocumentsScoped).
func (c *Client) SearchScoped(ctx context.Context, q string, projectID *string, tags ...string) ([]domain.SearchHit, error) {
	v := url.Values{}
	v.Set("q", q)
	if projectID != nil {
		v.Set("projectId", *projectID)
	}
	for _, t := range tags {
		v.Add("tag", t)
	}
	var out []domain.SearchHit
	err := c.do(ctx, http.MethodGet, "/api/v1/documents?"+v.Encode(), nil, &out)
	return out, err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapter/apiclient/ -run Scoped -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/apiclient/documents.go internal/adapter/apiclient/documents_test.go
git commit -m "feat(apiclient): ListDocumentsScoped/SearchScoped (projectId query)" -m "Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01GTHf7tSzKearihWVzL6i4n"
```

---

## Final verification

- [ ] Run `make ci`. Expected: build + lint + race tests green, coverage at/above the gate. Fix any lint (e.g. `gofmt`, `QF*` quickfixes) before declaring done.
- [ ] Manual curl smoke against a dev server (controller pattern, see `reference_flow_dev_env`): after `flow login`, create two docs in different projects, then
  - `GET /api/v1/documents?projectId=<idA>` → only project A's docs
  - `GET /api/v1/documents?projectId=none` → only unassigned docs
  - `GET /api/v1/documents?q=<term>&projectId=<idA>` → search scoped to A
  - `GET /api/v1/documents` (no param) → all docs (unchanged behavior)

## Self-review notes (spec coverage)

- Spec §5 (new types) → Task 1.
- Spec §8 (projectId through ports → pgstore → usecase → httpserver → apiclient; `"none"` sentinel; semantic scoped server-side) → Tasks 2-5.
- Out of scope for Slice 1 (Slice 2, planned just-in-time): `internal/clientauth`, `cmd/flow-mcp`, the 11 MCP tools, resources, the write guard, runbook. The new agent-owned types and `*Scoped` apiclient methods exist now but are first *consumed* in Slice 2.
