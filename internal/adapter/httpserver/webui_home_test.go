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

// TestHomeHome_ShowsNewestDocs verifies that GET / renders the "Zuletzt im Wissen"
// section with seeded documents newest-first, each linking to /wissen/{id}.
func TestHomeHome_ShowsNewestDocs(t *testing.T) {
	clk := testutil.FakeClock{T: time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	docs := testutil.NewFakeDocumentStore()
	projects := testutil.NewFakeNodeStore()
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x", "Martin")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour)

	srv := &httpserver.Server{
		Ensure:        usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:           sse.NewBus(),
		Clock:         clk,
		Users:         users,
		Session:       codec,
		ListDocuments: usecase.ListDocuments{Docs: docs},
		ListNodes:     usecase.ListNodes{Nodes: projects},
	}

	ctx := context.Background()
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	// Seed two docs with different UpdatedAt; Alpha is newest.
	_, _ = docs.Create(ctx, domain.Document{
		ID: "doc-alpha", OwnerID: "u1", Type: domain.DocFree, Path: "alpha",
		Title: "Alpha Article", UpdatedAt: now,
	})
	_, _ = docs.Create(ctx, domain.Document{
		ID: "doc-beta", OwnerID: "u1", Type: domain.DocFree, Path: "beta",
		Title: "Beta Article", UpdatedAt: now.Add(-time.Hour),
	})

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
		"Zuletzt",                    // "Zuletzt im Wissen" heading
		"Alpha Article",              // newest doc title
		`href="/wissen/doc-alpha"`,   // link to newest doc
		`href="/wissen/doc-beta"`,    // link to older doc (also within cap)
	} {
		if !strings.Contains(body, want) {
			t.Errorf("GET / newest-docs: missing %q", want)
		}
	}
	// Alpha (newer) must appear before Beta (older).
	alphaIdx := strings.Index(body, "Alpha Article")
	betaIdx := strings.Index(body, "Beta Article")
	if alphaIdx < 0 || betaIdx < 0 {
		t.Fatalf("both articles must appear; alpha=%d beta=%d", alphaIdx, betaIdx)
	}
	if betaIdx < alphaIdx {
		t.Errorf("Beta (older) rendered before Alpha (newer); want newest-first order")
	}
}

// TestHomeHome_RendersLanding verifies that GET / returns 200 and renders
// the Home heading plus section links for Zeit (/zeit), Wissen (/wissen),
// and Projekte (/nodes).
func TestHomeHome_RendersLanding(t *testing.T) {
	clk := testutil.FakeClock{T: time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x.de", "Martin")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour)

	srv := &httpserver.Server{
		Users:   users,
		Session: codec,
		Bus:     sse.NewBus(),
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

	// Authenticated → 200 with Home heading + three section links.
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
		"Home",           // heading — nav.home key renders "Home"
		`href="/zeit"`,   // Zeit section link
		`href="/wissen"`, // Wissen section link
		`href="/nodes"`,  // Projekte section link
		"/static/app.css",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("GET / missing %q", want)
		}
	}
}

// TestHomeHome_IdleShowsStartCard verifies GET / with no running session shows
// the home start card pointing at /ui/home/start and the SSE container.
func TestHomeHome_IdleShowsStartCard(t *testing.T) {
	srv := newWorktimeTestServer(t)
	cookieVal, _ := srv.codec.Issue("u1")
	req, _ := http.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET / status=%d body=%.500s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "/ui/home/start") {
		t.Errorf("idle home missing start form targeting /ui/home/start")
	}
	if strings.Contains(body, "/ui/home/stop") {
		t.Errorf("idle home must not show stop form")
	}
	if !strings.Contains(body, `id="content"`) {
		t.Errorf("home missing SSE content container id=content")
	}
}

// TestHomeHome_RunningShowsHero verifies GET / with a running session renders
// the live-timer hero (data-timer + stop form pointing at /ui/home/stop).
func TestHomeHome_RunningShowsHero(t *testing.T) {
	srv := newWorktimeTestServer(t)
	ctx := context.Background()
	// Running session started 2h before the fake clock (12:00 local).
	start := time.Date(2026, 6, 21, 10, 0, 0, 0, time.Local)
	if _, err := srv.ss.Create(ctx, domain.WorkSession{ID: "r", OwnerID: "u1", Start: start}); err != nil {
		t.Fatalf("seed running session: %v", err)
	}

	cookieVal, _ := srv.codec.Issue("u1")
	req, _ := http.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET / status=%d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{
		"data-timer",    // live-timer hook
		"/ui/home/stop", // stop form targets home stop handler
		`id="content"`,  // SSE container
	} {
		if !strings.Contains(body, want) {
			t.Errorf("running home missing %q", want)
		}
	}
	if strings.Contains(body, "/ui/home/start") {
		t.Errorf("running home must not show start form")
	}
}

// TestHomeFragment_IdleShowsStartCard verifies GET /ui/home (fragment) returns
// the start card in idle state.
func TestHomeFragment_IdleShowsStartCard(t *testing.T) {
	srv := newWorktimeTestServer(t)
	cookieVal, _ := srv.codec.Issue("u1")
	req, _ := http.NewRequest("GET", "/ui/home", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /ui/home status=%d body=%.500s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "/ui/home/start") {
		t.Errorf("home fragment missing start form")
	}
}

// TestHomeStart_StartsSession verifies POST /ui/home/start starts a session and
// renders the running hero (data-timer) in the Home fragment.
func TestHomeStart_StartsSession(t *testing.T) {
	srv := newWorktimeTestServer(t)
	cookieVal, _ := srv.codec.Issue("u1")
	req, _ := http.NewRequest("POST", "/ui/home/start", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("POST /ui/home/start status=%d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "data-timer") {
		t.Errorf("after home start, hero with data-timer should render; body=%.500s", body)
	}
	// Confirm session was actually started via GetRunningSession.
	_, ok, err := usecase.GetRunningSession{Sessions: srv.ss}.Execute(context.Background(), "u1")
	if err != nil || !ok {
		t.Errorf("no running session after POST /ui/home/start")
	}
}

// TestHomeHome_ShowsSaldoTilesAndBurndown verifies GET / with seeded sessions
// renders the 3 saldo tiles and the burndown banner with a non-zero saldo value.
// The fake clock is 2026-06-21 12:00 (local); a 2h session logged against an 8h
// default target produces a −6h TodaySaldo → "−" sign appears in the body.
func TestHomeHome_ShowsSaldoTilesAndBurndown(t *testing.T) {
	srv := newWorktimeTestServer(t)
	// Seed a 2h completed session today so the Today saldo is non-zero.
	srv.seedSession(t, "2026-06-21", "09:00", "11:00")

	cookieVal, _ := srv.codec.Issue("u1")
	req, _ := http.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET / status=%d body=%.500s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		"Monat gesamt",   // burndown banner eyebrow (stats.monthTotal → DE locale)
		"−",              // non-zero negative saldo (U+2212 from FmtSaldoVerbose)
		"sm:grid-cols-3", // saldo tile 3-column grid container
	} {
		if !strings.Contains(body, want) {
			t.Errorf("GET / saldo/burndown: missing %q", want)
		}
	}
}

// TestHomeStop_WithProjectStopsSession verifies POST /ui/home/stop with a valid
// projectId stops the session and renders the idle start card.
func TestHomeStop_WithProjectStopsSession(t *testing.T) {
	srv := newWorktimeTestServer(t)
	ctx := context.Background()
	p, err := (usecase.CreateNode{Nodes: srv.ps, IDs: srv.ids, Clock: srv.clk}).Execute(ctx, "u1", usecase.CreateNodeInput{Name: "flow", Color: "blue", Glyph: "◆", Kind: domain.KindEngagement})
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	start := time.Date(2026, 6, 21, 10, 0, 0, 0, time.Local)
	pid := p.ID
	if _, err := srv.ss.Create(ctx, domain.WorkSession{ID: "run", OwnerID: "u1", NodeID: &pid, Start: start}); err != nil {
		t.Fatalf("seed running session: %v", err)
	}

	cookieVal, _ := srv.codec.Issue("u1")
	req, _ := http.NewRequest("POST", "/ui/home/stop", strings.NewReader("sessionId=run&projectId="+p.ID))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("POST /ui/home/stop status=%d", rr.Code)
	}
	// Session must be stopped in the store.
	if got, _ := srv.ss.Get(ctx, "u1", "run"); got.Stop == nil {
		t.Errorf("session should be stopped after POST /ui/home/stop with project")
	}
	// After stop, idle home (start card) should render.
	if !strings.Contains(rr.Body.String(), "/ui/home/start") {
		t.Errorf("after home stop, idle start card should render; body=%.500s", rr.Body.String())
	}
}
