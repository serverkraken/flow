package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
)

func TestEmbedWorker_DrainEmbedsStaleDocs(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	emb := testutil.NewFakeEmbedder()
	ctx := context.Background()
	if _, err := docs.Create(ctx, domain.Document{ID: "a", OwnerID: "u", Type: domain.DocFree, Path: "a", Title: "Alpha", Body: "alpha body"}); err != nil {
		t.Fatal(err)
	}
	w := NewEmbedWorker(docs, emb, 0, 10, EmbedPolicy{}, slog.Default())
	w.drain(ctx)

	stale, _ := docs.StaleDocuments(ctx, 10)
	if len(stale) != 0 {
		t.Fatalf("doc should be embedded (not stale), got %d stale", len(stale))
	}
	hits, _ := docs.SemanticSearch(ctx, "u", mustEmbed(t, emb, "Alpha\n\nalpha body"), nil, nil, 10)
	if len(hits) != 1 || hits[0].ID != "a" {
		t.Fatalf("expected embedded doc to be semantically findable: %#v", hits)
	}
}

func TestEmbedWorker_ContentChangedDuringEmbedding_ReembedsLatestSnapshot(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	emb := testutil.NewFakeEmbedder()
	ctx := context.Background()
	doc, err := docs.Create(ctx, domain.Document{ID: "race", OwnerID: "u", Type: domain.DocFree, Path: "race", Title: "Old", Body: "old body"})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	emb.FailFunc = func([]string) error {
		calls++
		if calls == 1 {
			doc.Title = "New"
			doc.Body = "new body"
			doc.UpdatedAt = time.Now().Add(time.Second)
			if _, err := docs.Update(ctx, doc); err != nil {
				t.Fatalf("update during embed: %v", err)
			}
		}
		return nil
	}

	w := NewEmbedWorker(docs, emb, 0, 10, EmbedPolicy{}, slog.Default())
	w.drain(ctx)

	if calls != 2 {
		t.Fatalf("want old snapshot discarded and latest snapshot embedded, calls=%d", calls)
	}
	stale, err := docs.StaleDocuments(ctx, 10)
	if err != nil || len(stale) != 0 {
		t.Fatalf("latest snapshot should be embedded, stale=%v err=%v", stale, err)
	}
	hits, err := docs.SemanticSearch(ctx, "u", mustEmbed(t, emb, "New\n\nnew body"), nil, nil, 10)
	if err != nil || len(hits) != 1 || hits[0].Snippet != "New\n\nnew body" {
		t.Fatalf("semantic search should expose latest chunks, hits=%#v err=%v", hits, err)
	}
}

func TestEmbedWorker_FailedStaleSnapshot_DoesNotBackoffLatestContent(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	emb := testutil.NewFakeEmbedder()
	ctx := context.Background()
	doc, err := docs.Create(ctx, domain.Document{ID: "race-fail", OwnerID: "u", Type: domain.DocFree, Path: "race-fail", Title: "Old", Body: "old body"})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	emb.FailFunc = func([]string) error {
		calls++
		if calls == 1 {
			doc.Title = "New"
			doc.Body = "new body"
			doc.UpdatedAt = time.Now().Add(time.Second)
			if _, err := docs.Update(ctx, doc); err != nil {
				t.Fatalf("update during embed: %v", err)
			}
			return errors.New("old content rejected")
		}
		return nil
	}

	w := NewEmbedWorker(docs, emb, 0, 10, EmbedPolicy{MaxAttempts: 1}, slog.Default())
	w.drain(ctx)

	if calls != 2 {
		t.Fatalf("stale failure must not dead-letter new content, calls=%d", calls)
	}
	if status, err := docs.EmbedStatus(ctx, "u", doc.ID); err != nil || status.State != domain.EmbedOK {
		t.Fatalf("latest content should be embedded, status=%#v err=%v", status, err)
	}
}

func TestEmbedWorker_OllamaDown_LeavesStale(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	emb := testutil.NewFakeEmbedder()
	emb.Err = errDown
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{ID: "a", OwnerID: "u", Type: domain.DocFree, Path: "a", Title: "Alpha", Body: "x"})
	w := NewEmbedWorker(docs, emb, 0, 10, EmbedPolicy{}, slog.Default())
	w.drain(ctx)
	stale, _ := docs.StaleDocuments(ctx, 10)
	if len(stale) != 1 {
		t.Fatalf("Ollama down → doc must stay stale, got %d", len(stale))
	}
}

// errDown is transient so drain stops without penalizing the doc.
var errDown = fmt.Errorf("ollama down: %w", ports.ErrEmbedTransient)

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
	_ = docs.RecordEmbedFailure(ctx, "x", "u", docs.SnapshotHash("x"), 4, time.Now().Add(-time.Hour), false, "prev")

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

func mustEmbed(t *testing.T, e *testutil.FakeEmbedder, s string) []float32 {
	t.Helper()
	v, err := e.Embed(context.Background(), []string{s})
	if err != nil {
		t.Fatal(err)
	}
	return v[0]
}

// TestEmbedWorker_DocumentChanged covers DocumentChanged (0% coverage).
// DocumentChanged is a non-blocking channel send used to wake the worker.
func TestEmbedWorker_DocumentChanged(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	emb := testutil.NewFakeEmbedder()
	w := NewEmbedWorker(docs, emb, 0, 10, EmbedPolicy{}, slog.Default())

	// Call twice to exercise both the send path and the default (drop) path.
	w.DocumentChanged()
	w.DocumentChanged()
}

// TestEmbedWorker_Run_CancelledContext covers Run (0% coverage).
// With interval == 0, Run calls drain once then blocks on ctx.Done().
// Cancelling the context immediately exercises the <-ctx.Done() return path.
func TestEmbedWorker_Run_CancelledContext(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	emb := testutil.NewFakeEmbedder()
	w := NewEmbedWorker(docs, emb, 0, 10, EmbedPolicy{}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after ctx was cancelled")
	}
}

// TestEmbedWorker_Run_WithTicker covers the ticker path in Run when interval > 0.
func TestEmbedWorker_Run_WithTicker(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	emb := testutil.NewFakeEmbedder()
	// Very short interval — one tick fires before we cancel.
	w := NewEmbedWorker(docs, emb, 1*time.Millisecond, 10, EmbedPolicy{}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after ctx timeout")
	}
}
