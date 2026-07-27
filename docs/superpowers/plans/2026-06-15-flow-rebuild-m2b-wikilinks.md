# flow rebuild M2b — Wikilinks & Backlinks Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make M2a documents link to each other with `[[path]]` wikilinks and show "Referenced by" backlinks, navigable in both WebUI and TUI.

**Architecture:** Pure syntax + resolution authority in `domain`; a `document_links` table maintained on create/update; a `Backlinks` use case + REST endpoint; WebUI renders wikilinks as anchors + a backlinks footer; the TUI gets in-TUI wikilink navigation (focus cursor, Enter, back-stack) plus real weblinks that open in the OS default browser.

**Tech Stack:** Go, goldmark + bluemonday (WebUI markdown), pgx/goose (Postgres), charm.land bubbletea/v2 + lipgloss/v2 (TUI), templ (WebUI views).

**Spec:** `docs/superpowers/specs/2026-06-15-flow-rebuild-m2b-wikilinks-design.md`

**Branch:** `rebuild` (long-lived orphan, not merged). Work directly on it.

**Conventions observed in this codebase:**
- `documents.id` is **TEXT** (not UUID). The link table FK must be TEXT.
- Goose migrations: `-- +goose Up` / `-- +goose Down`, embedded via `//go:embed migrations/*.sql`.
- pgstore integration tests use `pgstore.NewPool(ctx, startPG(t))` + `pgstore.Migrate`.
- Use cases are plain structs with exported dependency fields, an `Execute` method.
- Run the full gate with `make ci` (lint + templ + build + tests, coverage ≥ 80 %).
- `gofmt` / `goimports` everything. The repo bans `find`/`grep`/`tree` in tooling but that does not affect Go code.

---

## File Structure

**Create:**
- `internal/domain/wikilink.go` — `WikilinkSpan`, `FindWikilinks`, `WikilinkTargets`, `ResolveWikilink`, `BacklinkRef`.
- `internal/domain/wikilink_test.go`
- `internal/adapter/pgstore/migrations/0007_document_links.sql`
- `internal/usecase/backlinks.go` — `Backlinks` use case.
- `internal/usecase/backlinks_test.go`
- `internal/adapter/webui/wikilink.go` — goldmark wikilink extension + `RenderDocument`.
- `internal/adapter/webui/wikilink_test.go`
- `internal/adapter/opener/opener.go` — OS default-browser opener.
- `internal/tui/weblink.go` — bare-URL + markdown-link scanner for the TUI.
- `internal/tui/weblink_test.go`

**Modify:**
- `internal/ports/ports.go` — extend `DocumentStore` with `ReplaceLinks` + `Backlinks`.
- `internal/testutil/fakes.go` — implement the two new methods on `FakeDocumentStore` (track links).
- `internal/adapter/pgstore/documents.go` — implement `ReplaceLinks` + `Backlinks`.
- `internal/adapter/pgstore/documents_test.go` — link round-trip test.
- `internal/usecase/create_document.go` + `update_document.go` — extract + persist links after save.
- `internal/usecase/document_test.go` — assert links written.
- `internal/adapter/httpserver/server.go` — add `Backlinks` use-case field + route.
- `internal/adapter/httpserver/documents.go` — `handleDocumentBacklinks`.
- `internal/adapter/httpserver/documents_test.go` — backlinks endpoint test.
- `internal/adapter/apiclient/documents.go` — `Backlinks` method.
- `internal/adapter/httpserver/webui_docs.go` — build resolver + backlinks in the view handler.
- `internal/adapter/webui/docs.templ` — backlinks footer + wikilink CSS classes; regenerate `docs_templ.go`.
- `internal/tui/styles.go` — `WikilinkValid`, `WikilinkBroken`, `WebLink`, focus style + cyan colour.
- `internal/tui/docs.go` — line styler, link focus set, Enter dispatch, back-stack, backlinks footer, opener.
- `cmd/flow-server/main.go` — wire the `Backlinks` use case.
- `cmd/flow/docs.go` — inject the opener into `tui.NewDocs`.

---

## Task 1: domain — `FindWikilinks` + `WikilinkTargets`

**Files:**
- Create: `internal/domain/wikilink.go`
- Test: `internal/domain/wikilink_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package domain

import (
	"reflect"
	"testing"
)

func TestFindWikilinks(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []WikilinkSpan
	}{
		{"none", "plain text", nil},
		{"simple", "see [[arch]] now", []WikilinkSpan{{Start: 4, End: 12, Target: "arch", Display: ""}}},
		{"pipe", "see [[arch|Architecture]] x", []WikilinkSpan{{Start: 4, End: 25, Target: "arch", Display: "Architecture"}}},
		{"two", "[[a]] and [[b]]", []WikilinkSpan{
			{Start: 0, End: 5, Target: "a", Display: ""},
			{Start: 10, End: 15, Target: "b", Display: ""},
		}},
		{"empty target ignored", "[[]] and [[|d]]", nil},
		{"unterminated", "[[arch", nil},
		{"newline aborts", "[[ar\nch]]", nil},
		{"adjacent brackets", "[[a]][[b]]", []WikilinkSpan{
			{Start: 0, End: 5, Target: "a", Display: ""},
			{Start: 5, End: 10, Target: "b", Display: ""},
		}},
		{"path slug", "[[daily/2026-06-15]]", []WikilinkSpan{{Start: 0, End: 20, Target: "daily/2026-06-15", Display: ""}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindWikilinks(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("FindWikilinks(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestWikilinkTargets(t *testing.T) {
	got := WikilinkTargets("[[a]] [[b]] [[a]] [[c|x]]")
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("WikilinkTargets = %#v, want %#v", got, want)
	}
	if WikilinkTargets("no links") != nil {
		t.Fatalf("expected nil for no links")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/domain/ -run 'Wikilink' -v`
Expected: FAIL — `undefined: FindWikilinks` / `WikilinkSpan` / `WikilinkTargets`.

- [ ] **Step 3: Implement**

```go
package domain

// WikilinkSpan is one `[[target]]` / `[[target|display]]` occurrence in a
// body, with byte offsets so a renderer can slice the surrounding text.
type WikilinkSpan struct {
	Start, End int // byte offsets into the source; [Start,End) covers the whole `[[...]]`
	Target     string
	Display    string // explicit display half; "" when no pipe
}

// FindWikilinks scans s for wikilinks. A candidate aborts at a newline (a
// wikilink never spans a line break) and an empty target is not a match, so
// other `[...]` constructs fall through untouched.
func FindWikilinks(s string) []WikilinkSpan {
	var out []WikilinkSpan
	for i := 0; i+1 < len(s); i++ {
		if s[i] != '[' || s[i+1] != '[' {
			continue
		}
		end := -1
		for j := i + 2; j+1 < len(s); j++ {
			if s[j] == '\n' {
				break
			}
			if s[j] == ']' && s[j+1] == ']' {
				end = j
				break
			}
		}
		if end < 0 {
			continue
		}
		target, display := splitWikilinkInner(s[i+2 : end])
		if target == "" {
			continue
		}
		out = append(out, WikilinkSpan{Start: i, End: end + 2, Target: target, Display: display})
		i = end + 1 // resume after `]]` (loop's i++ lands past it)
	}
	return out
}

// splitWikilinkInner splits `target|display`. Display is empty without a pipe.
// A newline or stray `]` inside aborts the match (returns empty target).
func splitWikilinkInner(s string) (target, display string) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '|':
			return s[:i], s[i+1:]
		case '\n', ']':
			return "", ""
		}
	}
	return s, ""
}

// WikilinkTargets returns the ordered, de-duplicated target paths in body,
// for the link index. Returns nil when there are none.
func WikilinkTargets(body string) []string {
	spans := FindWikilinks(body)
	if len(spans) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(spans))
	var out []string
	for _, sp := range spans {
		if !seen[sp.Target] {
			seen[sp.Target] = true
			out = append(out, sp.Target)
		}
	}
	return out
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/domain/ -run 'Wikilink' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/domain/wikilink.go internal/domain/wikilink_test.go
git add internal/domain/wikilink.go internal/domain/wikilink_test.go
git commit -m "feat(docs): domain wikilink scanner (FindWikilinks + targets)"
```

---

## Task 2: domain — `ResolveWikilink` + `BacklinkRef`

**Files:**
- Modify: `internal/domain/wikilink.go`
- Test: `internal/domain/wikilink_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/domain/wikilink_test.go`:

```go
func strptr(s string) *string { return &s }

func TestResolveWikilink(t *testing.T) {
	pA := strptr("proj-a")
	pB := strptr("proj-b")
	all := []Document{
		{ID: "free1", Path: "shared", ProjectID: nil, Title: "Shared Free"},
		{ID: "a1", Path: "notes", ProjectID: pA, Title: "A Notes"},
		{ID: "b1", Path: "notes", ProjectID: pB, Title: "B Notes"},
		{ID: "bonly", Path: "bsecret", ProjectID: pB, Title: "B Secret"},
	}

	tests := []struct {
		name   string
		src    Document
		target string
		wantID string // "" => not found
	}{
		{"same project wins", Document{ProjectID: pA}, "notes", "a1"},
		{"other project same slug", Document{ProjectID: pB}, "notes", "b1"},
		{"free from project falls back to free", Document{ProjectID: pA}, "shared", "free1"},
		{"free doc links free", Document{ProjectID: nil}, "shared", "free1"},
		{"free doc cannot reach project", Document{ProjectID: nil}, "notes", ""},
		{"project doc cannot reach foreign project", Document{ProjectID: pA}, "bsecret", ""},
		{"missing", Document{ProjectID: pA}, "nope", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ResolveWikilink(tt.src, tt.target, all)
			if tt.wantID == "" {
				if ok {
					t.Fatalf("expected broken, got %q", got.ID)
				}
				return
			}
			if !ok || got.ID != tt.wantID {
				t.Fatalf("ResolveWikilink = (%q,%v), want %q", got.ID, ok, tt.wantID)
			}
		})
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/domain/ -run 'ResolveWikilink' -v`
Expected: FAIL — `undefined: ResolveWikilink`.

- [ ] **Step 3: Implement**

Append to `internal/domain/wikilink.go`:

```go
// BacklinkRef is a lightweight reference to a document that links to another,
// surfaced in "Referenced by". Shared by the backlinks use case, REST, and the
// API client.
type BacklinkRef struct {
	ID    string       `json:"id"`
	Path  string       `json:"path"`
	Title string       `json:"title"`
	Type  DocumentType `json:"type"`
}

// sameScope reports whether two documents share a wikilink resolution scope:
// the same project, or both free/owner-level (nil ProjectID).
func sameScope(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// ResolveWikilink resolves target against the owner's document set all, from the
// perspective of src. Scope-isolated: a same-scope match wins; else a free
// (ProjectID == nil) match; else broken. A foreign-project match never
// resolves, even when owner-wide unique.
func ResolveWikilink(src Document, target string, all []Document) (Document, bool) {
	var free *Document
	for i := range all {
		d := all[i]
		if d.Path != target {
			continue
		}
		if sameScope(src.ProjectID, d.ProjectID) {
			return d, true
		}
		if d.ProjectID == nil && free == nil {
			free = &all[i]
		}
	}
	if free != nil {
		return *free, true
	}
	return Document{}, false
}
```

Note: when `src.ProjectID == nil`, a same-scope match is itself the free match, so the same-scope branch already returns it. The `free` fallback only fires for a project-scoped `src`.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/domain/ -run 'ResolveWikilink' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/domain/wikilink.go internal/domain/wikilink_test.go
git add internal/domain/wikilink.go internal/domain/wikilink_test.go
git commit -m "feat(docs): domain ResolveWikilink (scope-isolated) + BacklinkRef"
```

---

## Task 3: ports + fake — `ReplaceLinks` / `Backlinks`

**Files:**
- Modify: `internal/ports/ports.go:114-120`
- Modify: `internal/testutil/fakes.go` (FakeDocumentStore)
- Test: add to `internal/usecase/document_test.go` is later; test the fake here in `internal/testutil/fakes_test.go` (create if absent) OR inline in the usecase test. We test it through the use case in Task 5, but add a direct fake test now.
- Test: `internal/testutil/fakes_test.go`

- [ ] **Step 1: Extend the port**

In `internal/ports/ports.go`, replace the `DocumentStore` interface body:

```go
// DocumentStore persists compendium documents. All reads are owner-scoped.
// Create returns ErrDocumentExists on a (owner, project, path) collision.
type DocumentStore interface {
	Create(ctx context.Context, d domain.Document) (domain.Document, error)
	Get(ctx context.Context, ownerID, id string) (domain.Document, error)
	List(ctx context.Context, ownerID string) ([]domain.Document, error)
	Update(ctx context.Context, d domain.Document) (domain.Document, error)
	Delete(ctx context.Context, ownerID, id string) error

	// ReplaceLinks rewrites the outbound wikilink targets of one document
	// (delete-then-insert). Empty targets clears them.
	ReplaceLinks(ctx context.Context, srcDocID, ownerID string, targets []string) error
	// Backlinks returns the owner's documents whose recorded outbound links
	// include targetPath (candidate sources; the use case re-resolves scope).
	Backlinks(ctx context.Context, ownerID, targetPath string) ([]domain.Document, error)
}
```

- [ ] **Step 2: Write the failing fake test**

Create `internal/testutil/fakes_test.go`:

```go
package testutil

import (
	"context"
	"testing"
)

func TestFakeDocumentStore_Links(t *testing.T) {
	ctx := context.Background()
	s := NewFakeDocumentStore()
	mustCreate(t, s, "src1", "owner", "a", nil)
	mustCreate(t, s, "src2", "owner", "b", nil)

	if err := s.ReplaceLinks(ctx, "src1", "owner", []string{"b", "c"}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceLinks(ctx, "src2", "owner", []string{"b"}); err != nil {
		t.Fatal(err)
	}

	got, err := s.Backlinks(ctx, "owner", "b")
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, d := range got {
		ids[d.ID] = true
	}
	if !ids["src1"] || !ids["src2"] || len(got) != 2 {
		t.Fatalf("backlinks of b = %v, want src1+src2", ids)
	}

	// Replace clears old targets.
	if err := s.ReplaceLinks(ctx, "src1", "owner", nil); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Backlinks(ctx, "owner", "b")
	if len(got) != 1 || got[0].ID != "src2" {
		t.Fatalf("after clear, backlinks of b = %v, want only src2", got)
	}

	// Owner isolation.
	other, _ := s.Backlinks(ctx, "stranger", "b")
	if len(other) != 0 {
		t.Fatalf("expected no cross-owner backlinks, got %v", other)
	}
}
```

Add the `mustCreate` helper at the bottom of the same file:

```go
import alias note: also import "github.com/serverkraken/flow/internal/domain" and "time" in this test file.

func mustCreate(t *testing.T, s *FakeDocumentStore, id, owner, path string, proj *string) {
	t.Helper()
	_, err := s.Create(context.Background(), domain.Document{
		ID: id, OwnerID: owner, ProjectID: proj, Type: domain.DocFree,
		Path: path, Title: id, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
}
```

(Write the imports properly: `context`, `testing`, `time`, `github.com/serverkraken/flow/internal/domain`.)

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./internal/testutil/ -run 'Links' -v`
Expected: FAIL — `s.ReplaceLinks undefined` / `s.Backlinks undefined`.

- [ ] **Step 4: Implement on FakeDocumentStore**

In `internal/testutil/fakes.go`, add a links map to the struct and a constructor init. Change:

```go
type FakeDocumentStore struct {
	mu    sync.Mutex
	m     map[string]domain.Document // keyed by id
	links map[string][]string        // srcDocID -> target paths
}

func NewFakeDocumentStore() *FakeDocumentStore {
	return &FakeDocumentStore{m: map[string]domain.Document{}, links: map[string][]string{}}
}
```

Add the two methods (place after `Delete`):

```go
func (s *FakeDocumentStore) ReplaceLinks(_ context.Context, srcDocID, _ string, targets []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(targets) == 0 {
		delete(s.links, srcDocID)
		return nil
	}
	cp := make([]string, len(targets))
	copy(cp, targets)
	s.links[srcDocID] = cp
	return nil
}

func (s *FakeDocumentStore) Backlinks(_ context.Context, ownerID, targetPath string) ([]domain.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Document
	for srcID, targets := range s.links {
		d, ok := s.m[srcID]
		if !ok || d.OwnerID != ownerID {
			continue
		}
		for _, tgt := range targets {
			if tgt == targetPath {
				out = append(out, d)
				break
			}
		}
	}
	return out, nil
}
```

Also make `Delete` clear the links map for the deleted doc (mirror ON DELETE CASCADE). In `Delete`, before `delete(s.m, id)` add:

```go
	delete(s.links, id)
```

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./internal/testutil/ -run 'Links' -v`
Expected: PASS. Also `go build ./...` (the port grew; pgstore won't compile yet — that is Task 4, so expect a pgstore build error here only if you build the whole tree. Build just the touched packages: `go build ./internal/ports/ ./internal/testutil/`).

- [ ] **Step 6: Commit**

```bash
gofmt -w internal/ports/ports.go internal/testutil/fakes.go internal/testutil/fakes_test.go
git add internal/ports/ports.go internal/testutil/fakes.go internal/testutil/fakes_test.go
git commit -m "feat(docs): DocumentStore ReplaceLinks/Backlinks port + fake"
```

---

## Task 4: migration 0007 + pgstore `ReplaceLinks` / `Backlinks`

**Files:**
- Create: `internal/adapter/pgstore/migrations/0007_document_links.sql`
- Modify: `internal/adapter/pgstore/documents.go`
- Test: `internal/adapter/pgstore/documents_test.go`

- [ ] **Step 1: Write the migration**

`internal/adapter/pgstore/migrations/0007_document_links.sql`:

```sql
-- +goose Up
CREATE TABLE document_links (
    src_doc_id  TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    owner_id    TEXT NOT NULL REFERENCES users(id),
    target_path TEXT NOT NULL
);
CREATE INDEX document_links_lookup ON document_links (owner_id, target_path);
CREATE INDEX document_links_src ON document_links (src_doc_id);

-- +goose Down
DROP TABLE document_links;
```

- [ ] **Step 2: Write the failing integration test**

Append to `internal/adapter/pgstore/documents_test.go`:

```go
func TestDocumentStore_Links(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	users := pgstore.NewUserStore(pool)
	u, _ := domain.NewUser("u-lnk", "sub-lnk", "lnk", "lnk@x.de", "Lnk")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	st := pgstore.NewDocumentStore(pool)
	now := time.Now().UTC().Truncate(time.Second)
	mk := func(id, path string) {
		if _, err := st.Create(ctx, domain.Document{
			ID: id, OwnerID: "u-lnk", Type: domain.DocFree, Path: path,
			Title: id, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	mk("s1", "alpha")
	mk("s2", "beta")

	if err := st.ReplaceLinks(ctx, "s1", "u-lnk", []string{"beta", "gamma"}); err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceLinks(ctx, "s2", "u-lnk", []string{"beta"}); err != nil {
		t.Fatal(err)
	}
	got, err := st.Backlinks(ctx, "u-lnk", "beta")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("backlinks(beta) = %d docs, want 2", len(got))
	}

	// Replace is idempotent / clears.
	if err := st.ReplaceLinks(ctx, "s1", "u-lnk", []string{"gamma"}); err != nil {
		t.Fatal(err)
	}
	got, _ = st.Backlinks(ctx, "u-lnk", "beta")
	if len(got) != 1 || got[0].ID != "s2" {
		t.Fatalf("after replace, backlinks(beta) = %v, want only s2", got)
	}

	// ON DELETE CASCADE removes outbound rows.
	if err := st.Delete(ctx, "u-lnk", "s2"); err != nil {
		t.Fatal(err)
	}
	got, _ = st.Backlinks(ctx, "u-lnk", "beta")
	if len(got) != 0 {
		t.Fatalf("after delete s2, backlinks(beta) = %v, want none", got)
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./internal/adapter/pgstore/ -run 'TestDocumentStore_Links' -v`
Expected: FAIL — `st.ReplaceLinks undefined` / `st.Backlinks undefined`.

- [ ] **Step 4: Implement in `documents.go`**

Add to `internal/adapter/pgstore/documents.go` (after `Delete`):

```go
func (s *DocumentStore) ReplaceLinks(ctx context.Context, srcDocID, ownerID string, targets []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pgstore: begin links tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM document_links WHERE src_doc_id=$1`, srcDocID); err != nil {
		return fmt.Errorf("pgstore: clear links: %w", err)
	}
	for _, tgt := range targets {
		if _, err := tx.Exec(ctx,
			`INSERT INTO document_links (src_doc_id, owner_id, target_path) VALUES ($1,$2,$3)`,
			srcDocID, ownerID, tgt); err != nil {
			return fmt.Errorf("pgstore: insert link: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (s *DocumentStore) Backlinks(ctx context.Context, ownerID, targetPath string) ([]domain.Document, error) {
	const q = `SELECT DISTINCT ` + prefixedDocCols + `
FROM documents d
JOIN document_links l ON l.src_doc_id = d.id
WHERE l.owner_id=$1 AND l.target_path=$2
ORDER BY d.updated_at DESC`
	rows, err := s.pool.Query(ctx, q, ownerID, targetPath)
	if err != nil {
		return nil, fmt.Errorf("pgstore: backlinks: %w", err)
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

`scanDocument` reads columns positionally, so the joined query must select the same columns with the `d.` prefix. Add this constant near `docCols`:

```go
const prefixedDocCols = `d.id, d.owner_id, d.project_id, d.type, d.path, d.title, d.body, d.tags, d.doc_date, d.role, d.extra, d.created_at, d.updated_at`
```

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./internal/adapter/pgstore/ -run 'TestDocumentStore_Links' -v`
Expected: PASS. Then `go build ./...` should now succeed for the whole tree.

- [ ] **Step 6: Commit**

```bash
gofmt -w internal/adapter/pgstore/documents.go internal/adapter/pgstore/documents_test.go
git add internal/adapter/pgstore/migrations/0007_document_links.sql internal/adapter/pgstore/documents.go internal/adapter/pgstore/documents_test.go
git commit -m "feat(docs): document_links migration 0007 + pgstore ReplaceLinks/Backlinks"
```

---

## Task 5: use cases — extract links on save + `Backlinks` use case

**Files:**
- Modify: `internal/usecase/create_document.go`
- Modify: `internal/usecase/update_document.go`
- Create: `internal/usecase/backlinks.go`
- Test: `internal/usecase/document_test.go` (add cases) + `internal/usecase/backlinks_test.go`

- [ ] **Step 1: Write the failing backlinks-usecase test**

Create `internal/usecase/backlinks_test.go`:

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

func TestBacklinks_FiltersByScope(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewFakeDocumentStore()
	now := time.Now()
	pA := "proj-a"

	// target: free doc "spec"
	mk := func(id, path string, proj *string) {
		if _, err := store.Create(ctx, domain.Document{
			ID: id, OwnerID: "o", ProjectID: proj, Type: domain.DocFree,
			Path: path, Title: id, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	mk("target", "spec", nil)
	mk("good", "g", nil)          // free doc, [[spec]] resolves to target
	mk("bad", "b", &pA)           // project-A doc with [[spec]] -> free target, also valid
	mk("decoy", "d", &pA)         // project-A doc with [[other]] -> not a backlink

	// record raw outbound links (as create/update would)
	_ = store.ReplaceLinks(ctx, "good", "o", []string{"spec"})
	_ = store.ReplaceLinks(ctx, "bad", "o", []string{"spec"})
	_ = store.ReplaceLinks(ctx, "decoy", "o", []string{"other"})

	uc := usecase.Backlinks{Docs: store}
	refs, err := uc.Execute(ctx, "o", "target")
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, r := range refs {
		ids[r.ID] = true
	}
	if !ids["good"] || !ids["bad"] || ids["decoy"] || len(refs) != 2 {
		t.Fatalf("backlinks = %v, want good+bad", ids)
	}
}

func TestBacklinks_DropsForeignProjectFalsePositive(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewFakeDocumentStore()
	now := time.Now()
	pA, pB := "proj-a", "proj-b"
	mk := func(id, path string, proj *string) {
		_, _ = store.Create(ctx, domain.Document{
			ID: id, OwnerID: "o", ProjectID: proj, Type: domain.DocFree,
			Path: path, Title: id, CreatedAt: now, UpdatedAt: now,
		})
	}
	// Two docs named "notes": one in A (the target id we query), one in B.
	mk("notesA", "notes", &pA)
	mk("notesB", "notes", &pB)
	mk("srcB", "s", &pB) // in B, links [[notes]] -> resolves to notesB, NOT notesA
	_ = store.ReplaceLinks(ctx, "srcB", "o", []string{"notes"})

	uc := usecase.Backlinks{Docs: store}
	refs, err := uc.Execute(ctx, "o", "notesA")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 0 {
		t.Fatalf("notesA should have no backlinks (srcB resolves to notesB), got %v", refs)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/usecase/ -run 'Backlinks' -v`
Expected: FAIL — `undefined: usecase.Backlinks`.

- [ ] **Step 3: Implement the backlinks use case**

Create `internal/usecase/backlinks.go`:

```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Backlinks returns the documents that link to a given document, after
// re-resolving each candidate through domain.ResolveWikilink so foreign-scope
// collisions never produce false references.
type Backlinks struct {
	Docs ports.DocumentStore
}

func (uc Backlinks) Execute(ctx context.Context, ownerID, docID string) ([]domain.BacklinkRef, error) {
	target, err := uc.Docs.Get(ctx, ownerID, docID)
	if err != nil {
		return nil, err
	}
	candidates, err := uc.Docs.Backlinks(ctx, ownerID, target.Path)
	if err != nil {
		return nil, err
	}
	all, err := uc.Docs.List(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	var out []domain.BacklinkRef
	for _, src := range candidates {
		if src.ID == target.ID {
			continue // a doc linking itself is not a backlink
		}
		if resolved, ok := domain.ResolveWikilink(src, target.Path, all); ok && resolved.ID == target.ID {
			out = append(out, domain.BacklinkRef{
				ID: src.ID, Path: src.Path, Title: src.Title, Type: src.Type,
			})
		}
	}
	return out, nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/usecase/ -run 'Backlinks' -v`
Expected: PASS.

- [ ] **Step 5: Wire extraction into Create + Update**

In `internal/usecase/create_document.go`, replace the final `return uc.Docs.Create(...)` with:

```go
	created, err := uc.Docs.Create(ctx, d)
	if err != nil {
		return domain.Document{}, err
	}
	if err := uc.Docs.ReplaceLinks(ctx, created.ID, ownerID, domain.WikilinkTargets(created.Body)); err != nil {
		return domain.Document{}, err
	}
	return created, nil
```

In `internal/usecase/update_document.go`, replace `return uc.Docs.Update(ctx, cur)` with:

```go
	updated, err := uc.Docs.Update(ctx, cur)
	if err != nil {
		return domain.Document{}, err
	}
	if err := uc.Docs.ReplaceLinks(ctx, updated.ID, ownerID, domain.WikilinkTargets(updated.Body)); err != nil {
		return domain.Document{}, err
	}
	return updated, nil
```

- [ ] **Step 6: Add a regression test that create/update writes links**

Append to `internal/usecase/document_test.go` (uses the existing fixtures pattern in that file; adapt the constructor calls to match how CreateDocument is built there — it needs `Docs`, `IDs`, `Clock`):

```go
func TestCreateDocument_WritesLinks(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewFakeDocumentStore()
	uc := usecase.CreateDocument{
		Docs:  store,
		IDs:   testutil.FakeIDGen{ID: "doc-1"},
		Clock: testutil.FakeClock{T: time.Now()},
	}
	if _, err := uc.Execute(ctx, "o", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "src", Title: "Src", Body: "see [[dest]] and [[dest]]",
	}); err != nil {
		t.Fatal(err)
	}
	// make a dest so Backlinks(dest) finds src
	_, _ = store.Create(ctx, domain.Document{
		ID: "doc-2", OwnerID: "o", Type: domain.DocFree, Path: "dest", Title: "Dest",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	refs, err := (usecase.Backlinks{Docs: store}).Execute(ctx, "o", "doc-2")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].ID != "doc-1" {
		t.Fatalf("expected src as the only backlink of dest, got %v", refs)
	}
}
```

Before writing, open `internal/usecase/document_test.go` and confirm the exact names of the fake ID generator and clock helpers in `internal/testutil` (e.g. `FakeIDGen`/`FakeClock`); use whatever this repo already uses (grep the test file's existing CreateDocument construction and copy it verbatim).

- [ ] **Step 7: Run to verify it passes**

Run: `go test ./internal/usecase/ -v`
Expected: PASS (all document + backlinks tests).

- [ ] **Step 8: Commit**

```bash
gofmt -w internal/usecase/
git add internal/usecase/backlinks.go internal/usecase/backlinks_test.go internal/usecase/create_document.go internal/usecase/update_document.go internal/usecase/document_test.go
git commit -m "feat(docs): extract links on save + Backlinks use case"
```

---

## Task 6: REST — `GET /api/v1/documents/{id}/backlinks`

**Files:**
- Modify: `internal/adapter/httpserver/server.go` (struct field + route)
- Modify: `internal/adapter/httpserver/documents.go` (handler)
- Test: `internal/adapter/httpserver/documents_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/adapter/httpserver/documents_test.go` (mirror the existing helpers `doDoc`, `ts`):

```go
func TestBacklinksEndpoint(t *testing.T) {
	ts := newDocTestServer(t) // use whatever this file's server-builder helper is called
	defer ts.Close()

	// create dest, then src that links it
	dest := doDoc(t, ts, "POST", "/api/v1/documents", `{"type":"free","path":"dest","title":"Dest","body":""}`)
	var destDoc domain.Document
	decodeBody(t, dest, &destDoc) // use the file's existing decode helper
	doDoc(t, ts, "POST", "/api/v1/documents", `{"type":"free","path":"src","title":"Src","body":"[[dest]]"}`)

	res := doDoc(t, ts, "GET", "/api/v1/documents/"+destDoc.ID+"/backlinks", "")
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var refs []domain.BacklinkRef
	decodeBody(t, res, &refs)
	if len(refs) != 1 || refs[0].Path != "src" {
		t.Fatalf("backlinks = %v, want [src]", refs)
	}

	res404 := doDoc(t, ts, "GET", "/api/v1/documents/nope/backlinks", "")
	if res404.StatusCode != 404 {
		t.Fatalf("missing doc status = %d, want 404", res404.StatusCode)
	}
}
```

Note: match the existing test helpers in `documents_test.go` exactly (server builder, request runner, body decoder). Read the top of that file first and reuse its names instead of `newDocTestServer`/`decodeBody` if they differ.

- [ ] **Step 2: Add the use-case field + route**

In `internal/adapter/httpserver/server.go`, in the `Server` struct near the other document use cases (lines ~44-48) add:

```go
	BacklinksDocument usecase.Backlinks
```

In `Routes()`, after the DELETE documents route (line ~90):

```go
	mux.Handle("GET /api/v1/documents/{id}/backlinks", s.auth(http.HandlerFunc(s.handleDocumentBacklinks)))
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./internal/adapter/httpserver/ -run 'Backlinks' -v`
Expected: FAIL — `s.handleDocumentBacklinks undefined` (and the field unused build error until the handler exists).

- [ ] **Step 4: Implement the handler**

Append to `internal/adapter/httpserver/documents.go`:

```go
func (s *Server) handleDocumentBacklinks(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	refs, err := s.BacklinksDocument.Execute(r.Context(), u.ID, r.PathValue("id"))
	switch {
	case errors.Is(err, ports.ErrDocumentNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		if refs == nil {
			refs = []domain.BacklinkRef{}
		}
		writeJSON(w, http.StatusOK, refs)
	}
}
```

- [ ] **Step 5: Wire the field in the test server builder**

In `documents_test.go`'s server-builder helper (the one that constructs `httpserver.Server{...}` with the fake store), add:

```go
		BacklinksDocument: usecase.Backlinks{Docs: docStore},
```

(Use the same fake store variable the other use cases share.)

- [ ] **Step 6: Run to verify it passes**

Run: `go test ./internal/adapter/httpserver/ -run 'Backlinks' -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
gofmt -w internal/adapter/httpserver/
git add internal/adapter/httpserver/server.go internal/adapter/httpserver/documents.go internal/adapter/httpserver/documents_test.go
git commit -m "feat(docs): REST GET /documents/{id}/backlinks"
```

---

## Task 7: apiclient — `Backlinks`

**Files:**
- Modify: `internal/adapter/apiclient/documents.go`
- Test: add to the apiclient test file (find it: `internal/adapter/apiclient/*_test.go`); if a documents test exists, append, else create `documents_test.go` using the existing client-test harness pattern.

- [ ] **Step 1: Write the failing test**

Append (or create) in `internal/adapter/apiclient/documents_test.go`, following the existing httptest-based client test pattern in this package:

```go
func TestClient_Backlinks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/documents/d1/backlinks" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"s1","path":"src","title":"Src","type":"free"}]`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL) // use the package's existing client constructor for tests
	refs, err := c.Backlinks(context.Background(), "d1")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].ID != "s1" || refs[0].Type != domain.DocFree {
		t.Fatalf("refs = %v", refs)
	}
}
```

Read an existing apiclient test first to copy the exact client constructor used in tests (token/transport setup) instead of `newTestClient`.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/adapter/apiclient/ -run 'Backlinks' -v`
Expected: FAIL — `c.Backlinks undefined`.

- [ ] **Step 3: Implement**

Append to `internal/adapter/apiclient/documents.go`:

```go
func (c *Client) Backlinks(ctx context.Context, id string) ([]domain.BacklinkRef, error) {
	var out []domain.BacklinkRef
	err := c.do(ctx, http.MethodGet, "/api/v1/documents/"+id+"/backlinks", nil, &out)
	return out, err
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/adapter/apiclient/ -run 'Backlinks' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/adapter/apiclient/
git add internal/adapter/apiclient/documents.go internal/adapter/apiclient/documents_test.go
git commit -m "feat(docs): apiclient Backlinks"
```

---

## Task 8: WebUI — `RenderDocument` (goldmark wikilink extension)

**Files:**
- Create: `internal/adapter/webui/wikilink.go`
- Test: `internal/adapter/webui/wikilink_test.go`

The resolver is a plain func injected by the handler so this package stays free
of domain-resolution logic: `func(target string) (href, title string, ok bool)`.

- [ ] **Step 1: Write the failing test**

`internal/adapter/webui/wikilink_test.go`:

```go
package webui

import (
	"strings"
	"testing"
)

func TestRenderDocument_Wikilinks(t *testing.T) {
	resolve := func(target string) (string, string, bool) {
		if target == "arch" {
			return "/docs/d-arch", "Architecture", true
		}
		return "", "", false
	}
	html := string(RenderDocument("see [[arch]] and [[ghost]] and [[arch|the arch]]", resolve))

	if !strings.Contains(html, `href="/docs/d-arch"`) {
		t.Errorf("valid wikilink should link to /docs/d-arch:\n%s", html)
	}
	if !strings.Contains(html, "Architecture") {
		t.Errorf("valid wikilink should use resolved title as display:\n%s", html)
	}
	if !strings.Contains(html, "the arch") {
		t.Errorf("explicit display should win:\n%s", html)
	}
	if !strings.Contains(html, "wikilink-broken") {
		t.Errorf("ghost should render broken:\n%s", html)
	}
}

func TestRenderDocument_StillSanitises(t *testing.T) {
	html := string(RenderDocument("<script>alert(1)</script> [[x]]", func(string) (string, string, bool) {
		return "", "", false
	}))
	if strings.Contains(html, "<script>") {
		t.Errorf("script must be stripped:\n%s", html)
	}
}

func TestRenderDocument_RealLinksNative(t *testing.T) {
	html := string(RenderDocument("[site](https://example.com)", func(string) (string, string, bool) {
		return "", "", false
	}))
	if !strings.Contains(html, `href="https://example.com"`) {
		t.Errorf("real markdown links should render natively:\n%s", html)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/adapter/webui/ -run 'RenderDocument' -v`
Expected: FAIL — `undefined: RenderDocument`.

- [ ] **Step 3: Implement the extension + renderer**

`internal/adapter/webui/wikilink.go`:

```go
package webui

import (
	"bytes"
	"html"
	"html/template"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	gmtext "github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"

	"github.com/serverkraken/flow/internal/domain"
)

// WikilinkResolver maps a wikilink target to an href + display title. ok=false
// renders the link as broken.
type WikilinkResolver func(target string) (href, title string, ok bool)

// docPolicy is the UGC policy plus the attributes our wikilink anchors/spans
// need. Built once.
var docPolicy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowAttrs("class").OnElements("a", "span")
	// relative /docs/ hrefs for resolved wikilinks
	p.AllowAttrs("href").OnElements("a")
	return p
}()

// RenderDocument renders document Markdown to sanitised HTML, resolving
// `[[target]]` wikilinks via resolve. Regular Markdown links render natively.
func RenderDocument(src string, resolve WikilinkResolver) template.HTML {
	gm := goldmark.New(
		goldmark.WithParserOptions(parser.WithInlineParsers(
			util.Prioritized(&wikiLinkParser{}, 100),
		)),
		goldmark.WithRendererOptions(),
	)
	gm.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(&wikiLinkHTMLRenderer{resolve: resolve}, 100),
	))
	_ = gmhtml.Renderer{} // keep import if unused otherwise; remove if lints complain

	var buf bytes.Buffer
	if err := gm.Convert([]byte(src), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(src))
	}
	return template.HTML(docPolicy.SanitizeBytes(buf.Bytes()))
}

// --- AST node ---

var kindWikiLink = ast.NewNodeKind("WikiLink")

type wikiLinkNode struct {
	ast.BaseInline
	Target, Display string
}

func (n *wikiLinkNode) Kind() ast.NodeKind             { return kindWikiLink }
func (n *wikiLinkNode) Dump(src []byte, level int)     { ast.DumpHelper(n, src, level, nil, nil) }

// --- parser ---

type wikiLinkParser struct{}

func (wikiLinkParser) Trigger() []byte { return []byte{'['} }

func (wikiLinkParser) Parse(_ ast.Node, block gmtext.Reader, _ parser.Context) ast.Node {
	line, _ := block.PeekLine()
	if len(line) < 4 || line[0] != '[' || line[1] != '[' {
		return nil
	}
	end := -1
	for i := 2; i+1 < len(line); i++ {
		if line[i] == '\n' {
			break
		}
		if line[i] == ']' && line[i+1] == ']' {
			end = i
			break
		}
	}
	if end < 0 {
		return nil
	}
	target, display := splitInner(string(line[2:end]))
	if target == "" {
		return nil
	}
	block.Advance(end + 2)
	return &wikiLinkNode{Target: target, Display: display}
}

func splitInner(s string) (target, display string) {
	for i := 0; i < len(s); i++ {
		if s[i] == '|' {
			return s[:i], s[i+1:]
		}
		if s[i] == ']' {
			return "", ""
		}
	}
	return s, ""
}

// --- renderer ---

type wikiLinkHTMLRenderer struct{ resolve WikilinkResolver }

func (r *wikiLinkHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindWikiLink, r.render)
}

func (r *wikiLinkHTMLRenderer) render(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	wl := n.(*wikiLinkNode)
	display := wl.Display
	var href, title string
	ok := false
	if r.resolve != nil {
		href, title, ok = r.resolve(wl.Target)
	}
	if display == "" {
		if ok && title != "" {
			display = title
		} else {
			display = wl.Target
		}
	}
	esc := html.EscapeString(display)
	if ok {
		_, _ = w.WriteString(`<a class="wikilink" href="` + html.EscapeString(href) + `">` + esc + `</a>`)
	} else {
		_, _ = w.WriteString(`<span class="wikilink-broken">` + esc + `</span>`)
	}
	return ast.WalkSkipChildren, nil
}
```

Note for the implementer: the exact goldmark wiring API (`renderer.WithNodeRenderers`, `parser.WithInlineParsers`, `RegisterFuncs` signature) must match the goldmark version in `go.mod`. If the `gmhtml` import is unused, delete it and its line. Verify against the installed goldmark by reading its `renderer` and `parser` package docs (the old kompendium commit `8558375` used the same API shape and is a good reference).

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/adapter/webui/ -run 'RenderDocument' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/adapter/webui/wikilink.go internal/adapter/webui/wikilink_test.go
git add internal/adapter/webui/wikilink.go internal/adapter/webui/wikilink_test.go
git commit -m "feat(docs): WebUI RenderDocument with goldmark wikilink extension"
```

---

## Task 9: WebUI — view handler resolver + backlinks footer + CSS

**Files:**
- Modify: `internal/adapter/webui/docs.templ` (DocDetail fields + DocView footer + CSS)
- Modify: `internal/adapter/httpserver/webui_docs.go` (handleWebDocView)
- Regenerate: `internal/adapter/webui/docs_templ.go`
- Test: `internal/adapter/httpserver/webui_docs_test.go`

- [ ] **Step 1: Extend the view model + template**

In `internal/adapter/webui/docs.templ`, add a backlinks field to `DocDetail`:

```go
type DocDetail struct {
	ID        string
	Type      string
	Path      string
	Title     string
	HTML      template.HTML
	Body      string
	Backlinks []DocRow // referenced-by, reusing DocRow (ID/Type/Path/Title)
}
```

In `templ DocView`, inside the `<section ...>` after the rendered body `<div>` (after line ~145), add the backlinks footer:

```go
				if len(d.Current.Backlinks) > 0 {
					<div class="mt-4 border-t border-slate-200 pt-3">
						<div class="mb-2 text-xs font-semibold text-slate-500">↩ Referenced by</div>
						<ul class="space-y-1">
							for _, bl := range d.Current.Backlinks {
								<li>
									<a href={ templ.SafeURL("/docs/" + bl.ID) } class="text-sm text-blue-600 hover:underline">{ bl.Title }</a>
									<span class="ml-1 font-mono text-xs text-slate-400">{ bl.Path }</span>
								</li>
							}
						</ul>
					</div>
				}
```

For wikilink colours, add a small `<style>` to the `DocView` `<head>` (after the app.css link) so the `wikilink`/`wikilink-broken` classes read correctly even though Tailwind doesn't know them:

```html
				<style>
					.wikilink { color: #2563eb; text-decoration: underline; }
					.wikilink-broken { color: #e11d48; text-decoration: line-through; }
				</style>
```

- [ ] **Step 2: Update the view handler to build the resolver + fetch backlinks**

In `internal/adapter/httpserver/webui_docs.go`, replace the body of `handleWebDocView` (after fetching `doc`):

```go
	all, err := s.ListDocuments.Execute(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	resolve := func(target string) (string, string, bool) {
		if t, ok := domain.ResolveWikilink(doc, target, all); ok {
			return "/docs/" + t.ID, t.Title, true
		}
		return "", "", false
	}
	rendered := webui.RenderDocument(doc.Body, resolve)

	refs, err := s.BacklinksDocument.Execute(r.Context(), u.ID, id)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	blRows := make([]webui.DocRow, 0, len(refs))
	for _, ref := range refs {
		blRows = append(blRows, webui.DocRow{ID: ref.ID, Type: string(ref.Type), Path: ref.Path, Title: ref.Title})
	}

	d := webui.DocsPageData{
		User: u.Username,
		Current: &webui.DocDetail{
			ID: doc.ID, Type: string(doc.Type), Path: doc.Path, Title: doc.Title,
			HTML: rendered, Body: doc.Body, Backlinks: blRows,
		},
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.DocView(d).Render(r.Context(), w)
```

(Keep the existing `errors.Is(err, ports.ErrDocumentNotFound)` 404 branch for the initial `GetDocument`.)

- [ ] **Step 3: Regenerate templ + write the failing test**

Run: `templ generate` (or `make templ` — check the Makefile target name).

Append to `internal/adapter/httpserver/webui_docs_test.go` a test that a viewed doc with an inbound link shows the backlink and renders a wikilink anchor. Reuse the file's existing web-test server helper:

```go
func TestWebDocView_ShowsWikilinkAndBacklink(t *testing.T) {
	ts := newWebTestServer(t) // match the helper name already in this file
	defer ts.Close()

	// dest, then src that links it
	postForm(t, ts, "/docs", map[string]string{"type": "free", "path": "dest", "title": "Dest", "body": ""})
	postForm(t, ts, "/docs", map[string]string{"type": "free", "path": "src", "title": "Src", "body": "go to [[dest]]"})

	// find dest id via the list/home, then GET /docs/{destID}
	destID := findDocID(t, ts, "dest") // helper: scrape the list, or query the fake store the server shares
	body := getBody(t, ts, "/docs/"+destID)
	if !strings.Contains(body, "Referenced by") || !strings.Contains(body, "Src") {
		t.Fatalf("dest view should list src as backlink:\n%s", body)
	}

	srcID := findDocID(t, ts, "src")
	srcBody := getBody(t, ts, "/docs/"+srcID)
	if !strings.Contains(srcBody, `href="/docs/`+destID+`"`) {
		t.Fatalf("src view should render a wikilink anchor to dest:\n%s", srcBody)
	}
}
```

If the existing web test harness has no scrape/find helper, the simplest path is to keep a reference to the shared fake `*testutil.FakeDocumentStore` in the test and call `List` to get IDs. Read the top of `webui_docs_test.go` and adapt to its real helpers — do not invent ones that don't exist.

- [ ] **Step 4: Run to verify it fails then passes**

Run: `go test ./internal/adapter/httpserver/ -run 'WebDocView_ShowsWikilink' -v`
First expected: FAIL (before handler/templ wired). After Steps 1-2 + `templ generate`: PASS.

- [ ] **Step 5: Run the broader gate for these packages**

Run: `go test ./internal/adapter/httpserver/ ./internal/adapter/webui/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
gofmt -w internal/adapter/httpserver/webui_docs.go
git add internal/adapter/webui/docs.templ internal/adapter/webui/docs_templ.go internal/adapter/httpserver/webui_docs.go internal/adapter/httpserver/webui_docs_test.go
git commit -m "feat(docs): WebUI wikilink anchors + Referenced-by footer"
```

---

## Task 10: opener adapter (OS default browser)

**Files:**
- Create: `internal/adapter/opener/opener.go`
- Test: `internal/adapter/opener/opener_test.go`

- [ ] **Step 1: Write the failing test**

`internal/adapter/opener/opener_test.go`:

```go
package opener

import (
	"runtime"
	"testing"
)

func TestCommandFor(t *testing.T) {
	bin, args := commandFor("https://example.com")
	switch runtime.GOOS {
	case "darwin":
		if bin != "open" || len(args) != 1 || args[0] != "https://example.com" {
			t.Fatalf("darwin: got %s %v", bin, args)
		}
	case "linux":
		if bin != "xdg-open" {
			t.Fatalf("linux: got %s", bin)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/adapter/opener/ -v`
Expected: FAIL — `undefined: commandFor`.

- [ ] **Step 3: Implement**

`internal/adapter/opener/opener.go`:

```go
// Package opener launches the OS default handler for a URL (the default
// browser for http/https). Used by the docs TUI to follow real weblinks.
package opener

import (
	"os/exec"
	"runtime"
)

// Opener opens URLs in the OS default application.
type Opener struct{}

// New returns a ready Opener.
func New() *Opener { return &Opener{} }

// Open launches url without blocking. Errors from a missing opener binary are
// returned but the process is never waited on.
func (Opener) Open(url string) error {
	bin, args := commandFor(url)
	return exec.Command(bin, args...).Start()
}

func commandFor(url string) (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		return "xdg-open", []string{url}
	}
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/adapter/opener/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/adapter/opener/
git add internal/adapter/opener/
git commit -m "feat(docs): OS default-browser opener adapter"
```

---

## Task 11: TUI — weblink scanner + new styles

**Files:**
- Create: `internal/tui/weblink.go`
- Test: `internal/tui/weblink_test.go`
- Modify: `internal/tui/styles.go`

- [ ] **Step 1: Write the failing test**

`internal/tui/weblink_test.go`:

```go
package tui

import (
	"reflect"
	"testing"
)

func TestFindWeblinks(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []weblinkSpan
	}{
		{"none", "plain text", nil},
		{"bare", "go http://x.io now", []weblinkSpan{{Start: 3, End: 14, URL: "http://x.io", Display: "http://x.io"}}},
		{"https", "see https://example.com/a", []weblinkSpan{{Start: 4, End: 25, URL: "https://example.com/a", Display: "https://example.com/a"}}},
		{"markdown", "a [site](https://e.com) b", []weblinkSpan{{Start: 2, End: 23, URL: "https://e.com", Display: "site"}}},
		{"trailing punct trimmed", "(see https://e.com).", []weblinkSpan{{Start: 5, End: 18, URL: "https://e.com", Display: "https://e.com"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findWeblinks(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("findWeblinks(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run 'Weblinks' -v`
Expected: FAIL — `undefined: findWeblinks` / `weblinkSpan`.

- [ ] **Step 3: Implement the scanner**

`internal/tui/weblink.go`:

```go
package tui

import "regexp"

// weblinkSpan is one external link in a body line: a markdown `[text](url)` or
// a bare http(s) URL, with byte offsets into the line.
type weblinkSpan struct {
	Start, End int
	URL        string
	Display    string
}

var (
	// markdown link [text](http...): captured first so its inner URL is not
	// also matched as a bare URL.
	mdLinkRe = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^\s)]+)\)`)
	// bare http(s) URL.
	bareURLRe = regexp.MustCompile(`https?://[^\s)]+`)
)

// findWeblinks returns external links in s, ordered by position, with markdown
// links taking precedence over the bare URLs they contain.
func findWeblinks(s string) []weblinkSpan {
	var spans []weblinkSpan
	taken := make([]bool, len(s))

	for _, m := range mdLinkRe.FindAllStringSubmatchIndex(s, -1) {
		spans = append(spans, weblinkSpan{
			Start: m[0], End: m[1], URL: s[m[4]:m[5]], Display: s[m[2]:m[3]],
		})
		for i := m[0]; i < m[1]; i++ {
			taken[i] = true
		}
	}
	for _, loc := range bareURLRe.FindAllStringIndex(s, -1) {
		if taken[loc[0]] {
			continue // inside a markdown link already captured
		}
		url := trimTrailingPunct(s[loc[0]:loc[1]])
		spans = append(spans, weblinkSpan{
			Start: loc[0], End: loc[0] + len(url), URL: url, Display: url,
		})
	}
	sortByStart(spans)
	return spans
}

func trimTrailingPunct(u string) string {
	for len(u) > 0 {
		switch u[len(u)-1] {
		case '.', ',', ')', ']', '}', '!', '?', ';', ':':
			u = u[:len(u)-1]
		default:
			return u
		}
	}
	return u
}

func sortByStart(s []weblinkSpan) {
	for i := 0; i < len(s); i++ {
		for j := i + 1; j < len(s); j++ {
			if s[j].Start < s[i].Start {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/tui/ -run 'Weblinks' -v`
Expected: PASS.

- [ ] **Step 5: Add the styles**

In `internal/tui/styles.go`, add a cyan colour and four styles:

```go
	colCyan = lipgloss.Color("#7dcfff")
```

```go
	styleWikiValid  = lipgloss.NewStyle().Foreground(colAccent).Underline(true)
	styleWikiBroken = lipgloss.NewStyle().Foreground(colRed).Strikethrough(true)
	styleWebLink    = lipgloss.NewStyle().Foreground(colCyan).Underline(true)
	styleLinkFocus  = lipgloss.NewStyle().Foreground(colBg).Background(colAccent).Bold(true)
)
```

(Add `colCyan` to the colour `var(...)` block and the four styles to the style `var(...)` block.)

- [ ] **Step 6: Commit**

```bash
gofmt -w internal/tui/weblink.go internal/tui/weblink_test.go internal/tui/styles.go
git add internal/tui/weblink.go internal/tui/weblink_test.go internal/tui/styles.go
git commit -m "feat(docs): TUI weblink scanner + wikilink/weblink styles"
```

---

## Task 12: TUI — line styler (render wikilinks + weblinks)

**Files:**
- Modify: `internal/tui/docs.go`
- Test: `internal/tui/docs_test.go` (append; the file already tests DocsModel)

This task only renders styled links and builds the ordered focusable set; the
focus cursor + Enter dispatch come in Task 13. We add a pure helper that both
the view and the navigation use.

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/docs_test.go`:

```go
func TestBuildBodyLinks(t *testing.T) {
	all := []domain.Document{
		{ID: "d-dest", Path: "dest", Title: "Dest", Type: domain.DocFree},
	}
	src := domain.Document{ID: "d-src", Path: "src", Type: domain.DocFree}
	body := "see [[dest]], [[ghost]] and http://x.io"

	links := buildBodyLinks(body, src, all)
	// dest (valid wikilink) + weblink; ghost is broken so NOT focusable.
	if len(links) != 2 {
		t.Fatalf("want 2 focusable links, got %d: %#v", len(links), links)
	}
	if links[0].kind != linkWiki || links[0].docID != "d-dest" {
		t.Fatalf("first link should be the dest wikilink: %#v", links[0])
	}
	if links[1].kind != linkWeb || links[1].url != "http://x.io" {
		t.Fatalf("second link should be the weblink: %#v", links[1])
	}
}

func TestStyleBodyLine_BrokenWikilink(t *testing.T) {
	src := domain.Document{ID: "s", Path: "s"}
	out := styleBodyLine("x [[ghost]] y", src, nil, -1, func(string) int { return -1 })
	if !strings.Contains(out, "⊘") {
		t.Fatalf("broken wikilink should carry the ⊘ glyph: %q", out)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run 'BuildBodyLinks|StyleBodyLine' -v`
Expected: FAIL — undefined `buildBodyLinks` / `linkTarget` / `styleBodyLine` / `linkWiki` / `linkWeb`.

- [ ] **Step 3: Implement the link model + helpers**

Add to `internal/tui/docs.go` (a new section near the bottom, before `View`):

```go
// linkKind distinguishes an in-TUI wikilink jump from an external weblink.
type linkKind int

const (
	linkWiki linkKind = iota
	linkWeb
)

// linkTarget is one focusable link in the current view.
type linkTarget struct {
	kind  linkKind
	docID string // wikilink/backlink target document id
	url   string // weblink url
	label string
}

// buildBodyLinks returns the focusable links found in body, in reading order:
// resolved wikilinks (deduped per target doc) and weblinks. Broken wikilinks
// are not focusable.
func buildBodyLinks(body string, src domain.Document, all []domain.Document) []linkTarget {
	type pos struct {
		start int
		lt    linkTarget
	}
	var found []pos
	seenDoc := map[string]bool{}

	for _, sp := range domain.FindWikilinks(body) {
		if resolved, ok := domain.ResolveWikilink(src, sp.Target, all); ok {
			if seenDoc[resolved.ID] {
				continue
			}
			seenDoc[resolved.ID] = true
			label := sp.Display
			if label == "" {
				label = resolved.Title
			}
			if label == "" {
				label = sp.Target
			}
			found = append(found, pos{sp.Start, linkTarget{kind: linkWiki, docID: resolved.ID, label: label}})
		}
	}
	for _, ws := range findWeblinks(body) {
		found = append(found, pos{ws.Start, linkTarget{kind: linkWeb, url: ws.URL, label: ws.Display}})
	}
	// order by appearance
	for i := 0; i < len(found); i++ {
		for j := i + 1; j < len(found); j++ {
			if found[j].start < found[i].start {
				found[i], found[j] = found[j], found[i]
			}
		}
	}
	out := make([]linkTarget, 0, len(found))
	for _, p := range found {
		out = append(out, p.lt)
	}
	return out
}

// styleBodyLine renders one body line with styled wikilink + weblink segments.
// focusIdx is the globally-focused link index; focusOf maps a (line-local)
// occurrence to its global index (returns -1 for "not this one"). For Task 12
// callers pass a stub that never focuses; Task 13 wires the real mapping.
func styleBodyLine(line string, src domain.Document, all []domain.Document, focusIdx int, focusOf func(target string) int) string {
	// Collect spans from both scanners with a kind tag, then emit left-to-right.
	type seg struct {
		start, end int
		text       string
	}
	var segs []seg

	for _, sp := range domain.FindWikilinks(line) {
		resolved, ok := domain.ResolveWikilink(src, sp.Target, all)
		label := sp.Display
		if label == "" && ok {
			label = resolved.Title
		}
		if label == "" {
			label = sp.Target
		}
		var styled string
		if ok {
			styled = styleWikiValid.Render("→ " + label)
		} else {
			styled = styleWikiBroken.Render("⊘ " + label)
		}
		segs = append(segs, seg{sp.Start, sp.End, styled})
	}
	for _, ws := range findWeblinks(line) {
		styled := osc8(ws.URL, styleWebLink.Render(ws.Display))
		segs = append(segs, seg{ws.Start, ws.End, styled})
	}
	if len(segs) == 0 {
		return line
	}
	// sort by start; assume non-overlapping (wikilinks and weblinks don't nest)
	for i := 0; i < len(segs); i++ {
		for j := i + 1; j < len(segs); j++ {
			if segs[j].start < segs[i].start {
				segs[i], segs[j] = segs[j], segs[i]
			}
		}
	}
	var b strings.Builder
	prev := 0
	for _, sg := range segs {
		if sg.start < prev {
			continue // overlap guard
		}
		b.WriteString(line[prev:sg.start])
		b.WriteString(sg.text)
		prev = sg.end
	}
	b.WriteString(line[prev:])
	return b.String()
}

// osc8 wraps text in an OSC 8 hyperlink so terminals that support it open the
// URL on click. Harmless where unsupported.
func osc8(url, text string) string {
	return "\x1b]8;;" + url + "\x07" + text + "\x1b]8;;\x07"
}
```

(Task 12 keeps `focusIdx`/`focusOf` in the signature but unused beyond the stub; Task 13 makes the focus highlight live. If the linter flags unused params now, add a `_ = focusIdx` line — Task 13 removes it.)

Add `"strings"` to the imports if not already present (it is).

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/tui/ -run 'BuildBodyLinks|StyleBodyLine' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/tui/docs.go internal/tui/docs_test.go
git add internal/tui/docs.go internal/tui/docs_test.go
git commit -m "feat(docs): TUI body link styling + focusable link set"
```

---

## Task 13: TUI — focus cursor, Enter dispatch, back-stack, backlinks footer

**Files:**
- Modify: `internal/tui/docs.go`
- Test: `internal/tui/docs_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/docs_test.go`:

```go
type fakeOpener struct{ opened []string }

func (f *fakeOpener) Open(url string) error { f.opened = append(f.opened, url); return nil }

func TestDocsView_TabFocusAndEnter(t *testing.T) {
	// Model in view mode with one valid wikilink + one weblink.
	dest := domain.Document{ID: "d-dest", Path: "dest", Title: "Dest", Type: domain.DocFree}
	src := domain.Document{ID: "d-src", Path: "src", Type: domain.DocFree, Body: "go [[dest]] or http://x.io"}
	op := &fakeOpener{}
	m := DocsModel{
		mode:    modeView,
		viewing: &src,
		docs:    []domain.Document{src, dest},
		opener:  op,
	}
	m.viewLinks = buildBodyLinks(src.Body, src, m.docs)
	if len(m.viewLinks) != 2 {
		t.Fatalf("setup: want 2 links, got %d", len(m.viewLinks))
	}

	// Tab → focus first link (wikilink). Enter → would load dest (focus index 0).
	m2, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	mm := m2.(DocsModel)
	if mm.linkFocus != 0 {
		t.Fatalf("after Tab, linkFocus = %d, want 0", mm.linkFocus)
	}

	// Tab again → focus weblink (index 1). Enter → opener.Open called.
	m3, _ := mm.handleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	mmm := m3.(DocsModel)
	if mmm.linkFocus != 1 {
		t.Fatalf("after 2nd Tab, linkFocus = %d, want 1", mmm.linkFocus)
	}
	_, cmd := mmm.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on weblink should return a cmd")
	}
	cmd() // execute the tea.Cmd; opener.Open runs synchronously in the closure
	if len(op.opened) != 1 || op.opened[0] != "http://x.io" {
		t.Fatalf("opener.Open = %v, want [http://x.io]", op.opened)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run 'TabFocusAndEnter' -v`
Expected: FAIL — undefined `opener` field / `viewLinks` / `linkFocus`.

- [ ] **Step 3: Add model fields + opener interface**

In `internal/tui/docs.go`, add the opener interface near `docEditor`:

```go
// urlOpener opens a URL in the OS default browser. tui-local so DocsModel stays
// testable with a nil/fake opener.
type urlOpener interface {
	Open(url string) error
}
```

Add fields to `DocsModel`:

```go
	opener    urlOpener
	viewLinks []linkTarget
	linkFocus int // -1 = none focused
	viewStack []string // doc-id back-stack for in-TUI wikilink nav
```

Update `NewDocs` to take + store the opener and default `linkFocus = -1`:

```go
func NewDocs(client *apiclient.Client, ed docEditor, op urlOpener, user string) DocsModel {
	return DocsModel{client: client, editor: ed, opener: op, user: user, newType: domain.DocFree, linkFocus: -1}
}
```

(Update existing callers/tests of `NewDocs` — there is one in `cmd/flow/docs.go`, handled in Task 14, and possibly in `docs_test.go`; update those to pass `nil` for the opener.)

- [ ] **Step 4: Populate viewLinks when a doc is shown + reset focus**

In `Update`, in the `docViewMsg` case, after setting `m.viewing` / `m.mode`:

```go
	case docViewMsg:
		d := msg.doc
		m.viewing = &d
		m.mode = modeView
		m.viewLinks = buildBodyLinks(d.Body, d, m.docs)
		m.linkFocus = -1
		m.backlinks = nil
		return m, m.loadBacklinks(d.ID)
```

Add a `backlinks []domain.BacklinkRef` field to `DocsModel`, a `backlinksMsg` type, and the command:

```go
type backlinksMsg struct{ refs []domain.BacklinkRef }

func (m DocsModel) loadBacklinks(id string) tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		refs, err := m.client.Backlinks(ctx, id)
		if err != nil {
			return errMsg{err}
		}
		return backlinksMsg{refs: refs}
	}
}
```

Handle `backlinksMsg` in `Update` (append backlinks to the focusable set):

```go
	case backlinksMsg:
		m.backlinks = msg.refs
		for _, r := range msg.refs {
			m.viewLinks = append(m.viewLinks, linkTarget{kind: linkWiki, docID: r.ID, label: r.Title})
		}
		return m, nil
```

- [ ] **Step 5: Handle Tab / Shift+Tab / Enter / Esc in modeView**

Replace the `modeView` switch in `handleKey` with:

```go
	case modeView:
		switch {
		case k.Code == tea.KeyEsc:
			// pop back-stack → previous doc; else return to list
			if n := len(m.viewStack); n > 0 {
				prev := m.viewStack[n-1]
				m.viewStack = m.viewStack[:n-1]
				return m, m.loadDocNoPush(prev)
			}
			m.viewing = nil
			m.mode = modeList
			m.viewLinks = nil
			m.linkFocus = -1
			return m, nil
		case k.Text == "q" || (k.Code == 'c' && k.Mod == tea.ModCtrl):
			return m, tea.Quit
		case k.Code == tea.KeyTab && k.Mod == tea.ModShift:
			m.linkFocus = cycle(m.linkFocus, len(m.viewLinks), -1)
			return m, nil
		case k.Code == tea.KeyTab:
			m.linkFocus = cycle(m.linkFocus, len(m.viewLinks), +1)
			return m, nil
		case k.Code == tea.KeyEnter:
			return m.followFocusedLink()
		case k.Text == "e":
			if m.viewing == nil {
				return m, nil
			}
			return m, m.buildEditorCmd(m.viewing.ID)
		}
		return m, nil
```

Add the helpers:

```go
// cycle advances idx within [0,n) by delta, wrapping; -1/empty stays -1.
func cycle(idx, n, delta int) int {
	if n == 0 {
		return -1
	}
	if idx < 0 {
		if delta > 0 {
			return 0
		}
		return n - 1
	}
	return (idx + delta + n) % n
}

// followFocusedLink acts on the focused link: load a wikilink target in-TUI
// (pushing the current doc onto the back-stack) or open a weblink externally.
func (m DocsModel) followFocusedLink() (tea.Model, tea.Cmd) {
	if m.linkFocus < 0 || m.linkFocus >= len(m.viewLinks) {
		return m, nil
	}
	lt := m.viewLinks[m.linkFocus]
	switch lt.kind {
	case linkWeb:
		url := lt.url
		op := m.opener
		return m, func() tea.Msg {
			if op != nil {
				_ = op.Open(url)
			}
			return nil
		}
	case linkWiki:
		if m.viewing != nil {
			m.viewStack = append(m.viewStack, m.viewing.ID)
		}
		return m, m.loadDoc(lt.docID, false)
	}
	return m, nil
}

// loadDocNoPush loads a doc for the back-stack (Esc) without pushing again.
func (m DocsModel) loadDocNoPush(id string) tea.Cmd {
	return m.loadDoc(id, false)
}
```

- [ ] **Step 6: Render the styled body + backlinks footer + focus**

Replace `renderView`'s body loop and add a footer. The focus mapping: build a
running index as links are emitted so the focused one is highlighted. To keep
it simple and consistent with `buildBodyLinks` ordering, re-highlight by
rendering the focused link's label with `styleLinkFocus`. Replace the
`for _, ln := range strings.Split(body, "\n")` loop with:

```go
	for _, ln := range strings.Split(body, "\n") {
		b.WriteString("  " + styleBodyLine(ln, *d, m.docs, m.linkFocus, func(string) int { return -1 }) + "\n")
	}
	m.renderBacklinks(b)
```

Add the focus highlight by post-styling: since per-segment focus mapping across
lines is fiddly, apply the focus highlight to the footer + a focus hint line.
Implement `renderBacklinks`:

```go
func (m DocsModel) renderBacklinks(b *strings.Builder) {
	if len(m.backlinks) == 0 {
		return
	}
	b.WriteString("\n" + styleMuted.Render("  ↩ Referenced by") + "\n")
	for _, r := range m.backlinks {
		label := r.Title
		if label == "" {
			label = r.Path
		}
		b.WriteString("  " + styleWikiValid.Render("→ "+label) + styleMuted.Render("  "+r.Path) + "\n")
	}
}
```

For the focus indicator, add a status line under the body showing which link is
focused (simplest reliable approach given line-wrapped segments):

```go
	if m.mode == modeView && m.linkFocus >= 0 && m.linkFocus < len(m.viewLinks) {
		lt := m.viewLinks[m.linkFocus]
		tgt := lt.label
		if lt.kind == linkWeb {
			tgt = lt.url
		}
		b.WriteString("\n" + styleLinkFocus.Render(" ▸ "+tgt+" ") + styleMuted.Render("  enter to follow") + "\n")
	}
```

Place this block in `View()` after the mode switch, before the existing
status/err/footer block. (Read `View()` and insert accordingly.)

- [ ] **Step 7: Update the footer hint**

In `footer()`, change the `modeView` case:

```go
	case modeView:
		return "tab/⇧tab link · enter folgen/öffnen · e edit · esc zurück · q quit"
```

- [ ] **Step 8: Run to verify it passes**

Run: `go test ./internal/tui/ -v`
Expected: PASS (all docs tests incl. the new focus/enter test). Fix any
`NewDocs` call-site in `docs_test.go` to pass `nil` opener.

- [ ] **Step 9: Commit**

```bash
gofmt -w internal/tui/docs.go internal/tui/docs_test.go
git add internal/tui/docs.go internal/tui/docs_test.go
git commit -m "feat(docs): TUI in-TUI wikilink nav + browser weblinks + backlinks footer"
```

---

## Task 14: Wiring + verification

**Files:**
- Modify: `cmd/flow-server/main.go`
- Modify: `cmd/flow/docs.go`

- [ ] **Step 1: Wire the Backlinks use case in the server composition root**

In `cmd/flow-server/main.go`, in the `&httpserver.Server{...}` literal (after `DeleteDocument:` at line ~112) add:

```go
		BacklinksDocument: usecase.Backlinks{Docs: documentStore},
```

- [ ] **Step 2: Inject the opener into the docs TUI**

In `cmd/flow/docs.go`, add the import:

```go
	"github.com/serverkraken/flow/internal/adapter/opener"
```

Change the model construction:

```go
			m := tui.NewDocs(client, editor.New(), opener.New(), os.Getenv("USER"))
```

- [ ] **Step 3: Build everything**

Run: `go build ./...`
Expected: success, no unused/undefined errors. Fix any remaining `NewDocs`
call sites the compiler flags.

- [ ] **Step 4: Run the full gate**

Run: `make ci`
Expected: lint clean, templ up-to-date, build OK, all tests pass, coverage ≥ 80 %.
If coverage dipped below 80 %, add focused tests for the least-covered new code
(usually the `View()`/render branches) until the gate is green. Do NOT lower the
gate.

- [ ] **Step 5: Commit the wiring**

```bash
gofmt -w cmd/flow-server/main.go cmd/flow/docs.go
git add cmd/flow-server/main.go cmd/flow/docs.go
git commit -m "feat(docs): wire Backlinks use case + TUI opener into composition roots"
```

- [ ] **Step 6: Live done-gate (manual, dev stack)**

Per `reference_flow_dev_env`: `make dev-up`, `make dev-run`, get a token with
`make dev-token`. Then:

1. Migration: confirm `document_links` exists (DB version 7) — the server runs
   migrations on boot; check logs or `\dt` against the dev Postgres.
2. curl smoke (use the dev token as Bearer):
   - `POST /api/v1/documents` create `dest` (free, body empty) → 201, capture id.
   - `POST /api/v1/documents` create `src` (free, body `"go to [[dest]]"`) → 201.
   - `GET /api/v1/documents/{destID}/backlinks` → 200, array containing `src`.
   - `GET /api/v1/documents/{srcID}/backlinks` → 200, `[]`.
   - `GET /api/v1/documents/nope/backlinks` → 404.
3. WebUI dogfood (scripted Dex login): open `/docs/{srcID}` → the `[[dest]]`
   renders as a blue underlined anchor to `/docs/{destID}`; open `/docs/{destID}`
   → "↩ Referenced by" lists `src`. Add a `[[ghost]]` to `src`, save → renders
   struck-through. Add a real `https://…` link → opens externally.
4. TUI dogfood: `flow docs`, open `src`, press `Tab` to focus the `[[dest]]`
   link, `Enter` → view switches to `dest`; `Esc` → back to `src`. `Tab` to a
   weblink, `Enter` → browser opens. Open `dest` → "Referenced by" shows `src`.
5. Tear the dev stack down (`make dev-down`).

Report the results to the user and request confirmation before closing M2b.

---

## Self-Review notes (author)

- **Spec coverage:** domain scanner (T1), resolver (T2), `document_links` +
  store (T3/T4), extraction-on-save + backlinks usecase (T5), REST+SSE — SSE
  needs no new event, existing `document.*` re-fetch covered by WebUI templ
  trigger + TUI `eventMsg` reload (T6/T9/T13), apiclient (T7), WebUI render +
  footer (T8/T9), TUI nav + weblinks + opener (T10-T13), wiring + done-gate
  (T14). All spec sections map to a task.
- **Type consistency:** `domain.BacklinkRef` (T2) flows through usecase (T5),
  REST (T6), apiclient (T7); WebUI maps it to `webui.DocRow` (T9). `linkTarget`
  / `linkKind` defined T12, consumed T13. `urlOpener` (T13) implemented by
  `opener.Opener` (T10). `ReplaceLinks`/`Backlinks` signatures identical across
  port (T3), fake (T3), pgstore (T4).
- **Known limit:** TUI focus highlight is shown via a status line rather than
  in-place segment inversion (line-wrap makes per-segment focus fiddly); the
  spec's "extra highlight style" is satisfied pragmatically. Acceptable for the
  minimal renderer; revisit if it feels weak in dogfood.
