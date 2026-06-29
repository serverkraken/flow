package apiclient_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

func TestClient_ComposeContext(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/context" || r.URL.Query().Get("remote") != "flow" {
			t.Errorf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"resolution":{"unresolved":false},"instructions":[],"memories":{},"budget":{"used":10,"cap":6000}}`))
	}))
	defer ts.Close()
	c := apiclient.New(ts.URL, "tok")
	cc, err := c.ComposeContext(context.Background(), apiclient.ContextQuery{Remote: "flow"})
	if err != nil {
		t.Fatal(err)
	}
	if cc.Budget.Used != 10 {
		t.Fatalf("decode mismatch: %+v", cc)
	}
}

func TestClient_SetActiveContext(t *testing.T) {
	t.Parallel()
	var gotMethod, gotPath string
	var gotBody struct {
		Body string   `json:"body"`
		Tags []string `json:"tags"`
		Node string   `json:"node"`
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"id":"doc-1","updatedAt":"2026-06-29T00:00:00Z"}`))
	}))
	defer ts.Close()
	c := apiclient.New(ts.URL, "tok")
	res, err := c.SetActiveContext(context.Background(), apiclient.SetActiveContextInput{
		Remote: "flow",
		Body:   "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method: got %s, want PUT", gotMethod)
	}
	if gotPath != "/api/v1/context/active" {
		t.Errorf("path: got %s, want /api/v1/context/active", gotPath)
	}
	if res.ID != "doc-1" {
		t.Errorf("id: got %s, want doc-1", res.ID)
	}
	if gotBody.Body != "hello" {
		t.Errorf("body.body: got %q, want %q", gotBody.Body, "hello")
	}
}

func TestClient_SetPinned(t *testing.T) {
	t.Parallel()
	var gotMethod, gotPath string
	var gotBody struct {
		Pinned bool `json:"pinned"`
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	c := apiclient.New(ts.URL, "tok")
	if err := c.SetPinned(context.Background(), "doc-42", true); err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method: got %s, want POST", gotMethod)
	}
	if gotPath != "/api/v1/documents/doc-42/pin" {
		t.Errorf("path: got %s, want /api/v1/documents/doc-42/pin", gotPath)
	}
	if !gotBody.Pinned {
		t.Errorf("body.pinned: got %v, want true", gotBody.Pinned)
	}
}

func TestClient_SetArchived(t *testing.T) {
	t.Parallel()
	var gotPath, gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	c := apiclient.New(ts.URL, "tok")
	if err := c.SetArchived(context.Background(), "doc-42", true); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/documents/doc-42/archive" {
		t.Fatalf("path: %s", gotPath)
	}
	if !strings.Contains(gotBody, `"archived":true`) {
		t.Fatalf("body: %s", gotBody)
	}
}
