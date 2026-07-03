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
	as    *fakeActivityStore
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
	as := &fakeActivityStore{}
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
		ListActivity:      usecase.ListActivity{Activities: as},
		Stats: usecase.StatsComputer{
			Sessions: ss,
			Nodes:    ps, // REQUIRED for NodeStats subtree walk
			Settings: settings,
			Clock:    clk,
			Loc:      time.Local,
		},
	}
	return &cockpitTestServer{srv: srv, ss: ss, ps: ps, bs: bs, ds: ds, as: as, ids: ids, clk: clk, codec: codec}
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
// nodeCockpitData wires (subtree rollup + inherited rate) — the core deliverable
// of the original Task 2. Task 4 (Kristall K2) moved the Σ/earnings rollup tiles
// out of the always-visible rail and into the Übersicht tab's content, which is
// a later task's placeholder — so this test was consciously migrated to check
// the SAME underlying computations via surfaces Task 4 still renders: the
// rail's inherited-rate row (both nodes) and the Struktur tab's per-child
// subtree total (parent), plus the rail's own-node TodayHere line (child,
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

	// GET parent cockpit: rate resolves natively on p1 (rateLabel(9500,"EUR") = "95 €/h").
	rec := c.do(t, "GET", "/nodes/p1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /nodes/p1: status %d body=%.400s", rec.Code, rec.Body.String())
	}
	parentBody := rec.Body.String()
	if !strings.Contains(parentBody, "95 €/h") {
		t.Errorf("parent cockpit: missing rate label %q", "95 €/h")
	}

	// The Struktur tab still lists direct children with their subtree total —
	// the same NodeStats computation the old Σ-Gesamt tile used to show.
	recStruktur := c.do(t, "GET", "/nodes/p1/tab/struktur", nil)
	if recStruktur.Code != http.StatusOK {
		t.Fatalf("GET /nodes/p1/tab/struktur: status %d", recStruktur.Code)
	}
	if !strings.Contains(recStruktur.Body.String(), "2:00 h") {
		t.Errorf("parent struktur tab: missing child subtree total %q\nbody snippet: %.800s", "2:00 h", recStruktur.Body.String())
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
	// "geerbt von ParentVorhaben" — the rail shows the inheritance source.
	if !strings.Contains(childBody, "ParentVorhaben") {
		t.Errorf("child cockpit: missing inherited-from ancestor name %q", "ParentVorhaben")
	}
	// TodayHere (own-node, not subtree) sums the child's 2h session (it's "today").
	if !strings.Contains(childBody, "2:00 h") {
		t.Errorf("child cockpit: missing own-node TodayHere %q", "2:00 h")
	}
}

// TestCockpitRail_ContributorsFilledOnHeadPath pins the T5 fix: the rail's
// "Beiträger" row is filled in nodeCockpitData (the always-run path), so it
// survives the rail's OWN SSE reload (GET /nodes/{id}/head) which never calls
// fillPanelData/uebersichtData. A regression that moved the fill back into the
// panel builder would show an empty rail here.
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

	// The rail fragment path — /head — renders CockpitRail WITHOUT fillPanelData.
	rec := c.do(t, "GET", "/nodes/eng/head", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /nodes/eng/head: status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "claude-code") {
		t.Errorf("rail /head must show the subtree contributor 'claude-code' (Beiträger row): %.600s", rec.Body.String())
	}
}

// TestCockpitUebersicht_ArchivedEngagementExcludedFromOwnerTotal pins that an
// archived root engagement's time does NOT inflate the Chain card's owner
// total (the 100% denominator), matching the nav tree's active+paused
// visibility. With the archived engagement counted the repo's This-row would
// be a smaller %; excluding it, the repo (2h of a 2h active-only total) is 100%.
func TestCockpitUebersicht_ArchivedEngagementExcludedFromOwnerTotal(t *testing.T) {
	c := newCockpitTestServer(t)
	// Active engagement → repo (the cockpit node), 2h of work on the repo.
	c.seedNode(t, domain.Node{ID: "engA", OwnerID: "u1", Name: "Active", Slug: "active", Kind: domain.KindEngagement})
	engA := "engA"
	c.seedNode(t, domain.Node{ID: "repo", OwnerID: "u1", Name: "flow", Slug: "flow", Kind: domain.KindRepo, ParentID: &engA})
	// Archived engagement with a large amount of logged time.
	c.seedNode(t, domain.Node{ID: "engArch", OwnerID: "u1", Name: "Archived", Slug: "archived", Kind: domain.KindEngagement, Status: domain.NodeArchived})

	day := time.Date(2026, 6, 30, 0, 0, 0, 0, time.Local)
	repoID := "repo"
	archID := "engArch"
	if _, err := (usecase.AddSession{Sessions: c.ss, Nodes: c.ps, IDs: c.ids, Clock: c.clk}).Execute(
		context.Background(), "u1", &repoID, day.Add(8*time.Hour), day.Add(10*time.Hour), nil, ""); err != nil {
		t.Fatalf("AddSession repo: %v", err)
	}
	if _, err := (usecase.AddSession{Sessions: c.ss, Nodes: c.ps, IDs: c.ids, Clock: c.clk}).Execute(
		context.Background(), "u1", &archID, day.Add(1*time.Hour), day.Add(7*time.Hour), nil, ""); err != nil {
		t.Fatalf("AddSession archived: %v", err)
	}

	rec := c.do(t, "GET", "/nodes/repo/tab/uebersicht", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /nodes/repo/tab/uebersicht: status %d", rec.Code)
	}
	body := rec.Body.String()
	// Chain card present (repo → chain, not composition).
	if !strings.Contains(body, "Fließt nach oben") {
		t.Fatalf("repo cockpit missing chain card: %.600s", body)
	}
	// Repo This-row = 2h; owner total excludes the archived 6h, so total = 2h
	// and the This-row bar is at 100% (width:100%). If archived leaked in,
	// total would be 8h and the This-row would be 25%.
	if strings.Contains(body, "width:25%") {
		t.Errorf("archived engagement leaked into owner total (This-row at 25%%, expected 100%%): %.800s", body)
	}
}

// TestCockpitUebersicht_SelfArchivedAncestorCountedInOwnerTotal pins the
// EXCEPTION to the archived-exclusion rule: when the VIEWED repo's own root
// engagement is archived (reachable by direct URL), that root's subtree MUST
// still count in the owner total — otherwise the chain shows the repo's hours
// in its rows while excluding them from the denominator, making every Pct
// incoherent.
func TestCockpitUebersicht_SelfArchivedAncestorCountedInOwnerTotal(t *testing.T) {
	c := newCockpitTestServer(t)
	// Viewed repo sits under an ARCHIVED engagement; a separate ACTIVE
	// engagement carries other hours (the legitimate rest of the denominator).
	c.seedNode(t, domain.Node{ID: "engArch", OwnerID: "u1", Name: "Archived", Slug: "archived", Kind: domain.KindEngagement, Status: domain.NodeArchived})
	engArch := "engArch"
	c.seedNode(t, domain.Node{ID: "repo", OwnerID: "u1", Name: "flow", Slug: "flow", Kind: domain.KindRepo, ParentID: &engArch})
	c.seedNode(t, domain.Node{ID: "engActive", OwnerID: "u1", Name: "Active", Slug: "active", Kind: domain.KindEngagement})

	day := time.Date(2026, 6, 30, 0, 0, 0, 0, time.Local)
	repoID := "repo"
	activeID := "engActive"
	if _, err := (usecase.AddSession{Sessions: c.ss, Nodes: c.ps, IDs: c.ids, Clock: c.clk}).Execute(
		context.Background(), "u1", &repoID, day.Add(8*time.Hour), day.Add(10*time.Hour), nil, ""); err != nil { // 2h
		t.Fatalf("AddSession repo: %v", err)
	}
	if _, err := (usecase.AddSession{Sessions: c.ss, Nodes: c.ps, IDs: c.ids, Clock: c.clk}).Execute(
		context.Background(), "u1", &activeID, day.Add(1*time.Hour), day.Add(7*time.Hour), nil, ""); err != nil { // 6h
		t.Fatalf("AddSession active: %v", err)
	}

	body := c.do(t, "GET", "/nodes/repo/tab/uebersicht", nil).Body.String()
	// Owner total = repo's own archived-root subtree (2h) + active sibling (6h) =
	// 8h, so the This-row is 2/8 = 25%. WITHOUT the exception the archived root
	// would be excluded, ownerTotal = 6h, and the This-row would be 2/6 ≈ 33%.
	if !strings.Contains(body, "width:25%") {
		t.Errorf("viewed archived-root subtree must count in owner total (This-row 25%%): %.800s", body)
	}
	if strings.Contains(body, "width:33%") {
		t.Errorf("owner total wrongly excluded the viewed archived root (This-row at 33%%): %.800s", body)
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
	// "Σ Gesamt" (the old NodeHead rollup tile) moved to the Übersicht tab's
	// content (a later task); the default landing now shows the Übersicht
	// pill-tab active instead — asserted here in its place.
	for _, want := range []string{"flow", `id="cockpit-rail"`, `id="cockpit-main"`, "Übersicht", `aria-current="page"`} {
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
