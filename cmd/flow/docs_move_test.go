package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

func TestRunDocsMove_ResolvesProjectAndSendsCompleteMetadata(t *testing.T) {
	var got apiclient.MoveDocumentInput
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Node{{ID: "n1", Slug: "flow", Name: "Flow"}})
	})
	mux.HandleFunc("POST /api/v1/documents/d1/move", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(domain.Document{ID: "d1", Type: domain.DocProject, NodeID: got.NodeID, Path: got.Path})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	var out bytes.Buffer
	err := runDocsMove(context.Background(), apiclient.New(srv.URL, "token"), &out,
		"d1", "project", "flow", "readme", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != "project" || got.NodeID == nil || *got.NodeID != "n1" || got.Path != "readme" || got.Date != nil {
		t.Fatalf("move input = %+v", got)
	}
	if out.String() != "moved project [d1] readme\n" {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRunDocsMove_DailyRequiresDate(t *testing.T) {
	err := runDocsMove(context.Background(), apiclient.New("http://example.invalid", "token"), &bytes.Buffer{},
		"d1", "daily", "none", "", "")
	if err == nil {
		t.Fatal("expected missing-date error")
	}
}
