# flow Embed Poison-Doc Fix (Säule B) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the flow-server embed worker actually embed every document (remove the two self-inflicted deterministic failure modes) and survive the rest gracefully — no head-of-line block, no retry-storm, with per-doc backoff → dead-letter visible and retryable in the WebUI.

**Architecture:** Two layers. Layer 1 removes the poison sources: `chunk.Split` stops emitting empty chunks, and `embed.Ollama` sub-batches inputs so a large doc never trips the fixed 60s timeout. Layer 2 is the safety net: a persisted `document_embed_failures` side table drives per-doc capped backoff → dead-letter; the worker classifies transient (Ollama down/misconfigured) vs per-doc errors via `ports.ErrEmbedTransient`, continues past per-doc failures, and the WebUI shows status + a Retry button.

**Tech Stack:** Go, hexagonal layout (`domain` / `ports` / `adapter` / `usecase` / `worker`), Postgres via pgx + goose migrations, templ + htmx WebUI, bubbletea/lipgloss elsewhere (not touched here).

## Global Constraints

- **Branch:** `rebuild`, worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`. Do NOT merge to `main`.
- **Keine Monolithen:** one responsibility per file; new embed-failure SQL lives in `internal/adapter/pgstore/documents_embed.go`, not bloating `documents.go`.
- **Coverage gate:** `make cover` must stay ≥ **80%** (`COVER_THRESHOLD=80`, `COVER_PKG=./internal/...`). `make ci` = `lint verify-generate cover build`.
- **Lint (golangci-lint):** ST1005 — error strings lowercase, **no trailing punctuation**. Run `make lint` before each commit touching Go.
- **templ:** after editing any `*.templ`, run `make generate` (`go tool templ generate`) and commit the regenerated `*_templ.go`; `make ci` fails (`verify-generate`) if stale.
- **pgstore tests need Docker** — `startPG(t)` provisions Postgres and `pgstore.Migrate` applies all `migrations/*.sql` (so a new `0013_*.sql` auto-applies). Migrations require goose annotations (`-- +goose Up` / `-- +goose Down`); bare SQL fails at apply.
- **Defaults:** `maxInputsPerCall = 64`, `maxAttempts = 5`, backoff `base = 1m`, `cap = 6h`.
- **Commit trailers** — every commit message ends with these two lines:
  ```
  Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01CicEBhyR4hr3ci2gyoneiP
  ```
  Referenced below as “(+ standard trailers)”.
- **Spec:** `docs/superpowers/specs/2026-06-22-flow-embed-poison-doc-design.md`.

---

### Task 1: `chunk.Split` — never emit an empty/whitespace chunk

**Files:**
- Modify: `internal/chunk/chunk.go` (the window loop, ~lines 36-58)
- Test: `internal/chunk/chunk_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `chunk.Split(title, body string) []string` — unchanged signature; new guarantee: never returns an empty string, and a whitespace-only window yields no chunk (and no degenerate title-only duplicate).

- [ ] **Step 1: Write the failing tests**

Add to `internal/chunk/chunk_test.go`:

```go
func TestSplit_WhitespaceGap_EmptyTitle_NeverEmptyChunk(t *testing.T) {
	body := "A" + strings.Repeat(" ", 4000) + "Z" // forces an all-whitespace middle window
	got := Split("", body)
	if len(got) == 0 {
		t.Fatalf("want >=1 chunk for non-empty body")
	}
	for i, c := range got {
		if c == "" {
			t.Fatalf("chunk %d is empty; Split must never emit an empty chunk: %#v", i, got)
		}
	}
}

func TestSplit_WhitespaceGap_WithTitle_NoTitleOnlyDuplicate(t *testing.T) {
	body := "A" + strings.Repeat(" ", 4000) + "Z"
	got := Split("T", body)
	for i, c := range got {
		if c == "T\n\n" {
			t.Fatalf("chunk %d is a degenerate title-only duplicate: %#v", i, got)
		}
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/chunk/ -run 'TestSplit_WhitespaceGap' -v`
Expected: FAIL — `TestSplit_WhitespaceGap_EmptyTitle_NeverEmptyChunk` reports an empty chunk.

- [ ] **Step 3: Implement the filter**

In `internal/chunk/chunk.go`, replace the loop body so empty windows are skipped before the title is prepended:

```go
	for start := 0; start < len(r); start += step {
		end := start + MaxChars
		if end > len(r) {
			end = len(r)
		}
		w := strings.TrimSpace(string(r[start:end]))
		if w == "" {
			// all-whitespace window: emit nothing (no empty embed input, no
			// degenerate title-only duplicate).
			if end == len(r) {
				break
			}
			continue
		}
		if title != "" {
			w = title + "\n\n" + w
		}
		out = append(out, w)
		if end == len(r) {
			break
		}
	}
```

- [ ] **Step 4: Run to verify they pass (and no regressions)**

Run: `go test ./internal/chunk/ -v`
Expected: PASS (all, including the pre-existing `TestSplit_*`).

- [ ] **Step 5: Commit**

```bash
git add internal/chunk/chunk.go internal/chunk/chunk_test.go
git commit -m "fix(chunk): never emit empty/whitespace chunks (Säule B L1.1)" # (+ standard trailers)
```

---

### Task 2: `embed.Ollama.Embed` — sub-batch inputs per HTTP call

**Files:**
- Modify: `internal/adapter/embed/ollama.go` (`Embed`, extract `embedOnce`)
- Test: `internal/adapter/embed/ollama_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `(*Ollama).Embed(ctx, texts) ([][]float32, error)` — unchanged signature; now issues `ceil(len(texts)/64)` calls and concatenates in order. New unexported `(*Ollama).embedOnce(ctx, texts) ([][]float32, error)` holding the single-request logic.

- [ ] **Step 1: Write the failing test**

Add to `internal/adapter/embed/ollama_test.go` (package `embed_test`):

```go
func TestOllamaEmbed_SubBatchesAndPreservesOrder(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		var req struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		embs := make([][]float32, len(req.Input))
		for i, s := range req.Input {
			embs[i] = []float32{float32(len(s))} // value == input length → lets us assert order
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": embs})
	}))
	defer srv.Close()

	o := embed.NewOllama(srv.URL, "test-model")
	texts := make([]string, 130)
	for i := range texts {
		texts[i] = strings.Repeat("x", i+1) // unique length per input
	}
	vecs, err := o.Embed(context.Background(), texts)
	if err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("want 3 calls (64+64+2), got %d", got)
	}
	if len(vecs) != 130 {
		t.Fatalf("want 130 vectors, got %d", len(vecs))
	}
	for i := range texts {
		if vecs[i][0] != float32(i+1) {
			t.Fatalf("order broken at %d: want %d, got %v", i, i+1, vecs[i][0])
		}
	}
}
```

Ensure the test file imports: `context`, `encoding/json`, `net/http`, `net/http/httptest`, `strings`, `sync/atomic`, `testing`, and `github.com/serverkraken/flow/internal/adapter/embed`.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/adapter/embed/ -run TestOllamaEmbed_SubBatches -v`
Expected: FAIL — `want 3 calls, got 1` (current code sends everything in one request).

- [ ] **Step 3: Implement sub-batching**

In `internal/adapter/embed/ollama.go`, add the constant and rewrite `Embed`, moving the existing request body into `embedOnce`:

```go
// maxInputsPerCall caps how many texts go in one /api/embed request so a large
// document never grows a single call past the client timeout.
const maxInputsPerCall = 64

// Embed implements ports.Embedder.
func (o *Ollama) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	out := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += maxInputsPerCall {
		end := start + maxInputsPerCall
		if end > len(texts) {
			end = len(texts)
		}
		vecs, err := o.embedOnce(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		out = append(out, vecs...)
	}
	if len(out) != len(texts) {
		return nil, fmt.Errorf("ollama embed: got %d vectors for %d texts", len(out), len(texts))
	}
	return out, nil
}

func (o *Ollama) embedOnce(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(embedReq{Model: o.model, Input: texts})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.host+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<12))
		return nil, fmt.Errorf("ollama embed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var er embedResp
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		return nil, fmt.Errorf("ollama embed decode: %w", err)
	}
	if len(er.Embeddings) != len(texts) {
		return nil, fmt.Errorf("ollama embed: got %d vectors for %d texts", len(er.Embeddings), len(texts))
	}
	return er.Embeddings, nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/adapter/embed/ -v`
Expected: PASS (new test + any existing ones).

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/embed/ollama.go internal/adapter/embed/ollama_test.go
git commit -m "feat(embed): sub-batch Ollama inputs (64/call) to avoid timeouts (Säule B L1.2)" # (+ standard trailers)
```

---

### Task 3: `ports.ErrEmbedTransient` + Ollama transient classification

**Files:**
- Modify: `internal/ports/ports.go` (add the sentinel near the other `Err*` vars; ensure `errors` is imported)
- Modify: `internal/adapter/embed/ollama.go` (`embedOnce`: wrap transient causes)
- Test: `internal/adapter/embed/ollama_test.go`

**Interfaces:**
- Produces: `var ports.ErrEmbedTransient error`. After this task, a `*Ollama` embed failure satisfies `errors.Is(err, ports.ErrEmbedTransient)` iff the cause is a connection error, a client timeout, or HTTP 503/429/404.

- [ ] **Step 1: Add the sentinel to ports**

In `internal/ports/ports.go`, add (with the other sentinel errors; confirm `errors` is in the import block):

```go
// ErrEmbedTransient marks an embed failure as transient/global — the backend is
// unavailable or misconfigured (connection error, timeout, HTTP 503/429, or a
// missing model 404) — rather than caused by one document's content. The embed
// worker tests for it with errors.Is to decide whether to back off a single
// document or just stop and retry the whole batch next tick.
var ErrEmbedTransient = errors.New("embed backend transient failure")
```

- [ ] **Step 2: Write the failing tests**

Add to `internal/adapter/embed/ollama_test.go`:

```go
func TestOllamaEmbed_ClassifiesStatus(t *testing.T) {
	cases := []struct {
		code      int
		transient bool
	}{
		{http.StatusServiceUnavailable, true}, // 503
		{http.StatusTooManyRequests, true},    // 429
		{http.StatusNotFound, true},           // 404 model-not-found
		{http.StatusBadRequest, false},        // 400
		{http.StatusInternalServerError, false}, // 500 on one doc
	}
	for _, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", c.code)
		}))
		o := embed.NewOllama(srv.URL, "m")
		_, err := o.Embed(context.Background(), []string{"hi"})
		srv.Close()
		if err == nil {
			t.Fatalf("status %d: want error", c.code)
		}
		if got := errors.Is(err, ports.ErrEmbedTransient); got != c.transient {
			t.Fatalf("status %d: transient=%v want %v (err=%v)", c.code, got, c.transient, err)
		}
	}
}

func TestOllamaEmbed_ConnError_IsTransient(t *testing.T) {
	o := embed.NewOllama("http://127.0.0.1:1", "m") // nothing listening
	_, err := o.Embed(context.Background(), []string{"hi"})
	if err == nil || !errors.Is(err, ports.ErrEmbedTransient) {
		t.Fatalf("want transient connection error, got %v", err)
	}
}
```

Add imports `errors` and `github.com/serverkraken/flow/internal/ports` to the test file.

- [ ] **Step 3: Run to verify they fail**

Run: `go test ./internal/adapter/embed/ -run 'TestOllamaEmbed_Classif|TestOllamaEmbed_ConnError' -v`
Expected: FAIL — errors are not yet wrapped with `ErrEmbedTransient`.

- [ ] **Step 4: Wrap transient causes in `embedOnce`**

In `internal/adapter/embed/ollama.go`, import `"github.com/serverkraken/flow/internal/ports"`, then update the two error sites in `embedOnce` and add the helper:

```go
	resp, err := o.client.Do(req)
	if err != nil {
		// dial failure / connection refused / reset / client timeout — environmental.
		return nil, fmt.Errorf("ollama embed: %w: %w", ports.ErrEmbedTransient, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<12))
		if isTransientStatus(resp.StatusCode) {
			return nil, fmt.Errorf("ollama embed: %w: status %d: %s", ports.ErrEmbedTransient, resp.StatusCode, strings.TrimSpace(string(b)))
		}
		return nil, fmt.Errorf("ollama embed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
```

Add at the bottom of the file:

```go
// isTransientStatus reports whether an Ollama HTTP status means the backend is
// unavailable or misconfigured (retry later) rather than this input being bad.
// 404 = model not found — a server-config problem that fails every document.
func isTransientStatus(code int) bool {
	return code == http.StatusServiceUnavailable ||
		code == http.StatusTooManyRequests ||
		code == http.StatusNotFound
}
```

- [ ] **Step 5: Run to verify they pass**

Run: `go test ./internal/adapter/embed/ ./internal/ports/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/ports/ports.go internal/adapter/embed/ollama.go internal/adapter/embed/ollama_test.go
git commit -m "feat(embed): classify transient vs per-doc embed errors (Säule B L2.5)" # (+ standard trailers)
```

---

### Task 4: Migration `0013` + `ports.StaleDoc` + `StaleDocuments` rewrite

**Files:**
- Create: `internal/adapter/pgstore/migrations/0013_document_embed_failures.sql`
- Create: `internal/adapter/pgstore/documents_embed.go` (new home for the embed-worker query)
- Modify: `internal/ports/ports.go` (`StaleDoc` type; change `StaleDocuments` return type)
- Modify: `internal/adapter/pgstore/documents.go` (delete the old `StaleDocuments`)
- Modify: `internal/testutil/fakes.go` (fake `StaleDocuments` → `[]ports.StaleDoc`, sorted)
- Modify: `internal/worker/embed_worker.go` (compile-fix: `sd.Doc`)
- Modify: `internal/adapter/pgstore/documents_test.go` (existing stale test → `.Doc`)

**Interfaces:**
- Produces: `type ports.StaleDoc struct { Doc domain.Document; Attempts int }`; `StaleDocuments(ctx, limit) ([]ports.StaleDoc, error)` excluding dead-lettered/backing-off docs and carrying the prior failure count.

- [ ] **Step 1: Add the migration**

Create `internal/adapter/pgstore/migrations/0013_document_embed_failures.sql`:

```sql
-- +goose Up
CREATE TABLE document_embed_failures (
    document_id   text PRIMARY KEY REFERENCES documents(id) ON DELETE CASCADE,
    owner_id      text        NOT NULL,
    attempts      int         NOT NULL,
    next_retry_at timestamptz NOT NULL,
    last_error    text        NOT NULL DEFAULT '',
    dead          boolean     NOT NULL DEFAULT false
);

-- +goose Down
DROP TABLE document_embed_failures;
```

- [ ] **Step 2: Change the port**

In `internal/ports/ports.go`, add the type above `DocumentStore` and change the method:

```go
// StaleDoc is a document needing (re)embedding plus its prior consecutive
// embed-failure count, so the worker computes backoff without an extra read.
type StaleDoc struct {
	Doc      domain.Document
	Attempts int
}
```

```go
	// StaleDocuments returns up to limit documents needing (re)embedding
	// (chunks_hash out of date), excluding dead-lettered docs and those still
	// within a backoff window, each with its prior consecutive failure count.
	StaleDocuments(ctx context.Context, limit int) ([]StaleDoc, error)
```

- [ ] **Step 3: Move + rewrite `StaleDocuments` in pgstore**

Delete the existing `StaleDocuments` method from `internal/adapter/pgstore/documents.go` (lines ~243-262). Create `internal/adapter/pgstore/documents_embed.go`:

```go
// Package pgstore — embed-worker bookkeeping (stale selection + failure state).
package pgstore

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// StaleDocuments returns up to limit documents whose chunks are out of date,
// skipping dead-lettered docs and those still inside a backoff window, ordered
// oldest-update-first, each with its prior consecutive failure count.
func (s *DocumentStore) StaleDocuments(ctx context.Context, limit int) ([]ports.StaleDoc, error) {
	q := `SELECT ` + prefixedDocCols + `, coalesce(f.attempts, 0)
FROM documents d
LEFT JOIN document_embed_failures f ON f.document_id = d.id
WHERE d.chunks_hash IS DISTINCT FROM md5(coalesce(d.title,'')||coalesce(d.body,''))
  AND coalesce(f.dead, false) = false
  AND (f.next_retry_at IS NULL OR f.next_retry_at <= now())
ORDER BY d.updated_at ASC
LIMIT $1`
	rows, err := s.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("pgstore: stale documents: %w", err)
	}
	defer rows.Close()
	var out []ports.StaleDoc
	for rows.Next() {
		var d domain.Document
		var typ string
		var extra []byte
		var attempts int
		if err := rows.Scan(&d.ID, &d.OwnerID, &d.ProjectID, &typ, &d.Path, &d.Title, &d.Body,
			&d.Tags, &d.Date, &d.Role, &extra, &d.CreatedAt, &d.UpdatedAt, &attempts); err != nil {
			return nil, fmt.Errorf("pgstore: scan stale document: %w", err)
		}
		d.Type = domain.DocumentType(typ)
		if len(extra) > 0 {
			if err := json.Unmarshal(extra, &d.Extra); err != nil {
				return nil, fmt.Errorf("pgstore: unmarshal extra: %w", err)
			}
		}
		out = append(out, ports.StaleDoc{Doc: d, Attempts: attempts})
	}
	return out, rows.Err()
}
```

Add `"encoding/json"` to this file's imports (used by `json.Unmarshal`).

- [ ] **Step 4: Compile-fix the worker (mechanical)**

In `internal/worker/embed_worker.go`, the `drain` inner loop currently ranges `domain.Document`. Change only the iteration to use `sd.Doc` (behavior unchanged here — full refactor is Task 6):

```go
		for _, sd := range docs {
			if ctx.Err() != nil {
				return
			}
			if err := w.embedOne(ctx, sd.Doc); err != nil {
				w.log.Warn("embed worker: embed doc", "id", sd.Doc.ID, "err", err)
				return // backend likely down; retry next tick
			}
		}
```

- [ ] **Step 5: Update the fakes (sorted, `[]ports.StaleDoc`)**

In `internal/testutil/fakes.go`, replace the fake `StaleDocuments` (no failure-table awareness yet — that lands in Task 5):

```go
func (s *FakeDocumentStore) StaleDocuments(_ context.Context, limit int) ([]ports.StaleDoc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []ports.StaleDoc
	for _, d := range s.m {
		if s.chunksHash[d.ID] != fakeDocHash(d) {
			out = append(out, ports.StaleDoc{Doc: d})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Doc.UpdatedAt.Equal(out[j].Doc.UpdatedAt) {
			return out[i].Doc.ID < out[j].Doc.ID
		}
		return out[i].Doc.UpdatedAt.Before(out[j].Doc.UpdatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
```

(`sort` is already imported in `fakes.go`.)

- [ ] **Step 6: Fix existing pgstore stale test**

In `internal/adapter/pgstore/documents_test.go`, the existing stale-documents test does `stale, _ := s.StaleDocuments(ctx, 100)` then indexes documents. Update those references from `stale[i]` (a `domain.Document`) to `stale[i].Doc`. Search the file for `StaleDocuments(` and fix the assertions accordingly (e.g. `stale[0].ID` → `stale[0].Doc.ID`, `len(stale)` stays).

- [ ] **Step 7: Verify build + grep for other callers**

Run:
```bash
go build ./... 2>&1 | head
rg -n "StaleDocuments\(" --glob '*.go'
```
Expected: build OK. Fix any remaining caller (e.g. `internal/usecase/document_test.go`) to use `.Doc` the same way. Re-run `go build ./...` until clean.

- [ ] **Step 8: Run pgstore + worker tests**

Run: `go test ./internal/adapter/pgstore/ -run 'Stale|CRUD' ./internal/worker/ -v`
Expected: PASS (Docker must be running for pgstore).

- [ ] **Step 9: Commit**

```bash
git add internal/adapter/pgstore/migrations/0013_document_embed_failures.sql \
        internal/adapter/pgstore/documents_embed.go internal/adapter/pgstore/documents.go \
        internal/adapter/pgstore/documents_test.go internal/ports/ports.go \
        internal/testutil/fakes.go internal/worker/embed_worker.go
git commit -m "feat(pgstore): document_embed_failures migration + StaleDoc backoff-aware query (Säule B L2.1)" # (+ standard trailers)
```

---

### Task 5: `domain.EmbedStatus` + pgstore failure methods + `ReplaceChunks` clear

**Files:**
- Create: `internal/domain/embed_status.go`
- Modify: `internal/adapter/pgstore/documents_embed.go` (add `RecordEmbedFailure`, `ClearEmbedFailure`, `EmbedStatus`)
- Modify: `internal/adapter/pgstore/documents.go` (`ReplaceChunks`: delete failure row in-tx)
- Modify: `internal/ports/ports.go` (add the three methods + `time` import if missing)
- Modify: `internal/testutil/fakes.go` (failure map + three methods + exclusion + `ReplaceChunks` clear)
- Test: `internal/adapter/pgstore/documents_embed_test.go`

**Interfaces:**
- Consumes: `ports.StaleDoc`, migration `0013`.
- Produces:
  - `domain.EmbedState` (`EmbedOK|EmbedPending|EmbedRetrying|EmbedFailed`), `domain.EmbedStatus{State, Attempts, LastError, NextRetry}`.
  - `RecordEmbedFailure(ctx, docID, ownerID string, attempts int, nextRetryAt time.Time, dead bool, lastErr string) error`
  - `ClearEmbedFailure(ctx, docID, ownerID string) error`
  - `EmbedStatus(ctx, ownerID, docID string) (domain.EmbedStatus, error)`
  - `ReplaceChunks` now also deletes any failure row on success.

- [ ] **Step 1: Add the domain value object**

Create `internal/domain/embed_status.go`:

```go
package domain

import "time"

// EmbedState is a document's embedding state, derived from chunk freshness and
// any recorded embed-failure.
type EmbedState string

const (
	EmbedOK       EmbedState = "ok"       // chunks current
	EmbedPending  EmbedState = "pending"  // stale, queued, no failure recorded
	EmbedRetrying EmbedState = "retrying" // failed, within backoff, will retry
	EmbedFailed   EmbedState = "failed"   // dead-lettered; needs a manual retry
)

// EmbedStatus is the read model describing a document's embedding state.
type EmbedStatus struct {
	State     EmbedState
	Attempts  int
	LastError string
	NextRetry *time.Time
}
```

- [ ] **Step 2: Add the port methods**

In `internal/ports/ports.go` `DocumentStore`, after `ReplaceChunks`, add (ensure `time` is imported):

```go
	// RecordEmbedFailure upserts the per-document embed-failure state used for
	// backoff and dead-lettering.
	RecordEmbedFailure(ctx context.Context, docID, ownerID string, attempts int, nextRetryAt time.Time, dead bool, lastErr string) error
	// ClearEmbedFailure removes a document's recorded embed failure (manual
	// retry); a successful ReplaceChunks clears it implicitly.
	ClearEmbedFailure(ctx context.Context, docID, ownerID string) error
	// EmbedStatus returns the owner-scoped embedding status of one document.
	EmbedStatus(ctx context.Context, ownerID, docID string) (domain.EmbedStatus, error)
```

- [ ] **Step 3: Write the failing pgstore tests**

Create `internal/adapter/pgstore/documents_embed_test.go` (mirror the harness of `documents_test.go`: `startPG(t)`, `NewPool`, `Migrate`, seed a user, `NewDocumentStore`). Cover:

```go
func TestEmbedFailures_RecordExcludeClearStatus(t *testing.T) {
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
	u, _ := domain.NewUser("u-emb", "sub-emb", "emb", "emb@x.de", "Emb")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	st := pgstore.NewDocumentStore(pool)
	now := time.Now().UTC().Truncate(time.Second)
	mk := func(id string) domain.Document {
		return domain.Document{ID: id, OwnerID: "u-emb", Type: domain.DocFree, Path: id, Title: id, Body: "b", CreatedAt: now, UpdatedAt: now}
	}
	if _, err := st.Create(ctx, mk("d-dead")); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Create(ctx, mk("d-future")); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Create(ctx, mk("d-due")); err != nil {
		t.Fatal(err)
	}

	// dead-lettered → excluded; pending status reads as "pending" until recorded
	if s, err := st.EmbedStatus(ctx, "u-emb", "d-dead"); err != nil || s.State != domain.EmbedPending {
		t.Fatalf("fresh doc want pending, got %v err=%v", s.State, err)
	}
	if err := st.RecordEmbedFailure(ctx, "d-dead", "u-emb", 5, now, true, "boom"); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordEmbedFailure(ctx, "d-future", "u-emb", 2, now.Add(time.Hour), false, "later"); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordEmbedFailure(ctx, "d-due", "u-emb", 1, now.Add(-time.Hour), false, "due"); err != nil {
		t.Fatal(err)
	}

	stale, err := st.StaleDocuments(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, sd := range stale {
		got[sd.Doc.ID] = sd.Attempts
	}
	if _, ok := got["d-dead"]; ok {
		t.Fatalf("dead doc must be excluded: %v", got)
	}
	if _, ok := got["d-future"]; ok {
		t.Fatalf("backing-off doc must be excluded: %v", got)
	}
	if got["d-due"] != 1 {
		t.Fatalf("due doc must be present with attempts=1, got %v", got)
	}

	// status reflects dead + retrying
	if s, _ := st.EmbedStatus(ctx, "u-emb", "d-dead"); s.State != domain.EmbedFailed || s.LastError != "boom" {
		t.Fatalf("dead status: %#v", s)
	}
	if s, _ := st.EmbedStatus(ctx, "u-emb", "d-future"); s.State != domain.EmbedRetrying || s.NextRetry == nil {
		t.Fatalf("retrying status: %#v", s)
	}

	// clear restores eligibility
	if err := st.ClearEmbedFailure(ctx, "d-dead", "u-emb"); err != nil {
		t.Fatal(err)
	}
	stale, _ = st.StaleDocuments(ctx, 100)
	found := false
	for _, sd := range stale {
		if sd.Doc.ID == "d-dead" {
			found = true
		}
	}
	if !found {
		t.Fatalf("cleared doc must be stale again")
	}

	// success clears the row + flips status to ok
	if err := st.ReplaceChunks(ctx, "d-due", "u-emb", []string{"c"}, [][]float32{vec(0.1)}); err != nil {
		t.Fatal(err)
	}
	if s, _ := st.EmbedStatus(ctx, "u-emb", "d-due"); s.State != domain.EmbedOK {
		t.Fatalf("embedded doc want ok, got %v", s.State)
	}
}
```

Reuse the `vec(...)` helper already defined in the pgstore test package (used by the existing `ReplaceChunks` tests). Imports: `context`, `testing`, `time`, `github.com/serverkraken/flow/internal/adapter/pgstore`, `.../internal/domain`.

- [ ] **Step 4: Run to verify it fails**

Run: `go test ./internal/adapter/pgstore/ -run TestEmbedFailures -v`
Expected: FAIL — `RecordEmbedFailure`/`EmbedStatus`/`ClearEmbedFailure` not implemented.

- [ ] **Step 5: Implement the pgstore methods**

Append to `internal/adapter/pgstore/documents_embed.go` (add `"errors"`, `"time"`, and `"github.com/jackc/pgx/v5"` to its imports; match the pgx import path already used in `documents.go`):

```go
func (s *DocumentStore) RecordEmbedFailure(ctx context.Context, docID, ownerID string, attempts int, nextRetryAt time.Time, dead bool, lastErr string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO document_embed_failures (document_id, owner_id, attempts, next_retry_at, last_error, dead)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 ON CONFLICT (document_id) DO UPDATE
		   SET attempts = $3, next_retry_at = $4, last_error = $5, dead = $6`,
		docID, ownerID, attempts, nextRetryAt, lastErr, dead)
	if err != nil {
		return fmt.Errorf("pgstore: record embed failure: %w", err)
	}
	return nil
}

func (s *DocumentStore) ClearEmbedFailure(ctx context.Context, docID, ownerID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM document_embed_failures WHERE document_id = $1 AND owner_id = $2`, docID, ownerID)
	if err != nil {
		return fmt.Errorf("pgstore: clear embed failure: %w", err)
	}
	return nil
}

func (s *DocumentStore) EmbedStatus(ctx context.Context, ownerID, docID string) (domain.EmbedStatus, error) {
	q := `SELECT
  CASE
    WHEN f.dead THEN 'failed'
    WHEN f.document_id IS NOT NULL THEN 'retrying'
    WHEN d.chunks_hash IS DISTINCT FROM md5(coalesce(d.title,'')||coalesce(d.body,'')) THEN 'pending'
    ELSE 'ok'
  END,
  coalesce(f.attempts, 0), coalesce(f.last_error, ''), f.next_retry_at
FROM documents d
LEFT JOIN document_embed_failures f ON f.document_id = d.id
WHERE d.id = $1 AND d.owner_id = $2`
	var state string
	var st domain.EmbedStatus
	var next *time.Time
	err := s.pool.QueryRow(ctx, q, docID, ownerID).Scan(&state, &st.Attempts, &st.LastError, &next)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.EmbedStatus{}, ports.ErrDocumentNotFound
	}
	if err != nil {
		return domain.EmbedStatus{}, fmt.Errorf("pgstore: embed status: %w", err)
	}
	st.State = domain.EmbedState(state)
	st.NextRetry = next
	return st, nil
}
```

- [ ] **Step 6: `ReplaceChunks` clears the failure row in-tx**

In `internal/adapter/pgstore/documents.go`, inside `ReplaceChunks`, just before `return tx.Commit(ctx)` (after the `chunks_hash` stamp), add:

```go
	if _, err := tx.Exec(ctx, `DELETE FROM document_embed_failures WHERE document_id = $1`, docID); err != nil {
		return fmt.Errorf("pgstore: clear embed failure: %w", err)
	}
```

- [ ] **Step 7: Update the fakes (failure map + methods + exclusion + clear)**

In `internal/testutil/fakes.go`:

(a) Add a field to `FakeDocumentStore` and init it in `NewFakeDocumentStore`:

```go
	embedFail  map[string]fakeEmbedFail
```
```go
		embedFail:  map[string]fakeEmbedFail{},
```

(b) Add the helper type (near `fakeChunk`):

```go
type fakeEmbedFail struct {
	attempts  int
	nextRetry time.Time
	lastErr   string
	dead      bool
}
```

(c) Replace the fake `StaleDocuments` body's append/loop to honor exclusion + attempts:

```go
func (s *FakeDocumentStore) StaleDocuments(_ context.Context, limit int) ([]ports.StaleDoc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	var out []ports.StaleDoc
	for _, d := range s.m {
		if s.chunksHash[d.ID] == fakeDocHash(d) {
			continue
		}
		attempts := 0
		if f, ok := s.embedFail[d.ID]; ok {
			if f.dead || f.nextRetry.After(now) {
				continue
			}
			attempts = f.attempts
		}
		out = append(out, ports.StaleDoc{Doc: d, Attempts: attempts})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Doc.UpdatedAt.Equal(out[j].Doc.UpdatedAt) {
			return out[i].Doc.ID < out[j].Doc.ID
		}
		return out[i].Doc.UpdatedAt.Before(out[j].Doc.UpdatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
```

(d) In the fake `ReplaceChunks`, add `delete(s.embedFail, docID)` after stamping the hash.

(e) Add the three methods:

```go
func (s *FakeDocumentStore) RecordEmbedFailure(_ context.Context, docID, _ string, attempts int, nextRetryAt time.Time, dead bool, lastErr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.embedFail[docID] = fakeEmbedFail{attempts: attempts, nextRetry: nextRetryAt, lastErr: lastErr, dead: dead}
	return nil
}

func (s *FakeDocumentStore) ClearEmbedFailure(_ context.Context, docID, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.embedFail, docID)
	return nil
}

func (s *FakeDocumentStore) EmbedStatus(_ context.Context, ownerID, docID string) (domain.EmbedStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.m[docID]
	if !ok || d.OwnerID != ownerID {
		return domain.EmbedStatus{}, ports.ErrDocumentNotFound
	}
	if f, ok := s.embedFail[docID]; ok {
		st := domain.EmbedStatus{Attempts: f.attempts, LastError: f.lastErr}
		if f.dead {
			st.State = domain.EmbedFailed
		} else {
			st.State = domain.EmbedRetrying
			nr := f.nextRetry
			st.NextRetry = &nr
		}
		return st, nil
	}
	if s.chunksHash[docID] != fakeDocHash(d) {
		return domain.EmbedStatus{State: domain.EmbedPending}, nil
	}
	return domain.EmbedStatus{State: domain.EmbedOK}, nil
}
```

(Ensure `time` is imported in `fakes.go`.)

- [ ] **Step 8: Run to verify pass + build**

Run:
```bash
go build ./...
go test ./internal/adapter/pgstore/ -run TestEmbedFailures -v
go test ./internal/domain/ ./internal/testutil/ ./internal/worker/
```
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/domain/embed_status.go internal/adapter/pgstore/documents_embed.go \
        internal/adapter/pgstore/documents_embed_test.go internal/adapter/pgstore/documents.go \
        internal/ports/ports.go internal/testutil/fakes.go
git commit -m "feat(pgstore): embed-failure record/clear/status + ReplaceChunks clears row (Säule B L2)" # (+ standard trailers)
```

---

### Task 6: Worker drain refactor + backoff + policy

**Files:**
- Create: `internal/worker/backoff.go`
- Create: `internal/worker/backoff_test.go`
- Modify: `internal/worker/embed_worker.go` (policy + clock + `drain`/`embedDoc`)
- Modify: `internal/worker/embed_worker_test.go` (new ctor arg; transient `errDown`; new behavior tests)
- Modify: `internal/testutil/fakes.go` (`FakeEmbedder.FailFunc`)
- Modify: `cmd/flow-server/main.go` (pass `worker.EmbedPolicy{}`)

**Interfaces:**
- Consumes: `ports.StaleDoc`, `ports.ErrEmbedTransient`, `RecordEmbedFailure`.
- Produces:
  - `type worker.EmbedPolicy struct { MaxAttempts int; BackoffBase, BackoffCap time.Duration }`
  - `worker.NewEmbedWorker(docs, e, interval, batch, pol EmbedPolicy, log) *EmbedWorker`
  - unexported `backoff(attempts int, base, ceiling time.Duration) time.Duration`
  - `testutil.FakeEmbedder.FailFunc func(texts []string) error`

- [ ] **Step 1: Write the backoff test**

Create `internal/worker/backoff_test.go`:

```go
package worker

import (
	"testing"
	"time"
)

func TestBackoff(t *testing.T) {
	base, ceiling := time.Minute, 6*time.Hour
	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{1, 1 * time.Minute},
		{2, 2 * time.Minute},
		{3, 4 * time.Minute},
		{4, 8 * time.Minute},
		{100, ceiling}, // overflow clamps to ceiling
	}
	for _, c := range cases {
		if got := backoff(c.attempts, base, ceiling); got != c.want {
			t.Fatalf("backoff(%d) = %v, want %v", c.attempts, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/worker/ -run TestBackoff -v`
Expected: FAIL — `backoff` undefined.

- [ ] **Step 3: Implement `backoff`**

Create `internal/worker/backoff.go`:

```go
package worker

import "time"

// backoff returns the wait before the next retry of a document that has failed
// `attempts` times (attempts >= 1): exponential from base, clamped to ceiling.
// Pure; ceiling also guards against left-shift overflow.
func backoff(attempts int, base, ceiling time.Duration) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	d := base << (attempts - 1)
	if d <= 0 || d > ceiling {
		d = ceiling
	}
	return d
}
```

Run: `go test ./internal/worker/ -run TestBackoff -v` → PASS.

- [ ] **Step 4: Add `FailFunc` to the fake embedder**

In `internal/testutil/fakes.go`, extend `FakeEmbedder` and consult it first:

```go
type FakeEmbedder struct {
	Dim      int
	Err      error
	FailFunc func(texts []string) error // optional per-call hook (checked before Err)
}
```
```go
func (f *FakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if f.FailFunc != nil {
		if err := f.FailFunc(texts); err != nil {
			return nil, err
		}
	}
	if f.Err != nil {
		return nil, f.Err
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = pseudoVec(t, f.Dim)
	}
	return out, nil
}
```

- [ ] **Step 5: Refactor the worker (policy, clock, drain, embedDoc)**

In `internal/worker/embed_worker.go`: add `"errors"` to imports and drop the now-unused `"github.com/serverkraken/flow/internal/domain"` import. Add the policy type, constants, and fields; rewrite the constructor and `drain`; replace `embedOne` with `embedDoc`:

```go
const (
	defaultMaxAttempts = 5
	defaultBackoffBase = time.Minute
	defaultBackoffCap  = 6 * time.Hour
)

// EmbedPolicy tunes per-document failure handling. Zero fields take the defaults.
type EmbedPolicy struct {
	MaxAttempts int
	BackoffBase time.Duration
	BackoffCap  time.Duration
}
```

Add fields to `EmbedWorker`:

```go
	pol   EmbedPolicy
	clock func() time.Time
```

Constructor:

```go
func NewEmbedWorker(docs ports.DocumentStore, e ports.Embedder, interval time.Duration, batch int, pol EmbedPolicy, log *slog.Logger) *EmbedWorker {
	if batch <= 0 {
		batch = 16
	}
	if pol.MaxAttempts <= 0 {
		pol.MaxAttempts = defaultMaxAttempts
	}
	if pol.BackoffBase <= 0 {
		pol.BackoffBase = defaultBackoffBase
	}
	if pol.BackoffCap <= 0 {
		pol.BackoffCap = defaultBackoffCap
	}
	return &EmbedWorker{docs: docs, embedder: e, interval: interval, batch: batch, pol: pol, clock: time.Now, kick: make(chan struct{}, 1), log: log}
}
```

`drain` + `embedDoc` (replaces the old `drain` and `embedOne`):

```go
func (w *EmbedWorker) drain(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		docs, err := w.docs.StaleDocuments(ctx, w.batch)
		if err != nil {
			w.log.Warn("embed worker: list stale", "err", err)
			return
		}
		if len(docs) == 0 {
			return
		}
		for _, sd := range docs {
			if ctx.Err() != nil {
				return
			}
			if !w.embedDoc(ctx, sd) {
				return // transient backend failure or store error: stop, retry next tick
			}
		}
	}
}

// embedDoc embeds one document. It returns false when the drain must STOP (a
// transient backend failure or a store error — retry the whole batch next tick)
// and true when it may CONTINUE to the next document (success, or a per-doc
// failure that was recorded for backoff/dead-letter).
func (w *EmbedWorker) embedDoc(ctx context.Context, sd ports.StaleDoc) bool {
	d := sd.Doc
	texts := chunk.Split(d.Title, d.Body)
	if len(texts) == 0 {
		if err := w.docs.ReplaceChunks(ctx, d.ID, d.OwnerID, nil, nil); err != nil {
			w.log.Warn("embed worker: clear chunks", "id", d.ID, "err", err)
			return false
		}
		return true
	}
	vecs, err := w.embedder.Embed(ctx, texts)
	if err != nil {
		if errors.Is(err, ports.ErrEmbedTransient) {
			w.log.Warn("embed worker: backend unavailable", "id", d.ID, "err", err)
			return false // do not penalize the doc
		}
		attempts := sd.Attempts + 1
		dead := attempts >= w.pol.MaxAttempts
		next := w.clock().Add(backoff(attempts, w.pol.BackoffBase, w.pol.BackoffCap))
		if rerr := w.docs.RecordEmbedFailure(ctx, d.ID, d.OwnerID, attempts, next, dead, err.Error()); rerr != nil {
			w.log.Warn("embed worker: record failure", "id", d.ID, "err", rerr)
			return false
		}
		w.log.Warn("embed worker: per-doc embed failure", "id", d.ID, "attempts", attempts, "dead", dead, "err", err)
		return true // skip this doc, keep going (no head-of-line block)
	}
	if err := w.docs.ReplaceChunks(ctx, d.ID, d.OwnerID, texts, vecs); err != nil {
		w.log.Warn("embed worker: replace chunks", "id", d.ID, "err", err)
		return false
	}
	return true
}
```

- [ ] **Step 6: Update existing worker tests + add behavior tests**

In `internal/worker/embed_worker_test.go`:

(a) Update both `NewEmbedWorker(...)` calls to pass a policy: `NewEmbedWorker(docs, emb, 0, 10, EmbedPolicy{}, slog.Default())`.

(b) Make `errDown` transient so `TestEmbedWorker_OllamaDown_LeavesStale` still asserts "stays stale" (Ollama-down is transient → no penalty, doc stays stale). Replace the `downErr` type/var with:

```go
var errDown = fmt.Errorf("ollama down: %w", ports.ErrEmbedTransient)
```
and add imports `"fmt"` and `"github.com/serverkraken/flow/internal/ports"`; delete the now-unused `downErr` type and its `Error()` method.

(c) Add behavior tests:

```go
func TestEmbedWorker_PerDocFailure_NoHeadOfLineBlock(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	emb := testutil.NewFakeEmbedder()
	// poison doc fails per-doc (NOT transient); healthy doc succeeds.
	emb.FailFunc = func(texts []string) error {
		for _, s := range texts {
			if strings.Contains(s, "POISON") {
				return fmt.Errorf("bad content")
			}
		}
		return nil
	}
	ctx := context.Background()
	old := time.Now().Add(-2 * time.Hour)
	newer := time.Now().Add(-1 * time.Hour)
	_, _ = docs.Create(ctx, domain.Document{ID: "poison", OwnerID: "u", Type: domain.DocFree, Path: "p", Title: "P", Body: "POISON body", UpdatedAt: old})
	_, _ = docs.Create(ctx, domain.Document{ID: "good", OwnerID: "u", Type: domain.DocFree, Path: "g", Title: "G", Body: "good body", UpdatedAt: newer})

	w := NewEmbedWorker(docs, emb, 0, 10, EmbedPolicy{}, slog.Default())
	w.drain(ctx)

	// healthy doc embedded despite the poison doc ahead of it
	stale, _ := docs.StaleDocuments(ctx, 10)
	for _, sd := range stale {
		if sd.Doc.ID == "good" {
			t.Fatalf("healthy doc must be embedded (poison must not block the queue)")
		}
	}
	// poison doc recorded a failure with attempts=1, not dead yet
	if s, _ := docs.EmbedStatus(ctx, "u", "poison"); s.State != domain.EmbedRetrying || s.Attempts != 1 {
		t.Fatalf("poison want retrying attempts=1, got %#v", s)
	}
}

func TestEmbedWorker_PerDocFailure_DeadLettersAtCap(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	emb := testutil.NewFakeEmbedder()
	emb.FailFunc = func(texts []string) error { return fmt.Errorf("always bad") }
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{ID: "x", OwnerID: "u", Type: domain.DocFree, Path: "x", Title: "X", Body: "b"})
	// pre-seed 4 prior failures (maxAttempts default 5) that are already due
	_ = docs.RecordEmbedFailure(ctx, "x", "u", 4, time.Now().Add(-time.Hour), false, "prev")

	w := NewEmbedWorker(docs, emb, 0, 10, EmbedPolicy{}, slog.Default())
	w.drain(ctx)

	if s, _ := docs.EmbedStatus(ctx, "u", "x"); s.State != domain.EmbedFailed || s.Attempts != 5 {
		t.Fatalf("want failed attempts=5, got %#v", s)
	}
}

func TestEmbedWorker_Transient_StopsDrain_NoPenalty(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	emb := testutil.NewFakeEmbedder()
	emb.FailFunc = func(texts []string) error { return fmt.Errorf("down: %w", ports.ErrEmbedTransient) }
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{ID: "a", OwnerID: "u", Type: domain.DocFree, Path: "a", Title: "A", Body: "b"})

	w := NewEmbedWorker(docs, emb, 0, 10, EmbedPolicy{}, slog.Default())
	w.drain(ctx)

	if s, _ := docs.EmbedStatus(ctx, "u", "a"); s.State != domain.EmbedPending {
		t.Fatalf("transient failure must NOT record a per-doc failure; want pending, got %v", s.State)
	}
}
```

Add imports `strings`, `time`, `fmt`, `ports` to the test file as needed.

- [ ] **Step 7: Wire the policy in main**

In `cmd/flow-server/main.go`, update the worker construction:

```go
	embedWorker := worker.NewEmbedWorker(documentStore, embedder, embedInterval, embedBatch, worker.EmbedPolicy{}, logger)
```

- [ ] **Step 8: Run worker tests + build**

Run:
```bash
go build ./...
go test ./internal/worker/ ./internal/testutil/ -v
```
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/worker/backoff.go internal/worker/backoff_test.go internal/worker/embed_worker.go \
        internal/worker/embed_worker_test.go internal/testutil/fakes.go cmd/flow-server/main.go
git commit -m "feat(worker): backoff + dead-letter, continue past per-doc failures (Säule B L2.4)" # (+ standard trailers)
```

---

### Task 7: `RetryEmbedding` + `GetEmbedStatus` use cases

**Files:**
- Create: `internal/usecase/retry_embedding.go`
- Create: `internal/usecase/embed_status.go`
- Test: `internal/usecase/embed_usecases_test.go`

**Interfaces:**
- Consumes: `ports.DocumentStore` (`Get`, `ClearEmbedFailure`, `EmbedStatus`), `ports.DocChangeNotifier`.
- Produces:
  - `usecase.RetryEmbedding{Docs ports.DocumentStore; Notifier ports.DocChangeNotifier}` with `Execute(ctx, ownerID, docID string) error`.
  - `usecase.GetEmbedStatus{Docs ports.DocumentStore}` with `Execute(ctx, ownerID, docID string) (domain.EmbedStatus, error)`.

- [ ] **Step 1: Write the failing tests**

Create `internal/usecase/embed_usecases_test.go`:

```go
package usecase_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

type spyNotifier struct{ n int }

func (s *spyNotifier) DocumentChanged() { s.n++ }

func TestRetryEmbedding_ClearsAndKicks(t *testing.T) {
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	_, _ = docs.Create(ctx, domain.Document{ID: "d", OwnerID: "u", Type: domain.DocFree, Path: "d", Title: "D", Body: "b"})
	_ = docs.RecordEmbedFailure(ctx, "d", "u", 5, time.Now(), true, "boom")
	spy := &spyNotifier{}

	uc := usecase.RetryEmbedding{Docs: docs, Notifier: spy}
	if err := uc.Execute(ctx, "u", "d"); err != nil {
		t.Fatal(err)
	}
	if spy.n != 1 {
		t.Fatalf("want notifier kicked once, got %d", spy.n)
	}
	if s, _ := docs.EmbedStatus(ctx, "u", "d"); s.State != domain.EmbedPending {
		t.Fatalf("after retry want pending, got %v", s.State)
	}
}

func TestRetryEmbedding_UnknownDoc(t *testing.T) {
	uc := usecase.RetryEmbedding{Docs: testutil.NewFakeDocumentStore(), Notifier: &spyNotifier{}}
	if err := uc.Execute(context.Background(), "u", "nope"); err == nil || !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Fatalf("want ErrDocumentNotFound, got %v", err)
	}
}

func TestGetEmbedStatus_PassThrough(t *testing.T) {
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	_, _ = docs.Create(ctx, domain.Document{ID: "d", OwnerID: "u", Type: domain.DocFree, Path: "d", Title: "D", Body: "b"})
	uc := usecase.GetEmbedStatus{Docs: docs}
	s, err := uc.Execute(ctx, "u", "d")
	if err != nil || s.State != domain.EmbedPending {
		t.Fatalf("want pending, got %v err=%v", s.State, err)
	}
}
```

Add imports `errors` and `time` to the test file.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/usecase/ -run 'RetryEmbedding|GetEmbedStatus' -v`
Expected: FAIL — use cases undefined.

- [ ] **Step 3: Implement the use cases**

Create `internal/usecase/retry_embedding.go`:

```go
// Package usecase — RetryEmbedding re-queues a document for embedding by
// clearing its recorded failure (incl. a dead-letter) and waking the worker.
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/ports"
)

// RetryEmbedding clears a document's embed-failure state and kicks the worker.
type RetryEmbedding struct {
	Docs     ports.DocumentStore
	Notifier ports.DocChangeNotifier // optional; nil → no kick
}

// Execute verifies ownership, clears the failure row, and notifies the worker.
func (uc RetryEmbedding) Execute(ctx context.Context, ownerID, docID string) error {
	if _, err := uc.Docs.Get(ctx, ownerID, docID); err != nil {
		return err // ErrDocumentNotFound for unknown/forbidden
	}
	if err := uc.Docs.ClearEmbedFailure(ctx, docID, ownerID); err != nil {
		return err
	}
	if uc.Notifier != nil {
		uc.Notifier.DocumentChanged()
	}
	return nil
}
```

Create `internal/usecase/embed_status.go`:

```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// GetEmbedStatus returns the owner-scoped embedding status of one document.
type GetEmbedStatus struct {
	Docs ports.DocumentStore
}

// Execute returns the document's embed status (ErrDocumentNotFound if unknown).
func (uc GetEmbedStatus) Execute(ctx context.Context, ownerID, docID string) (domain.EmbedStatus, error) {
	return uc.Docs.EmbedStatus(ctx, ownerID, docID)
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/usecase/ -run 'RetryEmbedding|GetEmbedStatus' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/retry_embedding.go internal/usecase/embed_status.go internal/usecase/embed_usecases_test.go
git commit -m "feat(usecase): RetryEmbedding + GetEmbedStatus (Säule B L2.6)" # (+ standard trailers)
```

---

### Task 8: WebUI — embed status badge + Retry route

**Files:**
- Modify: `internal/adapter/webui/docs.templ` (`DocDetail.Embed` field; `EmbedView` type; `EmbedBadge` component; render in `DocView`)
- Modify: `internal/adapter/httpserver/webui_docs.go` (`handleWebDocView` populates status; new `handleWebDocReembed`)
- Modify: `internal/adapter/httpserver/server.go` (Server field `RetryEmbedding`, `GetEmbedStatus`; mount route)
- Modify: `internal/adapter/httpserver/routes_test.go` (add `{"POST", "/docs/x/reembed"}`)
- Modify: `cmd/flow-server/main.go` (wire the two use cases)
- Test: `internal/adapter/httpserver/webui_docs_embed_test.go`

**Interfaces:**
- Consumes: `usecase.RetryEmbedding`, `usecase.GetEmbedStatus`, `webui.EmbedView`, `webui.EmbedBadge`.
- Produces: `POST /docs/{id}/reembed` (owner-scoped) returning the re-rendered badge fragment (HTMX) or a 303 redirect to `/docs/{id}`.

- [ ] **Step 1: Add the templ types + component**

In `internal/adapter/webui/docs.templ`, add the `Embed` field to `DocDetail`:

```go
type DocDetail struct {
	ID        string
	Type      string
	Path      string
	Title     string
	HTML      template.HTML
	Body      string
	Backlinks []DocRow
	Tags      []TagLink
	Embed     *EmbedView // embedding status (nil → not shown)
}
```

Add the view model + component (e.g. just below `DocDetail`):

```go
// EmbedView is the embedding-status chip shown on a document.
type EmbedView struct {
	State     string // ok | pending | retrying | failed
	LastError string // already truncated by the handler
	ShowRetry bool   // true when dead-lettered (manual retry offered)
}

templ EmbedBadge(docID string, e EmbedView) {
	<div class="mb-3 flex items-center gap-2 text-xs">
		switch e.State {
			case "ok":
				<span class="rounded-full bg-emerald-50 px-2 py-0.5 text-emerald-700">embedded</span>
			case "pending":
				<span class="rounded-full bg-slate-100 px-2 py-0.5 text-slate-500">embedding queued</span>
			case "retrying":
				<span class="rounded-full bg-amber-50 px-2 py-0.5 text-amber-700">embedding retrying</span>
			case "failed":
				<span class="rounded-full bg-rose-50 px-2 py-0.5 text-rose-700">embedding failed</span>
		}
		if e.LastError != "" {
			<span class="font-mono text-slate-400">{ e.LastError }</span>
		}
		if e.ShowRetry {
			<form hx-post={ "/docs/" + docID + "/reembed" } hx-target="closest div" hx-swap="outerHTML" class="inline">
				<button type="submit" class="rounded bg-slate-800 px-2 py-0.5 text-white hover:bg-slate-700">Retry</button>
			</form>
		}
	</div>
}
```

Render it in `templ DocView(...)` right after the Path line (`<div class="mb-3 font-mono text-xs text-slate-400">{ d.Current.Path }</div>`):

```go
	if d.Current.Embed != nil {
		@EmbedBadge(d.Current.ID, *d.Current.Embed)
	}
```

- [ ] **Step 2: Regenerate templ**

Run: `make generate`
Expected: `internal/adapter/webui/docs_templ.go` regenerated (will be committed in Step 7).

- [ ] **Step 3: Add Server fields + route + handler**

In `internal/adapter/httpserver/server.go`, add to the `Server` struct (near the other document use cases, ~lines 57-63):

```go
	RetryEmbedding usecase.RetryEmbedding
	GetEmbedStatus usecase.GetEmbedStatus
```

Mount the route in `Routes()` next to the other `/docs/{id}` handlers (after line 157):

```go
	mux.Handle("POST /docs/{id}/reembed", s.webAuth(http.HandlerFunc(s.handleWebDocReembed)))
```

In `internal/adapter/httpserver/webui_docs.go`, populate the status in `handleWebDocView` — just before building `d := webui.DocsPageData{...}` (after the `tagLinks` block), add:

```go
	var embedView *webui.EmbedView
	if st, serr := s.GetEmbedStatus.Execute(r.Context(), u.ID, id); serr == nil {
		embedView = &webui.EmbedView{
			State:     string(st.State),
			LastError: truncateError(st.LastError),
			ShowRetry: st.State == domain.EmbedFailed,
		}
	}
```

and set it on the detail: add `Embed: embedView,` to the `webui.DocDetail{...}` literal.

Add the new handler + helper at the end of `webui_docs.go`:

```go
func (s *Server) handleWebDocReembed(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	if err := s.RetryEmbedding.Execute(r.Context(), u.ID, id); err != nil {
		if errors.Is(err, ports.ErrDocumentNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if r.Header.Get("HX-Request") == "" {
		http.Redirect(w, r, "/docs/"+id, http.StatusSeeOther)
		return
	}
	// After a retry the doc is queued again.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.EmbedBadge(id, webui.EmbedView{State: "pending"}).Render(r.Context(), w)
}

// truncateError shortens an embed error for inline display.
func truncateError(s string) string {
	const max = 80
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
```

Confirm `webui_docs.go` imports `errors`, `github.com/serverkraken/flow/internal/ports`, and `github.com/serverkraken/flow/internal/domain` (the view handler already uses `domain`).

- [ ] **Step 4: Add the route to routes_test**

In `internal/adapter/httpserver/routes_test.go`, add to the expected list (after the `{"POST", "/docs/x/delete"}` entry):

```go
		{"POST", "/docs/x/reembed"},
```

- [ ] **Step 5: Wire the use cases in main**

In `cmd/flow-server/main.go`, in the `Server{...}` literal (near the other document use cases, ~lines 137-145), add:

```go
		RetryEmbedding:    usecase.RetryEmbedding{Docs: documentStore, Notifier: embedWorker},
		GetEmbedStatus:    usecase.GetEmbedStatus{Docs: documentStore},
```

- [ ] **Step 6: Write + run the handler test**

Create `internal/adapter/httpserver/webui_docs_embed_test.go`. Follow the existing webui handler-test pattern in this package (construct a `Server` with fakes + a logged-in session; if a helper like `newTestServer`/`webRequest` exists, reuse it — otherwise mirror an existing `webui_docs` test). Assert:

```go
// 1) a failed doc renders the Retry control on GET /docs/{id}
// 2) POST /docs/{id}/reembed with HX-Request returns 200 and the "embedding queued" fragment
// 3) POST /docs/{id}/reembed for an unknown id returns 404
```

Use the fakes: seed a doc, `docs.RecordEmbedFailure(ctx, id, "u", 5, time.Now(), true, "boom")`, wire `RetryEmbedding{Docs: docs, Notifier: <nil or spy>}` and `GetEmbedStatus{Docs: docs}` onto the test `Server`.

Run: `go test ./internal/adapter/httpserver/ -run 'Reembed|Embed|Routes' -v`
Expected: PASS.

- [ ] **Step 7: Commit (incl. regenerated templ)**

```bash
git add internal/adapter/webui/docs.templ internal/adapter/webui/docs_templ.go \
        internal/adapter/httpserver/webui_docs.go internal/adapter/httpserver/server.go \
        internal/adapter/httpserver/routes_test.go internal/adapter/httpserver/webui_docs_embed_test.go \
        cmd/flow-server/main.go
git commit -m "feat(webui): embed status badge + POST /docs/{id}/reembed retry (Säule B L2.6)" # (+ standard trailers)
```

---

### Task 9: Wiring verification + done-gate

**Files:**
- Verify only (no new code unless a gap is found).

**Interfaces:**
- Consumes: everything above.

- [ ] **Step 1: Full CI**

Run: `make ci`
Expected: PASS — `lint` clean (ST1005 ok), `verify-generate` clean (templ up to date), `cover` ≥ 80%, `build` ok. If coverage dipped below 80%, add a focused test to the lowest-covered new file (e.g. `EmbedStatus` derivation branches, `truncateError`) and re-run.

- [ ] **Step 2: Main-wiring audit (per `feedback_plan_main_wiring_task`)**

Run:
```bash
rg -n "EmbedPolicy|RetryEmbedding|GetEmbedStatus|reembed" cmd/flow-server/main.go internal/adapter/httpserver/server.go
```
Confirm: the worker is built with `worker.EmbedPolicy{}`; the `Server` literal sets both `RetryEmbedding` and `GetEmbedStatus`; `POST /docs/{id}/reembed` is mounted. All three must be present.

- [ ] **Step 3: Build the server binary + route smoke**

Run:
```bash
go build -o /tmp/flow-server ./cmd/flow-server && echo build-ok
rg -n '"POST", "/docs/x/reembed"' internal/adapter/httpserver/routes_test.go
go test ./internal/adapter/httpserver/ -run Routes -v
```
Expected: build-ok; the routes test (which enumerates every mounted route) passes with the new entry.

- [ ] **Step 4: Live done-gate (manual, against the dev stack — see `reference_flow_dev_env`)**

Document the outcome in the final commit message or a follow-up note:

1. `make dev-up`, `make dev-run` (FLOW_DEV=1), log in.
2. **Layer 1 proof (calls now go through):** create a doc with an **empty title** and a body containing a long run of spaces between two words; create a second, large doc (paste a few hundred lines). Wait one embed interval. Both become searchable via semantic search (`?q=` with a semantic term) — neither stalls. Previously the empty-title/whitespace doc would have thrown a vector-count mismatch and the large doc could time out.
3. **Layer 2 proof (resilience):** `docker stop` the Ollama container (or point `FLOW_OLLAMA_HOST` at a dead port), create a doc → server logs show no storm and the worker does not wedge; restart Ollama → the doc embeds on the next tick.
4. **WebUI retry:** for any doc, confirm the status chip renders; (dead-letter is covered by unit/pgstore/httpserver tests) confirm the Retry button on a `failed` doc posts to `/docs/{id}/reembed` and the chip swaps to "embedding queued".

- [ ] **Step 5: Final commit / branch note**

```bash
git commit --allow-empty -m "chore(flow-embed): Säule B done-gate verified (CI green, wiring audited, live dogfood)" # (+ standard trailers)
```

(Deploy — `:rebuild` image build → homelab-study digest bump → ArgoCD sync — is shared with Säule A and tracked separately, not part of this branch's code.)

---

## Self-Review

**Spec coverage:**
- §3.1 empty-chunk filter → Task 1. §3.2 sub-batching → Task 2. §3.7 classification + §3.4 `ErrEmbedTransient` → Task 3. §3.3 migration + §3.4 `StaleDoc`/sig → Task 4. §3.5 `EmbedStatus` + §3.8 pgstore methods + `ReplaceChunks` clear → Task 5. §3.6 drain/backoff/policy → Task 6. §3.9 `RetryEmbedding` (+ status read) → Task 7. §3.9 WebUI badge + route + §3.10 wiring → Task 8. §6 done-gate → Task 9. All spec sections map to a task.
- `domain.EmbedState` constants used in Tasks 5–8 are defined once (Task 5 Step 1).
- `Execute` (not `Exec`) method name matches the codebase convention (`GetDocument.Execute`) — used consistently in Tasks 7–8.
- `EmbedPolicy` ctor arg order `(docs, e, interval, batch, pol, log)` is identical in the constructor (Task 6 Step 5), the test call sites (Task 6 Step 6a), and `main.go` (Task 6 Step 7, Task 8 Step 5).
- `StaleDoc{Doc, Attempts}` field names are consistent across ports (Task 4), pgstore scan (Task 4), fake (Tasks 4–5), and worker (Task 6).

**Placeholder scan:** No "TBD"/"add error handling"/"similar to". The one prose-only step is Task 8 Step 6 (handler test), which defers to the package's existing test harness rather than inventing a `Server` constructor that may not match — it lists exact assertions and the exact fake calls to make; this is intentional, not a placeholder.

**Type consistency:** `ReplaceChunks`, `RecordEmbedFailure`, `ClearEmbedFailure`, `EmbedStatus` signatures match between ports (Tasks 4–5), pgstore impls, and fakes. `webui.EmbedView{State, LastError, ShowRetry}` and `webui.EmbedBadge(docID, EmbedView)` match between the templ (Task 8 Step 1) and the handler (Task 8 Step 3).
