# flow rebuild M2c — Tags & Filter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make tags first-class in the compendium — entered via YAML frontmatter in a document body, displayed in both hosts, and used to filter the document list (multiple tags, AND).

**Architecture:** Vertical slice mirroring M2a/M2b: domain (frontmatter authority) → store/migration (variadic tag-filtered List + GIN index) → usecases (derive tags on save, list_tags) → REST+SSE (`?tag` filter, `/documents/tags`) → apiclient → WebUI → TUI. `Document.Tags` (the existing `TEXT[]` column) stays the queryable source of truth; frontmatter is the editable representation parsed into it on save. The stored body keeps the frontmatter verbatim; renderers skip it.

**Tech Stack:** Go, pgx/v5, Postgres `TEXT[]` + GIN, `gopkg.in/yaml.v3`, goldmark + bluemonday (WebUI), templ, charm.land/bubbletea/v2 (TUI).

**Spec:** `docs/superpowers/specs/2026-06-15-flow-rebuild-m2c-tags-filter-design.md`

---

## File map

| File | Responsibility | Action |
|------|----------------|--------|
| `internal/domain/frontmatter.go` | Frontmatter syntax + tag normalization + `TagCount`/`CollectTags` | Create |
| `internal/domain/frontmatter_test.go` | Domain tests | Create |
| `internal/adapter/pgstore/migrations/0008_documents_tags_gin.sql` | GIN index on `tags` | Create |
| `internal/ports/ports.go` | `DocumentStore.List` → variadic `tags ...string` | Modify |
| `internal/testutil/fakes.go` | Fake `List` AND-filter | Modify |
| `internal/adapter/pgstore/documents.go` | pgstore `List` `tags @> $2` | Modify |
| `internal/usecase/list_documents.go` | `Execute(ctx, owner, tags []string)` | Modify |
| `internal/usecase/create_document.go` | Derive tags from frontmatter | Modify |
| `internal/usecase/update_document.go` | Derive tags; drop `Tags` input field | Modify |
| `internal/usecase/list_tags.go` | `ListTags` usecase | Create |
| `internal/usecase/document_test.go` | Update for new signatures | Modify |
| `internal/adapter/httpserver/documents.go` | `?tag` filter, `/tags` handler, drop `tags` from update req | Modify |
| `internal/adapter/httpserver/server.go` | `ListTags` field + route | Modify |
| `internal/adapter/apiclient/documents.go` | `ListDocuments(tags...)`, `Tags()`, drop `Tags` input | Modify |
| `internal/adapter/webui/markdown.go` | Skip frontmatter in `RenderMarkdown` | Modify |
| `internal/adapter/webui/wikilink.go` | Skip frontmatter in `RenderDocument` | Modify |
| `internal/adapter/webui/docs.templ` | Filter bar, tag chips, CSS | Modify |
| `internal/adapter/httpserver/webui_docs.go` | Parse `?tag`, build chips, drop FIX-4 hack | Modify |
| `internal/tui/docs.go` | Filter overlay, tag display, frontmatter skip | Modify |
| `cmd/flow-server/main.go` | Wire `ListTags` usecase | Modify |

Tasks are ordered so the build stays green after each. Signature changes that span an interface + all implementations + all callers are done within a single task.

---

## Task 1: Domain — frontmatter authority

**Files:**
- Create: `internal/domain/frontmatter.go`
- Test: `internal/domain/frontmatter_test.go`

- [ ] **Step 1: Write the failing test**

```go
package domain

import (
	"reflect"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantTags  []string
		wantStart int
	}{
		{"none", "# Title\n\nbody", nil, 0},
		{"inline list", "---\ntags: [go, tui]\n---\nbody\n", []string{"go", "tui"}, 24},
		{"block list", "---\ntags:\n  - Go\n  - TUI\n---\nrest", []string{"go", "tui"}, 29},
		{"normalize+dedupe", "---\ntags: [Go, go, \" TUI \", \"\"]\n---\nx", []string{"go", "tui"}, 36},
		{"no tags key", "---\ntitle: hi\n---\nbody", nil, 18},
		{"missing close fence", "---\ntags: [go]\nbody without close", nil, 0},
		{"unparseable yaml", "---\ntags: [go\n---\nbody", nil, 0},
		{"close at EOF", "---\ntags: [go]\n---", []string{"go"}, 18},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tags, start := ParseFrontmatter(tc.body)
			if !reflect.DeepEqual(tags, tc.wantTags) {
				t.Errorf("tags = %#v, want %#v", tags, tc.wantTags)
			}
			if start != tc.wantStart {
				t.Errorf("start = %d, want %d", start, tc.wantStart)
			}
			if start > 0 && start <= len(tc.body) {
				_ = tc.body[start:] // must not panic
			}
		})
	}
}

func TestCollectTags(t *testing.T) {
	docs := []Document{
		{Tags: []string{"go", "tui"}},
		{Tags: []string{"go"}},
		{Tags: []string{"web"}},
		{Tags: nil},
	}
	got := CollectTags(docs)
	want := []TagCount{{"go", 2}, {"tui", 1}, {"web", 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CollectTags = %#v, want %#v", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/ -run 'Frontmatter|CollectTags' -v`
Expected: FAIL — `undefined: ParseFrontmatter`, `undefined: TagCount`, `undefined: CollectTags`.

- [ ] **Step 3: Write the implementation**

Create `internal/domain/frontmatter.go`:

```go
package domain

import (
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// TagCount is a tag and how many documents carry it.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// ParseFrontmatter extracts tags from a leading YAML frontmatter block and
// reports the byte offset where the real body begins (for renderers to skip the
// block). A body that does not start with a "---\n" fence, has no closing
// "---"/"..." fence, or whose block is unparseable YAML yields (nil, 0) — the
// whole body is then treated as content. Tags are normalized: trimmed,
// lowercased, empties dropped, de-duplicated, first-seen order preserved.
func ParseFrontmatter(body string) (tags []string, bodyStart int) {
	const open = "---\n"
	if !strings.HasPrefix(body, open) {
		return nil, 0
	}
	rest := body[len(open):]

	end, after := -1, -1
	for off := 0; off <= len(rest); {
		nl := strings.IndexByte(rest[off:], '\n')
		var line string
		if nl < 0 {
			line = rest[off:]
		} else {
			line = rest[off : off+nl]
		}
		if line == "---" || line == "..." {
			end = off
			if nl < 0 {
				after = len(rest)
			} else {
				after = off + nl + 1
			}
			break
		}
		if nl < 0 {
			break
		}
		off += nl + 1
	}
	if end < 0 {
		return nil, 0
	}

	var fm struct {
		Tags []string `yaml:"tags"`
	}
	if err := yaml.Unmarshal([]byte(rest[:end]), &fm); err != nil {
		return nil, 0
	}
	return normalizeTags(fm.Tags), len(open) + after
}

// normalizeTags trims, lowercases, drops empties, and de-duplicates while
// preserving first-seen order. Returns nil for an empty result.
func normalizeTags(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range in {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// CollectTags aggregates tag counts across a document set, sorted by count
// descending then tag ascending. Reads Document.Tags.
func CollectTags(docs []Document) []TagCount {
	counts := map[string]int{}
	for _, d := range docs {
		for _, t := range d.Tags {
			counts[t]++
		}
	}
	out := make([]TagCount, 0, len(counts))
	for t, c := range counts {
		out = append(out, TagCount{Tag: t, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag < out[j].Tag
	})
	return out
}
```

- [ ] **Step 4: Promote yaml to a direct dependency**

Run: `go mod tidy`
Expected: `gopkg.in/yaml.v3` moves out of the `// indirect` group in `go.mod` (it is now imported directly).

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/domain/ -run 'Frontmatter|CollectTags' -v`
Expected: PASS (all subtests).

- [ ] **Step 6: Commit**

```bash
git add internal/domain/frontmatter.go internal/domain/frontmatter_test.go go.mod go.sum
git commit -m "feat(m2c): domain frontmatter parser + tag aggregation"
```

---

## Task 2: GIN index migration

**Files:**
- Create: `internal/adapter/pgstore/migrations/0008_documents_tags_gin.sql`

- [ ] **Step 1: Write the migration**

Create `internal/adapter/pgstore/migrations/0008_documents_tags_gin.sql`:

```sql
-- Speeds up the `tags @> ARRAY[...]` containment filter used by tag filtering.
CREATE INDEX documents_tags_gin ON documents USING GIN (tags);
```

- [ ] **Step 2: Verify it is embedded**

Run: `rg -n "embed|migrations" internal/adapter/pgstore/*.go | rg -i migrat`
Expected: a `//go:embed migrations/*.sql` directive — confirm the new file is picked up by the glob (no code change needed).

- [ ] **Step 3: Build to confirm embedding compiles**

Run: `go build ./internal/adapter/pgstore/`
Expected: success.

- [ ] **Step 4: Commit**

```bash
git add internal/adapter/pgstore/migrations/0008_documents_tags_gin.sql
git commit -m "feat(m2c): migration 0008 GIN index on documents.tags"
```

---

## Task 3: Store — variadic tag-filtered List (port + fake + pgstore)

**Files:**
- Modify: `internal/ports/ports.go:117`
- Modify: `internal/testutil/fakes.go:378`
- Modify: `internal/adapter/pgstore/documents.go:52`
- Test: `internal/testutil/fakes_test.go`
- Test: `internal/adapter/pgstore/documents_test.go`

- [ ] **Step 1: Write the failing fake test**

Add to `internal/testutil/fakes_test.go`:

```go
func TestFakeDocumentStore_ListTagFilter(t *testing.T) {
	s := NewFakeDocumentStore()
	ctx := context.Background()
	mk := func(id string, tags ...string) {
		if _, err := s.Create(ctx, domain.Document{ID: id, OwnerID: "u", Type: domain.DocFree, Path: id, Tags: tags}); err != nil {
			t.Fatal(err)
		}
	}
	mk("a", "go", "tui")
	mk("b", "go")
	mk("c", "web")

	all, _ := s.List(ctx, "u")
	if len(all) != 3 {
		t.Fatalf("unfiltered = %d, want 3", len(all))
	}
	goDocs, _ := s.List(ctx, "u", "go")
	if len(goDocs) != 2 {
		t.Fatalf("tag=go = %d, want 2", len(goDocs))
	}
	both, _ := s.List(ctx, "u", "go", "tui")
	if len(both) != 1 || both[0].ID != "a" {
		t.Fatalf("tag=go,tui = %#v, want [a]", both)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/testutil/ -run ListTagFilter -v`
Expected: FAIL — too many arguments to `s.List` (signature is still 2-arg).

- [ ] **Step 3: Change the port interface**

In `internal/ports/ports.go`, change line 117 from:

```go
	List(ctx context.Context, ownerID string) ([]domain.Document, error)
```

to:

```go
	// List returns the owner's documents newest-first. When tags are given, only
	// documents containing ALL of them are returned (AND semantics).
	List(ctx context.Context, ownerID string, tags ...string) ([]domain.Document, error)
```

- [ ] **Step 4: Update the fake**

In `internal/testutil/fakes.go`, replace the `List` method (starts line 378) signature and add AND-filtering:

```go
func (s *FakeDocumentStore) List(_ context.Context, ownerID string, tags ...string) ([]domain.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Document
	for _, d := range s.m {
		if d.OwnerID == ownerID && hasAllTags(d.Tags, tags) {
			out = append(out, d)
		}
	}
	// newest-first by UpdatedAt
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].UpdatedAt.After(out[i].UpdatedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

func hasAllTags(have, want []string) bool {
	for _, w := range want {
		found := false
		for _, h := range have {
			if h == w {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
```

- [ ] **Step 5: Update pgstore**

In `internal/adapter/pgstore/documents.go`, replace the `List` method (starts line 52):

```go
func (s *DocumentStore) List(ctx context.Context, ownerID string, tags ...string) ([]domain.Document, error) {
	q := `SELECT ` + docCols + ` FROM documents WHERE owner_id=$1`
	args := []any{ownerID}
	if len(tags) > 0 {
		q += ` AND tags @> $2`
		args = append(args, tags)
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

- [ ] **Step 6: Add a pgstore tag-filter test (DB-gated)**

In `internal/adapter/pgstore/documents_test.go`, add (follow the existing DB-skip helper used by other tests in this file — reuse whatever `testPool(t)`/`t.Skip` pattern is already present):

```go
func TestDocumentStore_ListTagFilter(t *testing.T) {
	pool := testPool(t) // existing helper: skips when no TEST_DATABASE_URL
	s := NewDocumentStore(pool)
	ctx := context.Background()
	owner := "tagfilter-" + randSuffix(t)

	mk := func(path string, tags ...string) {
		_, err := s.Create(ctx, domain.Document{
			ID: newID(t), OwnerID: owner, Type: domain.DocFree, Path: path, Tags: tags,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	mk("a", "go", "tui")
	mk("b", "go")

	got, err := s.List(ctx, owner, "go", "tui")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != "a" {
		t.Fatalf("List(go,tui) = %#v, want [a]", got)
	}
}
```

If the existing test file uses different helper names (`testPool`, `randSuffix`, `newID`), match them exactly — read the top of `documents_test.go` first and copy its setup idiom rather than introducing new helpers.

- [ ] **Step 7: Run tests**

Run: `go test ./internal/testutil/ ./internal/adapter/pgstore/ -run 'ListTagFilter' -v`
Expected: fake PASS; pgstore PASS if a test DB is configured, else SKIP.

- [ ] **Step 8: Build the whole module to catch broken callers**

Run: `go build ./...`
Expected: success. Existing 2-arg `List(ctx, owner)` calls still compile (variadic). If anything fails, it is an unrelated call site — fix to the variadic form.

- [ ] **Step 9: Commit**

```bash
git add internal/ports/ports.go internal/testutil/fakes.go internal/adapter/pgstore/documents.go internal/testutil/fakes_test.go internal/adapter/pgstore/documents_test.go
git commit -m "feat(m2c): variadic tag-filtered DocumentStore.List (AND)"
```

---

## Task 4: usecase — ListDocuments passes tags through

**Files:**
- Modify: `internal/usecase/list_documents.go`
- Modify callers (compile-fix to `nil`): `internal/adapter/httpserver/documents.go:50`, `internal/adapter/httpserver/webui_docs.go:14,62`
- Test: `internal/usecase/document_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/usecase/document_test.go`:

```go
func TestListDocuments_TagFilter(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	ctx := context.Background()
	for _, d := range []domain.Document{
		{ID: "a", OwnerID: "u", Type: domain.DocFree, Path: "a", Tags: []string{"go", "tui"}},
		{ID: "b", OwnerID: "u", Type: domain.DocFree, Path: "b", Tags: []string{"go"}},
	} {
		if _, err := docs.Create(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	uc := usecase.ListDocuments{Docs: docs}
	got, err := uc.Execute(ctx, "u", []string{"go", "tui"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("got %#v, want [a]", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/usecase/ -run ListDocuments_TagFilter -v`
Expected: FAIL — not enough arguments to `uc.Execute` (still 2-arg).

- [ ] **Step 3: Change the usecase**

Replace `internal/usecase/list_documents.go` body:

```go
func (uc ListDocuments) Execute(ctx context.Context, ownerID string, tags []string) ([]domain.Document, error) {
	return uc.Docs.List(ctx, ownerID, tags...)
}
```

- [ ] **Step 4: Fix the three callers to compile**

In `internal/adapter/httpserver/documents.go:50`:

```go
	list, err := s.ListDocuments.Execute(r.Context(), u.ID, nil)
```

In `internal/adapter/httpserver/webui_docs.go:14` (inside `docsListData`):

```go
	list, err := s.ListDocuments.Execute(r.Context(), u.ID, nil)
```

In `internal/adapter/httpserver/webui_docs.go:62` (inside `handleWebDocView`):

```go
	all, err := s.ListDocuments.Execute(r.Context(), u.ID, nil)
```

(`?tag` parsing is added in Tasks 7 and 10; for now `nil` preserves current behavior.)

- [ ] **Step 5: Run tests + build**

Run: `go test ./internal/usecase/ -run ListDocuments_TagFilter -v && go build ./...`
Expected: PASS + build success.

- [ ] **Step 6: Commit**

```bash
git add internal/usecase/list_documents.go internal/adapter/httpserver/documents.go internal/adapter/httpserver/webui_docs.go internal/usecase/document_test.go
git commit -m "feat(m2c): ListDocuments accepts AND tag filter"
```

---

## Task 5: usecase — derive tags from frontmatter on create/update

**Files:**
- Modify: `internal/usecase/create_document.go`
- Modify: `internal/usecase/update_document.go`
- Modify: `internal/adapter/httpserver/documents.go` (drop `Tags` from `updateDocReq`)
- Modify: `internal/adapter/httpserver/webui_docs.go` (drop FIX-4 preserve-tags)
- Modify: `internal/adapter/apiclient/documents.go` (drop `Tags` from `UpdateDocumentInput`)
- Test: `internal/usecase/document_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/usecase/document_test.go`:

```go
func TestCreateDocument_TagsFromFrontmatter(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	uc := usecase.CreateDocument{Docs: docs, IDs: testutil.FakeIDGen{}, Clock: testutil.FixedClock{}}
	got, err := uc.Execute(context.Background(), "u", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "note", Title: "Note",
		Body: "---\ntags: [Go, go, tui]\n---\nhello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"go", "tui"}; !reflect.DeepEqual(got.Tags, want) {
		t.Fatalf("tags = %#v, want %#v", got.Tags, want)
	}
	if !strings.HasPrefix(got.Body, "---\n") {
		t.Fatal("body must keep frontmatter verbatim")
	}
}

func TestUpdateDocument_TagsFromFrontmatter(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	ctx := context.Background()
	seed, _ := docs.Create(ctx, domain.Document{
		ID: "d1", OwnerID: "u", Type: domain.DocFree, Path: "n", Tags: []string{"old"},
	})
	uc := usecase.UpdateDocument{Docs: docs, Clock: testutil.FixedClock{}}
	got, err := uc.Execute(ctx, "u", seed.ID, usecase.UpdateDocumentInput{
		Title: "N", Body: "---\ntags: [new]\n---\nbody",
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"new"}; !reflect.DeepEqual(got.Tags, want) {
		t.Fatalf("tags = %#v, want %#v", got.Tags, want)
	}
}
```

Use whatever fake `IDGen`/`Clock` the existing tests in this file use — read the top of `document_test.go` and match the exact helper names (the snippet above assumes `testutil.FakeIDGen{}` / `testutil.FixedClock{}`; replace with the real ones). Ensure `reflect` and `strings` are imported.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/usecase/ -run 'TagsFromFrontmatter' -v`
Expected: FAIL — created/updated `Tags` is empty (not yet derived).

- [ ] **Step 3: Derive tags in create**

In `internal/usecase/create_document.go`, inside `Execute`, after building `d` and before `d.Validate()` (around line 32), add:

```go
	d.Tags, _ = domain.ParseFrontmatter(d.Body)
```

- [ ] **Step 4: Derive tags in update + drop the Tags input field**

In `internal/usecase/update_document.go`:

Remove `Tags []string` from `UpdateDocumentInput` (lines 17-21 become):

```go
type UpdateDocumentInput struct {
	Title string
	Body  string
}
```

Change line 28 from:

```go
	cur.Title, cur.Body, cur.Tags = in.Title, in.Body, in.Tags
```

to:

```go
	cur.Title, cur.Body = in.Title, in.Body
	cur.Tags, _ = domain.ParseFrontmatter(in.Body)
```

- [ ] **Step 5: Fix the REST update handler**

In `internal/adapter/httpserver/documents.go`, change `updateDocReq` (lines 74-78) to drop `Tags`:

```go
type updateDocReq struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}
```

and the `UpdateDocument.Execute` call (lines 87-91) to drop `Tags`:

```go
	doc, err := s.UpdateDocument.Execute(r.Context(), u.ID, r.PathValue("id"), usecase.UpdateDocumentInput{
		Title: req.Title,
		Body:  req.Body,
	})
```

- [ ] **Step 6: Remove the WebUI FIX-4 preserve-tags hack**

In `internal/adapter/httpserver/webui_docs.go`, in `handleWebDocUpdate`, delete the existing-doc fetch added for tag preservation (lines 172-181, the comment + `s.GetDocument.Execute(...)` block) and change the update call (lines 183-187) to:

```go
	_, err := s.UpdateDocument.Execute(r.Context(), u.ID, id, usecase.UpdateDocumentInput{
		Title: r.FormValue("title"),
		Body:  r.FormValue("body"),
	})
```

(`err` is now first declared here — use `:=`. Confirm no remaining reference to `existing`.)

- [ ] **Step 7: Drop Tags from the apiclient input**

In `internal/adapter/apiclient/documents.go`, change `UpdateDocumentInput` (lines 37-42) to:

```go
// UpdateDocumentInput mirrors the server's update payload.
type UpdateDocumentInput struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}
```

- [ ] **Step 8: Fix any test/caller still passing Tags**

Run: `rg -n "UpdateDocumentInput\{" internal/ cmd/ | rg -i tags`
For each hit, remove the `Tags:` field. In `internal/usecase/document_test.go:103` the existing happy-path update test sets `Tags: []string{"go"}` — remove that line and, if it asserted the tag survived, update the assertion to expect tags derived from the body instead (or drop the tag assertion).

- [ ] **Step 9: Run tests + build**

Run: `go test ./internal/usecase/ ./internal/adapter/httpserver/ ./internal/adapter/apiclient/ && go build ./...`
Expected: PASS + build success.

- [ ] **Step 10: Commit**

```bash
git add internal/usecase/create_document.go internal/usecase/update_document.go internal/adapter/httpserver/documents.go internal/adapter/httpserver/webui_docs.go internal/adapter/apiclient/documents.go internal/usecase/document_test.go
git commit -m "feat(m2c): derive tags from frontmatter; drop explicit tags from write API"
```

---

## Task 6: usecase — ListTags

**Files:**
- Create: `internal/usecase/list_tags.go`
- Test: `internal/usecase/document_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/usecase/document_test.go`:

```go
func TestListTags(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	ctx := context.Background()
	for _, d := range []domain.Document{
		{ID: "a", OwnerID: "u", Type: domain.DocFree, Path: "a", Tags: []string{"go", "tui"}},
		{ID: "b", OwnerID: "u", Type: domain.DocFree, Path: "b", Tags: []string{"go"}},
	} {
		if _, err := docs.Create(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	uc := usecase.ListTags{Docs: docs}
	got, err := uc.Execute(ctx, "u")
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.TagCount{{Tag: "go", Count: 2}, {Tag: "tui", Count: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/usecase/ -run TestListTags -v`
Expected: FAIL — `undefined: usecase.ListTags`.

- [ ] **Step 3: Write the implementation**

Create `internal/usecase/list_tags.go`:

```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ListTags aggregates the owner's document tags with per-tag counts.
type ListTags struct{ Docs ports.DocumentStore }

func (uc ListTags) Execute(ctx context.Context, ownerID string) ([]domain.TagCount, error) {
	docs, err := uc.Docs.List(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	return domain.CollectTags(docs), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/usecase/ -run TestListTags -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/list_tags.go internal/usecase/document_test.go
git commit -m "feat(m2c): ListTags usecase"
```

---

## Task 7: REST — ?tag filter + /documents/tags endpoint

**Files:**
- Modify: `internal/adapter/httpserver/documents.go` (`handleListDocuments`, new `handleListTags`)
- Modify: `internal/adapter/httpserver/server.go` (field + route)
- Test: `internal/adapter/httpserver/documents_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/adapter/httpserver/documents_test.go` (mirror the existing test harness in this file — reuse its `newTestServer`/auth-request helpers; the names below are illustrative, match the real ones):

```go
func TestHandleListDocuments_TagFilter(t *testing.T) {
	srv, docs := newDocsTestServer(t) // existing helper returning *Server + fake store
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{ID: "a", OwnerID: testUserID, Type: domain.DocFree, Path: "a", Tags: []string{"go", "tui"}})
	_, _ = docs.Create(ctx, domain.Document{ID: "b", OwnerID: testUserID, Type: domain.DocFree, Path: "b", Tags: []string{"go"}})

	rec := doAuthedGET(t, srv, "/api/v1/documents?tag=go&tag=tui")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var out []domain.Document
	mustJSON(t, rec, &out)
	if len(out) != 1 || out[0].ID != "a" {
		t.Fatalf("got %#v, want [a]", out)
	}
}

func TestHandleListTags(t *testing.T) {
	srv, docs := newDocsTestServer(t)
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{ID: "a", OwnerID: testUserID, Type: domain.DocFree, Path: "a", Tags: []string{"go", "tui"}})
	_, _ = docs.Create(ctx, domain.Document{ID: "b", OwnerID: testUserID, Type: domain.DocFree, Path: "b", Tags: []string{"go"}})

	rec := doAuthedGET(t, srv, "/api/v1/documents/tags")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var out []domain.TagCount
	mustJSON(t, rec, &out)
	if len(out) != 2 || out[0].Tag != "go" || out[0].Count != 2 {
		t.Fatalf("got %#v", out)
	}
}
```

Read `documents_test.go` first and adapt to its actual helpers (the file already has list/create tests around lines 49-250 — copy that exact setup, including how `ListTags` must be added to the `&Server{...}` literal in the helper).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapter/httpserver/ -run 'TagFilter|ListTags' -v`
Expected: FAIL — `?tag` ignored / `/documents/tags` 404 / `ListTags` field missing.

- [ ] **Step 3: Parse ?tag in the list handler**

In `internal/adapter/httpserver/documents.go`, change `handleListDocuments` (line 50):

```go
	list, err := s.ListDocuments.Execute(r.Context(), u.ID, r.URL.Query()["tag"])
```

- [ ] **Step 4: Add the tags handler**

Append to `internal/adapter/httpserver/documents.go`:

```go
func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	tags, err := s.ListTags.Execute(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if tags == nil {
		tags = []domain.TagCount{}
	}
	writeJSON(w, http.StatusOK, tags)
}
```

- [ ] **Step 5: Wire the server field + route**

In `internal/adapter/httpserver/server.go`, add to the documents field group (after line 49 `BacklinksDocument usecase.Backlinks`):

```go
	ListTags          usecase.ListTags
```

And register the route in the m2a documents block (after line 92). Place it before the `{id}` route for clarity (Go's mux prefers the literal `tags` segment over `{id}` regardless of order, but grouping it with the list route reads best):

```go
	mux.Handle("GET /api/v1/documents/tags", s.auth(http.HandlerFunc(s.handleListTags)))
```

- [ ] **Step 6: Run tests + build**

Run: `go test ./internal/adapter/httpserver/ -run 'TagFilter|ListTags' -v && go build ./...`
Expected: PASS + build success.

- [ ] **Step 7: Commit**

```bash
git add internal/adapter/httpserver/documents.go internal/adapter/httpserver/server.go internal/adapter/httpserver/documents_test.go
git commit -m "feat(m2c): REST ?tag filter + GET /documents/tags"
```

---

## Task 8: apiclient — ListDocuments(tags...) + Tags()

**Files:**
- Modify: `internal/adapter/apiclient/documents.go`
- Test: `internal/adapter/apiclient/documents_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/adapter/apiclient/documents_test.go` (mirror the existing httptest-server pattern in this file):

```go
func TestListDocuments_TagQuery(t *testing.T) {
	var gotQuery string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte("[]"))
	})
	if _, err := c.ListDocuments(context.Background(), "go", "tui"); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "tag=go&tag=tui" {
		t.Fatalf("query = %q, want tag=go&tag=tui", gotQuery)
	}
}

func TestClientTags(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/documents/tags" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"tag":"go","count":2}]`))
	})
	got, err := c.Tags(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Tag != "go" || got[0].Count != 2 {
		t.Fatalf("got %#v", got)
	}
}
```

Match the real test-client constructor name used elsewhere in `documents_test.go` (e.g. the helper that wraps an `httptest.Server`).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapter/apiclient/ -run 'TagQuery|ClientTags' -v`
Expected: FAIL — `ListDocuments` takes no tags / `undefined: c.Tags`.

- [ ] **Step 3: Implement**

In `internal/adapter/apiclient/documents.go`, add `net/url` to imports and replace `ListDocuments` (lines 25-29):

```go
func (c *Client) ListDocuments(ctx context.Context, tags ...string) ([]domain.Document, error) {
	path := "/api/v1/documents"
	if len(tags) > 0 {
		q := url.Values{}
		for _, t := range tags {
			q.Add("tag", t)
		}
		path += "?" + q.Encode()
	}
	var out []domain.Document
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (c *Client) Tags(ctx context.Context) ([]domain.TagCount, error) {
	var out []domain.TagCount
	err := c.do(ctx, http.MethodGet, "/api/v1/documents/tags", nil, &out)
	return out, err
}
```

- [ ] **Step 4: Run tests + build**

Run: `go test ./internal/adapter/apiclient/ -run 'TagQuery|ClientTags' -v && go build ./...`
Expected: PASS + build success (existing no-tag `ListDocuments(ctx)` callers still compile).

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/apiclient/documents.go internal/adapter/apiclient/documents_test.go
git commit -m "feat(m2c): apiclient ListDocuments(tags...) + Tags()"
```

---

## Task 9: WebUI renderers — skip the frontmatter block

**Files:**
- Modify: `internal/adapter/webui/markdown.go`
- Modify: `internal/adapter/webui/wikilink.go`
- Test: `internal/adapter/webui/markdown_test.go` (create if absent), `internal/adapter/webui/wikilink_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/adapter/webui/markdown_test.go` (create the file with `package webui` if it does not exist):

```go
func TestRenderMarkdown_SkipsFrontmatter(t *testing.T) {
	html := string(RenderMarkdown("---\ntags: [go]\n---\n# Hello\n"))
	if strings.Contains(html, "tags:") {
		t.Fatalf("frontmatter leaked into output: %q", html)
	}
	if !strings.Contains(html, "<h1") {
		t.Fatalf("body heading missing: %q", html)
	}
}
```

Add to `internal/adapter/webui/wikilink_test.go`:

```go
func TestRenderDocument_SkipsFrontmatter(t *testing.T) {
	resolve := func(string) (string, string, bool) { return "", "", false }
	html := string(RenderDocument("---\ntags: [go]\n---\n[[x]]\n", resolve))
	if strings.Contains(html, "tags:") {
		t.Fatalf("frontmatter leaked: %q", html)
	}
}
```

Ensure `strings` is imported in both test files.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapter/webui/ -run SkipsFrontmatter -v`
Expected: FAIL — frontmatter renders as content (likely an `<hr>` + text).

- [ ] **Step 3: Skip frontmatter in RenderMarkdown**

In `internal/adapter/webui/markdown.go`, add the domain import and slice before converting:

```go
import (
	"bytes"
	"html/template"

	"github.com/microcosm-cc/bluemonday"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/yuin/goldmark"
)
```

In `RenderMarkdown`, at the top of the function body:

```go
func RenderMarkdown(src string) template.HTML {
	if _, start := domain.ParseFrontmatter(src); start > 0 {
		src = src[start:]
	}
	var buf bytes.Buffer
	...
}
```

- [ ] **Step 4: Skip frontmatter in RenderDocument**

In `internal/adapter/webui/wikilink.go`, add the domain import and slice at the top of `RenderDocument`:

```go
func RenderDocument(src string, resolve WikilinkResolver) template.HTML {
	if _, start := domain.ParseFrontmatter(src); start > 0 {
		src = src[start:]
	}
	gm := goldmark.New(
	...
}
```

Add `"github.com/serverkraken/flow/internal/domain"` to `wikilink.go` imports.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/adapter/webui/ -run SkipsFrontmatter -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/webui/markdown.go internal/adapter/webui/wikilink.go internal/adapter/webui/markdown_test.go internal/adapter/webui/wikilink_test.go
git commit -m "feat(m2c): WebUI renderers skip frontmatter block"
```

---

## Task 10: WebUI — filter bar, tag chips, view-model wiring

**Files:**
- Modify: `internal/adapter/webui/docs.templ` (view models + templates + CSS)
- Modify: `internal/adapter/httpserver/webui_docs.go` (`docsListData`, `handleWebDocView`)
- Modify: `internal/adapter/httpserver/server.go` (only if `ListTags` not yet present — already added in Task 7)
- Test: `internal/adapter/httpserver/webui_docs_test.go`

- [ ] **Step 1: Extend the view models in docs.templ**

In `internal/adapter/webui/docs.templ`, extend the structs:

`DocsPageData` (lines 6-12) — add filter state:

```go
type DocsPageData struct {
	User       string
	Docs       []DocRow
	Current    *DocDetail
	Error      string
	Form       *DocFormValues
	AllTags    []TagChip // every tag with count, for the filter bar
	ActiveTags []string  // currently applied filter
	Query      string    // encoded "?tag=..." to preserve filter on SSE refresh ("" when none)
}

// TagChip is one tag in the filter bar.
type TagChip struct {
	Tag    string
	Count  int
	Active bool
	// Href toggles this tag in the current filter.
	Href string
}
```

`DocRow` (lines 15-20) — add `Tags`:

```go
type DocRow struct {
	ID    string
	Type  string
	Path  string
	Title string
	Tags  []string
}
```

`DocDetail` (lines 23-31) — add `Tags`:

```go
type DocDetail struct {
	ID        string
	Type      string
	Path      string
	Title     string
	HTML      template.HTML
	Body      string
	Tags      []string
	Backlinks []DocRow
}
```

- [ ] **Step 2: Add the filter bar + chips + CSS to the templates**

In `DocsListFragment` (lines 76-98), insert a filter bar above the list and tag chips on each row. Replace the template body with:

```go
templ DocsListFragment(d DocsPageData) {
	<div class="mb-4 flex items-center justify-between">
		<h2 class="text-sm font-semibold uppercase tracking-wide text-slate-500">Documents</h2>
		<a href="/docs/new" class="rounded bg-slate-900 px-3 py-1 text-xs text-white hover:bg-slate-700">+ new doc</a>
	</div>
	if len(d.AllTags) > 0 {
		<div class="mb-3 flex flex-wrap items-center gap-1">
			for _, c := range d.AllTags {
				if c.Active {
					<a href={ templ.SafeURL(c.Href) } class="rounded-full bg-slate-900 px-2 py-0.5 text-xs text-white">#{ c.Tag } ({ fmt.Sprint(c.Count) })</a>
				} else {
					<a href={ templ.SafeURL(c.Href) } class="rounded-full bg-slate-100 px-2 py-0.5 text-xs text-slate-600 hover:bg-slate-200">#{ c.Tag } ({ fmt.Sprint(c.Count) })</a>
				}
			}
			if len(d.ActiveTags) > 0 {
				<a href="/docs" class="px-2 py-0.5 text-xs text-slate-400 hover:text-slate-600">Filter zurücksetzen</a>
			}
		</div>
	}
	if len(d.Docs) == 0 {
		<p class="text-sm text-slate-400">no documents</p>
	} else {
		<ul class="divide-y divide-slate-100">
			for _, doc := range d.Docs {
				<li class="py-2">
					<a href={ templ.SafeURL("/docs/" + doc.ID) } class="block hover:text-slate-700">
						<div class="flex items-center justify-between text-sm">
							<span class="font-medium text-slate-800">{ doc.Title }</span>
							<span class="text-xs text-slate-400">{ doc.Type }</span>
						</div>
						<div class="mt-0.5 font-mono text-xs text-slate-500">{ doc.Path }</div>
						if len(doc.Tags) > 0 {
							<div class="mt-1 flex flex-wrap gap-1">
								for _, t := range doc.Tags {
									<span class="rounded-full bg-slate-100 px-2 py-0.5 text-xs text-slate-500">#{ t }</span>
								}
							</div>
						}
					</a>
				</li>
			}
		</ul>
	}
}
```

Add `import "fmt"` to the top of `docs.templ` (alongside `import "html/template"`).

Make the SSE-refresh container preserve the filter: in `DocsPage` (line 55) change the `hx-get` to include the query:

```go
	<div id="dc" hx-get={ "/ui/docs/list" + d.Query } hx-trigger="sse:document.created, sse:document.updated, sse:document.deleted" hx-swap="innerHTML">
```

In `DocView` (lines 142-150), render the open document's tag chips under the path line, before the prose block:

```go
				<div class="mb-3 font-mono text-xs text-slate-400">{ d.Current.Path }</div>
				if len(d.Current.Tags) > 0 {
					<div class="mb-3 flex flex-wrap gap-1">
						for _, t := range d.Current.Tags {
							<a href={ templ.SafeURL("/docs?tag=" + t) } class="rounded-full bg-slate-100 px-2 py-0.5 text-xs text-slate-500 hover:bg-slate-200">#{ t }</a>
						}
					</div>
				}
				<div class="prose prose-sm max-w-none text-slate-700">
```

- [ ] **Step 3: Regenerate templ**

Run: `make generate`
Expected: `internal/adapter/webui/docs_templ.go` updates with no errors.

- [ ] **Step 4: Build the chips + filtered list in the handler**

In `internal/adapter/httpserver/webui_docs.go`, rewrite `docsListData` to accept the active tags, build chips via `ListTags`, and populate rows with their tags:

```go
func (s *Server) docsListData(r *http.Request, u domain.User) (webui.DocsPageData, error) {
	active := r.URL.Query()["tag"]
	list, err := s.ListDocuments.Execute(r.Context(), u.ID, active)
	if err != nil {
		return webui.DocsPageData{}, err
	}
	allTags, err := s.ListTags.Execute(r.Context(), u.ID)
	if err != nil {
		return webui.DocsPageData{}, err
	}

	activeSet := map[string]bool{}
	for _, t := range active {
		activeSet[t] = true
	}
	chips := make([]webui.TagChip, 0, len(allTags))
	for _, tc := range allTags {
		chips = append(chips, webui.TagChip{
			Tag:    tc.Tag,
			Count:  tc.Count,
			Active: activeSet[tc.Tag],
			Href:   toggleTagHref(active, tc.Tag),
		})
	}

	rows := make([]webui.DocRow, 0, len(list))
	for _, d := range list {
		rows = append(rows, webui.DocRow{
			ID: d.ID, Type: string(d.Type), Path: d.Path, Title: d.Title, Tags: d.Tags,
		})
	}
	return webui.DocsPageData{
		User: u.Username, Docs: rows,
		AllTags: chips, ActiveTags: active, Query: encodeTagQuery(active),
	}, nil
}

// toggleTagHref returns the /docs URL with tag added to (or removed from) the
// current filter set.
func toggleTagHref(active []string, tag string) string {
	var next []string
	removed := false
	for _, t := range active {
		if t == tag {
			removed = true
			continue
		}
		next = append(next, t)
	}
	if !removed {
		next = append(next, tag)
	}
	return "/docs" + encodeTagQuery(next)
}

func encodeTagQuery(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	q := url.Values{}
	for _, t := range tags {
		q.Add("tag", t)
	}
	return "?" + q.Encode()
}
```

Add `"net/url"` to the imports of `webui_docs.go`.

- [ ] **Step 5: Pass tags into the document view**

In `handleWebDocView` (lines 85-91), add `Tags: doc.Tags` to the `DocDetail`:

```go
		Current: &webui.DocDetail{
			ID: doc.ID, Type: string(doc.Type), Path: doc.Path, Title: doc.Title,
			HTML: rendered, Body: doc.Body, Tags: doc.Tags, Backlinks: blRows,
		},
```

- [ ] **Step 6: Write/extend the handler test**

Add to `internal/adapter/httpserver/webui_docs_test.go` (match the existing WebUI test harness — it already builds a `*Server` with the doc usecases; add `ListTags: usecase.ListTags{Docs: docs}` to that literal):

```go
func TestWebDocsList_FilterBarAndChips(t *testing.T) {
	srv, docs := newWebDocsTestServer(t) // existing helper
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{ID: "a", OwnerID: testUserID, Type: domain.DocFree, Path: "a", Title: "A", Tags: []string{"go", "tui"}})
	_, _ = docs.Create(ctx, domain.Document{ID: "b", OwnerID: testUserID, Type: domain.DocFree, Path: "b", Title: "B", Tags: []string{"web"}})

	// Filter bar lists all tags.
	body := doAuthedGETBody(t, srv, "/docs")
	if !strings.Contains(body, "#go") || !strings.Contains(body, "#web") {
		t.Fatalf("filter bar missing tags: %s", body)
	}
	// Filtering narrows the list (AND).
	body = doAuthedGETBody(t, srv, "/ui/docs/list?tag=go&tag=tui")
	if !strings.Contains(body, ">A<") || strings.Contains(body, ">B<") {
		t.Fatalf("tag filter wrong: %s", body)
	}
}
```

Adapt helper names to the file's real ones.

- [ ] **Step 7: Run tests + ci-verify generate**

Run: `go test ./internal/adapter/httpserver/ -run 'WebDocs' -v && make verify-generate`
Expected: PASS + `verify-generate: OK`.

- [ ] **Step 8: Commit**

```bash
git add internal/adapter/webui/docs.templ internal/adapter/webui/docs_templ.go internal/adapter/httpserver/webui_docs.go internal/adapter/httpserver/webui_docs_test.go
git commit -m "feat(m2c): WebUI tag chips + AND filter bar"
```

---

## Task 11: TUI — tag display, frontmatter skip, filter overlay

**Files:**
- Modify: `internal/tui/docs.go`
- Test: `internal/tui/docs_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/tui/docs_test.go` (match the existing test idiom — these drive `Update`/`View` with a nil client/editor/opener):

```go
func TestDocs_FilterOverlayToggleApply(t *testing.T) {
	m := NewDocs(nil, nil, nil, "u")
	m.docs = []domain.Document{{ID: "a", Type: domain.DocFree, Path: "a", Tags: []string{"go"}}}
	m.filterOpts = []domain.TagCount{{Tag: "go", Count: 1}, {Tag: "tui", Count: 2}}
	m.mode = modeFiltering

	// space toggles the cursor tag into the working set
	m2, _ := m.Update(keyPress(" "))
	dm := m2.(DocsModel)
	if len(dm.filterWork) != 1 || dm.filterWork[0] != "go" {
		t.Fatalf("working set = %#v, want [go]", dm.filterWork)
	}
	// enter commits and leaves filter mode
	m3, _ := dm.Update(keyPress("enter"))
	dm = m3.(DocsModel)
	if dm.mode != modeList {
		t.Fatalf("mode = %v, want modeList", dm.mode)
	}
	if len(dm.filterTags) != 1 || dm.filterTags[0] != "go" {
		t.Fatalf("active filter = %#v, want [go]", dm.filterTags)
	}
}

func TestDocs_RenderViewSkipsFrontmatter(t *testing.T) {
	m := NewDocs(nil, nil, nil, "u")
	d := domain.Document{ID: "a", Type: domain.DocFree, Path: "a", Title: "A",
		Body: "---\ntags: [go]\n---\nhello body", Tags: []string{"go"}}
	m.viewing = &d
	m.mode = modeView
	var b strings.Builder
	m.renderView(&b)
	out := b.String()
	if strings.Contains(out, "tags:") {
		t.Fatalf("frontmatter leaked into view: %q", out)
	}
	if !strings.Contains(out, "hello body") {
		t.Fatalf("body missing: %q", out)
	}
}
```

Use the test file's existing key-event helper (named `keyPress` above — replace with the real one, e.g. a `tea.KeyPressMsg{...}` constructor already used in `docs_test.go`).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run 'FilterOverlay|RenderViewSkips' -v`
Expected: FAIL — `modeFiltering`, `filterOpts`, `filterWork`, `filterTags` undefined; frontmatter still rendered.

- [ ] **Step 3: Add filter state to the model**

In `internal/tui/docs.go`, add the new mode constant (in the `const (...)` block at line 32):

```go
	modeFiltering // selecting tags to filter the list
```

Add fields to `DocsModel` (after line 71 `backlinks []domain.BacklinkRef`):

```go
	filterTags   []string         // applied filter (AND)
	filterWork   []string         // working set while in modeFiltering
	filterOpts   []domain.TagCount // available tags for the overlay
	filterCursor int
```

- [ ] **Step 4: Apply the active filter in reload + add a tags loader**

Change `reload` (line 120) to pass the active filter:

```go
		docs, err := m.client.ListDocuments(ctx, m.filterTags...)
```

Add a tags-loader command and message near the other cmds (after `loadBacklinks`, line 174):

```go
type tagsLoadedMsg struct{ tags []domain.TagCount }

func (m DocsModel) loadTags() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		tags, err := m.client.Tags(ctx)
		if err != nil {
			return errMsg{err}
		}
		return tagsLoadedMsg{tags: tags}
	}
}
```

Handle `tagsLoadedMsg` in `Update` (add a case alongside the others, e.g. after `backlinksMsg` at line 279):

```go
	case tagsLoadedMsg:
		m.filterOpts = msg.tags
		m.filterWork = append([]string(nil), m.filterTags...)
		m.filterCursor = 0
		m.mode = modeFiltering
		return m, nil
```

- [ ] **Step 5: Open the overlay from the list + handle its keys**

In the `modeList` switch in `handleKey` (after the `d` delete case, around line 388), add:

```go
	case k.Text == "f":
		return m, m.loadTags()
```

Dispatch the filter mode in `handleKey`'s mode switch (line 314, add a case):

```go
	case modeFiltering:
		return m.handleFilterKey(k)
```

Add the handler:

```go
func (m DocsModel) handleFilterKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		m.mode = modeList // discard working changes
		return m, nil
	case k.Text == "j":
		if m.filterCursor < len(m.filterOpts)-1 {
			m.filterCursor++
		}
		return m, nil
	case k.Text == "k":
		if m.filterCursor > 0 {
			m.filterCursor--
		}
		return m, nil
	case k.Text == " ":
		if m.filterCursor < len(m.filterOpts) {
			m.filterWork = toggleStr(m.filterWork, m.filterOpts[m.filterCursor].Tag)
		}
		return m, nil
	case k.Text == "c":
		m.filterWork = nil
		return m, nil
	case k.Code == tea.KeyEnter:
		m.filterTags = append([]string(nil), m.filterWork...)
		m.mode = modeList
		m.sel = 0
		return m, m.reload()
	}
	return m, nil
}

// toggleStr adds s to xs if absent, removes it if present.
func toggleStr(xs []string, s string) []string {
	for i, x := range xs {
		if x == s {
			return append(xs[:i:i], xs[i+1:]...)
		}
	}
	return append(xs, s)
}
```

- [ ] **Step 6: Strip frontmatter for the view + build links on the real body**

In the `docViewMsg` case (lines 256-263), strip the frontmatter from the viewing copy so both the rendered body and the link set ignore it:

```go
	case docViewMsg:
		d := msg.doc
		if _, start := domain.ParseFrontmatter(d.Body); start > 0 {
			d.Body = d.Body[start:]
		}
		m.viewing = &d
		m.mode = modeView
		m.viewLinks = buildBodyLinks(d.Body, d, m.docs)
		m.linkFocus = -1
		m.backlinks = nil
		return m, m.loadBacklinks(d.ID)
```

(`d.Tags` is untouched — the server already populated it, so the header shows tags regardless of stripping.)

- [ ] **Step 7: Render tags on rows, header, and add the overlay view**

In `renderList` (lines 707-720), append tags to each row line. Replace the loop body:

```go
	for i, d := range m.docs {
		label := fmt.Sprintf("  %-7s %s  %s", d.Type, d.Path, d.Title)
		if i == m.sel {
			label = styleSel.Render(fmt.Sprintf("▸ %-7s %s  %s", d.Type, d.Path, d.Title))
		}
		if len(d.Tags) > 0 {
			label += styleMuted.Render("  " + tagSuffix(d.Tags))
		}
		b.WriteString(label + "\n")
	}
```

Add a header line for the active filter at the top of `renderList` (after the `styleHeader.Render("Documents")` line 708):

```go
	if len(m.filterTags) > 0 {
		b.WriteString(styleMuted.Render("  filter: "+tagSuffix(m.filterTags)) + "\n")
	}
```

Add the tag suffix helper and the view header tags. In `renderView` (line 728), append tags to the header:

```go
	hdr := styleHeader.Render(d.Title) + styleMuted.Render("  "+string(d.Type)+" · "+d.Path)
	if len(d.Tags) > 0 {
		hdr += styleMuted.Render("  " + tagSuffix(d.Tags))
	}
	b.WriteString(hdr + "\n\n")
```

Add the helper near the other render helpers:

```go
func tagSuffix(tags []string) string {
	parts := make([]string, len(tags))
	for i, t := range tags {
		parts[i] = "#" + t
	}
	return strings.Join(parts, " ")
}
```

Add a `renderFilter` view and dispatch it. In the main `View`/render switch (the `switch m.mode` around line 660 that calls `renderView`/`renderList`), add a `modeFiltering` branch that calls `m.renderFilter(&b)`:

```go
func (m DocsModel) renderFilter(b *strings.Builder) {
	b.WriteString(styleHeader.Render("Filter by tag") + "\n")
	if len(m.filterOpts) == 0 {
		b.WriteString(styleMuted.Render("  no tags yet") + "\n")
		return
	}
	for i, tc := range m.filterOpts {
		mark := "  [ ] "
		if containsStr(m.filterWork, tc.Tag) {
			mark = "  [x] "
		}
		line := fmt.Sprintf("%s#%s (%d)", mark, tc.Tag, tc.Count)
		if i == m.filterCursor {
			line = styleSel.Render("▸ " + strings.TrimLeft(line, " "))
		}
		b.WriteString(line + "\n")
	}
}

func containsStr(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
```

- [ ] **Step 8: Update the footer**

In `footer` (lines 694-705), add a `modeFiltering` case and advertise `f` in the list footer:

```go
func (m DocsModel) footer() string {
	switch m.mode {
	case modeView:
		return "tab/⇧tab link · enter folgen/öffnen · e edit · esc zurück · q quit"
	case modeCreating:
		return "tab next · space type · enter next/open editor · esc cancel"
	case modeDeleting:
		return "y confirm · n/esc cancel"
	case modeFiltering:
		return "j/k move · space toggle · c clear · enter apply · esc cancel"
	default:
		return "j/k move · enter view · n new · e edit · d delete · f filter · q quit"
	}
}
```

- [ ] **Step 9: Run tests + build**

Run: `go test ./internal/tui/ -run 'FilterOverlay|RenderViewSkips' -v && go build ./...`
Expected: PASS + build success.

- [ ] **Step 10: Consult tui-usability for final polish**

Invoke the `tui-usability` skill and confirm: `f` keybinding fits the grammar, the `[x]`/`▸` glyphs are on the whitelist, colours use the semantic styles (`styleSel`/`styleMuted`), and the footer reads consistently. Apply any small adjustments it surfaces (no behavioral change expected).

- [ ] **Step 11: Commit**

```bash
git add internal/tui/docs.go internal/tui/docs_test.go
git commit -m "feat(m2c): TUI tag display + frontmatter skip + filter overlay"
```

---

## Task 12: Composition root wiring + full CI

**Files:**
- Modify: `cmd/flow-server/main.go`

- [ ] **Step 1: Wire the ListTags usecase**

In `cmd/flow-server/main.go`, in the `&httpserver.Server{...}` literal (documents block around lines 108-113), add:

```go
		ListTags:          usecase.ListTags{Docs: documentStore},
```

- [ ] **Step 2: Verify every new server field is wired**

Run: `rg -n "ListTags" cmd/flow-server/main.go internal/adapter/httpserver/server.go`
Expected: the field is declared in `server.go`, registered as a route, and constructed in `main.go`. If any `&httpserver.Server{}` literal elsewhere (tests aside) omits a required usecase, fix it.

- [ ] **Step 3: Build all binaries**

Run: `go build ./...`
Expected: success.

- [ ] **Step 4: Run the full CI gate**

Run: `make ci`
Expected: `lint`, `verify-generate`, `cover` (≥ 80 %), and `build` all green. Fix any lint/coverage shortfalls before proceeding.

- [ ] **Step 5: Commit**

```bash
git add cmd/flow-server/main.go
git commit -m "feat(m2c): wire ListTags into server composition root"
```

---

## Task 13: Live done-gate (manual, like M2a/M2b)

Not a code task — execute against the dev stack and record the result.

- [ ] **Step 1: Bring up the dev stack with migration 0008**

Run: `make dev-up` then `make dev-run` (Postgres + Dex). Confirm migration `0008_documents_tags_gin.sql` applied (check server startup logs / `\d documents` shows `documents_tags_gin`).

- [ ] **Step 2: curl-smoke the API**

```bash
TOKEN=$(make -s dev-token)
# create a doc with frontmatter tags
curl -s -H "Authorization: Bearer $TOKEN" -X POST localhost:8080/api/v1/documents \
  -d '{"type":"free","path":"smoke-a","title":"A","body":"---\ntags: [go, tui]\n---\nhello"}'
# tag list
curl -s -H "Authorization: Bearer $TOKEN" localhost:8080/api/v1/documents/tags
# AND filter
curl -s -H "Authorization: Bearer $TOKEN" 'localhost:8080/api/v1/documents?tag=go&tag=tui'
```

Expected: the created doc has `"tags":["go","tui"]`; `/tags` returns `[{"tag":"go","count":1},{"tag":"tui","count":1}]` (counts reflect your data); the AND filter returns the doc; `?tag=go&tag=missing` returns `[]`.

- [ ] **Step 3: Browser dogfood**

Log in via Dex, open `/docs`. Confirm: tag chips appear in the filter bar; clicking `#go` then `#tui` narrows the list with AND semantics and the URL carries `?tag=go&tag=tui`; "Filter zurücksetzen" clears; opening the doc shows its tag chips and the `---\ntags:…\n---` block does **not** render as content.

- [ ] **Step 4: TUI dogfood**

Run `flow docs`. Confirm: rows show `#tag` suffixes; `f` opens the filter overlay; `space` toggles tags, `enter` applies AND filtering, `esc` cancels, `c` clears; opening a doc hides the frontmatter block and shows tags in the header.

- [ ] **Step 5: Record the outcome**

Update the memory note for the rebuild (`project_flow_rebuild_*`) with M2c done + commit range, and report the done-gate result to Soenne. Await confirmation before starting M2d (Search).
