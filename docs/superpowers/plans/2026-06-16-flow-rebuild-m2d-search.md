# flow rebuild M2d — Search Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Free-text document search — ranked, with highlighted snippets, that finds partial words ("kompend"→"Kompendium") and typos, combinable with the M2c tag filter, in both hosts.

**Architecture:** Postgres FTS (`tsvector`, `simple` config, title-weighted) for ranked exact/phrase matching, **OR**-combined with `pg_trgm` `word_similarity` for fragment/typo recall — both via Postgres-maintained generated/expression indexes (no write-path code, no SSE concern). `?q=` rides the existing `GET /api/v1/documents` endpoint and composes with `?tag=`; a new `SearchDocuments` use case returns `[]SearchHit` (Document + snippet). Hosts render the snippet with shared sentinel highlight markers.

**Tech Stack:** Go, pgx/v5, Postgres 16 + pg_trgm, goldmark/bluemonday (WebUI), templ, charm.land/bubbletea/v2 (TUI).

**Spec:** `docs/superpowers/specs/2026-06-16-flow-rebuild-m2d-search-design.md`

---

## File map

| File | Responsibility | Action |
|------|----------------|--------|
| `internal/adapter/pgstore/migrations/0009_documents_search.sql` | pg_trgm + generated tsvector column + GIN indexes | Create |
| `internal/domain/search.go` | `SearchHit` type + highlight sentinel constants | Create |
| `internal/domain/search_test.go` | flat-JSON embed test | Create |
| `internal/ports/ports.go` | `DocumentStore.Search` method | Modify |
| `internal/testutil/fakes.go` | `FakeDocumentStore.Search` (substring + snippet) | Modify |
| `internal/adapter/pgstore/documents.go` | pgstore `Search` (FTS+trgm hybrid) | Modify |
| `internal/adapter/pgstore/documents_test.go` | DB-gated search test | Modify |
| `internal/usecase/search_documents.go` | `SearchDocuments` use case | Create |
| `internal/usecase/document_test.go` | use-case test | Modify |
| `internal/adapter/httpserver/documents.go` | `handleListDocuments` branch on `?q=` | Modify |
| `internal/adapter/httpserver/server.go` | `SearchDocuments` field | Modify |
| `internal/adapter/httpserver/documents_test.go` | REST search test | Modify |
| `internal/adapter/apiclient/documents.go` | `Search(ctx,q,tags...)` | Modify |
| `internal/adapter/apiclient/documents_test.go` | apiclient search test | Modify |
| `internal/adapter/webui/docs.templ` | search box + snippet `<mark>` + CSS | Modify |
| `internal/adapter/httpserver/webui_docs.go` | `docsListData` branch on `q` + Query incl. q | Modify |
| `internal/adapter/httpserver/webui_docs_test.go` | WebUI search test | Modify |
| `internal/tui/docs.go` | `/` search mode + snippet highlight | Modify |
| `internal/tui/docs_test.go` | TUI search tests | Modify |
| `cmd/flow-server/main.go` | wire `SearchDocuments` | Modify |

Tasks are ordered so the build stays green after each. Reminder ([[feedback_pgstore_goose_migrations]]): every migration needs goose `-- +goose Up`/`-- +goose Down`.

---

## Task 1: Migration 0009 — pg_trgm + generated tsvector + indexes

**Files:**
- Create: `internal/adapter/pgstore/migrations/0009_documents_search.sql`

- [ ] **Step 1: Write the migration** (goose-annotated, idempotent Down)

```sql
-- +goose Up
CREATE EXTENSION IF NOT EXISTS pg_trgm;

ALTER TABLE documents ADD COLUMN search tsvector GENERATED ALWAYS AS (
    setweight(to_tsvector('simple', coalesce(title, '')), 'A') ||
    setweight(to_tsvector('simple', coalesce(body, '')),  'B')
) STORED;
CREATE INDEX documents_search_gin ON documents USING GIN (search);

CREATE INDEX documents_trgm_title ON documents USING GIN (title gin_trgm_ops);
CREATE INDEX documents_trgm_body  ON documents USING GIN (body  gin_trgm_ops);

-- +goose Down
DROP INDEX IF EXISTS documents_trgm_body;
DROP INDEX IF EXISTS documents_trgm_title;
DROP INDEX IF EXISTS documents_search_gin;
ALTER TABLE documents DROP COLUMN IF EXISTS search;
-- pg_trgm extension intentionally left installed (harmless, shared).
```

- [ ] **Step 2: Build to confirm embedding compiles**

Run: `go build ./internal/adapter/pgstore/`
Expected: success (migrations are `//go:embed migrations/*.sql` in `pool.go`; the new file is globbed automatically).

- [ ] **Step 3: Commit**

```bash
git add internal/adapter/pgstore/migrations/0009_documents_search.sql
git commit -m "feat(m2d): migration 0009 pg_trgm + generated search tsvector"
```

---

## Task 2: Domain — SearchHit + highlight sentinels

**Files:**
- Create: `internal/domain/search.go`
- Test: `internal/domain/search_test.go`

- [ ] **Step 1: Write the failing test**

```go
package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSearchHit_FlatJSON(t *testing.T) {
	h := SearchHit{
		Document: Document{ID: "a", Type: DocFree, Path: "p", Title: "T", Tags: []string{"go"}},
		Snippet:  "hello " + HighlightStart + "world" + HighlightEnd,
	}
	b, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	// flat: document fields and snippet are siblings (no nested "Document" key)
	if strings.Contains(js, `"Document"`) {
		t.Fatalf("JSON should be flat, got %s", js)
	}
	if !strings.Contains(js, `"id":"a"`) || !strings.Contains(js, `"snippet":`) {
		t.Fatalf("missing fields: %s", js)
	}
	// decodes back into a plain Document (superset compatibility)
	var d Document
	if err := json.Unmarshal(b, &d); err != nil || d.ID != "a" {
		t.Fatalf("plain Document decode failed: %v / %#v", err, d)
	}
}

func TestHighlightSentinels(t *testing.T) {
	if HighlightStart == HighlightEnd || HighlightStart == "" {
		t.Fatal("sentinels must be distinct and non-empty")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/domain/ -run 'SearchHit|HighlightSentinels' -v`
Expected: FAIL — `undefined: SearchHit`, `HighlightStart`, `HighlightEnd`.

- [ ] **Step 3: Implement**

Create `internal/domain/search.go`:

```go
package domain

// Highlight sentinels wrap matched spans in a search Snippet (emitted by the
// store's ts_headline StartSel/StopSel) and are replaced by each host: WebUI →
// <mark>…</mark>, TUI → a lipgloss highlight on/off. Control chars are used so
// they never collide with document text.
const (
	HighlightStart = "\x02"
	HighlightEnd   = "\x03"
)

// SearchHit is a document plus its search snippet. The Document is embedded
// anonymously so the JSON is flat (Document fields + "snippet") — a plain
// []Document decoder still works against a SearchHit response.
type SearchHit struct {
	Document
	Snippet string `json:"snippet"`
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/domain/ -run 'SearchHit|HighlightSentinels' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/search.go internal/domain/search_test.go
git commit -m "feat(m2d): domain SearchHit + highlight sentinels"
```

---

## Task 3: Store — Search (port + fake + pgstore hybrid)

**Files:**
- Modify: `internal/ports/ports.go` (DocumentStore interface)
- Modify: `internal/testutil/fakes.go` (FakeDocumentStore.Search)
- Modify: `internal/adapter/pgstore/documents.go` (pgstore Search + scanSearchHit)
- Test: `internal/testutil/fakes_test.go`
- Test: `internal/adapter/pgstore/documents_test.go`

- [ ] **Step 1: Write the failing fake test**

Add to `internal/testutil/fakes_test.go`:

```go
func TestFakeDocumentStore_Search(t *testing.T) {
	s := NewFakeDocumentStore()
	ctx := context.Background()
	mk := func(id, title, body string, tags ...string) {
		if _, err := s.Create(ctx, domain.Document{ID: id, OwnerID: "u", Type: domain.DocFree, Path: id, Title: title, Body: body, Tags: tags}); err != nil {
			t.Fatal(err)
		}
	}
	mk("a", "Kompendium", "about the compendium", "go")
	mk("b", "Other", "unrelated text")

	hits, err := s.Search(ctx, "u", "kompend", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ID != "a" {
		t.Fatalf("search kompend = %#v, want [a]", hits)
	}
	if !strings.Contains(hits[0].Snippet, domain.HighlightStart) {
		t.Fatalf("snippet missing highlight markers: %q", hits[0].Snippet)
	}
	// composes with tag filter
	none, _ := s.Search(ctx, "u", "kompend", []string{"missing"})
	if len(none) != 0 {
		t.Fatalf("tag-filtered search = %d, want 0", len(none))
	}
}
```

(Ensure `strings`, `context`, `domain` are imported in the test file.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/testutil/ -run Search -v`
Expected: FAIL — `s.Search` undefined.

- [ ] **Step 3: Add the port method**

In `internal/ports/ports.go`, add to the `DocumentStore` interface (after the `Backlinks` method):

```go
	// Search returns owner documents matching q (FTS + fuzzy), ranked, each with
	// a highlighted snippet. When tags are given, results are AND-filtered to
	// documents carrying all of them. Empty q is not expected here (callers use
	// List for the no-query path).
	Search(ctx context.Context, ownerID, q string, tags []string) ([]domain.SearchHit, error)
```

- [ ] **Step 4: Implement the fake**

In `internal/testutil/fakes.go`, add (reuse the existing `hasAllTags` helper from M2c):

```go
func (s *FakeDocumentStore) Search(_ context.Context, ownerID, q string, tags []string) ([]domain.SearchHit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ql := strings.ToLower(q)
	var out []domain.SearchHit
	for _, d := range s.m {
		if d.OwnerID != ownerID || !hasAllTags(d.Tags, tags) {
			continue
		}
		hay := strings.ToLower(d.Title + " " + d.Body)
		idx := strings.Index(hay, ql)
		if ql == "" || idx < 0 {
			continue
		}
		out = append(out, domain.SearchHit{Document: d, Snippet: fakeSnippet(d.Title+" "+d.Body, idx, len(q))})
	}
	// stable-ish: newest first (real ranking is exercised by the pgstore test)
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].UpdatedAt.After(out[i].UpdatedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

// fakeSnippet wraps the matched [start,start+n) span of text with the shared
// highlight sentinels — enough to exercise host snippet rendering in tests.
func fakeSnippet(text string, start, n int) string {
	end := start + n
	if end > len(text) {
		end = len(text)
	}
	return text[:start] + domain.HighlightStart + text[start:end] + domain.HighlightEnd + text[end:]
}
```

(Add `"strings"` to `fakes.go` imports if not already present.)

- [ ] **Step 5: Implement pgstore Search + scanSearchHit**

In `internal/adapter/pgstore/documents.go`, add (note: `prefixedDocCols` already exists; ranking expressions live in ORDER BY only, so SELECT is docCols + snippet):

```go
// headlineOpts carries the highlight sentinels to ts_headline. Passed as a bound
// parameter (not a SQL literal) so the control chars need no escaping.
var headlineOpts = "StartSel=" + domain.HighlightStart + ",StopSel=" + domain.HighlightEnd +
	",MaxFragments=1,MinWords=5,MaxWords=18,HighlightAll=false"

func (s *DocumentStore) Search(ctx context.Context, ownerID, q string, tags []string) ([]domain.SearchHit, error) {
	sb := `SELECT ` + prefixedDocCols + `,
  ts_headline('simple', coalesce(d.title,'')||' '||coalesce(d.body,''), ftsq, $3) AS snippet
FROM documents d, websearch_to_tsquery('simple', $2) ftsq
WHERE d.owner_id = $1`
	args := []any{ownerID, q, headlineOpts}
	if len(tags) > 0 {
		sb += ` AND d.tags @> $4`
		args = append(args, tags)
	}
	sb += `
  AND (d.search @@ ftsq OR $2 <% (coalesce(d.title,'')||' '||coalesce(d.body,'')))
ORDER BY (d.search @@ ftsq) DESC,
         ts_rank(d.search, ftsq) DESC,
         GREATEST(word_similarity($2, d.title), word_similarity($2, d.body)) DESC,
         d.updated_at DESC
LIMIT 100`

	rows, err := s.pool.Query(ctx, sb, args...)
	if err != nil {
		return nil, fmt.Errorf("pgstore: search documents: %w", err)
	}
	defer rows.Close()
	var out []domain.SearchHit
	for rows.Next() {
		h, err := scanSearchHit(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// scanSearchHit scans prefixedDocCols + a trailing snippet column.
func scanSearchHit(r rowScanner) (domain.SearchHit, error) {
	var d domain.Document
	var typ string
	var extra []byte
	var snippet string
	if err := r.Scan(&d.ID, &d.OwnerID, &d.ProjectID, &typ, &d.Path, &d.Title, &d.Body,
		&d.Tags, &d.Date, &d.Role, &extra, &d.CreatedAt, &d.UpdatedAt, &snippet); err != nil {
		return domain.SearchHit{}, fmt.Errorf("pgstore: scan search hit: %w", err)
	}
	d.Type = domain.DocumentType(typ)
	if len(extra) > 0 {
		if err := json.Unmarshal(extra, &d.Extra); err != nil {
			return domain.SearchHit{}, fmt.Errorf("pgstore: unmarshal extra: %w", err)
		}
	}
	return domain.SearchHit{Document: d, Snippet: snippet}, nil
}
```

(`domain` is already imported in this file.)

- [ ] **Step 6: Add the DB-gated pgstore test**

In `internal/adapter/pgstore/documents_test.go`, add a test mirroring the existing `startPG`/setup idiom used by `TestDocumentStore_ListTagFilter` (read that test and copy its setup helpers exactly):

```go
func TestDocumentStore_SearchFuzzyAndTag(t *testing.T) {
	pool := startPG(t) // existing helper; skips when Docker/test DB absent
	s := NewDocumentStore(pool)
	ctx := context.Background()
	owner := "search-" + randSuffix(t) // match the file's real unique-suffix helper

	mk := func(path, title, body string, tags ...string) {
		_, err := s.Create(ctx, domain.Document{
			ID: newID(t), OwnerID: owner, Type: domain.DocFree, Path: path,
			Title: title, Body: body, Tags: tags, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	mk("a", "Kompendium", "notes about the compendium", "go")
	mk("b", "Anderes", "etwas ganz anderes")

	// partial word: "kompend" must find "Kompendium" (trigram arm)
	hits, err := s.Search(ctx, owner, "kompend", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Path != "a" {
		t.Fatalf(`search "kompend" = %#v, want [a]`, hits)
	}
	// exact FTS hit carries a highlighted snippet
	exact, _ := s.Search(ctx, owner, "compendium", nil)
	if len(exact) == 0 || !strings.Contains(exact[0].Snippet, domain.HighlightStart) {
		t.Fatalf("expected highlighted snippet, got %#v", exact)
	}
	// composes with tag filter
	none, _ := s.Search(ctx, owner, "kompend", []string{"missing"})
	if len(none) != 0 {
		t.Fatalf("tag-filtered search = %d, want 0", len(none))
	}
}
```

Match the real helper names (`startPG`/`randSuffix`/`newID`) to whatever `documents_test.go` already uses — do not invent new ones. Ensure `strings`/`time`/`domain` imports exist.

- [ ] **Step 7: Run + build**

Run: `go test ./internal/testutil/ -run Search -v && go test ./internal/adapter/pgstore/ -run SearchFuzzyAndTag -v && go build ./...`
Expected: fake PASS; pgstore PASS if Docker present (migration 0009 applies, "kompend"→Kompendium works), else SKIP; build clean.

- [ ] **Step 8: Commit**

```bash
git add internal/ports/ports.go internal/testutil/fakes.go internal/adapter/pgstore/documents.go internal/testutil/fakes_test.go internal/adapter/pgstore/documents_test.go
git commit -m "feat(m2d): DocumentStore.Search (FTS + pg_trgm hybrid)"
```

---

## Task 4: Use case — SearchDocuments

**Files:**
- Create: `internal/usecase/search_documents.go`
- Test: `internal/usecase/document_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/usecase/document_test.go` (match the file's package style / qualifiers):

```go
func TestSearchDocuments(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	ctx := context.Background()
	if _, err := docs.Create(ctx, domain.Document{ID: "a", OwnerID: "u", Type: domain.DocFree, Path: "a", Title: "Kompendium", Body: "x", Tags: []string{"go"}}); err != nil {
		t.Fatal(err)
	}
	uc := usecase.SearchDocuments{Docs: docs}
	hits, err := uc.Execute(ctx, "u", "kompend", []string{"go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ID != "a" {
		t.Fatalf("got %#v, want [a]", hits)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/usecase/ -run TestSearchDocuments -v`
Expected: FAIL — `undefined: usecase.SearchDocuments`.

- [ ] **Step 3: Implement**

Create `internal/usecase/search_documents.go`:

```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// SearchDocuments runs a ranked full-text + fuzzy search over the owner's
// documents, optionally AND-filtered by tags.
type SearchDocuments struct{ Docs ports.DocumentStore }

func (uc SearchDocuments) Execute(ctx context.Context, ownerID, q string, tags []string) ([]domain.SearchHit, error) {
	return uc.Docs.Search(ctx, ownerID, q, tags)
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/usecase/ -run TestSearchDocuments -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/search_documents.go internal/usecase/document_test.go
git commit -m "feat(m2d): SearchDocuments use case"
```

---

## Task 5: REST — branch handleListDocuments on ?q=

**Files:**
- Modify: `internal/adapter/httpserver/server.go` (Server struct field)
- Modify: `internal/adapter/httpserver/documents.go` (handler branch)
- Test: `internal/adapter/httpserver/documents_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/adapter/httpserver/documents_test.go` (reuse the existing `newDocServer` helper + auth-GET helper; add `SearchDocuments: usecase.SearchDocuments{Docs: docs}` to that helper's `&Server{...}` literal):

```go
func TestHandleListDocuments_SearchQuery(t *testing.T) {
	srv, docs := newDocServer(t)
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{ID: "a", OwnerID: testUserID, Type: domain.DocFree, Path: "a", Title: "Kompendium", Body: "x"})
	_, _ = docs.Create(ctx, domain.Document{ID: "b", OwnerID: testUserID, Type: domain.DocFree, Path: "b", Title: "Other", Body: "y"})

	rec := doAuthedGET(t, srv, "/api/v1/documents?q=kompend")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var hits []domain.SearchHit
	mustJSON(t, rec, &hits)
	if len(hits) != 1 || hits[0].ID != "a" {
		t.Fatalf("got %#v, want [a]", hits)
	}
	if !strings.Contains(hits[0].Snippet, domain.HighlightStart) {
		t.Fatalf("missing snippet markers: %q", hits[0].Snippet)
	}
}

func TestHandleListDocuments_NoQueryUnchanged(t *testing.T) {
	srv, docs := newDocServer(t)
	_, _ = docs.Create(context.Background(), domain.Document{ID: "a", OwnerID: testUserID, Type: domain.DocFree, Path: "a", Title: "A"})
	rec := doAuthedGET(t, srv, "/api/v1/documents")
	var list []domain.Document
	mustJSON(t, rec, &list) // still decodes as plain []Document
	if len(list) != 1 || list[0].ID != "a" {
		t.Fatalf("plain list broke: %#v", list)
	}
}
```

(Adapt helper names to the real ones in the file.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/adapter/httpserver/ -run 'SearchQuery|NoQueryUnchanged' -v`
Expected: FAIL — `?q=` ignored / `SearchDocuments` field missing.

- [ ] **Step 3: Add the Server field**

In `internal/adapter/httpserver/server.go`, add to the documents field group (next to `ListTags usecase.ListTags`):

```go
	SearchDocuments   usecase.SearchDocuments
```

- [ ] **Step 4: Branch the handler**

In `internal/adapter/httpserver/documents.go`, change `handleListDocuments` to branch on a trimmed `q` (add `"strings"` to imports if needed):

```go
func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	tags := r.URL.Query()["tag"]
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		hits, err := s.SearchDocuments.Execute(r.Context(), u.ID, q, tags)
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
	list, err := s.ListDocuments.Execute(r.Context(), u.ID, tags)
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

- [ ] **Step 5: Run + build**

Run: `go test ./internal/adapter/httpserver/ -run 'SearchQuery|NoQueryUnchanged' -v && go build ./...`
Expected: PASS + build clean (test harness now sets `SearchDocuments`).

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/httpserver/server.go internal/adapter/httpserver/documents.go internal/adapter/httpserver/documents_test.go
git commit -m "feat(m2d): REST ?q= search branch on documents list"
```

---

## Task 6: apiclient — Search

**Files:**
- Modify: `internal/adapter/apiclient/documents.go`
- Test: `internal/adapter/apiclient/documents_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/adapter/apiclient/documents_test.go` (reuse the real test-client helper):

```go
func TestSearch_QueryAndTags(t *testing.T) {
	var gotQuery string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`[{"id":"a","snippet":"hi"}]`))
	})
	hits, err := c.Search(context.Background(), "kompend", "go")
	if err != nil {
		t.Fatal(err)
	}
	if gotQuery != "q=kompend&tag=go" {
		t.Fatalf("query = %q, want q=kompend&tag=go", gotQuery)
	}
	if len(hits) != 1 || hits[0].ID != "a" || hits[0].Snippet != "hi" {
		t.Fatalf("decode failed: %#v", hits)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/adapter/apiclient/ -run TestSearch_QueryAndTags -v`
Expected: FAIL — `c.Search` undefined.

- [ ] **Step 3: Implement**

In `internal/adapter/apiclient/documents.go`, add (`net/url`, `net/http`, `domain` already imported):

```go
// Search runs a server-side ranked search; tags AND-filter the results.
func (c *Client) Search(ctx context.Context, q string, tags ...string) ([]domain.SearchHit, error) {
	v := url.Values{}
	v.Set("q", q)
	for _, t := range tags {
		v.Add("tag", t)
	}
	var out []domain.SearchHit
	err := c.do(ctx, http.MethodGet, "/api/v1/documents?"+v.Encode(), nil, &out)
	return out, err
}
```

Note: `url.Values.Encode()` sorts keys alphabetically → `q` before `tag`, giving the deterministic `q=kompend&tag=go`.

- [ ] **Step 4: Run + build**

Run: `go test ./internal/adapter/apiclient/ -run TestSearch_QueryAndTags -v && go build ./...`
Expected: PASS + clean.

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/apiclient/documents.go internal/adapter/apiclient/documents_test.go
git commit -m "feat(m2d): apiclient Search(q, tags...)"
```

---

## Task 7: WebUI — search box + snippet rendering

**Files:**
- Modify: `internal/adapter/webui/docs.templ` (view model + search form + snippet + CSS)
- Modify: `internal/adapter/httpserver/webui_docs.go` (docsListData branch on q; Query incl. q)
- Test: `internal/adapter/httpserver/webui_docs_test.go`

- [ ] **Step 1: Extend the view model + template (docs.templ)**

In `internal/adapter/webui/docs.templ`:

Add a search-result row type and extend `DocsPageData`:

```go
// SearchRow is one search hit: a doc row plus its rendered (highlighted) snippet
// HTML (already escaped + <mark>-wrapped by the handler; rendered via templ.Raw).
type SearchRow struct {
	DocRow
	Snippet string
}
```

Add fields to `DocsPageData`:

```go
	Query      string      // encoded "?...&q=..." preserving tags+q for SSE refresh
	SearchQ    string       // raw current query (echoed into the search box)
	Results    []SearchRow  // populated when SearchQ != ""
```

(Keep the existing `Docs`, `AllTags`, `ActiveTags`, etc.)

Add a search form to `DocsListFragment`, above the filter bar (hidden inputs carry the active tags so search composes with the filter):

```go
	<form method="get" action="/docs" class="mb-3">
		for _, t := range d.ActiveTags {
			<input type="hidden" name="tag" value={ t }/>
		}
		<input type="search" name="q" value={ d.SearchQ } placeholder="Suche…"
			class="w-full rounded border border-slate-300 px-2 py-1 text-sm"/>
	</form>
```

In the list body, when `d.SearchQ != ""` render `d.Results` (with snippet) instead of the plain `d.Docs` list:

```go
	if d.SearchQ != "" {
		if len(d.Results) == 0 {
			<p class="text-sm text-slate-400">no matches for "{ d.SearchQ }"</p>
		} else {
			<ul class="divide-y divide-slate-100">
				for _, r := range d.Results {
					<li class="py-2">
						<a href={ templ.SafeURL("/docs/" + r.ID) } class="block hover:text-slate-700">
							<div class="flex items-center justify-between text-sm">
								<span class="font-medium text-slate-800">{ r.Title }</span>
								<span class="text-xs text-slate-400">{ r.Type }</span>
							</div>
							<div class="mt-0.5 text-xs text-slate-500">
								@templ.Raw(r.Snippet)
							</div>
						</a>
					</li>
				}
			</ul>
		}
	} else {
		// ... existing plain doc list (the current for _, doc := range d.Docs loop) ...
	}
```

Add `mark` styling to the page `<style>` block (both `DocsPage` and `DocView` already have a `<style>`; add to `DocsPage`'s):

```css
mark { background: #fde68a; color: inherit; padding: 0 1px; border-radius: 2px; }
```

Read the current `docs.templ` and integrate these without disturbing the existing filter-bar/tag-chip markup.

- [ ] **Step 2: Regenerate templ**

Run: `make generate`
Expected: `internal/adapter/webui/docs_templ.go` regenerated, no errors.

- [ ] **Step 3: Build the snippet HTML + branch the handler (webui_docs.go)**

In `internal/adapter/httpserver/webui_docs.go`, add a snippet renderer and branch `docsListData` on `q`. Add `"html"` and `"strings"` to imports (no `html/template` needed — the result is a plain `string` rendered via `@templ.Raw`):

```go
// renderSnippet escapes the snippet then replaces the highlight sentinels with
// <mark> tags — escape-first so no document content can inject HTML. Returns a
// safe HTML string (rendered raw in the template via @templ.Raw).
func renderSnippet(s string) string {
	esc := html.EscapeString(s)
	// sentinels are control chars; html.EscapeString leaves them intact:
	esc = strings.ReplaceAll(esc, domain.HighlightStart, "<mark>")
	esc = strings.ReplaceAll(esc, domain.HighlightEnd, "</mark>")
	return esc
}
```

Then in `docsListData`, after computing `active` and the chips, branch:

```go
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	data := webui.DocsPageData{
		User: u.Username, AllTags: chips, ActiveTags: active,
		SearchQ: q, Query: encodeListQuery(active, q),
	}
	if q != "" {
		hits, err := s.SearchDocuments.Execute(r.Context(), u.ID, q, active)
		if err != nil {
			return webui.DocsPageData{}, err
		}
		results := make([]webui.SearchRow, 0, len(hits))
		for _, h := range hits {
			results = append(results, webui.SearchRow{
				DocRow:  webui.DocRow{ID: h.ID, Type: string(h.Type), Path: h.Path, Title: h.Title, Tags: h.Tags},
				Snippet: renderSnippet(h.Snippet),
			})
		}
		data.Results = results
		return data, nil
	}
	list, err := s.ListDocuments.Execute(r.Context(), u.ID, active)
	if err != nil {
		return webui.DocsPageData{}, err
	}
	rows := make([]webui.DocRow, 0, len(list))
	for _, d := range list {
		rows = append(rows, webui.DocRow{ID: d.ID, Type: string(d.Type), Path: d.Path, Title: d.Title, Tags: d.Tags})
	}
	data.Docs = rows
	return data, nil
```

Replace the old `docsListData` body with the above (it previously always called `ListDocuments`). Add `encodeListQuery` next to `encodeTagQuery`:

```go
// encodeListQuery encodes the active tags plus an optional q into a "?..." string
// (empty when nothing is set) for the SSE-refresh hx-get on #dc.
func encodeListQuery(tags []string, q string) string {
	v := url.Values{}
	for _, t := range tags {
		v.Add("tag", t)
	}
	if q != "" {
		v.Set("q", q)
	}
	enc := v.Encode()
	if enc == "" {
		return ""
	}
	return "?" + enc
}
```

(Leave `encodeTagQuery`/`toggleTagHref` as-is — the chip hrefs still use tag-only URLs.)

- [ ] **Step 4: Regenerate templ again if the model changed, then test**

Add to `internal/adapter/httpserver/webui_docs_test.go` (reuse the real harness; `SearchDocuments` must be wired into the test server — add it):

```go
func TestWebDocsList_Search(t *testing.T) {
	srv, docs := newWebDocsServer(t)
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{ID: "a", OwnerID: testUserID, Type: domain.DocFree, Path: "a", Title: "Kompendium", Body: "notes"})
	_, _ = docs.Create(ctx, domain.Document{ID: "b", OwnerID: testUserID, Type: domain.DocFree, Path: "b", Title: "Other", Body: "x"})

	body := doAuthedGETBody(t, srv, "/ui/docs/list?q=kompend")
	if !strings.Contains(body, "Kompendium") || strings.Contains(body, ">Other<") {
		t.Fatalf("search did not narrow list: %s", body)
	}
	if !strings.Contains(body, "<mark>") {
		t.Fatalf("snippet highlight missing: %s", body)
	}
}

func TestRenderSnippet_EscapesThenMarks(t *testing.T) {
	got := renderSnippet("a<b " + domain.HighlightStart + "x" + domain.HighlightEnd)
	if !strings.Contains(got, "&lt;b") {
		t.Fatalf("did not escape: %q", got)
	}
	if !strings.Contains(got, "<mark>x</mark>") {
		t.Fatalf("did not mark: %q", got)
	}
}
```

(`TestRenderSnippet_EscapesThenMarks` must live in a `package httpserver` test file to call the unexported helper — put it in an existing internal test file or a new `webui_docs_internal_test.go` with `package httpserver`.)

Run: `make generate && go test ./internal/adapter/httpserver/ -run 'WebDocs|RenderSnippet' -v`
Expected: PASS.

- [ ] **Step 5: Verify generate + build + vet**

Run: `make verify-generate && go build ./... && go vet ./internal/adapter/httpserver/ ./internal/adapter/webui/`
Expected: `verify-generate: OK`, clean.

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/webui/docs.templ internal/adapter/webui/docs_templ.go internal/adapter/httpserver/webui_docs.go internal/adapter/httpserver/webui_docs_test.go internal/adapter/httpserver/webui_docs_internal_test.go
git commit -m "feat(m2d): WebUI search box + highlighted snippets"
```

---

## Task 8: TUI — `/` search mode + snippet highlight

**Files:**
- Modify: `internal/tui/docs.go`
- Test: `internal/tui/docs_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/tui/docs_test.go` (mirror the existing key-event helper):

```go
func TestDocs_SearchInputAndRun(t *testing.T) {
	m := NewDocs(nil, nil, nil, "u")
	// enter search input mode
	m2, _ := m.Update(tea.KeyPressMsg{Text: "/"})
	dm := m2.(DocsModel)
	if dm.mode != modeSearch {
		t.Fatalf("mode = %v, want modeSearch", dm.mode)
	}
	// type "go"
	m3, _ := dm.Update(tea.KeyPressMsg{Text: "g"})
	dm = m3.(DocsModel)
	m4, _ := dm.Update(tea.KeyPressMsg{Text: "o"})
	dm = m4.(DocsModel)
	if dm.searchQuery != "go" {
		t.Fatalf("searchQuery = %q, want go", dm.searchQuery)
	}
	// esc returns to list and clears
	m5, _ := dm.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	dm = m5.(DocsModel)
	if dm.mode != modeList {
		t.Fatalf("esc did not return to list: %v", dm.mode)
	}
}

func TestDocs_RenderSearchHighlights(t *testing.T) {
	m := NewDocs(nil, nil, nil, "u")
	m.mode = modeSearch
	m.searching = true
	m.searchHits = []domain.SearchHit{
		{Document: domain.Document{ID: "a", Type: domain.DocFree, Path: "a", Title: "Kompendium"},
			Snippet: "see " + domain.HighlightStart + "Kompendium" + domain.HighlightEnd + " here"},
	}
	var b strings.Builder
	m.renderSearch(&b)
	out := b.String()
	if !strings.Contains(out, "Kompendium") {
		t.Fatalf("snippet not rendered: %q", out)
	}
	// sentinels must be consumed (replaced by styling), not printed raw
	if strings.Contains(out, domain.HighlightStart) || strings.Contains(out, domain.HighlightEnd) {
		t.Fatalf("raw sentinels leaked into output: %q", out)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run 'SearchInputAndRun|RenderSearchHighlights' -v`
Expected: FAIL — `modeSearch`, `searchQuery`, `searching`, `searchHits`, `renderSearch` undefined.

- [ ] **Step 3: Add model state + mode**

In `internal/tui/docs.go`, add `modeSearch` to the `docMode` const block. Add fields to `DocsModel`:

```go
	searchQuery string             // current query buffer (input phase)
	searching   bool               // true once a query has been run (results phase)
	searchHits  []domain.SearchHit
	searchSel   int
```

- [ ] **Step 4: Add the search command + message**

Add near `loadTags` (Task references the existing `errMsg`/timeout pattern):

```go
type searchDoneMsg struct{ hits []domain.SearchHit }

func (m DocsModel) runSearch(q string) tea.Cmd {
	if m.client == nil {
		return nil
	}
	tags := m.filterTags
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		hits, err := m.client.Search(ctx, q, tags...)
		if err != nil {
			return errMsg{err}
		}
		return searchDoneMsg{hits: hits}
	}
}
```

Handle `searchDoneMsg` in `Update`:

```go
	case searchDoneMsg:
		m.searchHits = msg.hits
		m.searching = true
		m.searchSel = 0
		return m, nil
```

- [ ] **Step 5: Open + handle search keys**

In the `modeList` switch in `handleKey`, add (near the `f` filter case):

```go
	case k.Text == "/":
		m.mode = modeSearch
		m.searchQuery = ""
		m.searching = false
		m.searchHits = nil
		return m, nil
```

In the `handleKey` mode switch, route `modeSearch` to `m.handleSearchKey(k)`. Add the handler (text input during the query phase; j/k/enter to navigate results once `searching`):

```go
func (m DocsModel) handleSearchKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		m.mode = modeList
		m.searching = false
		m.searchHits = nil
		return m, nil
	case k.Code == tea.KeyEnter:
		if !m.searching {
			if strings.TrimSpace(m.searchQuery) == "" {
				return m, nil
			}
			return m, m.runSearch(m.searchQuery)
		}
		// results phase: open selected hit
		if len(m.searchHits) == 0 {
			return m, nil
		}
		return m, m.loadDoc(m.searchHits[m.searchSel].ID, false)
	case m.searching && k.Text == "j":
		if m.searchSel < len(m.searchHits)-1 {
			m.searchSel++
		}
		return m, nil
	case m.searching && k.Text == "k":
		if m.searchSel > 0 {
			m.searchSel--
		}
		return m, nil
	case !m.searching && k.Code == tea.KeyBackspace:
		m.searchQuery = dropLast(m.searchQuery)
		return m, nil
	case !m.searching && k.Text != "":
		m.searchQuery += k.Text
		return m, nil
	}
	return m, nil
}
```

(`dropLast` already exists, used by the create form.)

- [ ] **Step 6: Render**

Add `renderSearch` and dispatch it from the `View` render switch (`case modeSearch: m.renderSearch(&b)`):

```go
func (m DocsModel) renderSearch(b *strings.Builder) {
	b.WriteString(styleHeader.Render("Search") + styleMuted.Render("  /"+m.searchQuery) + "\n")
	if !m.searching {
		b.WriteString(styleMuted.Render("  enter to search · esc cancel") + "\n")
		return
	}
	if len(m.searchHits) == 0 {
		b.WriteString(styleMuted.Render("  no matches") + "\n")
		return
	}
	for i, h := range m.searchHits {
		title := h.Title
		if i == m.searchSel {
			title = styleSel.Render("▸ " + h.Title)
		} else {
			title = "  " + h.Title
		}
		b.WriteString(title + styleMuted.Render("  "+h.Path) + "\n")
		b.WriteString("    " + highlightSnippet(h.Snippet) + "\n")
	}
}

// highlightSnippet replaces the shared sentinels with a lipgloss highlight.
func highlightSnippet(s string) string {
	var out strings.Builder
	for {
		i := strings.Index(s, domain.HighlightStart)
		if i < 0 {
			out.WriteString(styleMuted.Render(s))
			break
		}
		j := strings.Index(s, domain.HighlightEnd)
		if j < 0 || j < i {
			out.WriteString(styleMuted.Render(s))
			break
		}
		out.WriteString(styleMuted.Render(s[:i]))
		out.WriteString(styleSearchHit.Render(s[i+len(domain.HighlightStart) : j]))
		s = s[j+len(domain.HighlightEnd):]
	}
	return out.String()
}
```

Add a style to `internal/tui/styles.go` using an EXISTING palette color (the palette has `colAccent/colMuted/colBg/colGreen/colRed/colCyan` — `colCyan` reads as a distinct highlight vs the `colAccent` selection bar):

```go
	styleSearchHit = lipgloss.NewStyle().Foreground(colBg).Background(colCyan).Bold(true)
```

First read `internal/tui/styles.go` and confirm `colCyan` exists; if it does not, fall back to `colAccent`. Do not invent a new palette entry.

- [ ] **Step 7: Footer**

In `footer()`, add a `modeSearch` case (`"type query · enter search · esc cancel"` during input; the same string is fine for results) and advertise `/` in the `modeList` default footer:
`"j/k move · enter view · n new · e edit · d delete · f filter · / search · q quit"`.

- [ ] **Step 8: Run + build**

Run: `go test ./internal/tui/ -run 'Search' -v && go build ./... && go vet ./internal/tui/`
Expected: PASS + clean.

- [ ] **Step 9: Consult tui-usability**

Invoke the `tui-usability` skill; confirm `/` for search fits the grammar, the highlight style uses a semantic color, glyphs are on the whitelist, and the footer reads consistently. Apply any minor adjustments.

- [ ] **Step 10: Commit**

```bash
git add internal/tui/docs.go internal/tui/docs_test.go internal/tui/styles.go
git commit -m "feat(m2d): TUI / search mode with highlighted snippets"
```

---

## Task 9: Composition root + full CI

**Files:**
- Modify: `cmd/flow-server/main.go`

- [ ] **Step 1: Wire SearchDocuments**

In `cmd/flow-server/main.go`, in the `&httpserver.Server{...}` documents block (next to `ListTags:`), add:

```go
		SearchDocuments:   usecase.SearchDocuments{Docs: documentStore},
```

- [ ] **Step 2: Verify wiring**

Run: `rg -n "SearchDocuments" cmd/flow-server/main.go internal/adapter/httpserver/server.go`
Expected: declared on the struct (server.go), used in the handler (documents.go), constructed in main.go. Confirm no other non-test `httpserver.Server{` literal is missing the field (`rg -rn "httpserver.Server\{" cmd/ internal/ | rg -v _test`).

- [ ] **Step 3: Full CI gate**

Run: `make ci`
Expected: `lint`, `verify-generate`, `cover` (≥ 80%), `build` all green. If coverage dips below 80%, add targeted error-path tests (e.g. `SearchDocuments` store-error, `handleListDocuments` search 500 branch, `Client.Search` error) rather than lowering the gate. Paste the coverage %.

- [ ] **Step 4: Commit**

```bash
git add cmd/flow-server/main.go
git commit -m "feat(m2d): wire SearchDocuments into server composition root"
```

---

## Task 10: Live done-gate (manual, like M2a–c)

Not a code task — execute against the dev stack and record the result.

- [ ] **Step 1: Dev stack + migration 0009**

`make dev-up`, then run the M2d server build against it (note from M2c: a stale pre-M2d server may hold `:8080` — run on an alt port via `FLOW_LISTEN_ADDR=:8090 ./bin/flow-server` with `deploy/dev/flow.env` sourced if so). Confirm migration `0009` applied (`pg_trgm` created, `search` column + indexes) in the startup log.

- [ ] **Step 2: curl-smoke**

```bash
TOKEN=$(make -s dev-token); B=http://localhost:8090/api/v1/documents
curl -s -H "Authorization: Bearer $TOKEN" -X POST $B -d '{"type":"free","path":"komp","title":"Kompendium","body":"notes about the compendium"}'
curl -s -H "Authorization: Bearer $TOKEN" "$B?q=kompend"          # trigram → finds Kompendium, carries snippet
curl -s -H "Authorization: Bearer $TOKEN" "$B?q=compendium"        # FTS exact → ranked, highlighted snippet
curl -s -H "Authorization: Bearer $TOKEN" "$B?q=kompend&tag=missing" # composes → []
```

Expected: `?q=kompend` returns the doc with a `snippet` containing the `\x02/\x03` markers; tag composition narrows to `[]`. Clean up the test doc afterward (DELETE).

- [ ] **Step 3: Browser + TUI dogfood (Soenne, optional)**

Browser `/docs`: search box returns highlighted snippets, composes with the tag filter. TUI `flow docs`: `/` search, highlighted snippet line, `enter` opens, respects an active `f` filter. (Per M2c, Soenne may waive the interactive dogfood and accept the curl-smoke.)

- [ ] **Step 4: Record outcome**

Update the M2d memory note with done + commit range; report to Soenne; await confirmation before M2e (pgvector).
