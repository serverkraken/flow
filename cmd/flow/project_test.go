package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

func TestRunProjectRm(t *testing.T) {
	var deleted string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/nodes":
			_ = json.NewEncoder(w).Encode([]domain.Node{{ID: "p1", Slug: "x"}})
		case r.Method == "DELETE" && r.URL.Path == "/api/v1/nodes/p1":
			deleted = r.URL.Path
			w.WriteHeader(204)
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")
	if err := runNodeRm(context.Background(), c, "x"); err != nil {
		t.Fatal(err)
	}
	if deleted != "/api/v1/nodes/p1" {
		t.Fatalf("deleted = %q", deleted)
	}
}

func TestRunProjectRm_UnknownSlug(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/api/v1/nodes" {
			_ = json.NewEncoder(w).Encode([]domain.Node{{ID: "p1", Slug: "x"}})
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")
	err := runNodeRm(context.Background(), c, "nonexistent")
	if err == nil || !strings.Contains(err.Error(), "no project with slug") {
		t.Fatalf("want 'no project with slug' error, got %v", err)
	}
}
