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

// seedBookableNode creates an owner "u1" repo node with a unique slug and
// returns its ID — a minimal bookable node for MRU/timer tests.
func (w *worktimeTestServer) seedBookableNode(t *testing.T, slug string) string {
	t.Helper()
	n, err := w.ps.Create(context.Background(), domain.Node{
		ID: "n-" + slug, OwnerID: "u1", Name: "github.com/x/" + slug, Slug: slug, Kind: domain.KindRepo,
	})
	if err != nil {
		t.Fatalf("seedBookableNode: %v", err)
	}
	return n.ID
}

// seedBookableNodeWithLogo is seedBookableNode plus a LogoRef, for the
// NodeAvatar logo-vs-initials assertions.
func (w *worktimeTestServer) seedBookableNodeWithLogo(t *testing.T, slug, logoRef string) string {
	t.Helper()
	n, err := w.ps.Create(context.Background(), domain.Node{
		ID: "n-" + slug, OwnerID: "u1", Name: "github.com/x/" + slug, Slug: slug, Kind: domain.KindRepo, LogoRef: logoRef,
	})
	if err != nil {
		t.Fatalf("seedBookableNodeWithLogo: %v", err)
	}
	return n.ID
}

// startSession starts a running (unstopped) WorkSession directly against the
// fake store — nodeID nil seeds an unbound running timer.
func (w *worktimeTestServer) startSession(t *testing.T, ownerID string, nodeID *string) {
	t.Helper()
	ws, err := domain.NewWorkSession("s-running", ownerID, nodeID, w.clk.T.Add(-41*time.Minute))
	if err != nil {
		t.Fatalf("startSession: %v", err)
	}
	if _, err := w.ss.Create(context.Background(), ws); err != nil {
		t.Fatalf("startSession: create: %v", err)
	}
}

// seedStoppedSession seeds a completed WorkSession on nodeID spanning
// [start,stop).
func (w *worktimeTestServer) seedStoppedSession(t *testing.T, ownerID, nodeID string, start, stop time.Time) {
	t.Helper()
	ws, err := domain.NewWorkSession("s-"+nodeID+"-"+start.Format(time.RFC3339), ownerID, &nodeID, start)
	if err != nil {
		t.Fatalf("seedStoppedSession: %v", err)
	}
	ws.Stop = &stop
	if _, err := w.ss.Create(context.Background(), ws); err != nil {
		t.Fatalf("seedStoppedSession: create: %v", err)
	}
}

// TestHomeHome_RendersLanding verifies that GET / returns 200 and renders the
// L4 Schreibtisch landing (pagehead + section eyebrows), and that an
// unauthenticated request redirects to login.
func TestHomeHome_RendersLanding(t *testing.T) {
	clk := testutil.FakeClock{T: time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x.de", "Martin")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour)

	bus2 := sse.NewBus()
	srv := &httpserver.Server{
		Users:   users,
		Session: codec,
		Bus:     bus2,
		Emitter: sse.NewEmitter(bus2, &fakeActivityStore{}, ids, clk),
		Clock:   clk,
		Ensure:  usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
	}

	// Unauthenticated → redirect to login (webAuth gate).
	noRedir := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	res, err := noRedir.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("unauth GET / = %d, want 302", res.StatusCode)
	}

	// Authenticated → 200 with the Schreibtisch pagehead + section eyebrows.
	cookieVal, _ := codec.Issue("u1")
	req, _ := http.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET / status=%d body=%.500s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		"Schreibtisch", // home.desk (DE) — the L4 pagehead h1, not the Kristall "Home"
		"Jetzt",        // home.now eyebrow
		"Weiterarbeiten",
		"Zuletzt im Wissen",
		"Puls",
		"/static/app.css",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("GET / missing %q", want)
		}
	}
	// The Kristall dashboard chrome (saldo tile grid) must be gone.
	if strings.Contains(body, "sm:grid-cols-3") {
		t.Errorf("GET / must not render the retired saldo-tile grid: %.500s", body)
	}
}

// TestHomeFragment_IdleShowsNoTimerRowWithoutStartCTA verifies the Jetzt
// row's idle state (Spec §10 Offene Entsch. #1/#2): no running session →
// "Kein Timer läuft", today's total, and crucially NO /ui/timer/start form
// (no third Start-CTA on the Schreibtisch) and no Stop button.
func TestHomeFragment_IdleShowsNoTimerRowWithoutStartCTA(t *testing.T) {
	srv := newWorktimeTestServer(t)
	body := getBody(t, srv, "u1", "/")

	if !strings.Contains(body, "Kein Timer läuft") {
		t.Errorf("idle Jetzt row missing 'Kein Timer läuft': %.1500s", body)
	}
	if strings.Contains(body, "/ui/timer/start") {
		t.Errorf("Schreibtisch must not render a Start-CTA (Spec §10): %.1500s", body)
	}
	if strings.Contains(body, "/ui/timer/stop") {
		t.Errorf("idle Jetzt row must not render a Stop button: %.1500s", body)
	}
}

// TestHomeFragment_RunningBoundShowsPanelrowAndStop verifies the Jetzt
// row's running+bound state: the live clock (data-timer/data-base), the
// bound node's link, and a Stop button posting to the ONE /ui/timer/stop
// with hx-swap="none" (never swapping #timer-pill — Spec §10 Timer-Vorgabe).
func TestHomeFragment_RunningBoundShowsPanelrowAndStop(t *testing.T) {
	srv := newWorktimeTestServer(t)
	nodeID := srv.seedBookableNode(t, "repo-a")
	srv.startSession(t, "u1", &nodeID)

	body := getBody(t, srv, "u1", "/")

	if !strings.Contains(body, `data-timer`) || !strings.Contains(body, `data-timer-fmt="clock"`) {
		t.Errorf("running Jetzt row must carry the live-tick attributes: %.2000s", body)
	}
	if !strings.Contains(body, `href="/nodes/`+nodeID+`"`) {
		t.Errorf("running Jetzt row must link the bound node: %.2000s", body)
	}
	if !strings.Contains(body, `hx-post="/ui/timer/stop"`) || !strings.Contains(body, `hx-swap="none"`) {
		t.Errorf("bound running timer must offer a Stop posting to /ui/timer/stop with hx-swap=none: %.2000s", body)
	}
	if strings.Contains(body, `hx-target="#timer-pill"`) {
		t.Errorf("Jetzt Stop must never target #timer-pill (would swap Sheet markup into the Pill mount): %.2000s", body)
	}
}

// TestHomeFragment_RunningUnboundHidesStop verifies that an unbound running
// session (no node chosen yet) shows no Stop button on the Jetzt row — Spec
// §10: handleTimerStop errors timer.needNode without a node for an unbound
// session, so Stop for that case lives only on the Topbar-Pill's sheet.
func TestHomeFragment_RunningUnboundHidesStop(t *testing.T) {
	srv := newWorktimeTestServer(t)
	srv.startSession(t, "u1", nil)

	body := getBody(t, srv, "u1", "/")
	if strings.Contains(body, `hx-post="/ui/timer/stop"`) {
		t.Errorf("unbound running timer must not offer a Stop on the Schreibtisch: %.2000s", body)
	}
}

// TestHomeFragment_RunningBoundWithLogoUsesNodeAvatar verifies the Jetzt
// row's avatar is the shared components.NodeAvatar (L4 Task 2, Offene
// Entsch. #12): a bound running session on a node with a LogoRef renders
// its uploaded logo, not the initials tile.
func TestHomeFragment_RunningBoundWithLogoUsesNodeAvatar(t *testing.T) {
	srv := newWorktimeTestServer(t)
	nodeID := srv.seedBookableNodeWithLogo(t, "repo-logo", "logohash1")
	srv.startSession(t, "u1", &nodeID)

	body := getBody(t, srv, "u1", "/")
	if !strings.Contains(body, `/nodes/`+nodeID+`/logo?v=logohash1`) {
		t.Errorf("Jetzt row must render the bound node's logo via NodeAvatar: %.2000s", body)
	}
}

// TestHomeFragment_ContinueShowsMRUNodesNewestFirst verifies "Weiterarbeiten"
// lists distinct bookable nodes ordered by most-recent session.
func TestHomeFragment_ContinueShowsMRUNodesNewestFirst(t *testing.T) {
	srv := newWorktimeTestServer(t)
	older := srv.seedBookableNode(t, "repo-older")
	newer := srv.seedBookableNode(t, "repo-newer")
	srv.seedStoppedSession(t, "u1", older, srv.clk.T.Add(-3*time.Hour), srv.clk.T.Add(-2*time.Hour))
	srv.seedStoppedSession(t, "u1", newer, srv.clk.T.Add(-1*time.Hour), srv.clk.T.Add(-30*time.Minute))

	body := getBody(t, srv, "u1", "/")
	newerIdx := strings.Index(body, `href="/nodes/`+newer+`"`)
	olderIdx := strings.Index(body, `href="/nodes/`+older+`"`)
	if newerIdx < 0 || olderIdx < 0 {
		t.Fatalf("both MRU nodes must render; newer=%d older=%d body=%.2000s", newerIdx, olderIdx, body)
	}
	if olderIdx < newerIdx {
		t.Errorf("newer session's node must render before the older one")
	}
}

// TestHomeFragment_ContinueNodeAvatarLogoOrInitials verifies "Weiterarbeiten"
// rows use the shared components.NodeAvatar: a node with a LogoRef shows its
// logo, a node without one falls back to initials (Mockup Z.382 shows a
// logo-avatar in this exact section).
func TestHomeFragment_ContinueNodeAvatarLogoOrInitials(t *testing.T) {
	srv := newWorktimeTestServer(t)
	withLogo := srv.seedBookableNodeWithLogo(t, "repo-logo", "logohash2")
	plain := srv.seedBookableNode(t, "repo-plain")
	srv.seedStoppedSession(t, "u1", withLogo, srv.clk.T.Add(-2*time.Hour), srv.clk.T.Add(-1*time.Hour))
	srv.seedStoppedSession(t, "u1", plain, srv.clk.T.Add(-4*time.Hour), srv.clk.T.Add(-3*time.Hour))

	body := getBody(t, srv, "u1", "/")
	if !strings.Contains(body, `/nodes/`+withLogo+`/logo?v=logohash2`) {
		t.Errorf("Weiterarbeiten row with a LogoRef must render its logo: %.2000s", body)
	}
	if strings.Contains(body, `/nodes/`+plain+`/logo?v=`) {
		t.Errorf("Weiterarbeiten row without a LogoRef must not render a logo <img>: %.2000s", body)
	}
}

// TestHomeFragment_ContinueEmptyState verifies the quiet empty-state row
// (no empty cards) when the owner has no session history yet.
func TestHomeFragment_ContinueEmptyState(t *testing.T) {
	srv := newWorktimeTestServer(t)
	body := getBody(t, srv, "u1", "/")
	if !strings.Contains(body, "Noch keine Aktivität") {
		t.Errorf("empty Weiterarbeiten must show the quiet empty state: %.1500s", body)
	}
}

// newHomeKnowledgeServer wires ListDocuments + ListActivity + NodeAncestors
// on top of the worktime plumbing — TestHomeHome_ShowsNewestDocs's
// precedent, extended for Puls + owner-scope negative tests.
type homeKnowledgeServer struct {
	srv   *httpserver.Server
	docs  *testutil.FakeDocumentStore
	nodes *testutil.FakeNodeStore
	acts  *fakeActivityStore
	codec *websession.Codec
	clk   testutil.FakeClock
}

func newHomeKnowledgeServer(t *testing.T) *homeKnowledgeServer {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	docs := testutil.NewFakeDocumentStore()
	nodes := testutil.NewFakeNodeStore()
	users := testutil.NewFakeUserStore()
	for _, uid := range []string{"u1", "u2"} {
		u, _ := domain.NewUser(uid, "sub-"+uid, uid, uid+"@x", uid)
		_, _ = users.UpsertBySub(context.Background(), u)
	}
	codec := websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour)
	acts := &fakeActivityStore{}
	bus := sse.NewBus()
	srv := &httpserver.Server{
		Ensure:        usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:           bus,
		Emitter:       sse.NewEmitter(bus, acts, ids, clk),
		Clock:         clk,
		Users:         users,
		Session:       codec,
		ListDocuments: usecase.ListDocuments{Docs: docs},
		ListNodes:     usecase.ListNodes{Nodes: nodes},
		ListActivity:  usecase.ListActivity{Activities: acts},
	}
	return &homeKnowledgeServer{srv: srv, docs: docs, nodes: nodes, acts: acts, codec: codec, clk: clk}
}

func (h *homeKnowledgeServer) body(t *testing.T, ownerID string) string {
	t.Helper()
	cookieVal, _ := h.codec.Issue(ownerID)
	req, _ := http.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	h.srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET / status=%d body=%.500s", rr.Code, rr.Body.String())
	}
	return rr.Body.String()
}

// TestHomeHome_ShowsNewestDocs verifies that GET / renders "Zuletzt im
// Wissen" with seeded documents newest-first via the shared wissenRow (only
// .right .v, no .k — the WissenRowVM contract, not the Mockup's illustrative
// two-line .right).
func TestHomeHome_ShowsNewestDocs(t *testing.T) {
	h := newHomeKnowledgeServer(t)
	_, _ = h.docs.Create(context.Background(), domain.Document{
		ID: "doc-alpha", OwnerID: "u1", Type: domain.DocFree, Path: "alpha",
		Title: "Alpha Article", UpdatedAt: h.clk.T,
	})
	_, _ = h.docs.Create(context.Background(), domain.Document{
		ID: "doc-beta", OwnerID: "u1", Type: domain.DocFree, Path: "beta",
		Title: "Beta Article", UpdatedAt: h.clk.T.Add(-time.Hour),
	})

	body := h.body(t, "u1")
	for _, want := range []string{
		"Zuletzt im Wissen",
		"Alpha Article",
		`href="/wissen/doc-alpha"`,
		`href="/wissen/doc-beta"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("GET / newest-docs: missing %q", want)
		}
	}
	alphaIdx := strings.Index(body, "Alpha Article")
	betaIdx := strings.Index(body, "Beta Article")
	if alphaIdx < 0 || betaIdx < 0 || betaIdx < alphaIdx {
		t.Errorf("want newest-first order (Alpha before Beta); alpha=%d beta=%d", alphaIdx, betaIdx)
	}
}

// TestHomeFragment_WissenOwnerScoped is the owner-scope negative test for
// "Zuletzt im Wissen": user B's document must never appear on user A's
// Schreibtisch (AGENTS.md §Grundsätze — flow is multi-tenant).
func TestHomeFragment_WissenOwnerScoped(t *testing.T) {
	h := newHomeKnowledgeServer(t)
	_, _ = h.docs.Create(context.Background(), domain.Document{
		ID: "doc-b", OwnerID: "u2", Type: domain.DocFree, Path: "secret",
		Title: "User B Secret Doc", UpdatedAt: h.clk.T,
	})

	body := h.body(t, "u1")
	if strings.Contains(body, "User B Secret Doc") {
		t.Errorf("owner-scope leak: u1's Schreibtisch rendered u2's document: %.2000s", body)
	}
}

// TestHomeFragment_PulsShowsActivity verifies the Puls section renders the
// shared pulseRow (livechip + who/verb) for seeded activity entries.
func TestHomeFragment_PulsShowsActivity(t *testing.T) {
	h := newHomeKnowledgeServer(t)
	ref := "flow"
	h.acts.items = []domain.ActivityEntry{
		{OwnerID: "u1", ActorKind: "human", ActorRef: "msoent", Kind: "session.started", Label: &ref, At: h.clk.T.Add(-5 * time.Minute)},
	}

	body := h.body(t, "u1")
	if !strings.Contains(body, "LIVE") {
		t.Errorf("Puls section must show the LIVE chip: %.2000s", body)
	}
	if !strings.Contains(body, "msoent") {
		t.Errorf("Puls row must show the actor: %.2000s", body)
	}
}

// TestHomeFragment_PulsOwnerScoped is the owner-scope negative test for
// Puls: a fakeActivityStore that (incorrectly) ignored ownerID would leak —
// this pins that ListActivity.Execute is always called with u.ID and that a
// same-store cross-owner entry is filtered by the store, not by Home.
func TestHomeFragment_PulsOwnerScoped(t *testing.T) {
	h := newHomeKnowledgeServer(t)
	h.acts.items = nil // ListPage is owner-blind in this fake; assert the call carries u1
	_ = h.body(t, "u1")
	if h.acts.lastOwner != "u1" {
		t.Errorf("ListActivity must be called with the requesting owner, got %q", h.acts.lastOwner)
	}
}

// TestHomeFragment_PulsEmptyState verifies the quiet empty-state row when
// there is no activity yet.
func TestHomeFragment_PulsEmptyState(t *testing.T) {
	h := newHomeKnowledgeServer(t)
	body := h.body(t, "u1")
	if !strings.Contains(body, "Noch keine Aktivität") {
		t.Errorf("empty Puls must show the quiet empty state: %.1500s", body)
	}
}

// TestHomeLogstreamRoute_Retired verifies the L2/K5-era logstream fragment
// route is gone (L4 Task 2 retirement) — a stray caller (bookmark, cached
// htmx fragment) now 404s instead of silently reviving Kristall chrome.
func TestHomeLogstreamRoute_Retired(t *testing.T) {
	srv := newWorktimeTestServer(t)
	cookieVal, _ := srv.codec.Issue("u1")
	req, _ := http.NewRequest("GET", "/ui/home/logstream", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("GET /ui/home/logstream = %d, want 404 (route retired)", rr.Code)
	}
}
