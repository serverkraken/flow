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

func TestValidateCreateNode(t *testing.T) {
	cases := []struct {
		name    string
		in      createNodeIn
		wantErr string // substring; "" = must pass
	}{
		{"engagement as root", createNodeIn{Name: "Alpha", Kind: "engagement"}, ""},
		{"engagement with parent", createNodeIn{Name: "Alpha", Kind: "engagement", Parent: "zeta"}, "always a root"},
		{"vorhaben with parent", createNodeIn{Name: "Rebuild", Kind: "vorhaben", Parent: "alpha"}, ""},
		{"vorhaben without parent", createNodeIn{Name: "Rebuild", Kind: "vorhaben"}, `needs a "parent"`},
		{"repo with parent", createNodeIn{Name: "Jukebox", Kind: "repo", Parent: "rebuild"}, ""},
		{"repo without parent", createNodeIn{Name: "Jukebox", Kind: "repo"}, `needs a "parent"`},
		{"branch is reserved", createNodeIn{Name: "wip", Kind: "branch", Parent: "jukebox"}, "reserved"},
		{"unknown kind", createNodeIn{Name: "X", Kind: "folder"}, "invalid kind"},
		{"missing kind", createNodeIn{Name: "X"}, "invalid kind"},
		{"missing name", createNodeIn{Kind: "engagement"}, "name is required"},
		{"blank name", createNodeIn{Name: "   ", Kind: "engagement"}, "name is required"},
		{"upstream on repo", createNodeIn{Name: "J", Kind: "repo", Parent: "r", Upstream: "git@github.com:a/b.git"}, ""},
		{"upstream on engagement", createNodeIn{Name: "A", Kind: "engagement", Upstream: "git@github.com:a/b.git"}, `only valid for kind "repo"`},
		{"upstream on vorhaben", createNodeIn{Name: "V", Kind: "vorhaben", Parent: "a", Upstream: "git@github.com:a/b.git"}, `only valid for kind "repo"`},
		{"bind_path on repo", createNodeIn{Name: "J", Kind: "repo", Parent: "r", BindPath: "/tmp"}, ""},
		{"bind_path on engagement", createNodeIn{Name: "A", Kind: "engagement", BindPath: "/tmp"}, `"bind_path" is only valid for kind "repo"`},
		{"bind_path on vorhaben", createNodeIn{Name: "V", Kind: "vorhaben", Parent: "a", BindPath: "/tmp"}, `"bind_path" is only valid for kind "repo"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateCreateNode(c.in)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("validateCreateNode(%#v) = %v, want nil", c.in, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateCreateNode(%#v) = nil, want an error containing %q", c.in, c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), c.wantErr)
			}
		})
	}
}

func TestValidateCreateNode_InvalidKindListsTheValidOnes(t *testing.T) {
	err := validateCreateNode(createNodeIn{Name: "X", Kind: "folder"})
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range nodeKindsForCreate {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q must list the valid kind %q (never come back empty)", err.Error(), want)
		}
	}
	if strings.Contains(err.Error(), "branch") {
		t.Errorf("error %q must not offer the reserved kind branch", err.Error())
	}
}

// createRecorder captures the create bodies so the tests can prove which
// endpoint was used and that omitted fields stayed zero/nil.
type createRecorder struct {
	mu         sync.Mutex
	plain      []apiclient.CreateNodeFields
	bound      []apiclient.CreateBoundNodeInput
	plainCalls int
	boundCalls int
}

func (r *createRecorder) counts() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.plainCalls, r.boundCalls
}

func (r *createRecorder) lastPlain(t *testing.T) apiclient.CreateNodeFields {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.plain) == 0 {
		t.Fatal("POST /api/v1/nodes was never called")
	}
	return r.plain[len(r.plain)-1]
}

func (r *createRecorder) lastBound(t *testing.T) apiclient.CreateBoundNodeInput {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.bound) == 0 {
		t.Fatal("POST /api/v1/nodes/create-bound was never called")
	}
	return r.bound[len(r.bound)-1]
}

// fakeCreateBackend serves the create endpoints. Fixture tree:
// e1/alpha (engagement) → v1/rebuild (vorhaben) → r1/jukebox (repo).
func fakeCreateBackend(t *testing.T, rec *createRecorder) *httptest.Server {
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
	mux.HandleFunc("POST /api/v1/nodes", func(w http.ResponseWriter, r *http.Request) {
		var f apiclient.CreateNodeFields
		_ = json.NewDecoder(r.Body).Decode(&f)
		rec.mu.Lock()
		rec.plain = append(rec.plain, f)
		rec.plainCalls++
		rec.mu.Unlock()
		slug := strings.ToLower(strings.ReplaceAll(f.Name, " ", "-"))
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(domain.Node{
			ID: "new1", Name: f.Name, Slug: slug, Kind: domain.NodeKind(f.Kind),
			ParentID: f.ParentID, Status: domain.NodeActive,
		})
	})
	mux.HandleFunc("POST /api/v1/nodes/create-bound", func(w http.ResponseWriter, r *http.Request) {
		var in apiclient.CreateBoundNodeInput
		_ = json.NewDecoder(r.Body).Decode(&in)
		rec.mu.Lock()
		rec.bound = append(rec.bound, in)
		rec.boundCalls++
		rec.mu.Unlock()
		slug := strings.ToLower(strings.ReplaceAll(in.Node.Name, " ", "-"))
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(apiclient.CreateBoundNodeResult{
			Node: domain.Node{ID: "new1", Name: in.Node.Name, Slug: slug,
				Kind: domain.NodeKind(in.Node.Kind), ParentID: in.Node.ParentID, Status: domain.NodeActive},
			Binding: domain.ProjectBinding{ID: "b1", NodeID: "new1"},
		})
	})
	return httptest.NewServer(mux)
}

func authedCreateServer(t *testing.T) (*mcp.ClientSession, *createRecorder) {
	t.Helper()
	rec := &createRecorder{}
	be := fakeCreateBackend(t, rec)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	_, h := managerFor(t, client, domain.Node{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo})
	return connect(t, h.srv), rec
}

func TestLoopback_CreateNode_Advertised(t *testing.T) {
	sess, _ := authedCreateServer(t)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTool(tools.Tools, "flow_create_node") {
		t.Fatalf("flow_create_node not advertised; got %v", toolNames(tools.Tools))
	}
}

func TestLoopback_CreateNode_EngagementUsesPlainCreateWithNoParent(t *testing.T) {
	sess, rec := authedCreateServer(t)

	res, out := callText(t, sess, "flow_create_node", map[string]any{
		"name": "Neues Engagement", "kind": "engagement", "description": "d",
	})
	if res.IsError {
		t.Fatalf("create engagement errored: %s", out)
	}
	plain, bound := rec.counts()
	if plain != 1 || bound != 0 {
		t.Fatalf("plainCalls=%d boundCalls=%d, want 1/0 (no bind_path → plain CreateNode)", plain, bound)
	}
	f := rec.lastPlain(t)
	if f.Kind != "engagement" || f.ParentID != nil {
		t.Fatalf("body = %+v, want engagement with nil parentId", f)
	}
	if f.Slug != "" {
		t.Fatalf("Slug = %q, want empty: the server derives the slug from the name", f.Slug)
	}
	if f.CountsTowardTarget != nil {
		t.Fatalf("CountsTowardTarget = %v, want nil so the server default survives", f.CountsTowardTarget)
	}
	if !strings.Contains(out, "Neues Engagement") || !strings.Contains(out, "new1") {
		t.Fatalf("result = %q, want it to name the node and its id", out)
	}
}

func TestLoopback_CreateNode_RepoUnderVorhabenResolvesParentToID(t *testing.T) {
	sess, rec := authedCreateServer(t)

	res, out := callText(t, sess, "flow_create_node", map[string]any{
		"name": "Neues Repo", "kind": "repo", "parent": "rebuild",
		"upstream": "git@github.com:serverkraken/neu.git",
	})
	if res.IsError {
		t.Fatalf("create repo errored: %s", out)
	}
	f := rec.lastPlain(t)
	if f.ParentID == nil || *f.ParentID != "v1" {
		t.Fatalf("ParentID = %v, want the resolved id v1 (not the slug)", f.ParentID)
	}
	if f.UpstreamGit != "git@github.com:serverkraken/neu.git" {
		t.Fatalf("UpstreamGit = %q, want the passed clone URL verbatim", f.UpstreamGit)
	}
}

func TestLoopback_CreateNode_RepoUnderRepoIsRejectedBeforeAnyWrite(t *testing.T) {
	sess, rec := authedCreateServer(t)

	res, out := callText(t, sess, "flow_create_node", map[string]any{
		"name": "Nested", "kind": "repo", "parent": "jukebox", // jukebox IS a repo
	})
	if !res.IsError {
		t.Fatalf("repo under repo: want IsError, got %q", out)
	}
	if !strings.Contains(out, "engagement or a vorhaben") {
		t.Fatalf("error = %q, want it to name the legal parent kinds", out)
	}
	if plain, bound := rec.counts(); plain != 0 || bound != 0 {
		t.Fatalf("plainCalls=%d boundCalls=%d, want 0/0 (the pre-check must precede the write)", plain, bound)
	}
}

func TestLoopback_CreateNode_UnknownParentSaysParent(t *testing.T) {
	sess, rec := authedCreateServer(t)
	res, out := callText(t, sess, "flow_create_node", map[string]any{
		"name": "X", "kind": "vorhaben", "parent": "bogus",
	})
	if !res.IsError {
		t.Fatalf("unknown parent: want IsError, got %q", out)
	}
	// The result text is the structured error JSON, so assert on substrings.
	if !strings.Contains(out, "parent:") {
		t.Errorf("error = %q, want the message prefixed with 'parent:' so the model knows WHICH argument was bad", out)
	}
	if !strings.Contains(out, "unknown project") {
		t.Errorf("error = %q, want lookupNode's actionable message to survive the prefix", out)
	}
	if plain, bound := rec.counts(); plain != 0 || bound != 0 {
		t.Errorf("plainCalls=%d boundCalls=%d, want 0/0", plain, bound)
	}
}

func TestLoopback_CreateNode_BindPathUsesOneAtomicCreateBoundCommand(t *testing.T) {
	sess, rec := authedCreateServer(t)
	dir := t.TempDir() // real directory, no git origin → path binding

	res, out := callText(t, sess, "flow_create_node", map[string]any{
		"name": "Bound Repo", "kind": "repo", "parent": "rebuild", "bind_path": dir,
	})
	if res.IsError {
		t.Fatalf("create with bind_path errored: %s", out)
	}
	plain, bound := rec.counts()
	if plain != 0 || bound != 1 {
		t.Fatalf("plainCalls=%d boundCalls=%d, want 0/1 — node+binding must stay ONE atomic REST command (Finding 56, 2026-07-15)", plain, bound)
	}
	in := rec.lastBound(t)
	if in.Binding.Kind != "path" || in.Binding.Path != dir {
		t.Fatalf("binding = %+v, want a path binding on %q", in.Binding, dir)
	}
	if in.Binding.MachineID == "" {
		t.Fatalf("binding = %+v, want a machine id on a path binding", in.Binding)
	}
	if in.Node.ParentID == nil || *in.Node.ParentID != "v1" {
		t.Fatalf("node = %+v, want parent v1", in.Node)
	}
	if !strings.Contains(out, dir) {
		t.Fatalf("result = %q, want it to name the bound directory", out)
	}
}

func TestLoopback_CreateNode_BindPathMissingDirectoryFailsBeforeAnyWrite(t *testing.T) {
	sess, rec := authedCreateServer(t)

	res, out := callText(t, sess, "flow_create_node", map[string]any{
		"name": "X", "kind": "repo", "parent": "rebuild", "bind_path": "/definitely/not/here",
	})
	if !res.IsError || !strings.Contains(out, "does not exist") {
		t.Fatalf("missing bind_path = (IsError=%v, %q), want a 'does not exist' error", res.IsError, out)
	}
	if plain, bound := rec.counts(); plain != 0 || bound != 0 {
		t.Fatalf("plainCalls=%d boundCalls=%d, want 0/0 — a bad path must never create a node", plain, bound)
	}
}

// TestLoopback_CreateNode_CountsTowardTargetIsThreeValued walks all three states
// Spec §3 requires: omitted → nil (server default survives), false → Privat,
// true → Work. The omitted case is asserted in the engagement test above.
func TestLoopback_CreateNode_CountsTowardTargetIsThreeValued(t *testing.T) {
	sess, rec := authedCreateServer(t)

	res, out := callText(t, sess, "flow_create_node", map[string]any{
		"name": "Privat", "kind": "engagement", "counts_toward_target": false,
	})
	if res.IsError {
		t.Fatalf("create with false errored: %s", out)
	}
	f := rec.lastPlain(t)
	if f.CountsTowardTarget == nil || *f.CountsTowardTarget != false {
		t.Fatalf("CountsTowardTarget = %v, want an explicit false (Privat), not nil", f.CountsTowardTarget)
	}

	res, out = callText(t, sess, "flow_create_node", map[string]any{
		"name": "Arbeit", "kind": "engagement", "counts_toward_target": true,
	})
	if res.IsError {
		t.Fatalf("create with true errored: %s", out)
	}
	f = rec.lastPlain(t)
	if f.CountsTowardTarget == nil || *f.CountsTowardTarget != true {
		t.Fatalf("CountsTowardTarget = %v, want an explicit true (Work), not nil", f.CountsTowardTarget)
	}

	res, out = callText(t, sess, "flow_create_node", map[string]any{
		"name": "Erbt", "kind": "engagement",
	})
	if res.IsError {
		t.Fatalf("create without the flag errored: %s", out)
	}
	if f = rec.lastPlain(t); f.CountsTowardTarget != nil {
		t.Fatalf("CountsTowardTarget = %v, want nil so the server default survives", f.CountsTowardTarget)
	}
}

// TestLoopback_CreateNode_BindPathOnANonRepoIsRejectedBeforeAnyWrite: the atomic
// usecase refuses a bound node that is not a repo (create_bound_node.go:46), so
// the client says so precisely instead of letting a server 400 through.
func TestLoopback_CreateNode_BindPathOnANonRepoIsRejectedBeforeAnyWrite(t *testing.T) {
	sess, rec := authedCreateServer(t)

	res, out := callText(t, sess, "flow_create_node", map[string]any{
		"name": "Bound Vor", "kind": "vorhaben", "parent": "alpha", "bind_path": t.TempDir(),
	})
	if !res.IsError {
		t.Fatalf("bind_path on a vorhaben: want IsError, got %q", out)
	}
	// The result text is the structured error JSON, so the message's own quotes
	// come back JSON-escaped (\"...\") — match that, not raw quotes.
	if !strings.Contains(out, `\"bind_path\" is only valid for kind \"repo\"`) {
		t.Fatalf("error = %q, want the repo-only guard", out)
	}
	if !strings.Contains(out, "flow_node_binding") {
		t.Fatalf("error = %q, want it to point at the two-step alternative", out)
	}
	if plain, bound := rec.counts(); plain != 0 || bound != 0 {
		t.Fatalf("plainCalls=%d boundCalls=%d, want 0/0", plain, bound)
	}
}
