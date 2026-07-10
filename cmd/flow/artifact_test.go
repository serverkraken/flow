package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// fakeArtifactCLIBackend serves the artifact endpoints; p1 (slug "alpha") is
// owned, other1 (slug "other") is in the node list (client-side resolvable)
// but the artifacts endpoints 404 for it — modeling a foreign/unowned node
// the ownership guard rejects server-side.
func fakeArtifactCLIBackend(t *testing.T) *httptest.Server {
	t.Helper()
	store := map[string][]domain.Artifact{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Node{
			{ID: "p1", Name: "Alpha", Slug: "alpha"},
			{ID: "other1", Name: "Other", Slug: "other"},
		})
	})
	mux.HandleFunc("POST /api/v1/nodes/{id}/artifacts", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id != "p1" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var in struct {
			Name       string `json:"name"`
			Mime       string `json:"mime"`
			DataBase64 string `json:"dataBase64"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		data, err := base64.StdEncoding.DecodeString(in.DataBase64)
		if err != nil {
			http.Error(w, "bad base64", http.StatusBadRequest)
			return
		}
		a := domain.Artifact{ID: "a1", NodeID: id, Slug: "report", Name: in.Name, Mime: in.Mime, SizeBytes: int64(len(data))}
		store[id] = append(store[id], a)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(a)
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}/artifacts", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id != "p1" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(store[id])
	})
	mux.HandleFunc("DELETE /api/v1/nodes/{id}/artifacts/{slug}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id != "p1" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		slug := r.PathValue("slug")
		as := store[id]
		out := as[:0]
		found := false
		for _, a := range as {
			if a.Slug == slug {
				found = true
				continue
			}
			out = append(out, a)
		}
		store[id] = out
		if !found {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return httptest.NewServer(mux)
}

func TestRunArtifactAdd_UploadsFile(t *testing.T) {
	t.Parallel()
	srv := fakeArtifactCLIBackend(t)
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	dir := t.TempDir()
	path := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runArtifactAdd(context.Background(), c, &out, path, "alpha", ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "report") {
		t.Fatalf("output = %q, want it to mention the artifact", out.String())
	}
}

func TestRunArtifactAdd_UnreadableFile(t *testing.T) {
	t.Parallel()
	srv := fakeArtifactCLIBackend(t)
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	var out bytes.Buffer
	err := runArtifactAdd(context.Background(), c, &out, filepath.Join(t.TempDir(), "missing.txt"), "alpha", "")
	if err == nil {
		t.Fatal("want an error for an unreadable file")
	}
}

func TestRunArtifactLs_EmptyAndAfterUpload(t *testing.T) {
	t.Parallel()
	srv := fakeArtifactCLIBackend(t)
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	var out bytes.Buffer
	if err := runArtifactLs(context.Background(), c, &out, "alpha"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "keine Artefakte") {
		t.Fatalf("empty list = %q, want 'keine Artefakte'", out.String())
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "report.txt")
	_ = os.WriteFile(path, []byte("hi"), 0o644)
	if err := runArtifactAdd(context.Background(), c, &bytes.Buffer{}, path, "alpha", ""); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := runArtifactLs(context.Background(), c, &out, "alpha"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "report") {
		t.Fatalf("list after upload = %q, want the artifact", out.String())
	}
}

func TestRunArtifactRm_UnknownSlug(t *testing.T) {
	t.Parallel()
	srv := fakeArtifactCLIBackend(t)
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	var out bytes.Buffer
	err := runArtifactRm(context.Background(), c, &out, "nope", "alpha")
	if err == nil {
		t.Fatal("want an error for an unknown slug")
	}
}

func TestRunArtifactRm_DeletesUploaded(t *testing.T) {
	t.Parallel()
	srv := fakeArtifactCLIBackend(t)
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	dir := t.TempDir()
	path := filepath.Join(dir, "report.txt")
	_ = os.WriteFile(path, []byte("hi"), 0o644)
	if err := runArtifactAdd(context.Background(), c, &bytes.Buffer{}, path, "alpha", ""); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runArtifactRm(context.Background(), c, &out, "report", "alpha"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "report") {
		t.Fatalf("rm output = %q, want it to name the slug", out.String())
	}
}

// TestRunArtifact_OwnerScope404 is the Gemini-Fund-#6 guard for the CLI: a
// --node ref that resolves client-side (it's in the node list) but whose
// artifacts endpoint the backend 404s must surface as an error (non-nil, so
// Cobra's Execute exits non-zero) — never a silent empty/success result.
func TestRunArtifact_OwnerScope404(t *testing.T) {
	t.Parallel()
	srv := fakeArtifactCLIBackend(t)
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	dir := t.TempDir()
	path := filepath.Join(dir, "x.txt")
	_ = os.WriteFile(path, []byte("x"), 0o644)

	if err := runArtifactAdd(context.Background(), c, &bytes.Buffer{}, path, "other", ""); err == nil {
		t.Fatal("add on foreign node should error")
	}
	if err := runArtifactLs(context.Background(), c, &bytes.Buffer{}, "other"); err == nil {
		t.Fatal("ls on foreign node should error")
	}
	if err := runArtifactRm(context.Background(), c, &bytes.Buffer{}, "whatever", "other"); err == nil {
		t.Fatal("rm on foreign node should error")
	}
}
