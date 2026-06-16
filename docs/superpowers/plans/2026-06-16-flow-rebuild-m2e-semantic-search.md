# flow rebuild M2e — Semantic Search Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Meaning-based document recall (e.g. "Strandferien" finds "Urlaub am Meer") fused with M2d's keyword search on the same `?q=` box, via local Ollama embeddings + pgvector.

**Architecture:** Documents are chunked and embedded asynchronously by a background worker (Ollama `nomic-embed-text`, 768-dim) into a `document_chunks` table (pgvector, HNSW). At query time the `SearchDocuments` use case runs the keyword arm (M2d) and a vector arm (pgvector ANN over chunks) in parallel and fuses them with Reciprocal Rank Fusion (k=60). If Ollama is unreachable the vector arm is skipped and the search degrades to keyword-only, so the app always works.

**Tech Stack:** Go, pgx/v5, Postgres 16 + pgvector, Ollama, charm.land/bubbletea/v2 (TUI), templ (WebUI).

**Spec:** `docs/superpowers/specs/2026-06-16-flow-rebuild-m2e-semantic-search-design.md`

---

## File map

| File | Responsibility | Action |
|------|----------------|--------|
| `internal/adapter/pgstore/migrations/0010_document_chunks.sql` | vector ext + `document_chunks` + HNSW + `chunks_hash` | Create |
| `internal/domain/search.go` | `SemanticHit` type | Modify |
| `internal/domain/search_test.go` | `SemanticHit` test | Modify |
| `internal/chunk/chunk.go` | pure character-window chunker | Create |
| `internal/chunk/chunk_test.go` | chunker tests | Create |
| `internal/usecase/rrf.go` | pure RRF fusion | Create |
| `internal/usecase/rrf_test.go` | RRF tests | Create |
| `internal/ports/ports.go` | `Embedder`, `DocChangeNotifier`, `DocumentStore.{StaleDocuments,ReplaceChunks,SemanticSearch}` | Modify |
| `internal/adapter/embed/ollama.go` | Ollama Embedder adapter | Create |
| `internal/adapter/embed/ollama_test.go` | adapter test (httptest) | Create |
| `internal/testutil/fakes.go` | `FakeEmbedder`, fake store chunk/semantic methods, fake notifier | Modify |
| `internal/testutil/fakes_test.go` | fake embedder/semantic tests | Modify |
| `internal/adapter/pgstore/documents.go` | `StaleDocuments`, `ReplaceChunks`, `SemanticSearch`, `vectorLiteral` | Modify |
| `internal/adapter/pgstore/documents_test.go` | DB-gated semantic test | Modify |
| `internal/worker/embed_worker.go` | background embed worker (ticker + kick) | Create |
| `internal/worker/embed_worker_test.go` | worker tests | Create |
| `internal/usecase/search_documents.go` | hybrid fusion + degrade | Modify |
| `internal/usecase/document_test.go` | fusion/degrade tests | Modify |
| `internal/usecase/create_document.go`, `update_document.go` | notify on write | Modify |
| `cmd/flow-server/main.go` | wire Embedder + worker; start/stop | Modify |
| `deploy/dev/compose.yml`, `scripts/dev-up.sh`, `deploy/dev/flow.env` | dev Ollama + model pull + env | Modify |

Tasks are ordered so the build stays green after each. Reminder ([[feedback_pgstore_goose_migrations]]): every migration needs goose `-- +goose Up`/`-- +goose Down`.

---

## Task 1: Migration 0010 — pgvector + document_chunks + chunks_hash

**Files:**
- Create: `internal/adapter/pgstore/migrations/0010_document_chunks.sql`

- [ ] **Step 1: Write the migration** (goose-annotated, idempotent Down)

```sql
-- +goose Up
CREATE EXTENSION IF NOT EXISTS vector;

ALTER TABLE documents ADD COLUMN chunks_hash text;

CREATE TABLE document_chunks (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    owner_id    text NOT NULL,
    chunk_index int  NOT NULL,
    content     text NOT NULL,
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

Before writing, read migration `0009_documents_search.sql` to confirm the goose annotation format, and confirm `documents.id` is `uuid` (so the FK type matches). `gen_random_uuid()` is built into Postgres 13+ (no pgcrypto needed).

- [ ] **Step 2: Build to confirm embedding compiles**

Run: `go build ./internal/adapter/pgstore/`
Expected: success (migrations are `//go:embed migrations/*.sql`; the new file is globbed automatically).

- [ ] **Step 3: Commit**

```bash
git add internal/adapter/pgstore/migrations/0010_document_chunks.sql
git commit -m "feat(m2e): migration 0010 pgvector + document_chunks + chunks_hash"
```

---

## Task 2: Domain — SemanticHit

**Files:**
- Modify: `internal/domain/search.go`
- Test: `internal/domain/search_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/domain/search_test.go`:

```go
func TestSemanticHit_EmbedsDocument(t *testing.T) {
	h := SemanticHit{
		Document: Document{ID: "a", Type: DocFree, Path: "p", Title: "T"},
		Snippet:  "best chunk text",
		Distance: 0.25,
	}
	if h.ID != "a" || h.Snippet != "best chunk text" || h.Distance != 0.25 {
		t.Fatalf("unexpected: %#v", h)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/domain/ -run SemanticHit -v`
Expected: FAIL — `undefined: SemanticHit`.

- [ ] **Step 3: Implement**

Add to `internal/domain/search.go` (below `SearchHit`):

```go
// SemanticHit is a document matched by vector similarity. The Document is
// embedded for ergonomic field access (.ID/.Title). Snippet is the text of the
// document's best-matching chunk; Distance is the cosine distance (smaller =
// closer). Internal type — not serialized to the wire (fusion turns it into a
// SearchHit).
type SemanticHit struct {
	Document
	Snippet  string
	Distance float32
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/domain/ -run SemanticHit -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/search.go internal/domain/search_test.go
git commit -m "feat(m2e): domain SemanticHit type"
```

---

## Task 3: Chunker — pure character-window splitter

**Files:**
- Create: `internal/chunk/chunk.go`
- Test: `internal/chunk/chunk_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/chunk/chunk_test.go`:

```go
package chunk

import (
	"strings"
	"testing"
)

func TestSplit_EmptyBody_TitleOnly(t *testing.T) {
	got := Split("Title", "")
	if len(got) != 1 || got[0] != "Title" {
		t.Fatalf("got %#v, want [\"Title\"]", got)
	}
}

func TestSplit_EmptyBoth_Nil(t *testing.T) {
	if got := Split("", ""); got != nil {
		t.Fatalf("got %#v, want nil", got)
	}
}

func TestSplit_ShortBody_SingleChunkWithTitle(t *testing.T) {
	got := Split("Subj", "a short body")
	if len(got) != 1 {
		t.Fatalf("want 1 chunk, got %d: %#v", len(got), got)
	}
	if !strings.HasPrefix(got[0], "Subj\n\n") || !strings.Contains(got[0], "a short body") {
		t.Fatalf("chunk missing title prefix or body: %q", got[0])
	}
}

func TestSplit_LongBody_MultipleOverlappingChunks(t *testing.T) {
	body := strings.Repeat("x", MaxChars*2)
	got := Split("T", body)
	if len(got) < 2 {
		t.Fatalf("want multiple chunks, got %d", len(got))
	}
	// every chunk carries the title
	for i, c := range got {
		if !strings.HasPrefix(c, "T\n\n") {
			t.Fatalf("chunk %d missing title prefix: %q", i, c[:min(20, len(c))])
		}
	}
	// each non-final window is MaxChars of body; the step is MaxChars-OverlapChars,
	// so there are ceil((2*MaxChars - MaxChars)/step)+1 windows — just assert >= 2 and
	// that consecutive windows overlap (share body characters).
}

func min(a, b int) int { if a < b { return a }; return b }
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/chunk/ -v`
Expected: FAIL — `undefined: Split` / `MaxChars`.

- [ ] **Step 3: Implement**

Create `internal/chunk/chunk.go`:

```go
// Package chunk splits a document into overlapping character windows for
// embedding. It is intentionally simple (character windows, not paragraph-aware):
// embedding recall tolerates mid-sentence cuts, and a deterministic window with
// fixed overlap is easy to reason about and test. Paragraph-aware packing is a
// possible future refinement.
package chunk

import "strings"

const (
	// MaxChars is the window size (~500 tokens at ~4 chars/token).
	MaxChars = 2000
	// OverlapChars is carried between consecutive windows (~15%).
	OverlapChars = 300
)

// Split returns the embeddable chunk texts for a document. The (trimmed) title is
// prepended to every chunk so each carries the document's subject. An empty body
// yields a single title-only chunk; an empty title+body yields nil. Deterministic,
// no I/O.
func Split(title, body string) []string {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if body == "" {
		if title == "" {
			return nil
		}
		return []string{title}
	}
	r := []rune(body)
	step := MaxChars - OverlapChars
	var out []string
	for start := 0; start < len(r); start += step {
		end := start + MaxChars
		if end > len(r) {
			end = len(r)
		}
		w := strings.TrimSpace(string(r[start:end]))
		if title != "" {
			w = title + "\n\n" + w
		}
		out = append(out, w)
		if end == len(r) {
			break
		}
	}
	return out
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/chunk/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/chunk/chunk.go internal/chunk/chunk_test.go
git commit -m "feat(m2e): pure character-window document chunker"
```

---

## Task 4: RRF — pure Reciprocal Rank Fusion

**Files:**
- Create: `internal/usecase/rrf.go`
- Test: `internal/usecase/rrf_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/usecase/rrf_test.go` (match the file's package — if the package's tests are `package usecase` internal, use that; this needs the unexported `rrfFuse`, so it MUST be `package usecase`):

```go
package usecase

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func kw(id string) domain.SearchHit {
	return domain.SearchHit{Document: domain.Document{ID: id}, Snippet: "kw-" + id}
}
func sem(id string) domain.SemanticHit {
	return domain.SemanticHit{Document: domain.Document{ID: id}, Snippet: "sem-" + id}
}

func TestRRFFuse_UnionAndOrder(t *testing.T) {
	keyword := []domain.SearchHit{kw("a"), kw("b")}
	semantic := []domain.SemanticHit{sem("b"), sem("c")}
	out := rrfFuse(keyword, semantic, 60)
	// union of {a,b,c}; b appears in both arms so ranks highest.
	if len(out) != 3 {
		t.Fatalf("want 3, got %d: %#v", len(out), out)
	}
	if out[0].ID != "b" {
		t.Fatalf("want b first (in both arms), got %q", out[0].ID)
	}
	ids := map[string]bool{}
	for _, h := range out {
		ids[h.ID] = true
	}
	if !ids["a"] || !ids["b"] || !ids["c"] {
		t.Fatalf("missing ids: %#v", ids)
	}
}

func TestRRFFuse_KeywordSnippetWins(t *testing.T) {
	out := rrfFuse([]domain.SearchHit{kw("a")}, []domain.SemanticHit{sem("a")}, 60)
	if len(out) != 1 || out[0].Snippet != "kw-a" {
		t.Fatalf("keyword snippet should win: %#v", out)
	}
}

func TestRRFFuse_SemanticOnlyUsesChunkSnippet(t *testing.T) {
	out := rrfFuse(nil, []domain.SemanticHit{sem("z")}, 60)
	if len(out) != 1 || out[0].Snippet != "sem-z" {
		t.Fatalf("semantic-only snippet wrong: %#v", out)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/usecase/ -run RRFFuse -v`
Expected: FAIL — `undefined: rrfFuse`.

- [ ] **Step 3: Implement**

Create `internal/usecase/rrf.go`:

```go
package usecase

import (
	"sort"

	"github.com/serverkraken/flow/internal/domain"
)

// rrfK is the Reciprocal Rank Fusion constant (standard default).
const rrfK = 60

// rrfFuse merges the keyword and semantic ranked lists into one ranking. A
// document's score is the sum, over the arms it appears in, of 1/(k+rank) with a
// 1-based rank. Keyword hits are added first, so a document present in both arms
// keeps its highlighted keyword snippet; a document seen only in the semantic arm
// keeps its chunk snippet. Ties break by first-seen order (stable).
func rrfFuse(keyword []domain.SearchHit, semantic []domain.SemanticHit, k int) []domain.SearchHit {
	type agg struct {
		hit   domain.SearchHit
		score float64
		order int
	}
	m := map[string]*agg{}
	order := 0
	add := func(id string, hit domain.SearchHit, rank int) {
		a, ok := m[id]
		if !ok {
			a = &agg{hit: hit, order: order}
			order++
			m[id] = a
		}
		a.score += 1.0 / float64(k+rank)
	}
	for i, h := range keyword {
		add(h.ID, h, i+1)
	}
	for i, h := range semantic {
		add(h.ID, domain.SearchHit{Document: h.Document, Snippet: h.Snippet}, i+1)
	}
	out := make([]*agg, 0, len(m))
	for _, a := range m {
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].order < out[j].order
	})
	res := make([]domain.SearchHit, len(out))
	for i, a := range out {
		res[i] = a.hit
	}
	return res
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/usecase/ -run RRFFuse -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/rrf.go internal/usecase/rrf_test.go
git commit -m "feat(m2e): pure RRF fusion"
```

---

## Task 5: Embedder port + fake embedder

**Files:**
- Modify: `internal/ports/ports.go`
- Modify: `internal/testutil/fakes.go`
- Test: `internal/testutil/fakes_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/testutil/fakes_test.go`:

```go
func TestFakeEmbedder_DeterministicAndError(t *testing.T) {
	e := NewFakeEmbedder()
	a, err := e.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 2 || len(a[0]) != e.Dim {
		t.Fatalf("shape wrong: %d vecs, dim %d", len(a), len(a[0]))
	}
	b, _ := e.Embed(context.Background(), []string{"hello"})
	for i := range a[0] {
		if a[0][i] != b[0][i] {
			t.Fatalf("not deterministic at %d", i)
		}
	}
	e.Err = errTest
	if _, err := e.Embed(context.Background(), []string{"x"}); err == nil {
		t.Fatal("expected error when Err set")
	}
}

var errTest = errors.New("boom")
```

(Ensure `context`, `errors` are imported in the test file.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/testutil/ -run FakeEmbedder -v`
Expected: FAIL — `undefined: NewFakeEmbedder`.

- [ ] **Step 3: Add the port**

In `internal/ports/ports.go`, add (near the other ports; `context` already imported):

```go
// Embedder turns texts into embedding vectors (one per input, order-preserving).
// Implementations are batched. A non-nil error means the backend (e.g. Ollama) is
// unavailable; callers degrade gracefully.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}
```

- [ ] **Step 4: Implement the fake**

In `internal/testutil/fakes.go`, add (add `crypto/sha256` and `math` to imports):

```go
// FakeEmbedder returns deterministic unit vectors derived from a hash of each
// text — identical text yields an identical vector, so similarity *ordering* is
// reproducible without a real model. It does NOT model semantic nearness; tests
// assert wiring/ordering, not embedding quality. Set Err to simulate Ollama down.
type FakeEmbedder struct {
	Dim int
	Err error
}

func NewFakeEmbedder() *FakeEmbedder { return &FakeEmbedder{Dim: 768} }

func (f *FakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = pseudoVec(t, f.Dim)
	}
	return out, nil
}

func pseudoVec(s string, dim int) []float32 {
	h := sha256.Sum256([]byte(s))
	v := make([]float32, dim)
	var norm float64
	for i := 0; i < dim; i++ {
		x := float32(int(h[i%len(h)])-128) / 128.0
		x += float32((i*2654435761)%1000) / 1000.0
		v[i] = x
		norm += float64(x) * float64(x)
	}
	if n := float32(math.Sqrt(norm)); n > 0 {
		for i := range v {
			v[i] /= n
		}
	}
	return v
}
```

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./internal/testutil/ -run FakeEmbedder -v && go build ./...`
Expected: PASS + clean.

- [ ] **Step 6: Commit**

```bash
git add internal/ports/ports.go internal/testutil/fakes.go internal/testutil/fakes_test.go
git commit -m "feat(m2e): Embedder port + deterministic fake embedder"
```

---

## Task 6: Ollama embedder adapter

**Files:**
- Create: `internal/adapter/embed/ollama.go`
- Test: `internal/adapter/embed/ollama_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/adapter/embed/ollama_test.go`:

```go
package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOllama_Embed_OK(t *testing.T) {
	var gotModel string
	var gotInput []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotModel, gotInput = req.Model, req.Input
		_, _ = w.Write([]byte(`{"embeddings":[[0.1,0.2,0.3],[0.4,0.5,0.6]]}`))
	}))
	defer srv.Close()

	o := NewOllama(srv.URL, "nomic-embed-text")
	vecs, err := o.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if gotModel != "nomic-embed-text" || len(gotInput) != 2 {
		t.Fatalf("request wrong: model=%q input=%v", gotModel, gotInput)
	}
	if len(vecs) != 2 || len(vecs[0]) != 3 || vecs[1][2] != 0.6 {
		t.Fatalf("decode wrong: %#v", vecs)
	}
}

func TestOllama_Embed_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "model not found", http.StatusNotFound)
	}))
	defer srv.Close()
	o := NewOllama(srv.URL, "x")
	if _, err := o.Embed(context.Background(), []string{"a"}); err == nil {
		t.Fatal("expected error on non-200")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/adapter/embed/ -v`
Expected: FAIL — `undefined: NewOllama`.

- [ ] **Step 3: Implement**

Create `internal/adapter/embed/ollama.go`:

```go
// Package embed provides the Ollama implementation of ports.Embedder.
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Ollama calls a local Ollama server's /api/embed endpoint.
type Ollama struct {
	host   string
	model  string
	client *http.Client
}

// NewOllama returns an Ollama embedder. Empty host/model fall back to the
// localhost default and nomic-embed-text.
func NewOllama(host, model string) *Ollama {
	if host == "" {
		host = "http://localhost:11434"
	}
	if model == "" {
		model = "nomic-embed-text"
	}
	return &Ollama{
		host:   strings.TrimRight(host, "/"),
		model:  model,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

type embedReq struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResp struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// Embed implements ports.Embedder.
func (o *Ollama) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
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
	defer resp.Body.Close()
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

- [ ] **Step 4: Run + build**

Run: `go test ./internal/adapter/embed/ -v && go build ./...`
Expected: PASS + clean.

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/embed/ollama.go internal/adapter/embed/ollama_test.go
git commit -m "feat(m2e): Ollama embedder adapter"
```

---

## Task 7: Store — StaleDocuments, ReplaceChunks, SemanticSearch

**Files:**
- Modify: `internal/ports/ports.go` (DocumentStore interface)
- Modify: `internal/testutil/fakes.go` (fake store methods)
- Modify: `internal/adapter/pgstore/documents.go` (pgstore methods + vectorLiteral)
- Test: `internal/testutil/fakes_test.go`
- Test: `internal/adapter/pgstore/documents_test.go`

Before coding, read `internal/adapter/pgstore/documents.go` for `prefixedDocCols`, `docCols`, the existing `scanDocument` column order, the `rowScanner` interface, and how `Search` builds dynamic args; and `internal/testutil/fakes.go` for `FakeDocumentStore`'s real field names (`mu`, `m`) and the `hasAllTags` helper. Read a DB-gated test (e.g. `TestDocumentStore_SearchFuzzyAndTag`) for the real pool-setup idiom (`startPG(t)` returns a DSN; `pgstore.NewPool` + `pgstore.Migrate`).

- [ ] **Step 1: Write the failing fake test**

Add to `internal/testutil/fakes_test.go`:

```go
func TestFakeStore_ChunksAndSemantic(t *testing.T) {
	s := NewFakeDocumentStore()
	e := NewFakeEmbedder()
	ctx := context.Background()
	mk := func(id, title, body string, tags ...string) domain.Document {
		d, err := s.Create(ctx, domain.Document{ID: id, OwnerID: "u", Type: domain.DocFree, Path: id, Title: title, Body: body, Tags: tags})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	a := mk("a", "Alpha", "alpha body", "go")
	mk("b", "Beta", "beta body")

	// freshly created docs are stale (no chunks yet)
	stale, err := s.StaleDocuments(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 2 {
		t.Fatalf("want 2 stale, got %d", len(stale))
	}

	// embed + store chunks for a
	texts := []string{"Alpha\n\nalpha body"}
	vecs, _ := e.Embed(ctx, texts)
	if err := s.ReplaceChunks(ctx, a.ID, a.OwnerID, texts, vecs); err != nil {
		t.Fatal(err)
	}
	// a is no longer stale; b still is
	stale, _ = s.StaleDocuments(ctx, 10)
	if len(stale) != 1 || stale[0].ID != "b" {
		t.Fatalf("want only b stale, got %#v", stale)
	}

	// semantic search: query with a's exact chunk vector → a is returned
	hits, err := s.SemanticSearch(ctx, "u", vecs[0], nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ID != "a" || hits[0].Snippet == "" {
		t.Fatalf("semantic want [a] with snippet, got %#v", hits)
	}
	// tag filter composes
	none, _ := s.SemanticSearch(ctx, "u", vecs[0], []string{"missing"}, 10)
	if len(none) != 0 {
		t.Fatalf("tag-filtered semantic want 0, got %d", len(none))
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/testutil/ -run ChunksAndSemantic -v`
Expected: FAIL — `s.StaleDocuments` undefined.

- [ ] **Step 3: Add the port methods**

In `internal/ports/ports.go`, add to the `DocumentStore` interface (after `Search`):

```go
	// StaleDocuments returns up to limit documents whose chunks are missing or
	// out of date (chunks_hash != md5(title||body)), across all owners, for the
	// embedding worker.
	StaleDocuments(ctx context.Context, limit int) ([]domain.Document, error)

	// ReplaceChunks atomically replaces a document's chunks with the given
	// (content, embedding) pairs (len-equal, may be empty) and stamps chunks_hash
	// so the document is no longer stale.
	ReplaceChunks(ctx context.Context, docID, ownerID string, contents []string, embeddings [][]float32) error

	// SemanticSearch returns the owner's documents whose chunks are nearest to the
	// query vector (cosine), best chunk per document, optionally AND-filtered by
	// tags, each with that chunk's text as Snippet. Ordered nearest-first.
	SemanticSearch(ctx context.Context, ownerID string, query []float32, tags []string, limit int) ([]domain.SemanticHit, error)
```

- [ ] **Step 4: Implement the fake**

In `internal/testutil/fakes.go`, add a per-doc chunk store to `FakeDocumentStore`. Add fields to the struct (next to its existing `m` map):

```go
	chunks     map[string][]fakeChunk // docID -> chunks
	chunksHash map[string]string      // docID -> stamped md5
```

Initialize both maps in `NewFakeDocumentStore` (alongside the existing map init). Then add:

```go
type fakeChunk struct {
	content string
	emb     []float32
}

func fakeDocHash(d domain.Document) string {
	sum := sha256.Sum256([]byte(d.Title + d.Body))
	return string(sum[:])
}

func (s *FakeDocumentStore) StaleDocuments(_ context.Context, limit int) ([]domain.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Document
	for _, d := range s.m {
		if s.chunksHash[d.ID] != fakeDocHash(d) {
			out = append(out, d)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (s *FakeDocumentStore) ReplaceChunks(_ context.Context, docID, _ string, contents []string, embeddings [][]float32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cs := make([]fakeChunk, len(contents))
	for i := range contents {
		cs[i] = fakeChunk{content: contents[i], emb: embeddings[i]}
	}
	s.chunks[docID] = cs
	if d, ok := s.m[docID]; ok {
		s.chunksHash[docID] = fakeDocHash(d)
	}
	return nil
}

func (s *FakeDocumentStore) SemanticSearch(_ context.Context, ownerID string, query []float32, tags []string, limit int) ([]domain.SemanticHit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var hits []domain.SemanticHit
	for _, d := range s.m {
		if d.OwnerID != ownerID || !hasAllTags(d.Tags, tags) {
			continue
		}
		best := -1.0
		bestContent := ""
		for _, c := range s.chunks[d.ID] {
			sim := cosine(query, c.emb)
			if sim > best {
				best = sim
				bestContent = c.content
			}
		}
		if bestContent == "" {
			continue // no chunks for this doc
		}
		hits = append(hits, domain.SemanticHit{Document: d, Snippet: bestContent, Distance: float32(1 - best)})
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Distance < hits[j].Distance })
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func cosine(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return -1
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return -1
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
```

(Add `sort` to imports if missing; `sha256` and `math` were added in Task 5.)

- [ ] **Step 5: Implement pgstore methods + vectorLiteral**

In `internal/adapter/pgstore/documents.go`, add (`strconv` to imports). `prefixedDocCols`/`scanDocument` column order must match exactly — read them first.

```go
// vectorLiteral formats a vector as a pgvector text literal ("[1,2,3]") for a
// $N::vector bind — no extra dependency needed.
func vectorLiteral(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(x), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

func (s *DocumentStore) StaleDocuments(ctx context.Context, limit int) ([]domain.Document, error) {
	q := `SELECT ` + docCols + ` FROM documents
WHERE chunks_hash IS DISTINCT FROM md5(coalesce(title,'')||coalesce(body,''))
ORDER BY updated_at ASC
LIMIT $1`
	rows, err := s.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("pgstore: stale documents: %w", err)
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

func (s *DocumentStore) ReplaceChunks(ctx context.Context, docID, ownerID string, contents []string, embeddings [][]float32) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pgstore: replace chunks begin: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM document_chunks WHERE document_id = $1`, docID); err != nil {
		return fmt.Errorf("pgstore: delete chunks: %w", err)
	}
	for i := range contents {
		if _, err := tx.Exec(ctx,
			`INSERT INTO document_chunks (document_id, owner_id, chunk_index, content, embedding)
			 VALUES ($1,$2,$3,$4,$5::vector)`,
			docID, ownerID, i, contents[i], vectorLiteral(embeddings[i])); err != nil {
			return fmt.Errorf("pgstore: insert chunk: %w", err)
		}
	}
	if _, err := tx.Exec(ctx,
		`UPDATE documents SET chunks_hash = md5(coalesce(title,'')||coalesce(body,'')) WHERE id = $1`,
		docID); err != nil {
		return fmt.Errorf("pgstore: stamp chunks_hash: %w", err)
	}
	return tx.Commit(ctx)
}

func (s *DocumentStore) SemanticSearch(ctx context.Context, ownerID string, query []float32, tags []string, limit int) ([]domain.SemanticHit, error) {
	q := `SELECT ` + prefixedDocCols + `, x.content, x.dist
FROM (
  SELECT DISTINCT ON (c.document_id) c.document_id AS did, c.content AS content,
         (c.embedding <=> $2::vector) AS dist
  FROM document_chunks c
  WHERE c.owner_id = $1
  ORDER BY c.document_id, dist
) x
JOIN documents d ON d.id = x.did`
	args := []any{ownerID, vectorLiteral(query)}
	if len(tags) > 0 {
		q += `
WHERE d.tags @> $3
ORDER BY x.dist
LIMIT $4`
		args = append(args, tags, limit)
	} else {
		q += `
ORDER BY x.dist
LIMIT $3`
		args = append(args, limit)
	}
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("pgstore: semantic search: %w", err)
	}
	defer rows.Close()
	var out []domain.SemanticHit
	for rows.Next() {
		h, err := scanSemanticHit(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// scanSemanticHit scans prefixedDocCols + a trailing content + dist column.
func scanSemanticHit(r rowScanner) (domain.SemanticHit, error) {
	var d domain.Document
	var typ string
	var extra []byte
	var content string
	var dist float32
	if err := r.Scan(&d.ID, &d.OwnerID, &d.ProjectID, &typ, &d.Path, &d.Title, &d.Body,
		&d.Tags, &d.Date, &d.Role, &extra, &d.CreatedAt, &d.UpdatedAt, &content, &dist); err != nil {
		return domain.SemanticHit{}, fmt.Errorf("pgstore: scan semantic hit: %w", err)
	}
	d.Type = domain.DocumentType(typ)
	if len(extra) > 0 {
		if err := json.Unmarshal(extra, &d.Extra); err != nil {
			return domain.SemanticHit{}, fmt.Errorf("pgstore: unmarshal extra: %w", err)
		}
	}
	return domain.SemanticHit{Document: d, Snippet: content, Distance: dist}, nil
}
```

CRITICAL: the `scanSemanticHit` Scan order must match `prefixedDocCols` exactly (copy from `scanDocument`/`scanSearchHit`), with `&content, &dist` appended to match the trailing `x.content, x.dist` SELECT columns.

- [ ] **Step 6: Add the DB-gated pgstore test**

In `internal/adapter/pgstore/documents_test.go`, mirror the existing DB-gated setup. **Copy the exact pool-setup preamble from `TestDocumentStore_SearchFuzzyAndTag`** (the real sequence is `dsn := startPG(t)` → `pool, _ := pgstore.NewPool(ctx, dsn)` → `pgstore.Migrate(ctx, pool)` — use whatever that test actually does, do not invent helper names). Use a small deterministic embedding so the assertion is stable:

```go
func TestDocumentStore_SemanticSearch(t *testing.T) {
	// --- copy the real pool setup from TestDocumentStore_SearchFuzzyAndTag here ---
	// (yields a *DocumentStore named s and a context ctx)
	owner := "sem-owner"

	// helper to make a 768-dim vector that is all v
	vec := func(v float32) []float32 {
		out := make([]float32, 768)
		for i := range out {
			out[i] = v
		}
		return out
	}

	mkDoc := func(id, title, body string, tags ...string) {
		if _, err := s.Create(ctx, domain.Document{ID: id, OwnerID: owner, Type: domain.DocFree, Path: id, Title: title, Body: body, Tags: tags, CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	mkDoc("near", "Near", "near doc", "go")
	mkDoc("far", "Far", "far doc")

	// near's chunk points "up" (0.9), far's points "down" (-0.9)
	if err := s.ReplaceChunks(ctx, "near", owner, []string{"near chunk"}, [][]float32{vec(0.9)}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceChunks(ctx, "far", owner, []string{"far chunk"}, [][]float32{vec(-0.9)}); err != nil {
		t.Fatal(err)
	}

	// query "up" → near ranks first
	hits, err := s.SemanticSearch(ctx, owner, vec(1.0), nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 || hits[0].Path != "near" {
		t.Fatalf("want near first, got %#v", hits)
	}
	if hits[0].Snippet != "near chunk" {
		t.Fatalf("snippet = %q, want near chunk", hits[0].Snippet)
	}
	// tag filter composes
	tagged, _ := s.SemanticSearch(ctx, owner, vec(1.0), []string{"go"}, 10)
	if len(tagged) != 1 || tagged[0].Path != "near" {
		t.Fatalf("tag-filtered semantic = %#v, want [near]", tagged)
	}
	// stale tracking: a fresh doc is stale, embedded one is not
	mkDoc("fresh", "Fresh", "fresh")
	stale, _ := s.StaleDocuments(ctx, 100)
	foundFresh := false
	for _, d := range stale {
		if d.Path == "fresh" {
			foundFresh = true
		}
		if d.Path == "near" {
			t.Fatalf("near should not be stale after ReplaceChunks")
		}
	}
	if !foundFresh {
		t.Fatal("fresh doc should be stale")
	}
}
```

NOTE: this is a DB-gated test — it runs only when a container runtime is present (matching `TestDocumentStore_SearchFuzzyAndTag`'s skip behavior), else it skips. The query vector is all-`1.0`; `near`'s chunk is all-`0.9` (cosine ≈ 1, closest) and `far`'s is all-`-0.9` (cosine ≈ -1), so `near` deterministically ranks first regardless of the real embedding model. Ensure `time`/`domain` imports exist.

- [ ] **Step 7: Run + build**

Run: `go test ./internal/testutil/ -run ChunksAndSemantic -v && go test ./internal/adapter/pgstore/ -run SemanticSearch -v && go build ./...`
Expected: fake PASS; pgstore PASS if container runtime present (migration 0010 applies, cosine ordering works), else SKIP; build clean.

- [ ] **Step 8: Commit**

```bash
git add internal/ports/ports.go internal/testutil/fakes.go internal/testutil/fakes_test.go internal/adapter/pgstore/documents.go internal/adapter/pgstore/documents_test.go
git commit -m "feat(m2e): store StaleDocuments + ReplaceChunks + SemanticSearch (pgvector)"
```

---

## Task 8: Embedding worker

**Files:**
- Create: `internal/worker/embed_worker.go`
- Test: `internal/worker/embed_worker_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/worker/embed_worker_test.go`:

```go
package worker

import (
	"context"
	"log/slog"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
)

func TestEmbedWorker_DrainEmbedsStaleDocs(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	emb := testutil.NewFakeEmbedder()
	ctx := context.Background()
	if _, err := docs.Create(ctx, domain.Document{ID: "a", OwnerID: "u", Type: domain.DocFree, Path: "a", Title: "Alpha", Body: "alpha body"}); err != nil {
		t.Fatal(err)
	}
	w := NewEmbedWorker(docs, emb, 0, 10, slog.Default())
	w.drain(ctx)

	stale, _ := docs.StaleDocuments(ctx, 10)
	if len(stale) != 0 {
		t.Fatalf("doc should be embedded (not stale), got %d stale", len(stale))
	}
	hits, _ := docs.SemanticSearch(ctx, "u", mustEmbed(t, emb, "Alpha\n\nalpha body"), nil, 10)
	if len(hits) != 1 || hits[0].ID != "a" {
		t.Fatalf("expected embedded doc to be semantically findable: %#v", hits)
	}
}

func TestEmbedWorker_OllamaDown_LeavesStale(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	emb := testutil.NewFakeEmbedder()
	emb.Err = errDown
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{ID: "a", OwnerID: "u", Type: domain.DocFree, Path: "a", Title: "Alpha", Body: "x"})
	w := NewEmbedWorker(docs, emb, 0, 10, slog.Default())
	w.drain(ctx)
	stale, _ := docs.StaleDocuments(ctx, 10)
	if len(stale) != 1 {
		t.Fatalf("Ollama down → doc must stay stale, got %d", len(stale))
	}
}

var errDown = &downErr{}

type downErr struct{}

func (*downErr) Error() string { return "ollama down" }

func mustEmbed(t *testing.T, e *testutil.FakeEmbedder, s string) []float32 {
	t.Helper()
	v, err := e.Embed(context.Background(), []string{s})
	if err != nil {
		t.Fatal(err)
	}
	return v[0]
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/worker/ -v`
Expected: FAIL — `undefined: NewEmbedWorker`.

- [ ] **Step 3: Implement**

Create `internal/worker/embed_worker.go`:

```go
// Package worker holds background workers. EmbedWorker keeps document embeddings
// up to date asynchronously so the write path never depends on Ollama.
package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/serverkraken/flow/internal/chunk"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// EmbedWorker periodically embeds stale documents. It also exposes DocumentChanged
// so writes can wake it promptly (ports.DocChangeNotifier).
type EmbedWorker struct {
	docs     ports.DocumentStore
	embedder ports.Embedder
	interval time.Duration
	batch    int
	kick     chan struct{}
	log      *slog.Logger
}

// NewEmbedWorker constructs the worker. interval <= 0 disables the periodic tick
// (used in tests that call drain directly).
func NewEmbedWorker(docs ports.DocumentStore, e ports.Embedder, interval time.Duration, batch int, log *slog.Logger) *EmbedWorker {
	if batch <= 0 {
		batch = 16
	}
	return &EmbedWorker{docs: docs, embedder: e, interval: interval, batch: batch, kick: make(chan struct{}, 1), log: log}
}

// DocumentChanged wakes the worker (non-blocking, coalesced). Implements
// ports.DocChangeNotifier.
func (w *EmbedWorker) DocumentChanged() {
	select {
	case w.kick <- struct{}{}:
	default:
	}
}

// Run loops until ctx is cancelled: backfill once, then react to ticks and kicks.
func (w *EmbedWorker) Run(ctx context.Context) {
	w.drain(ctx)
	if w.interval <= 0 {
		<-ctx.Done()
		return
	}
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.drain(ctx)
		case <-w.kick:
			w.drain(ctx)
		}
	}
}

// drain embeds stale documents until none remain or an embed error stops the cycle.
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
		for _, d := range docs {
			if ctx.Err() != nil {
				return
			}
			if err := w.embedOne(ctx, d); err != nil {
				w.log.Warn("embed worker: embed doc", "id", d.ID, "err", err)
				return // backend likely down; retry next tick
			}
		}
	}
}

func (w *EmbedWorker) embedOne(ctx context.Context, d domain.Document) error {
	texts := chunk.Split(d.Title, d.Body)
	if len(texts) == 0 {
		return w.docs.ReplaceChunks(ctx, d.ID, d.OwnerID, nil, nil)
	}
	vecs, err := w.embedder.Embed(ctx, texts)
	if err != nil {
		return err
	}
	return w.docs.ReplaceChunks(ctx, d.ID, d.OwnerID, texts, vecs)
}
```

- [ ] **Step 4: Run + build**

Run: `go test ./internal/worker/ -v && go build ./...`
Expected: PASS + clean.

- [ ] **Step 5: Commit**

```bash
git add internal/worker/embed_worker.go internal/worker/embed_worker_test.go
git commit -m "feat(m2e): async embedding worker (ticker + kick, degrades on backend error)"
```

---

## Task 9: Use case — hybrid fusion + graceful degrade

**Files:**
- Modify: `internal/usecase/search_documents.go`
- Test: `internal/usecase/document_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/usecase/document_test.go` (match the file's package/qualifier style; it constructs `usecase.SearchDocuments{...}` and uses `testutil`):

```go
func TestSearchDocuments_FusesSemanticArm(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	emb := testutil.NewFakeEmbedder()
	ctx := context.Background()
	// A matches the keyword query; B only matches semantically (has chunks, no keyword overlap).
	_, _ = docs.Create(ctx, domain.Document{ID: "a", OwnerID: "u", Type: domain.DocFree, Path: "a", Title: "Alpha", Body: "alpha keyword"})
	_, _ = docs.Create(ctx, domain.Document{ID: "b", OwnerID: "u", Type: domain.DocFree, Path: "b", Title: "Beta", Body: "totally different"})
	texts := []string{"Beta\n\ntotally different"}
	vecs, _ := emb.Embed(ctx, texts)
	if err := docs.ReplaceChunks(ctx, "b", "u", texts, vecs); err != nil {
		t.Fatal(err)
	}

	uc := usecase.SearchDocuments{Docs: docs, Embedder: emb}
	hits, err := uc.Execute(ctx, "u", "alpha", nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, h := range hits {
		got[h.ID] = true
	}
	if !got["a"] || !got["b"] {
		t.Fatalf("fused result should contain a (keyword) and b (semantic): %#v", hits)
	}
}

func TestSearchDocuments_DegradesWhenEmbedderErrors(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	emb := testutil.NewFakeEmbedder()
	emb.Err = context.DeadlineExceeded
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{ID: "a", OwnerID: "u", Type: domain.DocFree, Path: "a", Title: "Alpha", Body: "alpha keyword"})

	uc := usecase.SearchDocuments{Docs: docs, Embedder: emb}
	hits, err := uc.Execute(ctx, "u", "alpha", nil)
	if err != nil {
		t.Fatalf("degrade must not error: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "a" {
		t.Fatalf("degrade should return keyword-only [a], got %#v", hits)
	}
}

func TestSearchDocuments_NilEmbedderIsKeywordOnly(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{ID: "a", OwnerID: "u", Type: domain.DocFree, Path: "a", Title: "Alpha", Body: "alpha keyword"})
	uc := usecase.SearchDocuments{Docs: docs} // no Embedder
	hits, err := uc.Execute(ctx, "u", "alpha", nil)
	if err != nil || len(hits) != 1 || hits[0].ID != "a" {
		t.Fatalf("nil embedder → keyword-only [a]; got %#v err %v", hits, err)
	}
}
```

(Keep the existing `TestSearchDocuments` from M2d — it still passes: nil Embedder → keyword-only.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/usecase/ -run 'SearchDocuments' -v`
Expected: FAIL — `SearchDocuments` has no `Embedder` field.

- [ ] **Step 3: Implement**

Replace the body of `internal/usecase/search_documents.go`:

```go
package usecase

import (
	"context"
	"log/slog"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// SearchDocuments runs a ranked keyword search (FTS + fuzzy) and, when an Embedder
// is configured and reachable, fuses it with a semantic (vector) arm via RRF.
// If the Embedder errors (e.g. Ollama down) the search degrades to keyword-only.
type SearchDocuments struct {
	Docs     ports.DocumentStore
	Embedder ports.Embedder // optional; nil → keyword-only
	Limit    int            // candidates per arm; <=0 → 50
	Log      *slog.Logger   // optional
}

func (uc SearchDocuments) Execute(ctx context.Context, ownerID, q string, tags []string) ([]domain.SearchHit, error) {
	keyword, err := uc.Docs.Search(ctx, ownerID, q, tags)
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
	semantic, err := uc.Docs.SemanticSearch(ctx, ownerID, vecs[0], tags, limit)
	if err != nil {
		uc.warn("semantic search failed; keyword-only", err)
		return keyword, nil
	}
	return rrfFuse(keyword, semantic, rrfK), nil
}

func (uc SearchDocuments) warn(msg string, err error) {
	if uc.Log != nil {
		uc.Log.Warn(msg, "err", err)
	}
}
```

- [ ] **Step 4: Run + build**

Run: `go test ./internal/usecase/ -run 'SearchDocuments|RRFFuse' -v && go build ./...`
Expected: PASS + clean (the M2d `TestSearchDocuments` still passes).

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/search_documents.go internal/usecase/document_test.go
git commit -m "feat(m2e): hybrid keyword+semantic fusion with graceful degrade"
```

---

## Task 10: Write path — notify worker on create/update

**Files:**
- Modify: `internal/ports/ports.go` (DocChangeNotifier)
- Modify: `internal/usecase/create_document.go`, `internal/usecase/update_document.go`
- Test: `internal/usecase/document_test.go`

Before coding, read `create_document.go` and `update_document.go` for their struct/field/Execute shapes and where the successful-write point is.

- [ ] **Step 1: Write the failing test**

Add to `internal/usecase/document_test.go`:

```go
type countingNotifier struct{ n int }

func (c *countingNotifier) DocumentChanged() { c.n++ }

func TestCreateDocument_NotifiesOnWrite(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	note := &countingNotifier{}
	ctx := context.Background()
	uc := usecase.CreateDocument{Docs: docs, Notifier: note} // add any other required fields the real struct has
	if _, err := uc.Execute(ctx, /* match the real Execute signature */); err != nil {
		t.Fatal(err)
	}
	if note.n != 1 {
		t.Fatalf("expected 1 notify, got %d", note.n)
	}
}
```

ADAPT the `CreateDocument` construction and `Execute` call to the REAL struct fields and method signature in `create_document.go` (read it first). The assertion (notifier fires once on a successful create) is the point.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/usecase/ -run NotifiesOnWrite -v`
Expected: FAIL — `CreateDocument` has no `Notifier` field.

- [ ] **Step 3: Add the port**

In `internal/ports/ports.go`, add:

```go
// DocChangeNotifier is notified after a document is created or updated, so the
// embedding worker can re-embed promptly. Optional for callers (nil → no-op).
type DocChangeNotifier interface {
	DocumentChanged()
}
```

- [ ] **Step 4: Implement**

In `create_document.go` and `update_document.go`, add a field to each struct:

```go
	Notifier ports.DocChangeNotifier // optional; nil → no notification
```

At the end of each `Execute`, after the write has succeeded and just before returning success, add:

```go
	if uc.Notifier != nil {
		uc.Notifier.DocumentChanged()
	}
```

(Place it after the successful store call/commit so a failed write does not notify. Match each file's exact return structure.)

- [ ] **Step 5: Run + build**

Run: `go test ./internal/usecase/ -run 'NotifiesOnWrite|Document' -v && go build ./...`
Expected: PASS + clean (existing create/update tests unaffected — nil Notifier).

- [ ] **Step 6: Commit**

```bash
git add internal/ports/ports.go internal/usecase/create_document.go internal/usecase/update_document.go internal/usecase/document_test.go
git commit -m "feat(m2e): notify embedding worker on document create/update"
```

---

## Task 11: Composition root + dev env

**Files:**
- Modify: `cmd/flow-server/main.go`
- Modify: `deploy/dev/compose.yml`
- Modify: `scripts/dev-up.sh`
- Modify: `deploy/dev/flow.env`

Before coding, read `cmd/flow-server/main.go` for: how `documentStore` is built, how the logger is created, how the server context / graceful shutdown works (so the worker goroutine is tied to it), and the `&httpserver.Server{...}` literal where `SearchDocuments`/`CreateDocument`/`UpdateDocument` use cases are constructed.

- [ ] **Step 1: Wire the embedder, worker, and use cases**

In `cmd/flow-server/main.go`:

1. Read config (near other env reads), with the documented defaults:

```go
	ollamaHost := getenv("FLOW_OLLAMA_HOST", "http://localhost:11434")
	embedModel := getenv("FLOW_EMBED_MODEL", "nomic-embed-text")
	embedInterval := getdur("FLOW_EMBED_INTERVAL", 15*time.Second)
	embedBatch := getint("FLOW_EMBED_BATCH", 16)
```

(Use the file's existing env-helper idioms; if there is no `getenv`/`getdur`/`getint` helper, inline `os.Getenv` + `time.ParseDuration` + `strconv.Atoi` with the same defaults. Match the real helpers.)

2. Construct the embedder + worker after `documentStore` exists:

```go
	embedder := embed.NewOllama(ollamaHost, embedModel)
	embedWorker := worker.NewEmbedWorker(documentStore, embedder, embedInterval, embedBatch, logger)
	go embedWorker.Run(ctx) // ctx is the server's lifecycle context (cancelled on shutdown)
```

(Use the real lifecycle context variable. If main builds its context later, construct/start the worker at the point where that context and `documentStore` are both available.)

3. Wire the use cases in the `&httpserver.Server{...}` (or wherever they're constructed):

```go
		SearchDocuments: usecase.SearchDocuments{Docs: documentStore, Embedder: embedder, Log: logger},
		CreateDocument:  usecase.CreateDocument{ /* existing fields */ , Notifier: embedWorker},
		UpdateDocument:  usecase.UpdateDocument{ /* existing fields */ , Notifier: embedWorker},
```

(Preserve all existing fields on CreateDocument/UpdateDocument; only ADD `Notifier: embedWorker`. Update the `SearchDocuments` literal in place.)

Add imports: `github.com/serverkraken/flow/internal/adapter/embed`, `github.com/serverkraken/flow/internal/worker` (and `time`/`strconv` if newly needed).

- [ ] **Step 2: Add Ollama to the dev stack**

In `deploy/dev/compose.yml`, add a service (match the file's existing indentation/style; read the `db`/`dex` services first):

```yaml
  ollama:
    image: docker.io/ollama/ollama:latest
    ports:
      - "11434:11434"
    volumes:
      - ollama-data:/root/.ollama
```

And add `ollama-data:` to the `volumes:` block (match how the db volume is declared).

- [ ] **Step 3: Pull the model in dev-up**

In `scripts/dev-up.sh`, after Dex is confirmed ready (mirror the existing readiness-wait pattern), add an Ollama wait + model pull:

```bash
printf 'waiting for ollama'
ok_ollama=
for _ in $(seq 1 30); do
  if curl -fsS http://localhost:11434/api/tags >/dev/null 2>&1; then ok_ollama=1; break; fi
  printf '.'; sleep 1
done
[ -n "$ok_ollama" ] && echo " ready" || { echo " TIMEOUT"; exit 1; }

echo "pulling embedding model (nomic-embed-text)…"
podman compose -f "$COMPOSE" exec -T ollama ollama pull nomic-embed-text
```

(Match the script's real `$COMPOSE` variable and `podman compose` usage. Update the closing "dev env up" heredoc to mention Ollama if appropriate.)

- [ ] **Step 4: Add env defaults**

In `deploy/dev/flow.env`, add:

```
FLOW_OLLAMA_HOST=http://localhost:11434
FLOW_EMBED_MODEL=nomic-embed-text
FLOW_EMBED_INTERVAL=15s
FLOW_EMBED_BATCH=16
```

- [ ] **Step 5: Verify wiring + build**

Run: `rg -n "embedWorker|SearchDocuments|Notifier:" cmd/flow-server/main.go && go build ./... && go vet ./cmd/... ./internal/worker/ ./internal/adapter/embed/`
Expected: embedder+worker constructed, worker.Run started on the lifecycle ctx, all three use cases wired; build + vet clean. Confirm no other non-test `httpserver.Server{` literal is missing the updated fields (`rg -rn "httpserver.Server\{" cmd/ internal/ | rg -v _test`).

- [ ] **Step 6: Commit**

```bash
git add cmd/flow-server/main.go deploy/dev/compose.yml scripts/dev-up.sh deploy/dev/flow.env
git commit -m "feat(m2e): wire embedder + embedding worker into composition root + dev Ollama"
```

---

## Task 12: Full CI + live done-gate

**Files:** none (verification only)

- [ ] **Step 1: Full CI gate**

Run: `make ci`
Expected: `lint`, `verify-generate`, `cover` (≥ 80%), `build` all green. If coverage dips below 80%, add targeted error-path tests (e.g. `SemanticSearch` store-error in the use case via a fake that errors on the semantic arm, `ReplaceChunks` length-mismatch guard, `Ollama.Embed` transport error) rather than lowering the gate. Paste the coverage %.

- [ ] **Step 2: Dev stack + migration 0010**

`make dev-up` (brings up Postgres + Dex + Ollama, pulls `nomic-embed-text`). Build and run the M2e server against it (per M2c/M2d note: a stale server may hold `:8080`; run on an alt port via `FLOW_LISTEN_ADDR=:8090 ./bin/flow-server` with `deploy/dev/flow.env` sourced). Confirm migration `0010` applied (vector ext + `document_chunks` + HNSW + `chunks_hash`) in the startup log, and that the embed worker logged a backfill pass.

- [ ] **Step 3: curl-smoke (semantic recall + degrade)**

```bash
TOKEN=$(make -s dev-token); B=http://localhost:8090/api/v1/documents
H="Authorization: Bearer $TOKEN"
# a doc that shares NO keyword with the later query:
curl -s -H "$H" -X POST $B -d '{"type":"free","path":"sea","title":"Urlaub am Meer","body":"Strand, Sonne, Wellen und Sand"}'
sleep 3   # let the worker embed (kick + tick)
curl -s -H "$H" "$B?q=Strandferien"   # semantic arm should surface the "Urlaub am Meer" doc
curl -s -H "$H" "$B?q=Strandferien&tag=missing"  # tag filter composes → []
```

Expected: `?q=Strandferien` returns the "Urlaub am Meer" doc via the semantic arm (no shared keyword), fused with any keyword hits; tag composition narrows correctly. Then test **degradation**: stop Ollama (`podman compose -f deploy/dev/compose.yml stop ollama`), repeat a `?q=` request, and confirm it still returns keyword results with HTTP 200 (no error) — the server logs a WARN about the degraded semantic arm. Clean up the test doc afterward (DELETE).

- [ ] **Step 4: Browser + TUI dogfood (Soenne, optional)**

Browser `/docs`: a semantic query surfaces meaning-related docs; keyword hits still highlight; tag filter composes. TUI `flow docs`: `/` search returns fused results. (Per M2c/M2d, Soenne may waive the interactive dogfood and accept the curl-smoke.)

- [ ] **Step 5: Record outcome**

Update the M2e memory note with done + commit range; report to Soenne; await confirmation before the next milestone. Note the deferred `homelab-study` Ollama-provisioning companion (spec §9) as the prerequisite for prod semantic search.

---

## Notes for the implementer

- **pgvector literal binding:** vectors are passed as text literals cast with `$N::vector` (`vectorLiteral`), so no extra Go dependency is needed. Both insert (`ReplaceChunks`) and query (`SemanticSearch`) use this.
- **`scanSemanticHit` column order** is the single highest-risk spot: it MUST match `prefixedDocCols`/`scanDocument` exactly, with `&content, &dist` appended. Copy from the existing scanner, don't hand-write the order.
- **Backward compatibility:** `SearchDocuments` gains optional fields; the M2d wiring and httpserver test harness (which set only `Docs`) keep compiling and behave as keyword-only. Only `main.go` enables the semantic arm.
- **No host changes:** REST/WebUI/TUI ride the existing `?q=` surface unchanged — M2e is invisible plumbing plus richer results.
