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
	tags  *testutil.FakeTagStore
	as    *fakeActivityStore
	arts  *testutil.FakeArtifactStore
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
	tags := testutil.NewFakeTagStore()
	as := &fakeActivityStore{}
	arts := testutil.NewFakeArtifactStore()
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
		SwitchSession:     usecase.SwitchSession{Sessions: ss, Nodes: ps, IDs: ids, Clock: clk, Loc: time.Local},
		AddSession:        usecase.AddSession{Sessions: ss, Nodes: ps, IDs: ids, Clock: clk},
		EditSession:       usecase.EditSession{Sessions: ss, Nodes: ps, Tags: tags},
		DeleteSession:     usecase.DeleteSession{Sessions: ss, Tags: tags},
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
		ListArchived:      usecase.ListArchived{Docs: ds},
		GetDocument:       usecase.GetDocument{Docs: ds},
		SetPinned:         usecase.SetPinned{Docs: ds},
		SetContextMode:    usecase.SetContextMode{Docs: ds},
		ListActivity:      usecase.ListActivity{Activities: as},
		ListArtifacts:     usecase.ListArtifacts{Nodes: ps, Artifacts: arts},
		Stats: usecase.StatsComputer{
			Sessions: ss,
			Nodes:    ps, // REQUIRED for NodeStats subtree walk
			Settings: settings,
			Clock:    clk,
			Loc:      time.Local,
		},
		// L5 Task 5: the cockpit rail's context-instrument panel composes via
		// ExecuteForNode, which only needs Nodes+Docs+Tags (Resolve is unused
		// by the ID-based entry point).
		ComposeContext:     usecase.ComposeContext{Nodes: ps, Docs: ds, Tags: tags},
		ContextBudget:      12000,
		ReorderContextDocs: usecase.ReorderContextDocs{Docs: ds},
	}
	return &cockpitTestServer{srv: srv, ss: ss, ps: ps, bs: bs, ds: ds, tags: tags, as: as, arts: arts, ids: ids, clk: clk, codec: codec}
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
// 2-hour completed session on the child. It verifies the underlying data pipeline
// nodeCockpitData wires (subtree rollup + inherited rate) via the flat page's own
// surfaces (Task 7): the Enthält section's per-child subtree total (parent) and
// the rail's inherited-rate row + instr-band's own-node TodayHere line (child,
// whose 2h session happens to be "today" per the fake clock).
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
	// Clock is 12:00 on 2026-06-30, so these times are in the past AND "today". ✓
	day := time.Date(2026, 6, 30, 0, 0, 0, 0, time.Local)
	start := day.Add(8 * time.Hour)
	stop := day.Add(10 * time.Hour)
	c1ID := "c1"
	if _, err := (usecase.AddSession{Sessions: c.ss, Nodes: c.ps, IDs: c.ids, Clock: c.clk}).Execute(
		context.Background(), "u1", &c1ID, start, stop, nil, "",
	); err != nil {
		t.Fatalf("AddSession on child: %v", err)
	}

	// GET parent cockpit: rate resolves natively on p1 (rateLabel(9500,"EUR") = "95 €/h"),
	// and the Enthält section lists the child with its subtree total.
	rec := c.do(t, "GET", "/nodes/p1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /nodes/p1: status %d body=%.400s", rec.Code, rec.Body.String())
	}
	parentBody := rec.Body.String()
	if !strings.Contains(parentBody, "95 €/h") {
		t.Errorf("parent cockpit: missing rate label %q", "95 €/h")
	}
	if !strings.Contains(parentBody, "2:00 h") {
		t.Errorf("parent cockpit: Enthält section missing child subtree total %q\nbody snippet: %.800s", "2:00 h", parentBody)
	}

	// GET child cockpit: rate inherited from parent + own-node TodayHere.
	rec2 := c.do(t, "GET", "/nodes/c1", nil)
	if rec2.Code != http.StatusOK {
		t.Fatalf("GET /nodes/c1: status %d body=%.400s", rec2.Code, rec2.Body.String())
	}
	childBody := rec2.Body.String()

	// Child has no Rate of its own; ResolveRate walks ancestors and finds parent's rate.
	if !strings.Contains(childBody, "95 €/h") {
		t.Errorf("child cockpit: missing inherited rate label %q", "95 €/h")
	}
	// The parent (p1) is the chain's root here (no grandparent), so its name
	// surfaces both via the instr-band's chain stats segment and the rail's Kette block.
	if !strings.Contains(childBody, "ParentVorhaben") {
		t.Errorf("child cockpit: missing chain ancestor name %q", "ParentVorhaben")
	}
	// TodayHere (own-node, not subtree) sums the child's 2h session (it's "today").
	if !strings.Contains(childBody, "2:00 h") {
		t.Errorf("child cockpit: missing own-node TodayHere %q", "2:00 h")
	}
}

// TestCockpitRail_ContributorsFilledOnHeadPath pins the T5 fix carried into the
// flat rail (Task 7): the "Beiträger" row is filled in nodeCockpitData (the
// always-run path), so it survives the rail's OWN SSE reload (GET
// /nodes/{id}/rail) independent of the main content fragment.
func TestCockpitRail_ContributorsFilledOnHeadPath(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "eng", OwnerID: "u1", Name: "Engagement", Slug: "eng", Kind: domain.KindEngagement})
	engID := "eng"
	c.seedNode(t, domain.Node{ID: "repo", OwnerID: "u1", Name: "flow", Slug: "flow", Kind: domain.KindRepo, ParentID: &engID})

	// Seed a subtree activity entry authored by "claude-code" on the repo child.
	repoRef := "repo"
	c.as.items = []domain.ActivityEntry{
		{ID: "a1", OwnerID: "u1", ActorKind: "agent", ActorRef: "claude-code", Kind: "session.started", NodeRef: &repoRef, At: c.clk.Now()},
	}

	rec := c.do(t, "GET", "/nodes/eng/rail", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /nodes/eng/rail: status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "claude-code") {
		t.Errorf("rail /rail must show the subtree contributor 'claude-code' (Beiträger row): %.600s", rec.Body.String())
	}
}

// TestCockpitView_RollupAndIdentity verifies the flat page (Task 7): the three
// SSE fragment IDs, no tab-strip remnants, and the 404 for an unknown node.
func TestCockpitView_RollupAndIdentity(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo, Color: "cyan"})

	rec := c.do(t, "GET", "/nodes/n1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"flow", `id="cockpit-head"`, `id="cockpit-main"`, `id="cockpit-rail"`, `class="cock"`} {
		if !strings.Contains(body, want) {
			t.Errorf("cockpit missing %q", want)
		}
	}
	for _, gone := range []string{"pill-tabs", "pill-tab", "/tab/", "data-tabstrip"} {
		if strings.Contains(body, gone) {
			t.Errorf("flat cockpit must not contain tab remnant %q", gone)
		}
	}
	if rec2 := c.do(t, "GET", "/nodes/nope", nil); rec2.Code != http.StatusNotFound {
		t.Errorf("unknown id status=%d want 404", rec2.Code)
	}
}

// TestCockpitFragments_NoNestedContainerIDs is the flat-architecture successor
// to the old tab-strip's DOM-nesting-bug regression tests: each of the three
// fragment endpoints must render ONLY its own content, never re-emitting one
// of the OTHER two container ids — that would mean a fragment handler
// accidentally rendered the full page (or another fragment) instead of its
// own slice, causing duplicate ids and a broken SSE swap target.
func TestCockpitFragments_NoNestedContainerIDs(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})
	// Seed a binding + child structure so every section has real content to render.
	_, _ = (usecase.BindNode{Bindings: c.bs, Nodes: c.ps, IDs: c.ids, Clock: c.clk}).Execute(
		context.Background(), "u1", "n1", usecase.BindKey{Kind: domain.BindingRemote, RemoteSlug: "github.com/x/y"})

	cases := []struct {
		route  string
		others []string
	}{
		{"/nodes/n1/head", []string{`id="cockpit-main"`, `id="cockpit-rail"`}},
		{"/nodes/n1/main", []string{`id="cockpit-head"`, `id="cockpit-rail"`}},
		{"/nodes/n1/rail", []string{`id="cockpit-head"`, `id="cockpit-main"`}},
	}
	for _, c2 := range cases {
		rec := c.do(t, "GET", c2.route, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d", c2.route, rec.Code)
		}
		body := rec.Body.String()
		for _, other := range c2.others {
			if strings.Contains(body, other) {
				t.Errorf("%s: fragment must NOT contain %s (nesting bug): %.400s", c2.route, other, body)
			}
		}
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

// TestCockpitMain_FragmentReturnsOK verifies the flat /main fragment (Task 7):
// the content column renders (Enthält/Wissen/Buchungen/Puls sections, no tab
// strip), and an unknown node 404s.
func TestCockpitMain_FragmentReturnsOK(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	rec := c.do(t, "GET", "/nodes/n1/main", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("main fragment status %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "pill-tab") {
		t.Errorf("main fragment must not contain tab-strip markup: %.400s", rec.Body.String())
	}
	if rec2 := c.do(t, "GET", "/nodes/nope/main", nil); rec2.Code != http.StatusNotFound {
		t.Errorf("unknown node /main status=%d want 404", rec2.Code)
	}
}

// TestCockpitMain_ReloadURLTargetsSelf verifies the flat page's #cockpit-main
// container hx-gets its OWN fragment route (/main) — the successor to the old
// tab-strip's "panel SSE reload must target the outer container, not itself"
// nesting guard: in the flat architecture there IS only one outer container
// per fragment, so the invariant becomes "the container reloads its own
// route". Uses an Engagement (default Wissen scope "subtree", no query param)
// rather than a Repo, whose Wissen scope is always "self" and would legitimately
// carry ?scope=self even on the plain reload (cockpitMainReloadURL).
func TestCockpitMain_ReloadURLTargetsSelf(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "e1", OwnerID: "u1", Name: "Engagement", Kind: domain.KindEngagement})

	rec := c.do(t, "GET", "/nodes/e1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `hx-get="/nodes/e1/main"`) {
		t.Errorf("#cockpit-main must hx-get its own /main fragment: %.600s", body)
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
	archived := doc
	archived.ID = "d2"
	archived.Path = "archiv"
	archived.Title = "Alte Architektur"
	_, _ = c.ds.Create(context.Background(), archived)
	_ = c.ds.SetArchived(context.Background(), "u1", archived.ID, true)

	rec := c.do(t, "GET", "/nodes/n1/main", nil)
	body := rec.Body.String()
	if !strings.Contains(body, "Architektur") || !strings.Contains(body, "/wissen/neu?node=n1") {
		t.Errorf("wissen section missing doc / scoped-new link: %.300s", body)
	}
	for _, want := range []string{`href="/wissen?node=n1&amp;scope=self"`, "1 aktiv", "1 archiviert", "Verwalten"} {
		if !strings.Contains(body, want) {
			t.Errorf("wissen section missing manager summary %q: %.900s", want, body)
		}
	}
	if strings.Contains(body, "Alte Architektur") {
		t.Errorf("cockpit active preview must not render archived rows: %.900s", body)
	}
}

// TestCockpitWissen_EngagementDefaultShowsSubtreeDocs pins the §4 containment
// default: an Engagement's Wissen section shows its whole subtree's docs
// (here, a doc booked on a child Repo) with no ?scope query.
func TestCockpitWissen_EngagementDefaultShowsSubtreeDocs(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "eng", OwnerID: "u1", Name: "Engagement", Kind: domain.KindEngagement})
	engID := "eng"
	c.seedNode(t, domain.Node{ID: "repo", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo, ParentID: &engID})
	repoID := "repo"
	_, _ = c.ds.Create(context.Background(), domain.Document{
		ID: "d1", OwnerID: "u1", NodeID: &repoID, Type: domain.DocFree,
		Path: "doc-on-repo", Title: "doc-on-repo-title", Body: "# A",
		CreatedAt: c.clk.Now(), UpdatedAt: c.clk.Now(),
	})

	rec := c.do(t, "GET", "/nodes/eng/main", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "doc-on-repo-title") {
		t.Errorf("engagement wissen section must include subtree (child) docs: %.600s", body)
	}
	if !strings.Contains(body, `href="/wissen?node=eng&amp;scope=subtree"`) {
		t.Errorf("engagement wissen manager must preserve subtree scope: %.900s", body)
	}
	if !strings.Contains(body, "scope=self") || !strings.Contains(body, "scope=subtree") {
		t.Errorf("engagement wissen section missing the subtree/self scope toggle: %.600s", body)
	}
}

// TestCockpitWissen_ScopeSelfIsOwnOnly pins the ?scope=self toggle: it drops
// child-subtree docs and shows only the node's own.
func TestCockpitWissen_ScopeSelfIsOwnOnly(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "eng", OwnerID: "u1", Name: "Engagement", Kind: domain.KindEngagement})
	engID := "eng"
	c.seedNode(t, domain.Node{ID: "repo", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo, ParentID: &engID})
	repoID := "repo"
	_, _ = c.ds.Create(context.Background(), domain.Document{
		ID: "d1", OwnerID: "u1", NodeID: &repoID, Type: domain.DocFree,
		Path: "doc-on-repo", Title: "doc-on-repo-title", Body: "# A",
		CreatedAt: c.clk.Now(), UpdatedAt: c.clk.Now(),
	})

	rec := c.do(t, "GET", "/nodes/eng/main?scope=self", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "doc-on-repo-title") {
		t.Errorf("scope=self must NOT include child docs: %.600s", body)
	}
	if !strings.Contains(body, `href="/wissen?node=eng&amp;scope=self"`) {
		t.Errorf("scope=self manager link must preserve the explicit cockpit scope: %.900s", body)
	}
}

// TestCockpitWissen_RepoOwnOnlyNoToggle pins that a Repo cockpit's Wissen
// section stays own-only and never renders the subtree/self toggle.
func TestCockpitWissen_RepoOwnOnlyNoToggle(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "r1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	rec := c.do(t, "GET", "/nodes/r1/main", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "/nodes/r1/main?scope=self") || strings.Contains(body, "/nodes/r1/main?scope=subtree") {
		t.Errorf("repo wissen section must not render the subtree toggle: %.600s", body)
	}
}

// TestCockpitWissen_ForeignDocNotLeaked pins owner-scoping on the subtree
// rollup path: wissenTabDocs fetches ALL of the requesting owner's docs
// (ListDocuments.Execute(ctx, u.ID, nil, nil)) and filters in-memory by
// subtree node id — it must never surface another owner's document even if
// that document happens to carry a NodeID matching a node in the requester's
// own subtree.
func TestCockpitWissen_ForeignDocNotLeaked(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "eng", OwnerID: "u1", Name: "Engagement", Kind: domain.KindEngagement})
	engID := "eng"
	c.seedNode(t, domain.Node{ID: "repo", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo, ParentID: &engID})
	repoID := "repo"

	u2, _ := domain.NewUser("u2", "sub-2", "other", "o@x.de", "O")
	_, _ = c.srv.Users.UpsertBySub(context.Background(), u2)
	_, _ = c.ds.Create(context.Background(), domain.Document{
		ID: "d-foreign", OwnerID: "u2", NodeID: &repoID, Type: domain.DocFree,
		Path: "foreign-doc", Title: "foreign-doc-title", Body: "# A",
		CreatedAt: c.clk.Now(), UpdatedAt: c.clk.Now(),
	})

	rec := c.do(t, "GET", "/nodes/eng/main", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "foreign-doc-title") {
		t.Errorf("foreign owner's subtree doc leaked into wissen section: %.600s", rec.Body.String())
	}
}

// TestCockpitWissen_ScopeSelfSSEReloadPreservesScope pins that #cockpit-main's
// own SSE-driven live-reload URL (fired on sse:document.*, among others)
// carries ?scope=self when the user toggled to "Nur dieser Knoten" —
// otherwise a document event would silently revert the section to the
// subtree default on next reload (cockpitMainReloadURL, Task 7).
func TestCockpitWissen_ScopeSelfSSEReloadPreservesScope(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "eng", OwnerID: "u1", Name: "Engagement", Kind: domain.KindEngagement})

	rec := c.do(t, "GET", "/nodes/eng?scope=self", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `hx-get="/nodes/eng/main?scope=self"`) {
		t.Errorf("#cockpit-main SSE-reload must carry scope=self: %.800s", body)
	}
}

// TestCockpitWissen_ScopeSubtreeSSEReloadOmitsScope pins the counterpart: the
// default subtree scope must NOT gain a stray ?scope= param on #cockpit-main's
// SSE live-reload URL.
func TestCockpitWissen_ScopeSubtreeSSEReloadOmitsScope(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "eng", OwnerID: "u1", Name: "Engagement", Kind: domain.KindEngagement})

	rec := c.do(t, "GET", "/nodes/eng", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// The #cockpit-main container's own reload attribute must be the plain
	// URL — the Wissen section's "Nur dieser Knoten" scope-toggle LINK also
	// legitimately carries a "?scope=self" hx-get (that's the toggle itself,
	// not a leak), so the check targets the container's attribute specifically.
	if !strings.Contains(body, `id="cockpit-main" class="min-w-0" hx-get="/nodes/eng/main" `) {
		t.Errorf("#cockpit-main container's own SSE-reload (subtree) must be the plain /main URL: %.800s", body)
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

// TestCockpitEnthaelt_ListsChildren verifies the flat page's Enthält section
// (Task 7: replaces the old Struktur tab) lists direct children + the
// scoped add-child link.
func TestCockpitEnthaelt_ListsChildren(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "p1", OwnerID: "u1", Name: "Plattform", Kind: domain.KindVorhaben})
	pp := "p1"
	c.seedNode(t, domain.Node{ID: "c1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo, ParentID: &pp})

	rec := c.do(t, "GET", "/nodes/p1/main", nil)
	body := rec.Body.String()
	if !strings.Contains(body, "flow") {
		t.Errorf("Enthält section missing child: %.300s", body)
	}
	if !strings.Contains(body, "/nodes/new?parent=p1") {
		t.Errorf("Enthält section missing add-child link")
	}
}

// TestNodeEdit_ShowsMoveForm verifies the flat design's Move-on-Edit-page
// wiring (Task 7 Step 5): the edit page for a node with a valid alternate
// parent renders the Move form (action .../move) with that target as an
// option — the Struktur tab's old cockpitMoveForm moved here.
func TestNodeEdit_ShowsMoveForm(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "p1", OwnerID: "u1", Name: "Plattform", Slug: "plattform", Kind: domain.KindVorhaben})
	c.seedNode(t, domain.Node{ID: "e2", OwnerID: "u1", Name: "Acme", Slug: "acme", Kind: domain.KindEngagement})
	pp := "p1"
	c.seedNode(t, domain.Node{ID: "c1", OwnerID: "u1", Name: "flow", Slug: "flow", Kind: domain.KindRepo, ParentID: &pp})

	rec := c.do(t, "GET", "/nodes/c1/edit", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("edit page status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `action="/nodes/c1/move"`) {
		t.Errorf("edit page missing Move form action /nodes/c1/move: %.800s", body)
	}
	if !strings.Contains(body, "Acme") {
		t.Errorf("edit page Move form missing valid target 'Acme': %.800s", body)
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
	// Bind/unbind mutations target #cockpit-rail (Task 7 — the Bindings block
	// lives on the rail, not the content column).
	if !strings.Contains(rec.Body.String(), `hx-target="#cockpit-rail"`) {
		t.Errorf("bindings panel forms must target #cockpit-rail: %.600s", rec.Body.String())
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

// TestCockpitContext_PanelShowsMeterAndCurateLink pins the L5 rail panel's
// wiring end-to-end: a repo node with a memory doc composes via
// ExecuteForNode (nodeCockpitData) into d.Context, and the rendered page
// shows the meter + Kuratieren link pointing at THIS node.
func TestCockpitContext_PanelShowsMeterAndCurateLink(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})
	nodeID := "n1"
	_, err := c.ds.Create(context.Background(), domain.Document{
		ID: "mem-1", OwnerID: "u1", NodeID: &nodeID, Type: domain.DocMemory,
		Path: "mem-1", Title: "Tailwind v4 gotchas", Body: "some memory body",
		CreatedAt: c.clk.Now(), UpdatedAt: c.clk.Now(),
	})
	if err != nil {
		t.Fatalf("seed memory doc: %v", err)
	}

	rec := c.do(t, "GET", "/nodes/n1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"Kontext für Agenten", `class="meter`, "Kuratieren", "/kontext/n1", "1 Docs"} {
		if !strings.Contains(body, want) {
			t.Errorf("cockpit page misses context panel %q: %.800s", want, body)
		}
	}
}

// TestCockpitContext_OwnerScopeNoForeignDocsInMeter is the L5 owner-scope
// negative test (Global Constraints §"Jede neue Datenfläche bekommt einen
// Owner-Scope-Negativtest"): a foreign owner's memory doc — even one that
// happens to carry u1's own node id — must never inflate u1's Included
// counter (ExecuteForNode's ListForContext call is owner-scoped).
func TestCockpitContext_OwnerScopeNoForeignDocsInMeter(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})
	nodeID := "n1"

	u2, _ := domain.NewUser("u2", "sub-2", "other", "o@x.de", "O")
	_, _ = c.srv.Users.UpsertBySub(context.Background(), u2)
	_, err := c.ds.Create(context.Background(), domain.Document{
		ID: "mem-foreign", OwnerID: "u2", NodeID: &nodeID, Type: domain.DocMemory,
		Path: "mem-foreign", Title: "foreign memory", Body: "should not leak",
		CreatedAt: c.clk.Now(), UpdatedAt: c.clk.Now(),
	})
	if err != nil {
		t.Fatalf("seed foreign doc: %v", err)
	}

	rec := c.do(t, "GET", "/nodes/n1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "foreign memory") {
		t.Errorf("foreign owner's memory doc leaked into cockpit page: %.800s", body)
	}
	if !strings.Contains(body, "0 Docs") {
		t.Errorf("context panel must count 0 included docs (foreign doc excluded): %.800s", body)
	}
}

// TestCockpitContext_NoDocsRendersWithoutCrash covers the empty state: a
// node with no context docs still renders the full cockpit page (meter at
// 0%, no pins), guarded degrade-to-no-panic per nodeCockpitData's contract.
func TestCockpitContext_NoDocsRendersWithoutCrash(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	rec := c.do(t, "GET", "/nodes/n1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Kontext für Agenten") {
		t.Errorf("context panel must still render (guarded, not absent) when compose succeeds with zero docs: %.800s", body)
	}
	if !strings.Contains(body, "0 Docs") {
		t.Errorf("no-docs cockpit must show 0 Docs, got: %.800s", body)
	}
}

// TestCockpitArtifacts_FreeCardShowsFreiOrigin is the free-artifacts Task 3
// mandatory wiring test (E4): a free (node-less) artifact appears in the
// node cockpit's gallery as inherited, read-only, with FromNode == the
// "cockpit.artifacts.free" i18n label ("Frei") — nodeCockpitData must set
// names[""] to that label BEFORE calling BuildArtifactCards, or the free
// card's origin marker renders blank instead of "geerbt von Frei". A node's
// own artifact (same node, not inherited) must NOT show an origin marker at
// all, proving the cockpit still distinguishes "own" from "free" correctly
// (E4: both appear, but only the free one is marked as inherited-from-Frei).
func TestCockpitArtifacts_FreeCardShowsFreiOrigin(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})
	ctx := context.Background()
	if err := c.arts.Put(ctx, domain.Artifact{
		OwnerID: "u1", NodeID: "n1", Slug: "own-logo", Name: "own-logo.png", Mime: "image/png",
	}); err != nil {
		t.Fatalf("seed node artifact: %v", err)
	}
	if err := c.arts.Put(ctx, domain.Artifact{
		OwnerID: "u1", NodeID: "", Slug: "brand", Name: "brand.png", Mime: "image/png",
	}); err != nil {
		t.Fatalf("seed free artifact: %v", err)
	}

	rec := c.do(t, "GET", "/nodes/n1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "brand.png") {
		t.Fatalf("free artifact card missing from cockpit gallery: %.1200s", body)
	}
	if !strings.Contains(body, "geerbt von Frei") {
		t.Fatalf("free artifact card must show the 'Frei' origin label (geerbt von Frei), got: %.1200s", body)
	}
	if !strings.Contains(body, "own-logo.png") {
		t.Fatalf("own node artifact card missing: %.1200s", body)
	}
	// own-logo is on n1 itself (not inherited) — only the free card's origin
	// marker ("geerbt von Frei") may appear, exactly once.
	if got := strings.Count(body, "geerbt von"); got != 1 {
		t.Fatalf("want exactly 1 origin marker (the free card only), got %d in: %.1200s", got, body)
	}
}
