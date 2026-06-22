package embed_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/embed"
)

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
