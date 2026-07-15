package httpserver_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func decodeJSON(r io.Reader, v any) error {
	return json.NewDecoder(r).Decode(v)
}

func newProjectsSrv(t *testing.T) (*httptest.Server, func(method, path, body string) *http.Response, *testutil.FakeProjectBindingStore) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	ps := testutil.NewFakeNodeStore()
	bs := testutil.NewFakeProjectBindingStore()
	users := testutil.NewFakeUserStore()

	bus := sse.NewBus()
	srv := &httpserver.Server{
		Verifier:   testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:     usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:        bus,
		Emitter:    sse.NewEmitter(bus, &fakeActivityStore{}, ids, clk),
		Clock:      clk,
		CreateNode: usecase.CreateNode{Nodes: ps, IDs: ids, Clock: clk},
		ListNodes:  usecase.ListNodes{Nodes: ps},
		DeleteNode: usecase.DeleteNode{Nodes: ps},
		GetNode:    usecase.GetNode{Nodes: ps},
		UpdateNode: usecase.UpdateNode{Nodes: ps, Clock: clk},
	}

	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	do := func(method, path, body string) *http.Response {
		req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer x")
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	return ts, do, bs
}

func TestDeleteProject_204AndGone(t *testing.T) {
	_, do, _ := newProjectsSrv(t)

	// Create a project first.
	res := do("POST", "/api/v1/nodes", `{"name":"Testprojekt","kind":"engagement"}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create project status %d, want 201", res.StatusCode)
	}
	var p domain.Node
	if err := decodeJSON(res.Body, &p); err != nil {
		t.Fatalf("decode project: %v", err)
	}
	_ = res.Body.Close()

	// DELETE the project → 204.
	res = do("DELETE", "/api/v1/nodes/"+p.ID, "")
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status %d, want 204", res.StatusCode)
	}

	// GET the list → project must be gone.
	res = do("GET", "/api/v1/nodes", "")
	var list []domain.Node
	if err := decodeJSON(res.Body, &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	_ = res.Body.Close()
	for _, proj := range list {
		if proj.ID == p.ID {
			t.Fatalf("deleted project still in list: %+v", proj)
		}
	}
}

func TestDeleteProject_UnknownID404(t *testing.T) {
	_, do, _ := newProjectsSrv(t)
	res := do("DELETE", "/api/v1/nodes/does-not-exist", "")
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown id status %d, want 404", res.StatusCode)
	}
}

// remoteSlugs returns all remote binding slugs stored in bs (across all owners).
func remoteSlugs(bs *testutil.FakeProjectBindingStore) []string {
	var out []string
	for _, b := range bs.All() {
		if b.Kind == domain.BindingRemote {
			out = append(out, b.RemoteSlug)
		}
	}
	return out
}

func TestUpdateAndGetProjectRoutes(t *testing.T) {
	_, do, bs := newProjectsSrv(t)

	// Canonical upstream identity is stored on the node; explicit bindings are
	// separate aliases/overrides and are not synthesized by create.
	res := do("POST", "/api/v1/nodes", `{"name":"Flow","kind":"engagement","upstreamGit":"git@github.com:serverkraken/flow.git"}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create status %d, want 201", res.StatusCode)
	}
	var created map[string]any
	if err := decodeJSON(res.Body, &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	_ = res.Body.Close()
	id := created["id"].(string)
	if got := remoteSlugs(bs); len(got) != 0 {
		t.Fatalf("create-with-upstream synthesized an explicit binding: %v", got)
	}

	// GET one
	res = do("GET", "/api/v1/nodes/"+id, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET status %d, want 200", res.StatusCode)
	}
	var one map[string]any
	if err := decodeJSON(res.Body, &one); err != nil {
		t.Fatalf("decode one: %v", err)
	}
	_ = res.Body.Close()
	if one["upstreamGit"] != "git@github.com:serverkraken/flow.git" {
		t.Errorf("GET returned %v", one)
	}

	// PATCH → pause + change description
	res = do("PATCH", "/api/v1/nodes/"+id,
		`{"name":"Flow","slug":"flow","description":"hi","upstreamGit":"git@github.com:serverkraken/flow.git","status":"paused"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PATCH status %d, want 200", res.StatusCode)
	}
	var upd map[string]any
	if err := decodeJSON(res.Body, &upd); err != nil {
		t.Fatalf("decode upd: %v", err)
	}
	_ = res.Body.Close()
	if upd["status"] != "paused" || upd["description"] != "hi" {
		t.Errorf("PATCH returned %v", upd)
	}

	// PATCH bad upstream → 400
	res = do("PATCH", "/api/v1/nodes/"+id,
		`{"name":"Flow","slug":"flow","status":"active","upstreamGit":"garbage"}`)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad upstream status %d, want 400", res.StatusCode)
	}

	// PATCH unknown id → 404
	res = do("PATCH", "/api/v1/nodes/missing",
		`{"name":"X","slug":"x","status":"active"}`)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown id status %d, want 404", res.StatusCode)
	}
}

// TestCreateAndUpdateNode_IconRoundTrips pins REST parity for the icon field
// (cockpit-story Task 5): create persists icon and upstream in the same node
// write, and a PATCH can change the icon.
func TestCreateAndUpdateNode_IconRoundTrips(t *testing.T) {
	_, do, _ := newProjectsSrv(t)

	// Create with an icon and upstreamGit in one request.
	res := do("POST", "/api/v1/nodes",
		`{"name":"Iconic","kind":"engagement","icon":"rocket","upstreamGit":"git@github.com:serverkraken/flow.git"}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create status %d, want 201", res.StatusCode)
	}
	var created map[string]any
	if err := decodeJSON(res.Body, &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	_ = res.Body.Close()
	id := created["id"].(string)
	if created["icon"] != "rocket" {
		t.Fatalf("create response icon = %v, want rocket", created["icon"])
	}

	// GET confirms the icon persisted (not just echoed on create response).
	res = do("GET", "/api/v1/nodes/"+id, "")
	var one map[string]any
	if err := decodeJSON(res.Body, &one); err != nil {
		t.Fatalf("decode one: %v", err)
	}
	_ = res.Body.Close()
	if one["icon"] != "rocket" {
		t.Fatalf("GET icon = %v, want rocket", one["icon"])
	}

	// PATCH with a different icon → response reflects the new value.
	res = do("PATCH", "/api/v1/nodes/"+id,
		`{"name":"Iconic","slug":"iconic","status":"active","icon":"flask-conical"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PATCH status %d, want 200", res.StatusCode)
	}
	var upd map[string]any
	if err := decodeJSON(res.Body, &upd); err != nil {
		t.Fatalf("decode upd: %v", err)
	}
	_ = res.Body.Close()
	if upd["icon"] != "flask-conical" {
		t.Errorf("PATCH icon = %v, want flask-conical", upd["icon"])
	}
}

// TestHandleUpdateNode_PartialBodyKeepsIcon pins the pointer-DTO partial
// semantics (Task 2): a PATCH body that only sets "description" must leave
// unrelated fields (icon) untouched, since Task 1's UpdateNodeInput treats an
// absent JSON key as nil → field skipped.
func TestHandleUpdateNode_PartialBodyKeepsIcon(t *testing.T) {
	_, do, _ := newProjectsSrv(t)

	res := do("POST", "/api/v1/nodes", `{"name":"R","kind":"engagement","icon":"rocket"}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create status %d, want 201", res.StatusCode)
	}
	var created map[string]any
	if err := decodeJSON(res.Body, &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	_ = res.Body.Close()
	id := created["id"].(string)

	res = do("PATCH", "/api/v1/nodes/"+id, `{"description":"neu"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PATCH status %d, want 200", res.StatusCode)
	}
	var got map[string]any
	if err := decodeJSON(res.Body, &got); err != nil {
		t.Fatalf("decode got: %v", err)
	}
	_ = res.Body.Close()
	if got["icon"] != "rocket" {
		t.Errorf("icon = %v, want preserved rocket", got["icon"])
	}
	if got["description"] != "neu" {
		t.Errorf("description = %v, want neu", got["description"])
	}
}

func TestListProjectsStatusFilter(t *testing.T) {
	_, do, _ := newProjectsSrv(t)

	res := do("POST", "/api/v1/nodes", `{"name":"Aaa","kind":"engagement"}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create Aaa status %d, want 201", res.StatusCode)
	}
	var a map[string]any
	if err := decodeJSON(res.Body, &a); err != nil {
		t.Fatalf("decode a: %v", err)
	}
	_ = res.Body.Close()

	res = do("POST", "/api/v1/nodes", `{"name":"Bbb","kind":"engagement"}`)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create Bbb status %d, want 201", res.StatusCode)
	}

	// archive Aaa
	res = do("PATCH", "/api/v1/nodes/"+a["id"].(string),
		`{"name":"Aaa","slug":"aaa","status":"archived"}`)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("archive status %d, want 200", res.StatusCode)
	}

	// no filter → all
	res = do("GET", "/api/v1/nodes", "")
	var all []map[string]any
	if err := decodeJSON(res.Body, &all); err != nil {
		t.Fatalf("decode all: %v", err)
	}
	_ = res.Body.Close()
	if len(all) != 2 {
		t.Errorf("no filter → all, got %d", len(all))
	}

	// status=archived → 1
	res = do("GET", "/api/v1/nodes?status=archived", "")
	var arch []map[string]any
	if err := decodeJSON(res.Body, &arch); err != nil {
		t.Fatalf("decode arch: %v", err)
	}
	_ = res.Body.Close()
	if len(arch) != 1 {
		t.Errorf("status=archived → 1, got %d", len(arch))
	}

	// status=active,paused → 1 (Bbb is active)
	res = do("GET", "/api/v1/nodes?status=active,paused", "")
	var act []map[string]any
	if err := decodeJSON(res.Body, &act); err != nil {
		t.Fatalf("decode act: %v", err)
	}
	_ = res.Body.Close()
	if len(act) != 1 {
		t.Errorf("status=active,paused → 1, got %d", len(act))
	}
}

// TestGetProject_NotFound covers the ErrNodeNotFound branch of handleGetNode.
func TestGetProject_NotFound(t *testing.T) {
	_, do, _ := newProjectsSrv(t)
	// GET a non-existent project ID → 404.
	res := do("GET", "/api/v1/nodes/does-not-exist", "")
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /api/v1/nodes/does-not-exist: status=%d, want 404", res.StatusCode)
	}
}
