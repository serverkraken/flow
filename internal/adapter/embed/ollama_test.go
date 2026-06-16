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
