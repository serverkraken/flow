# flow rebuild M2c — Tags & Filter (Design)

**Date:** 2026-06-15
**Branch:** `rebuild` (long-lived orphan, not merged)
**Predecessor:** M2b Wikilinks & Backlinks (`8f24790..e75a8e7`)
**Status:** Design approved — ready for implementation plan.

## Goal

Make tags first-class in the compendium: a document carries tags entered via a
YAML frontmatter block in its body, both hosts display them, and the document
list can be filtered by one or more tags. Available in **both hosts** (WebUI +
TUI) plus the CLI, following the generic-in-every-host principle.

This is the third slice of the M2 Kompendium vertical. Search (M2d) and pgvector
(M2e) come later and are out of scope here.

### Starting point

`Document.Tags []string` has existed since M2a and is persisted as a Postgres
`TEXT[]` column. The **update** path already threads tags through
(`UpdateDocumentInput.Tags`, REST `PUT` `tags`), but **create** does not, and
**no host** lets a user enter, see, or filter by tags. The WebUI even carries a
"preserve immutable tags (FIX 4)" hack because it has no tag input. M2c turns
the latent schema into a real feature.

## Core decisions (locked during brainstorming)

1. **Tag input:** YAML **frontmatter** in the document body
   (`---\ntags: [go, tui]\n---`). The one place a user types tags is the same
   `$EDITOR` they already use for the whole document. No per-host tag UI.
2. **Frontmatter storage:** kept **verbatim in the stored body** (lossless — the
   tags are right there on the next `$EDITOR` open). Renderers skip the block so
   it never shows as content.
3. **Source of truth for filtering:** the `documents.tags` **column**, not body
   parsing. The use case parses frontmatter → writes the column on save; filter
   and counts read the column.
4. **Filter semantics:** **multiple tags, AND**, store-level via Postgres
   `tags @> ARRAY[...]`.
5. **Tag discovery:** a **dedicated endpoint** `GET /api/v1/documents/tags`
   → `[{tag, count}]`, with apiclient support, so every consumer sees the same
   list.
6. **Write API:** the explicit `tags` field is **removed** from POST/PUT.
   Frontmatter is the single source; programmatic callers put frontmatter in the
   body. This deletes the WebUI FIX-4 hack.

## Architecture

Vertical slice mirroring M2a/M2b: **domain → store/migration → usecase →
REST+SSE → apiclient → WebUI → TUI**. Each unit below has one purpose and a
defined interface.

### 1. Domain — frontmatter authority

`internal/domain/frontmatter.go` — pure, no adapter deps. The single place that
knows the frontmatter syntax and tag normalization.

- `type TagCount struct { Tag string; Count int }`
- `ParseFrontmatter(body string) (tags []string, bodyStart int)`:
  - if `body` begins with `---\n`, find the next `\n---\n` (or `\n---` at EOF);
    parse the enclosed YAML with `yaml.v3` for a `tags:` key, accepting both
    inline (`tags: [a, b]`) and block-list (`tags:\n  - a\n  - b`) forms;
  - **normalize** each tag: trim, lowercase, drop empties, de-duplicate,
    preserve first-seen order;
  - return the normalized tags and the byte offset where the real body begins
    (after the closing fence + newline), for renderers to skip;
  - no leading fence, no closing fence, or unparseable YAML → `(nil, 0)`
    (treat the whole body as content; a malformed block renders literally rather
    than swallowing text).
- `CollectTags(docs []Document) []TagCount` — aggregate counts across a document
  set, sorted by count desc then tag asc. Reads `Document.Tags`.

Normalization is intentionally lenient (trim + lowercase + dedupe). Tags with
spaces are not rejected but are discouraged; the slug-like simple form is the
expectation. This is documented, not enforced.

### 2. Storage — column filter + GIN index

**No tag migration is needed** — the `tags TEXT[]` column already exists
(migration `0006`). New migration `0008_documents_tags_gin.sql`:

```sql
CREATE INDEX documents_tags_gin ON documents USING GIN (tags);
```

Cheap and correct for the `@>` containment filter; matches the store-level
filtering decision even at personal scale.

`ports.DocumentStore.List` becomes variadic so existing call sites compile
unchanged:

- `List(ctx, ownerID string, tags ...string) ([]Document, error)` — when
  `len(tags) > 0`, filter with `WHERE owner_id = $1 AND tags @> $2` (the
  full tag slice as the containment array → AND semantics). Empty → all.

`pgstore` builds the parameterized containment query; `FakeDocumentStore`
filters in memory with the same AND semantics. `CollectTags` needs no store
change — the tags use case lists unfiltered and aggregates in the domain.

### 3. Use cases

- `create_document`: parse frontmatter from `in.Body` and set
  `d.Tags = ParseFrontmatter(body)` before validate/persist. **Create gains tags
  for the first time.** `CreateDocumentInput` keeps `Body`; no `Tags` field.
- `update_document`: derive `cur.Tags` from `ParseFrontmatter(in.Body)`.
  **Remove** `UpdateDocumentInput.Tags`.
- `list_documents.go`: `Execute(ctx, owner string, tags []string)` →
  `store.List(owner, tags...)`.
- New `internal/usecase/list_tags.go` — `ListTags{Docs}.Execute(ctx, owner)`:
  `store.List(owner)` → `domain.CollectTags`.

Frontmatter parsing happens in the use case (the store stays a thin CRUD port),
mirroring M2b's "link extraction in the use case" decision. The stored body is
never rewritten — it keeps the frontmatter verbatim.

### 4. REST + SSE

- `GET /api/v1/documents?tag=go&tag=tui` — collect repeated `tag` query params →
  `ListDocuments(owner, tags)`; AND filter; `200 []Document`.
- `GET /api/v1/documents/tags` → `200 [{tag, count}]`; registered behind
  `s.auth` next to the other document routes.
- `POST` / `PUT` document: the request shape **drops** the `tags` field. Tags are
  body-derived. (Removes the FIX-4 preserve-tags path in the WebUI write
  handler.)
- **No new SSE event type.** Tag sets and filtered lists change only when a
  document is created/updated/deleted, which already emits
  `document.created|updated|deleted`. Both hosts re-fetch on those events.

### 5. apiclient

- `List(ctx, tags ...string) ([]Document, error)` — append a `tag` query param
  per tag.
- `Tags(ctx) ([]TagCount, error)` — call the new endpoint.

### 6. WebUI

- `handleWebDocList`: read repeated `?tag=` params → filtered list via
  `ListDocuments`. Render a **filter bar** of tag chips built from the
  `ListTags` use case directly (no HTTP self-call). Active tags are highlighted;
  a chip link toggles its tag in the query string; a "Filter zurücksetzen" link
  clears all. List rows show their tags as chips.
- Document view: render the document's tags as chips in the header area.
- `RenderMarkdown` and `RenderDocument` slice the body from `bodyStart`
  (via `ParseFrontmatter`) so the `---\ntags:…\n---` block never renders as an
  `<hr>`/heading/content. Plain callers are unaffected for bodies without
  frontmatter (`bodyStart == 0`).
- `docs.templ`: filter bar + tag-chip CSS (Tokyonight semantic colours, no
  emoji). Chip links are internal navigation (`hx-boost` fine).

### 7. TUI

`internal/tui/docs.go`:

- **Filter overlay:** key `f` in `modeList` opens a tag-filter overlay listing
  tags with counts (from `apiclient.Tags`, or `domain.CollectTags` over the
  loaded `m.docs`). `space` toggles a tag, `enter` applies the selection
  (reloads via `loadDocs(tags...)`), `esc` closes; an empty selection clears the
  filter. The active filter is shown in the list header/footer.
- **Tag display:** each list row shows its tags as a dim suffix; the view header
  shows the open document's tags.
- `renderView` skips the frontmatter block (start rendering from the parsed body
  offset) so `[[…]]`/weblink scanning and styling operate on the real body only.
- Footer updated to advertise the filter key.
- Exact keybinding choice and overlay layout are finalized against the
  `tui-usability` skill during planning (consistent with the existing
  `tab/enter/e/esc/q` grammar).

## Deliberate decisions / known limits

- **Single source of truth:** tags come only from frontmatter; the write API has
  no `tags` field. A programmatic client must emit a frontmatter block. This is
  the explicit trade for one wahrheitsort and removes the FIX-4 hack.
- **Body holds frontmatter verbatim:** the stored body is never rewritten, so
  tags survive round-trips losslessly; renderers are responsible for hiding the
  block. A body whose frontmatter is malformed renders the block literally
  (fail-visible) rather than silently eating content.
- **Filter is store-level AND:** OR / free-text tag search is out of scope (M2d
  search covers broader querying).
- **Lenient tag normalization:** trim + lowercase + dedupe only; no charset
  enforcement.
- **Non-atomic, like M2b:** tag derivation and link extraction both run in the
  use case after persist; a re-save heals any drift.

## Testing & gate

- **domain:** `ParseFrontmatter` table-driven — inline list, block list, no
  frontmatter, missing closing fence, unparseable YAML, normalization
  (trim/lowercase/dedupe/order), body-offset correctness, frontmatter without a
  `tags:` key. `CollectTags` — counts, sort order, empty set.
- **store:** `List` with tags filter (AND match, partial non-match, empty = all)
  against the fake and `pgstore`; migration `0008` applied.
- **usecase:** create & update derive tags from frontmatter (incl. tag removal
  on re-save); `list_tags` aggregation.
- **REST:** `?tag` AND filter; `/tags` endpoint shape; POST/PUT no longer
  accept/require `tags`.
- **WebUI:** frontmatter skipped in `RenderMarkdown`/`RenderDocument`; filter bar
  toggles tags in the query; chips render; bluemonday still strips XSS.
- **TUI:** filter overlay toggle/apply/clear; tags rendered on rows + header;
  frontmatter skipped in `renderView`.
- `make ci` green, coverage ≥ 80 % gate.

## Done-gate (live, like M2a/M2b)

Dev stack (Postgres + Dex), migration `0008` applied. curl-smoke the `?tag`
filter (AND) and the `/tags` endpoint. Browser dogfood: create a document with a
`---\ntags: [go, tui]\n---` block, see the tag chips, confirm the filter bar
narrows the list with AND semantics, and confirm the frontmatter block does not
render as content. TUI dogfood: `f` opens the filter, multi-tag AND narrows the
list, tags show on rows, frontmatter is hidden in the view. User confirms before
M2d.
