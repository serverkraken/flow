package embed

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/ports"
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

	o := NewOllama(srv.URL, "nomic-embed-text", 0)
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
	o := NewOllama(srv.URL, "x", 0)
	if _, err := o.Embed(context.Background(), []string{"a"}); err == nil {
		t.Fatal("expected error on non-200")
	}
}

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
		o := NewOllama(srv.URL, "m", 0)
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
	o := NewOllama("http://127.0.0.1:1", "m", 0) // nothing listening
	_, err := o.Embed(context.Background(), []string{"hi"})
	if err == nil || !errors.Is(err, ports.ErrEmbedTransient) {
		t.Fatalf("want transient connection error, got %v", err)
	}
}

func TestOllamaEmbed_RespectsTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second) // slower than the client timeout below
		_, _ = w.Write([]byte(`{"embeddings":[[0.1]]}`))
	}))
	defer srv.Close()

	o := NewOllama(srv.URL, "m", 100*time.Millisecond)
	start := time.Now()
	_, err := o.Embed(context.Background(), []string{"hi"})
	if err == nil {
		t.Fatal("want a timeout error")
	}
	if !errors.Is(err, ports.ErrEmbedTransient) {
		t.Fatalf("a client timeout must be transient, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("must fail fast at the client timeout, took %v", elapsed)
	}
}
