package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// fakeArtifactBackend serves the artifact endpoints the tools touch. p1 (slug
// "alpha") is the owned node; other1 (slug "other") resolves fine client-side
// (it's in the node list) but the artifacts endpoints 404 for it — modeling a
// foreign/unknown node the ownership guard rejects server-side (Gemini #6).
// The free (node-less) library is stored under the "" key and served at
// /api/v1/artifacts, mirroring the free-artifacts REST surface.
func fakeArtifactBackend(t *testing.T) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	store := map[string][]domain.Artifact{}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
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
		mu.Lock()
		defer mu.Unlock()
		a := domain.Artifact{ID: "a1", NodeID: id, Slug: "logo", Name: in.Name, Mime: in.Mime, SizeBytes: int64(len(data))}
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
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewEncoder(w).Encode(store[id])
	})
	mux.HandleFunc("DELETE /api/v1/nodes/{id}/artifacts/{slug}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id != "p1" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		slug := r.PathValue("slug")
		mu.Lock()
		defer mu.Unlock()
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
		mu.Lock()
		defer mu.Unlock()
		a := domain.Artifact{ID: "f1", NodeID: "", Slug: "freeslug", Name: in.Name, Mime: in.Mime, SizeBytes: int64(len(data))}
		store[""] = append(store[""], a)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(a)
	})
	mux.HandleFunc("GET /api/v1/artifacts", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewEncoder(w).Encode(store[""])
	})
	mux.HandleFunc("DELETE /api/v1/artifacts/{slug}", func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		mu.Lock()
		defer mu.Unlock()
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

// fakeFreeArtifact404Backend serves /api/v1/me + /api/v1/nodes (needed for
// connect/scope setup) but always 404s the free-artifact endpoints — modeling
// the server-side owner guard rejecting a foreign/unowned free library.
func fakeFreeArtifact404Backend(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Node{{ID: "p1", Name: "Alpha", Slug: "alpha"}})
	})
	mux.HandleFunc("/api/v1/artifacts", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	mux.HandleFunc("/api/v1/artifacts/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	return httptest.NewServer(mux)
}

func authedArtifactServer(t *testing.T) *mcp.ClientSession {
	t.Helper()
	be := fakeArtifactBackend(t)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	proj := domain.Node{ID: "p1", Name: "Alpha", Slug: "alpha"}
	mgr, h := managerFor(t, client, proj)
	_ = mgr
	return connect(t, h.srv)
}

// unboundArtifactServer builds a session with no project bound to the
// directory (h.matched=false) — the ordinary node path (`h.artifactNode` →
// `resolveScope("")`) errors "a node is required" in this state, so it also
// serves as a bypass witness: a free:true call that still succeeds proves it
// never reached `h.artifactNode`.
func unboundArtifactServer(t *testing.T) *mcp.ClientSession {
	t.Helper()
	be := fakeArtifactBackend(t)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	mgr, h := managerFor(t, client, domain.Node{})
	h.projMu.Lock()
	h.matched = false
	h.projMu.Unlock()
	_ = mgr
	return connect(t, h.srv)
}

func TestLoopback_ArtifactTools_Advertised(t *testing.T) {
	sess := authedArtifactServer(t)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"flow_upload_artifact", "flow_list_artifacts", "flow_delete_artifact"} {
		if !hasTool(tools.Tools, name) {
			t.Fatalf("%s not advertised; got %v", name, toolNames(tools.Tools))
		}
	}
}

func TestLoopback_UploadListDeleteArtifact(t *testing.T) {
	sess := authedArtifactServer(t)
	b64 := base64.StdEncoding.EncodeToString([]byte("hello world"))

	res, out := callText(t, sess, "flow_upload_artifact", map[string]any{
		"name": "logo.png", "mime": "image/png", "base64": b64,
	})
	if res.IsError {
		t.Fatalf("upload errored: %s", out)
	}
	if !strings.Contains(out, "logo") {
		t.Fatalf("upload result = %q, want it to name the slug", out)
	}

	_, out = callText(t, sess, "flow_list_artifacts", map[string]any{})
	if !strings.Contains(out, "logo") || !strings.Contains(out, "logo.png") {
		t.Fatalf("list after upload = %q, want the artifact listed", out)
	}

	res, out = callText(t, sess, "flow_delete_artifact", map[string]any{"slug": "logo"})
	if res.IsError {
		t.Fatalf("delete errored: %s", out)
	}

	_, out = callText(t, sess, "flow_list_artifacts", map[string]any{})
	if strings.Contains(out, "logo.png") {
		t.Fatalf("list after delete still shows the artifact: %q", out)
	}
}

func TestLoopback_UploadArtifact_InvalidBase64(t *testing.T) {
	sess := authedArtifactServer(t)
	res, out := callText(t, sess, "flow_upload_artifact", map[string]any{
		"name": "x", "mime": "image/png", "base64": "not valid base64!!",
	})
	if !res.IsError {
		t.Fatalf("upload with invalid base64 should error, got %q", out)
	}
}

func TestDecodeArtifactBase64_RejectsOversizedInputBeforeDecode(t *testing.T) {
	t.Parallel()
	raw := strings.Repeat("A", base64.StdEncoding.EncodedLen(int(domain.MaxArtifactBytes))+4)
	if _, err := decodeArtifactBase64(raw); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized base64 error = %v, want size rejection", err)
	}
}

func TestLoopback_UploadArtifact_MissingFields(t *testing.T) {
	sess := authedArtifactServer(t)
	res, out := callText(t, sess, "flow_upload_artifact", map[string]any{"base64": "aGk="})
	if !res.IsError || !strings.Contains(out, "required") {
		t.Fatalf("upload without name/mime = (IsError=%v, %q), want a 'required' error", res.IsError, out)
	}
}

func TestLoopback_DeleteArtifact_UnknownSlug(t *testing.T) {
	sess := authedArtifactServer(t)
	res, out := callText(t, sess, "flow_delete_artifact", map[string]any{"slug": "nope"})
	if !res.IsError {
		t.Fatalf("delete of unknown slug should error, got %q", out)
	}
}

// TestLoopback_ArtifactTools_OwnerScope404 is the Gemini-Fund-#6 guard: a node
// ref that resolves client-side (it's in the node list) but whose artifacts
// endpoint the backend 404s — as it would for a foreign/unowned node — must
// come back as an MCP error, never a silent empty/success result.
func TestLoopback_ArtifactTools_OwnerScope404(t *testing.T) {
	sess := authedArtifactServer(t)

	res, out := callText(t, sess, "flow_list_artifacts", map[string]any{"node": "other"})
	if !res.IsError {
		t.Fatalf("list on foreign node = (IsError=%v, %q), want an error", res.IsError, out)
	}

	res, out = callText(t, sess, "flow_upload_artifact", map[string]any{
		"node": "other", "name": "x.png", "mime": "image/png", "base64": base64.StdEncoding.EncodeToString([]byte("x")),
	})
	if !res.IsError {
		t.Fatalf("upload on foreign node = (IsError=%v, %q), want an error", res.IsError, out)
	}

	res, out = callText(t, sess, "flow_delete_artifact", map[string]any{"node": "other", "slug": "whatever"})
	if !res.IsError {
		t.Fatalf("delete on foreign node = (IsError=%v, %q), want an error", res.IsError, out)
	}
}

// TestLoopback_FreeArtifact_BypassesNodeResolution is Task 5's core guard
// (OE #1): free:true must never reach h.artifactNode/resolveScope. It runs
// against an *unbound* session (no project bound to cwd), where the ordinary
// node path errors "a node is required" — so a free:true call that still
// succeeds proves it bypassed that path entirely.
func TestLoopback_FreeArtifact_BypassesNodeResolution(t *testing.T) {
	sess := unboundArtifactServer(t)
	b64 := base64.StdEncoding.EncodeToString([]byte("hello world"))

	res, out := callText(t, sess, "flow_upload_artifact", map[string]any{
		"name": "logo.png", "mime": "image/png", "base64": b64, "free": true,
	})
	if res.IsError {
		t.Fatalf("free upload on an unbound session errored (want it to bypass node resolution): %s", out)
	}
	if !strings.Contains(out, "freeslug") {
		t.Fatalf("free upload result = %q, want it to name the slug", out)
	}

	_, out = callText(t, sess, "flow_list_artifacts", map[string]any{"free": true})
	if !strings.Contains(out, "freeslug") || !strings.Contains(out, "logo.png") {
		t.Fatalf("free list = %q, want the uploaded artifact listed", out)
	}

	res, out = callText(t, sess, "flow_delete_artifact", map[string]any{"slug": "freeslug", "free": true})
	if res.IsError {
		t.Fatalf("free delete errored: %s", out)
	}

	// Sanity: the same unbound session errors on the ordinary (non-free) path —
	// proving the free calls above really bypassed h.artifactNode, not that the
	// session is simply lenient.
	res, out = callText(t, sess, "flow_upload_artifact", map[string]any{
		"name": "x.png", "mime": "image/png", "base64": b64,
	})
	if !res.IsError {
		t.Fatalf("non-free upload on an unbound session should error, got %q", out)
	}
}

// TestLoopback_FreeArtifact_MutuallyExclusiveWithNode covers free:true +
// node:"x" → an explicit error, never a silent pick of one side.
func TestLoopback_FreeArtifact_MutuallyExclusiveWithNode(t *testing.T) {
	sess := authedArtifactServer(t)
	b64 := base64.StdEncoding.EncodeToString([]byte("hello world"))

	res, out := callText(t, sess, "flow_upload_artifact", map[string]any{
		"name": "logo.png", "mime": "image/png", "base64": b64, "free": true, "node": "alpha",
	})
	if !res.IsError || !strings.Contains(out, "mutually exclusive") {
		t.Fatalf("free+node upload = (IsError=%v, %q), want a mutually-exclusive error", res.IsError, out)
	}

	res, out = callText(t, sess, "flow_list_artifacts", map[string]any{"free": true, "node": "alpha"})
	if !res.IsError || !strings.Contains(out, "mutually exclusive") {
		t.Fatalf("free+node list = (IsError=%v, %q), want a mutually-exclusive error", res.IsError, out)
	}

	res, out = callText(t, sess, "flow_delete_artifact", map[string]any{"slug": "x", "free": true, "node": "alpha"})
	if !res.IsError || !strings.Contains(out, "mutually exclusive") {
		t.Fatalf("free+node delete = (IsError=%v, %q), want a mutually-exclusive error", res.IsError, out)
	}
}

// TestLoopback_ArtifactTools_NodeSetFreeFalse_Unchanged pins that an explicit
// node + free:false (the JSON zero value, indistinguishable from omitted)
// takes the ordinary node path unchanged.
func TestLoopback_ArtifactTools_NodeSetFreeFalse_Unchanged(t *testing.T) {
	sess := authedArtifactServer(t)
	b64 := base64.StdEncoding.EncodeToString([]byte("hello world"))

	res, out := callText(t, sess, "flow_upload_artifact", map[string]any{
		"name": "logo.png", "mime": "image/png", "base64": b64, "node": "alpha", "free": false,
	})
	if res.IsError {
		t.Fatalf("node+free:false upload errored: %s", out)
	}
	if !strings.Contains(out, "in project Alpha") {
		t.Fatalf("upload result = %q, want the ordinary node label", out)
	}
}

// TestLoopback_FreeArtifact_OwnerScope404 is the free-library counterpart of
// TestLoopback_ArtifactTools_OwnerScope404: a 404 from the free-artifact
// endpoints (as the server-side owner guard would produce for a foreign
// owner) must come back as an MCP error, never a silent empty/success result.
func TestLoopback_FreeArtifact_OwnerScope404(t *testing.T) {
	be := fakeFreeArtifact404Backend(t)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	proj := domain.Node{ID: "p1", Name: "Alpha", Slug: "alpha"}
	mgr, h := managerFor(t, client, proj)
	_ = mgr
	sess := connect(t, h.srv)

	res, out := callText(t, sess, "flow_list_artifacts", map[string]any{"free": true})
	if !res.IsError {
		t.Fatalf("free list against a 404ing backend = (IsError=%v, %q), want an error", res.IsError, out)
	}

	res, out = callText(t, sess, "flow_upload_artifact", map[string]any{
		"free": true, "name": "x.png", "mime": "image/png", "base64": base64.StdEncoding.EncodeToString([]byte("x")),
	})
	if !res.IsError {
		t.Fatalf("free upload against a 404ing backend = (IsError=%v, %q), want an error", res.IsError, out)
	}

	res, out = callText(t, sess, "flow_delete_artifact", map[string]any{"free": true, "slug": "whatever"})
	if !res.IsError {
		t.Fatalf("free delete against a 404ing backend = (IsError=%v, %q), want an error", res.IsError, out)
	}
}
