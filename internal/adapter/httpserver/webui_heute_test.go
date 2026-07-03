package httpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// TestHeuteHome_RendersLedgerNoLiveTimerHook verifies GET /zeit renders the
// Heute page on the AppShell with the offline app.css + the SSE content
// container, and — since Kristall K3 — carries NO Heute-owned timer control
// forms: start/stop is owned entirely by the K1 shell sidebar widget now
// (mounted separately via its own lazy hx-get="/ui/timer", not by Heute). A
// running-today session still shows as a read-only ledger row (no stop button).
func TestHeuteHome_RendersLedgerNoLiveTimerHook(t *testing.T) {
	srv := newWorktimeTestServer(t)
	ctx := context.Background()
	// A running session started 2h before the fake clock (12:00 local).
	start := time.Date(2026, 6, 21, 10, 0, 0, 0, time.Local)
	if _, err := srv.ss.Create(ctx, domain.WorkSession{ID: "r", OwnerID: "u1", Start: start}); err != nil {
		t.Fatalf("seed running: %v", err)
	}

	cookieVal, _ := srv.codec.Issue("u1")
	req, _ := http.NewRequest("GET", "/zeit", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		"/static/app.css", // offline stylesheet
		"id=\"content\"",  // SSE swap container
		"glass",           // Kristall ledger surface
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Heute home missing %q", want)
		}
	}
	for _, unwanted := range []string{"/ui/worktime/stop", "/ui/worktime/start"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("Heute must not render timer control markup %q (owned by the sidebar widget)", unwanted)
		}
	}
}

// TestHeuteHome_OvernightRunningSessionNoTimerMarkup is the K3 update of the
// old #5 regression (a timer left running from a PREVIOUS day must not break
// Heute): the running session itself is now surfaced by the sidebar widget,
// not the day-scoped ledger, so this only pins that /zeit still renders
// cleanly (200, no leaked timer-control markup) with an overnight-running
// timer in play.
func TestHeuteHome_OvernightRunningSessionNoTimerMarkup(t *testing.T) {
	srv := newWorktimeTestServer(t)
	ctx := context.Background()
	// Clock is 2026-06-21 12:00; seed a running session that started the DAY
	// BEFORE (no stop). It is outside today's range yet must not break /zeit.
	start := time.Date(2026, 6, 20, 18, 51, 0, 0, time.Local)
	if _, err := srv.ss.Create(ctx, domain.WorkSession{ID: "overnight", OwnerID: "u1", Start: start}); err != nil {
		t.Fatalf("seed overnight: %v", err)
	}

	cookieVal, _ := srv.codec.Issue("u1")
	req, _ := http.NewRequest("GET", "/zeit", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, unwanted := range []string{"/ui/worktime/stop", "/ui/worktime/start"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("Heute must not render timer control markup %q with an overnight running session: %.500s", unwanted, body)
		}
	}
}

// TestHeuteHome_EmptyShowsLedgerEmptyState verifies the idle state renders
// the ledger's empty state + the Nachbuchen add dialog — NOT a start form
// (the K1 shell sidebar widget owns starting a timer now).
func TestHeuteHome_EmptyShowsLedgerEmptyState(t *testing.T) {
	srv := newWorktimeTestServer(t)
	cookieVal, _ := srv.codec.Issue("u1")
	req, _ := http.NewRequest("GET", "/zeit", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if strings.Contains(body, "/ui/worktime/start") {
		t.Errorf("idle Heute must not render the start form (owned by the sidebar widget)")
	}
	if !strings.Contains(body, "/ui/worktime/add") {
		t.Errorf("idle Heute missing the Nachbuchen add SessionDialog")
	}
}

// TestHeuteFragment_ListsSessions verifies the fragment lists a completed
// session row with its time range.
func TestHeuteFragment_ListsSessions(t *testing.T) {
	srv := newWorktimeTestServer(t)
	ctx := context.Background()
	day := time.Date(2026, 6, 21, 0, 0, 0, 0, time.Local)
	from := day.Add(9 * time.Hour)
	to := day.Add(11 * time.Hour)
	if _, err := (usecase.AddSession{Sessions: srv.ss, IDs: srv.ids, Clock: srv.clk}).Execute(
		ctx, "u1", nil, from, to, nil, "",
	); err != nil {
		t.Fatalf("seed completed: %v", err)
	}

	cookieVal, _ := srv.codec.Issue("u1")
	req, _ := http.NewRequest("GET", "/ui/worktime", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "09:00–11:00") {
		t.Errorf("fragment missing session row, got:\n%s", rr.Body.String())
	}
}

// TestWebStop_NoProjectSurfacesError covers the #6 fix: stopping a running
// session without booking a project must surface the "choose a project" error
// (and keep the timer running) instead of silently doing nothing.
func TestWebStop_NoProjectSurfacesError(t *testing.T) {
	srv := newWorktimeTestServer(t)
	ctx := context.Background()
	start := time.Date(2026, 6, 21, 10, 0, 0, 0, time.Local)
	if _, err := srv.ss.Create(ctx, domain.WorkSession{ID: "run", OwnerID: "u1", Start: start}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cookieVal, _ := srv.codec.Issue("u1")
	req, _ := http.NewRequest("POST", "/ui/worktime/stop", strings.NewReader("sessionId=run"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Projekt wählen") {
		t.Errorf("stop without project should surface the project-required error; body=%s", body[:min(400, len(body))])
	}
	// timer still running (not stopped)
	if got, _ := srv.ss.Get(ctx, "u1", "run"); got.Stop != nil {
		t.Errorf("session should NOT be stopped without a project")
	}
}

// TestWebStop_WithProjectBooksAndStops covers the #6 happy path: stopping with
// a project books + stops the session and re-renders the Heute ledger without
// the "choose a project" error.
func TestWebStop_WithProjectBooksAndStops(t *testing.T) {
	srv := newWorktimeTestServer(t)
	ctx := context.Background()
	p, err := (usecase.CreateNode{Nodes: srv.ps, IDs: srv.ids, Clock: srv.clk}).Execute(ctx, "u1", usecase.CreateNodeInput{Name: "flow", Color: "blue", Glyph: "◆", Kind: domain.KindEngagement})
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	start := time.Date(2026, 6, 21, 10, 0, 0, 0, time.Local)
	pid := p.ID
	if _, err := srv.ss.Create(ctx, domain.WorkSession{ID: "run", OwnerID: "u1", NodeID: &pid, Start: start}); err != nil {
		t.Fatalf("seed running: %v", err)
	}
	cookieVal, _ := srv.codec.Issue("u1")
	req, _ := http.NewRequest("POST", "/ui/worktime/stop", strings.NewReader("sessionId=run&projectId="+p.ID))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if got, _ := srv.ss.Get(ctx, "u1", "run"); got.Stop == nil {
		t.Errorf("session should be stopped after booking a project")
	}
	if strings.Contains(rr.Body.String(), "Projekt wählen") {
		t.Errorf("stop with a project must not surface the project-required error")
	}
}

// TestHeuteEditDialog_BookableKindsOnly verifies the Spec #1-Fix survives the
// Kristall ledger rewrite: the booking picker now lives in a completed
// session's per-row edit SessionDialog (the old stop-form picker is gone —
// start/stop is owned by the sidebar timer widget). It lists every BOOKABLE
// kind — Engagement, Vorhaben, AND Repo (domain.IsBookable) — not just
// Engagement, excludes the non-bookable Branch, and preselects the session's
// own node (a Repo here) regardless of kind.
func TestHeuteEditDialog_BookableKindsOnly(t *testing.T) {
	srv := newWorktimeTestServer(t)
	ctx := context.Background()

	// Seed one of each kind: Engagement, its Vorhaben child, the Vorhaben's
	// Repo child, and the Repo's Branch child (not bookable).
	eng, err := (usecase.CreateNode{Nodes: srv.ps, IDs: srv.ids, Clock: srv.clk}).Execute(
		ctx, "u1", usecase.CreateNodeInput{Name: "MyEngagement", Kind: domain.KindEngagement},
	)
	if err != nil {
		t.Fatalf("seed engagement: %v", err)
	}
	vor, err := (usecase.CreateNode{Nodes: srv.ps, IDs: srv.ids, Clock: srv.clk}).Execute(
		ctx, "u1", usecase.CreateNodeInput{Name: "MyVorhaben", Kind: domain.KindVorhaben, ParentID: &eng.ID},
	)
	if err != nil {
		t.Fatalf("seed vorhaben: %v", err)
	}
	repo, err := (usecase.CreateNode{Nodes: srv.ps, IDs: srv.ids, Clock: srv.clk}).Execute(
		ctx, "u1", usecase.CreateNodeInput{Name: "MyRepo", Kind: domain.KindRepo, ParentID: &vor.ID},
	)
	if err != nil {
		t.Fatalf("seed repo: %v", err)
	}
	_, err = (usecase.CreateNode{Nodes: srv.ps, IDs: srv.ids, Clock: srv.clk}).Execute(
		ctx, "u1", usecase.CreateNodeInput{Name: "MyBranch", Kind: domain.KindBranch, ParentID: &repo.ID},
	)
	if err != nil {
		t.Fatalf("seed branch: %v", err)
	}

	// Completed session booked on the Repo (not the Engagement) — the edit
	// dialog's preselection must follow the session's own node regardless of
	// kind.
	start := time.Date(2026, 6, 21, 10, 0, 0, 0, time.Local)
	stop := start.Add(2 * time.Hour)
	if _, err := srv.ss.Create(ctx, domain.WorkSession{ID: "sess-bk", OwnerID: "u1", NodeID: &repo.ID, Start: start, Stop: &stop}); err != nil {
		t.Fatalf("seed completed session: %v", err)
	}

	cookieVal, _ := srv.codec.Issue("u1")
	req, _ := http.NewRequest("GET", "/zeit", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"MyEngagement", "MyVorhaben", "MyRepo"} {
		if !strings.Contains(body, want) {
			t.Errorf("edit dialog missing bookable node %q: %.2000s", want, body)
		}
	}
	if strings.Contains(body, "MyBranch") {
		t.Errorf("edit dialog must NOT list non-bookable branch node: %.2000s", body)
	}
	// The session's own Repo must be preselected in its edit dialog.
	if !strings.Contains(body, `<option value="`+repo.ID+`" selected>MyRepo</option>`) {
		t.Errorf("edit dialog must preselect the session's own repo node: %.2000s", body)
	}
}

// newHeuteTestServer wraps newWorktimeTestServer for the compact Kristall
// ledger tests below (Task 4), returning the fixed test owner id alongside it.
func newHeuteTestServer(t *testing.T) (*worktimeTestServer, string) {
	t.Helper()
	return newWorktimeTestServer(t), "u1"
}

// seedCompletedSession inserts a completed session directly into the fake
// store for owner u, booked to nodeID (auto-seeding a bookable node with that
// id if nodeID != ""), on the fixed test clock's day, from/to as "HH:MM".
func seedCompletedSession(t *testing.T, srv *worktimeTestServer, u, nodeID, from, to string, tags []string, note string) domain.WorkSession {
	t.Helper()
	if nodeID != "" {
		srv.seedNode(t, domain.Node{ID: nodeID, OwnerID: u, Name: nodeID, Slug: nodeID, Kind: domain.KindEngagement})
	}
	day := srv.clk.T.Format("2006-01-02")
	fromT, err := time.ParseInLocation("2006-01-02 15:04", day+" "+from, time.Local)
	if err != nil {
		t.Fatalf("seedCompletedSession: parse from %q: %v", from, err)
	}
	toT, err := time.ParseInLocation("2006-01-02 15:04", day+" "+to, time.Local)
	if err != nil {
		t.Fatalf("seedCompletedSession: parse to %q: %v", to, err)
	}
	var nid *string
	if nodeID != "" {
		nid = &nodeID
	}
	sess := domain.WorkSession{
		ID:      srv.ids.NewID(),
		OwnerID: u,
		NodeID:  nid,
		Start:   fromT,
		Stop:    &toT,
		Tags:    tags,
		Note:    note,
	}
	if _, err := srv.ss.Create(context.Background(), sess); err != nil {
		t.Fatalf("seedCompletedSession: %v", err)
	}
	return sess
}

// getBody GETs path as authenticated user u and returns the response body,
// failing the test on a non-200 status.
func getBody(t *testing.T, srv *worktimeTestServer, u, path string) string {
	t.Helper()
	cookieVal, _ := srv.codec.Issue(u)
	req, _ := http.NewRequest("GET", path, nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s status=%d body=%s", path, rr.Code, rr.Body.String())
	}
	return rr.Body.String()
}

// TestHeutePage_LedgerNoTimerForms is the Task 4 RED→GREEN guard: Heute
// becomes a pure glass ledger. The K1 shell timer widget (sidebar) owns
// start/stop now, so /zeit must render neither timer control form; the
// Nachbuchen affordance opens the shared add SessionDialog, each session
// carries a per-row edit SessionDialog, delete keeps its ConfirmDialog, and
// the ledger renders on the Kristall glass surface.
func TestHeutePage_LedgerNoTimerForms(t *testing.T) {
	srv, u := newHeuteTestServer(t)
	seedCompletedSession(t, srv, u, "n1", "09:00", "11:00", nil, "")
	body := getBody(t, srv, u, "/zeit")
	// the timer control forms are gone
	if strings.Contains(body, "/ui/worktime/start") || strings.Contains(body, "/ui/worktime/stop") {
		t.Errorf("Heute must not render start/stop forms")
	}
	// add dialog is the shared SessionDialog (session.dialog.date key rendered)
	if !strings.Contains(body, "/ui/worktime/add") {
		t.Errorf("add SessionDialog missing")
	}
	// per-row edit dialog present + delete confirm present
	if !strings.Contains(body, `id="edit-`) {
		t.Errorf("per-row edit dialog missing")
	}
	if !strings.Contains(body, "/ui/worktime/delete") {
		t.Errorf("delete confirm missing")
	}
	// glass ledger cards
	if !strings.Contains(body, "glass") {
		t.Errorf("ledger not on glass")
	}
}
