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

// bindRecorder captures which binding endpoint was hit with which body, so the
// tests can prove a path argument really became a path binding on that directory
// instead of the MCP process's own cwd.
type bindRecorder struct {
	mu           sync.Mutex
	bindNodeID   string
	bindKind     string
	bindRemote   string
	bindMachine  string
	bindPath     string
	bindCalls    int
	unbindQuery  string
	unbindCalls  int
	createBounds int
}

// bindSnapshot is bindRecorder's data without the mutex, so callers can copy,
// print (%+v in t.Fatalf) and compare it freely — govet's copylocks check
// would otherwise flag any by-value use of bindRecorder itself.
type bindSnapshot struct {
	bindNodeID   string
	bindKind     string
	bindRemote   string
	bindMachine  string
	bindPath     string
	bindCalls    int
	unbindQuery  string
	unbindCalls  int
	createBounds int
}

func (r *bindRecorder) snapshot() bindSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return bindSnapshot{
		bindNodeID: r.bindNodeID, bindKind: r.bindKind, bindRemote: r.bindRemote,
		bindMachine: r.bindMachine, bindPath: r.bindPath, bindCalls: r.bindCalls,
		unbindQuery: r.unbindQuery, unbindCalls: r.unbindCalls, createBounds: r.createBounds,
	}
}

// fakeBindingBackend serves every endpoint the binding family touches.
// Nodes: e1/alpha (engagement), v1/rebuild (vorhaben under e1), r1/jukebox (repo under v1).
func fakeBindingBackend(t *testing.T, rec *bindRecorder) *httptest.Server {
	t.Helper()
	e1, v1 := "e1", "v1"
	nodes := []domain.Node{
		{ID: "e1", Name: "Alpha", Slug: "alpha", Kind: domain.KindEngagement, Status: domain.NodeActive},
		{ID: "v1", Name: "Rebuild", Slug: "rebuild", Kind: domain.KindVorhaben, ParentID: &e1, Status: domain.NodeActive},
		{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo, ParentID: &v1, Status: domain.NodeActive},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(nodes)
	})
	mux.HandleFunc("PUT /api/v1/nodes/{id}/bindings", func(w http.ResponseWriter, r *http.Request) {
		var body apiclient.BindingFields
		_ = json.NewDecoder(r.Body).Decode(&body)
		rec.mu.Lock()
		rec.bindNodeID, rec.bindKind = r.PathValue("id"), body.Kind
		rec.bindRemote, rec.bindMachine, rec.bindPath = body.RemoteSlug, body.MachineID, body.Path
		rec.bindCalls++
		rec.mu.Unlock()
		_ = json.NewEncoder(w).Encode(domain.ProjectBinding{ID: "b1", NodeID: r.PathValue("id")})
	})
	mux.HandleFunc("DELETE /api/v1/nodes/bindings", func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.unbindQuery = r.URL.RawQuery
		rec.unbindCalls++
		rec.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/nodes/create-bound", func(w http.ResponseWriter, _ *http.Request) {
		rec.mu.Lock()
		rec.createBounds++
		rec.mu.Unlock()
		http.Error(w, "create-bound must not be reachable from flow_bind_project any more", http.StatusInternalServerError)
	})
	return httptest.NewServer(mux)
}

// authedBindingServer builds a loopback session bound to r1/jukebox.
func authedBindingServer(t *testing.T) (*mcp.ClientSession, *bindRecorder) {
	t.Helper()
	rec := &bindRecorder{}
	be := fakeBindingBackend(t, rec)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	_, h := managerFor(t, client, domain.Node{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo})
	return connect(t, h.srv), rec
}

func TestLoopback_BindProject_SchemaHasPathAndRemoteAndNoCreateBranch(t *testing.T) {
	sess, _ := authedBindingServer(t)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		if tool.Name != "flow_bind_project" {
			continue
		}
		schema, ok := tool.InputSchema.(map[string]any)
		if !ok {
			t.Fatalf("input schema type = %T, want map", tool.InputSchema)
		}
		props, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("schema has no properties: %#v", schema)
		}
		if props["path"] == nil || props["remote"] == nil {
			t.Errorf("flow_bind_project schema must offer path and remote, got %v", keysOf(props))
		}
		if props["create_name"] != nil || props["create_parent"] != nil {
			t.Errorf("flow_bind_project must no longer offer create_name/create_parent, got %v", keysOf(props))
		}
		return
	}
	t.Fatal("flow_bind_project not advertised")
}

// keysOf is a stable-ish helper for assertion messages only.
func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestLoopback_BindProject_PathArgumentBindsThatDirectoryNotTheProcessCwd(t *testing.T) {
	sess, rec := authedBindingServer(t)
	dir := t.TempDir() // a real directory with no git origin → path binding

	res, out := callText(t, sess, "flow_bind_project", map[string]any{
		"project": "jukebox", "path": dir, "kind": "path",
	})
	if res.IsError {
		t.Fatalf("bind with path errored: %s", out)
	}
	got := rec.snapshot()
	if got.bindCalls != 1 || got.bindNodeID != "r1" || got.bindKind != "path" {
		t.Fatalf("recorder = %+v, want exactly one path bind on r1", got)
	}
	if got.bindPath != dir {
		t.Fatalf("bound path = %q, want the passed directory %q (not the MCP process cwd)", got.bindPath, dir)
	}
	if got.createBounds != 0 {
		t.Fatalf("create-bound was called %d times; flow_bind_project must never create", got.createBounds)
	}
}

func TestLoopback_BindProject_RemoteArgumentNeedsNoLocalDirectory(t *testing.T) {
	sess, rec := authedBindingServer(t)

	res, out := callText(t, sess, "flow_bind_project", map[string]any{
		"project": "jukebox", "remote": "git@github.com:serverkraken/elsewhere.git",
	})
	if res.IsError {
		t.Fatalf("bind with remote errored: %s", out)
	}
	got := rec.snapshot()
	if got.bindKind != "remote" || got.bindRemote != "github.com/serverkraken/elsewhere" {
		t.Fatalf("recorder = %+v, want a normalized remote binding", got)
	}
}

// TestLoopback_BindProject_ProjectOnlyStaysTheHistoricalCall is the backward
// compatibility contract from Spec §3: the pre-slice invocation — project alone,
// no path, no remote, no kind — must keep working and keep binding the flow-mcp
// process's working directory with auto-detected kind. The test process runs
// inside a git checkout with an origin, so auto-detect lands on remote.
func TestLoopback_BindProject_ProjectOnlyStaysTheHistoricalCall(t *testing.T) {
	sess, rec := authedBindingServer(t)

	res, out := callText(t, sess, "flow_bind_project", map[string]any{"project": "jukebox"})
	if res.IsError {
		t.Fatalf("the historical project-only call errored: %s", out)
	}
	got := rec.snapshot()
	if got.bindCalls != 1 || got.bindNodeID != "r1" {
		t.Fatalf("recorder = %+v, want exactly one bind on r1", got)
	}
	if got.bindKind != "remote" && got.bindKind != "path" {
		t.Fatalf("bindKind = %q, want an auto-detected remote or path binding", got.bindKind)
	}
	if got.bindKind == "path" && got.bindPath == "" {
		t.Fatal("an auto-detected path binding must carry the process cwd")
	}
	if got.createBounds != 0 {
		t.Fatalf("create-bound was called %d times", got.createBounds)
	}
}

func TestLoopback_BindProject_MissingProjectNamesCreateNode(t *testing.T) {
	sess, rec := authedBindingServer(t)

	res, out := callText(t, sess, "flow_bind_project", map[string]any{})
	if !res.IsError {
		t.Fatalf("bind without project: want IsError, got %q", out)
	}
	if !strings.Contains(out, "flow_create_node") {
		t.Fatalf("error = %q, want it to point at flow_create_node", out)
	}
	if got := rec.snapshot(); got.bindCalls != 0 {
		t.Fatalf("bindCalls = %d, want 0 (validation must short-circuit before any HTTP call)", got.bindCalls)
	}
}

func TestLoopback_BindProject_PathAndRemoteTogetherIsAnError(t *testing.T) {
	sess, rec := authedBindingServer(t)

	res, out := callText(t, sess, "flow_bind_project", map[string]any{
		"project": "jukebox", "path": t.TempDir(), "remote": "github.com/a/b",
	})
	if !res.IsError || !strings.Contains(out, "mutually exclusive") {
		t.Fatalf("path+remote = (IsError=%v, %q), want a mutually-exclusive error", res.IsError, out)
	}
	if got := rec.snapshot(); got.bindCalls != 0 {
		t.Fatalf("bindCalls = %d, want 0", got.bindCalls)
	}
}

func TestLoopback_BindProject_UnknownProjectErrors(t *testing.T) {
	sess, _ := authedBindingServer(t)
	res, out := callText(t, sess, "flow_bind_project", map[string]any{"project": "bogus", "kind": "path"})
	if !res.IsError || !strings.Contains(out, "unknown project") {
		t.Fatalf("unknown project = (IsError=%v, %q), want IsError + 'unknown project'", res.IsError, out)
	}
}

// TestUnbindTarget_RemoteAndPathHitTheRightEndpoint exercises unbindTarget
// directly: it has no MCP-tool caller yet (that lands with flow_node_binding
// in a later task) but is already part of this task's produced surface
// (bind.go), so it needs its own coverage independent of flow_bind_project.
func TestUnbindTarget_RemoteAndPathHitTheRightEndpoint(t *testing.T) {
	rec := &bindRecorder{}
	be := fakeBindingBackend(t, rec)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")

	if err := unbindTarget(context.Background(), client, bindTarget{Kind: "remote", RemoteSlug: "github.com/a/b"}); err != nil {
		t.Fatalf("unbindTarget(remote): %v", err)
	}
	if got := rec.snapshot(); got.unbindCalls != 1 || !strings.Contains(got.unbindQuery, "kind=remote") {
		t.Fatalf("recorder = %+v, want one remote unbind", got)
	}

	if err := unbindTarget(context.Background(), client, bindTarget{Kind: "path", MachineID: "m1", Path: "/tmp/x"}); err != nil {
		t.Fatalf("unbindTarget(path): %v", err)
	}
	if got := rec.snapshot(); got.unbindCalls != 2 || !strings.Contains(got.unbindQuery, "kind=path") {
		t.Fatalf("recorder = %+v, want two unbinds total", got)
	}
}
