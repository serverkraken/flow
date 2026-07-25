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

func TestValidateMoveNode(t *testing.T) {
	cases := []struct {
		name    string
		in      moveNodeIn
		wantErr string
	}{
		{"parent only", moveNodeIn{Node: "jukebox", Parent: "alpha"}, ""},
		{"to_root only", moveNodeIn{Node: "alpha", ToRoot: true}, ""},
		{"both", moveNodeIn{Node: "jukebox", Parent: "alpha", ToRoot: true}, "exactly one destination"},
		{"neither", moveNodeIn{Node: "jukebox"}, "exactly one destination"},
		{"blank parent counts as absent", moveNodeIn{Node: "jukebox", Parent: "   "}, "exactly one destination"},
		{"blank parent plus to_root is fine", moveNodeIn{Node: "alpha", Parent: "  ", ToRoot: true}, ""},
		{"missing node", moveNodeIn{Parent: "alpha"}, "node is required"},
		{"blank node", moveNodeIn{Node: "  ", ToRoot: true}, "node is required"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateMoveNode(c.in)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("validateMoveNode(%#v) = %v, want nil", c.in, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("validateMoveNode(%#v) = %v, want an error containing %q", c.in, err, c.wantErr)
			}
		})
	}
}

// lifecycleRecorder captures the move and delete calls.
type lifecycleRecorder struct {
	mu          sync.Mutex
	moveNodeID  string
	moveParent  *string
	moveCalls   int
	deleteID    string
	deleteCalls int
}

// lifecycleSnapshot is the lock-free copy returned by snapshot(); it exists so
// asserting on it (e.g. via t.Fatalf("%+v", got)) never copies the mutex
// (govet: copylocks) the way copying lifecycleRecorder itself would.
type lifecycleSnapshot struct {
	moveNodeID  string
	moveParent  *string
	moveCalls   int
	deleteID    string
	deleteCalls int
}

func (r *lifecycleRecorder) snapshot() lifecycleSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return lifecycleSnapshot{
		moveNodeID: r.moveNodeID, moveParent: r.moveParent, moveCalls: r.moveCalls,
		deleteID: r.deleteID, deleteCalls: r.deleteCalls,
	}
}

// lifecycleNodes is the fixture tree used by the move and delete tests:
// e1/alpha (engagement) → v1/rebuild (vorhaben) → r1/jukebox (repo);
// e2/zeta is a second engagement, and l1/leaf is a childless repo under e2.
func lifecycleNodes() []domain.Node {
	e1, v1, e2 := "e1", "v1", "e2"
	return []domain.Node{
		{ID: "e1", Name: "Alpha", Slug: "alpha", Kind: domain.KindEngagement, Status: domain.NodeActive},
		{ID: "v1", Name: "Rebuild", Slug: "rebuild", Kind: domain.KindVorhaben, ParentID: &e1, Status: domain.NodeActive},
		{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo, ParentID: &v1, Status: domain.NodeActive},
		{ID: "e2", Name: "Zeta", Slug: "zeta", Kind: domain.KindEngagement, Status: domain.NodeActive},
		{ID: "l1", Name: "Leaf", Slug: "leaf", Kind: domain.KindRepo, ParentID: &e2, Status: domain.NodeActive},
	}
}

// fakeLifecycleBackend serves move, delete and every endpoint the delete dry run
// reads. Deleting v1 (which has the child r1) answers 409 like the real server.
func fakeLifecycleBackend(t *testing.T, rec *lifecycleRecorder) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(lifecycleNodes())
	})
	mux.HandleFunc("POST /api/v1/nodes/{id}/move", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ParentID *string `json:"parentId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		rec.mu.Lock()
		rec.moveNodeID, rec.moveParent = r.PathValue("id"), body.ParentID
		rec.moveCalls++
		rec.mu.Unlock()
		for _, n := range lifecycleNodes() {
			if n.ID == r.PathValue("id") {
				n.ParentID = body.ParentID
				_ = json.NewEncoder(w).Encode(n)
				return
			}
		}
		http.Error(w, "not found", http.StatusNotFound)
	})
	mux.HandleFunc("DELETE /api/v1/nodes/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		rec.mu.Lock()
		rec.deleteID = id
		rec.deleteCalls++
		rec.mu.Unlock()
		if id == "v1" { // has child r1 — mirrors worktime.go:281
			http.Error(w, "node has children; move or remove them first", http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}", func(w http.ResponseWriter, r *http.Request) {
		for _, n := range lifecycleNodes() {
			if n.ID == r.PathValue("id") {
				if n.ID == "l1" {
					n.LogoRef = "sha256:abc" // the childless repo carries a logo
				}
				_ = json.NewEncoder(w).Encode(n)
				return
			}
		}
		http.Error(w, "not found", http.StatusNotFound)
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}/stats", func(w http.ResponseWriter, r *http.Request) {
		out := apiclient.NodeRollup{}
		if r.PathValue("id") == "l1" {
			out = apiclient.NodeRollup{TotalMin: 750, WeekMin: 120, MonthMin: 300}
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}/artifacts", func(w http.ResponseWriter, r *http.Request) {
		// Mirrors usecase.ListArtifacts: the node's OWN artifacts plus an
		// ancestor's plus a free (node-less) one. Only the own ones are deleted.
		_ = json.NewEncoder(w).Encode([]domain.Artifact{
			{ID: "a1", NodeID: r.PathValue("id"), Slug: "own-1", Name: "own-1.png", Mime: "image/png"},
			{ID: "a2", NodeID: r.PathValue("id"), Slug: "own-2", Name: "own-2.png", Mime: "image/png"},
			{ID: "a3", NodeID: "e2", Slug: "ancestor", Name: "ancestor.png", Mime: "image/png"},
			{ID: "a4", NodeID: "", Slug: "free", Name: "free.png", Mime: "image/png"},
		})
	})
	mux.HandleFunc("GET /api/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		pid := r.URL.Query().Get("projectId")
		var out []domain.Document
		if pid == "v1" { // the blocked node also owns a project document
			nid := "v1"
			out = append(out, domain.Document{ID: "d1", NodeID: &nid, Type: domain.DocProject, Path: "projekt/rebuild", Title: "Rebuild"})
		}
		if pid == "l1" { // a non-project document: silently detached, not blocking
			nid := "l1"
			out = append(out, domain.Document{ID: "d2", NodeID: &nid, Type: domain.DocMemory, Path: "notes/x", Title: "X"})
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	return httptest.NewServer(mux)
}

func authedLifecycleServer(t *testing.T) (*mcp.ClientSession, *lifecycleRecorder) {
	t.Helper()
	rec := &lifecycleRecorder{}
	be := fakeLifecycleBackend(t, rec)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	_, h := managerFor(t, client, domain.Node{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo})
	return connect(t, h.srv), rec
}

func TestLoopback_MoveNode_Advertised(t *testing.T) {
	sess, _ := authedLifecycleServer(t)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTool(tools.Tools, "flow_move_node") {
		t.Fatalf("flow_move_node not advertised; got %v", toolNames(tools.Tools))
	}
}

func TestLoopback_MoveNode_ParentResolvesToIDAndSendsOneCall(t *testing.T) {
	sess, rec := authedLifecycleServer(t)

	res, out := callText(t, sess, "flow_move_node", map[string]any{"node": "jukebox", "parent": "zeta"})
	if res.IsError {
		t.Fatalf("move errored: %s", out)
	}
	got := rec.snapshot()
	if got.moveCalls != 1 || got.moveNodeID != "r1" {
		t.Fatalf("recorder = %+v, want exactly one move of r1", got)
	}
	if got.moveParent == nil || *got.moveParent != "e2" {
		t.Fatalf("parentId = %v, want the resolved id e2", got.moveParent)
	}
	if !strings.Contains(out, "Zeta") {
		t.Fatalf("result = %q, want it to name the destination", out)
	}
}

func TestLoopback_MoveNode_ToRootSendsNullParent(t *testing.T) {
	sess, rec := authedLifecycleServer(t)

	res, out := callText(t, sess, "flow_move_node", map[string]any{"node": "alpha", "to_root": true})
	if res.IsError {
		t.Fatalf("move to root errored: %s", out)
	}
	got := rec.snapshot()
	if got.moveCalls != 1 || got.moveNodeID != "e1" {
		t.Fatalf("recorder = %+v, want one move of e1", got)
	}
	if got.moveParent != nil {
		t.Fatalf("parentId = %v, want nil (to_root)", got.moveParent)
	}
	if !strings.Contains(out, "root") {
		t.Fatalf("result = %q, want it to say root", out)
	}
}

func TestLoopback_MoveNode_BothDestinationsIsAnErrorBeforeAnyCall(t *testing.T) {
	sess, rec := authedLifecycleServer(t)

	res, out := callText(t, sess, "flow_move_node", map[string]any{
		"node": "jukebox", "parent": "zeta", "to_root": true,
	})
	if !res.IsError || !strings.Contains(out, "exactly one destination") {
		t.Fatalf("both destinations = (IsError=%v, %q), want an exactly-one error", res.IsError, out)
	}
	if got := rec.snapshot(); got.moveCalls != 0 {
		t.Fatalf("moveCalls = %d, want 0", got.moveCalls)
	}
}

func TestLoopback_MoveNode_NoDestinationIsAnError(t *testing.T) {
	sess, rec := authedLifecycleServer(t)

	res, out := callText(t, sess, "flow_move_node", map[string]any{"node": "jukebox"})
	if !res.IsError || !strings.Contains(out, "exactly one destination") {
		t.Fatalf("no destination = (IsError=%v, %q), want an exactly-one error", res.IsError, out)
	}
	if got := rec.snapshot(); got.moveCalls != 0 {
		t.Fatalf("moveCalls = %d, want 0", got.moveCalls)
	}
}

func TestLoopback_MoveNode_RepoUnderRepoIsRejectedBeforeAnyCall(t *testing.T) {
	sess, rec := authedLifecycleServer(t)

	res, out := callText(t, sess, "flow_move_node", map[string]any{"node": "jukebox", "parent": "leaf"})
	if !res.IsError || !strings.Contains(out, "engagement or a vorhaben") {
		t.Fatalf("repo under repo = (IsError=%v, %q), want the kind guard", res.IsError, out)
	}
	if got := rec.snapshot(); got.moveCalls != 0 {
		t.Fatalf("moveCalls = %d, want 0", got.moveCalls)
	}
}

func TestLoopback_MoveNode_ToRootOnANonEngagementIsRejected(t *testing.T) {
	sess, rec := authedLifecycleServer(t)

	res, out := callText(t, sess, "flow_move_node", map[string]any{"node": "jukebox", "to_root": true})
	if !res.IsError || !strings.Contains(out, "only an engagement") {
		t.Fatalf("repo to root = (IsError=%v, %q), want the root guard (ValidParentKind allows only an engagement as root)", res.IsError, out)
	}
	if got := rec.snapshot(); got.moveCalls != 0 {
		t.Fatalf("moveCalls = %d, want 0", got.moveCalls)
	}
}

func TestLoopback_MoveNode_UnknownParentSaysParent(t *testing.T) {
	sess, _ := authedLifecycleServer(t)
	res, out := callText(t, sess, "flow_move_node", map[string]any{"node": "jukebox", "parent": "bogus"})
	if !res.IsError || !strings.Contains(out, "parent:") {
		t.Fatalf("unknown parent = (IsError=%v, %q), want a 'parent:'-prefixed message", res.IsError, out)
	}
}

// TestLoopback_MoveNode_ServerCycleConflictReachesTheModel proves the division of
// labour: the client pre-checks KINDS, the server owns cycle detection, and its
// 409 must arrive readable rather than as a bare status. A cycle cannot be
// provoked through the kind rules alone, so the fixture answers the move route
// the way nodemove.go:27 does.
func TestLoopback_MoveNode_ServerCycleConflictReachesTheModel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(lifecycleNodes())
	})
	mux.HandleFunc("POST /api/v1/nodes/{id}/move", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "move would create a cycle", http.StatusConflict)
	})
	be := httptest.NewServer(mux)
	t.Cleanup(be.Close)

	client := apiclient.New(be.URL, "tok")
	_, h := managerFor(t, client, domain.Node{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo})
	sess := connect(t, h.srv)

	// vorhaben under engagement is kind-legal, so the client lets it through and
	// only the server's 409 stops it.
	res, out := callText(t, sess, "flow_move_node", map[string]any{"node": "rebuild", "parent": "alpha"})
	if !res.IsError {
		t.Fatalf("server 409: want IsError, got %q", out)
	}
	if !strings.Contains(out, "cycle") {
		t.Fatalf("error = %q, want the server's 'move would create a cycle' message verbatim", out)
	}
}
