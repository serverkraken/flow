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
	// The combination form-action + wrong target must NOT appear.
	if strings.Contains(body, `hx-post="/nodes/n1/sessions" hx-target="#cockpit-panel"`) {
		t.Errorf("Nachbuchen form MUST NOT target #cockpit-panel (nesting bug)")
	}
}
