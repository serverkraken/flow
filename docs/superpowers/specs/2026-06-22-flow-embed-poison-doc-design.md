# flow Embed Poison-Doc Fix (Säule B) — Design

Date: 2026-06-22 · Branch: `rebuild` · Status: approved for planning

## 1. Context

The **embed worker** (`internal/worker/embed_worker.go`) keeps document embeddings up to
date asynchronously so the write path never blocks on Ollama. It runs inside **flow-server**
(the long-running PROD deployment on `flow.thebackend.org`, the same `:rebuild` image that
Säule A targets), not in flow-mcp. Its loop:

1. `StaleDocuments(batch)` — docs whose chunks are out of date
   (`chunks_hash IS DISTINCT FROM md5(title||body)`), ordered `updated_at ASC`.
2. For each: `chunk.Split(title, body)` → `Embedder.Embed(texts)` (Ollama `/api/embed`) →
   `ReplaceChunks(...)`, which stamps `chunks_hash` **only on success**.
3. Woken by a periodic tick (`FLOW_EMBED_INTERVAL`, default 15s) and by a coalesced kick
   (`DocChangeNotifier.DocumentChanged`) fired after every create/update/import.

`Embedder` is `embed.Ollama` (`internal/adapter/embed/ollama.go`), a thin client over a
local Ollama server, model `nomic-embed-text` (768-dim), 60s HTTP timeout. Deploy path is
shared with Säule A: CI builds `:rebuild` → bump the digest in homelab-study → ArgoCD sync.

This is **Säule B**, deferred from the Säule A spec (durable auth) §8.

### 1.1 The problem — poison-doc retry-storm

A document whose embedding fails never gets its `chunks_hash` stamped, so it stays "stale"
forever. Two design facts turn that into a corpus-wide stall:

- `StaleDocuments` orders `updated_at ASC`, so the oldest failing doc sits at the **head**.
- `drain` **returns on the first `embedOne` error** (intended for "Ollama is down — don't
  spin"), so it never advances past the failing doc.

Result: one failing doc at the head blocks **every newer doc behind it forever**, and the
worker re-attempts it on every tick *and* every write-kick — hammering Ollama and spamming
logs. That is the retry-storm.

### 1.2 Will a failed call ever succeed? — two failure classes

The honest answer is "it depends on the cause", and both classes exist:

**A. Transient / environmental** — Ollama container restart, model not yet pulled,
momentary load/OOM, timeout, network. The retried call is identical but the *environment*
changed, so it **does eventually go through**. Retry is the correct cure.

**B. Deterministic per-input** — same doc content → byte-identical request → identical
failure **every time**. Retry is futile; it **never** goes through until the *code* or the
*doc content* changes. In the current code there are two real deterministic modes, and both
are **our own bugs, not unembeddable content** (embedding models embed anything — they do
not reject content semantically):

- **(a) Oversized batch → 60s timeout, every time.** `Ollama.Embed` sends *all* of a doc's
  chunks as **one** `/api/embed` call under a fixed 60s client timeout. A large import
  (hundreds of chunks) can exceed 60s on every attempt. Worse: a timeout *looks* transient,
  so the safety net (§3.2) would `return` and re-issue the same giant call next tick — a
  permanent head-of-line block that never dead-letters. Reachable: `Document.Validate()`
  imposes no body-size cap.
- **(b) Empty/whitespace chunk + empty title → vector-count mismatch, every time.**
  Verified empirically: `chunk.Split("", "A"+strings.Repeat(" ",4000)+"Z")` returns
  `["A", "", "Z"]` — an empty-string chunk. With a non-empty title the empty window becomes
  `"Title\n\n"` (harmless), but with an **empty title** (allowed — `Validate()` does not
  require one) the chunk is `""`. Ollama drops/zero-handles the empty input and returns
  fewer vectors than inputs → `got N vectors for M texts` → fails identically forever.

### 1.3 Goal / non-goals

**Goal:** (1) make the embed calls *actually succeed* by removing the self-inflicted
deterministic failures, so docs that previously could never embed now do; and (2) make the
worker resilient — a genuine failure (Ollama down) or any unforeseen deterministic failure
must not block the queue or storm the backend, must be visible, and must be recoverable.

**Non-goals:** paragraph-aware chunking (character windows stay); a background keep-alive
goroutine; changing the embedding model or vector dimension; live SSE push of "now
embedded" to the WebUI (status refreshes on load/retry); CLI/TUI surfaces (WebUI only).

## 2. Resolved decisions

1. **Two layers.** Layer 1 eliminates the poison sources (root-cause fix); Layer 2 is the
   safety net. Both are in scope.
2. **Failure state → persisted in Postgres** (side table), so a poison doc never re-storms
   after a restart/redeploy.
3. **Classify errors** — transient/global vs per-doc. Transient never penalizes a doc.
4. **Give-up policy → capped exponential backoff for the first N attempts, then
   dead-letter** (excluded from the queue), surfaced in the WebUI with a **manual Retry**.
5. **Surfaces → WebUI only** (status badge + Retry button on the doc view).
6. **Defaults:** `maxInputsPerCall = 64`, `maxAttempts = 5`, backoff `base = 1m`,
   `cap = 6h`.

## 3. Architecture

### Layer 1 — eliminate the poison sources (calls actually succeed)

#### 3.1 `chunk.Split` — never emit an empty/whitespace chunk

Decide emptiness on the **trimmed window content**, before prepending the title; skip empty
windows entirely (so a whitespace gap produces neither a `""` chunk nor a degenerate
title-only duplicate). Invariants after the change:

- a non-empty (trimmed) body yields **≥1 non-empty chunk**;
- `Split` **never returns an empty string**;
- empty title + body still yields `nil` (unchanged); empty body + non-empty title still
  yields `[title]` (unchanged).

Eliminates failure (b). File: `internal/chunk/chunk.go` (+ `chunk_test.go` cases).

#### 3.2 `Ollama.Embed` — sub-batch inputs per HTTP call

Introduce `const maxInputsPerCall = 64`. `Embed` loops over `texts` in slices of at most
`maxInputsPerCall`, issues one `/api/embed` per slice (each under the existing 60s timeout),
concatenates the results in order, and asserts the **total** count equals `len(texts)`. The
current single-call body (marshal → POST → decode → per-call count check → classification)
becomes an unexported `embedOnce(ctx, slice)`.

- No single call grows unbounded → eliminates the deterministic timeout (a) and the
  "timeout masquerading as transient → permanent block" hole.
- The `ports.Embedder` contract is **unchanged** (`Embed(texts) → vectors for all texts`);
  the worker stays unaware of batching.

File: `internal/adapter/embed/ollama.go` (+ `ollama_test.go`: a loopback `httptest` server
asserting the expected number of calls and that concatenation preserves order).

### Layer 2 — safety net (storm/block management)

#### 3.3 Data model — side table (migration `0013`)

```sql
-- 0013_document_embed_failures.sql (goose-annotated Up/Down)
CREATE TABLE document_embed_failures (
    document_id   text PRIMARY KEY REFERENCES documents(id) ON DELETE CASCADE,
    owner_id      text        NOT NULL,
    attempts      int         NOT NULL,
    next_retry_at timestamptz NOT NULL,
    last_error    text        NOT NULL DEFAULT '',
    dead          boolean     NOT NULL DEFAULT false
);
```

A row exists **only** for docs that have failed ≥1 time, so healthy docs stay zero-overhead
and `domain.Document` stays free of embed bookkeeping. `owner_id` is carried for direct
owner-scoped cleanup. (Goose annotations are mandatory — bare SQL fails at apply.)

#### 3.4 `ports` additions

```go
// classification seam — the adapter wraps transient/global causes with this; the worker
// (which imports ports, not the adapter) classifies via errors.Is.
var ErrEmbedTransient = errors.New("embed backend transient failure")

// StaleDocuments now carries the prior consecutive-failure count so the worker owns the
// backoff policy in Go.
type StaleDoc struct {
    Doc      domain.Document
    Attempts int // 0 if never failed
}

type DocumentStore interface {
    // ... existing ...
    StaleDocuments(ctx context.Context, limit int) ([]StaleDoc, error) // signature change
    ReplaceChunks(ctx context.Context, docID, ownerID string, contents []string, embeddings [][]float32) error // now also clears the failure row in-tx
    RecordEmbedFailure(ctx context.Context, docID, ownerID string, attempts int, nextRetryAt time.Time, dead bool, lastErr string) error
    ClearEmbedFailure(ctx context.Context, docID, ownerID string) error
    EmbedStatus(ctx context.Context, ownerID, docID string) (domain.EmbedStatus, error)
}
```

#### 3.5 `domain.EmbedStatus` (read model)

```go
type EmbedState string
const (
    EmbedOK       EmbedState = "ok"
    EmbedPending  EmbedState = "pending"
    EmbedRetrying EmbedState = "retrying"
    EmbedFailed   EmbedState = "failed"
)
type EmbedStatus struct {
    State     EmbedState
    Attempts  int
    LastError string
    NextRetry *time.Time
}
```

Derivation (single query, §3.8): `failed` if `dead`; else `retrying` if a failure row
exists; else `pending` if the doc is stale (hash mismatch); else `ok`.

#### 3.6 Worker — drain refactor, classification, backoff

`drain` no longer returns on the first per-doc error. Per doc:

```
texts := chunk.Split(d.Title, d.Body)
if len(texts) == 0:
    if err := ReplaceChunks(d.ID, d.OwnerID, nil, nil); err != nil { log; return }   // clears row too
    continue
vecs, err := Embedder.Embed(ctx, texts)
if err != nil:
    if errors.Is(err, ports.ErrEmbedTransient):
        log; return                         // Ollama down: stop, retry next tick, NO penalty
    attempts := sd.Attempts + 1             // per-doc deterministic
    dead := attempts >= maxAttempts
    next  := clock().Add(backoff(attempts)) // value unused when dead
    if err := RecordEmbedFailure(d.ID, d.OwnerID, attempts, next, dead, err.Error()); err != nil { log; return }
    continue                                // do NOT block the queue
if err := ReplaceChunks(d.ID, d.OwnerID, texts, vecs); err != nil { log; return }   // success: stamp + clear row
continue
```

- **Store errors** (`StaleDocuments` / `ReplaceChunks` / `RecordEmbedFailure`) always stop
  the drain (`return`) and retry next tick — they are environmental, never recorded as a
  per-doc embed failure. The success path's `ReplaceChunks` is *not* misread as a failure.
- **Termination invariant:** each processed doc leaves the eligible set for the next
  `StaleDocuments` (success → not stale; per-doc → future `next_retry_at` or `dead`), or the
  drain returns (transient/store error). The eligible set strictly shrinks → no infinite
  loop, no head-of-line block.

`backoff` (new `internal/worker/backoff.go`, pure + table-tested):

```go
func backoff(attempts int) time.Duration { // attempts >= 1
    d := base << (attempts - 1)             // 1m,2m,4m,8m,...
    if d <= 0 || d > cap { d = cap }        // d<=0 guards shift overflow
    return d
}
```

Policy is worker-owned, injectable for tests: `NewEmbedWorker` gains an `EmbedPolicy{
MaxAttempts, BackoffBase, BackoffCap }` param (zero fields → defaults of §2.6); an
unexported `clock func() time.Time` field defaults to `time.Now` and is overridden in
white-box tests.

#### 3.7 Error classification in `ollama.go`

Inside `embedOnce`, wrap **transient/global** causes with `ports.ErrEmbedTransient`:

- any `o.client.Do` error — dial failures, connection refused, resets, **and timeouts**
  (all environmental);
- HTTP **503**, **429**, and **404 / model-not-found** (a missing model fails *every* doc;
  classifying it per-doc would silently dead-letter the whole corpus after N attempts).

Everything else stays a plain (per-doc) error: other `4xx`, a `500` on one doc, decode
failure, per-call vector-count mismatch. Wrapping uses `fmt.Errorf("ollama embed: %w: %w",
ports.ErrEmbedTransient, cause)` (or `%w: status %d: %s`). With Layer 1 the count-mismatch
and timeout per-doc paths are practically unreachable; they remain as guards.

`testutil` fake embedder gains a way to return a transient (`ErrEmbedTransient`-wrapped) vs
a per-doc error for worker tests.

#### 3.8 pgstore queries (`documents_embed.go`, new file)

```sql
-- StaleDocuments (excludes dead + not-due; carries attempts)
SELECT <prefixedDocCols>, coalesce(f.attempts,0)
FROM documents d
LEFT JOIN document_embed_failures f ON f.document_id = d.id
WHERE d.chunks_hash IS DISTINCT FROM md5(coalesce(d.title,'')||coalesce(d.body,''))
  AND coalesce(f.dead,false) = false
  AND (f.next_retry_at IS NULL OR f.next_retry_at <= now())
ORDER BY d.updated_at ASC
LIMIT $1;

-- RecordEmbedFailure (upsert)
INSERT INTO document_embed_failures (document_id, owner_id, attempts, next_retry_at, last_error, dead)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (document_id) DO UPDATE
  SET attempts=$3, next_retry_at=$4, last_error=$5, dead=$6;

-- ClearEmbedFailure (manual retry / belt-and-suspenders; success path uses the in-tx delete)
DELETE FROM document_embed_failures WHERE document_id=$1 AND owner_id=$2;

-- EmbedStatus
SELECT
  CASE
    WHEN f.dead THEN 'failed'
    WHEN f.document_id IS NOT NULL THEN 'retrying'
    WHEN d.chunks_hash IS DISTINCT FROM md5(coalesce(d.title,'')||coalesce(d.body,'')) THEN 'pending'
    ELSE 'ok'
  END,
  coalesce(f.attempts,0), coalesce(f.last_error,''), f.next_retry_at
FROM documents d
LEFT JOIN document_embed_failures f ON f.document_id = d.id
WHERE d.id=$1 AND d.owner_id=$2;
```

`ReplaceChunks` adds, inside its existing transaction, `DELETE FROM
document_embed_failures WHERE document_id=$1` so a success (or an empty-text clear)
atomically removes any failure row.

#### 3.9 WebUI (`/docs`, WebUI-only)

- **Status** on the doc view: `handleWebDocView` fetches `EmbedStatus` and passes it to
  `docs.templ`, which renders a small badge — `ok` / `pending` / `retrying — <last error>`
  / `failed — <last error>`.
- **Retry**: new route `POST /docs/{id}/reembed` → `handleWebDocReembed` → usecase
  `RetryEmbedding` → `ClearEmbedFailure` + `DocumentChanged()` kick. The doc is still
  hash-stale, so it re-queues immediately. HTMX swaps the status fragment to `pending`;
  non-HTMX redirects back to `/docs/{id}`. Owner-scoped like the other `/docs` handlers; route
  registered in `server.go` and added to `routes_test.go`.

```go
type RetryEmbedding struct {
    Docs     ports.DocumentStore
    Notifier ports.DocChangeNotifier // = embedWorker
}
func (uc RetryEmbedding) Exec(ctx context.Context, ownerID, docID string) error {
    if _, err := uc.Docs.Get(ctx, ownerID, docID); err != nil { return err } // 404 / authz
    if err := uc.Docs.ClearEmbedFailure(ctx, docID, ownerID); err != nil { return err }
    if uc.Notifier != nil { uc.Notifier.DocumentChanged() }
    return nil
}
```

#### 3.10 Wiring (`cmd/flow-server/main.go`) — explicit, per the plan-wiring rule

- Pass `worker.EmbedPolicy{}` (defaults) into `NewEmbedWorker` (the `maxInputsPerCall` cap
  lives as a const in the embed adapter — not surfaced through main).
- Construct `usecase.RetryEmbedding{Docs: documentStore, Notifier: embedWorker}` and add it
  to the handlers struct; mount `POST /docs/{id}/reembed`.

## 4. File layout (Keine Monolithen)

```
internal/chunk/chunk.go                                # L1.1 empty-chunk filter (+ _test)
internal/adapter/embed/ollama.go                       # L1.2 sub-batching + §3.7 classification (+ _test)
internal/adapter/pgstore/migrations/0013_document_embed_failures.sql  # NEW (goose Up/Down)
internal/adapter/pgstore/documents_embed.go            # NEW: Record/Clear/EmbedStatus + StaleDocuments join (+ _test)
internal/adapter/pgstore/documents.go                  # ReplaceChunks clears failure row in-tx; StaleDocuments signature
internal/ports/ports.go                                # ErrEmbedTransient, StaleDoc, new methods, EmbedStatus port shape
internal/domain/embed_status.go                        # NEW: EmbedState/EmbedStatus value object (+ _test)
internal/worker/embed_worker.go                        # drain refactor + EmbedPolicy + clock
internal/worker/backoff.go                             # NEW pure backoff (+ _test)
internal/usecase/retry_embedding.go                    # NEW RetryEmbedding usecase (+ _test)
internal/adapter/httpserver/webui_docs.go              # handleWebDocReembed + status in view
internal/adapter/httpserver/server.go                  # mount POST /docs/{id}/reembed
internal/adapter/webui/docs.templ                      # status badge + Retry button
internal/testutil/fakes.go                             # fake embedder transient/per-doc; fake store methods
cmd/flow-server/main.go                                # EmbedPolicy + RetryEmbedding + route wiring
```

## 5. Testing

- **chunk:** whitespace-gap body with empty title → no empty chunk and ≥1 non-empty chunk;
  with a title → no degenerate title-only duplicate; existing cases stay green.
- **ollama (`httptest` loopback):** N inputs → `ceil(N/64)` calls; concatenated vectors keep
  input order; a 503/429/404/`client.Do` error is `errors.Is(err, ports.ErrEmbedTransient)`;
  a 400/`500`/count-mismatch is **not**.
- **worker (white-box, fake store + fake embedder):** a poison doc ahead of a healthy doc →
  the healthy doc still embeds (no head-of-line block); transient error → drain stops and
  **no** attempt bump; per-doc error → attempt bump + future `next_retry_at` + continue;
  `attempts == maxAttempts` → `dead` and excluded next pass; store error → drain stops;
  `backoff()` table test.
- **pgstore (Docker):** `StaleDocuments` excludes `dead` + not-due and returns `attempts`;
  `ReplaceChunks` success clears the failure row in-tx; `RecordEmbedFailure` upsert;
  `EmbedStatus` derives each of the four states; `ClearEmbedFailure` restores eligibility.
- **httpserver:** `POST /docs/{id}/reembed` exists, owner-scoped, kicks the notifier; the doc
  view renders each status.
- `make ci` green at/above the current coverage gate (lint = `golangci-lint run`; ST1005:
  error strings lowercase, no trailing punctuation).

## 6. Done-gate

1. `make ci` green; `go build ./...` OK.
2. **Main-wiring verification** (per `feedback_plan_main_wiring_task`): the worker receives
   the policy; `usecase.RetryEmbedding` is constructed and reachable; `POST /docs/{id}/reembed`
   is mounted and in `routes_test.go`.
3. **Live — Layer 1 (the point: calls now go through):** create a previously-poison-shaped
   doc — a large import (hundreds of chunks) and a doc with an empty title + a ≥2000-rune
   whitespace gap — and confirm both **embed successfully** (semantic search finds them); no
   timeout, no count-mismatch.
4. **Live — Layer 2 (resilience):** stop Ollama, create a doc → worker logs no storm and
   does not block; create a *second* doc; restart Ollama → both embed on the next tick. The
   per-doc backoff → dead-letter transition is hard to trigger naturally once Layer 1 lands
   (that is the goal), so it is verified by the worker unit tests (per-doc error → backoff →
   `dead` at `maxAttempts`) and the pgstore/httpserver tests; the live gate confirms the
   **Retry** button visibly clears state and re-queues a doc to `pending`.
5. **Deploy** (shared with Säule A's post-merge step if landed together): `:rebuild` image
   build → homelab-study digest bump → ArgoCD sync.

## 7. Out of scope / sequencing

Säule A (durable auth) is independent and already implemented; this work may land and deploy
together with it (one digest bump) or separately. No background keep-alive, no model change,
no CLI/TUI surface, no live SSE embed-completion push. Paragraph-aware chunking remains a
possible future refinement.
