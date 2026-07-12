package apiclient_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

func stubArtifact(nodeID, slug string) domain.Artifact {
	return domain.Artifact{
		ID: "art-1", NodeID: nodeID, Slug: slug, Name: "Diagram.png", Mime: "image/png",
		SizeBytes: 42, Ref: "abc123def456",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
}

func TestUploadArtifact_PostsBase64Body(t *testing.T) {
	want := stubArtifact("n1", "diagram")
	data := []byte{0x89, 0x50, 0x4e, 0x47} // arbitrary bytes, content doesn't matter here

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method: got %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/nodes/n1/artifacts" {
			t.Errorf("path: got %s, want /api/v1/nodes/n1/artifacts", r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["name"] != "Diagram.png" || body["mime"] != "application/octet-stream" {
			t.Errorf("body = %+v, want name/mime set", body)
		}
		gotData, err := base64.StdEncoding.DecodeString(body["dataBase64"])
		if err != nil || string(gotData) != string(data) {
			t.Errorf("dataBase64 decode = %v (%v), want %v", gotData, err, data)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	got, err := c.UploadArtifact(context.Background(), "n1", "Diagram.png", "application/octet-stream", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Slug != want.Slug || got.Mime != want.Mime {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestListArtifacts_DecodesSlice(t *testing.T) {
	items := []domain.Artifact{stubArtifact("n1", "a"), stubArtifact("n1", "b")}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method: got %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/nodes/n1/artifacts" {
			t.Errorf("path: got %s, want /api/v1/nodes/n1/artifacts", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(items)
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	got, err := c.ListArtifacts(context.Background(), "n1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 artifacts, got %d", len(got))
	}
}

func TestDeleteArtifact_SendsDelete(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	if err := c.DeleteArtifact(context.Background(), "n1", "diagram"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/api/v1/nodes/n1/artifacts/diagram" {
		t.Errorf("path = %q, want /api/v1/nodes/n1/artifacts/diagram", gotPath)
	}
}
