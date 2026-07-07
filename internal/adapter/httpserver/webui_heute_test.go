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
// Zeit-Hub page on the AppShell with the offline app.css + the SSE content
// container, on the Lesesaal .row/.led-when ledger surface (L4 Task 3 — the
// Kristall "glass" ledger card is gone), and carries NO Heute-owned timer
// control forms: start/stop is owned entirely by the K1 shell sidebar widget
// now (mounted separately via its own lazy hx-get="/ui/timer", not by Zeit).
// A running-today session still shows as a read-only LIVE ledger row (no
// stop button — Spec §10 anzeige-only).
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
		"led-when",        // Lesesaal ledger row (L4 Task 3)
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Heute home missing %q", want)
		}
	}
	for _, unwanted := range []string{"/ui/worktime/stop", "/ui/worktime/start", "glass"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("Heute must not render timer control markup / Kristall glass %q (owned by the sidebar widget / retired by L4)", unwanted)
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

// The mandatory-booking-on-stop behavior (project-required error, keep
// running without a project, book+stop with one) is now covered end-to-end
// against the K1 shell timer widget's /ui/timer/stop route by
// TestTimerWidget_Lifecycle (webui_timer_test.go) — the sole stop surface
// since K3 Task 6 retired /ui/worktime/stop.

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
// becomes a pure Lesesaal .row ledger (L4 Task 3 — the Kristall glass card
// is retired). The K1 shell timer widget (sidebar) owns start/stop now, so
// /zeit must render neither timer control form; the Nachbuchen affordance
// opens the shared add SessionDialog, each session carries a per-row edit
// SessionDialog, and delete keeps its ConfirmDialog.
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
	// Lesesaal ledger row, not the retired Kristall glass card
	if strings.Contains(body, "glass") {
		t.Errorf("ledger must not render on the retired Kristall glass surface")
	}
	if !strings.Contains(body, "led-when") {
		t.Errorf("ledger not on the Lesesaal .row/.led-when surface")
	}
}

// TestZeitHub_WeekbarAndWerkzeuge is the L4 Task 3 RED→GREEN guard: /zeit
// renders the vertical 7-day Wochenskala (.weekbar/.day) and the four
// Werkzeuge rows (Export/Freie Tage/Statistik/Historie hrefs), and carries
// NEITHER the retired sub-tab-strip (TabStrip's own .pill-tabs marker) NOR
// the retired Saldo-Kacheln (StatTileAccent's own .rtile-ac marker).
func TestZeitHub_WeekbarAndWerkzeuge(t *testing.T) {
	srv := newWorktimeTestServer(t)
	srv.seedSession(t, "2026-06-15", "09:00", "17:00") // Monday this ISO week

	rr := histGet(t, srv, "/zeit")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		`class="weekbar"`,
		`class="day has"`,
		`href="/export"`,
		`href="/dayoffs"`,
		`href="/woche"`,
		`href="/historie"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Zeit-Hub missing %q, got:\n%.3000s", want, body)
		}
	}
	for _, unwanted := range []string{"pill-tabs", "rtile-ac"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("Zeit-Hub must not render retired marker %q (sub-tab-strip / Saldo-Kachel)", unwanted)
		}
	}
}

// TestZeitHub_RunningSessionShowsLiveRow verifies a session running today
// renders the LIVE ledger row: the livechip, a ticking data-timer span with
// its data-base seconds, and the "{start} – Läuft" led-when label — never a
// stop control (Spec §10 anzeige-only, l4-global-constraints.md).
func TestZeitHub_RunningSessionShowsLiveRow(t *testing.T) {
	srv := newWorktimeTestServer(t)
	start := time.Date(2026, 6, 21, 10, 0, 0, 0, time.Local) // 2h before the 12:00 clock
	if _, err := srv.ss.Create(context.Background(), domain.WorkSession{ID: "r", OwnerID: "u1", Start: start}); err != nil {
		t.Fatalf("seed running: %v", err)
	}

	rr := histGet(t, srv, "/zeit")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		"livechip",
		`data-timer data-timer-fmt="clock" data-base="7200"`,
		"10:00 – Läuft",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Zeit-Hub LIVE row missing %q, got:\n%.3000s", want, body)
		}
	}
	if strings.Contains(body, "/ui/worktime/stop") {
		t.Errorf("Zeit-Hub LIVE row must never render a stop control (anzeige-only, Spec §10)")
	}
}

// TestZeitHub_LedgerOwnerScoped is the owner-scope negative test for the Zeit
// ledger + the Σ all-time line: user B's session must never surface on user
// A's Zeit-Hub (AGENTS.md §Grundsätze — flow is multi-tenant).
func TestZeitHub_LedgerOwnerScoped(t *testing.T) {
	srv := newWorktimeTestServer(t)
	// u1's own session today (so the ledger/Σ line aren't simply empty).
	srv.seedSession(t, "2026-06-21", "09:00", "10:00")
	// u2's session, same day, distinctly tagged.
	start := time.Date(2026, 6, 21, 9, 0, 0, 0, time.Local)
	stop := start.Add(3 * time.Hour)
	if _, err := srv.ss.Create(context.Background(), domain.WorkSession{
		ID: "u2-secret", OwnerID: "u2", Start: start, Stop: &stop, Tags: []string{"u2-only-tag"},
	}); err != nil {
		t.Fatalf("seed u2 session: %v", err)
	}

	rr := histGet(t, srv, "/zeit")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if strings.Contains(body, "u2-only-tag") {
		t.Errorf("owner-scope leak: u1's Zeit-Hub rendered u2's session tag: %.2000s", body)
	}
	if strings.Contains(body, "09:00–12:00") {
		t.Errorf("owner-scope leak: u1's Zeit-Hub rendered u2's 3h session time range: %.2000s", body)
	}
}

// TestZeitHub_LedgerRowShowsNoteAndClockDuration is the Review Fix 1/2
// RED→GREEN guard: a completed session's ledger .s sub-line shows its
// free-text Note (Mockup Z.858–866, e.g. "Daily + DACORE-10279 Review"), and
// its duration column renders in the Mockup's colon clock format ("2:00"),
// not the codebase-wide FmtVerbose "2h 00m".
func TestZeitHub_LedgerRowShowsNoteAndClockDuration(t *testing.T) {
	srv, u := newHeuteTestServer(t)
	seedCompletedSession(t, srv, u, "n1", "09:00", "11:00", []string{"deep"}, "Daily + Review")

	body := getBody(t, srv, u, "/zeit")
	if !strings.Contains(body, "Daily + Review") {
		t.Errorf("ledger row missing the session's Note text: %.2000s", body)
	}
	if strings.Contains(body, "#deep") {
		t.Errorf("ledger row must show the Note, not the tag fallback, when Note is set: %.2000s", body)
	}
	if !strings.Contains(body, ">2:00<") {
		t.Errorf("ledger row missing the clock-format duration '2:00': %.2000s", body)
	}
}

// TestZeitHub_LedgerRowFallsBackToTagsWhenNoteEmpty verifies the .s sub-line
// falls back to the session's tags (Review Fix 2) when there is no Note.
func TestZeitHub_LedgerRowFallsBackToTagsWhenNoteEmpty(t *testing.T) {
	srv, u := newHeuteTestServer(t)
	seedCompletedSession(t, srv, u, "n1", "09:00", "11:00", []string{"deep", "review"}, "")

	body := getBody(t, srv, u, "/zeit")
	if !strings.Contains(body, "#deep #review") {
		t.Errorf("ledger row missing the tag fallback when Note is empty: %.2000s", body)
	}
}
