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
