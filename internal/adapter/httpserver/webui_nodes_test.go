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

	// Seed: an engagement with a child repo, and an archived engagement.
	eng := seedTreeNode(t, ns, "eng1", "Privat", domain.KindEngagement, nil)
	seedTreeNode(t, ns, "repo1", "flow", domain.KindRepo, &eng.ID)
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
