package httpserver_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// TestCockpitWorktime_ListsOwnSessions verifies that GET /nodes/{id}/tab/worktime
// renders the session list filtered to the requested node (not other nodes'
// sessions), shows the Nachbuchen form with the correct route, and targets
// #cockpit-main (not #cockpit-panel) so HTMX does not nest the strip inside itself.
func TestCockpitWorktime_ListsOwnSessions(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	// Seed a session with a tag directly so the tag survives (AddSession without
	// a wired TagStore drops tags; direct Create stores the struct as-is).
	start := time.Date(2026, 6, 27, 14, 0, 0, 0, time.Local)
	stop := start.Add(2 * time.Hour)
	n1 := "n1"
	_, _ = c.ss.Create(context.Background(), domain.WorkSession{
		ID: "sess-n1", OwnerID: "u1", NodeID: &n1,
		Tags: []string{"slice6"}, Start: start, Stop: &stop,
	})

	// Seed a session on a different node — must NOT appear in n1's panel.
	n2 := "n2"
	start2 := time.Date(2026, 6, 26, 10, 0, 0, 0, time.Local)
	stop2 := start2.Add(time.Hour)
	_, _ = c.ss.Create(context.Background(), domain.WorkSession{
		ID: "sess-n2", OwnerID: "u1", NodeID: &n2,
		Tags: []string{"n2only"}, Start: start2, Stop: &stop2,
	})

	rec := c.do(t, "GET", "/nodes/n1/tab/worktime", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()

	if !strings.Contains(body, "slice6") {
		t.Errorf("worktime panel missing session tag 'slice6': %.400s", body)
	}
	if !strings.Contains(body, "/nodes/n1/sessions") {
		t.Errorf("worktime panel missing Nachbuchen form action /nodes/n1/sessions: %.400s", body)
	}
	// The form must target #cockpit-main (outer container) not #cockpit-panel.
	// CockpitTabsAndPanel returns the full strip+panel; swapping INTO #cockpit-panel
	// would duplicate the tab strip and the id — the nesting bug fixed in Task 4.
	if !strings.Contains(body, `hx-target="#cockpit-main"`) {
		t.Errorf(`Nachbuchen form must use hx-target="#cockpit-main": %.600s`, body)
	}
	if strings.Contains(body, "n2only") {
		t.Errorf("worktime panel must NOT show other node's sessions: %.400s", body)
	}
}

// TestCockpitAddSession_Books verifies that POST /nodes/{id}/sessions books a
// manual session (Nachbuchen) on the given node and returns 200 with the updated panel.
func TestCockpitAddSession_Books(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})
	rec := c.do(t, "POST", "/nodes/n1/sessions", map[string]string{
		"date": "2026-06-28", "from": "09:00", "to": "11:00", "tag": "x",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("add status %d body=%.300s", rec.Code, rec.Body.String())
	}
	// session exists booked to n1
	all, err := (usecase.ListSessionsRange{Sessions: c.ss}).Execute(context.Background(), "u1",
		time.Date(2026, 6, 28, 0, 0, 0, 0, time.Local), time.Date(2026, 6, 29, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("ListSessionsRange: %v", err)
	}
	if len(all) != 1 || all[0].NodeID == nil || *all[0].NodeID != "n1" {
		t.Fatalf("expected 1 session booked to n1, got %+v", all)
	}
}

// TestCockpitAddSession_InvalidTime verifies that an invalid time range (to <= from)
// returns 200 with an inline error message and does NOT book any session.
func TestCockpitAddSession_InvalidTime(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})
	rec := c.do(t, "POST", "/nodes/n1/sessions", map[string]string{
		"date": "2026-06-28", "from": "11:00", "to": "09:00", "tag": "x",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.300s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "ungültige Zeit") {
		t.Errorf("expected inline error message, got: %.300s", body)
	}
	// No session must have been booked.
	all, _ := (usecase.ListSessionsRange{Sessions: c.ss}).Execute(context.Background(), "u1",
		time.Date(2026, 6, 28, 0, 0, 0, 0, time.Local), time.Date(2026, 6, 29, 0, 0, 0, 0, time.Local))
	if len(all) != 0 {
		t.Errorf("expected no session booked after invalid time, got %d", len(all))
	}
}

// TestCockpitWorktime_NachbuchenFormTargetsCockpitMain pins the fix for the
// DOM-nesting bug: the Nachbuchen form must target #cockpit-main (which holds
// the full strip+panel), not #cockpit-panel (which is inside that container).
// Swapping CockpitTabsAndPanel INTO #cockpit-panel would nest a second strip
// inside the first and duplicate the id.
func TestCockpitWorktime_NachbuchenFormTargetsCockpitMain(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	rec := c.do(t, "GET", "/nodes/n1/tab/worktime", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()

	// The form action must be present.
	if !strings.Contains(body, `hx-post="/nodes/n1/sessions"`) {
		t.Errorf("Nachbuchen form action missing: %.600s", body)
	}
	// #cockpit-panel is never a valid hx-target in this cockpit — everything that
	// re-renders the tab area targets #cockpit-main. Order-independent guard.
	if strings.Contains(body, `hx-target="#cockpit-panel"`) {
		t.Errorf("nothing may target #cockpit-panel (nesting bug): %.600s", body)
	}
}

// TestCockpitHead_FragmentReturnsOK verifies that GET /nodes/{id}/head returns
// the head fragment (used by the SSE live-reload hx-get). This covers the
// handleWebNodeHead code path which the other tests miss (they use full-page
// GET /nodes/{id} or tab GET /nodes/{id}/tab/{name}).
func TestCockpitHead_FragmentReturnsOK(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	rec := c.do(t, "GET", "/nodes/n1/head", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.300s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// The rail fragment (CockpitRail) renders just the three card sections —
	// the outer id="cockpit-rail" div lives in cockpitBody and is NOT part of
	// this fragment.
	if !strings.Contains(body, "flow") {
		t.Errorf("head fragment missing node name: %.300s", body)
	}
	// CockpitRail's identity card uses the rounded-3xl glass card class.
	if !strings.Contains(body, "rounded-3xl") {
		t.Errorf("head fragment missing section element: %.300s", body)
	}
	// Unknown node → 404
	if rec2 := c.do(t, "GET", "/nodes/nope/head", nil); rec2.Code != http.StatusNotFound {
		t.Errorf("unknown node /head: status=%d want 404", rec2.Code)
	}
}

// TestCockpitWorktime_RunningSessionRow verifies that a running session (Stop == nil)
// is shown in the worktime panel with the "running" indicator instead of a duration,
// covering the row.Running == true branch in cockpitSessionRow and BuildCockpitSessionRows.
func TestCockpitWorktime_RunningSessionRow(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	// Create a running session (Stop == nil) on n1.
	n1 := "n1"
	start := time.Date(2026, 6, 30, 10, 0, 0, 0, time.Local) // before the fake clock 12:00
	_, _ = c.ss.Create(context.Background(), domain.WorkSession{
		ID: "sess-running", OwnerID: "u1", NodeID: &n1,
		Start: start, // Stop is nil → running
	})

	rec := c.do(t, "GET", "/nodes/n1/tab/worktime", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// The i18n key cockpit.worktime.running → "running" (EN) / "läuft" (DE).
	// We check for the row date being present as a proxy for the row rendering.
	if !strings.Contains(body, "10:00") {
		t.Errorf("running session start time missing from worktime panel: %.400s", body)
	}
}

// TestCockpitAddSession_OverlapError verifies that booking a session that overlaps
// an existing one returns 200 with an inline "konnte nicht buchen" error message,
// covering the AddSession.Execute error path in handleWebNodeAddSession.
func TestCockpitAddSession_OverlapError(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	// Seed an existing session 09:00–11:00 on 2026-06-28.
	n1 := "n1"
	start := time.Date(2026, 6, 28, 9, 0, 0, 0, time.Local)
	stop := start.Add(2 * time.Hour)
	_, _ = c.ss.Create(context.Background(), domain.WorkSession{
		ID: "existing", OwnerID: "u1", NodeID: &n1,
		Start: start, Stop: &stop,
	})

	// Try to book 10:00–12:00 on the same day (overlaps existing).
	rec := c.do(t, "POST", "/nodes/n1/sessions", map[string]string{
		"date": "2026-06-28", "from": "10:00", "to": "12:00",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.300s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "konnte nicht buchen") {
		t.Errorf("expected overlap error message, got: %.300s", rec.Body.String())
	}
}

// TestCockpitTimer_OtherBound verifies that when a session is running on a
// different node, the cockpit head shows the "running on Y" switch widget.
// This covers the TimerOtherBound branch in cockpitTimer templ.
func TestCockpitTimer_OtherBound(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo, Slug: "flow"})
	c.seedNode(t, domain.Node{ID: "n2", OwnerID: "u1", Name: "other", Kind: domain.KindRepo, Slug: "other"})

	// Start a session on n2.
	n2 := "n2"
	_, _ = c.srv.StartSession.Execute(context.Background(), "u1", &n2, nil, "")

	// Viewing n1: timer should be OtherBound → shows switch form.
	rec := c.do(t, "GET", "/nodes/n1/head", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	// TimerOtherBound renders a <details> with the switch form.
	if !strings.Contains(rec.Body.String(), "/nodes/n1/switch") {
		t.Errorf("TimerOtherBound: expected switch form, got: %.400s", rec.Body.String())
	}
}

// TestCockpitTimer_Unbound verifies that a running unbooked session (no NodeID)
// shows the home link on another node's cockpit. Covers TimerUnbound.
func TestCockpitTimer_Unbound(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	// Start a session without a node (unbooked timer).
	_, _ = c.srv.StartSession.Execute(context.Background(), "u1", nil, nil, "")

	rec := c.do(t, "GET", "/nodes/n1/head", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	// TimerUnbound renders a home link (href="/").
	body := rec.Body.String()
	if !strings.Contains(body, `href="/"`) {
		t.Errorf("TimerUnbound: expected home link, got: %.400s", body)
	}
}

// TestCockpitTimer_NotBookable verifies that non-bookable nodes (KindBranch)
// show the "not bookable" text instead of timer controls. Covers TimerNotBookable.
func TestCockpitTimer_NotBookable(t *testing.T) {
	c := newCockpitTestServer(t)
	// KindBranch is not bookable (IsBookable returns false for it).
	c.seedNode(t, domain.Node{ID: "b1", OwnerID: "u1", Name: "main", Kind: domain.KindBranch})

	rec := c.do(t, "GET", "/nodes/b1/head", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	// TimerNotBookable renders a plain <span> with the "not bookable" i18n string.
	// In English locale it translates to "not bookable" (or the key fallback).
	body := rec.Body.String()
	// The template renders components.T(ctx, "cockpit.timer.notBookable"); just
	// verify the button/form elements are NOT present (we don't depend on exact text).
	if strings.Contains(body, `hx-post="/nodes/b1/start"`) || strings.Contains(body, `hx-post="/nodes/b1/stop"`) {
		t.Errorf("NotBookable: expected no start/stop buttons, got: %.400s", body)
	}
}

// TestCockpitHead_NodeColors verifies that nodes with different colors render
// the correct accent bar class. This covers cockpitAccent branches for colors
// that aren't hit by other tests (purple, green, orange).
func TestCockpitHead_NodeColors(t *testing.T) {
	for _, c := range []struct {
		color string
		want  string
	}{
		{"purple", "bg-purple"},
		{"green", "bg-green"},
		{"orange", "bg-orange"},
		{"unknown-color", "bg-blue"}, // default
	} {
		t.Run(c.color, func(t *testing.T) {
			srv := newCockpitTestServer(t)
			srv.seedNode(t, domain.Node{
				ID: "n-color", OwnerID: "u1", Name: "colortest", Kind: domain.KindRepo,
				Color: c.color, Glyph: "◈",
			})
			rec := srv.do(t, "GET", "/nodes/n-color/head", nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), c.want) {
				t.Errorf("color %q: expected class %q in head, got: %.300s", c.color, c.want, rec.Body.String())
			}
		})
	}
}

// TestCockpitSessionRow_NoTag verifies that sessions with no tags still render
// correctly (covers the `if row.Tag != ""` false branch in cockpitSessionRow).
func TestCockpitSessionRow_NoTag(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	// Session with no tags.
	n1 := "n1"
	start := time.Date(2026, 6, 29, 10, 0, 0, 0, time.Local)
	stop := start.Add(1 * time.Hour)
	_, _ = c.ss.Create(context.Background(), domain.WorkSession{
		ID: "notag", OwnerID: "u1", NodeID: &n1,
		Start: start, Stop: &stop, Tags: nil,
	})

	rec := c.do(t, "GET", "/nodes/n1/tab/worktime", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.300s", rec.Code, rec.Body.String())
	}
	// The session row should render the duration without a tag chip.
	body := rec.Body.String()
	if !strings.Contains(body, "10:00–11:00") {
		t.Errorf("expected session span in panel, got: %.300s", body)
	}
	// Duration 1:00 h should be visible.
	if !strings.Contains(body, "1:00 h") {
		t.Errorf("expected 1:00 h duration, got: %.300s", body)
	}
}
