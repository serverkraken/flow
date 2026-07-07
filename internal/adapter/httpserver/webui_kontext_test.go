package httpserver_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// TestWebKontextView_RendersMeterRowsAndActions verifies GET /kontext/{id}
// renders the budget meter, the rang-list rows, and the ↑/↓/pin actions with
// aria-label + title (Codex-Fund #5), and that the response carries the
// #kontext-fragment element the SSE hx-select relies on (Codex-Fund #3).
func TestWebKontextView_RendersMeterRowsAndActions(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})
	nodeID := "n1"
	_, err := c.ds.Create(context.Background(), domain.Document{
		ID: "mem-1", OwnerID: "u1", NodeID: &nodeID, Type: domain.DocMemory,
		Path: "mem-1", Title: "Tailwind v4 gotchas", Body: "some memory body",
		CreatedAt: c.clk.Now(), UpdatedAt: c.clk.Now(),
	})
	if err != nil {
		t.Fatalf("seed memory doc: %v", err)
	}

	rec := c.do(t, "GET", "/kontext/n1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="kontext-fragment"`,
		`class="meter`,
		"Tailwind v4 gotchas",
		"repo:flow",
		`aria-label="Höher"`,
		`aria-label="Tiefer"`,
		`aria-label="Anpinnen"`,
		"Kontext kuratieren",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("kontext view missing %q: %.1200s", want, body)
		}
	}
}

// TestWebKontextView_FullPageHasHTMLHull guards against a kopfloses-HTML
// regression (Soenne live-Befund): the full GET /kontext/{id} view must render
// inside the shared HTML hull (components.Base) — DOCTYPE + app.css link —
// not just the bare AppShell fragment, otherwise the browser gets unstyled
// text with no CSS.
func TestWebKontextView_FullPageHasHTMLHull(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	rec := c.do(t, "GET", "/kontext/n1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<!doctype html") {
		t.Errorf("kontext full page missing <!doctype html>: %.400s", body)
	}
	if !strings.Contains(body, "app.css") {
		t.Errorf("kontext full page missing app.css link: %.400s", body)
	}
}

// TestWebKontextView_EmptyState covers the no-docs node: the quiet empty-state
// line renders instead of an empty list or a crash.
func TestWebKontextView_EmptyState(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	rec := c.do(t, "GET", "/kontext/n1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Kein kuratierbarer Kontext für diesen Knoten.") {
		t.Errorf("expected empty-state line, got %.600s", rec.Body.String())
	}
}

// TestWebKontextView_OwnerScope404 is the owner-scope negative test: a node
// owned by another tenant must 404, not leak into u1's Kuratieren page.
func TestWebKontextView_OwnerScope404(t *testing.T) {
	c := newCockpitTestServer(t)
	u2, _ := domain.NewUser("u2", "sub-2", "other", "o@x.de", "O")
	_, _ = c.srv.Users.UpsertBySub(context.Background(), u2)
	c.seedNode(t, domain.Node{ID: "foreign", OwnerID: "u2", Name: "secret-repo", Kind: domain.KindRepo})

	rec := c.do(t, "GET", "/kontext/foreign", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404 for a foreign node", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret-repo") {
		t.Error("foreign node name leaked into 404 response")
	}
}

// TestWebKontextReorder_SwapsPriorityAndEmitsFragmentOnly is the reorder
// round-trip: "up" on the second-ranked doc swaps it to first, stamps dense
// descending priorities via ReorderContextDocs, emits document.updated, and
// returns ONLY the #kontext-fragment (never a full AppShell page — Codex-Fund
// #3, since the button's own hx-swap="outerHTML" targets #kontext-fragment).
func TestWebKontextReorder_SwapsPriorityAndEmitsFragmentOnly(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})
	nodeID := "n1"
	older := c.clk.Now()
	newer := older.Add(time.Hour)
	_, _ = c.ds.Create(context.Background(), domain.Document{
		ID: "mem-1", OwnerID: "u1", NodeID: &nodeID, Type: domain.DocMemory,
		Path: "mem-1", Title: "Older memory", Body: "b1", CreatedAt: older, UpdatedAt: older,
	})
	_, _ = c.ds.Create(context.Background(), domain.Document{
		ID: "mem-2", OwnerID: "u1", NodeID: &nodeID, Type: domain.DocMemory,
		Path: "mem-2", Title: "Newer memory", Body: "b2", CreatedAt: newer, UpdatedAt: newer,
	})
	// Default Compose order (pinned desc, priority desc, tierRank asc, updatedAt
	// desc): both same tier/pin/priority(0) → newer first: [mem-2, mem-1].

	ch, cancel := c.srv.Bus.Subscribe("u1")
	defer cancel()

	rec := c.do(t, "POST", "/kontext/n1/reorder", map[string]string{"doc": "mem-1", "dir": "up"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="kontext-fragment"`) {
		t.Errorf("reorder response must contain #kontext-fragment: %.600s", body)
	}
	if strings.Contains(body, "<html") || strings.Contains(body, "<!DOCTYPE") {
		t.Errorf("reorder response must be fragment-only, not a full page: %.600s", body)
	}

	d1, err := c.ds.Get(context.Background(), "u1", "mem-1")
	if err != nil {
		t.Fatalf("get mem-1: %v", err)
	}
	d2, err := c.ds.Get(context.Background(), "u1", "mem-2")
	if err != nil {
		t.Fatalf("get mem-2: %v", err)
	}
	if d1.Priority <= d2.Priority {
		t.Errorf("mem-1 (moved up) priority %d must be > mem-2 priority %d", d1.Priority, d2.Priority)
	}

	select {
	case ev := <-ch:
		if ev.Type != domain.EventDocumentUpdated {
			t.Errorf("want document.updated, got %q", ev.Type)
		}
	default:
		t.Error("want document.updated SSE event after reorder, got none")
	}
}

// TestWebKontextReorder_UnknownDocIsNoOp covers Codex-Fund #4: a doc id that
// is no longer part of the composed context (deleted concurrently, or never
// existed) must degrade to a clean re-render — no 500, no panic, no mutation.
func TestWebKontextReorder_UnknownDocIsNoOp(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})
	nodeID := "n1"
	_, _ = c.ds.Create(context.Background(), domain.Document{
		ID: "mem-1", OwnerID: "u1", NodeID: &nodeID, Type: domain.DocMemory,
		Path: "mem-1", Title: "Only memory", Body: "b1", CreatedAt: c.clk.Now(), UpdatedAt: c.clk.Now(),
	})

	rec := c.do(t, "POST", "/kontext/n1/reorder", map[string]string{"doc": "does-not-exist", "dir": "up"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s, want 200 no-op", rec.Code, rec.Body.String())
	}
	d1, err := c.ds.Get(context.Background(), "u1", "mem-1")
	if err != nil {
		t.Fatalf("get mem-1: %v", err)
	}
	if d1.Priority != 0 {
		t.Errorf("mem-1 priority = %d, want unchanged 0 (no-op)", d1.Priority)
	}
}

// TestWebKontextReorder_OwnerScope404 mirrors the view handler's owner-scope
// check for the mutating route.
func TestWebKontextReorder_OwnerScope404(t *testing.T) {
	c := newCockpitTestServer(t)
	u2, _ := domain.NewUser("u2", "sub-2", "other", "o@x.de", "O")
	_, _ = c.srv.Users.UpsertBySub(context.Background(), u2)
	c.seedNode(t, domain.Node{ID: "foreign", OwnerID: "u2", Name: "secret-repo", Kind: domain.KindRepo})

	rec := c.do(t, "POST", "/kontext/foreign/reorder", map[string]string{"doc": "x", "dir": "up"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404 for a foreign node", rec.Code)
	}
}

// TestWebKontextPin_TogglesAndEmits covers the pin round-trip: SetPinned
// flips the doc's Pinned flag, document.updated is emitted, and the fragment
// re-renders with the flipped Anpinnen/Angepinnt label.
func TestWebKontextPin_TogglesAndEmits(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})
	nodeID := "n1"
	_, _ = c.ds.Create(context.Background(), domain.Document{
		ID: "mem-1", OwnerID: "u1", NodeID: &nodeID, Type: domain.DocMemory,
		Path: "mem-1", Title: "Pin me", Body: "b1", CreatedAt: c.clk.Now(), UpdatedAt: c.clk.Now(),
	})

	ch, cancel := c.srv.Bus.Subscribe("u1")
	defer cancel()

	rec := c.do(t, "POST", "/kontext/n1/pin", map[string]string{"doc": "mem-1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Angepinnt") {
		t.Errorf("expected Angepinnt after pin, got %.600s", rec.Body.String())
	}
	d, err := c.ds.Get(context.Background(), "u1", "mem-1")
	if err != nil {
		t.Fatalf("get mem-1: %v", err)
	}
	if !d.Pinned {
		t.Error("mem-1.Pinned = false, want true after toggle")
	}
	select {
	case ev := <-ch:
		if ev.Type != domain.EventDocumentUpdated {
			t.Errorf("want document.updated, got %q", ev.Type)
		}
	default:
		t.Error("want document.updated SSE event after pin, got none")
	}

	rec2 := c.do(t, "POST", "/kontext/n1/pin", map[string]string{"doc": "mem-1"})
	if rec2.Code != http.StatusOK {
		t.Fatalf("second pin status %d", rec2.Code)
	}
	if strings.Contains(rec2.Body.String(), "Angepinnt") {
		t.Errorf("expected unpin (Anpinnen) on second toggle, got %.600s", rec2.Body.String())
	}
}

// TestWebKontextPin_UnknownDocIsNoOp covers the analogous no-op path for pin:
// a doc id that doesn't exist (or belongs to another owner) must not 500.
func TestWebKontextPin_UnknownDocIsNoOp(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo})

	rec := c.do(t, "POST", "/kontext/n1/pin", map[string]string{"doc": "does-not-exist"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 no-op", rec.Code)
	}
}
