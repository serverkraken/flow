# flow rebuild M2d — Search (Design)

**Date:** 2026-06-16
**Branch:** `rebuild` (long-lived orphan, not merged)
**Predecessor:** M2c Tags & Filter (`68ef70f..f0e6879`)
**Status:** Design approved — ready for implementation plan.

## Goal

Free-text search across the owner's documents, combinable with the M2c tag
filter, ranked by relevance, with a highlighted snippet per hit. Available in
**both hosts** (WebUI + TUI) plus the apiclient, following the
generic-in-every-host principle.

This is the fourth slice of the M2 Kompendium vertical. Semantic (meaning-based)
search via pgvector is **M2e** and out of scope here — but the recall design
below is deliberately complementary to it (see "Recall strategy").

## Core decisions (locked during brainstorming)

1. **Engine:** Postgres full-text search (`tsvector`) over `title`+`body`, title
   weighted higher (`A` vs `B`).
2. **Config:** `simple` (lowercasing + tokenization, **no stemming/stopwords**)
   — predictable for Soenne's mixed DE/EN corpus; avoids wrong cross-language
   stemming.
3. **Recall strategy — REQUIREMENT, not optional:** **FTS + `pg_trgm` trigram**
   hybrid. Exact-token-only search is unacceptable to the user ("kompend" MUST
   find "Kompendium"; cf. [[feedback_search_partial_word_recall]]). FTS provides
   ranked exact/phrase/operator matching; pg_trgm `word_similarity` adds
   **partial-word/fragment + typo** recall. This is **complementary to M2e**
   (pgvector = semantic/meaning; trigram = character-based fragments/typos).
4. **Composition:** `?q=` lives on the existing list endpoint and combines with
   `?tag=` (AND — search *within* the tag-filtered set). Empty `q` = current
   list/tag behavior (backward compatible).
5. **Presentation:** results ranked by relevance with a **highlighted snippet**
   (`ts_headline`); fuzzy-only hits show a plain (unhighlighted) lead excerpt.

## Architecture

Vertical slice mirroring M2a–c: **domain → store/migration → usecase →
REST(+no new SSE) → apiclient → WebUI → TUI**.

### 1. Storage — generated tsvector column + trigram index (auto-maintained)

Migration `0009_documents_search.sql`. Both indexes are **Postgres-maintained**
— no app-side write-path code and no SSE concerns (unlike M2c's frontmatter
parse).

```sql
-- +goose Up
CREATE EXTENSION IF NOT EXISTS pg_trgm;

ALTER TABLE documents ADD COLUMN search tsvector GENERATED ALWAYS AS (
    setweight(to_tsvector('simple', coalesce(title,'')), 'A') ||
    setweight(to_tsvector('simple', coalesce(body,'')),  'B')
) STORED;
CREATE INDEX documents_search_gin ON documents USING GIN (search);

-- trigram recall over title+body (immutable expression index)
CREATE INDEX documents_trgm_title ON documents USING GIN (title gin_trgm_ops);
CREATE INDEX documents_trgm_body  ON documents USING GIN (body  gin_trgm_ops);

-- +goose Down
DROP INDEX IF EXISTS documents_trgm_body;
DROP INDEX IF EXISTS documents_trgm_title;
DROP INDEX IF EXISTS documents_search_gin;
ALTER TABLE documents DROP COLUMN IF EXISTS search;
-- pg_trgm extension left installed (harmless; other features may use it)
```

`ports.DocumentStore` gains:

```go
Search(ctx context.Context, ownerID, q string, tags []string) ([]domain.SearchHit, error)
```

**pgstore** query (one statement, hybrid OR with combined ranking):

```sql
SELECT <docCols>,
  ts_rank(d.search, ftsq)                                      AS frank,
  GREATEST(word_similarity($2, d.title),
           word_similarity($2, d.body))                        AS sim,
  ts_headline('simple', coalesce(d.title,'')||' '||coalesce(d.body,''),
              ftsq, 'StartSel=\x02,StopSel=\x03,MaxFragments=1,MinWords=5,MaxWords=18')
                                                               AS snippet
FROM documents d, websearch_to_tsquery('simple', $2) ftsq
WHERE d.owner_id = $1
  [AND d.tags @> $3]                       -- only when tags given
  AND ( d.search @@ ftsq                    -- FTS arm
        OR $2 <% (d.title||' '||d.body) )   -- pg_trgm word_similarity arm
ORDER BY (d.search @@ ftsq) DESC,           -- exact FTS hits first
         frank DESC, sim DESC, d.updated_at DESC
LIMIT 100;
```

- `<%` is the pg_trgm `word_similarity` operator (asymmetric: "is `$2` similar to
  part of the text"), governed by `pg_trgm.word_similarity_threshold` (default
  0.6). The threshold can be tuned at session level (`SET LOCAL`) if needed;
  v1 uses the default.
- The trigram arm catches fragments ("kompend") and typos ("kompendiun"); the
  FTS arm provides ranked exact/phrase/operator matching and the snippet.
- Snippet: `ts_headline` highlights FTS lexeme matches with sentinel control
  chars (`\x02`/`\x03`). The options string (`StartSel=…,StopSel=…,…`) carrying
  those control chars is **built in Go and bound as a query parameter** (not a
  SQL literal) to avoid escape-string pitfalls. For trigram-only hits (no FTS
  lexeme) `ts_headline` returns an unhighlighted lead fragment — acceptable.

**FakeDocumentStore.Search** (tests): in-memory — lowercase substring match of
`q` against title+body (which covers both the exact and fragment cases at test
scale), producing a naive snippet with the same sentinel markers around the
matched substring. Good enough to exercise the use-case/REST/host wiring without
Postgres; the real FTS/trigram behavior is covered by a DB-gated pgstore test.

### 2. Domain — `SearchHit` + shared highlight markers

`internal/domain/search.go` (pure):

```go
// Highlight sentinels emitted by ts_headline (StartSel/StopSel) and replaced
// by each host: WebUI → <mark>…</mark>, TUI → lipgloss highlight on/off.
const (
    HighlightStart = "\x02"
    HighlightEnd   = "\x03"
)

// SearchHit is a document plus its search snippet. Embedding Document keeps the
// JSON flat (Document fields + "snippet") so plain []Document decoders still work.
type SearchHit struct {
    Document        // anonymous embed of domain.Document → flat JSON
    Snippet  string `json:"snippet"`
}
```

(No query parsing in domain — `websearch_to_tsquery`/`word_similarity` own the
syntax in SQL. The sentinels are the only shared contract between hosts.)

### 3. Use case + REST

- New `internal/usecase/search_documents.go` —
  `SearchDocuments{Docs}.Execute(ctx, owner, q string, tags []string) ([]domain.SearchHit, error)`
  → `store.Search(owner, q, tags)`. Single-purpose; the q-less path stays in the
  existing `ListDocuments`.
- `handleListDocuments` branches on `q`:
  - `q` (trimmed) non-empty → `SearchDocuments` → `200 []SearchHit`;
  - else → existing `ListDocuments` → `200 []Document`.
  One route (`GET /api/v1/documents`), `?q=` + `?tag=` both parsed from the
  query. **No new SSE event type** — hits change only on
  `document.created|updated|deleted` (already emitted); hosts re-query on those.

### 4. apiclient

`Search(ctx, q string, tags ...string) ([]domain.SearchHit, error)` — calls
`GET /api/v1/documents?q=…[&tag=…]`, decodes the snippet. `ListDocuments`
(q-less) is unchanged.

### 5. WebUI

- A **search input** in the `/docs` filter area (a GET form → `/docs?q=…` that
  carries the active `tag` params as hidden inputs, so search composes with the
  filter). `handleWebDocsList`/`docsListData`: when `q` present, call
  `SearchDocuments` and render hits with the snippet; else the current list.
- Snippet rendering: HTML-escape the snippet, **then** replace `\x02`/`\x03`
  with `<mark>`/`</mark>` (escape-first → no injection). Extend the page CSS for
  `mark` (Tokyonight-ish, no emoji).
- `Query` (threaded into the `#dc` SSE-refresh `hx-get`) is extended to include
  `q` alongside the tags, so live refresh preserves the active search.

### 6. TUI

`internal/tui/docs.go`:

- Key `/` in `modeList` opens a **search input** (a `bubbles/textinput`,
  tui-local like the editor); `enter` runs `apiclient.Search(q, m.filterTags...)`
  → a `modeSearch` results view; `esc` clears and returns to the list.
- Results render title/path + a **snippet line** with the matched span
  highlighted (replace `\x02`/`\x03` with a lipgloss highlight style on/off).
- `enter` on a result opens the doc (reuses `loadDoc`); coexists with the `f`
  tag-filter overlay (search respects the active filter by passing
  `m.filterTags`).
- Footer + glyph choices finalized against `tui-usability` in the plan
  (consistent with the existing `j/k/enter/n/e/d/f/q` grammar; `/` is the
  conventional search key).

## Deliberate decisions / known limits

- **Frontmatter in the index:** the tsvector + trigram indexes cover the body
  verbatim, including the `---tags:…---` block (minor token pollution). Tags are
  separately filterable; accepted in exchange for zero-maintenance generated
  indexes.
- **Trigram threshold is fixed** at the pg_trgm default (0.6 word_similarity)
  for v1; tunable later via `SET LOCAL` if recall feels off.
- **Snippet for fuzzy-only hits is unhighlighted** (no FTS lexeme to mark);
  shows a lead excerpt.
- **Very short queries (1–2 chars)** have no trigrams → only the FTS arm applies
  (exact short token). Edge case, acceptable.
- **Ranking is heuristic** (exact-FTS-first, then ts_rank, then similarity,
  then recency) — good enough for a personal corpus; not tuned beyond that.

## Testing & gate

- **domain:** `SearchHit` JSON is flat (embed) and decodes into both `SearchHit`
  and plain `Document`; highlight-sentinel constants.
- **store:** `FakeDocumentStore.Search` (substring + snippet markers, AND with
  tags). DB-gated `pgstore` test against real Postgres: migration `0009`
  applied; "kompend" finds a "Kompendium" doc (trigram), an exact phrase ranks
  above a fuzzy hit, tag+q composes, snippet carries `\x02/\x03` around the
  match.
- **usecase:** `SearchDocuments` delegates with tags; empty result.
- **REST:** `?q=` returns `[]SearchHit` with snippet; `?q=&tag=` composes; no `q`
  still returns `[]Document` (shape unchanged); the SearchHit JSON is a superset
  of Document.
- **apiclient:** `Search` builds `?q=…&tag=…` and decodes the snippet.
- **WebUI:** snippet escape-then-`<mark>` (XSS-safe: a `q`/body with `<script>`
  stays escaped); search form preserves active tags; empty q → list.
- **TUI:** `/` opens input, enter runs search, snippet highlight on/off from
  sentinels, esc returns; search passes the active tag filter.
- `make ci` green, coverage ≥ 80 % gate.

## Done-gate (live, like M2a–c)

Dev stack (Postgres + Dex), migration `0009` applied (`pg_trgm` + generated
column). curl-smoke: create docs incl. one titled/bodied "Kompendium";
`?q=kompend` finds it (trigram), `?q="exact phrase"` ranks + snippets correctly,
`?q=foo&tag=bar` composes, response carries `snippet`. Browser dogfood: search
box returns highlighted snippets, composes with the tag filter, frontmatter not
rendered. TUI dogfood: `/` search, highlighted snippet line, `enter` opens,
respects an active `f` filter. User confirms before M2e.
