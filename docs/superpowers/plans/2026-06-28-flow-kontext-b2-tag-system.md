---
type: agent
project: github.com/serverkraken/flow
---
# flow Kontext-Redesign · B2 — Tag-System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the document-only, YAML-frontmatter-derived `tags TEXT[]` with a generic, polymorphic, explicitly-parameterised tag system — a `tags` registry + `taggings` junction over `document` · `node` · `work_session` (asset reserved) — with the body becoming pure content.

**Architecture:** Normalised `tags`(registry) + `taggings`(polymorphic junction) tables. The existing `documents.tags`/`work_sessions.tag` columns are **backfilled** into `taggings` by migration `0019` (pure SQL — tags already live in those columns), then code switches to read tags by **hydrating from the junction** and write them via an explicit `tags []string` parameter; the legacy columns are dropped last (`0020`) once no code selects them. Frontmatter parsing leaves the write path entirely; a `flow docs strip-frontmatter` maintenance command does the verlustfrei body cleanup (Go YAML parse → whole frontmatter preserved into `documents.extra.frontmatter`). Hexagonal: domain → ports → usecase → adapters (pgstore/httpserver/apiclient/webui/tui/mcp) → cmd wiring.

**Tech Stack:** Go, Postgres + pgx/v5 + goose migrations (applied on server boot via `pgstore.Migrate`), testcontainers (`pgvector/pgvector:pg16`) for pgstore tests, templ + htmx + Tailwind (WebUI), charm.land/bubbletea-v2 + lipgloss-v2 + `internal/tui` design system (TUI), cobra (CLI), `github.com/modelcontextprotocol/go-sdk/mcp` (MCP).

**Spec:** `docs/superpowers/specs/2026-06-28-flow-kontext-b2-tag-system-design.md` (read it; this plan implements §1–§11 + B2-1…B2-9). **Übersicht:** `docs/superpowers/specs/2026-06-27-flow-kontext-redesign-design.md` (D1–D11, esp. D7). **Prereq:** B1 is landed (recursive `nodes` hierarchy, migrations 0015–0018; `domain.Node`, `NodeKind`).

## Global Constraints

- Module path `github.com/serverkraken/flow`. Hexagonal — one responsibility per file, no monoliths ([[feedback_no_monoliths]]).
- **Canonical names (obey verbatim across all slices):** `domain.Tag`, `domain.TaggableType` (`TaggableDocument="document"` | `TaggableNode="node"` | `TaggableWorkSession="work_session"`), `domain.TagMatch` (`TagMatchAll`=AND iota 0, `TagMatchAny`=OR), `domain.TagScope{Type *TaggableType}`, `domain.TagCount{Tag,Count}` (already exists — keep), `domain.NormalizeTag(raw)(slug string,ok bool)`, `domain.NormalizeTags([]string)[]string`, `ports.TagStore`, `pgstore.TagStore`/`pgstore.NewTagStore`, `testutil.FakeTagStore`/`NewFakeTagStore`, tables `tags`/`taggings`, `usecase.SetTags`/`usecase.GetTags`/`usecase.TagTimeReport`, `domain.WorkSession.Tags []string` (was `Tag string`).
- **TagStore port contract (PINNED — every slice consumes exactly these signatures):**
  ```go
  type TagStore interface {
      SetTags(ctx context.Context, ownerID string, typ domain.TaggableType, taggableID string, raw []string) ([]domain.Tag, error)
      TagsFor(ctx context.Context, ownerID string, typ domain.TaggableType, taggableID string) ([]domain.Tag, error)
      TagsForMany(ctx context.Context, ownerID string, typ domain.TaggableType, ids []string) (map[string][]domain.Tag, error)
      FilterIDs(ctx context.Context, ownerID string, typ domain.TaggableType, slugs []string, mode domain.TagMatch) ([]string, error)
      ListTags(ctx context.Context, ownerID string, scope domain.TagScope) ([]domain.TagCount, error)
      ClearTaggable(ctx context.Context, ownerID string, typ domain.TaggableType, taggableID string) error
      MergeTags(ctx context.Context, ownerID, fromSlug, intoSlug string) error
  }
  ```
- **Tag identity:** `slug` = `NormalizeTag` (trim → lower → drop empty); `UNIQUE(owner_id, slug)`. `display` = first-seen raw input, **set only on tag creation** (`UpsertTags` never mutates an existing `display`). Flat — no hierarchy. Tags are **neutral** — no isolation logic in the tag (D7 reach is a B3 concern).
- **Read-hydration, not column:** after the cutover, `domain.Document.Tags` and `domain.WorkSession.Tags` are populated by **joining `taggings`** in the pgstore read methods (`List`/`Get`/`Search`/`SemanticSearch` for docs; the session reads). `domain.Document.Tags` / `domain.WorkSession.Tags` stay as JSON-serialised fields on the wire.
- **Filter semantics:** the existing `?tag=` AND-filter stays AND. `DocumentStore.List/ListPage/Search/SemanticSearch` keep their current signatures (`tags ...string` / `tags []string`); only the **implementation** changes from `tags @> $N` to a junction subquery (`GROUP BY HAVING count(DISTINCT slug)=cardinality`). `TagStore.FilterIDs` is the standalone primitive (B3 + session/node filters).
- **Frontmatter cutover:** `domain.ParseFrontmatter` leaves the document **write** path. It **stays** in the render-strip callers (`webui/markdown.go:22`, `webui/wikilink.go:64`, `tui/docs.go:453,1339,1395` — they consume only `bodyStart`, never `tags`) and in the new `strip-frontmatter` command. After body-strip those render callers become inert but harmless; do **not** delete them in B2.
- **Migrations:** goose-annotated (`-- +goose Up`/`Down`), embedded via `//go:embed migrations/*.sql`, applied on boot by `pgstore.Migrate` ([[feedback_pgstore_goose_migrations]] — bare SQL without annotations fails at apply, build doesn't catch it, only Docker pgstore tests do). Backfill/idempotent inserts use deterministic ids (`'tag-' || md5(owner_id||':'||slug)`) + `ON CONFLICT DO NOTHING`. Test a data migration by `pgstore.MigrateUpTo(ctx, pool, <N-1>)` → seed legacy rows → `pgstore.Migrate` → assert.
- **Tests:** `package <pkg>_test`, `t.Parallel()`, table-driven where natural; fakes via `testutil.New*`; struct-literal usecase injection; stdlib `errors.Is` + `t.Fatalf`/`t.Errorf` (no assert lib); httptest for handlers (`newDocServer`/`doDoc`/`primeUser` helpers); Docker Postgres via `startPG(t)` (skips when docker absent). DOCKER_HOST may need the podman socket ([[feedback_tailwind_v4_templ_gotchas]]).
- **CI:** `make ci` (gofumpt + staticcheck incl. QF1002, verify-generate/css/no-popups, coverage gate, build) must stay green and the coverage gate must not regress ([[project_flow_rebuild_phase2_planned]] — run `make ci` (lint), not just `go test`). Run `templ generate` after any `.templ` change and commit the generated `_templ.go`.
- **i18n:** WebUI strings via `components.T(ctx,"key")`/`Tn`; add keys to BOTH `internal/i18n/catalog_de.go` (full, primary) + `catalog_en.go` (stub). German primary.
- **Glyphs:** monospace whitelist only, no emoji ([[feedback_no_icons]]). Tag chips neutral `#slug`. Colors via `theme.Sem()`/`kindcolor` only.
- **Owner-scoping** on every store/usecase call. Foreign ids never leak or mutate. Cross-store calls (Docs.Create + Tags.SetTags) are **not** wrapped in one DB tx (no shared-tx infra today) — acceptable for a single-user tool; do tag writes **after** the entity write so a tag failure can't orphan a successful entity silently (log + surface).
- **Subagent hygiene:** verify `git HEAD` advanced after each task ([[feedback_subagent_git_commits_isolated]]); recover orphan commits via reflog.
- Every `git commit` message ends with the trailer `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

## Slice map (execution order; each slice ends `make ci`-green + independently testable)

| Slice | Deliverable |
|---|---|
| **A — Tag core (additive)** | `domain.Tag`/`TaggableType`/`TagMatch`/`TagScope`/`NormalizeTag` · `ports.TagStore` + `FakeTagStore` · migration `0019` (create `tags`+`taggings` + backfill from `documents.tags`/`work_sessions.tag`) · `pgstore.TagStore` + Docker tests. Nothing removed yet; tags now live in BOTH the columns and the junction. |
| **B — Document cutover** | `DocumentStore` reads hydrate `Tags` from `taggings` + AND-filter via junction subquery (signatures unchanged; `tags` dropped from `docCols`/scan — column stays in DB) · doc usecases take `Tags []string`, call `TagStore.SetTags`, stop `ParseFrontmatter` · `SetTags`/`GetTags` usecases · `DeleteDocument` clears taggings. |
| **C — Doc API surface** | `tags` on REST doc DTOs + apiclient inputs + MCP `flow_create_doc`/`flow_update_doc` · `usecase.ListTags` → registry-backed (`TagStore.ListTags`) + `TagScope`; handler + mcp `flow_list_tags` scope. |
| **D — Worktime session tags** | `domain.WorkSession.Tag string` → `Tags []string` across domain/pgstore/port/fake/usecases/http/apiclient/cli (one cohesive cutover) · `TagTimeReport` usecase + `GET /api/v1/sessions/tag-times` + `flow session stats --by-tag`. |
| **E — UI (tagging überall)** | WebUI tag editor (doc/node/session forms) + junction-backed filter · TUI tag-editor overlay (fuzzylist) on doc/node/session + `loadTags` scope fix. |
| **F — Cleanup + Wiring + Done-Gate** | `flow docs strip-frontmatter [--dry-run]` (verlustfrei body cleanup → `extra.frontmatter`) · migration `0020` drop legacy `documents.tags`+GIN / `work_sessions.tag` + remove from `docCols`/scan · composition-root wiring + curl-smoke every route + live dogfood + `make ci` + memory/spec status. |

> **Cross-slice reconciliation (PINS the genuinely ambiguous shared shapes — Slice A is authoritative; obey these over any divergent "Consumes" annotation later):**
> 1. **Migration numbering:** `0019_tags.sql` (Slice A — create + backfill, **no drop**); `0020_drop_legacy_tag_columns.sql` (Slice F — drop columns/GIN). The body-strip is **not** a migration; it is the `flow docs strip-frontmatter` command (Slice F).
> 2. **Tag id scheme:** migration backfill uses `'tag-' || md5(owner_id || ':' || slug)` (deterministic, idempotent). Runtime `UpsertTags` uses the injected `ports.IDGen` (ULID). Both write the same `tags` table; ids are opaque TEXT — never parse them.
> 3. **Hydration lives in `pgstore`, not usecases:** `DocumentStore` read methods join `taggings` directly (same package, raw SQL) for both filter and `Tags` hydration — so `domain.Document.Tags` stays populated transparently and read-usecase signatures don't change. `DocumentStore` does NOT hold a `TagStore` reference; it owns the `taggings`/`tags` SQL it needs. Same for the session reads in Slice D.
> 4. **`UpdateDocumentInput.Tags` is `*[]string`** (tri-state): `nil` = leave tags unchanged; `&[]string{}` = clear all; `&[]string{"a"}` = replace with `{a}`. Mirrors the MCP `updateDocIn` pointer semantics. `CreateDocumentInput.Tags`/`ImportDocumentInput.Tags` are plain `[]string` (always a full set on create).
> 5. **`work_sessions.tag` (singular) → `WorkSession.Tags []string`:** Slice D changes the domain field, the `SessionStore.Update` signature (`tag, note string` → `tags []string, note string`), `StartSession`/`AddSession` params (`tag, note string` → `tags []string, note string`), `EditSessionInput.Tag` → `.Tags`, and all callers in ONE task to keep `make ci` green. The `work_sessions.tag` column is removed from the SQL in that task (not selected) and DROPped in `0020`.

---

## Slice A — Tag core (additive)

> Everything in Slice A is purely additive. The existing `documents.tags`/`work_sessions.tag` columns and the `tags @> $N` filter keep working unchanged; the new `tags`/`taggings` tables run in parallel and are backfilled so later slices can switch reads onto them without a data gap.

### Task A1: Domain — `Tag`, `TaggableType`, `TagMatch`, `TagScope`, `NormalizeTag`

**Files:**
- Create: `internal/domain/tag.go`
- Create: `internal/domain/tag_test.go`
- Modify: `internal/domain/frontmatter.go` (move tag normalization here from `tag.go`'s new home — `normalizeTags` body relocates; `frontmatter.go` keeps `ParseFrontmatter`, `CollectTags`, `TagCount` and now calls `NormalizeTags`)

**Interfaces:**
- Produces: `domain.Tag`, `domain.TaggableType` (+ 3 consts), `domain.TagMatch` (+2 consts), `domain.TagScope`, `domain.NormalizeTag(raw string)(slug string, ok bool)`, `domain.NormalizeTags(in []string)[]string`. (`domain.TagCount` already exists in `frontmatter.go` — unchanged.)

- [ ] **Step 1: Write the failing test** — `internal/domain/tag_test.go`:
```go
package domain_test

import (
	"reflect"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestNormalizeTag(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in       string
		wantSlug string
		wantOK   bool
	}{
		{"Django", "django", true},
		{"  Postgres  ", "postgres", true},
		{"TF", "tf", true},
		{"", "", false},
		{"   ", "", false},
		{"lang/python", "lang/python", true},
	}
	for _, c := range cases {
		slug, ok := domain.NormalizeTag(c.in)
		if slug != c.wantSlug || ok != c.wantOK {
			t.Errorf("NormalizeTag(%q) = (%q,%v), want (%q,%v)", c.in, slug, ok, c.wantSlug, c.wantOK)
		}
	}
}

func TestNormalizeTags_DedupLowerFirstSeen(t *testing.T) {
	t.Parallel()
	got := domain.NormalizeTags([]string{"Go", "go", " TUI ", "", "Go"})
	want := []string{"go", "tui"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NormalizeTags = %v, want %v", got, want)
	}
}

func TestTaggableType_Values(t *testing.T) {
	t.Parallel()
	if domain.TaggableDocument != "document" || domain.TaggableNode != "node" || domain.TaggableWorkSession != "work_session" {
		t.Errorf("taggable type string values drifted: %q %q %q",
			domain.TaggableDocument, domain.TaggableNode, domain.TaggableWorkSession)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/ -run 'TestNormalizeTag|TestTaggableType' -v`
Expected: FAIL — `undefined: domain.NormalizeTag`, `undefined: domain.TaggableDocument`.

- [ ] **Step 3: Write `internal/domain/tag.go`:**
```go
package domain

import (
	"strings"
	"time"
)

// TaggableType is the polymorphic kind an assignment (tagging) points at.
type TaggableType string

const (
	TaggableDocument    TaggableType = "document"
	TaggableNode        TaggableType = "node"
	TaggableWorkSession TaggableType = "work_session"
	// TaggableAsset is reserved for Phase 2 (B4); not yet a valid taggable_type.
)

// TagMatch selects AND (all slugs) vs OR (any slug) filtering.
type TagMatch int

const (
	TagMatchAll TagMatch = iota // AND — the entity carries ALL given slugs
	TagMatchAny                 // OR  — the entity carries AT LEAST ONE
)

// TagScope narrows ListTags. Type nil = across all taggable types.
// NodeSubtree is reserved for B3 (hierarchy-scoped tag listing) and is unused in B2.
type TagScope struct {
	Type *TaggableType
}

// Tag is a neutral, owner-scoped label in the registry.
type Tag struct {
	ID        string    `json:"id"`
	OwnerID   string    `json:"-"`
	Slug      string    `json:"slug"`
	Display   string    `json:"display"`
	CreatedAt time.Time `json:"createdAt"`
}

// NormalizeTag trims and lowercases a raw tag into its slug identity.
// ok=false for an empty/whitespace-only input.
func NormalizeTag(raw string) (slug string, ok bool) {
	slug = strings.ToLower(strings.TrimSpace(raw))
	return slug, slug != ""
}

// NormalizeTags normalizes a list: trim, lower, drop empties, de-duplicate,
// preserving first-seen order. Returns nil for an empty result.
func NormalizeTags(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range in {
		s, ok := NormalizeTag(t)
		if !ok || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
```

- [ ] **Step 4: Relocate `normalizeTags` in `internal/domain/frontmatter.go`** — delete the unexported `normalizeTags` func (lines 65-79) and change its single caller `ParseFrontmatter` (line 62) from `return normalizeTags(fm.Tags), len(open) + after` to `return NormalizeTags(fm.Tags), len(open) + after`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/domain/ -v`
Expected: PASS (all domain tests, including the existing frontmatter/CollectTags tests still green).

- [ ] **Step 6: Commit**
```bash
git add internal/domain/tag.go internal/domain/tag_test.go internal/domain/frontmatter.go
git commit -m "feat(domain): Tag/TaggableType/TagMatch/TagScope + NormalizeTag (B2 A1)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task A2: Port `TagStore` + `testutil.FakeTagStore`

**Files:**
- Modify: `internal/ports/ports.go` (add `TagStore` interface near `DocumentStore`)
- Modify: `internal/testutil/fakes.go` (add `FakeTagStore`)
- Create: `internal/testutil/faketags_test.go` (sanity test for the fake's diff/filter logic)

**Interfaces:**
- Consumes: `domain.Tag`, `domain.TaggableType`, `domain.TagMatch`, `domain.TagScope`, `domain.TagCount`, `domain.NormalizeTag` (A1).
- Produces: `ports.TagStore` (the PINNED contract), `testutil.FakeTagStore`/`NewFakeTagStore`.

- [ ] **Step 1: Add the `TagStore` interface** to `internal/ports/ports.go` (after the `DocumentStore` interface, line ~216) — verbatim the PINNED contract from Global Constraints:
```go
// TagStore is the polymorphic tag registry + junction (B2).
type TagStore interface {
	SetTags(ctx context.Context, ownerID string, typ domain.TaggableType, taggableID string, raw []string) ([]domain.Tag, error)
	TagsFor(ctx context.Context, ownerID string, typ domain.TaggableType, taggableID string) ([]domain.Tag, error)
	TagsForMany(ctx context.Context, ownerID string, typ domain.TaggableType, ids []string) (map[string][]domain.Tag, error)
	FilterIDs(ctx context.Context, ownerID string, typ domain.TaggableType, slugs []string, mode domain.TagMatch) ([]string, error)
	ListTags(ctx context.Context, ownerID string, scope domain.TagScope) ([]domain.TagCount, error)
	ClearTaggable(ctx context.Context, ownerID string, typ domain.TaggableType, taggableID string) error
	MergeTags(ctx context.Context, ownerID, fromSlug, intoSlug string) error
}
```

- [ ] **Step 2: Write the failing fake test** — `internal/testutil/faketags_test.go`:
```go
package testutil_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
)

func TestFakeTagStore_SetThenFilterAnd(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := testutil.NewFakeTagStore()
	if _, err := ts.SetTags(ctx, "u1", domain.TaggableDocument, "d1", []string{"Go", "tui"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.SetTags(ctx, "u1", domain.TaggableDocument, "d2", []string{"go"}); err != nil {
		t.Fatal(err)
	}
	ids, err := ts.FilterIDs(ctx, "u1", domain.TaggableDocument, []string{"go", "tui"}, domain.TagMatchAll)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "d1" {
		t.Fatalf("AND filter want [d1], got %v", ids)
	}
}

func TestFakeTagStore_SetReplacesAndHydrates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := testutil.NewFakeTagStore()
	_, _ = ts.SetTags(ctx, "u1", domain.TaggableWorkSession, "s1", []string{"deep"})
	_, _ = ts.SetTags(ctx, "u1", domain.TaggableWorkSession, "s1", []string{"meeting", "deep"})
	got, err := ts.TagsFor(ctx, "u1", domain.TaggableWorkSession, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 tags after replace, got %d: %+v", len(got), got)
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./internal/testutil/ -run TestFakeTagStore -v`
Expected: FAIL — `undefined: testutil.NewFakeTagStore`.

- [ ] **Step 4: Add `FakeTagStore`** to `internal/testutil/fakes.go` (keyed by `owner|type|id`, slug set per taggable + a registry map for display/counts):
```go
// FakeTagStore is an in-memory ports.TagStore.
type FakeTagStore struct {
	mu      sync.Mutex
	display map[string]string              // owner|slug -> display
	links   map[string]map[string]bool     // owner|type|id -> set of slugs
	idgen   int
}

func NewFakeTagStore() *FakeTagStore {
	return &FakeTagStore{display: map[string]string{}, links: map[string]map[string]bool{}}
}

func (s *FakeTagStore) key(owner string, typ domain.TaggableType, id string) string {
	return owner + "|" + string(typ) + "|" + id
}

func (s *FakeTagStore) SetTags(_ context.Context, ownerID string, typ domain.TaggableType, id string, raw []string) ([]domain.Tag, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	set := map[string]bool{}
	var out []domain.Tag
	for _, r := range domain.NormalizeTags(raw) {
		set[r] = true
		dk := ownerID + "|" + r
		if _, ok := s.display[dk]; !ok {
			s.display[dk] = r
		}
		s.idgen++
		out = append(out, domain.Tag{ID: "ft", OwnerID: ownerID, Slug: r, Display: s.display[dk]})
	}
	s.links[s.key(ownerID, typ, id)] = set
	return out, nil
}

func (s *FakeTagStore) TagsFor(_ context.Context, ownerID string, typ domain.TaggableType, id string) ([]domain.Tag, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Tag
	for slug := range s.links[s.key(ownerID, typ, id)] {
		out = append(out, domain.Tag{OwnerID: ownerID, Slug: slug, Display: s.display[ownerID+"|"+slug]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

func (s *FakeTagStore) TagsForMany(ctx context.Context, ownerID string, typ domain.TaggableType, ids []string) (map[string][]domain.Tag, error) {
	out := map[string][]domain.Tag{}
	for _, id := range ids {
		t, _ := s.TagsFor(ctx, ownerID, typ, id)
		if len(t) > 0 {
			out[id] = t
		}
	}
	return out, nil
}

func (s *FakeTagStore) FilterIDs(_ context.Context, ownerID string, typ domain.TaggableType, slugs []string, mode domain.TagMatch) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := domain.NormalizeTags(slugs)
	var out []string
	prefix := ownerID + "|" + string(typ) + "|"
	for k, set := range s.links {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		n := 0
		for _, w := range want {
			if set[w] {
				n++
			}
		}
		ok := (mode == domain.TagMatchAll && n == len(want)) || (mode == domain.TagMatchAny && n > 0)
		if ok {
			out = append(out, strings.TrimPrefix(k, prefix))
		}
	}
	sort.Strings(out)
	return out, nil
}

func (s *FakeTagStore) ListTags(_ context.Context, ownerID string, scope domain.TagScope) ([]domain.TagCount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	counts := map[string]int{}
	for k, set := range s.links {
		parts := strings.SplitN(k, "|", 3)
		if parts[0] != ownerID {
			continue
		}
		if scope.Type != nil && parts[1] != string(*scope.Type) {
			continue
		}
		for slug := range set {
			counts[slug]++
		}
	}
	var out []domain.TagCount
	for slug, n := range counts {
		out = append(out, domain.TagCount{Tag: slug, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag < out[j].Tag
	})
	return out, nil
}

func (s *FakeTagStore) ClearTaggable(_ context.Context, ownerID string, typ domain.TaggableType, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.links, s.key(ownerID, typ, id))
	return nil
}

func (s *FakeTagStore) MergeTags(_ context.Context, ownerID, fromSlug, intoSlug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	from, _ := domain.NormalizeTag(fromSlug)
	into, _ := domain.NormalizeTag(intoSlug)
	prefix := ownerID + "|"
	for k, set := range s.links {
		if strings.HasPrefix(k, prefix) && set[from] {
			delete(set, from)
			set[into] = true
		}
	}
	return nil
}
```
(If `sort`/`strings`/`sync` are not yet imported in `fakes.go`, they already are — the file imports them for the other fakes.)

- [ ] **Step 5: Run to verify pass**

Run: `go test ./internal/testutil/ -run TestFakeTagStore -v`
Expected: PASS.

- [ ] **Step 6: Compile-assert the fake satisfies the port** — add to `internal/testutil/fakes.go` (near other such asserts if present, else at end):
```go
var _ ports.TagStore = (*FakeTagStore)(nil)
```
Run: `go build ./...` → Expected: success.

- [ ] **Step 7: Commit**
```bash
git add internal/ports/ports.go internal/testutil/fakes.go internal/testutil/faketags_test.go
git commit -m "feat(ports): TagStore interface + FakeTagStore (B2 A2)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task A3: Migration `0019_tags.sql` + `pgstore.TagStore`

**Files:**
- Create: `internal/adapter/pgstore/migrations/0019_tags.sql`
- Create: `internal/adapter/pgstore/tags.go`
- Create: `internal/adapter/pgstore/tags_test.go`

**Interfaces:**
- Consumes: `ports.TagStore` (A2), `domain.Tag`/`TaggableType`/`TagMatch`/`TagScope`/`TagCount`/`NormalizeTag` (A1), `ports.IDGen` (existing — `NewID() string`).
- Produces: `pgstore.TagStore`, `pgstore.NewTagStore(pool *pgxpool.Pool, ids ports.IDGen) *TagStore`. Tables `tags`/`taggings` populated by backfill.

- [ ] **Step 1: Write `internal/adapter/pgstore/migrations/0019_tags.sql`:**
```sql
-- +goose Up
CREATE TABLE tags (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL REFERENCES users(id),
    slug       TEXT NOT NULL,
    display    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (owner_id, slug)
);

CREATE TABLE taggings (
    tag_id        TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    taggable_type TEXT NOT NULL CHECK (taggable_type IN ('document','node','work_session')),
    taggable_id   TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tag_id, taggable_type, taggable_id)
);
CREATE INDEX taggings_taggable ON taggings (taggable_type, taggable_id);
CREATE INDEX taggings_tag      ON taggings (tag_id);

-- Backfill: document tags already live (normalized) in documents.tags[].
INSERT INTO tags (id, owner_id, slug, display, created_at)
SELECT DISTINCT 'tag-' || md5(d.owner_id || ':' || u.tag), d.owner_id, u.tag, u.tag, now()
FROM documents d
CROSS JOIN LATERAL unnest(d.tags) AS u(tag)
WHERE cardinality(d.tags) > 0
ON CONFLICT (owner_id, slug) DO NOTHING;

INSERT INTO taggings (tag_id, taggable_type, taggable_id, created_at)
SELECT t.id, 'document', d.id, now()
FROM documents d
CROSS JOIN LATERAL unnest(d.tags) AS u(tag)
JOIN tags t ON t.owner_id = d.owner_id AND t.slug = u.tag
ON CONFLICT (tag_id, taggable_type, taggable_id) DO NOTHING;

-- Backfill: work_sessions.tag is a single freetext value (normalize lower/trim).
INSERT INTO tags (id, owner_id, slug, display, created_at)
SELECT DISTINCT 'tag-' || md5(ws.owner_id || ':' || lower(btrim(ws.tag))), ws.owner_id, lower(btrim(ws.tag)), btrim(ws.tag), now()
FROM work_sessions ws
WHERE btrim(ws.tag) <> ''
ON CONFLICT (owner_id, slug) DO NOTHING;

INSERT INTO taggings (tag_id, taggable_type, taggable_id, created_at)
SELECT t.id, 'work_session', ws.id, now()
FROM work_sessions ws
JOIN tags t ON t.owner_id = ws.owner_id AND t.slug = lower(btrim(ws.tag))
WHERE btrim(ws.tag) <> ''
ON CONFLICT (tag_id, taggable_type, taggable_id) DO NOTHING;

-- +goose Down
DROP TABLE taggings;
DROP TABLE tags;
```

- [ ] **Step 2: Write the failing pgstore test** — `internal/adapter/pgstore/tags_test.go` (mirror the `startPG`/`NewPool`/`Migrate` harness from `documents_test.go`; seed a user FK):
```go
package pgstore_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
)

func newTagStore(t *testing.T) (*pgstore.TagStore, *pgstore.UserStore, func()) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return pgstore.NewTagStore(pool, &testutil.FakeIDGen{}), pgstore.NewUserStore(pool), func() { pool.Close() }
}

func seedUser(t *testing.T, us *pgstore.UserStore, id string) {
	t.Helper()
	if _, err := us.Upsert(context.Background(), domain.User{ID: id, Subject: "sub-" + id, Username: "u" + id}); err != nil {
		t.Fatal(err)
	}
}

func TestTagStore_SetTagsThenFilterAndHydrate(t *testing.T) {
	t.Parallel()
	ts, us, done := newTagStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")

	if _, err := ts.SetTags(ctx, "u1", domain.TaggableDocument, "d1", []string{"Go", "tui"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.SetTags(ctx, "u1", domain.TaggableDocument, "d2", []string{"go"}); err != nil {
		t.Fatal(err)
	}

	// AND filter: only d1 has both go+tui.
	ids, err := ts.FilterIDs(ctx, "u1", domain.TaggableDocument, []string{"go", "tui"}, domain.TagMatchAll)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "d1" {
		t.Fatalf("AND want [d1], got %v", ids)
	}

	// OR filter: both d1+d2 have go.
	ids, _ = ts.FilterIDs(ctx, "u1", domain.TaggableDocument, []string{"go"}, domain.TagMatchAny)
	if len(ids) != 2 {
		t.Fatalf("OR want 2, got %v", ids)
	}

	// Hydrate.
	m, err := ts.TagsForMany(ctx, "u1", domain.TaggableDocument, []string{"d1", "d2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(m["d1"]) != 2 || len(m["d2"]) != 1 {
		t.Fatalf("hydrate mismatch: %+v", m)
	}
}

func TestTagStore_SetReplacesDiff_AndDisplayFirstWriteWins(t *testing.T) {
	t.Parallel()
	ts, us, done := newTagStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")

	_, _ = ts.SetTags(ctx, "u1", domain.TaggableDocument, "d1", []string{"Django"}) // display "Django"
	_, _ = ts.SetTags(ctx, "u1", domain.TaggableDocument, "d1", []string{"django", "postgres"})

	got, _ := ts.TagsFor(ctx, "u1", domain.TaggableDocument, "d1")
	if len(got) != 2 {
		t.Fatalf("want 2 tags, got %+v", got)
	}
	for _, tg := range got {
		if tg.Slug == "django" && tg.Display != "Django" {
			t.Errorf("display should be first-write-wins 'Django', got %q", tg.Display)
		}
	}
}

func TestTagStore_ListTags_TypeScopeAndMerge(t *testing.T) {
	t.Parallel()
	ts, us, done := newTagStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	_, _ = ts.SetTags(ctx, "u1", domain.TaggableDocument, "d1", []string{"go"})
	_, _ = ts.SetTags(ctx, "u1", domain.TaggableWorkSession, "s1", []string{"go", "deep"})

	docType := domain.TaggableDocument
	tc, err := ts.ListTags(ctx, "u1", domain.TagScope{Type: &docType})
	if err != nil {
		t.Fatal(err)
	}
	if len(tc) != 1 || tc[0].Tag != "go" || tc[0].Count != 1 {
		t.Fatalf("doc-scoped ListTags want [{go,1}], got %+v", tc)
	}

	// Merge deep→go across everything.
	if err := ts.MergeTags(ctx, "u1", "deep", "go"); err != nil {
		t.Fatal(err)
	}
	all, _ := ts.ListTags(ctx, "u1", domain.TagScope{})
	for _, c := range all {
		if c.Tag == "deep" {
			t.Errorf("deep should be merged away, got %+v", all)
		}
	}
}

func TestTagStore_BackfillFromLegacyColumns(t *testing.T) {
	t.Parallel()
	// Migrate up to 0018 (pre-tags), insert a doc with tags[] + a session with a tag,
	// then run Migrate (0019 backfill) and assert taggings exist.
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pgstore.MigrateUpTo(ctx, pool, 18); err != nil {
		t.Fatal(err)
	}
	mustExec(t, pool, `INSERT INTO users (id, subject, username, created_at) VALUES ('u1','s','u',now())`)
	mustExec(t, pool, `INSERT INTO documents (id,owner_id,node_id,type,path,title,body,tags,extra,created_at,updated_at)
		VALUES ('d1','u1',NULL,'free','p','t','body',ARRAY['go','tui'],'{}',now(),now())`)
	mustExec(t, pool, `INSERT INTO work_sessions (id,owner_id,node_id,tag,note,start_at,created_at)
		VALUES ('s1','u1',NULL,'Deep','n',now(),now())`)

	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	ts := pgstore.NewTagStore(pool, &testutil.FakeIDGen{})
	got, _ := ts.TagsFor(ctx, "u1", domain.TaggableDocument, "d1")
	if len(got) != 2 {
		t.Fatalf("backfilled doc tags want 2, got %+v", got)
	}
	st, _ := ts.TagsFor(ctx, "u1", domain.TaggableWorkSession, "s1")
	if len(st) != 1 || st[0].Slug != "deep" {
		t.Fatalf("backfilled session tag want [deep], got %+v", st)
	}
}

func mustExec(t *testing.T, pool interface {
	Exec(context.Context, string, ...any) (interface{ String() string }, error)
}, sql string) {
	t.Helper()
	// replaced in Step 4 with the real pgxpool exec helper; see note
}
```
> **Note for the implementer:** `mustExec` above is a placeholder for the raw-SQL exec used to stage legacy rows. Use the project's existing test exec idiom — `pool.Exec(ctx, sql)` on `*pgxpool.Pool` (pgx returns `(pgconn.CommandTag, error)`). Replace the stub with:
> ```go
> func mustExec(t *testing.T, pool *pgxpool.Pool, sql string) {
> 	t.Helper()
> 	if _, err := pool.Exec(context.Background(), sql); err != nil {
> 		t.Fatal(err)
> 	}
> }
> ```
> and `import "github.com/jackc/pgx/v5/pgxpool"`.

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./internal/adapter/pgstore/ -run TestTagStore -v`
Expected: FAIL — `undefined: pgstore.NewTagStore` (and migration not yet present).

- [ ] **Step 4: Write `internal/adapter/pgstore/tags.go`:**
```go
package pgstore

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type TagStore struct {
	pool *pgxpool.Pool
	ids  ports.IDGen
}

func NewTagStore(pool *pgxpool.Pool, ids ports.IDGen) *TagStore { return &TagStore{pool: pool, ids: ids} }

// upsertTag returns the tag id for (owner, slug), creating it (display=raw) if new.
func (s *TagStore) upsertTag(ctx context.Context, q pgx.Tx, ownerID, slug, display string) (string, error) {
	id := "tag-" + s.ids.NewID()
	const sql = `INSERT INTO tags (id, owner_id, slug, display)
VALUES ($1,$2,$3,$4)
ON CONFLICT (owner_id, slug) DO UPDATE SET slug = EXCLUDED.slug
RETURNING id`
	var got string
	err := q.QueryRow(ctx, sql, id, ownerID, slug, display).Scan(&got)
	return got, err
}

func (s *TagStore) SetTags(ctx context.Context, ownerID string, typ domain.TaggableType, taggableID string, raw []string) ([]domain.Tag, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM taggings WHERE taggable_type=$1 AND taggable_id=$2
		AND tag_id IN (SELECT id FROM tags WHERE owner_id=$3)`, string(typ), taggableID, ownerID); err != nil {
		return nil, fmt.Errorf("pgstore: clear taggings: %w", err)
	}
	var out []domain.Tag
	for _, slug := range domain.NormalizeTags(raw) {
		tagID, err := s.upsertTag(ctx, tx, ownerID, slug, slug)
		if err != nil {
			return nil, fmt.Errorf("pgstore: upsert tag: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO taggings (tag_id, taggable_type, taggable_id)
			VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, tagID, string(typ), taggableID); err != nil {
			return nil, fmt.Errorf("pgstore: insert tagging: %w", err)
		}
		out = append(out, domain.Tag{ID: tagID, OwnerID: ownerID, Slug: slug})
	}
	return out, tx.Commit(ctx)
}

func (s *TagStore) TagsFor(ctx context.Context, ownerID string, typ domain.TaggableType, id string) ([]domain.Tag, error) {
	m, err := s.TagsForMany(ctx, ownerID, typ, []string{id})
	return m[id], err
}

func (s *TagStore) TagsForMany(ctx context.Context, ownerID string, typ domain.TaggableType, ids []string) (map[string][]domain.Tag, error) {
	out := map[string][]domain.Tag{}
	if len(ids) == 0 {
		return out, nil
	}
	const q = `SELECT tg.taggable_id, t.id, t.slug, t.display
FROM taggings tg JOIN tags t ON t.id = tg.tag_id
WHERE t.owner_id=$1 AND tg.taggable_type=$2 AND tg.taggable_id = ANY($3)
ORDER BY t.slug`
	rows, err := s.pool.Query(ctx, q, ownerID, string(typ), ids)
	if err != nil {
		return nil, fmt.Errorf("pgstore: tags for many: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var tid string
		var tag domain.Tag
		if err := rows.Scan(&tid, &tag.ID, &tag.Slug, &tag.Display); err != nil {
			return nil, err
		}
		tag.OwnerID = ownerID
		out[tid] = append(out[tid], tag)
	}
	return out, rows.Err()
}

func (s *TagStore) FilterIDs(ctx context.Context, ownerID string, typ domain.TaggableType, slugs []string, mode domain.TagMatch) ([]string, error) {
	want := domain.NormalizeTags(slugs)
	if len(want) == 0 {
		return nil, nil
	}
	q := `SELECT tg.taggable_id FROM taggings tg JOIN tags t ON t.id = tg.tag_id
WHERE t.owner_id=$1 AND tg.taggable_type=$2 AND t.slug = ANY($3)
GROUP BY tg.taggable_id`
	if mode == domain.TagMatchAll {
		q += ` HAVING count(DISTINCT t.slug) = cardinality($3)`
	}
	rows, err := s.pool.Query(ctx, q, ownerID, string(typ), want)
	if err != nil {
		return nil, fmt.Errorf("pgstore: filter ids: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *TagStore) ListTags(ctx context.Context, ownerID string, scope domain.TagScope) ([]domain.TagCount, error) {
	q := `SELECT t.slug, count(*) AS n
FROM tags t JOIN taggings tg ON tg.tag_id = t.id
WHERE t.owner_id=$1`
	args := []any{ownerID}
	if scope.Type != nil {
		args = append(args, string(*scope.Type))
		q += fmt.Sprintf(` AND tg.taggable_type = $%d`, len(args))
	}
	q += ` GROUP BY t.slug ORDER BY n DESC, t.slug ASC`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list tags: %w", err)
	}
	defer rows.Close()
	var out []domain.TagCount
	for rows.Next() {
		var tc domain.TagCount
		if err := rows.Scan(&tc.Tag, &tc.Count); err != nil {
			return nil, err
		}
		out = append(out, tc)
	}
	return out, rows.Err()
}

func (s *TagStore) ClearTaggable(ctx context.Context, ownerID string, typ domain.TaggableType, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM taggings WHERE taggable_type=$1 AND taggable_id=$2
		AND tag_id IN (SELECT id FROM tags WHERE owner_id=$3)`, string(typ), id, ownerID)
	return err
}

func (s *TagStore) MergeTags(ctx context.Context, ownerID, fromSlug, intoSlug string) error {
	from, _ := domain.NormalizeTag(fromSlug)
	into, _ := domain.NormalizeTag(intoSlug)
	if from == "" || into == "" || from == into {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	intoID, err := s.upsertTag(ctx, tx, ownerID, into, into)
	if err != nil {
		return err
	}
	// Re-point taggings from `from` to `into`, dropping dup-conflicts, then delete `from`.
	if _, err := tx.Exec(ctx, `UPDATE taggings SET tag_id=$1
		WHERE tag_id IN (SELECT id FROM tags WHERE owner_id=$2 AND slug=$3)
		AND NOT EXISTS (SELECT 1 FROM taggings x WHERE x.tag_id=$1
			AND x.taggable_type=taggings.taggable_type AND x.taggable_id=taggings.taggable_id)`,
		intoID, ownerID, from); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM tags WHERE owner_id=$1 AND slug=$2`, ownerID, from); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
```
> **Note:** confirm `ports.IDGen` exposes `NewID() string` (the constructors in `main.go:65-72` inject an IDGen into stores/usecases). If the method name differs (e.g. `New()`), match it. `pgx.Tx` is the transaction type from `github.com/jackc/pgx/v5`.

- [ ] **Step 5: Run to verify pass**

Run: `go test ./internal/adapter/pgstore/ -run TestTagStore -v`
Expected: PASS (skips with "docker unavailable" if no container runtime — run where Docker/Podman is available; set `DOCKER_HOST` to the podman socket if needed).

- [ ] **Step 6: Run the full migration set forwards+down once** to prove `0019` Up/Down are valid:

Run: `go test ./internal/adapter/pgstore/ -run 'TestTagStore_Backfill' -v`
Expected: PASS (this exercises `MigrateUpTo(18)` → seed → `Migrate` → assert).

- [ ] **Step 7: Commit**
```bash
git add internal/adapter/pgstore/migrations/0019_tags.sql internal/adapter/pgstore/tags.go internal/adapter/pgstore/tags_test.go
git commit -m "feat(pgstore): tags+taggings tables, backfill migration 0019, TagStore (B2 A3)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

- [ ] **Step 8: Slice-A gate** — `make ci` green (new code additive; existing tag behavior untouched).
```bash
make ci
```
Expected: PASS, coverage gate held.

## Slice B — Document tag cutover

> Sequenced to avoid any regression window: **B1** plumbs the explicit `tags` parameter end-to-end *additively* and double-writes (legacy `documents.tags` column **and** `taggings`), with frontmatter still working as a fallback. **B2** switches reads onto `taggings`, drops the column from the SQL, and removes the frontmatter fallback. **B3** adds the generic `SetTags`/`GetTags` usecases for non-document taggables.

### Task B1: Additive `tags` parameter (double-write; frontmatter fallback kept)

**Files:**
- Modify: `internal/usecase/create_document.go`, `update_document.go`, `import_document.go` (add `Tags` to Input; inject `Tags ports.TagStore`; double-write)
- Modify: `internal/adapter/httpserver/documents.go` (add `Tags` to the 3 DTOs; pass through)
- Modify: `internal/adapter/apiclient/documents.go` (add `Tags` to the 3 Input structs)
- Modify: `cmd/flow-mcp/tools_write.go` (add `Tags` to `createDocIn`/`updateDocIn`; pass through)
- Modify: `internal/adapter/httpserver/server_test.go` / `documents_test.go` (`newDocServer` injects `FakeTagStore`)
- Test: `internal/usecase/create_document_test.go`, `internal/adapter/httpserver/documents_test.go`

**Interfaces:**
- Consumes: `ports.TagStore`, `domain.TaggableDocument`, `testutil.FakeTagStore` (A2).
- Produces: `CreateDocumentInput.Tags []string`, `UpdateDocumentInput.Tags *[]string`, `ImportDocumentInput.Tags []string`; `createDocReq.Tags`/`updateDocReq.Tags`/`importDocReq.Tags`; apiclient inputs `.Tags`; MCP `createDocIn.Tags`/`updateDocIn.Tags`; usecase field `Tags ports.TagStore` on Create/Update/Import/Delete document usecases.

- [ ] **Step 1: Write the failing usecase test** — append to `internal/usecase/create_document_test.go`:
```go
func TestCreateDocument_ExplicitTagsParam(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	uc := usecase.CreateDocument{Docs: docs, Tags: tags, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: time.Now()}}
	got, err := uc.Execute(ctx, "u1", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "p", Title: "T", Body: "pure content, no frontmatter",
		Tags: []string{"Go", "tui"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"go", "tui"}
	if !reflect.DeepEqual(got.Tags, want) {
		t.Fatalf("response tags = %v, want %v", got.Tags, want)
	}
	stored, _ := tags.TagsFor(ctx, "u1", domain.TaggableDocument, got.ID)
	if len(stored) != 2 {
		t.Fatalf("taggings want 2, got %+v", stored)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/usecase/ -run TestCreateDocument_ExplicitTagsParam -v`
Expected: FAIL — `unknown field 'Tags' in struct literal` / `unknown field 'Tags' of usecase.CreateDocument`.

- [ ] **Step 3: Add `Tags` to the usecase Input structs + a `Tags ports.TagStore` field + a `slugsOf` helper.** In `internal/usecase/create_document.go`:
  - Add to `CreateDocumentInput`: `Tags []string`.
  - Add to the `CreateDocument` struct: `Tags ports.TagStore`.
  - Replace the frontmatter-tag line (was `tags, bodyStart := domain.ParseFrontmatter(d.Body); d.Tags = tags`) with a fallback-effective-tags computation + double-write:
```go
	_, bodyStart := domain.ParseFrontmatter(d.Body)
	eff := in.Tags
	if eff == nil { // B1 fallback: legacy frontmatter still wins when no explicit tags given
		eff, _ = domain.ParseFrontmatter(d.Body)
	}
	d.Tags = domain.NormalizeTags(eff) // legacy column double-write (removed in B2)
	if err := d.Validate(); err != nil {
		return domain.Document{}, err
	}
	created, err := uc.Docs.Create(ctx, d)
	if err != nil {
		return domain.Document{}, err
	}
	if err := uc.Docs.ReplaceLinks(ctx, created.ID, ownerID, domain.WikilinkTargets(created.Body[bodyStart:])); err != nil {
		return domain.Document{}, err
	}
	tags, err := uc.Tags.SetTags(ctx, ownerID, domain.TaggableDocument, created.ID, eff)
	if err != nil {
		return created, err
	}
	created.Tags = slugsOf(tags)
	return created, nil
```
  - Add to a shared usecase helper file (e.g. `internal/usecase/tags_helpers.go`, new):
```go
package usecase

import "github.com/serverkraken/flow/internal/domain"

func slugsOf(tags []domain.Tag) []string {
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = t.Slug
	}
	return out
}
```

- [ ] **Step 4: Mirror in `update_document.go` and `import_document.go`.**
  - `UpdateDocumentInput`: add `Tags *[]string`. `UpdateDocument` struct: add `Tags ports.TagStore`. After `Docs.Update`, replace the frontmatter line with:
```go
	if in.Tags != nil {
		tags, err := uc.Tags.SetTags(ctx, ownerID, domain.TaggableDocument, updated.ID, *in.Tags)
		if err != nil {
			return updated, err
		}
		updated.Tags = slugsOf(tags)
	} else if len(in.Body) > 0 { // B1 fallback: re-derive from frontmatter when no explicit tags
		fmTags, _ := domain.ParseFrontmatter(in.Body)
		tags, err := uc.Tags.SetTags(ctx, ownerID, domain.TaggableDocument, updated.ID, fmTags)
		if err != nil {
			return updated, err
		}
		updated.Tags = slugsOf(tags)
	}
```
  (Keep the existing legacy column write `cur.Tags = domain.ParseFrontmatter(in.Body)` for B1; B2 removes it.)
  - `ImportDocumentInput`: add `Tags []string`. `ImportDocument` struct: add `Tags ports.TagStore`. After Create, `SetTags(ctx, ownerID, domain.TaggableDocument, created.ID, effImport)` where `effImport := in.Tags; if effImport == nil { effImport, _ = domain.ParseFrontmatter(d.Body) }`.

- [ ] **Step 5: Add `Tags` to REST DTOs + handlers** in `internal/adapter/httpserver/documents.go`:
  - `createDocReq`: add `Tags []string \`json:"tags"\``; in `handleCreateDocument` pass `Tags: req.Tags` into `usecase.CreateDocumentInput{...}`.
  - `updateDocReq`: add `Tags *[]string \`json:"tags"\``; pass `Tags: req.Tags`.
  - `importDocReq`: add `Tags []string \`json:"tags"\``; pass `Tags: req.Tags`.

- [ ] **Step 6: Add `Tags` to apiclient inputs** in `internal/adapter/apiclient/documents.go`:
  - `CreateDocumentInput`: add `Tags []string \`json:"tags,omitempty"\``.
  - `UpdateDocumentInput`: add `Tags *[]string \`json:"tags,omitempty"\``.
  - `ImportDocumentInput`: add `Tags []string \`json:"tags,omitempty"\``.

- [ ] **Step 7: Add `Tags` to MCP doc-write tools** in `cmd/flow-mcp/tools_write.go`:
  - `createDocIn`: add `Tags []string \`json:"tags,omitempty" jsonschema:"tags as a flat list; replaces the whole set. Body is pure content — do NOT put tags in YAML frontmatter."\``; drop the "tags are set via YAML frontmatter" note from the `Body` jsonschema. Pass `Tags: in.Tags` into `apiclient.CreateDocumentInput{...}`.
  - `updateDocIn`: add `Tags *[]string \`json:"tags,omitempty" jsonschema:"replace the whole tag set; omit to leave unchanged; [] to clear"\``; thread into `mergeUpdate`/`UpdateDocumentInput` (set `payload.Tags = in.Tags`).

- [ ] **Step 8: Wire `FakeTagStore` into the httptest harness** — in `newDocServer` (`documents_test.go`) add `tags := testutil.NewFakeTagStore()` and set `Tags: tags` on `CreateDocument`, `ImportDocument`, `UpdateDocument`, `DeleteDocument`. Add a REST test:
```go
func TestHandleCreateDocument_TagsParam(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	primeUser(t, ts.URL)
	res := doDoc(t, ts, "POST", "/api/v1/documents",
		`{"type":"free","path":"tp","title":"T","body":"pure","tags":["go","tui"]}`)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("want 201, got %d", res.StatusCode)
	}
	var doc domain.Document
	_ = json.NewDecoder(res.Body).Decode(&doc)
	if len(doc.Tags) != 2 {
		t.Fatalf("want 2 tags, got %+v", doc.Tags)
	}
}
```

- [ ] **Step 9: Run the usecase + httpserver tests**

Run: `go test ./internal/usecase/ ./internal/adapter/httpserver/ -run 'Document' -v`
Expected: PASS — new tag-param tests pass; existing frontmatter tests still pass (fallback active).

- [ ] **Step 10: Commit**
```bash
git add internal/usecase/ internal/adapter/httpserver/documents.go internal/adapter/httpserver/documents_test.go internal/adapter/apiclient/documents.go cmd/flow-mcp/tools_write.go
git commit -m "feat(docs): explicit tags param (REST+apiclient+MCP), double-write to taggings (B2 B1)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task B2: Read cutover — hydrate from `taggings`, junction filter, drop frontmatter fallback

**Files:**
- Modify: `internal/adapter/pgstore/documents.go` (docCols/scan drop `tags`; List/ListPage/Search/SemanticSearch junction filter + hydrate; Get hydrate; Create/Update stop writing the column; delete `orEmptyTags`)
- Modify: `internal/usecase/create_document.go`, `update_document.go`, `import_document.go` (remove frontmatter fallback + legacy `d.Tags=` column write; tags ONLY from param)
- Modify: `internal/usecase/delete_document.go` (clear taggings)
- Modify: `internal/adapter/httpserver/documents_test.go` (`TestHandleListDocuments_TagFilter` seeds via `tags` param, not frontmatter)
- Test: `internal/adapter/pgstore/documents_test.go`

**Interfaces:**
- Consumes: `taggings`/`tags` tables (A3), `ports.TagStore` (delete usecase).
- Produces: `DocumentStore.List/Get/Search/SemanticSearch` return docs with `Tags` hydrated from `taggings`; `documents.tags` column no longer read or written by code.

- [ ] **Step 1: Write the failing pgstore test** — append to `documents_test.go`:
```go
func TestDocumentStore_HydratesTagsFromJunction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil { t.Fatal(err) }
	defer pool.Close()
	if err := pgstore.Migrate(ctx, pool); err != nil { t.Fatal(err) }
	us := pgstore.NewUserStore(pool)
	seedUser(t, us, "u1")
	docs := pgstore.NewDocumentStore(pool)
	tags := pgstore.NewTagStore(pool, &testutil.FakeIDGen{})

	d, err := docs.Create(ctx, domain.Document{ID: "d1", OwnerID: "u1", Type: domain.DocFree, Path: "p", Title: "T", Body: "b", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	if err != nil { t.Fatal(err) }
	if _, err := tags.SetTags(ctx, "u1", domain.TaggableDocument, d.ID, []string{"go", "tui"}); err != nil { t.Fatal(err) }

	got, err := docs.Get(ctx, "u1", "d1")
	if err != nil { t.Fatal(err) }
	if len(got.Tags) != 2 { t.Fatalf("Get hydrate want 2 tags, got %+v", got.Tags) }

	// AND filter via junction.
	list, err := docs.List(ctx, "u1", nil, "go", "tui")
	if err != nil { t.Fatal(err) }
	if len(list) != 1 || list[0].ID != "d1" { t.Fatalf("junction AND filter want [d1], got %+v", list) }
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/adapter/pgstore/ -run TestDocumentStore_HydratesTagsFromJunction -v`
Expected: FAIL — `Get` returns `Tags=nil` (still scanning the empty column), filter still uses `tags @> $N` against the now-unwritten column.

- [ ] **Step 3: Edit `internal/adapter/pgstore/documents.go`:**
  - `docCols`/`prefixedDocCols`: remove `tags`/`d.tags` (12 columns now):
```go
const docCols = `id, owner_id, node_id, type, path, title, body, doc_date, role, extra, created_at, updated_at`
const prefixedDocCols = `d.id, d.owner_id, d.node_id, d.type, d.path, d.title, d.body, d.doc_date, d.role, d.extra, d.created_at, d.updated_at`
```
  - `scanDocument`: remove `&d.Tags` from the `Scan(...)` call.
  - `Create`: drop `tags` → `VALUES ($1..$12)`, remove `orEmptyTags(d.Tags)` from the args. Delete the now-unused `orEmptyTags` helper.
  - `Update`: `UPDATE documents SET title=$1, body=$2, extra=$3, updated_at=$4 WHERE owner_id=$5 AND id=$6` (drop `tags=$3`, renumber).
  - Add the two helpers (verbatim) + call them from List/ListPage/Search/SemanticSearch/Get:
```go
// appendTagFilter adds an AND-containment junction subquery for the given tag slugs.
func appendTagFilter(q string, args *[]any, ownerID string, tags []string) string {
	if len(tags) == 0 {
		return q
	}
	*args = append(*args, ownerID, tags)
	ownPos, tagPos := len(*args)-1, len(*args)
	return q + fmt.Sprintf(` AND id IN (SELECT tg.taggable_id FROM taggings tg JOIN tags t ON t.id = tg.tag_id `+
		`WHERE t.owner_id=$%d AND tg.taggable_type='document' AND t.slug = ANY($%d) `+
		`GROUP BY tg.taggable_id HAVING count(DISTINCT t.slug) = cardinality($%d))`, ownPos, tagPos, tagPos)
}

func (s *DocumentStore) hydrateTags(ctx context.Context, ownerID string, docs []domain.Document) ([]domain.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}
	ids := make([]string, len(docs))
	for i, d := range docs {
		ids[i] = d.ID
	}
	const q = `SELECT tg.taggable_id, t.slug FROM taggings tg JOIN tags t ON t.id = tg.tag_id ` +
		`WHERE t.owner_id=$1 AND tg.taggable_type='document' AND tg.taggable_id = ANY($2) ORDER BY t.slug`
	rows, err := s.pool.Query(ctx, q, ownerID, ids)
	if err != nil {
		return nil, fmt.Errorf("pgstore: hydrate doc tags: %w", err)
	}
	defer rows.Close()
	byID := map[string][]string{}
	for rows.Next() {
		var id, slug string
		if err := rows.Scan(&id, &slug); err != nil {
			return nil, err
		}
		byID[id] = append(byID[id], slug)
	}
	for i := range docs {
		docs[i].Tags = byID[docs[i].ID]
	}
	return docs, rows.Err()
}
```
  - `List`/`ListPage`: replace the `if len(tags) > 0 { ... tags @> $N }` block with `q = appendTagFilter(q, &args, ownerID, tags)`, and wrap the scanned result: `docs, err := scanDocuments(rows); if err != nil { return ... }; return s.hydrateTags(ctx, ownerID, docs)`.
  - `Get`: after scanning the single doc, hydrate it: `out, err := scanDocument(...); if err != nil { return ... }; hyd, err := s.hydrateTags(ctx, ownerID, []domain.Document{out}); ...; return hyd[0], nil`.
  - `Search`/`SemanticSearch`: replace `tags @> $N` with `appendTagFilter(q, &args, ownerID, tags)` (note: these build `args` similarly — adapt to the existing arg-append flow), and hydrate the `domain.Document` embedded in each returned `domain.SearchHit`/`domain.SemanticHit` (read `domain.SearchHit`'s doc field name and apply the same `taggings` join over the hit ids).

- [ ] **Step 4: Remove the frontmatter fallback from the usecases** (`create_document.go`, `update_document.go`, `import_document.go`):
  - Delete the legacy column write (`d.Tags = ...`) and the `if eff == nil { eff,_ = domain.ParseFrontmatter(...) }` fallback. `eff := in.Tags` (Create/Import); Update uses only the `in.Tags != nil` branch (drop the `else if len(in.Body)>0` frontmatter branch). Keep the `_, bodyStart := domain.ParseFrontmatter(...)` line ONLY where it feeds `WikilinkTargets` (link extraction).

- [ ] **Step 5: `DeleteDocument` clears taggings** — `internal/usecase/delete_document.go`: add `Tags ports.TagStore` field; after `uc.Docs.Delete(...)`:
```go
	if err := uc.Docs.Delete(ctx, ownerID, id); err != nil {
		return err
	}
	return uc.Tags.ClearTaggable(ctx, ownerID, domain.TaggableDocument, id)
```

- [ ] **Step 6: Fix the frontmatter-seeding REST test** — in `documents_test.go` rewrite `TestHandleListDocuments_TagFilter` to seed via the `tags` param:
```go
	resA := doDoc(t, ts, "POST", "/api/v1/documents", `{"type":"free","path":"tag-filter-a","title":"A","body":"some content","tags":["go","tui"]}`)
	_ = resA.Body.Close()
	resB := doDoc(t, ts, "POST", "/api/v1/documents", `{"type":"free","path":"tag-filter-b","title":"B","body":"other","tags":["go"]}`)
	_ = resB.Body.Close()
	// ... GET ?tag=go&tag=tui still asserts exactly doc A ...
```
  Also update any usecase test that asserted tags came from frontmatter (e.g. in `create_document_test.go`) to pass `Tags:` explicitly.

- [ ] **Step 7: Run the doc tests**

Run: `go test ./internal/adapter/pgstore/ ./internal/usecase/ ./internal/adapter/httpserver/ -run 'Document' -v`
Expected: PASS.

- [ ] **Step 8: Commit**
```bash
git add internal/adapter/pgstore/documents.go internal/adapter/pgstore/documents_test.go internal/usecase/create_document.go internal/usecase/update_document.go internal/usecase/import_document.go internal/usecase/delete_document.go internal/adapter/httpserver/documents_test.go
git commit -m "feat(docs): read tags from taggings (hydrate+junction filter), drop frontmatter fallback (B2 B2)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task B3: Generic `SetTags`/`GetTags` usecases

**Files:**
- Create: `internal/usecase/set_tags.go`, `internal/usecase/get_tags.go`
- Create: `internal/usecase/set_tags_test.go`

**Interfaces:**
- Consumes: `ports.TagStore`, `domain.TaggableType`.
- Produces: `usecase.SetTags{Tags ports.TagStore}` with `Execute(ctx, ownerID string, typ domain.TaggableType, id string, raw []string) ([]domain.Tag, error)`; `usecase.GetTags{Tags ports.TagStore}` with `Execute(ctx, ownerID string, typ domain.TaggableType, id string) ([]domain.Tag, error)`. (Consumed by node/session tag REST endpoints in C2/D1 and UI in Slice E.)

- [ ] **Step 1: Write the failing test** — `internal/usecase/set_tags_test.go`:
```go
package usecase_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestSetThenGetTags(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := testutil.NewFakeTagStore()
	set := usecase.SetTags{Tags: ts}
	get := usecase.GetTags{Tags: ts}
	if _, err := set.Execute(ctx, "u1", domain.TaggableNode, "n1", []string{"infra", "terraform"}); err != nil {
		t.Fatal(err)
	}
	got, err := get.Execute(ctx, "u1", domain.TaggableNode, "n1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 tags, got %+v", got)
	}
}
```

- [ ] **Step 2: Run → fail.** `go test ./internal/usecase/ -run TestSetThenGetTags -v` → `undefined: usecase.SetTags`.

- [ ] **Step 3: Write the usecases.** `internal/usecase/set_tags.go`:
```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type SetTags struct{ Tags ports.TagStore }

func (uc SetTags) Execute(ctx context.Context, ownerID string, typ domain.TaggableType, id string, raw []string) ([]domain.Tag, error) {
	return uc.Tags.SetTags(ctx, ownerID, typ, id, raw)
}
```
`internal/usecase/get_tags.go`:
```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type GetTags struct{ Tags ports.TagStore }

func (uc GetTags) Execute(ctx context.Context, ownerID string, typ domain.TaggableType, id string) ([]domain.Tag, error) {
	return uc.Tags.TagsFor(ctx, ownerID, typ, id)
}
```

- [ ] **Step 4: Run → pass.** `go test ./internal/usecase/ -run TestSetThenGetTags -v` → PASS.

- [ ] **Step 5: Commit**
```bash
git add internal/usecase/set_tags.go internal/usecase/get_tags.go internal/usecase/set_tags_test.go
git commit -m "feat(usecase): generic SetTags/GetTags over TagStore (B2 B3)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

- [ ] **Step 6: Slice-B gate** — wire the new usecase fields in `cmd/flow-server/main.go` (`CreateDocument`/`UpdateDocument`/`ImportDocument`/`DeleteDocument` get `Tags: tagStore`; construct `tagStore := pgstore.NewTagStore(pool, ids)`), then `make ci`.
```bash
make ci
```
Expected: PASS. (If `main.go` isn't updated, the build fails on the new required struct fields — this is the wiring backstop; Slice F re-audits.)

### Task B4: Vault importer — client-side frontmatter conversion

> Once B2 stops the server parsing frontmatter, `flow docs import` (which today ships the raw body and relies on server-side parsing) would import docs with **no tags**. This task moves the conversion client-side: parse the foreign frontmatter, send `tags` explicitly, and ship a body that has been stripped of the block.

**Files:**
- Modify: `cmd/flow/docs_import.go` (`vaultFrontmatter` gains `Tags`; `runImport` extracts tags + strips body)
- Modify: `cmd/flow/docs_import_test.go` (assert tags arrive via the param, body has no frontmatter)

**Interfaces:**
- Consumes: `apiclient.ImportDocumentInput.Tags` (B1), `domain.ParseFrontmatterMap` (F1 — if B4 runs before F1, add the minimal `ParseFrontmatter` tags read here and let F1 generalise).

- [ ] **Step 1: Failing test** — `docs_import_test.go`: a vault file with `---\ntags: [go, tui]\n---\n# Title\nbody` imports via `ImportDocumentInput` whose `Tags == ["go","tui"]` and whose `Body` starts with `# Title` (no `---`). Use a fake/recording apiclient or assert the built input.

- [ ] **Step 2: Extend `vaultFrontmatter`** (`docs_import.go:39`): add `Tags []string \`yaml:"tags"\``.

- [ ] **Step 3: In `runImport`** (around line 274), compute the stripped body + tags and pass them:
```go
	tags := fmData.Tags                       // from parseVaultFrontmatter (now includes Tags)
	_, bodyStart := domain.ParseFrontmatter(body)
	cleanBody := strings.TrimLeft(body[bodyStart:], "\n")
	if _, ierr := c.ImportDocument(ctx, apiclient.ImportDocumentInput{
		Type: typ, Path: path, Title: title, Body: cleanBody, Date: date, NodeID: nodeID, Tags: tags,
	}); ierr != nil {
```
(`title` is already derived via `titleFromBody`, which uses `domain.ParseFrontmatter` for the offset — unchanged.)

- [ ] **Step 4: Run + commit**
```bash
go test ./cmd/flow/ -run Import -v
git add cmd/flow/docs_import.go cmd/flow/docs_import_test.go
git commit -m "feat(import): vault importer parses frontmatter client-side, sends tags + clean body (B2 B4)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Slice C — `ListTags` registry + node tagging API

### Task C1: `ListTags` → registry-backed + `TagScope`

**Files:**
- Modify: `internal/usecase/list_tags.go` (use `TagStore.ListTags`, add `scope` param)
- Modify: `internal/adapter/httpserver/documents.go` (`handleListTags` reads `?type=`, passes scope)
- Modify: `internal/adapter/httpserver/webui_wissen.go` (`wissenBaseVM` passes doc-type scope)
- Modify: `internal/usecase/document_test.go` (the `ListTags` assertion → `FakeTagStore`)
- Modify: `internal/adapter/httpserver/documents_test.go` (`newDocServer`: `ListTags{Tags: tags}`)

**Interfaces:**
- Consumes: `ports.TagStore.ListTags`, `domain.TagScope` (A).
- Produces: `usecase.ListTags{Tags ports.TagStore}` with `Execute(ctx, ownerID string, scope domain.TagScope) ([]domain.TagCount, error)`.

- [ ] **Step 1: Write the failing test** — rewrite the `ListTags` test in `internal/usecase/document_test.go` (or add to `list_tags_test.go`):
```go
func TestListTags_RegistryScoped(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := testutil.NewFakeTagStore()
	_, _ = ts.SetTags(ctx, "u1", domain.TaggableDocument, "d1", []string{"go", "tui"})
	_, _ = ts.SetTags(ctx, "u1", domain.TaggableDocument, "d2", []string{"go"})
	_, _ = ts.SetTags(ctx, "u1", domain.TaggableWorkSession, "s1", []string{"deep"})

	uc := usecase.ListTags{Tags: ts}
	docType := domain.TaggableDocument
	got, err := uc.Execute(ctx, "u1", domain.TagScope{Type: &docType})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Tag != "go" || got[0].Count != 2 {
		t.Fatalf("doc-scoped ListTags want go(2),tui(1), got %+v", got)
	}
}
```

- [ ] **Step 2: Run → fail.** `go test ./internal/usecase/ -run TestListTags_RegistryScoped -v` → compile error (`ListTags` has `Docs`, not `Tags`; `Execute` arity).

- [ ] **Step 3: Rewrite `internal/usecase/list_tags.go`:**
```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type ListTags struct{ Tags ports.TagStore }

func (uc ListTags) Execute(ctx context.Context, ownerID string, scope domain.TagScope) ([]domain.TagCount, error) {
	return uc.Tags.ListTags(ctx, ownerID, scope)
}
```

- [ ] **Step 4: Update `handleListTags`** (`internal/adapter/httpserver/documents.go`) to read an optional `?type=` and pass scope:
```go
func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var scope domain.TagScope
	if t := strings.TrimSpace(r.URL.Query().Get("type")); t != "" {
		tt := domain.TaggableType(t)
		scope.Type = &tt
	}
	tags, err := s.ListTags.Execute(r.Context(), u.ID, scope)
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

- [ ] **Step 5: Update `wissenBaseVM`** (`internal/adapter/httpserver/webui_wissen.go`) — the Wissen view is documents, so scope tag chips to documents (also removes cross-entity noise):
```go
	docType := domain.TaggableDocument
	allTags, err := s.ListTags.Execute(r.Context(), u.ID, domain.TagScope{Type: &docType})
```

- [ ] **Step 6: Fix `newDocServer`** — change `ListTags: usecase.ListTags{Docs: docs}` to `ListTags: usecase.ListTags{Tags: tags}`.

- [ ] **Step 7: Run.** `go test ./internal/usecase/ ./internal/adapter/httpserver/ -run 'Tag' -v` → PASS.

- [ ] **Step 8: Commit**
```bash
git add internal/usecase/list_tags.go internal/usecase/document_test.go internal/adapter/httpserver/documents.go internal/adapter/httpserver/webui_wissen.go internal/adapter/httpserver/documents_test.go
git commit -m "feat(tags): ListTags registry-backed + TagScope (B2 C1)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

> **Note:** the MCP `flow_list_tags` handler (`tools_docs.go`) keeps working unchanged — its global branch hits `GET /api/v1/documents/tags` (now registry-backed, returns the same `[]domain.TagCount` shape) and its scoped branch aggregates via `CollectTags` over hydrated docs. No change required.

---

### Task C2: Node tagging REST + apiclient; node delete clears taggings

**Files:**
- Modify: `internal/adapter/httpserver/server.go` (Server fields `SetTags`/`GetTags`; routes `/api/v1/nodes/{id}/tags`)
- Create: `internal/adapter/httpserver/nodetags.go` (handlers)
- Create: `internal/adapter/httpserver/nodetags_test.go`
- Modify: `internal/adapter/apiclient/nodes.go` (or wherever node methods live — `NodeTags`/`SetNodeTags`)
- Modify: `internal/usecase/delete_node.go` (clear taggings on delete)

**Interfaces:**
- Consumes: `usecase.SetTags`/`usecase.GetTags` (B3), `domain.TaggableNode`.
- Produces: `GET /api/v1/nodes/{id}/tags` → `[]domain.Tag`; `PUT /api/v1/nodes/{id}/tags` body `{"tags":[...]}` → `[]domain.Tag`. apiclient `NodeTags(ctx, id)`/`SetNodeTags(ctx, id, tags)`.

- [ ] **Step 1: Write the failing handler test** — `internal/adapter/httpserver/nodetags_test.go`:
```go
package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	// + the shared imports used by the other httpserver tests
)

func TestNodeTags_SetThenGet(t *testing.T) {
	srv, _ := newDocServer(t) // extend newDocServer to also wire SetTags/GetTags + a node
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	primeUser(t, ts.URL)

	// PUT tags on node "n1"
	res := doDoc(t, ts, "PUT", "/api/v1/nodes/n1/tags", `{"tags":["infra","terraform"]}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT want 200, got %d", res.StatusCode)
	}
	_ = res.Body.Close()

	res = doDoc(t, ts, "GET", "/api/v1/nodes/n1/tags", "")
	defer func() { _ = res.Body.Close() }()
	var tags []domain.Tag
	_ = json.NewDecoder(res.Body).Decode(&tags)
	if len(tags) != 2 {
		t.Fatalf("want 2 node tags, got %+v", tags)
	}
}
```
(Extend `newDocServer` to set `SetTags: usecase.SetTags{Tags: tags}` and `GetTags: usecase.GetTags{Tags: tags}`.)

- [ ] **Step 2: Run → fail** (404 — routes absent).

- [ ] **Step 3: Add Server fields + routes** (`server.go`): in the struct add `SetTags usecase.SetTags` and `GetTags usecase.GetTags`; in `Routes()` add:
```go
	mux.Handle("GET /api/v1/nodes/{id}/tags", s.auth(http.HandlerFunc(s.handleGetNodeTags)))
	mux.Handle("PUT /api/v1/nodes/{id}/tags", s.auth(http.HandlerFunc(s.handleSetNodeTags)))
```

- [ ] **Step 4: Write `internal/adapter/httpserver/nodetags.go`:**
```go
package httpserver

import (
	"encoding/json"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
)

type setTagsReq struct {
	Tags []string `json:"tags"`
}

func (s *Server) handleGetNodeTags(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	tags, err := s.GetTags.Execute(r.Context(), u.ID, domain.TaggableNode, r.PathValue("id"))
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if tags == nil {
		tags = []domain.Tag{}
	}
	writeJSON(w, http.StatusOK, tags)
}

func (s *Server) handleSetNodeTags(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req setTagsReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	tags, err := s.SetTags.Execute(r.Context(), u.ID, domain.TaggableNode, r.PathValue("id"), req.Tags)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if tags == nil {
		tags = []domain.Tag{}
	}
	s.Bus.Publish(domain.Event{Type: domain.EventNodeUpdated, UserID: u.ID, Data: map[string]any{"id": r.PathValue("id")}})
	writeJSON(w, http.StatusOK, tags)
}
```
(Use the existing `EventNodeUpdated` constant from B1; if the name differs, match it.)

- [ ] **Step 5: apiclient** — add to the node methods file:
```go
func (c *Client) NodeTags(ctx context.Context, id string) ([]domain.Tag, error) {
	var out []domain.Tag
	err := c.do(ctx, http.MethodGet, "/api/v1/nodes/"+id+"/tags", nil, &out)
	return out, err
}

func (c *Client) SetNodeTags(ctx context.Context, id string, tags []string) ([]domain.Tag, error) {
	var out []domain.Tag
	err := c.do(ctx, http.MethodPut, "/api/v1/nodes/"+id+"/tags", map[string]any{"tags": tags}, &out)
	return out, err
}
```

- [ ] **Step 6: Node delete clears taggings** — `internal/usecase/delete_node.go`: add `Tags ports.TagStore` field; after the node delete succeeds, `_ = uc.Tags.ClearTaggable(ctx, ownerID, domain.TaggableNode, id)` (best-effort; node delete is RESTRICT-guarded already).

- [ ] **Step 7: Run.** `go test ./internal/adapter/httpserver/ -run TestNodeTags -v` → PASS.

- [ ] **Step 8: Commit**
```bash
git add internal/adapter/httpserver/server.go internal/adapter/httpserver/nodetags.go internal/adapter/httpserver/nodetags_test.go internal/adapter/apiclient/ internal/usecase/delete_node.go
git commit -m "feat(nodes): node tagging REST + apiclient + clear-on-delete (B2 C2)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

- [ ] **Step 9: Slice-C gate** — wire `SetTags`/`GetTags`/`DeleteNode.Tags` in `main.go`, then `make ci`. Expected: PASS.

## Slice D — Worktime session tags

### Task D1: `WorkSession.Tag string` → `Tags []string` (cohesive cutover)

> This task changes the domain field and every reader/writer in ONE commit so `make ci` stays green (a half-done field rename does not compile). Session tags persist in `taggings` (`taggable_type='work_session'`, backfilled by `0019`); the row's `tag` column is dropped from the SQL here and DROPped in `0020` (Slice F).

**Files:**
- Modify: `internal/domain/worksession.go` (`Tag string` → `Tags []string`; adapt `NewWorkSession`)
- Modify: `internal/ports/ports.go` (`SessionStore.Update` drops the `tag` param)
- Modify: `internal/adapter/pgstore/sessions.go` (drop `tag` from all SQL; hydrate `Tags` via taggings)
- Modify: `internal/testutil/fakes.go` (`FakeSessionStore.Update` drops `tag`; store `Tags`)
- Modify: `internal/usecase/start_session.go`, `add_session.go`, `edit_session.go` (params `tag string`→`tags []string`; call `Tags.SetTags`)
- Modify: `internal/adapter/httpserver/worktime.go` (`startReq`/`editSessionReq` `Tag`→`Tags`)
- Modify: `internal/adapter/apiclient/client.go` (session methods `tag string`→`tags []string`)
- Modify: `cmd/flow/session.go` (`--tag`→`--tags` repeatable)
- Modify: any `internal/tui/**` + `internal/adapter/webui/**` that reads `session.Tag` (display join)

**Interfaces:**
- Consumes: `ports.TagStore`, `domain.TaggableWorkSession`.
- Produces: `domain.WorkSession.Tags []string`; `SessionStore.Update(ctx, ownerID, id string, nodeID *string, note string, start time.Time, stop *time.Time)`; `StartSession.Execute(ctx, ownerID, nodeID, tags []string, note string)`; `AddSession.Execute(ctx, ownerID, nodeID, start, stop, tags []string, note string)`; `EditSessionInput{NodeID, Tags []string, Note, Start, Stop}`; apiclient session methods take `tags []string`.

- [ ] **Step 1: Write the failing test** — `internal/adapter/httpserver/worktime_tags_test.go`:
```go
func TestSession_MultiTagsRoundTrip(t *testing.T) {
	srv, _ := newDocServer(t) // ensure StartSession/AddSession/EditSession wired with FakeTagStore
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	primeUser(t, ts.URL)
	// Nachbuchen a past session with two tags
	body := `{"start":"2026-06-20T09:00:00Z","stop":"2026-06-20T11:00:00Z","tags":["deep","django"],"note":"x"}`
	res := doDoc(t, ts, "POST", "/api/v1/sessions", body)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("want 201, got %d", res.StatusCode)
	}
	var s domain.WorkSession
	_ = json.NewDecoder(res.Body).Decode(&s)
	if len(s.Tags) != 2 {
		t.Fatalf("want 2 session tags, got %+v", s.Tags)
	}
}
```

- [ ] **Step 2: Run → fail** (compile: `unknown field Tags`, `s.Tag` undefined, etc.).

- [ ] **Step 3: Domain** — `internal/domain/worksession.go`: replace `Tag string \`json:"tag,omitempty"\`` with `Tags []string \`json:"tags,omitempty"\``. If `NewWorkSession` takes a `tag` arg, drop it (sessions are tagged via `TagStore` after creation, not in the constructor).

- [ ] **Step 4: Ports** — `SessionStore.Update` signature: `Update(ctx context.Context, ownerID, id string, nodeID *string, note string, start time.Time, stop *time.Time) (domain.WorkSession, error)` (drop `tag string`).

- [ ] **Step 5: pgstore `sessions.go`** — remove `tag` from the column list in INSERT/UPDATE/SELECT/`scanSession` (positions shift; column list becomes `id, owner_id, node_id, note, start_at, stop_at, created_at`). After each read path (`Running`/`Get`/`List`/`ListRange`/`ListPage`) hydrate `Tags` from taggings via a helper mirroring `documents.hydrateTags` but with `taggable_type='work_session'`:
```go
func (s *SessionStore) hydrateTags(ctx context.Context, ownerID string, ws []domain.WorkSession) ([]domain.WorkSession, error) {
	if len(ws) == 0 {
		return ws, nil
	}
	ids := make([]string, len(ws))
	for i, w := range ws {
		ids[i] = w.ID
	}
	const q = `SELECT tg.taggable_id, t.slug FROM taggings tg JOIN tags t ON t.id = tg.tag_id ` +
		`WHERE t.owner_id=$1 AND tg.taggable_type='work_session' AND tg.taggable_id = ANY($2) ORDER BY t.slug`
	rows, err := s.pool.Query(ctx, q, ownerID, ids)
	if err != nil {
		return nil, fmt.Errorf("pgstore: hydrate session tags: %w", err)
	}
	defer rows.Close()
	byID := map[string][]string{}
	for rows.Next() {
		var id, slug string
		if err := rows.Scan(&id, &slug); err != nil {
			return nil, err
		}
		byID[id] = append(byID[id], slug)
	}
	for i := range ws {
		ws[i].Tags = byID[ws[i].ID]
	}
	return ws, rows.Err()
}
```
(For single-row `Get`/`Running`/`Create`/`Update`, wrap in a one-element slice and take `[0]`.)

- [ ] **Step 6: testutil** — `FakeSessionStore.Update` drops the `tag` param + stop setting `e.Tag`; the fake stores `Tags []string` on its records (set by whatever the usecase passes — but sessions are tagged via `FakeTagStore`, so the fake session record's `Tags` stays whatever was last set; for test simplicity the usecase hydrates from `FakeTagStore` after write).

- [ ] **Step 7: Usecases** — `start_session.go`: `Execute(ctx, ownerID, nodeID *string, tags []string, note string)`; set `s.Note = note` (drop `s.Tag`); after `Sessions.Create`, `t, err := uc.Tags.SetTags(ctx, ownerID, domain.TaggableWorkSession, created.ID, tags)`; `created.Tags = slugsOf(t)`. Add `Tags ports.TagStore` field. Same shape for `add_session.go`. `edit_session.go`: `EditSessionInput.Tag`→`Tags []string`; call `Sessions.Update(ctx, ownerID, id, in.NodeID, in.Note, in.Start, in.Stop)` then `SetTags(...work_session, id, in.Tags)` + hydrate response.

- [ ] **Step 8: httpserver `worktime.go`** — `startReq.Tag string`→`Tags []string \`json:"tags"\``; `editSessionReq.Tag`→`Tags`. Update `handleStartSession` calls: `s.AddSession.Execute(..., req.Tags, req.Note)` and `s.StartSession.Execute(..., req.Tags, req.Note)`; `handleEditSession`: `usecase.EditSessionInput{NodeID: req.NodeID, Tags: req.Tags, Note: req.Note, Start: req.Start, Stop: req.Stop}`.

- [ ] **Step 9: apiclient `client.go`** — change the three session methods to `tags []string` and the body map key to `"tags": tags`:
```go
func (c *Client) StartSession(ctx context.Context, nodeID *string, tags []string, note string) (domain.WorkSession, error) {
	var s domain.WorkSession
	err := c.do(ctx, http.MethodPost, "/api/v1/sessions", map[string]any{"projectId": nodeID, "tags": tags, "note": note}, &s)
	return s, err
}
func (c *Client) EditSession(ctx context.Context, id string, nodeID *string, tags []string, note string, start time.Time, stop *time.Time) (domain.WorkSession, error) {
	var s domain.WorkSession
	err := c.do(ctx, http.MethodPatch, "/api/v1/sessions/"+id, map[string]any{"projectId": nodeID, "tags": tags, "note": note, "start": start, "stop": stop}, &s)
	return s, err
}
func (c *Client) AddSession(ctx context.Context, nodeID *string, start, stop time.Time, tags []string, note string) (domain.WorkSession, error) {
	var s domain.WorkSession
	err := c.do(ctx, http.MethodPost, "/api/v1/sessions", map[string]any{"projectId": nodeID, "tags": tags, "note": note, "start": start, "stop": stop}, &s)
	return s, err
}
```

- [ ] **Step 10: CLI `cmd/flow/session.go`** — `sessionAddInput.Tag string`→`Tags []string`; flag `cmd.Flags().StringArrayVar(&in.Tags, "tags", nil, "tag (repeatable: --tags=foo --tags=bar)")`; call `c.AddSession(ctx, pid, start, stop, in.Tags, in.Note)`. `sessionEditInput.Tag *string`→`Tags *[]string`; flag `StringArrayVar` + `if f.Changed("tags") { in.Tags = &tags }`; resolve: `tags := cur.Tags; if in.Tags != nil { tags = *in.Tags }`; `c.EditSession(ctx, id, nodeID, tags, note, start, stop)`. Update the list rendering (`session.go:147`) from `s.Tag` to `strings.Join(s.Tags, ",")`.

- [ ] **Step 11: TUI/WebUI session-tag readers** — grep and adapt:
```bash
rg -n "\.Tag\b" internal/tui internal/adapter/webui | rg -iv "tags|TagCount|TagChip"
```
For each session display of the old single `Tag`, render `strings.Join(sess.Tags, " ")` (or a chip per tag). (Worktime historie/today rows.)

- [ ] **Step 12: Run the worktime + session tests**

Run: `go test ./internal/... -run 'Session|Worktime|Stats' 2>&1 | tail -30`
Expected: PASS. Fix any compile fallout from the field rename (this is the point of the cohesive task).

- [ ] **Step 13: Commit**
```bash
git add internal/domain/worksession.go internal/ports/ports.go internal/adapter/pgstore/sessions.go internal/testutil/fakes.go internal/usecase/start_session.go internal/usecase/add_session.go internal/usecase/edit_session.go internal/adapter/httpserver/worktime.go internal/adapter/httpserver/worktime_tags_test.go internal/adapter/apiclient/client.go cmd/flow/session.go internal/tui internal/adapter/webui
git commit -m "feat(worktime): session multi-tags via taggings (Tag string -> Tags []string) (B2 D1)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task D2: `TagTimeReport` — Σ time per tag

**Files:**
- Modify: `internal/ports/ports.go` (`SessionStore.TagTimes`)
- Create: `internal/domain/tagtime.go` (`TagTime{Tag string; Minutes int}`)
- Modify: `internal/adapter/pgstore/sessions.go` (`TagTimes` query)
- Modify: `internal/testutil/fakes.go` (`FakeSessionStore.TagTimes`)
- Create: `internal/usecase/tag_time_report.go` + test
- Modify: `internal/adapter/httpserver/worktime.go` + `server.go` (`GET /api/v1/sessions/tag-times`)
- Modify: `internal/adapter/apiclient/client.go` (`TagTimes`)
- Modify: `cmd/flow/session.go` (`flow session stats --by-tag`)

**Interfaces:**
- Produces: `domain.TagTime`; `SessionStore.TagTimes(ctx, ownerID string, from, to time.Time) ([]domain.TagTime, error)`; `usecase.TagTimeReport{Sessions ports.SessionStore}`; `GET /api/v1/sessions/tag-times?from=&to=`.

- [ ] **Step 1: Failing usecase test** — `internal/usecase/tag_time_report_test.go`:
```go
func TestTagTimeReport(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	// FakeSessionStore.TagTimes returns a deterministic fixture (see Step 4)
	uc := usecase.TagTimeReport{Sessions: ss}
	_, err := uc.Execute(ctx, "u1", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: domain** — `internal/domain/tagtime.go`:
```go
package domain

// TagTime is the total tracked minutes carrying a given tag.
type TagTime struct {
	Tag     string `json:"tag"`
	Minutes int    `json:"minutes"`
}
```

- [ ] **Step 3: port + pgstore** — add `TagTimes(ctx context.Context, ownerID string, from, to time.Time) ([]domain.TagTime, error)` to `SessionStore`; implement in `sessions.go`:
```go
func (s *SessionStore) TagTimes(ctx context.Context, ownerID string, from, to time.Time) ([]domain.TagTime, error) {
	const q = `SELECT t.slug,
	  COALESCE(SUM(EXTRACT(EPOCH FROM (COALESCE(ws.stop_at, now()) - ws.start_at)))/60, 0)::int AS minutes
	FROM work_sessions ws
	JOIN taggings tg ON tg.taggable_type='work_session' AND tg.taggable_id = ws.id
	JOIN tags t ON t.id = tg.tag_id
	WHERE ws.owner_id=$1 AND ($2::timestamptz IS NULL OR ws.start_at >= $2)
	  AND ($3::timestamptz IS NULL OR ws.start_at < $3)
	GROUP BY t.slug ORDER BY minutes DESC, t.slug`
	var fromArg, toArg any
	if !from.IsZero() {
		fromArg = from
	}
	if !to.IsZero() {
		toArg = to
	}
	rows, err := s.pool.Query(ctx, q, ownerID, fromArg, toArg)
	if err != nil {
		return nil, fmt.Errorf("pgstore: tag times: %w", err)
	}
	defer rows.Close()
	var out []domain.TagTime
	for rows.Next() {
		var tt domain.TagTime
		if err := rows.Scan(&tt.Tag, &tt.Minutes); err != nil {
			return nil, err
		}
		out = append(out, tt)
	}
	return out, rows.Err()
}
```
Add a fixture-returning `TagTimes` to `FakeSessionStore`.

- [ ] **Step 4: usecase** — `internal/usecase/tag_time_report.go`:
```go
package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type TagTimeReport struct{ Sessions ports.SessionStore }

func (uc TagTimeReport) Execute(ctx context.Context, ownerID string, from, to time.Time) ([]domain.TagTime, error) {
	return uc.Sessions.TagTimes(ctx, ownerID, from, to)
}
```

- [ ] **Step 5: REST + apiclient + CLI** — add `GET /api/v1/sessions/tag-times` handler (parse `?from=&to=` RFC3339, call `s.TagTimeReport.Execute`, `writeJSON`); Server field `TagTimeReport usecase.TagTimeReport`; apiclient `TagTimes(ctx, from, to) ([]domain.TagTime, error)`; CLI `flow session stats --by-tag [--from --to]` prints `#tag   Σh`.

- [ ] **Step 6: Run + commit**
```bash
go test ./internal/... -run 'TagTime' -v
git add internal/domain/tagtime.go internal/ports/ports.go internal/adapter/pgstore/sessions.go internal/testutil/fakes.go internal/usecase/tag_time_report.go internal/usecase/tag_time_report_test.go internal/adapter/httpserver/worktime.go internal/adapter/httpserver/server.go internal/adapter/apiclient/client.go cmd/flow/session.go
git commit -m "feat(worktime): TagTimeReport (sum tracked time per tag) (B2 D2)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

- [ ] **Step 7: Slice-D gate** — wire `StartSession.Tags`/`AddSession.Tags`/`EditSession.Tags`/`TagTimeReport` in `main.go`, then `make ci`. Expected: PASS.

## Slice E — UI (tagging überall)

### Task E1: WebUI doc tag editor

**Files:**
- Modify: `internal/adapter/webui/editor.templ` (tags input + `EditorVM.TagsCSV`)
- Modify: `internal/adapter/webui/*.go` (the `EditorVM` struct — add `TagsCSV string`)
- Modify: `internal/adapter/httpserver/webui_editor.go` (parse `tags`, pass to usecase inputs; set `TagsCSV` in the VM builder)
- Modify: `internal/i18n/catalog_de.go` + `catalog_en.go` (`wissen.tags`)
- Test: `internal/adapter/httpserver/webui_editor_test.go`

**Interfaces:**
- Consumes: `CreateDocumentInput.Tags`/`UpdateDocumentInput.Tags` (B1).
- Produces: a free-text space-separated tags field on the doc create/edit form.

- [ ] **Step 1: Failing handler test** — assert posting `tags=go tui` creates a doc with two tags (use the existing webui editor test harness; if none, mirror `newDocServer` + form-POST):
```go
func TestWebEditorCreate_ParsesTags(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	primeUser(t, ts.URL)
	form := url.Values{"type": {"free"}, "path": {"e1"}, "title": {"T"}, "body": {"b"}, "tags": {"go tui"}}
	res := postForm(t, ts, "/wissen/new", form) // postForm: helper sending application/x-www-form-urlencoded with auth cookie/bearer
	defer func() { _ = res.Body.Close() }()
	// follow-up: GET the doc, assert 2 tags (or assert the redirect target + a follow GET)
}
```

- [ ] **Step 2: Add the tags input to `editor.templ`** (after the title `<label>`):
```templ
			<label class="mt-4 block">
				<span class="mb-1 block text-[.78rem] font-semibold uppercase text-muted">{ components.T(ctx, "wissen.tags") }</span>
				<input type="text" name="tags" value={ vm.TagsCSV }
					placeholder="go tui postgres"
					class="w-full rounded-xl border border-line bg-sunken/60 px-3 py-2 text-[.9rem] focus:border-blue/40 focus:outline-none"/>
			</label>
```
Add `TagsCSV string` to `EditorVM`. Run `templ generate`.

- [ ] **Step 3: Parse in the handler** (`webui_editor.go`): in `handleWebEditorCreate` add `tags := strings.Fields(r.FormValue("tags"))` and set `Tags: tags` on `usecase.CreateDocumentInput{...}`; mirror in the update handler with `Tags: &tags`. In the VM builder that loads an existing doc, set `TagsCSV: strings.Join(doc.Tags, " ")`. Surface parsed tags back into `submitted.TagsCSV` so a validation error re-render keeps them.

- [ ] **Step 4: i18n** — add `"wissen.tags": "Tags"` to `catalog_de.go` and `catalog_en.go` `strings` maps.

- [ ] **Step 5: Run + `templ generate` + commit**
```bash
templ generate
go test ./internal/adapter/httpserver/ -run WebEditor -v
git add internal/adapter/webui/editor.templ internal/adapter/webui/*_templ.go internal/adapter/webui/*.go internal/adapter/httpserver/webui_editor.go internal/adapter/httpserver/webui_editor_test.go internal/i18n/catalog_de.go internal/i18n/catalog_en.go
git commit -m "feat(webui): doc tag editor (free-text tags field) (B2 E1)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task E2: WebUI node + session tag editors

**Files:**
- Modify: the node create/edit form templ + handler (`internal/adapter/webui/node*.templ` + `internal/adapter/httpserver/webui_node*.go`) — tags field → `SetNodeTags` after save
- Modify: the worktime session edit form templ + handler — tags field → `editSessionReq.Tags`
- Modify: `internal/i18n/catalog_*.go` (reuse `wissen.tags`)

**Interfaces:**
- Consumes: `apiclient`-less direct usecase `SetTags` (node, via `s.SetTags.Execute(...TaggableNode...)`); `editSessionReq.Tags` (D1).

- [ ] **Step 1: Failing test** — node edit form POST with `tags=infra terraform` results in the node carrying 2 tags (`GET /api/v1/nodes/{id}/tags`).

- [ ] **Step 2: Node form** — add the same `<input name="tags">` to the node form templ (+ `TagsCSV` on its VM, prefilled from `GetTags` for edit). In the node create/update handler, after the node usecase succeeds, call `s.SetTags.Execute(r.Context(), u.ID, domain.TaggableNode, node.ID, strings.Fields(r.FormValue("tags")))`.

- [ ] **Step 3: Session edit form** — add the tags input to the worktime session-edit fragment; the edit handler already accepts `editSessionReq.Tags` (D1) — ensure the form posts `tags` (space-split server-side or one input per chip). Prefill from `session.Tags`.

- [ ] **Step 4: `templ generate`, run, commit**
```bash
templ generate && go test ./internal/adapter/httpserver/ -run 'Node|Session' -v
git add internal/adapter/webui internal/adapter/httpserver
git commit -m "feat(webui): node + session tag editors (B2 E2)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task E3: TUI tag editor + `loadTags` scope fix

**Files:**
- Modify: `internal/tui/docs.go` (new `modeDocTags` editor + scope the `loadTags` call)
- Create: `internal/tui/docs_tags_test.go`
- (Reuse the same overlay pattern on node detail + session edit if those TUI routes exist — fold in or note as follow-up)

**Interfaces:**
- Consumes: `client.Tags(ctx)` (or a typed scope), `client.UpdateDocument(id, {Tags})` (B1), the `renderFilter` `[x]/[ ]` toggle pattern.
- Produces: a `t` key on the doc view opening a tag editor; commit persists via `UpdateDocument`.

- [ ] **Step 1: Failing test** — `internal/tui/docs_tags_test.go`: drive the model into `modeDocTags`, toggle a tag, press Enter, assert the fake client received an `UpdateDocument` with the expected `Tags`. (Mirror the existing `docs_coverage_test.go` model-driving idiom.)

- [ ] **Step 2: Add `modeDocTags` to the `docMode` enum** and a `t` key in the `modeView` (or `modeList` on the selected doc) handler that loads all tags (scoped to documents) and seeds the editor with the current doc's tags:
```go
case k.Text == "t" && m.mode == modeView:
	return m, m.loadTags() // tagsLoadedMsg now opens modeDocTags when a doc is focused
```
Reuse the `[x]/[ ]` rendering of `renderFilter` for a `renderDocTags` (current doc's tags marked). Space toggles a tag in the working set; typing into an "add" line creates a new tag (inline); Enter commits.

- [ ] **Step 3: Commit on Enter** — build the working tag set and persist:
```go
case k.Code == tea.KeyEnter: // in handleDocTagsKey
	id, title, body, tags := m.cur.ID, m.cur.Title, m.cur.Body, append([]string(nil), m.tagWork...)
	m.mode = modeView
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := m.client.UpdateDocument(ctx, id, apiclient.UpdateDocumentInput{Title: title, Body: body, Tags: &tags}); err != nil {
			return errMsg{err}
		}
		return reloadMsg{} // or re-fetch the doc
	}
```

- [ ] **Step 4: Fix `loadTags` scope** — the cross-project bug: `m.client.Tags(ctx)` is owner-wide. Add an apiclient method `TagsScoped(ctx, typ string)` hitting `GET /api/v1/documents/tags?type=document` (or thread the project scope) and call it from `loadTags`. (The filter overlay should show document tags only.)

- [ ] **Step 5: Run + commit**
```bash
go test ./internal/tui/ -run 'DocTags|Filter' -v
git add internal/tui/docs.go internal/tui/docs_tags_test.go internal/adapter/apiclient/documents.go
git commit -m "feat(tui): doc tag editor overlay + scoped loadTags (B2 E3)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

- [ ] **Step 6: Slice-E gate** — `make ci` (incl. `templ generate` verify + no-popups guard). Expected: PASS.

## Slice F — Cleanup + Wiring + Done-Gate

### Task F1: `flow docs strip-frontmatter [--dry-run]` (verlustfrei body cleanup)

> **Delivery refinement of B2-5:** the spec framed body-strip as part of the migration, but stripping requires Go YAML parsing (preserve the whole frontmatter map into `documents.extra.frontmatter`), which can't live in a goose SQL migration. It ships instead as an idempotent, `--dry-run`-able maintenance command run once during rollout — same outcome (whole block out of body, verlustfrei, reversible via `extra.frontmatter`), safer for the real corpus, and it honours Soenne's caution about destructive body rewrites.

**Files:**
- Modify: `internal/domain/frontmatter.go` (`ParseFrontmatterMap`)
- Create: `internal/usecase/strip_frontmatter.go` + test
- Modify: `internal/adapter/httpserver/server.go` + a `maintenance.go` (`POST /api/v1/maintenance/strip-frontmatter`)
- Modify: `internal/adapter/apiclient/documents.go` (`StripFrontmatter`)
- Modify: `cmd/flow/docs.go` (or `docs_import.go`) — `flow docs strip-frontmatter [--dry-run]`

**Interfaces:**
- Produces: `domain.ParseFrontmatterMap(body string) (map[string]any, int)`; `usecase.StripFrontmatter{Docs ports.DocumentStore}` with `Execute(ctx, ownerID string, dryRun bool) (domain.StripReport, error)`; `domain.StripReport{Scanned, Stripped int}`.

- [ ] **Step 1: Failing usecase test** — `internal/usecase/strip_frontmatter_test.go`:
```go
func TestStripFrontmatter_MovesBlockToExtra(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	_, _ = docs.Create(ctx, domain.Document{ID: "d1", OwnerID: "u1", Type: domain.DocFree, Path: "p", Body: "---\ntags: [go]\naliases: [x]\n---\nreal body", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	uc := usecase.StripFrontmatter{Docs: docs, Clock: testutil.FakeClock{T: time.Now()}}

	rep, err := uc.Execute(ctx, "u1", true) // dry-run
	if err != nil || rep.Stripped != 1 {
		t.Fatalf("dry-run want Stripped=1, got %+v err=%v", rep, err)
	}
	d, _ := docs.Get(ctx, "u1", "d1")
	if d.Body != "---\ntags: [go]\naliases: [x]\n---\nreal body" {
		t.Fatalf("dry-run must not mutate, got body %q", d.Body)
	}

	_, _ = uc.Execute(ctx, "u1", false) // real
	d, _ = docs.Get(ctx, "u1", "d1")
	if d.Body != "real body" {
		t.Fatalf("body not stripped: %q", d.Body)
	}
	if d.Extra["frontmatter"] == nil {
		t.Fatalf("frontmatter not preserved into extra")
	}
}
```

- [ ] **Step 2: `ParseFrontmatterMap`** in `frontmatter.go` (full-map variant of `ParseFrontmatter`, reusing the same fence scan; unmarshal into `map[string]any`):
```go
// ParseFrontmatterMap returns the full parsed YAML frontmatter map + the body
// offset. (nil, 0) when there is no parseable leading block.
func ParseFrontmatterMap(body string) (map[string]any, int) {
	const open = "---\n"
	if !strings.HasPrefix(body, open) {
		return nil, 0
	}
	// ... identical fence scan to ParseFrontmatter to find `end`/`after` ...
	var fm map[string]any
	if err := yaml.Unmarshal([]byte(rest[:end]), &fm); err != nil {
		return nil, 0
	}
	return fm, len(open) + after
}
```
(Factor the fence-scan out of `ParseFrontmatter` into a private helper both call, to stay DRY.)

- [ ] **Step 3: usecase** — `internal/usecase/strip_frontmatter.go`:
```go
package usecase

import (
	"context"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type StripFrontmatter struct {
	Docs  ports.DocumentStore
	Clock ports.Clock
}

func (uc StripFrontmatter) Execute(ctx context.Context, ownerID string, dryRun bool) (domain.StripReport, error) {
	docs, err := uc.Docs.List(ctx, ownerID, nil)
	if err != nil {
		return domain.StripReport{}, err
	}
	var rep domain.StripReport
	for _, d := range docs {
		rep.Scanned++
		fm, bodyStart := domain.ParseFrontmatterMap(d.Body)
		if fm == nil || bodyStart == 0 {
			continue
		}
		rep.Stripped++
		if dryRun {
			continue
		}
		if d.Extra == nil {
			d.Extra = map[string]any{}
		}
		d.Extra["frontmatter"] = fm
		d.Body = strings.TrimLeft(d.Body[bodyStart:], "\n")
		d.UpdatedAt = uc.Clock.Now()
		if _, err := uc.Docs.Update(ctx, d); err != nil {
			return rep, err
		}
	}
	return rep, nil
}
```
Add `domain.StripReport{Scanned, Stripped int}` to `tag.go` or a new `domain/strip.go`.
> **Note:** `Docs.Update` persists `Title`/`Body`/`Extra` (see `documents.go` Update) — tags are untouched (managed via `taggings`). Idempotent: a second run finds no leading `---` fence (already stripped) → `Stripped=0`.

- [ ] **Step 4: REST + apiclient + CLI** — `POST /api/v1/maintenance/strip-frontmatter?dry_run=true` (authAny) → `domain.StripReport`; `Server.StripFrontmatter usecase.StripFrontmatter`; apiclient `StripFrontmatter(ctx, dryRun bool) (domain.StripReport, error)`; `flow docs strip-frontmatter [--dry-run]` prints `scanned N, stripped M`.

- [ ] **Step 5: Run + commit**
```bash
go test ./internal/usecase/ ./internal/domain/ -run 'Strip|Frontmatter' -v
git add internal/domain/frontmatter.go internal/domain/strip.go internal/usecase/strip_frontmatter.go internal/usecase/strip_frontmatter_test.go internal/adapter/httpserver internal/adapter/apiclient/documents.go cmd/flow/docs.go
git commit -m "feat(docs): strip-frontmatter maintenance command (verlustfrei -> extra.frontmatter) (B2 F1)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task F2: Migration `0020` — drop legacy tag columns

**Files:**
- Create: `internal/adapter/pgstore/migrations/0020_drop_legacy_tag_columns.sql`
- Modify: `internal/adapter/pgstore/documents_test.go` (assert latest migration applies clean + a doc with no taggings has empty `Tags`)

**Interfaces:** none new — purely removes the now-unreferenced columns.

- [ ] **Step 1: Write `0020_drop_legacy_tag_columns.sql`:**
```sql
-- +goose Up
-- The tags now live entirely in taggings (backfilled by 0019); no code selects
-- these columns after the B2 read-cutover. Drop them.
DROP INDEX IF EXISTS documents_tags_gin;
ALTER TABLE documents DROP COLUMN tags;
ALTER TABLE work_sessions DROP COLUMN tag;

-- +goose Down
ALTER TABLE work_sessions ADD COLUMN tag TEXT NOT NULL DEFAULT '';
ALTER TABLE documents ADD COLUMN tags TEXT[] NOT NULL DEFAULT '{}';
CREATE INDEX documents_tags_gin ON documents USING GIN (tags);
-- Re-project tags from taggings (best-effort; display casing not restored).
UPDATE documents d SET tags = sub.arr
FROM (SELECT tg.taggable_id AS id, array_agg(t.slug ORDER BY t.slug) AS arr
      FROM taggings tg JOIN tags t ON t.id = tg.tag_id
      WHERE tg.taggable_type='document' GROUP BY tg.taggable_id) sub
WHERE d.id = sub.id;
```

- [ ] **Step 2: Test that the full migration set applies + a no-tag doc round-trips** — add to `documents_test.go`:
```go
func TestDocumentStore_NoTagsHydratesEmpty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil { t.Fatal(err) }
	defer pool.Close()
	if err := pgstore.Migrate(ctx, pool); err != nil { t.Fatal(err) } // applies through 0020
	us := pgstore.NewUserStore(pool); seedUser(t, us, "u1")
	docs := pgstore.NewDocumentStore(pool)
	d, err := docs.Create(ctx, domain.Document{ID: "d1", OwnerID: "u1", Type: domain.DocFree, Path: "p", Title: "T", Body: "b", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	if err != nil { t.Fatal(err) }
	if len(d.Tags) != 0 { t.Fatalf("want no tags, got %+v", d.Tags) }
}
```

- [ ] **Step 3: Run + commit**
```bash
go test ./internal/adapter/pgstore/ -run 'Document|TagStore|Session' -v
git add internal/adapter/pgstore/migrations/0020_drop_legacy_tag_columns.sql internal/adapter/pgstore/documents_test.go
git commit -m "feat(pgstore): drop legacy documents.tags/work_sessions.tag columns (migration 0020) (B2 F2)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task F3: Composition-root wiring + curl-smoke + done-gate

> [[feedback_plan_main_wiring_task]]: per-task reviews don't catch "the composition root never calls the new constructor." This task audits `cmd/flow-server/main.go` and curl-smokes every new/changed route against a live server.

**Files:**
- Modify: `cmd/flow-server/main.go` (construct `tagStore`; inject `Tags`/`SetTags`/`GetTags`/`ListTags`/`TagTimeReport`/`StripFrontmatter` into every usecase + `Server` field that gained them)
- Modify: `docs/superpowers/specs/2026-06-28-flow-kontext-b2-tag-system-design.md` (status → implemented)
- Update memory per the repo's memory rules

- [ ] **Step 1: Audit the composition root.** Grep every usecase/Server field that gained a `Tags ports.TagStore` (CreateDocument, UpdateDocument, ImportDocument, DeleteDocument, DeleteNode, StartSession, AddSession, EditSession, SetTags, GetTags, ListTags, TagTimeReport, StripFrontmatter) and confirm `main.go` constructs `tagStore := pgstore.NewTagStore(pool, ids)` and injects it into ALL of them. Build:
```bash
go build ./... && echo "wiring compiles"
```

- [ ] **Step 2: `make ci`** — full gate green (gofumpt, staticcheck incl. QF1002, verify-generate/css/no-popups, coverage gate not regressed, build).
```bash
make ci
```

- [ ] **Step 3: Bring up the dev stack** ([[reference_flow_dev_env]]): `make dev-up && make dev-run` (FLOW_DEV=1, Postgres + Dex). In another shell `TOKEN=$(make dev-token)`.

- [ ] **Step 4: curl-smoke every new/changed route** (expect the noted status; a non-2xx is a wiring miss):
```bash
BASE=http://localhost:8080; H="Authorization: Bearer $TOKEN"
# doc create with explicit tags (no frontmatter in body)
curl -fsS -X POST $BASE/api/v1/documents -H "$H" -H 'Content-Type: application/json' \
  -d '{"type":"free","path":"b2-smoke","title":"Smoke","body":"pure content","tags":["b2","django"]}' | tee /tmp/doc.json
# AND filter
curl -fsS "$BASE/api/v1/documents?tag=b2&tag=django" -H "$H" | grep b2-smoke
# registry tag list, doc-scoped
curl -fsS "$BASE/api/v1/documents/tags?type=document" -H "$H"
# node tags
curl -fsS -X PUT $BASE/api/v1/nodes/<engagement-id>/tags -H "$H" -H 'Content-Type: application/json' -d '{"tags":["infra"]}'
curl -fsS $BASE/api/v1/nodes/<engagement-id>/tags -H "$H"
# session with tags + tag-time report
curl -fsS -X POST $BASE/api/v1/sessions -H "$H" -H 'Content-Type: application/json' \
  -d '{"start":"2026-06-20T09:00:00Z","stop":"2026-06-20T11:00:00Z","tags":["deep","django"],"note":"x"}'
curl -fsS "$BASE/api/v1/sessions/tag-times" -H "$H"
# strip-frontmatter dry-run
curl -fsS -X POST "$BASE/api/v1/maintenance/strip-frontmatter?dry_run=true" -H "$H"
```

- [ ] **Step 5: Live dogfood** — TUI (`flow ui wissen`): create a doc with the `t` tag editor, filter by tag, confirm body has no frontmatter; WebUI (`/wissen`): edit a doc's tags, filter chips work, node form tags persist. `flow session add --tags deep --tags django ...` then `flow session stats --by-tag`. Run `flow docs strip-frontmatter --dry-run` then for-real, re-open a doc, confirm clean body.

- [ ] **Step 6: Run the real strip on the dev corpus** and verify idempotency (second run reports `stripped 0`).

- [ ] **Step 7: Flip spec status + memory.** Set the spec header `Status:` → `implementiert`. Write a memory note per the repo rules (project memory: B2 done, commit range, `make ci` %, dogfood result). Mirror the spec/plan to flow (`flow_create_doc` / `flow_update_doc`) once auth is restored.

- [ ] **Step 8: Final commit**
```bash
git add cmd/flow-server/main.go docs/superpowers/specs/2026-06-28-flow-kontext-b2-tag-system-design.md
git commit -m "chore(b2): wire TagStore composition root + flip spec to implemented (B2 F3)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Done-Gate Summary (the whole B2 is done when ALL hold)

- [ ] `make ci` green; coverage gate not regressed.
- [ ] Migrations `0019` (create+backfill) and `0020` (drop legacy columns) apply clean forward; pgstore Docker tests green (incl. `MigrateUpTo` backfill test).
- [ ] A document created with `tags:["a","b"]` and **no YAML frontmatter** in its body round-trips with those tags; `?tag=a&tag=b` AND-filters correctly; `documents.tags` column is gone.
- [ ] A work session carries multiple tags; `flow session stats --by-tag` / `GET /sessions/tag-times` aggregates time per tag across engagements; `work_sessions.tag` column is gone.
- [ ] Nodes are taggable via `PUT /nodes/{id}/tags`; deleting a taggable clears its `taggings`.
- [ ] `flow_create_doc`/`flow_update_doc` accept `tags`; the Body jsonschema no longer mentions frontmatter.
- [ ] WebUI + TUI can add/remove tags + filter on docs/nodes/sessions; TUI `loadTags` is scoped (no cross-project bleed).
- [ ] `flow docs strip-frontmatter` ran on the corpus; bodies are pure content; original frontmatter preserved in `extra.frontmatter`; second run is a no-op.
- [ ] Spec status flipped to implemented; memory written; flow mirror updated (when auth restored).

## Deferred (NOT in B2 — per spec §"Out")

- Rename/Merge **UI** (the `MergeTags` store primitive ships; the bedien-surface is Querschnitt A/B).
- Tag `color`/`description`/lifecycle (`veraltet`) — per-taggable lifecycle is Querschnitt A.
- Asset tagging (Phase 2, B4) — `taggable_type='asset'` reserved.
- Bootstrap tag reach (D7 global-only) — Kontext-Store B3.
- Tags in search ranking (tsvector/RRF).





