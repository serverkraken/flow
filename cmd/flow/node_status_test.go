package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// TestRunNodeSetStatus is the regression guard for the icon-zeroing bug:
// pause/resume/archive must PATCH status only, never a GetNode-then-full-
// replace body that would clobber icon (or any other field) with a zero value.
func TestRunNodeSetStatus(t *testing.T) {
	t.Parallel()
	var patched map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/nodes":
			_ = json.NewEncoder(w).Encode([]domain.Node{{ID: "n1", Slug: "flow"}})
		case r.Method == "PATCH" && r.URL.Path == "/api/v1/nodes/n1":
			_ = json.NewDecoder(r.Body).Decode(&patched)
			_ = json.NewEncoder(w).Encode(domain.Node{ID: "n1", Slug: "flow", Status: domain.NodePaused})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	var out bytes.Buffer
	if err := runNodeSetStatus(context.Background(), c, &out, "flow", "paused"); err != nil {
		t.Fatal(err)
	}
	if patched["status"] != "paused" {
		t.Fatalf("patched status = %v", patched["status"])
	}
	if len(patched) != 1 {
		t.Fatalf("expected status-only body, got %v", patched)
	}
}

func TestRunNodeRm_HasChildrenFriendly(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/nodes":
			_ = json.NewEncoder(w).Encode([]domain.Node{{ID: "n1", Slug: "eng"}})
		case r.Method == "DELETE" && r.URL.Path == "/api/v1/nodes/n1":
			http.Error(w, "node has children", http.StatusConflict)
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	err := runNodeRm(context.Background(), c, "eng")
	if err == nil || !strings.Contains(err.Error(), "children") {
		t.Fatalf("want friendly children error, got %v", err)
	}
}
