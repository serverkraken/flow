package httpserver_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/adapter/websession"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// newWebNodesServer builds a test server with the node usecases wired and a
// seeded user. Returns the server, a session cookie, and the fake node store.
func newWebNodesServer(t *testing.T) (*httptest.Server, *http.Cookie, *testutil.FakeNodeStore) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	ns := testutil.NewFakeNodeStore()
	bs := testutil.NewFakeProjectBindingStore()
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x.de", "M")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("test-secret-test-secret-test-12", time.Hour)
	ss := testutil.NewFakeSessionStore()
	docs := testutil.NewFakeDocumentStore()
	srv := &httpserver.Server{
		Users:   users,
		Session: codec,
		Bus:     sse.NewBus(),
		Clock:   clk,
		Ensure: usecase.EnsureUser{
			Users: users,
			IDs:   ids,
			Allow: func(ports.Identity) bool { return true },
		},
		CreateNode:        usecase.CreateNode{Nodes: ns, IDs: ids, Clock: clk},
		ListNodes:         usecase.ListNodes{Nodes: ns},
		GetNode:           usecase.GetNode{Nodes: ns},
		UpdateNode:        usecase.UpdateNode{Nodes: ns, Bindings: bs, IDs: ids, Clock: clk},
		DeleteNode:        usecase.DeleteNode{Nodes: ns},
		SetNodeRate:       usecase.SetNodeRate{Nodes: ns},
		MoveNode:          usecase.MoveNode{Nodes: ns},
		ListNodeBindings:  usecase.ListNodeBindings{Bindings: bs},
		ListSessionsRange: usecase.ListSessionsRange{Sessions: ss},
		ListDocuments:     usecase.ListDocuments{Docs: docs},
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cv, _ := codec.Issue("u1")
	return ts, &http.Cookie{Name: "flow_session", Value: cv}, ns
}

// seedTreeNode seeds a node with the given kind, name, and optional parent.
// It is distinct from the nodemove_test seedNode to allow a return value.
func seedTreeNode(t *testing.T, ns *testutil.FakeNodeStore, id, name string, kind domain.NodeKind, parent *string) domain.Node {
	t.Helper()
	now := time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)
	n, err := domain.NewNode(id, "u1", name, name, now)
	if err != nil {
		t.Fatalf("seedTreeNode NewNode: %v", err)
	}
	n.Kind = kind
	n.ParentID = parent
	n.Status = domain.NodeActive
	_, _ = ns.Create(context.Background(), n)
	return n
}

func getN(t *testing.T, ts *httptest.Server, c *http.Cookie, path string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest("GET", ts.URL+path, nil)
	req.AddCookie(c)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func postN(t *testing.T, ts *httptest.Server, c *http.Cookie, path string, form url.Values) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", ts.URL+path, strings.NewReader(form.Encode()))
	req.AddCookie(c)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	cl := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := cl.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestWebNodeTree_IndentAndFilter(t *testing.T) {
	ts, c, ns := newWebNodesServer(t)

	// Seed: an engagement with a child repo (has color+glyph+upstreamGit), and an archived engagement.
	eng := seedTreeNode(t, ns, "eng1", "Privat", domain.KindEngagement, nil)
	repo := seedTreeNode(t, ns, "repo1", "flow", domain.KindRepo, &eng.ID)
	repo.Color = domain.NodeColors[0]
	repo.Glyph = domain.NodeGlyphs[0]
	repo.UpstreamGit = "git@github.com:serverkraken/flow.git"
	_, _ = ns.Update(context.Background(), "u1", repo)
	arch := seedTreeNode(t, ns, "eng2", "Alt", domain.KindEngagement, nil)
	arch.Status = domain.NodeArchived
	_, _ = ns.Update(context.Background(), "u1", arch)

	// Default view: GET /nodes.
	code, body := getN(t, ts, c, "/nodes")
	if code != 200 {
		t.Fatalf("GET /nodes = %d; body=%.500s", code, body)
	}
	// Engagement + child repo must appear, with kind badge labels.
	for _, want := range []string{"Privat", "flow", "Engagement", "Repo"} {
		if !strings.Contains(body, want) {
			t.Errorf("tree page missing %q; body=%.500s", want, body)
		}
	}
	// Archived node must be hidden by default.
	if strings.Contains(body, "Alt") {
		t.Errorf("default view must hide archived; body=%.500s", body)
	}

	// Archived filter: GET /nodes?status=archived.
	_, arr := getN(t, ts, c, "/nodes?status=archived")
	if !strings.Contains(arr, "Alt") {
		t.Errorf("archived filter must show Alt; body=%.500s", arr)
	}

	// SSE fragment route must return 200 and render indented child node.
	code, frag := getN(t, ts, c, "/ui/nodes/list")
	if code != 200 {
		t.Errorf("GET /ui/nodes/list = %d, want 200", code)
	}
	if !strings.Contains(frag, "padding-left:1rem") {
		t.Errorf("fragment missing child indentation style padding-left:1rem; body=%.500s", frag)
	}
}

// Ensure postN compiles (coverage guard).
var _ = postN

// TestWebNodeFormErrorPaths covers handler error branches: 404s and ErrNodeHasChildren.
func TestWebNodeFormErrorPaths(t *testing.T) {
	ts, c, ns := newWebNodesServer(t)

	// Seed: engagement with a child repo.
	eng := seedTreeNode(t, ns, "eng1", "Privat", domain.KindEngagement, nil)
	seedTreeNode(t, ns, "repo1", "flow", domain.KindRepo, &eng.ID)

	// GET /nodes/{missing}/edit → 404
	code, _ := getN(t, ts, c, "/nodes/missing/edit")
	if code != http.StatusNotFound {
		t.Errorf("edit 404 = %d, want 404", code)
	}

	// POST /nodes/{missing} (update) → 404
	res := postN(t, ts, c, "/nodes/missing", url.Values{"name": {"x"}, "status": {"active"}})
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("update 404 = %d, want 404", res.StatusCode)
	}
	_ = res.Body.Close()

	// POST /nodes/{missing}/status → 404
	res = postN(t, ts, c, "/nodes/missing/status", url.Values{"status": {"archived"}})
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status 404 = %d, want 404", res.StatusCode)
	}
	_ = res.Body.Close()

	// POST /nodes/{id}/delete with children → redirect with err=children
	res = postN(t, ts, c, "/nodes/"+eng.ID+"/delete", url.Values{})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete-with-children = %d, want 303", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	_ = res.Body.Close()
	if !strings.Contains(loc, "err=children") {
		t.Errorf("delete-with-children redirect = %q, want ?err=children", loc)
	}
	// Engagement still exists (not deleted).
	if _, err := ns.Get(context.Background(), "u1", eng.ID); err != nil {
		t.Errorf("engagement should still exist after failed delete: %v", err)
	}
}

// TestWebNodeForm exercises the NodeForm-based create / edit / status / delete
// handlers (D4): kind select, constrained parent, engagement-only rate.
func TestWebNodeForm(t *testing.T) {
	ts, c, ns := newWebNodesServer(t)

	// Seed an engagement to appear as a parent candidate.
	eng := seedTreeNode(t, ns, "eng1", "RTL Extern", domain.KindEngagement, nil)

	// ── GET /nodes/new: form must contain the kind select and the parent option ──
	code, form := getN(t, ts, c, "/nodes/new")
	if code != 200 {
		t.Fatalf("GET /nodes/new = %d; body=%.500s", code, form)
	}
	if !strings.Contains(form, `select name="kind"`) {
		t.Errorf("new-form must contain kind select; body=%.500s", form)
	}
	if !strings.Contains(form, "RTL Extern") {
		t.Errorf("new-form must list parent engagement; body=%.500s", form)
	}

	// ── POST /nodes: create a repo under the engagement ──
	res := postN(t, ts, c, "/nodes", url.Values{
		"name":     {"flow"},
		"slug":     {"flow"},
		"kind":     {"repo"},
		"parentId": {eng.ID},
		"color":    {domain.NodeColors[0]},
		"glyph":    {domain.NodeGlyphs[0]},
		"status":   {"active"},
	})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("create repo = %d, want 303", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	_ = res.Body.Close()
	if !strings.HasPrefix(loc, "/nodes/") {
		t.Fatalf("create redirect = %q, want /nodes/...", loc)
	}
	repoID := strings.TrimPrefix(loc, "/nodes/")

	got, err := ns.Get(context.Background(), "u1", repoID)
	if err != nil {
		t.Fatalf("created node not found: %v", err)
	}
	if got.Kind != domain.KindRepo {
		t.Errorf("created node kind = %q, want repo", got.Kind)
	}
	if got.ParentID == nil || *got.ParentID != eng.ID {
		t.Errorf("created node parentID = %v, want %q", got.ParentID, eng.ID)
	}

	// ── GET /nodes/{id}/edit: form pre-fills kind and name ──
	_, editBody := getN(t, ts, c, "/nodes/"+repoID+"/edit")
	if !strings.Contains(editBody, "flow") {
		t.Errorf("edit form must pre-fill node name; body=%.500s", editBody)
	}
	if !strings.Contains(editBody, `value="repo"`) {
		t.Errorf("edit form must pre-fill kind=repo; body=%.500s", editBody)
	}

	// ── GET /nodes: tree renders color/glyph swatch for node with those fields ──
	_, treePage := getN(t, ts, c, "/nodes")
	if !strings.Contains(treePage, "flow") {
		t.Errorf("tree should show created repo; body=%.300s", treePage)
	}

	// ── POST /nodes: create engagement with rate ──
	res = postN(t, ts, c, "/nodes", url.Values{
		"name":         {"Beratung"},
		"kind":         {"engagement"},
		"rateAmount":   {"95.00"},
		"rateCurrency": {"EUR"},
	})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("create engagement = %d, want 303", res.StatusCode)
	}
	eid := strings.TrimPrefix(res.Header.Get("Location"), "/nodes/")
	_ = res.Body.Close()
	e2, err2 := ns.Get(context.Background(), "u1", eid)
	if err2 != nil {
		t.Fatalf("created engagement not found: %v", err2)
	}
	if e2.Rate == nil || e2.Rate.Amount != 9500 {
		t.Errorf("engagement rate not set: %+v", e2.Rate)
	}

	// ── POST /nodes (missing name) → 400 re-render ──
	res = postN(t, ts, c, "/nodes", url.Values{"kind": {"repo"}, "parentId": {eng.ID}})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing name = %d, want 400", res.StatusCode)
	}
	_ = res.Body.Close()

	// ── POST /nodes/{id}/status → archive repo ──
	res = postN(t, ts, c, "/nodes/"+repoID+"/status", url.Values{"status": {"archived"}})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", res.StatusCode)
	}
	_ = res.Body.Close()
	got, _ = ns.Get(context.Background(), "u1", repoID)
	if got.Status != domain.NodeArchived {
		t.Errorf("not archived: %s", got.Status)
	}

	// ── POST /nodes/{id}/delete (leaf) → deleted ──
	res = postN(t, ts, c, "/nodes/"+repoID+"/delete", url.Values{})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete = %d, want 303", res.StatusCode)
	}
	_ = res.Body.Close()
	if _, err := ns.Get(context.Background(), "u1", repoID); err == nil {
		t.Errorf("node should be deleted")
	}
}
