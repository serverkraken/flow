package httpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// histGet performs an authenticated GET for user "u1".
func histGet(t *testing.T, srv *worktimeTestServer, path string) *httptest.ResponseRecorder {
	t.Helper()
	cookieVal, _ := srv.codec.Issue("u1")
	req, _ := http.NewRequest("GET", path, nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)
	return rr
}

// seedHistProject creates a project for "u1" and returns its id.
func seedHistProject(t *testing.T, srv *worktimeTestServer, name string) string {
	t.Helper()
	p, err := (usecase.CreateNode{Nodes: srv.ps, IDs: srv.ids, Clock: srv.clk}).
		Execute(context.Background(), "u1", usecase.CreateNodeInput{Name: name, Color: "blue", Glyph: "◆", Kind: domain.KindEngagement})
	if err != nil {
		t.Fatalf("seedHistProject: %v", err)
	}
	return p.ID
}

// histSessionIDs lists "u1" session ids in the given local-date range.
func histSessionIDs(t *testing.T, srv *worktimeTestServer, from, to string) []string {
	t.Helper()
	day := func(s string) time.Time {
		d, _ := time.ParseInLocation("2006-01-02", s, time.Local)
		return d
	}
	sessions, _ := (usecase.ListSessionsRange{Sessions: srv.ss}).
		Execute(context.Background(), "u1", day(from), day(to))
	ids := make([]string, 0, len(sessions))
	for _, s := range sessions {
		ids = append(ids, s.ID)
	}
	return ids
}

// TestHistorieHome_RendersLesesaalChrome: GET /historie → 200 with the
// L4 Task 5 Lesesaal chrome (pagehead, "‹ Zeit" spine back-link, the time-grid)
// and NO worktime sub-tab-strip — that pill strip (and its worktimeSubnav
// definition) is retired; Historie was its last caller.
func TestHistorieHome_RendersLesesaalChrome(t *testing.T) {
	srv := newWorktimeTestServer(t)
	// Clock 2026-06-21 (Sun); ISO Monday 2026-06-15. Seed unassigned sessions.
	srv.seedSession(t, "2026-06-15", "09:00", "11:00")
	srv.seedSession(t, "2026-06-16", "14:00", "16:30")

	rr := histGet(t, srv, "/historie")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		"grid-lines",                    // the week time-grid columns
		"pagehead",                       // Lesesaal pagehead
		"‹ Zeit",                        // the spine "up" back-link to /zeit
		"id=\"content\"",                // SSE swap container
		"sse:session",                   // live-reload trigger
		"sse:dayoff.changed",            // SSE-Härtung (Berater-Fund #8): day-off must live-update the calendar
		"/static/js/historie-select.js", // selection JS loaded
		"/static/app.css",               // AppShell head
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Historie home missing %q", want)
		}
	}
	// The worktime sub-tab-strip (Heute/Woche/Historie pill nav) is retired
	// from Historie in L4 Task 5 — its href="/historie" active-tab link must
	// no longer appear (the "‹ Zeit" spine back-link replaces it).
	if strings.Contains(body, `href="/historie"`) {
		t.Errorf("Historie home must not render the retired worktime sub-tab-strip, got:\n%.2000s", body)
	}
}

// TestHistorieList_RendersRowsAndPagination: GET /historie?view=list → 200 with
// pagination + session rows.
func TestHistorieList_RendersRowsAndPagination(t *testing.T) {
	srv := newWorktimeTestServer(t)
	srv.seedSession(t, "2026-06-15", "09:00", "11:00")

	rr := histGet(t, srv, "/historie?view=list")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	// Pagination nav carries the localized aria-label + the "Seite X / Y" indicator.
	if !strings.Contains(body, "Seitennavigation") || !strings.Contains(body, "Seite 1 / 1") {
		t.Errorf("list view missing pagination nav, got:\n%s", body)
	}
	if !strings.Contains(body, "09:00–11:00") {
		t.Errorf("list view missing session row, got:\n%s", body)
	}
	if !strings.Contains(body, "data-session-id") {
		t.Errorf("list view missing selectable session rows")
	}
}

// TestHistorieReassign_AssignsAndRendersFragment: POST reassign ids=a,b&projectId=p1
// assigns the project (asserted via store) and returns the calendar fragment.
func TestHistorieReassign_AssignsAndRendersFragment(t *testing.T) {
	srv := newWorktimeTestServer(t)
	pid := seedHistProject(t, srv, "flow")
	srv.seedSession(t, "2026-06-15", "09:00", "11:00")
	srv.seedSession(t, "2026-06-16", "14:00", "16:30")
	ids := histSessionIDs(t, srv, "2026-06-15", "2026-06-22")
	if len(ids) != 2 {
		t.Fatalf("seed: expected 2 sessions, got %d", len(ids))
	}

	form := url.Values{
		"ids":       {strings.Join(ids, ",")},
		"projectId": {pid},
		"view":      {"cal"},
	}
	rr := srv.postForm(t, "/ui/historie/reassign", form)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "grid-lines") {
		t.Errorf("reassign should return the calendar fragment, got:\n%s", rr.Body.String())
	}
	// Assert via store: both sessions now carry the project id.
	sessions, _ := (usecase.ListSessionsRange{Sessions: srv.ss}).Execute(
		context.Background(), "u1",
		time.Date(2026, 6, 15, 0, 0, 0, 0, time.Local),
		time.Date(2026, 6, 22, 0, 0, 0, 0, time.Local),
	)
	for _, s := range sessions {
		if s.NodeID == nil || *s.NodeID != pid {
			t.Errorf("session %s not assigned to %s (got %v)", s.ID, pid, s.NodeID)
		}
	}
}

// TestHistorieReassign_InlineCreate: POST reassign with newProject creates the
// project and assigns the sessions to it.
func TestHistorieReassign_InlineCreate(t *testing.T) {
	srv := newWorktimeTestServer(t)
	srv.seedSession(t, "2026-06-15", "09:00", "11:00")
	ids := histSessionIDs(t, srv, "2026-06-15", "2026-06-22")
	if len(ids) != 1 {
		t.Fatalf("seed: expected 1 session, got %d", len(ids))
	}

	form := url.Values{
		"ids":        {ids[0]},
		"newProject": {"flux-migration"},
		"view":       {"cal"},
	}
	rr := srv.postForm(t, "/ui/historie/reassign", form)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	// The inline-created project exists and the session points at it.
	projects, _ := (usecase.ListNodes{Nodes: srv.ps}).Execute(context.Background(), "u1")
	if len(projects) != 1 || projects[0].Name != "flux-migration" {
		t.Fatalf("inline-create: expected project flux-migration, got %#v", projects)
	}
	sessions, _ := (usecase.ListSessionsRange{Sessions: srv.ss}).Execute(
		context.Background(), "u1",
		time.Date(2026, 6, 15, 0, 0, 0, 0, time.Local),
		time.Date(2026, 6, 22, 0, 0, 0, 0, time.Local),
	)
	if len(sessions) != 1 || sessions[0].NodeID == nil || *sessions[0].NodeID != projects[0].ID {
		t.Errorf("session not assigned to inline-created project, got %#v", sessions)
	}
}

// TestHistorieReassign_NoProjectErrors: a reassign with neither projectId nor
// newProject returns the fragment with an error banner (and changes nothing).
func TestHistorieReassign_NoProjectErrors(t *testing.T) {
	srv := newWorktimeTestServer(t)
	srv.seedSession(t, "2026-06-15", "09:00", "11:00")
	ids := histSessionIDs(t, srv, "2026-06-15", "2026-06-22")

	form := url.Values{"ids": {strings.Join(ids, ",")}, "view": {"cal"}}
	rr := srv.postForm(t, "/ui/historie/reassign", form)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	// Still unassigned.
	sessions, _ := (usecase.ListSessionsRange{Sessions: srv.ss}).Execute(
		context.Background(), "u1",
		time.Date(2026, 6, 15, 0, 0, 0, 0, time.Local),
		time.Date(2026, 6, 22, 0, 0, 0, 0, time.Local),
	)
	if sessions[0].NodeID != nil {
		t.Errorf("expected session to remain unassigned, got %v", sessions[0].NodeID)
	}
}

// TestHistorieBulkDelete_RemovesSessions: POST bulk-delete removes the sessions
// and returns the refreshed fragment (the #content innerHTML swap target).
func TestHistorieBulkDelete_RemovesSessions(t *testing.T) {
	srv := newWorktimeTestServer(t)
	srv.seedSession(t, "2026-06-15", "09:00", "11:00")
	srv.seedSession(t, "2026-06-16", "14:00", "16:30")
	ids := histSessionIDs(t, srv, "2026-06-15", "2026-06-22")
	if len(ids) != 2 {
		t.Fatalf("seed: expected 2 sessions, got %d", len(ids))
	}

	form := url.Values{"ids": {strings.Join(ids, ",")}, "view": {"list"}}
	rr := srv.postForm(t, "/ui/historie/bulk-delete", form)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	// Handler returns the inner fragment (no AppShell <html> wrapper), matching
	// what htmx swaps into #content via innerHTML.
	body := rr.Body.String()
	if strings.Contains(body, "<html") {
		t.Errorf("bulk-delete should return the fragment (not the full page), got html tag")
	}
	if !strings.Contains(body, "Seitennavigation") && !strings.Contains(body, "data-session-id") && !strings.Contains(body, "keine Sitzungen") {
		// At least one of: list fragment with pagination or empty state is expected.
		// After delete both sessions are gone, so the empty state banner should appear.
		if !strings.Contains(body, "Keine Sitzungen") && !strings.Contains(body, "historie") {
			t.Errorf("bulk-delete response doesn't look like the list fragment, got:\n%s", body[:min(500, len(body))])
		}
	}
	after := histSessionIDs(t, srv, "2026-06-15", "2026-06-22")
	if len(after) != 0 {
		t.Errorf("expected 0 sessions after bulk-delete, got %d", len(after))
	}
}

// TestHistorieCalFragment_EditFormHasProjects: GET /ui/historie/calendar renders
// the edit dialog with project options populated in the select.
func TestHistorieCalFragment_EditFormHasProjects(t *testing.T) {
	srv := newWorktimeTestServer(t)
	seedHistProject(t, srv, "flow-rebuild")
	srv.seedSession(t, "2026-06-16", "09:00", "10:30")

	rr := histGet(t, srv, "/ui/historie/calendar")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `value="id-1"`) {
		t.Errorf("edit form missing project option with project id, got:\n%s", body[:min(2000, len(body))])
	}
	if !strings.Contains(body, "flow-rebuild") {
		t.Errorf("edit form missing project name 'flow-rebuild'")
	}
	// The single-edit dialog's project select must post the `node` field, matching
	// the /ui/worktime/edit handler (handleWebEdit reads webNode() → "node"). It
	// used to post the stale `projectId` name, which the handler now ignores.
	if !strings.Contains(body, `name="node" data-edit-field-project`) {
		t.Errorf("edit form project select must post name=\"node\" (matching /ui/worktime/edit), got:\n%s", body[:min(2000, len(body))])
	}
	// The bulk-reassign form (SelectionActionBar) legitimately still carries a
	// hidden newProject field, so scope the "no inline-create in single-edit"
	// check to the historieEditForm markup itself (bounded by its <form>..</form>).
	editFormStart := strings.Index(body, `hx-post="/ui/worktime/edit"`)
	editFormEnd := strings.Index(body[editFormStart:], "</form>")
	if editFormStart < 0 || editFormEnd < 0 {
		t.Fatalf("could not locate historieEditForm boundaries in body")
	}
	editFormHTML := body[editFormStart : editFormStart+editFormEnd]
	if strings.Contains(editFormHTML, "newProject") {
		t.Errorf("single-edit dialog must not carry a newProject inline-create field (dead: /ui/worktime/edit doesn't create), got:\n%s", editFormHTML)
	}
}

// nonDialogContent trims a rendered page/fragment to the part BEFORE the
// first native <dialog> element. Historie legitimately keeps its mandated
// components.Dialog/ConfirmDialog modals (edit + 2 confirms — Bestand, shared
// with Heute/Editor/Frei), which carry their own rounded-3xl/shadow-lift/
// shadow-soft/font-display chrome by design (a floating modal, not a
// page-flow card) — so the "retired Kristall card chrome" check below is
// scoped to the calendar/list page-flow content, not the modals.
func nonDialogContent(body string) string {
	if i := strings.Index(body, "<dialog"); i >= 0 {
		return body[:i]
	}
	return body
}

// TestHistorie_NoKristallChromeAndBulkAttrsPreserved: the L4 Task 5 Lesesaal
// rebuild must not break the bulk-select data-attrs or the single-edit
// dialog's post target, and the calendar/list page-flow content must no
// longer render the retired Kristall card chrome (glass/shadow-soft/
// rounded-3xl) it used to carry.
func TestHistorie_NoKristallChromeAndBulkAttrsPreserved(t *testing.T) {
	srv := newWorktimeTestServer(t)
	srv.seedSession(t, "2026-06-15", "09:00", "11:00")

	rr := histGet(t, srv, "/historie")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	content := nonDialogContent(body)
	for _, unwanted := range []string{"glass", "shadow-soft", "rounded-3xl"} {
		if strings.Contains(content, unwanted) {
			t.Errorf("historie page-flow content must not render retired Kristall chrome %q, got:\n%.3000s", unwanted, content)
		}
	}
	for _, want := range []string{"data-select-toggle", "data-block-wrap", "/ui/worktime/edit"} {
		if !strings.Contains(body, want) {
			t.Errorf("historie lost %q", want)
		}
	}

	rrList := histGet(t, srv, "/historie?view=list")
	if rrList.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rrList.Code, rrList.Body.String())
	}
	listBody := rrList.Body.String()
	listContent := nonDialogContent(listBody)
	for _, unwanted := range []string{"glass", "shadow-soft", "rounded-3xl"} {
		if strings.Contains(listContent, unwanted) {
			t.Errorf("historie list view page-flow content must not render retired Kristall chrome %q, got:\n%.3000s", unwanted, listContent)
		}
	}
}

// TestHistorieCalFragment_BlockWrapCarriesEditTo: the calendar fragment's block
// wrappers carry data-edit-to so the JS can prefill the stop field.
func TestHistorieCalFragment_BlockWrapCarriesEditTo(t *testing.T) {
	srv := newWorktimeTestServer(t)
	srv.seedSession(t, "2026-06-16", "09:00", "10:30")

	rr := histGet(t, srv, "/ui/historie/calendar")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "data-edit-to=") {
		t.Errorf("block wrapper missing data-edit-to attribute")
	}
	// The stop time "10:30" should appear as data-edit-to value.
	if !strings.Contains(body, `data-edit-to="10:30"`) {
		t.Errorf("block wrapper data-edit-to missing expected stop time '10:30', body excerpt:\n%s", body[:min(3000, len(body))])
	}
}

// TestHistorieBulkErr_ProjectNotFound: historieBulkErr maps ErrNodeNotFound
// to a clean message (not the raw Go error).
func TestHistorieBulkErr_ProjectNotFound(t *testing.T) {
	srv := newWorktimeTestServer(t)
	srv.seedSession(t, "2026-06-15", "09:00", "11:00")
	ids := histSessionIDs(t, srv, "2026-06-15", "2026-06-22")

	// POST a non-existent project id → backend returns ErrNodeNotFound.
	form := url.Values{
		"ids":       {strings.Join(ids, ",")},
		"projectId": {"does-not-exist"},
		"view":      {"cal"},
	}
	rr := srv.postForm(t, "/ui/historie/reassign", form)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if strings.Contains(body, "project not found") || strings.Contains(body, "does-not-exist") {
		t.Errorf("raw error leaked to UI: %q", body[:min(500, len(body))])
	}
	if !strings.Contains(body, "Projekt nicht gefunden") {
		t.Errorf("expected 'Projekt nicht gefunden' banner, got:\n%s", body[:min(1000, len(body))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestHistorieCal_ShowsDayOffBadge covers the #8 day-off rendering: a vacation
// day in the week must surface its label badge in the calendar.
func TestHistorieCal_ShowsDayOffBadge(t *testing.T) {
	srv := newWorktimeTestServer(t)
	ctx := context.Background()
	// Clock is 2026-06-21 (week Mon 06-15 .. Sun 06-21). Seed vacation on Wed 06-17.
	day := time.Date(2026, 6, 17, 0, 0, 0, 0, time.Local)
	if err := srv.dos.Add(ctx, "u1", domain.DayOff{Date: day, Kind: domain.KindVacation, Label: "Urlaub"}); err != nil {
		t.Fatalf("seed dayoff: %v", err)
	}
	cookieVal, _ := srv.codec.Issue("u1")
	req, _ := http.NewRequest("GET", "/historie?week=2026-06-15", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Urlaub") {
		t.Errorf("calendar missing day-off badge label %q", "Urlaub")
	}
}

// TestHistorieFragments_NoKristallChrome checks the raw calendar/list fragment
// endpoints (no AppShell topbar, so "font-display" cannot leak in from the
// shared brand mark either) for the retired-Kristall marker set, scoped to the
// page-flow content (the mandated edit/confirm <dialog> modals keep their own
// rounded-3xl/shadow-lift/shadow-soft/font-display chrome by design).
func TestHistorieFragments_NoKristallChrome(t *testing.T) {
	srv := newWorktimeTestServer(t)
	srv.seedSession(t, "2026-06-15", "09:00", "11:00")

	for _, path := range []string{"/ui/historie/calendar", "/ui/historie/list"} {
		rr := histGet(t, srv, path)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, rr.Code, rr.Body.String())
		}
		content := nonDialogContent(rr.Body.String())
		for _, unwanted := range []string{"glass", "shadow-soft", "rounded-3xl", "font-display"} {
			if strings.Contains(content, unwanted) {
				t.Errorf("%s page-flow content must not render retired Kristall chrome %q, got:\n%.3000s", path, unwanted, content)
			}
		}
	}
}

// TestHistorie_OwnerScoped is the owner-scope negative test for Historie
// (AGENTS.md §Grundsätze — flow is multi-tenant): another user's session must
// never surface in u1's Historie calendar or list.
func TestHistorie_OwnerScoped(t *testing.T) {
	srv := newWorktimeTestServer(t)
	srv.seedSession(t, "2026-06-15", "09:00", "10:00") // u1's own session

	start := time.Date(2026, 6, 15, 9, 0, 0, 0, time.Local)
	stop := start.Add(6 * time.Hour)
	if _, err := srv.ss.Create(context.Background(), domain.WorkSession{
		ID: "u2-secret", OwnerID: "u2", Start: start, Stop: &stop, Tags: []string{"u2-only-tag"},
	}); err != nil {
		t.Fatalf("seed u2 session: %v", err)
	}

	rr := histGet(t, srv, "/historie")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if strings.Contains(body, "u2-only-tag") {
		t.Errorf("owner-scope leak: u1's Historie calendar rendered u2's session tag: %.2000s", body)
	}

	rrList := histGet(t, srv, "/historie?view=list")
	if rrList.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rrList.Code, rrList.Body.String())
	}
	listBody := rrList.Body.String()
	if strings.Contains(listBody, "u2-only-tag") {
		t.Errorf("owner-scope leak: u1's Historie list rendered u2's session tag: %.2000s", listBody)
	}
	if strings.Contains(listBody, "u2-secret") {
		t.Errorf("owner-scope leak: u1's Historie list rendered u2's session id: %.2000s", listBody)
	}
}
