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
