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
	mux.HandleFunc("GET /api/v1/nodes/bindings", func(w http.ResponseWriter, _ *http.Request) {
		// Owner-wide across ALL devices — two machines, two nodes.
		_ = json.NewEncoder(w).Encode([]domain.ProjectBinding{
			{ID: "b1", NodeID: "r1", Kind: domain.BindingPath, MachineID: "m1", MachineLabel: "notebook-a", Path: "/work/jukebox"},
			{ID: "b2", NodeID: "e1", Kind: domain.BindingPath, MachineID: "m2", MachineLabel: "notebook-b", Path: "/work/alpha"},
			{ID: "b3", NodeID: "r1", Kind: domain.BindingRemote, RemoteSlug: "github.com/serverkraken/jukebox"},
		})
	})
	mux.HandleFunc("GET /api/v1/nodes/resolve", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("slug") == "github.com/serverkraken/jukebox" {
			_ = json.NewEncoder(w).Encode(nodes[2]) // r1/jukebox
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	})
	mux.HandleFunc("GET /api/v1/nodes/resolve-engagement", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("slug") == "github.com/serverkraken/jukebox" {
			_ = json.NewEncoder(w).Encode(nodes[0]) // e1/alpha
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
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

func TestValidateNodeBinding(t *testing.T) {
	cases := []struct {
		name    string
		in      nodeBindingIn
		wantErr string
	}{
		{"bind with node", nodeBindingIn{Action: "bind", Node: "jukebox"}, ""},
		{"bind without node", nodeBindingIn{Action: "bind"}, `needs "node"`},
		{"unbind without node", nodeBindingIn{Action: "unbind"}, ""},
		{"unbind with node", nodeBindingIn{Action: "unbind", Node: "jukebox"}, "by its target only"},
		{"resolve without node", nodeBindingIn{Action: "resolve"}, ""},
		{"resolve with node", nodeBindingIn{Action: "resolve", Node: "jukebox"}, "by its target only"},
		{"list without node", nodeBindingIn{Action: "list"}, ""},
		{"list with node is a filter", nodeBindingIn{Action: "list", Node: "jukebox"}, ""},
		{"unknown action", nodeBindingIn{Action: "attach"}, "invalid action"},
		{"missing action", nodeBindingIn{}, "invalid action"},
		{"kind on bind", nodeBindingIn{Action: "bind", Node: "jukebox", Kind: "path"}, ""},
		{"kind on unbind", nodeBindingIn{Action: "unbind", Kind: "path"}, ""},
		{"kind on resolve", nodeBindingIn{Action: "resolve", Kind: "path"}, `does not take "kind"`},
		{"kind on list", nodeBindingIn{Action: "list", Kind: "remote"}, `does not take "kind"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := validateNodeBinding(c.in)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("validateNodeBinding(%#v) = %v, want nil", c.in, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("validateNodeBinding(%#v) = %v, want an error containing %q", c.in, err, c.wantErr)
			}
		})
	}
}

func TestValidateNodeBinding_InvalidActionListsThem(t *testing.T) {
	_, err := validateNodeBinding(nodeBindingIn{Action: "attach"})
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range nodeBindingActions {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q must list the valid action %q", err.Error(), want)
		}
	}
}

func TestLoopback_NodeBinding_Advertised(t *testing.T) {
	sess, _ := authedBindingServer(t)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTool(tools.Tools, "flow_node_binding") {
		t.Fatalf("flow_node_binding not advertised; got %v", toolNames(tools.Tools))
	}
}

func TestLoopback_NodeBinding_BindAttachesTheTargetToTheNode(t *testing.T) {
	sess, rec := authedBindingServer(t)
	dir := t.TempDir()

	res, out := callText(t, sess, "flow_node_binding", map[string]any{
		"action": "bind", "node": "jukebox", "path": dir, "kind": "path",
	})
	if res.IsError {
		t.Fatalf("bind errored: %s", out)
	}
	got := rec.snapshot()
	if got.bindCalls != 1 || got.bindNodeID != "r1" || got.bindPath != dir {
		t.Fatalf("recorder = %+v, want one path bind of %q on r1", got, dir)
	}
}

func TestLoopback_NodeBinding_UnbindAddressesTheTargetAndRejectsNode(t *testing.T) {
	sess, rec := authedBindingServer(t)

	res, out := callText(t, sess, "flow_node_binding", map[string]any{
		"action": "unbind", "remote": "github.com/serverkraken/jukebox",
	})
	if res.IsError {
		t.Fatalf("unbind errored: %s", out)
	}
	got := rec.snapshot()
	if got.unbindCalls != 1 {
		t.Fatalf("unbindCalls = %d, want 1", got.unbindCalls)
	}
	if !strings.Contains(got.unbindQuery, "kind=remote") || !strings.Contains(got.unbindQuery, "jukebox") {
		t.Fatalf("unbind query = %q, want kind=remote plus the slug", got.unbindQuery)
	}

	// A node argument is a hard error: the apiclient unbind calls take none, so
	// passing one would be silently ignored.
	resNode, outNode := callText(t, sess, "flow_node_binding", map[string]any{
		"action": "unbind", "node": "jukebox", "remote": "github.com/serverkraken/jukebox",
	})
	if !resNode.IsError || !strings.Contains(outNode, "by its target only") {
		t.Fatalf("unbind with node = (IsError=%v, %q), want a rejection", resNode.IsError, outNode)
	}
	if got := rec.snapshot(); got.unbindCalls != 1 {
		t.Fatalf("unbindCalls = %d, want still 1 (the rejected call must not reach the server)", got.unbindCalls)
	}
}

func TestLoopback_NodeBinding_ListShowsEveryDeviceWithItsMachine(t *testing.T) {
	sess, _ := authedBindingServer(t)

	res, out := callText(t, sess, "flow_node_binding", map[string]any{"action": "list"})
	if res.IsError {
		t.Fatalf("list errored: %s", out)
	}
	for _, want := range []string{"3 binding", "notebook-a", "m1", "notebook-b", "m2",
		"/work/jukebox", "/work/alpha", "Jukebox", "Alpha"} {
		if !strings.Contains(out, want) {
			t.Errorf("list missing %q in:\n%s", want, out)
		}
	}
}

func TestLoopback_NodeBinding_ListWithNodeFiltersClientSide(t *testing.T) {
	sess, _ := authedBindingServer(t)

	res, out := callText(t, sess, "flow_node_binding", map[string]any{"action": "list", "node": "jukebox"})
	if res.IsError {
		t.Fatalf("filtered list errored: %s", out)
	}
	if !strings.Contains(out, "2 binding") {
		t.Fatalf("filtered list = %q, want the 2 bindings of r1 only", out)
	}
	if strings.Contains(out, "/work/alpha") || strings.Contains(out, "notebook-b") {
		t.Fatalf("filter leaked another node's binding:\n%s", out)
	}
}

func TestLoopback_NodeBinding_ListNeedsNoFilesystem(t *testing.T) {
	sess, _ := authedBindingServer(t)
	// A path that does not exist must NOT break list: list reports what the
	// server knows and never touches the filesystem.
	res, out := callText(t, sess, "flow_node_binding", map[string]any{
		"action": "list", "path": "/definitely/not/here",
	})
	if res.IsError {
		t.Fatalf("list must ignore a target argument, got IsError: %s", out)
	}
	if !strings.Contains(out, "3 binding") {
		t.Fatalf("list = %q, want all three bindings", out)
	}
}

// TestLoopback_NodeBinding_ResolveRejectsKind: resolve reports what the server's
// resolution chain already decided, and that chain prefers a remote binding over
// a path binding (domain.ResolveBinding). A kind argument would read like an
// override and change nothing, so it is a hard error rather than a silent no-op.
func TestLoopback_NodeBinding_ResolveRejectsKind(t *testing.T) {
	sess, _ := authedBindingServer(t)
	for _, action := range []string{"resolve", "list"} {
		res, out := callText(t, sess, "flow_node_binding", map[string]any{
			"action": action, "kind": "path",
		})
		// The result text is JSON-escaped (structuredErrorResult), so a literal
		// `"` in the message appears as `\"`.
		if !res.IsError || !strings.Contains(out, `does not take \"kind\"`) {
			t.Errorf("%s with kind = (IsError=%v, %q), want a rejection", action, res.IsError, out)
		}
	}
}

func TestLoopback_NodeBinding_ResolveReportsNodeAndEngagementWithoutBinding(t *testing.T) {
	sess, rec := authedBindingServer(t)

	res, out := callText(t, sess, "flow_node_binding", map[string]any{
		"action": "resolve", "remote": "github.com/serverkraken/jukebox",
	})
	if res.IsError {
		t.Fatalf("resolve errored: %s", out)
	}
	for _, want := range []string{"resolves to", "Jukebox", "jukebox", "Alpha"} {
		if !strings.Contains(out, want) {
			t.Errorf("resolve missing %q in: %s", want, out)
		}
	}
	got := rec.snapshot()
	if got.bindCalls != 0 || got.unbindCalls != 0 {
		t.Fatalf("recorder = %+v, want resolve to mutate nothing", got)
	}
}

func TestLoopback_NodeBinding_ResolveWithNothingBoundSaysSoAndSuggestsBind(t *testing.T) {
	sess, _ := authedBindingServer(t)

	res, out := callText(t, sess, "flow_node_binding", map[string]any{
		"action": "resolve", "remote": "github.com/serverkraken/unbound",
	})
	if res.IsError {
		t.Fatalf("an unresolved target is a normal answer, not an error: %s", out)
	}
	if !strings.Contains(out, "Nothing is bound") || !strings.Contains(out, "bind") {
		t.Fatalf("result = %q, want it to state the miss and suggest binding", out)
	}
}

func TestLoopback_NodeBinding_ResolveRejectsNode(t *testing.T) {
	sess, _ := authedBindingServer(t)
	res, out := callText(t, sess, "flow_node_binding", map[string]any{
		"action": "resolve", "node": "jukebox", "remote": "github.com/serverkraken/jukebox",
	})
	if !res.IsError || !strings.Contains(out, "by its target only") {
		t.Fatalf("resolve with node = (IsError=%v, %q), want a rejection", res.IsError, out)
	}
}

func TestLoopback_NodeBinding_BindWithoutNodeIsAnError(t *testing.T) {
	sess, rec := authedBindingServer(t)
	res, out := callText(t, sess, "flow_node_binding", map[string]any{"action": "bind", "path": t.TempDir()})
	// JSON-escaped result text: a literal `"` appears as `\"`.
	if !res.IsError || !strings.Contains(out, `needs \"node\"`) {
		t.Fatalf("bind without node = (IsError=%v, %q), want a rejection", res.IsError, out)
	}
	if got := rec.snapshot(); got.bindCalls != 0 {
		t.Fatalf("bindCalls = %d, want 0", got.bindCalls)
	}
}

// TestLoopback_NodeBinding_ServerRejectsAnUnbindableKind covers the error path
// the client deliberately does NOT pre-check: whether a node may carry a binding
// at all depends on its kind AND on whether it is a leaf, which only the server
// knows (usecase.ErrInvalidBindTarget → 400, internal/adapter/httpserver/projectbindings.go:101).
// The message must reach the model instead of a bare status.
func TestLoopback_NodeBinding_ServerRejectsAnUnbindableKind(t *testing.T) {
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
	mux.HandleFunc("PUT /api/v1/nodes/{id}/bindings", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "binding target has the wrong kind (remote→repo, path→repo or leaf vorhaben)", http.StatusBadRequest)
	})
	be := httptest.NewServer(mux)
	t.Cleanup(be.Close)

	client := apiclient.New(be.URL, "tok")
	_, h := managerFor(t, client, domain.Node{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo})
	sess := connect(t, h.srv)

	res, out := callText(t, sess, "flow_node_binding", map[string]any{
		"action": "bind", "node": "alpha", "path": t.TempDir(), "kind": "path",
	})
	if !res.IsError {
		t.Fatalf("binding an engagement: want IsError, got %q", out)
	}
	if !strings.Contains(out, "wrong kind") {
		t.Fatalf("error = %q, want the server's bind-target message verbatim", out)
	}
}

func TestLoopback_NodeBinding_InvalidActionListsTheValidOnes(t *testing.T) {
	sess, _ := authedBindingServer(t)
	res, out := callText(t, sess, "flow_node_binding", map[string]any{"action": "attach"})
	if !res.IsError {
		t.Fatalf("invalid action: want IsError, got %q", out)
	}
	for _, want := range []string{"bind", "unbind", "list", "resolve"} {
		if !strings.Contains(out, want) {
			t.Errorf("error %q must list the valid action %q", out, want)
		}
	}
}
