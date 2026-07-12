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
// the ownership guard rejects server-side. The free (node-less) library is
// stored under the "" key and served at /api/v1/artifacts.
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
	mux.HandleFunc("POST /api/v1/artifacts", func(w http.ResponseWriter, r *http.Request) {
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
		a := domain.Artifact{ID: "f1", NodeID: "", Slug: "freeslug", Name: in.Name, Mime: in.Mime, SizeBytes: int64(len(data))}
		store[""] = append(store[""], a)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(a)
	})
	mux.HandleFunc("GET /api/v1/artifacts", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(store[""])
	})
	mux.HandleFunc("DELETE /api/v1/artifacts/{slug}", func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		as := store[""]
		out := as[:0]
		found := false
		for _, a := range as {
			if a.Slug == slug {
				found = true
				continue
			}
			out = append(out, a)
		}
		store[""] = out
		if !found {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return httptest.NewServer(mux)
}

// fakeFreeArtifactCLI404Backend always 404s the free-artifact endpoints —
// modeling the server-side owner guard rejecting a foreign/unowned free
// library.
func fakeFreeArtifactCLI404Backend(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/artifacts", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	mux.HandleFunc("/api/v1/artifacts/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
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
	if err := runArtifactAdd(context.Background(), c, &out, path, "alpha", "", false); err != nil {
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
	err := runArtifactAdd(context.Background(), c, &out, filepath.Join(t.TempDir(), "missing.txt"), "alpha", "", false)
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
	if err := runArtifactLs(context.Background(), c, &out, "alpha", false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "keine Artefakte") {
		t.Fatalf("empty list = %q, want 'keine Artefakte'", out.String())
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "report.txt")
	_ = os.WriteFile(path, []byte("hi"), 0o644)
	if err := runArtifactAdd(context.Background(), c, &bytes.Buffer{}, path, "alpha", "", false); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := runArtifactLs(context.Background(), c, &out, "alpha", false); err != nil {
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
	err := runArtifactRm(context.Background(), c, &out, "nope", "alpha", false)
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
	if err := runArtifactAdd(context.Background(), c, &bytes.Buffer{}, path, "alpha", "", false); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runArtifactRm(context.Background(), c, &out, "report", "alpha", false); err != nil {
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

	if err := runArtifactAdd(context.Background(), c, &bytes.Buffer{}, path, "other", "", false); err == nil {
		t.Fatal("add on foreign node should error")
	}
	if err := runArtifactLs(context.Background(), c, &bytes.Buffer{}, "other", false); err == nil {
		t.Fatal("ls on foreign node should error")
	}
	if err := runArtifactRm(context.Background(), c, &bytes.Buffer{}, "whatever", "other", false); err == nil {
		t.Fatal("rm on foreign node should error")
	}
}

// TestRunArtifact_Free_UploadsListsDeletes covers add/ls/rm --free against
// the free-verb REST surface (/api/v1/artifacts), not the node-scoped one.
func TestRunArtifact_Free_UploadsListsDeletes(t *testing.T) {
	t.Parallel()
	srv := fakeArtifactCLIBackend(t)
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	dir := t.TempDir()
	path := filepath.Join(dir, "logo.png")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runArtifactAdd(context.Background(), c, &out, path, "", "", true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "logo") {
		t.Fatalf("free add output = %q, want it to mention the artifact", out.String())
	}

	out.Reset()
	if err := runArtifactLs(context.Background(), c, &out, "", true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "logo") {
		t.Fatalf("free list = %q, want the uploaded artifact", out.String())
	}

	out.Reset()
	if err := runArtifactRm(context.Background(), c, &out, "freeslug", "", true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "freeslug") {
		t.Fatalf("free rm output = %q, want it to name the slug", out.String())
	}

	out.Reset()
	if err := runArtifactLs(context.Background(), c, &out, "", true); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "logo.png") {
		t.Fatalf("free list after delete still shows the artifact: %q", out.String())
	}
}

// TestRunArtifact_Free_MutuallyExclusiveWithNode covers --free --node x → an
// explicit error (non-nil, exit≠0), never a silent pick of one side.
func TestRunArtifact_Free_MutuallyExclusiveWithNode(t *testing.T) {
	t.Parallel()
	srv := fakeArtifactCLIBackend(t)
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	dir := t.TempDir()
	path := filepath.Join(dir, "x.txt")
	_ = os.WriteFile(path, []byte("x"), 0o644)

	if err := runArtifactAdd(context.Background(), c, &bytes.Buffer{}, path, "alpha", "", true); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("add --free --node = %v, want a mutually-exclusive error", err)
	}
	if err := runArtifactLs(context.Background(), c, &bytes.Buffer{}, "alpha", true); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("ls --free --node = %v, want a mutually-exclusive error", err)
	}
	if err := runArtifactRm(context.Background(), c, &bytes.Buffer{}, "whatever", "alpha", true); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("rm --free --node = %v, want a mutually-exclusive error", err)
	}
}

// TestRunArtifact_Free_OwnerScope404 is the free-library counterpart of
// TestRunArtifact_OwnerScope404: a 404 from the free-artifact endpoints must
// surface as a non-nil error, never a silent empty/success result.
func TestRunArtifact_Free_OwnerScope404(t *testing.T) {
	t.Parallel()
	srv := fakeFreeArtifactCLI404Backend(t)
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	dir := t.TempDir()
	path := filepath.Join(dir, "x.txt")
	_ = os.WriteFile(path, []byte("x"), 0o644)

	if err := runArtifactAdd(context.Background(), c, &bytes.Buffer{}, path, "", "", true); err == nil {
		t.Fatal("free add against a 404ing backend should error")
	}
	if err := runArtifactLs(context.Background(), c, &bytes.Buffer{}, "", true); err == nil {
		t.Fatal("free ls against a 404ing backend should error")
	}
	if err := runArtifactRm(context.Background(), c, &bytes.Buffer{}, "whatever", "", true); err == nil {
		t.Fatal("free rm against a 404ing backend should error")
	}
}
