package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// newNodeTools is the complete set this slice adds. The surface test asserts all
// of them, so forgetting a registration in server.go can never pass review.
var newNodeTools = []string{
	"flow_create_node", "flow_move_node", "flow_delete_node",
	"flow_get_node", "flow_set_node_tags", "flow_node_binding",
}

// TestLoopback_NodeToolSurfaceIsComplete is the wiring gate: 32 tools, every new
// name advertised, and the two changed tools still present.
func TestLoopback_NodeToolSurfaceIsComplete(t *testing.T) {
	sess := authedNodeChainServer(t)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 32 {
		t.Fatalf("tool count = %d, want 32 (25 before node-mgmt + 6 node tools + fr-node-logo); got %v",
			len(tools.Tools), toolNames(tools.Tools))
	}
	for _, name := range newNodeTools {
		if !hasTool(tools.Tools, name) {
			t.Errorf("%s is implemented but NOT registered in server.go; got %v", name, toolNames(tools.Tools))
		}
	}
	for _, name := range []string{"flow_list_projects", "flow_bind_project", "flow_update_node"} {
		if !hasTool(tools.Tools, name) {
			t.Errorf("%s must survive this slice; got %v", name, toolNames(tools.Tools))
		}
	}
}

// TestLoopback_NewNodeToolSchemas is the schema smoke over all six new tools:
// the properties a caller needs must be present, and nothing may be required
// that the handler treats as optional.
func TestLoopback_NewNodeToolSchemas(t *testing.T) {
	sess := authedNodeChainServer(t)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]*mcp.Tool, len(tools.Tools))
	for _, tool := range tools.Tools {
		byName[tool.Name] = tool
	}

	want := map[string]struct {
		props    []string
		notProps []string
	}{
		"flow_create_node":   {props: []string{"name", "kind", "parent", "description", "color", "glyph", "upstream", "counts_toward_target", "bind_path"}, notProps: []string{"slug", "icon"}},
		"flow_move_node":     {props: []string{"node", "parent", "to_root"}},
		"flow_delete_node":   {props: []string{"node", "confirm"}},
		"flow_get_node":      {props: []string{"node"}},
		"flow_set_node_tags": {props: []string{"node", "tags"}},
		"flow_node_binding":  {props: []string{"action", "node", "path", "remote", "kind"}},
		"flow_bind_project":  {props: []string{"project", "path", "remote", "kind"}, notProps: []string{"create_name", "create_parent"}},
	}
	for name, expect := range want {
		tool, ok := byName[name]
		if !ok {
			t.Errorf("%s not advertised", name)
			continue
		}
		schema, ok := tool.InputSchema.(map[string]any)
		if !ok {
			t.Errorf("%s: input schema type = %T, want map", name, tool.InputSchema)
			continue
		}
		props, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Errorf("%s: schema has no properties: %#v", name, schema)
			continue
		}
		for _, p := range expect.props {
			if props[p] == nil {
				t.Errorf("%s: schema is missing property %q", name, p)
			}
		}
		for _, p := range expect.notProps {
			if props[p] != nil {
				t.Errorf("%s: schema must NOT offer property %q", name, p)
			}
		}
	}

	// Optional-by-handler fields must not be schema-required.
	optional := map[string][]string{
		"flow_create_node":   {"parent", "bind_path", "counts_toward_target", "upstream"},
		"flow_move_node":     {"parent", "to_root"},
		"flow_delete_node":   {"confirm"},
		"flow_get_node":      {"node"},
		"flow_set_node_tags": {"node"},
		"flow_node_binding":  {"node", "path", "remote", "kind"},
		"flow_bind_project":  {"path", "remote", "kind"},
	}
	for name, fields := range optional {
		tool, ok := byName[name]
		if !ok {
			continue
		}
		schema, _ := tool.InputSchema.(map[string]any)
		required, _ := schema["required"].([]any)
		for _, raw := range required {
			for _, f := range fields {
				if raw == f {
					t.Errorf("%s: %q must not be schema-required (the handler treats it as optional)", name, f)
				}
			}
		}
	}
}

// TestLoopback_NodeTools_DegradedRequireLogin is the cross-cut the existing
// suite has for the read tools (loopback_test.go:634): a logged-out caller must
// get the actionable login message from EVERY new tool, never a silent success
// and never a confusing validation error. Arguments are chosen valid on purpose,
// so the only thing that can fail is authentication.
func TestLoopback_NodeTools_DegradedRequireLogin(t *testing.T) {
	sess := degradedSession(t)
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"flow_create_node", map[string]any{"name": "X", "kind": "engagement"}},
		{"flow_move_node", map[string]any{"node": "x", "to_root": true}},
		{"flow_delete_node", map[string]any{"node": "x"}},
		{"flow_get_node", map[string]any{"node": "x"}},
		{"flow_set_node_tags", map[string]any{"node": "x", "tags": []any{"a"}}},
		{"flow_node_binding", map[string]any{"action": "list"}},
		{"flow_bind_project", map[string]any{"project": "x", "remote": "github.com/a/b"}},
	} {
		res, got := callText(t, sess, tc.name, tc.args)
		if !res.IsError || !strings.Contains(got, "Login required") {
			t.Errorf("%s degraded = (IsError=%v, %q), want IsError + 'Login required'", tc.name, res.IsError, got)
		}
	}
}

// chainState is the mutable fixture the chain test drives: a real little node
// store with bindings, so the chain proves cause and effect rather than
// asserting against canned answers.
type chainState struct {
	mu       sync.Mutex
	nodes    []domain.Node
	bindings []domain.ProjectBinding
	tags     map[string][]string
	nextID   int
}

func (s *chainState) find(ref string) (domain.Node, bool) {
	for _, n := range s.nodes {
		if n.ID == ref || n.Slug == ref {
			return n, true
		}
	}
	return domain.Node{}, false
}

// fakeNodeChainBackend serves a small but honest node store: create, move,
// delete, tags, bindings, resolve, stats, ancestors, artifacts, documents.
func fakeNodeChainBackend(t *testing.T) *httptest.Server {
	t.Helper()
	st := &chainState{tags: map[string][]string{}, nextID: 1}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		st.mu.Lock()
		defer st.mu.Unlock()
		out := append([]domain.Node{}, st.nodes...)
		_ = json.NewEncoder(w).Encode(out)
	})
	create := func(f apiclient.CreateNodeFields) domain.Node {
		st.mu.Lock()
		defer st.mu.Unlock()
		id := fmt.Sprintf("n%d", st.nextID)
		st.nextID++
		n := domain.Node{
			ID: id, Name: f.Name, Slug: strings.ToLower(strings.ReplaceAll(f.Name, " ", "-")),
			Kind: domain.NodeKind(f.Kind), ParentID: f.ParentID, Status: domain.NodeActive,
			Description: f.Description, UpstreamGit: f.UpstreamGit, CountsTowardTarget: f.CountsTowardTarget,
		}
		st.nodes = append(st.nodes, n)
		return n
	}
	mux.HandleFunc("POST /api/v1/nodes", func(w http.ResponseWriter, r *http.Request) {
		var f apiclient.CreateNodeFields
		_ = json.NewDecoder(r.Body).Decode(&f)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(create(f))
	})
	mux.HandleFunc("POST /api/v1/nodes/create-bound", func(w http.ResponseWriter, r *http.Request) {
		var in apiclient.CreateBoundNodeInput
		_ = json.NewDecoder(r.Body).Decode(&in)
		n := create(in.Node)
		st.mu.Lock()
		st.bindings = append(st.bindings, domain.ProjectBinding{
			ID: "b" + n.ID, NodeID: n.ID, Kind: domain.BindingKind(in.Binding.Kind),
			RemoteSlug: in.Binding.RemoteSlug, MachineID: in.Binding.MachineID,
			MachineLabel: in.Binding.MachineLabel, Path: in.Binding.Path,
		})
		st.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(apiclient.CreateBoundNodeResult{Node: n})
	})
	mux.HandleFunc("POST /api/v1/nodes/{id}/move", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ParentID *string `json:"parentId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		st.mu.Lock()
		defer st.mu.Unlock()
		for i := range st.nodes {
			if st.nodes[i].ID == r.PathValue("id") {
				st.nodes[i].ParentID = body.ParentID
				_ = json.NewEncoder(w).Encode(st.nodes[i])
				return
			}
		}
		http.Error(w, "not found", http.StatusNotFound)
	})
	mux.HandleFunc("DELETE /api/v1/nodes/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		st.mu.Lock()
		defer st.mu.Unlock()
		for _, n := range st.nodes {
			if n.ParentID != nil && *n.ParentID == id {
				http.Error(w, "node has children; move or remove them first", http.StatusConflict)
				return
			}
		}
		kept := st.nodes[:0]
		for _, n := range st.nodes {
			if n.ID != id {
				kept = append(kept, n)
			}
		}
		st.nodes = kept
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("PUT /api/v1/nodes/{id}/bindings", func(w http.ResponseWriter, r *http.Request) {
		var f apiclient.BindingFields
		_ = json.NewDecoder(r.Body).Decode(&f)
		st.mu.Lock()
		defer st.mu.Unlock()
		b := domain.ProjectBinding{
			ID: "b" + f.Kind + f.Path + f.RemoteSlug, NodeID: r.PathValue("id"),
			Kind: domain.BindingKind(f.Kind), RemoteSlug: f.RemoteSlug,
			MachineID: f.MachineID, MachineLabel: f.MachineLabel, Path: f.Path,
		}
		// UPSERT on the target, not append: the real store conflicts on
		// (owner_id, remote_slug) resp. (owner_id, machine_id, path) and replaces
		// only node_id (internal/adapter/pgstore/projectbindings.go:49). A fake
		// that appended would hide the silent re-point this models.
		replaced := false
		for i := range st.bindings {
			cur := st.bindings[i]
			sameRemote := b.Kind == domain.BindingRemote && cur.Kind == domain.BindingRemote && cur.RemoteSlug == b.RemoteSlug
			samePath := b.Kind == domain.BindingPath && cur.Kind == domain.BindingPath && cur.MachineID == b.MachineID && cur.Path == b.Path
			if sameRemote || samePath {
				st.bindings[i].NodeID = b.NodeID
				replaced = true
				break
			}
		}
		if !replaced {
			st.bindings = append(st.bindings, b)
		}
		_ = json.NewEncoder(w).Encode(b)
	})
	mux.HandleFunc("DELETE /api/v1/nodes/bindings", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		st.mu.Lock()
		defer st.mu.Unlock()
		kept := st.bindings[:0]
		for _, b := range st.bindings {
			match := (q.Get("kind") == "remote" && b.Kind == domain.BindingRemote && b.RemoteSlug == q.Get("slug")) ||
				(q.Get("kind") == "path" && b.Kind == domain.BindingPath && b.MachineID == q.Get("machine") && b.Path == q.Get("path"))
			if !match {
				kept = append(kept, b)
			}
		}
		st.bindings = kept
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/v1/nodes/bindings", func(w http.ResponseWriter, _ *http.Request) {
		st.mu.Lock()
		defer st.mu.Unlock()
		out := append([]domain.ProjectBinding{}, st.bindings...)
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("GET /api/v1/nodes/resolve", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		st.mu.Lock()
		bs := append([]domain.ProjectBinding{}, st.bindings...)
		st.mu.Unlock()
		if b, ok := domain.ResolveBinding(bs, q.Get("slug"), q.Get("machine"), q.Get("path")); ok {
			if n, found := st.find(b.NodeID); found {
				_ = json.NewEncoder(w).Encode(n)
				return
			}
		}
		http.Error(w, "not found", http.StatusNotFound)
	})
	mux.HandleFunc("GET /api/v1/nodes/resolve-engagement", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		st.mu.Lock()
		bs := append([]domain.ProjectBinding{}, st.bindings...)
		st.mu.Unlock()
		b, ok := domain.ResolveBinding(bs, q.Get("slug"), q.Get("machine"), q.Get("path"))
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		cur, found := st.find(b.NodeID)
		for found && cur.ParentID != nil {
			cur, found = st.find(*cur.ParentID)
		}
		if !found || cur.Kind != domain.KindEngagement {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(cur)
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}/ancestors", func(w http.ResponseWriter, r *http.Request) {
		var chain []domain.Node
		cur, ok := st.find(r.PathValue("id"))
		for ok {
			chain = append(chain, cur) // leaf→root
			if cur.ParentID == nil {
				break
			}
			cur, ok = st.find(*cur.ParentID)
		}
		_ = json.NewEncoder(w).Encode(chain)
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}/tags", func(w http.ResponseWriter, r *http.Request) {
		st.mu.Lock()
		defer st.mu.Unlock()
		_ = json.NewEncoder(w).Encode(tagsOf(st.tags[r.PathValue("id")]))
	})
	mux.HandleFunc("PUT /api/v1/nodes/{id}/tags", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Tags []string `json:"tags"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		st.mu.Lock()
		st.tags[r.PathValue("id")] = body.Tags
		st.mu.Unlock()
		_ = json.NewEncoder(w).Encode(tagsOf(body.Tags))
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}/stats", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.NodeRollup{TotalMin: 750, WeekMin: 120, MonthMin: 300})
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}/artifacts", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Artifact{})
	})
	mux.HandleFunc("GET /api/v1/documents", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Document{})
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}", func(w http.ResponseWriter, r *http.Request) {
		if n, ok := st.find(r.PathValue("id")); ok {
			_ = json.NewEncoder(w).Encode(n)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	})
	return httptest.NewServer(mux)
}

func tagsOf(slugs []string) []domain.Tag {
	out := make([]domain.Tag, 0, len(slugs))
	for _, s := range slugs {
		out = append(out, domain.Tag{ID: "t-" + s, Slug: s, Display: s})
	}
	return out
}

func authedNodeChainServer(t *testing.T) *mcp.ClientSession {
	t.Helper()
	be := fakeNodeChainBackend(t)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	_, h := managerFor(t, client, domain.Node{ID: "seed", Name: "Seed", Slug: "seed", Kind: domain.KindRepo})
	return connect(t, h.srv)
}

// TestLoopback_NodeManagementChain walks the exact chain from Spec §7: create an
// engagement, a vorhaben under it, a repo under that with bind_path, resolve it,
// list it with its machine, unbind, bind again to a foreign directory, read the
// detail, replace the tags, move the node, dry-run the delete, then delete.
func TestLoopback_NodeManagementChain(t *testing.T) {
	sess := authedNodeChainServer(t)
	repoDir := t.TempDir()
	foreignDir := t.TempDir()

	// 1. engagement (root, no parent)
	res, out := callText(t, sess, "flow_create_node", map[string]any{"name": "Chain Eng", "kind": "engagement"})
	if res.IsError {
		t.Fatalf("1 create engagement: %s", out)
	}

	// 2. vorhaben under it
	res, out = callText(t, sess, "flow_create_node", map[string]any{
		"name": "Chain Vor", "kind": "vorhaben", "parent": "chain-eng",
	})
	if res.IsError {
		t.Fatalf("2 create vorhaben: %s", out)
	}

	// 3. repo under the vorhaben, bound to a real directory in one atomic command
	res, out = callText(t, sess, "flow_create_node", map[string]any{
		"name": "Chain Repo", "kind": "repo", "parent": "chain-vor", "bind_path": repoDir,
	})
	if res.IsError {
		t.Fatalf("3 create bound repo: %s", out)
	}
	if !strings.Contains(out, repoDir) {
		t.Fatalf("3 result = %q, want it to name the bound directory", out)
	}

	// 4. resolve finds it through the path binding
	res, out = callText(t, sess, "flow_node_binding", map[string]any{"action": "resolve", "path": repoDir})
	if res.IsError {
		t.Fatalf("4 resolve: %s", out)
	}
	if !strings.Contains(out, "Chain Repo") || !strings.Contains(out, "Chain Eng") {
		t.Fatalf("4 resolve = %q, want the repo and its engagement", out)
	}

	// 5. list shows it with its machine
	res, out = callText(t, sess, "flow_node_binding", map[string]any{"action": "list", "node": "chain-repo"})
	if res.IsError {
		t.Fatalf("5 list: %s", out)
	}
	if !strings.Contains(out, repoDir) || !strings.Contains(out, "machine") {
		t.Fatalf("5 list = %q, want the path and its machine", out)
	}

	// 6. unbind that target
	res, out = callText(t, sess, "flow_node_binding", map[string]any{"action": "unbind", "path": repoDir})
	if res.IsError {
		t.Fatalf("6 unbind: %s", out)
	}
	res, out = callText(t, sess, "flow_node_binding", map[string]any{"action": "resolve", "path": repoDir})
	if res.IsError || !strings.Contains(out, "Nothing is bound") {
		t.Fatalf("6 resolve after unbind = (IsError=%v, %q), want 'Nothing is bound'", res.IsError, out)
	}

	// 7. bind again — to a DIFFERENT, foreign directory
	res, out = callText(t, sess, "flow_node_binding", map[string]any{
		"action": "bind", "node": "chain-repo", "path": foreignDir, "kind": "path",
	})
	if res.IsError {
		t.Fatalf("7 rebind: %s", out)
	}
	res, out = callText(t, sess, "flow_node_binding", map[string]any{"action": "resolve", "path": foreignDir})
	if res.IsError || !strings.Contains(out, "Chain Repo") {
		t.Fatalf("7 resolve foreign dir = (IsError=%v, %q), want the repo", res.IsError, out)
	}

	// 7b. re-binding an ALREADY-bound target MOVES it: the store upserts on
	// (owner_id, machine_id, path) and only replaces node_id
	// (internal/adapter/pgstore/projectbindings.go:49). This is silent by design
	// on the server, so the tool descriptions warn about it — and the chain
	// proves the behaviour rather than assuming a conflict.
	res, out = callText(t, sess, "flow_node_binding", map[string]any{
		"action": "bind", "node": "chain-vor", "path": foreignDir, "kind": "path",
	})
	if res.IsError {
		t.Fatalf("7b rebind to another node: %s", out)
	}
	res, out = callText(t, sess, "flow_node_binding", map[string]any{"action": "resolve", "path": foreignDir})
	if res.IsError || !strings.Contains(out, "Chain Vor") {
		t.Fatalf("7b resolve after rebind = (IsError=%v, %q), want the target moved to Chain Vor", res.IsError, out)
	}
	res, out = callText(t, sess, "flow_node_binding", map[string]any{"action": "list", "node": "chain-repo"})
	if res.IsError {
		t.Fatalf("7b list: %s", out)
	}
	if strings.Contains(out, foreignDir) {
		t.Fatalf("7b the moved target still shows on the old node:\n%s", out)
	}
	// Put it back so the rest of the chain reads naturally.
	res, out = callText(t, sess, "flow_node_binding", map[string]any{
		"action": "bind", "node": "chain-repo", "path": foreignDir, "kind": "path",
	})
	if res.IsError {
		t.Fatalf("7b rebind back: %s", out)
	}

	// 8. flow_get_node shows the ancestor chain
	res, out = callText(t, sess, "flow_get_node", map[string]any{"node": "chain-repo"})
	if res.IsError {
		t.Fatalf("8 get_node: %s", out)
	}
	if !strings.Contains(out, "Chain Eng / Chain Vor / Chain Repo") {
		t.Fatalf("8 get_node = %q, want the root→leaf breadcrumb", out)
	}

	// 9. flow_set_node_tags replaces the set
	res, out = callText(t, sess, "flow_set_node_tags", map[string]any{
		"node": "chain-repo", "tags": []any{"go", "audio"},
	})
	if res.IsError {
		t.Fatalf("9 set tags: %s", out)
	}
	res, out = callText(t, sess, "flow_set_node_tags", map[string]any{
		"node": "chain-repo", "tags": []any{"go"},
	})
	if res.IsError {
		t.Fatalf("9 replace tags: %s", out)
	}
	res, out = callText(t, sess, "flow_get_node", map[string]any{"node": "chain-repo"})
	if res.IsError {
		t.Fatalf("9 get_node after tags: %s", out)
	}
	if strings.Contains(out, "audio") {
		t.Fatalf("9 tags were added, not replaced: %q", out)
	}

	// 10. move the repo up to the engagement
	res, out = callText(t, sess, "flow_move_node", map[string]any{"node": "chain-repo", "parent": "chain-eng"})
	if res.IsError {
		t.Fatalf("10 move: %s", out)
	}
	res, out = callText(t, sess, "flow_get_node", map[string]any{"node": "chain-repo"})
	if res.IsError {
		t.Fatalf("10 get_node after move: %s", out)
	}
	if !strings.Contains(out, "Chain Eng / Chain Repo") {
		t.Fatalf("10 breadcrumb after move = %q, want the vorhaben gone from the chain", out)
	}

	// 11. dry run reports and deletes nothing
	res, out = callText(t, sess, "flow_delete_node", map[string]any{"node": "chain-repo"})
	if res.IsError {
		t.Fatalf("11 dry run: %s", out)
	}
	if !strings.Contains(out, "Would delete") || !strings.Contains(out, "confirm=true") {
		t.Fatalf("11 dry run = %q, want a report inviting confirm=true", out)
	}
	res, out = callText(t, sess, "flow_get_node", map[string]any{"node": "chain-repo"})
	if res.IsError {
		t.Fatalf("11 the node must still exist after a dry run: %s", out)
	}

	// 12. confirm deletes it
	res, out = callText(t, sess, "flow_delete_node", map[string]any{"node": "chain-repo", "confirm": true})
	if res.IsError {
		t.Fatalf("12 confirmed delete: %s", out)
	}
	res, out = callText(t, sess, "flow_get_node", map[string]any{"node": "chain-repo"})
	if !res.IsError {
		t.Fatalf("12 the node still resolves after deletion: %q", out)
	}

	// 13. the vorhaben still holds nothing and the tree renders it
	res, out = callText(t, sess, "flow_list_projects", map[string]any{})
	if res.IsError {
		t.Fatalf("13 list_projects: %s", out)
	}
	for _, want := range []string{"Chain Eng", "engagement", "Chain Vor", "vorhaben"} {
		if !strings.Contains(out, want) {
			t.Errorf("13 tree missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Chain Repo") {
		t.Errorf("13 tree still shows the deleted repo:\n%s", out)
	}
}
