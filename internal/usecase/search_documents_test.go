package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// blockingEmbedder blocks until the context is cancelled — a hung/slow Ollama.
type blockingEmbedder struct{}

func (blockingEmbedder) Embed(ctx context.Context, _ []string) ([][]float32, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestSearchDocuments_QueryEmbedTimeout_DegradesFast(t *testing.T) {
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	_, _ = docs.Create(ctx, domain.Document{ID: "d", OwnerID: "u", Type: domain.DocFree, Path: "d", Title: "Alpha", Body: "alpha body"})

	uc := usecase.SearchDocuments{Docs: docs, Embedder: blockingEmbedder{}, QueryEmbedTimeout: 50 * time.Millisecond}
	start := time.Now()
	_, err := uc.Execute(ctx, "u", "alpha", nil, nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("search must degrade to keyword-only, got err %v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("query embed must time out fast and degrade, took %v", elapsed)
	}
}
