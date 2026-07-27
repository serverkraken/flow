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

func TestLoopback_DeleteNode_Advertised(t *testing.T) {
	sess, _ := authedLifecycleServer(t)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTool(tools.Tools, "flow_delete_node") {
		t.Fatalf("flow_delete_node not advertised; got %v", toolNames(tools.Tools))
	}
}

func TestLoopback_DeleteNode_WithoutConfirmReportsAndDeletesNothing(t *testing.T) {
	sess, rec := authedLifecycleServer(t)

	res, out := callText(t, sess, "flow_delete_node", map[string]any{"node": "leaf"})
	if res.IsError {
		t.Fatalf("dry run errored: %s", out)
	}
	if got := rec.snapshot(); got.deleteCalls != 0 {
		t.Fatalf("deleteCalls = %d, want 0 — a dry run must not delete", got.deleteCalls)
	}
	// The fixture gives l1 two own artifacts, one ancestor artifact, one free
	// artifact, a logo, and 750 minutes.
	if !strings.Contains(out, "2 artifact") {
		t.Fatalf("report = %q, want 2 own artifacts — the ancestor's and the free one must not be counted", out)
	}
	for _, want := range []string{"Would delete", "Leaf", "leaf", "12h 30m", "1 logo", "confirm=true"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(strings.ToLower(out), "session") {
		t.Errorf("report must speak of minutes, not sessions:\n%s", out)
	}
}

func TestLoopback_DeleteNode_NonProjectDocumentDoesNotBlock(t *testing.T) {
	sess, _ := authedLifecycleServer(t)
	// l1 owns a memory document; only project documents block deletion.
	res, out := callText(t, sess, "flow_delete_node", map[string]any{"node": "leaf"})
	if res.IsError {
		t.Fatalf("dry run errored: %s", out)
	}
	if strings.Contains(out, "Cannot delete") {
		t.Fatalf("a non-project document must not block deletion:\n%s", out)
	}
}

func TestLoopback_DeleteNode_ConfirmDeletesAndReportsIt(t *testing.T) {
	sess, rec := authedLifecycleServer(t)

	res, out := callText(t, sess, "flow_delete_node", map[string]any{"node": "leaf", "confirm": true})
	if res.IsError {
		t.Fatalf("confirmed delete errored: %s", out)
	}
	got := rec.snapshot()
	if got.deleteCalls != 1 || got.deleteID != "l1" {
		t.Fatalf("recorder = %+v, want exactly one delete of l1", got)
	}
	if !strings.Contains(out, "Deleted") || !strings.Contains(out, "leaf") {
		t.Fatalf("result = %q, want it to confirm the deletion", out)
	}
}

func TestLoopback_DeleteNode_ChildrenAndProjectDocsBlockTheDryRun(t *testing.T) {
	sess, rec := authedLifecycleServer(t)

	// v1/rebuild has the child r1 AND a project document.
	res, out := callText(t, sess, "flow_delete_node", map[string]any{"node": "rebuild"})
	if res.IsError {
		t.Fatalf("dry run of a blocked node must still be a normal report, got IsError: %s", out)
	}
	for _, want := range []string{"Cannot delete", "jukebox", "projekt/rebuild"} {
		if !strings.Contains(out, want) {
			t.Errorf("blocked report missing %q in:\n%s", want, out)
		}
	}
	if got := rec.snapshot(); got.deleteCalls != 0 {
		t.Fatalf("deleteCalls = %d, want 0", got.deleteCalls)
	}
}

// TestLoopback_DeleteNode_ConfirmOnABlockedNodeSurfacesTheServer409 is the
// error-path regression from Spec §7: the client-side report is advisory, the
// server is the authority, and its 409 must reach the model readably.
func TestLoopback_DeleteNode_ConfirmOnABlockedNodeSurfacesTheServer409(t *testing.T) {
	sess, rec := authedLifecycleServer(t)

	res, out := callText(t, sess, "flow_delete_node", map[string]any{"node": "rebuild", "confirm": true})
	if !res.IsError {
		t.Fatalf("confirmed delete of a node with children: want IsError, got %q", out)
	}
	if !strings.Contains(out, "children") {
		t.Fatalf("error = %q, want the server's 'node has children' message verbatim", out)
	}
	if got := rec.snapshot(); got.deleteCalls != 1 {
		t.Fatalf("deleteCalls = %d, want 1 (confirm must actually attempt it)", got.deleteCalls)
	}
}

// TestLoopback_DeleteNode_ProjectDocumentConflictSurfaces completes the 409
// matrix: besides the children conflict, the store refuses a node that still
// owns project documents (ports.ErrNodeHasProjectDocuments →
// internal/adapter/httpserver/worktime.go:284). The client report already warns,
// but a confirmed delete must surface the server's reason too.
func TestLoopback_DeleteNode_ProjectDocumentConflictSurfaces(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(lifecycleNodes())
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}", func(w http.ResponseWriter, r *http.Request) {
		for _, n := range lifecycleNodes() {
			if n.ID == r.PathValue("id") {
				_ = json.NewEncoder(w).Encode(n)
				return
			}
		}
		http.Error(w, "not found", http.StatusNotFound)
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}/stats", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.NodeRollup{})
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}/artifacts", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Artifact{})
	})
	// l1/leaf owns a project document but has no children.
	mux.HandleFunc("GET /api/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		var out []domain.Document
		if r.URL.Query().Get("projectId") == "l1" {
			nid := "l1"
			out = append(out, domain.Document{ID: "d9", NodeID: &nid, Type: domain.DocProject, Path: "projekt/leaf", Title: "Leaf"})
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("DELETE /api/v1/nodes/{id}", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "node has project documents; move or reclassify them first", http.StatusConflict)
	})
	be := httptest.NewServer(mux)
	t.Cleanup(be.Close)

	client := apiclient.New(be.URL, "tok")
	_, h := managerFor(t, client, domain.Node{ID: "l1", Name: "Leaf", Slug: "leaf", Kind: domain.KindRepo})
	sess := connect(t, h.srv)

	// The dry run must already call it out, without asking for confirm.
	res, out := callText(t, sess, "flow_delete_node", map[string]any{"node": "leaf"})
	if res.IsError {
		t.Fatalf("dry run errored: %s", out)
	}
	if !strings.Contains(out, "Cannot delete") || !strings.Contains(out, "projekt/leaf") {
		t.Fatalf("dry run = %q, want the project-document block named", out)
	}
	if strings.Contains(out, "confirm=true") {
		t.Fatalf("a blocked report must not invite confirm=true: %q", out)
	}

	// A confirmed delete anyway must surface the server's 409 verbatim.
	res, out = callText(t, sess, "flow_delete_node", map[string]any{"node": "leaf", "confirm": true})
	if !res.IsError {
		t.Fatalf("confirmed delete of a node with project documents: want IsError, got %q", out)
	}
	if !strings.Contains(out, "project documents") {
		t.Fatalf("error = %q, want the server's message verbatim", out)
	}
}

// TestLoopback_DeleteNode_MissingNodeIsAnError covers the outer of two guards:
// `node` carries no omitempty, so the MCP SDK declares it required and rejects
// the call during schema validation — the handler never runs. The assertion
// deliberately does not pin the SDK's wording, only that the call fails naming
// the node property and that nothing was deleted.
func TestLoopback_DeleteNode_MissingNodeIsAnError(t *testing.T) {
	sess, rec := authedLifecycleServer(t)
	res, out := callText(t, sess, "flow_delete_node", map[string]any{})
	if !res.IsError || !strings.Contains(out, "node") {
		t.Fatalf("missing node = (IsError=%v, %q), want a schema rejection naming node", res.IsError, out)
	}
	if got := rec.snapshot(); got.deleteCalls != 0 {
		t.Fatalf("deleteCalls = %d, want 0", got.deleteCalls)
	}
}

// TestLoopback_DeleteNode_BlankNodeIsAnError covers the inner guard, the one a
// schema cannot express: `node` is present but holds only whitespace. Here the
// handler's own message must reach the model.
func TestLoopback_DeleteNode_BlankNodeIsAnError(t *testing.T) {
	sess, rec := authedLifecycleServer(t)
	res, out := callText(t, sess, "flow_delete_node", map[string]any{"node": "   "})
	if !res.IsError || !strings.Contains(out, "node is required") {
		t.Fatalf("blank node = (IsError=%v, %q), want 'node is required'", res.IsError, out)
	}
	if got := rec.snapshot(); got.deleteCalls != 0 {
		t.Fatalf("deleteCalls = %d, want 0", got.deleteCalls)
	}
}
