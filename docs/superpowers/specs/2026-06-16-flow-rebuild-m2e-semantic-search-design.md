# flow rebuild M2e — Semantic Search (Design Spec)

**Date:** 2026-06-16
**Milestone:** M2e — the fifth slice of the M2 Kompendium vertical (after M2a Document-Spine, M2b Wikilinks, M2c Tags/Filter, M2d Keyword-Search).
**Status:** approved, ready for implementation plan.

## Goal

Meaning-based document recall: find documents by *semantic similarity* even when they
share no literal words with the query (e.g. query "Strandferien" finds a doc titled
"Urlaub am Meer"). This is **complementary** to M2d: M2d's FTS+pg_trgm hybrid handles
exact tokens, fragments, and typos (character-based); M2e adds meaning (embedding-based).
The two are fused into a single ranked result on the **same `?q=` search box** — no new
host surface.

## Decisions (locked during brainstorming)

1. **Embeddings: local via Ollama** (`nomic-embed-text`, 768 dims). Privacy-preserving,
   self-hosted, no document content leaves the box. Behind a hexagonal `Embedder` port
   (Ollama impl + deterministic fake for tests).
2. **Combination: hybrid fusion**, one search box. Keyword arm (M2d) + vector arm run in
   parallel, fused with **Reciprocal Rank Fusion (RRF), k=60**.
3. **Embedding lifecycle: fully async (worker queue).** Writes only mark a document
   embedding-stale; a background worker computes embeddings. Save never depends on Ollama.
4. **Granularity: chunked** — one document → N chunks → N vectors in a `document_chunks`
   table. Search aggregates to the best chunk per document; the best chunk's text is the
   semantic snippet.
5. **Model: `nomic-embed-text` (768 dims)** → `vector(768)` column.
6. **Scope: flow-code only.** Provisioning Ollama in the homelab is **deferred companion
   work** in a separate `homelab-study` PR (see §9). flow degrades to keyword-only without
   Ollama, so prod is never broken.

## 1 · Data model (migration 0010)

```sql
-- +goose Up
CREATE EXTENSION IF NOT EXISTS vector;

ALTER TABLE documents ADD COLUMN chunks_hash text;  -- md5(title||body) that produced
                                                    -- the current chunks; NULL = never embedded

CREATE TABLE document_chunks (
    id          uuid PRIMARY KEY,
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    owner_id    text NOT NULL,                 -- denormalized for scope-filtered ANN
    chunk_index int  NOT NULL,
    content     text NOT NULL,                 -- chunk text = semantic snippet source
    embedding   vector(768) NOT NULL,
    UNIQUE (document_id, chunk_index)
);
CREATE INDEX document_chunks_doc   ON document_chunks (document_id);
CREATE INDEX document_chunks_owner ON document_chunks (owner_id);
CREATE INDEX document_chunks_hnsw  ON document_chunks USING hnsw (embedding vector_cosine_ops);

-- +goose Down
DROP TABLE IF EXISTS document_chunks;
ALTER TABLE documents DROP COLUMN IF EXISTS chunks_hash;
-- vector extension intentionally left installed (harmless, shared).
```

- **HNSW** over IVFFlat: better recall/latency, no training phase; negligible build/RAM
  cost at this volume.
- **Staleness via `chunks_hash`**: re-embed condition is
  `chunks_hash IS DISTINCT FROM md5(coalesce(title,'')||coalesce(body,''))`. A no-op update
  that doesn't change content does not trigger re-embedding. Backfill is uniform: every
  pre-M2e document starts with `chunks_hash = NULL` and is therefore stale.
- `ON DELETE CASCADE` keeps chunks in sync when a document is deleted.

## 2 · Embedder port

```go
// ports
type Embedder interface {
    // Embed returns one vector per input text, order-preserving. Batched.
    Embed(ctx context.Context, texts []string) ([][]float32, error)
}
```

- **Ollama adapter** (`internal/adapter/ollama` or `internal/adapter/embed`):
  `POST {FLOW_OLLAMA_HOST}/api/embed` with `{"model": <model>, "input": [texts...]}`,
  decodes `embeddings`. Config: `FLOW_OLLAMA_HOST` (default `http://localhost:11434`),
  `FLOW_EMBED_MODEL` (default `nomic-embed-text`). Returns a typed error when Ollama is
  unreachable so callers can degrade.
- **Fake embedder** (`testutil`): deterministic vector derived from a text hash (stable,
  L2-normalized) so similarity tests are reproducible and CI needs no Ollama. Same-text →
  same vector; near-text need NOT be near (tests assert wiring/fusion/ordering mechanics,
  not embedding quality — real quality is exercised by the live done-gate).

## 3 · Chunker

A pure, independently-tested function (domain or a small `internal/chunk` unit):

- Split `body` on blank-line / paragraph boundaries, pack into **~500-token (~2000-char)
  windows with ~15% overlap**. Token count approximated as `chars/4` to avoid a tokenizer
  dependency.
- **Title prepended to each chunk** for context (so a chunk deep in the body still carries
  the document's subject into its embedding).
- Returns `[]string` (chunk texts). Empty/whitespace body → a single chunk of just the
  title (so even bodyless docs are semantically findable by title).
- Deterministic; no I/O. Unit-tested for: boundary packing, overlap, title prefixing,
  long-paragraph splitting, empty body.

## 4 · Embedding worker

A background goroutine started in `cmd/flow-server/main.go`, wired with the store + the
Embedder, with graceful shutdown on context cancel.

- **Trigger:** a `time.Ticker` (`FLOW_EMBED_INTERVAL`, default 15s) as the backstop, **plus
  a buffered "kick" channel** that document create/update publishes so the worker wakes
  promptly instead of waiting up to a full interval. Coalesce kicks (non-blocking send).
- **Per cycle:** fetch a batch of stale documents
  (`chunks_hash IS DISTINCT FROM md5(...)`, `LIMIT batch`). For each: chunk (pure) →
  **batch-embed the chunks via the Embedder OUTSIDE any DB transaction** (never hold a DB
  tx open across the Ollama network call) → **then, in one short tx**: delete the doc's
  existing chunks → insert the new chunks+vectors → set `chunks_hash`. Backfill is automatic
  (all old docs are stale).
- **Resilience:** if the Embedder errors (Ollama down), log a WARN, leave the doc stale,
  move on; the next tick retries. **The app runs fully without Ollama** — semantic search
  is simply dormant and re-activates when Ollama returns and the backlog drains.
- Single worker, low concurrency (single-user, low volume). Startup kicks once for backfill.

## 5 · Query / fusion path

Fusion lives in the `SearchDocuments` use case (SQL stays in the store, fusion is pure Go
and fake-testable). The use case gains an `Embedder` and a new store port method.

```
SearchDocuments.Execute(ctx, ownerID, q, tags):
  keyword := Docs.Search(ctx, ownerID, q, tags)          // existing M2d arm (FTS+trgm), highlighted snippet
  vec, err := Embedder.Embed(ctx, [q])
  if err != nil:                                          // Ollama down → graceful degrade
      log.Warn(...); return keyword                       // keyword-only result, feature still works
  semantic := Docs.SemanticSearch(ctx, ownerID, vec[0], tags, K)
  return rrfFuse(keyword, semantic, k=60)                 // pure, tested
```

- **New store port method:**
  `SemanticSearch(ctx, ownerID string, query []float32, tags []string, limit int) ([]domain.SemanticHit, error)`
  where `SemanticHit = { Document domain.Document; Snippet string; Distance float32 }`.
  pgstore SQL: ANN over `document_chunks` (`embedding <=> $vec` cosine distance), **best
  chunk per document** (`DISTINCT ON (document_id)` ordered by distance, or
  `GROUP BY document_id` with `MIN(distance)` + a lateral to fetch that chunk's `content`),
  owner-scoped, **tag AND-filter via join to `documents.tags @> $tags`**, `ORDER BY distance
  LIMIT $limit`. Joins `documents` to return full Document rows; snippet = the best chunk's
  `content` (leading slice).
- **RRF fusion** (pure function, domain or usecase): each arm contributes a rank list; a
  document's fused score is `Σ 1/(k + rank_i)` over the arms it appears in (k=60). Sort by
  fused score desc. Each arm contributes ~50 candidates (`K`); return the fused top ~50.
- **Snippet selection:** a doc present in the keyword arm uses that arm's `<mark>`-highlighted
  `ts_headline` snippet; a *purely semantic* hit uses its best chunk's text **without
  highlight markers** (no token match exists to highlight — same accepted limitation as
  M2d's true-typo case). The `SearchHit` wire type is unchanged (flat Document + `snippet`).

## 6 · Hosts / UX

**Transparent.** Same `GET /api/v1/documents?q=`, same WebUI search box, same TUI `/` mode.
No new endpoint, key, or toggle. Results simply get semantically richer. Snippet rendering is
unchanged (keyword hits highlighted, semantic hits show plain chunk text). A "why it matched"
indicator (semantic vs keyword) is deliberately **out of scope (YAGNI)** for v1.

## 7 · Config

| Env | Default | Purpose |
|-----|---------|---------|
| `FLOW_OLLAMA_HOST` | `http://localhost:11434` | Ollama base URL |
| `FLOW_EMBED_MODEL` | `nomic-embed-text` | embedding model |
| `FLOW_EMBED_INTERVAL` | `15s` | worker backstop tick |
| `FLOW_EMBED_BATCH` | `16` | stale-docs per cycle |

## 8 · Dev / deploy

- **dev:** add an Ollama service to `deploy/dev/compose.yml`; `scripts/dev-up.sh` waits for
  it and runs `ollama pull nomic-embed-text`. CPU embedding is fine at this volume. Add the
  env defaults to `deploy/dev/flow.env`.
- **prod: out of scope for M2e.** flow degrades to keyword-only without Ollama, so prod is
  never broken by shipping this code. See §9.

## 9 · Deferred companion work (NOT in this milestone)

Provisioning Ollama in the homelab is required before **prod** semantic search works, and is
tracked as a **separate `homelab-study` PR** with its own spec, following that repo's GitOps
conventions (render-then-commit, `mise exec` for makejinja, secrets mirror): an Ollama
Deployment + Service + a PVC for model storage + a model-pull (init container or Job running
`ollama pull nomic-embed-text`); GPU-vs-CPU node placement to be confirmed there. Once it
exists, flow-server is pointed at it via `FLOW_OLLAMA_HOST` and the worker backfills. This
spec records the dependency; it does not build it.

## 10 · Testing & gate

- **chunk:** pure unit tests (boundary packing, overlap, title prefix, long-paragraph split,
  empty body).
- **rrf:** pure unit tests (rank fusion math, doc appearing in one vs both arms, ordering,
  empty arm).
- **embedder fake:** deterministic, reproducible.
- **store (DB-gated, pgvector image already present):** `SemanticSearch` test — insert chunks
  with known vectors, query, assert distance ordering, best-chunk-per-doc, owner scope, and
  tag AND-composition.
- **worker:** with fake embedder — a stale doc gets chunks + `chunks_hash` set; an
  Embedder error leaves it stale (no partial write); a content change re-chunks; delete
  cascades chunks.
- **usecase:** fusion with a fake store (both arms) → RRF order; **semantic-arm error →
  keyword-only** (degradation path); tags thread into both arms.
- **REST / apiclient / WebUI / TUI:** largely unchanged (same `?q=`); verify fused results
  flow end-to-end and that degradation (no Ollama) still returns keyword results. Wire the
  new `Embedder` + worker into `main.go` (composition-root verification task).
- `make ci` green, coverage ≥ 80% gate.

## 11 · Done-gate (live, like M2a–d)

Dev stack incl. Ollama (`nomic-embed-text` pulled), migration 0010 applied (vector ext +
`document_chunks` + HNSW). Create semantically-related docs that share **no keywords** (e.g.
one "Urlaub am Meer", query "Strandferien"); after the worker embeds, `?q=Strandferien` finds
it via the semantic arm and fuses sensibly with keyword hits. Verify: (a) pure-semantic hit
carries a chunk snippet; (b) keyword hits still highlight; (c) tag filter composes with
semantic results; (d) **degradation** — stop Ollama, confirm `?q=` still returns keyword-only
results without error. Browser + TUI dogfood optional (Soenne may waive per M2c/M2d
precedent). Update the M2e memory note with done + commit range; report; await confirmation.

## File map (orientation — finalized in the plan)

| File | Responsibility | Action |
|------|----------------|--------|
| `internal/adapter/pgstore/migrations/0010_document_chunks.sql` | vector ext + chunks table + HNSW + chunks_hash | Create |
| `internal/domain/search.go` | `SemanticHit` type | Modify |
| `internal/chunk/chunk.go` (+ test) | pure chunker | Create |
| `internal/usecase/rrf.go` (+ test) | pure RRF fusion | Create |
| `internal/ports/ports.go` | `Embedder` + `DocumentStore.SemanticSearch` + stale-docs/chunk-write methods | Modify |
| `internal/adapter/embed/ollama.go` (+ test) | Ollama Embedder adapter | Create |
| `internal/testutil/fakes.go` | fake Embedder + fake SemanticSearch + chunk-write fakes | Modify |
| `internal/adapter/pgstore/documents.go` (+ test) | `SemanticSearch`, stale-docs query, chunk upsert/delete | Modify |
| `internal/worker/embed_worker.go` (+ test) | background embed worker (ticker + kick) | Create |
| `internal/usecase/search_documents.go` (+ test) | hybrid fusion + degrade | Modify |
| `internal/usecase/create_document.go` / `update_document.go` | publish kick on write | Modify |
| `cmd/flow-server/main.go` | wire Embedder + worker; start/stop | Modify |
| `deploy/dev/compose.yml`, `scripts/dev-up.sh`, `deploy/dev/flow.env` | dev Ollama + model pull + env | Modify |

Reminder ([[feedback_pgstore_goose_migrations]]): migration 0010 needs goose
`-- +goose Up`/`-- +goose Down` annotations. Hosts (REST/WebUI/TUI) need little to no change
because M2e rides the existing `?q=` surface from M2d.
