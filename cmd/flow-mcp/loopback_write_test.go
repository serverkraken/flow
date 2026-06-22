package main

import (
	"context"
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

func authedWriteServerWithResources(t *testing.T) (*mcp.ClientSession, *handlers) {
	t.Helper()
	be := fakeWriteBackend(t)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	proj := domain.Project{ID: "p1", Name: "Alpha", Slug: "alpha"}
	srv, h := newServerH(client, true, proj, true)
	if err := h.registerResources(context.Background()); err != nil {
		t.Fatalf("registerResources: %v", err)
	}
	return connect(t, srv), h
}

func TestLoopback_Resources_BootAndLiveSync(t *testing.T) {
	sess, _ := authedWriteServerWithResources(t)
	ctx := context.Background()

	// boot: the one seeded project doc (d-human) is a resource
	rs, err := sess.ListResources(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasResource(rs.Resources, "flow://doc/d-human") {
		t.Fatalf("boot resources = %v, want d-human", resourceURIs(rs.Resources))
	}
	// read returns the (fresh) body
	rr, err := sess.ReadResource(ctx, &mcp.ReadResourceParams{URI: "flow://doc/d-human"})
	if err != nil || len(rr.Contents) == 0 || !strings.Contains(rr.Contents[0].Text, "human note") {
		t.Fatalf("read d-human = (%+v,%v), want the body", rr, err)
	}
	// create in-project → a new resource appears
	_, _ = callText(t, sess, "flow_create_doc", map[string]any{"type": "memory", "path": "notes/r", "title": "R", "body": "rbody"})
	rs, _ = sess.ListResources(ctx, nil)
	if !hasResource(rs.Resources, "flow://doc/new1") {
		t.Fatalf("after create resources = %v, want new1", resourceURIs(rs.Resources))
	}
	// delete (agent-owned, no confirm needed) → resource removed
	_, _ = callText(t, sess, "flow_delete_doc", map[string]any{"id": "new1"})
	rs, _ = sess.ListResources(ctx, nil)
	if hasResource(rs.Resources, "flow://doc/new1") {
		t.Fatalf("after delete resources still has new1: %v", resourceURIs(rs.Resources))
	}
}

func hasResource(rs []*mcp.Resource, uri string) bool {
	for _, r := range rs {
		if r.URI == uri {
			return true
		}
	}
	return false
}

func resourceURIs(rs []*mcp.Resource) []string {
	var u []string
	for _, r := range rs {
		u = append(u, r.URI)
	}
	return u
}

// fakeWriteBackend serves the CRUD endpoints the write tools touch, backed by an
// in-memory map. p1 = the resolved project (Alpha).
func fakeWriteBackend(t *testing.T) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	p1 := "p1"
	docs := map[string]domain.Document{
		"d-human": {ID: "d-human", OwnerID: "u1", ProjectID: &p1, Type: domain.DocFree, Path: "notes/keep", Title: "Keep", Body: "human note"},
	}
	seq := 0
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/projects", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Project{{ID: "p1", Name: "Alpha", Slug: "alpha"}})
	})
	mux.HandleFunc("POST /api/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		var in apiclient.CreateDocumentInput
		_ = json.NewDecoder(r.Body).Decode(&in)
		mu.Lock()
		defer mu.Unlock()
		seq++
		id := "new" + string(rune('0'+seq))
		d := domain.Document{ID: id, OwnerID: "u1", ProjectID: in.ProjectID, Type: domain.DocumentType(in.Type), Path: in.Path, Title: in.Title, Body: in.Body}
		docs[id] = d
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(d)
	})
	mux.HandleFunc("GET /api/v1/documents/{id}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		d, ok := docs[r.PathValue("id")]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(d)
	})
	mux.HandleFunc("PUT /api/v1/documents/{id}", func(w http.ResponseWriter, r *http.Request) {
		var in apiclient.UpdateDocumentInput
		_ = json.NewDecoder(r.Body).Decode(&in)
		mu.Lock()
		defer mu.Unlock()
		d, ok := docs[r.PathValue("id")]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		d.Title, d.Body = in.Title, in.Body
		docs[d.ID] = d
		_ = json.NewEncoder(w).Encode(d)
	})
	mux.HandleFunc("DELETE /api/v1/documents/{id}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		delete(docs, r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	})
	// list (used by resolveScope's nothing here, but harmless to provide)
	mux.HandleFunc("GET /api/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		var out []domain.Document
		pid, has := r.URL.Query().Get("projectId"), r.URL.Query().Has("projectId")
		for _, d := range docs {
			if !has || (d.ProjectID != nil && *d.ProjectID == pid) {
				out = append(out, d)
			}
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	return httptest.NewServer(mux)
}

func authedWriteServer(t *testing.T) *mcp.ClientSession {
	t.Helper()
	be := fakeWriteBackend(t)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	return connect(t, newServer(client, true, domain.Project{ID: "p1", Name: "Alpha", Slug: "alpha"}, true))
}

func TestLoopback_WriteTools_Advertised(t *testing.T) {
	sess := authedWriteServer(t)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"flow_create_doc", "flow_update_doc", "flow_delete_doc"} {
		if !hasTool(tools.Tools, name) {
			t.Fatalf("%s not advertised; got %v", name, toolNames(tools.Tools))
		}
	}
}

func TestLoopback_CreateThenGet(t *testing.T) {
	sess := authedWriteServer(t)
	res, out := callText(t, sess, "flow_create_doc", map[string]any{
		"type": "memory", "path": "notes/new", "title": "New", "body": "fresh body",
	})
	if res.IsError {
		t.Fatalf("create errored: %s", out)
	}
	if !strings.Contains(out, "new1") || !strings.Contains(out, "memory") {
		t.Fatalf("create result = %q, want it to name the new id + type", out)
	}
	_, got := callText(t, sess, "flow_get_doc", map[string]any{"id": "new1"})
	if !strings.Contains(got, "fresh body") {
		t.Fatalf("get after create = %q, want the body", got)
	}
}

func TestLoopback_CreateInvalidType(t *testing.T) {
	sess := authedWriteServer(t)
	res, out := callText(t, sess, "flow_create_doc", map[string]any{"type": "bogus", "path": "p", "title": "T", "body": "B"})
	if !res.IsError || !strings.Contains(out, "memory") {
		t.Fatalf("create bad type = (IsError=%v, %q), want IsError listing valid types", res.IsError, out)
	}
}

func TestLoopback_UpdateGuard(t *testing.T) {
	sess := authedWriteServer(t)
	// human-owned (free) without confirm → refused
	res, out := callText(t, sess, "flow_update_doc", map[string]any{"id": "d-human", "title": "Hacked"})
	if !res.IsError || !strings.Contains(out, "confirm") {
		t.Fatalf("guarded update = (IsError=%v, %q), want refusal naming confirm", res.IsError, out)
	}
	// with confirm → allowed, body carried over (partial merge)
	res, out = callText(t, sess, "flow_update_doc", map[string]any{"id": "d-human", "title": "Edited", "confirm": true})
	if res.IsError {
		t.Fatalf("confirmed update errored: %s", out)
	}
	_, got := callText(t, sess, "flow_get_doc", map[string]any{"id": "d-human"})
	if !strings.Contains(got, "Edited") || !strings.Contains(got, "human note") {
		t.Fatalf("after confirmed partial update = %q, want new title + carried-over body", got)
	}
}

func TestLoopback_UpdateAgentOwnedNoConfirm(t *testing.T) {
	sess := authedWriteServer(t)
	_, _ = callText(t, sess, "flow_create_doc", map[string]any{"type": "memory", "path": "notes/a", "title": "A", "body": "B"})
	res, out := callText(t, sess, "flow_update_doc", map[string]any{"id": "new1", "body": "B2"})
	if res.IsError {
		t.Fatalf("agent-owned update without confirm should succeed, got error: %s", out)
	}
}

func TestLoopback_DeleteGuard(t *testing.T) {
	sess := authedWriteServer(t)
	// human-owned delete without confirm → refused
	res, out := callText(t, sess, "flow_delete_doc", map[string]any{"id": "d-human"})
	if !res.IsError || !strings.Contains(out, "confirm") {
		t.Fatalf("guarded delete = (IsError=%v, %q), want refusal", res.IsError, out)
	}
	// with confirm → gone
	res, _ = callText(t, sess, "flow_delete_doc", map[string]any{"id": "d-human", "confirm": true})
	if res.IsError {
		t.Fatal("confirmed delete should succeed")
	}
	res, _ = callText(t, sess, "flow_get_doc", map[string]any{"id": "d-human"})
	if !res.IsError {
		t.Fatal("get after delete should error (not found)")
	}
}

func TestLoopback_CreateNoneScope(t *testing.T) {
	sess := authedWriteServer(t)
	// Create a doc with project="none" — must produce an unassigned document.
	res, out := callText(t, sess, "flow_create_doc", map[string]any{
		"type": "memory", "path": "notes/unassigned", "title": "Unassigned", "body": "no project", "project": "none",
	})
	if res.IsError {
		t.Fatalf("create with project=none errored: %s", out)
	}
	// Extract the id from the create result ("Created memory [new1] Unassigned · …")
	if !strings.Contains(out, "new1") {
		t.Fatalf("expected id new1 in create result, got %q", out)
	}
	// Round-trip via flow_get_doc: formatDoc renders "project: —" when ProjectID is nil.
	_, got := callText(t, sess, "flow_get_doc", map[string]any{"id": "new1"})
	if !strings.Contains(got, "project: —") {
		t.Fatalf("get after create-with-none = %q; want 'project: —' (nil ProjectID)", got)
	}
}

func TestLoopback_WriteTools_DegradedRequireLogin(t *testing.T) {
	sess := connect(t, newServer(nil, false, domain.Project{}, false))
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"flow_create_doc", map[string]any{"type": "memory", "path": "p", "title": "T", "body": "B"}},
		{"flow_update_doc", map[string]any{"id": "d1", "title": "X"}},
		{"flow_delete_doc", map[string]any{"id": "d1"}},
	} {
		res, got := callText(t, sess, tc.name, tc.args)
		if !res.IsError || !strings.Contains(got, "Login required") {
			t.Fatalf("%s degraded = (IsError=%v, %q), want Login required", tc.name, res.IsError, got)
		}
	}
}
