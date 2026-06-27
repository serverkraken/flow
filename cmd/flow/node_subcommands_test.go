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

// runNodeList/Create/Move are the testable cores (no clientFromStore/env).

func TestRunNodeCreate_PostsKindAndParent(t *testing.T) {
	t.Parallel()
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/nodes":
			_ = json.NewEncoder(w).Encode([]domain.Node{{ID: "eng1", Slug: "privat", Kind: domain.KindEngagement}})
		case r.Method == "POST" && r.URL.Path == "/api/v1/nodes":
			_ = json.NewDecoder(r.Body).Decode(&got)
			_ = json.NewEncoder(w).Encode(domain.Node{ID: "n1", Name: "flow", Slug: "flow", Kind: domain.KindRepo})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	var out bytes.Buffer
	if err := runNodeCreate(context.Background(), c, &out, "flow", "repo", "privat", "", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if got["kind"] != "repo" || got["parentId"] != "eng1" {
		t.Fatalf("posted = %v", got)
	}
	if !strings.Contains(out.String(), "flow") {
		t.Errorf("output missing node name: %q", out.String())
	}
}

func TestRunNodeList_Tree(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pid := "eng1"
		_ = json.NewEncoder(w).Encode([]domain.Node{
			{ID: "eng1", Name: "Privat", Slug: "privat", Kind: domain.KindEngagement, Status: domain.NodeActive},
			{ID: "r1", Name: "flow", Slug: "flow", Kind: domain.KindRepo, ParentID: &pid, Status: domain.NodeActive},
		})
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	var out bytes.Buffer
	if err := runNodeList(context.Background(), c, &out, true, "", "all"); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 2 || strings.HasPrefix(lines[0], " ") || !strings.HasPrefix(lines[1], "  ") {
		t.Fatalf("tree output:\n%s", out.String())
	}
}

func TestRunNodeList_KindFilter(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pid := "eng1"
		_ = json.NewEncoder(w).Encode([]domain.Node{
			{ID: "eng1", Name: "Privat", Slug: "privat", Kind: domain.KindEngagement, Status: domain.NodeActive},
			{ID: "r1", Name: "flow", Slug: "flow", Kind: domain.KindRepo, ParentID: &pid, Status: domain.NodeActive},
		})
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	var out bytes.Buffer
	if err := runNodeList(context.Background(), c, &out, false, "engagement", "all"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "flow") || !strings.Contains(out.String(), "privat") {
		t.Fatalf("kind filter wrong:\n%s", out.String())
	}
}

func TestRunNodeMove_CycleSurfaced(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/nodes":
			_ = json.NewEncoder(w).Encode([]domain.Node{
				{ID: "a", Slug: "a"}, {ID: "b", Slug: "b"},
			})
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/move"):
			http.Error(w, "move would create a cycle", http.StatusConflict)
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	var out bytes.Buffer
	err := runNodeMove(context.Background(), c, &out, "a", "b")
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("want cycle error, got %v", err)
	}
}
