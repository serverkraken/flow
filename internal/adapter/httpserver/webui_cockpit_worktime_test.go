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

// TestCockpitWorktime_ListsOwnSessions verifies that GET /nodes/{id}/main
// renders the Buchungen section filtered to the requested node (not other
// nodes' sessions), with the edit link targeting #cockpit-main.
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

	rec := c.do(t, "GET", "/nodes/n1/main", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()

	if !strings.Contains(body, "slice6") {
		t.Errorf("Buchungen section missing session tag 'slice6': %.400s", body)
	}
	// The completed row's edit link must target #cockpit-main (Task 7 — the
	// old worktime tab's Nachbuchen mini-form is gone; booking now goes through
	// the page-level SessionDialog, but the row's Edit round-trip still lands
	// back in #cockpit-main).
	if !strings.Contains(body, `hx-target="#cockpit-main"`) {
		t.Errorf(`Buchung row edit link must use hx-target="#cockpit-main": %.600s`, body)
	}
	if strings.Contains(body, "n2only") {
		t.Errorf("Buchungen section must NOT show other node's sessions: %.400s", body)
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

// TestCockpitWorktime_NachbuchenFormTargetsCockpitMain verifies the shared
// Nachbuchen SessionDialog (mounted once on the full page, Task 7 — no more
// per-tab mini-form) posts to /nodes/{id}/sessions targeting #cockpit-main,
// and that the now-meaningless #cockpit-panel target never appears anywhere.
func TestCockpitWorktime_NachbuchenFormTargetsCockpitMain(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	rec := c.do(t, "GET", "/nodes/n1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()

	if !strings.Contains(body, `hx-post="/nodes/n1/sessions"`) {
		t.Errorf("Nachbuchen dialog action missing: %.600s", body)
	}
	if strings.Contains(body, `hx-target="#cockpit-panel"`) {
		t.Errorf("nothing may target #cockpit-panel (Kristall-era id, gone since Task 7): %.600s", body)
	}
}

// TestCockpitHead_FragmentReturnsOK verifies that GET /nodes/{id}/head returns
// the spine fragment (used by the SSE live-reload hx-get) — the outer
// id="cockpit-head" div lives in cockpitBody and is NOT part of this fragment.
func TestCockpitHead_FragmentReturnsOK(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	rec := c.do(t, "GET", "/nodes/n1/head", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.300s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "flow") {
		t.Errorf("head fragment missing node name: %.300s", body)
	}
	// CockpitHead's spine wrapper class.
	if !strings.Contains(body, `class="spine"`) {
		t.Errorf("head fragment missing the spine wrapper: %.300s", body)
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

	rec := c.do(t, "GET", "/nodes/n1/main", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// The i18n key cockpit.worktime.running → "running" (EN) / "läuft" (DE).
	// We check for the row date being present as a proxy for the row rendering.
	if !strings.Contains(body, "10:00") {
		t.Errorf("running session start time missing from Buchungen section: %.400s", body)
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
// different node, the cockpit's instr-band shows the "running on Y" switch
// widget. This covers the TimerOtherBound branch in cockpitInstr (the
// timer forms live in #cockpit-main now, Task 7 — not the spine).
func TestCockpitTimer_OtherBound(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo, Slug: "flow"})
	c.seedNode(t, domain.Node{ID: "n2", OwnerID: "u1", Name: "other", Kind: domain.KindRepo, Slug: "other"})

	// Start a session on n2.
	n2 := "n2"
	_, _ = c.srv.StartSession.Execute(context.Background(), "u1", &n2, nil, "")

	// Viewing n1: timer should be OtherBound → shows switch form.
	rec := c.do(t, "GET", "/nodes/n1/main", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
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

	rec := c.do(t, "GET", "/nodes/n1/main", nil)
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

	rec := c.do(t, "GET", "/nodes/n1/main", nil)
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

// ── Task 6: containment + session edit/delete ──────────────────────────────

// TestCockpitWorktime_EngagementListsSubtreeSessionWithNodePill verifies the
// containment rule (Spec §4): an Engagement's worktime tab lists a session
// booked on a GRANDCHILD repo (Engagement → Vorhaben → Repo), carrying that
// repo's node-pill (kind glyph + name), and lists the Engagement's own direct
// booking WITHOUT the "own bookings only" footer note.
func TestCockpitWorktime_EngagementListsSubtreeSessionWithNodePill(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "e1", OwnerID: "u1", Name: "Acme", Kind: domain.KindEngagement})
	c.seedNode(t, domain.Node{ID: "v1", OwnerID: "u1", Name: "Redesign", Kind: domain.KindVorhaben, ParentID: strPtr("e1")})
	c.seedNode(t, domain.Node{ID: "r1", OwnerID: "u1", Name: "flow-api", Kind: domain.KindRepo, ParentID: strPtr("v1")})

	r1 := "r1"
	start := time.Date(2026, 6, 29, 9, 0, 0, 0, time.Local)
	stop := start.Add(time.Hour)
	if _, err := c.ss.Create(context.Background(), domain.WorkSession{
		ID: "sess-r1", OwnerID: "u1", NodeID: &r1, Start: start, Stop: &stop,
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	rec := c.do(t, "GET", "/nodes/e1/main", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "flow-api") {
		t.Errorf("Engagement worktime tab missing grandchild repo session: %.600s", body)
	}
	if !strings.Contains(body, `href="/nodes/r1"`) {
		t.Errorf("Engagement worktime tab missing node-pill link to /nodes/r1: %.600s", body)
	}
	// The subtree footer note replaces the own-only note once containment applies.
	if strings.Contains(body, "eigene Buchungen dieses Knotens") {
		t.Errorf("Engagement worktime tab must NOT show the Repo-only footer note: %.600s", body)
	}
}

// TestCockpitWorktime_RepoUnderTreeListsOwnOnly verifies the other half of the
// containment rule: a Repo's worktime tab, even nested under an Engagement
// with its OWN sessions on ancestor nodes, lists ONLY its own bookings — no
// upward containment for a Repo (Spec §4).
func TestCockpitWorktime_RepoUnderTreeListsOwnOnly(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "e1", OwnerID: "u1", Name: "Acme", Kind: domain.KindEngagement})
	c.seedNode(t, domain.Node{ID: "r1", OwnerID: "u1", Name: "flow-api", Kind: domain.KindRepo, ParentID: strPtr("e1")})

	e1, r1 := "e1", "r1"
	start := time.Date(2026, 6, 29, 9, 0, 0, 0, time.Local)
	stop := start.Add(time.Hour)
	if _, err := c.ss.Create(context.Background(), domain.WorkSession{
		ID: "sess-e1", OwnerID: "u1", NodeID: &e1, Tags: []string{"engagement-own"}, Start: start, Stop: &stop,
	}); err != nil {
		t.Fatalf("seed engagement session: %v", err)
	}
	start2 := start.Add(2 * time.Hour)
	stop2 := start2.Add(time.Hour)
	if _, err := c.ss.Create(context.Background(), domain.WorkSession{
		ID: "sess-r1", OwnerID: "u1", NodeID: &r1, Tags: []string{"repo-own"}, Start: start2, Stop: &stop2,
	}); err != nil {
		t.Fatalf("seed repo session: %v", err)
	}

	rec := c.do(t, "GET", "/nodes/r1/main", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "repo-own") {
		t.Errorf("Repo worktime tab missing its own session: %.400s", body)
	}
	if strings.Contains(body, "engagement-own") {
		t.Errorf("Repo worktime tab must NOT show the ancestor Engagement's session: %.400s", body)
	}
	if !strings.Contains(body, "eigene Buchungen dieses Knotens") {
		t.Errorf("Repo worktime tab must show the own-only footer note: %.400s", body)
	}
}

// TestCockpitEditSession_ChangesTimes verifies that POST
// /nodes/{id}/sessions/{sid}/edit updates the session's stored times (and
// note/tags) and re-renders the panel (canonical #cockpit-main target).
func TestCockpitEditSession_ChangesTimes(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	n1 := "n1"
	start := time.Date(2026, 6, 28, 9, 0, 0, 0, time.Local)
	stop := start.Add(time.Hour)
	if _, err := c.ss.Create(context.Background(), domain.WorkSession{
		ID: "s1", OwnerID: "u1", NodeID: &n1, Start: start, Stop: &stop,
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	rec := c.do(t, "POST", "/nodes/n1/sessions/s1/edit", map[string]string{
		"date": "2026-06-28", "from": "10:00", "to": "12:30", "tag": "edited", "note": "moved", "node": "n1",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `hx-target="#cockpit-main"`) {
		t.Errorf("edit response must target #cockpit-main: %.400s", rec.Body.String())
	}

	got, err := c.ss.Get(context.Background(), "u1", "s1")
	if err != nil {
		t.Fatalf("Get after edit: %v", err)
	}
	if got.Start.Format("15:04") != "10:00" || got.Stop == nil || got.Stop.Format("15:04") != "12:30" {
		t.Fatalf("edit did not update times, got start=%v stop=%v", got.Start, got.Stop)
	}
	if got.Note != "moved" {
		t.Errorf("edit did not update note, got %q", got.Note)
	}
}

// TestCockpitEditSession_PreservesNodeAcrossContainmentView is the correctness
// regression for the hidden-node-field design: editing a descendant Repo's
// session from its ANCESTOR Engagement's containment view (path {id}=e1, the
// viewed node) must NOT reassign the session up to e1 — the form's "node"
// field (hidden in the real dialog, set explicitly here to simulate it)
// carries the session's OWN node (r1), which the handler must honor over the
// path {id}.
func TestCockpitEditSession_PreservesNodeAcrossContainmentView(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "e1", OwnerID: "u1", Name: "Acme", Kind: domain.KindEngagement})
	c.seedNode(t, domain.Node{ID: "r1", OwnerID: "u1", Name: "flow-api", Kind: domain.KindRepo, ParentID: strPtr("e1")})

	r1 := "r1"
	start := time.Date(2026, 6, 28, 9, 0, 0, 0, time.Local)
	stop := start.Add(time.Hour)
	if _, err := c.ss.Create(context.Background(), domain.WorkSession{
		ID: "s1", OwnerID: "u1", NodeID: &r1, Start: start, Stop: &stop,
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	// Edited via the ENGAGEMENT's cockpit ({id}=e1), not the Repo's.
	rec := c.do(t, "POST", "/nodes/e1/sessions/s1/edit", map[string]string{
		"date": "2026-06-28", "from": "10:00", "to": "11:00", "node": "r1",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}

	got, err := c.ss.Get(context.Background(), "u1", "s1")
	if err != nil {
		t.Fatalf("Get after edit: %v", err)
	}
	if got.NodeID == nil || *got.NodeID != "r1" {
		t.Fatalf("edit from Engagement containment view must preserve the session's own node r1, got %v", got.NodeID)
	}
}

// TestCockpitDeleteSession_RemovesAndEmitsEvent verifies that POST
// /nodes/{id}/sessions/{sid}/delete removes the session and emits
// session.deleted with the session's id (SSE live-sync contract).
func TestCockpitDeleteSession_RemovesAndEmitsEvent(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	n1 := "n1"
	start := time.Date(2026, 6, 28, 9, 0, 0, 0, time.Local)
	stop := start.Add(time.Hour)
	if _, err := c.ss.Create(context.Background(), domain.WorkSession{
		ID: "s1", OwnerID: "u1", NodeID: &n1, Start: start, Stop: &stop,
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	ch, cancel := c.srv.Bus.Subscribe("u1")
	defer cancel()

	rec := c.do(t, "POST", "/nodes/n1/sessions/s1/delete", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}

	if _, err := c.ss.Get(context.Background(), "u1", "s1"); err == nil {
		t.Fatalf("session s1 must be gone after delete")
	}

	select {
	case ev := <-ch:
		if ev.Type != domain.EventSessionDeleted {
			t.Fatalf("event type=%q, want session.deleted", ev.Type)
		}
		if ev.Data["id"] != "s1" {
			t.Errorf("session.deleted event Data[id]=%v, want s1", ev.Data["id"])
		}
	case <-time.After(time.Second):
		t.Fatal("no session.deleted event received")
	}
}

// TestCockpitEditSession_ForeignSessionReturnsPanelErr verifies the
// multi-tenant guard: editing a session owned by a DIFFERENT owner returns 200
// with an inline PanelErr (not a 500, not a silent success) and leaves the
// foreign session untouched — EditSession's owner-scoped Get/Update already
// enforce this; this test pins the handler's error path around it.
func TestCockpitEditSession_ForeignSessionReturnsPanelErr(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	other := "n1"
	start := time.Date(2026, 6, 28, 9, 0, 0, 0, time.Local)
	stop := start.Add(time.Hour)
	if _, err := c.ss.Create(context.Background(), domain.WorkSession{
		ID: "foreign", OwnerID: "u2", NodeID: &other, Start: start, Stop: &stop,
	}); err != nil {
		t.Fatalf("seed foreign session: %v", err)
	}

	rec := c.do(t, "POST", "/nodes/n1/sessions/foreign/edit", map[string]string{
		"date": "2026-06-28", "from": "10:00", "to": "11:00", "node": "n1",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d (must degrade to 200+PanelErr, not %d): body=%.400s", rec.Code, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "konnte nicht bearbeiten") {
		t.Errorf("expected inline PanelErr for foreign session edit, got: %.400s", rec.Body.String())
	}

	got, err := c.ss.Get(context.Background(), "u2", "foreign")
	if err != nil {
		t.Fatalf("foreign session must still exist: %v", err)
	}
	if got.Start.Format("15:04") != "09:00" {
		t.Errorf("foreign session must be unchanged, got start=%v", got.Start)
	}
}

// TestCockpitDeleteSession_ForeignSessionReturnsPanelErr mirrors the edit
// guard above for delete.
func TestCockpitDeleteSession_ForeignSessionReturnsPanelErr(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	other := "n1"
	start := time.Date(2026, 6, 28, 9, 0, 0, 0, time.Local)
	stop := start.Add(time.Hour)
	if _, err := c.ss.Create(context.Background(), domain.WorkSession{
		ID: "foreign", OwnerID: "u2", NodeID: &other, Start: start, Stop: &stop,
	}); err != nil {
		t.Fatalf("seed foreign session: %v", err)
	}

	rec := c.do(t, "POST", "/nodes/n1/sessions/foreign/delete", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d (must degrade to 200+PanelErr): body=%.400s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "konnte nicht löschen") {
		t.Errorf("expected inline PanelErr for foreign session delete, got: %.400s", rec.Body.String())
	}
	if _, err := c.ss.Get(context.Background(), "u2", "foreign"); err != nil {
		t.Fatalf("foreign session must still exist after failed delete: %v", err)
	}
}

// TestCockpitWorktime_EditQueryOpensPrefilledDialog verifies the ?edit={sid}
// round-trip end-to-end: GET /nodes/{id}/main?edit={sid} renders the
// shared SessionDialog pre-opened (native <dialog open>, no click needed) and
// prefilled from the session, posting back to this same node's edit route.
func TestCockpitWorktime_EditQueryOpensPrefilledDialog(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	n1 := "n1"
	start := time.Date(2026, 6, 28, 9, 0, 0, 0, time.Local)
	stop := start.Add(time.Hour)
	if _, err := c.ss.Create(context.Background(), domain.WorkSession{
		ID: "s1", OwnerID: "u1", NodeID: &n1, Note: "impl", Start: start, Stop: &stop,
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	rec := c.do(t, "GET", "/nodes/n1/main?edit=s1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="session-dialog-edit"`) {
		t.Errorf("missing edit dialog: %.800s", body)
	}
	if !strings.Contains(body, `hx-post="/nodes/n1/sessions/s1/edit"`) {
		t.Errorf("edit dialog form must post to /nodes/n1/sessions/s1/edit: %.800s", body)
	}
	if !strings.Contains(body, "impl") {
		t.Errorf("edit dialog must prefill note 'impl': %.800s", body)
	}
}

// strPtr returns a pointer to s (test helper for ParentID fields).
func strPtr(s string) *string { return &s }
