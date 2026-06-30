package httpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
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

type cockpitTestServer struct {
	srv   *httpserver.Server
	ss    *testutil.FakeSessionStore
	ps    *testutil.FakeNodeStore
	bs    *testutil.FakeProjectBindingStore
	ds    *testutil.FakeDocumentStore
	ids   *testutil.FakeIDGen
	clk   testutil.FakeClock
	codec *websession.Codec
}

func newCockpitTestServer(t *testing.T) *cockpitTestServer {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 30, 12, 0, 0, 0, time.Local)}
	ids := &testutil.FakeIDGen{}
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeNodeStore()
	bs := testutil.NewFakeProjectBindingStore()
	ds := testutil.NewFakeDocumentStore()
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x", "Martin")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour)
	bus := sse.NewBus()
	settings := testutil.NewFakeUserSettingsStore()
	srv := &httpserver.Server{
		Ensure:            usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:               bus,
		Emitter:           sse.NewEmitter(bus, &fakeActivityStore{}, &testutil.FakeIDGen{}, clk),
		Clock:             clk,
		Users:             users,
		Session:           codec,
		StartSession:      usecase.StartSession{Sessions: ss, Nodes: ps, IDs: ids, Clock: clk},
		StopSession:       usecase.StopSession{Sessions: ss, Nodes: ps, Clock: clk},
		AddSession:        usecase.AddSession{Sessions: ss, Nodes: ps, IDs: ids, Clock: clk},
		ListSessionsRange: usecase.ListSessionsRange{Sessions: ss},
		GetRunningSession: usecase.GetRunningSession{Sessions: ss},
		GetNode:           usecase.GetNode{Nodes: ps},
		ListNodes:         usecase.ListNodes{Nodes: ps},
		NodeAncestors:     usecase.NodeAncestors{Nodes: ps},
		CreateNode:        usecase.CreateNode{Nodes: ps, IDs: ids, Clock: clk},
		UpdateNode:        usecase.UpdateNode{Nodes: ps, Clock: clk},
		ListNodeBindings:  usecase.ListNodeBindings{Bindings: bs},
		BindNode:          usecase.BindNode{Bindings: bs, Nodes: ps, IDs: ids, Clock: clk},
		UnbindNode:        usecase.UnbindNode{Bindings: bs},
		ListDocuments:     usecase.ListDocuments{Docs: ds},
		Stats: usecase.StatsComputer{
			Sessions: ss,
			Nodes:    ps, // REQUIRED for NodeStats subtree walk
			Settings: settings,
			Clock:    clk,
			Loc:      time.Local,
		},
	}
	return &cockpitTestServer{srv: srv, ss: ss, ps: ps, bs: bs, ds: ds, ids: ids, clk: clk, codec: codec}
}

func (c *cockpitTestServer) do(t *testing.T, method, path string, form map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var body *strings.Reader
	req, _ := http.NewRequest(method, path, strings.NewReader(""))
	if form != nil {
		vals := make([]string, 0, len(form))
		for k, v := range form {
			vals = append(vals, k+"="+v)
		}
		body = strings.NewReader(strings.Join(vals, "&"))
		req, _ = http.NewRequest(method, path, body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	cookieVal, _ := c.codec.Issue("u1")
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rec := httptest.NewRecorder()
	c.srv.Routes().ServeHTTP(rec, req)
	return rec
}

// seedNode inserts a node directly into the fake store.
func (c *cockpitTestServer) seedNode(t *testing.T, n domain.Node) {
	t.Helper()
	if n.Status == "" {
		n.Status = domain.NodeActive
	}
	if _, err := c.ps.Create(context.Background(), n); err != nil {
		t.Fatalf("seedNode: %v", err)
	}
}

func TestCockpitView_RollupAndIdentity(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo, Color: "cyan"})

	rec := c.do(t, "GET", "/nodes/n1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"flow", `id="cockpit-head"`, `id="cockpit-main"`, "Σ Gesamt"} {
		if !strings.Contains(body, want) {
			t.Errorf("cockpit missing %q", want)
		}
	}
	if rec2 := c.do(t, "GET", "/nodes/nope", nil); rec2.Code != http.StatusNotFound {
		t.Errorf("unknown id status=%d want 404", rec2.Code)
	}
}
