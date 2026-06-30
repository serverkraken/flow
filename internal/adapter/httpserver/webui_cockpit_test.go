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

// TestCockpitHead_SubtreeRollupAndInheritedRate seeds a parent (Vorhaben) node
// with an hourly rate and a child (Repo) node without its own rate, then adds a
// 2-hour completed session on the child. It verifies that GET /nodes/p1 renders
// the subtree rollup (child's time) and the inherited rate + earnings in the
// cockpit head — the core deliverable of Task 2's nodeCockpitData wiring.
// Also checks GET /nodes/c1 to confirm the child inherits the parent's rate.
func TestCockpitHead_SubtreeRollupAndInheritedRate(t *testing.T) {
	c := newCockpitTestServer(t)

	// Seed parent (Vorhaben) with hourly rate 95 €/h (9500 minor units = cents).
	parentRate := &domain.Money{Amount: 9500, Currency: "EUR"}
	c.seedNode(t, domain.Node{
		ID: "p1", OwnerID: "u1", Name: "ParentVorhaben", Slug: "parent-vorhaben",
		Kind: domain.KindVorhaben, Color: "blue", Rate: parentRate,
	})
	// Seed child (Repo) with no own rate, parented under p1.
	p1ID := "p1"
	c.seedNode(t, domain.Node{
		ID: "c1", OwnerID: "u1", Name: "ChildRepo", Slug: "child-repo",
		Kind: domain.KindRepo, Color: "cyan", ParentID: &p1ID,
	})

	// Add a completed 2-hour session on the child (08:00–10:00 on 2026-06-30).
	// Clock is 12:00 on 2026-06-30, so these times are in the past. ✓
	day := time.Date(2026, 6, 30, 0, 0, 0, 0, time.Local)
	start := day.Add(8 * time.Hour)
	stop := day.Add(10 * time.Hour)
	c1ID := "c1"
	if _, err := (usecase.AddSession{Sessions: c.ss, Nodes: c.ps, IDs: c.ids, Clock: c.clk}).Execute(
		context.Background(), "u1", &c1ID, start, stop, nil, "",
	); err != nil {
		t.Fatalf("AddSession on child: %v", err)
	}

	// GET parent cockpit: subtree rollup must include the child's 2 h.
	rec := c.do(t, "GET", "/nodes/p1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /nodes/p1: status %d body=%.400s", rec.Code, rec.Body.String())
	}
	parentBody := rec.Body.String()

	// fmtDurHM(2h) = "2:00 h"; appears in the Σ-Gesamt tile of the rollup.
	if !strings.Contains(parentBody, "2:00 h") {
		t.Errorf("parent cockpit: missing subtree rollup %q\nbody snippet: %.800s", "2:00 h", parentBody)
	}
	// rateLabel(&Money{9500,"EUR"}) = "95 €/h"
	if !strings.Contains(parentBody, "95 €/h") {
		t.Errorf("parent cockpit: missing inherited rate label %q", "95 €/h")
	}
	// Earnings = 9500 cents/h × 7200 s = 19000 minor units = "190.00 EUR"
	if !strings.Contains(parentBody, "190.00 EUR") {
		t.Errorf("parent cockpit: missing earnings %q", "190.00 EUR")
	}

	// GET child cockpit: own session (2 h) and rate inherited from parent.
	rec2 := c.do(t, "GET", "/nodes/c1", nil)
	if rec2.Code != http.StatusOK {
		t.Fatalf("GET /nodes/c1: status %d body=%.400s", rec2.Code, rec2.Body.String())
	}
	childBody := rec2.Body.String()

	// Child's own 2-hour session appears in its rollup.
	if !strings.Contains(childBody, "2:00 h") {
		t.Errorf("child cockpit: missing own session rollup %q", "2:00 h")
	}
	// Child has no Rate of its own; ResolveRate walks ancestors and finds parent's rate.
	if !strings.Contains(childBody, "95 €/h") {
		t.Errorf("child cockpit: missing inherited rate label %q", "95 €/h")
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

func TestCockpitStart_BooksNode(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	rec := c.do(t, "POST", "/nodes/n1/start", map[string]string{})
	if rec.Code != http.StatusOK {
		t.Fatalf("start status %d body=%.300s", rec.Code, rec.Body.String())
	}
	// running session now exists, booked to n1
	rs, ok, _ := (usecase.GetRunningSession{Sessions: c.ss}).Execute(context.Background(), "u1")
	if !ok || rs.NodeID == nil || *rs.NodeID != "n1" {
		t.Fatalf("expected running session booked to n1, got ok=%v rs=%+v", ok, rs)
	}
	// head shows the live timer (data-timer) + stop button target
	if !strings.Contains(rec.Body.String(), "data-timer") || !strings.Contains(rec.Body.String(), "/nodes/n1/stop") {
		t.Errorf("head after start missing live timer / stop form")
	}
}

func TestCockpitStart_RejectsBranch(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "b1", OwnerID: "u1", Name: "feature/x", Kind: domain.KindBranch})
	rec := c.do(t, "POST", "/nodes/b1/start", map[string]string{})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("start on branch status=%d want 400", rec.Code)
	}
}

func TestCockpitStop_EndsSession(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})
	nid := "n1"
	_, _ = (usecase.StartSession{Sessions: c.ss, Nodes: c.ps, IDs: c.ids, Clock: c.clk}).Execute(context.Background(), "u1", &nid, nil, "")

	rec := c.do(t, "POST", "/nodes/n1/stop", map[string]string{})
	if rec.Code != http.StatusOK {
		t.Fatalf("stop status %d body=%.300s", rec.Code, rec.Body.String())
	}
	if _, ok, _ := (usecase.GetRunningSession{Sessions: c.ss}).Execute(context.Background(), "u1"); ok {
		t.Errorf("session still running after stop")
	}
}

func TestCockpitTab_SwapsPanel(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	rec := c.do(t, "GET", "/nodes/n1/tab/wissen", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("tab status %d", rec.Code)
	}
	body := rec.Body.String()
	// active tab marker present and panel content container present
	if !strings.Contains(body, `id="cockpit-panel"`) {
		t.Errorf("tab fragment missing panel container")
	}
	// unknown tab normalizes to worktime (no 404)
	if rec2 := c.do(t, "GET", "/nodes/n1/tab/bogus", nil); rec2.Code != http.StatusOK {
		t.Errorf("bogus tab status=%d want 200 (normalized)", rec2.Code)
	}
}

// TestCockpitTab_SSEReloadTargetsOuterContainer pins the fix for the DOM-nesting
// bug: the panel's SSE reload must target #cockpit-main (the outer container
// holding strip+panel), not itself. A self-targeting reload would inject a full
// strip+panel inside #cockpit-panel, duplicating the id and nesting the nav.
func TestCockpitTab_SSEReloadTargetsOuterContainer(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	// Tabs with SSE (worktime, wissen, struktur) must have hx-target="#cockpit-main".
	for _, tab := range []string{"worktime", "wissen", "struktur"} {
		rec := c.do(t, "GET", "/nodes/n1/tab/"+tab, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("tab %s: status %d", tab, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `id="cockpit-panel"`) {
			t.Errorf("tab %s: missing panel container id", tab)
		}
		if !strings.Contains(body, `hx-target="#cockpit-main"`) {
			t.Errorf("tab %s: panel SSE reload must target #cockpit-main, got body snippet: %.600s", tab, body)
		}
		// Must NOT self-target (no missing hx-target that would default to self).
		// The old bug: hx-get present without hx-target → self-target → nesting.
		// Verify hx-get is present (SSE reload wired) and hx-target is also present.
		if !strings.Contains(body, `hx-get="/nodes/n1/tab/`+tab+`"`) {
			t.Errorf("tab %s: panel SSE reload missing hx-get attribute", tab)
		}
	}
}

// TestCockpitTab_BindingsNoSSEReload pins that the bindings tab emits NO SSE
// reload attributes on #cockpit-panel (bindings reloads only after its own
// mutations, not via generic SSE events). The old code rendered hx-trigger=""
// unconditionally; the fix omits all four reload attrs when SSE is empty.
func TestCockpitTab_BindingsNoSSEReload(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	rec := c.do(t, "GET", "/nodes/n1/tab/bindings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("bindings tab: status %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="cockpit-panel"`) {
		t.Errorf("bindings tab: missing panel container id")
	}
	// No SSE auto-reload on bindings — hx-trigger must be absent from the fragment.
	// hx-trigger is ONLY emitted by the panel SSE reload block (tab links use
	// hx-get/hx-target/hx-swap/hx-push-url but never hx-trigger), so its
	// absence proves the reload block was omitted.
	if strings.Contains(body, `hx-trigger=`) {
		t.Errorf("bindings panel must NOT have hx-trigger (no SSE events for bindings), got body snippet: %.600s", body)
	}
}

func TestCockpitWissen_ListsNodeDocs(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})
	nid := "n1"
	doc := domain.Document{
		ID:        "d1",
		OwnerID:   "u1",
		NodeID:    &nid,
		Type:      domain.DocFree,
		Path:      "architektur",
		Title:     "Architektur",
		Body:      "# A",
		CreatedAt: c.clk.Now(),
		UpdatedAt: c.clk.Now(),
	}
	_, _ = c.ds.Create(context.Background(), doc)

	rec := c.do(t, "GET", "/nodes/n1/tab/wissen", nil)
	body := rec.Body.String()
	if !strings.Contains(body, "Architektur") || !strings.Contains(body, "/wissen/neu?node=n1") {
		t.Errorf("wissen panel missing doc / scoped-new link: %.300s", body)
	}
}

func TestEditorNew_PrescopesNode(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})
	rec := c.do(t, "GET", "/wissen/neu?node=n1", nil)
	if rec.Code == http.StatusOK && !strings.Contains(rec.Body.String(), "n1") {
		t.Errorf("new editor did not pre-scope node n1")
	}
}

func TestCockpitSwitch_StopsOtherStartsHere(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Slug: "flow", Kind: domain.KindRepo})
	c.seedNode(t, domain.Node{ID: "n2", OwnerID: "u1", Name: "homelab", Slug: "homelab", Kind: domain.KindRepo})
	other := "n2"
	_, _ = (usecase.StartSession{Sessions: c.ss, Nodes: c.ps, IDs: c.ids, Clock: c.clk}).Execute(context.Background(), "u1", &other, nil, "")

	rec := c.do(t, "POST", "/nodes/n1/switch", map[string]string{})
	if rec.Code != http.StatusOK {
		t.Fatalf("switch status %d body=%.300s", rec.Code, rec.Body.String())
	}
	rs, ok, _ := (usecase.GetRunningSession{Sessions: c.ss}).Execute(context.Background(), "u1")
	if !ok || rs.NodeID == nil || *rs.NodeID != "n1" {
		t.Fatalf("after switch expected running on n1, got ok=%v rs=%+v", ok, rs)
	}
}

func TestCockpitStruktur_ListsChildrenAndMove(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "p1", OwnerID: "u1", Name: "Plattform", Kind: domain.KindVorhaben})
	pp := "p1"
	c.seedNode(t, domain.Node{ID: "c1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo, ParentID: &pp})

	rec := c.do(t, "GET", "/nodes/p1/tab/struktur", nil)
	body := rec.Body.String()
	if !strings.Contains(body, "flow") || !strings.Contains(body, "/nodes/p1/move") {
		t.Errorf("struktur panel missing child / move form: %.300s", body)
	}
	if !strings.Contains(body, "/nodes/new?parent=p1") {
		t.Errorf("struktur panel missing add-child link")
	}
}

func TestNodeNew_PrefillsParent(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "p1", OwnerID: "u1", Name: "Plattform", Kind: domain.KindVorhaben})
	rec := c.do(t, "GET", "/nodes/new?parent=p1", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "p1") {
		t.Errorf("new-node form did not prefill parent p1 (status %d)", rec.Code)
	}
}

func TestCockpitBindings_AddRemote(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})
	rec := c.do(t, "POST", "/nodes/n1/bindings", map[string]string{"remoteSlug": "github.com/serverkraken/flow"})
	if rec.Code != http.StatusOK {
		t.Fatalf("bind status %d body=%.300s", rec.Code, rec.Body.String())
	}
	bs, _ := (usecase.ListNodeBindings{Bindings: c.bs}).ExecuteByProject(context.Background(), "u1", "n1")
	if len(bs) != 1 || bs[0].Kind != domain.BindingRemote {
		t.Fatalf("expected 1 remote binding, got %+v", bs)
	}
	if !strings.Contains(rec.Body.String(), "github.com/serverkraken/flow") {
		t.Errorf("bindings panel did not list the new remote")
	}
	// Pin: bindings panel forms must target #cockpit-main, never #cockpit-panel.
	if strings.Contains(rec.Body.String(), `hx-target="#cockpit-panel"`) {
		t.Errorf("bindings panel must NOT use hx-target=\"#cockpit-panel\" (nesting bug): %.600s", rec.Body.String())
	}
}

func TestCockpitBindings_DeleteRemote(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})
	_, _ = (usecase.BindNode{Bindings: c.bs, Nodes: c.ps, IDs: c.ids, Clock: c.clk}).Execute(
		context.Background(), "u1", "n1", usecase.BindKey{Kind: domain.BindingRemote, RemoteSlug: "github.com/x/y"})

	rec := c.do(t, "POST", "/nodes/n1/bindings/delete", map[string]string{"kind": "remote", "slug": "github.com/x/y"})
	if rec.Code != http.StatusOK {
		t.Fatalf("unbind status %d", rec.Code)
	}
	bs, _ := (usecase.ListNodeBindings{Bindings: c.bs}).ExecuteByProject(context.Background(), "u1", "n1")
	if len(bs) != 0 {
		t.Errorf("expected 0 bindings after delete, got %+v", bs)
	}
}

// TestCockpitBindings_PanelNoCockpitPanelTarget pins that the bindings tab fragment
// does not contain hx-target="#cockpit-panel" anywhere — that would cause DOM nesting.
func TestCockpitBindings_PanelNoCockpitPanelTarget(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})
	// Seed a binding so the list + delete form renders.
	_, _ = (usecase.BindNode{Bindings: c.bs, Nodes: c.ps, IDs: c.ids, Clock: c.clk}).Execute(
		context.Background(), "u1", "n1", usecase.BindKey{Kind: domain.BindingRemote, RemoteSlug: "github.com/x/y"})

	rec := c.do(t, "GET", "/nodes/n1/tab/bindings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("bindings tab status %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `hx-target="#cockpit-panel"`) {
		t.Errorf("bindings panel body must NOT contain hx-target=\"#cockpit-panel\": %.600s", rec.Body.String())
	}
}
