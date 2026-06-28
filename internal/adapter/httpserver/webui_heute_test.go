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

// TestHeuteHome_RendersLiveAndSessions verifies GET / renders the Heute page on
// the AppShell with the live-timer hook, the offline app.css, the SSE content
// container, and the running session's elapsed-seconds base attribute.
func TestHeuteHome_RendersLiveAndSessions(t *testing.T) {
	srv := newWorktimeTestServer(t)
	ctx := context.Background()
	// A running session started 2h before the fake clock (12:00 local).
	start := time.Date(2026, 6, 21, 10, 0, 0, 0, time.Local)
	if _, err := srv.ss.Create(ctx, domain.WorkSession{ID: "r", OwnerID: "u1", Start: start}); err != nil {
		t.Fatalf("seed running: %v", err)
	}

	cookieVal, _ := srv.codec.Issue("u1")
	req, _ := http.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		"data-timer",         // Base live-timer hook
		"data-base=\"7200\"", // 2h elapsed seed in seconds
		"/static/app.css",    // offline stylesheet
		"id=\"content\"",     // SSE swap container
		"/ui/worktime/stop",  // stop control for the running session
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Heute home missing %q", want)
		}
	}
}

// TestHeuteHome_EmptyShowsStart verifies the idle state renders the start form.
// TestHeuteHome_ShowsOvernightRunningSession is the #5 regression: a timer left
// running from a PREVIOUS day must still appear on Heute (hero + stop control),
// not be hidden because Heute is "today-only" — otherwise it silently blocks a
// new start with no way to stop it.
func TestHeuteHome_ShowsOvernightRunningSession(t *testing.T) {
	srv := newWorktimeTestServer(t)
	ctx := context.Background()
	// Clock is 2026-06-21 12:00; seed a running session that started the DAY
	// BEFORE (no stop). It is outside today's range yet must show as running.
	start := time.Date(2026, 6, 20, 18, 51, 0, 0, time.Local)
	if _, err := srv.ss.Create(ctx, domain.WorkSession{ID: "overnight", OwnerID: "u1", Start: start}); err != nil {
		t.Fatalf("seed overnight: %v", err)
	}

	cookieVal, _ := srv.codec.Issue("u1")
	req, _ := http.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"data-timer", "/ui/worktime/stop"} {
		if !strings.Contains(body, want) {
			t.Errorf("overnight running session not surfaced on Heute: missing %q", want)
		}
	}
	if strings.Contains(body, "/ui/worktime/start") {
		t.Errorf("start card shown while a timer is running (should show the running hero)")
	}
}

func TestHeuteHome_EmptyShowsStart(t *testing.T) {
	srv := newWorktimeTestServer(t)
	cookieVal, _ := srv.codec.Issue("u1")
	req, _ := http.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "/ui/worktime/start") {
		t.Errorf("idle Heute missing start form")
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

// TestWebStop_WithProjectBooksAndStops covers the #6 happy path: stopping with a
// project books + stops the session and returns the idle Heute (start card).
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
	if !strings.Contains(rr.Body.String(), "/ui/worktime/start") {
		t.Errorf("after stop, idle Heute (start card) should render")
	}
}

// TestHeuteStopSelector_EngagementsOnly verifies that the booking picker in the
// stop form lists ONLY engagements — not repos, vorhaben, or other node kinds.
// D7: Slice-B ensures only engagement nodes are bookable worktime targets.
func TestHeuteStopSelector_EngagementsOnly(t *testing.T) {
	srv := newWorktimeTestServer(t)
	ctx := context.Background()

	// Seed one engagement and one repo node.
	eng, err := (usecase.CreateNode{Nodes: srv.ps, IDs: srv.ids, Clock: srv.clk}).Execute(
		ctx, "u1", usecase.CreateNodeInput{Name: "MyEngagement", Kind: domain.KindEngagement},
	)
	if err != nil {
		t.Fatalf("seed engagement: %v", err)
	}
	_, err = (usecase.CreateNode{Nodes: srv.ps, IDs: srv.ids, Clock: srv.clk}).Execute(
		ctx, "u1", usecase.CreateNodeInput{Name: "MyRepo", Kind: domain.KindRepo, ParentID: &eng.ID},
	)
	if err != nil {
		t.Fatalf("seed repo: %v", err)
	}

	// Start a running session.
	start := time.Date(2026, 6, 21, 10, 0, 0, 0, time.Local)
	if _, err := srv.ss.Create(ctx, domain.WorkSession{ID: "run2", OwnerID: "u1", Start: start}); err != nil {
		t.Fatalf("seed running session: %v", err)
	}

	cookieVal, _ := srv.codec.Issue("u1")
	req, _ := http.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	// Engagement must appear in the booking selector.
	if !strings.Contains(body, "MyEngagement") {
		t.Errorf("booking selector missing engagement %q", "MyEngagement")
	}
	// Repo must NOT appear in the booking selector.
	if strings.Contains(body, "MyRepo") {
		t.Errorf("booking selector must NOT list repo node %q", "MyRepo")
	}
}
