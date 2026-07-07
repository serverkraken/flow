package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// TestWebDocumentView_ContextBlockShowsModeSwitcherForMemoryDoc is L5.5 Task
// 4's Step 5 coverage: a memory doc in auto mode shows the Auto/Immer/Nie
// switcher in the docrail's "Im Agenten-Kontext" block, with "Auto" as the
// pressed segment.
func TestWebDocumentView_ContextBlockShowsModeSwitcherForMemoryDoc(t *testing.T) {
	srv, codec, docs, projects := newWebWissenServer(t)
	srv.ComposeContext = usecase.ComposeContext{Nodes: projects, Docs: docs, Tags: testutil.NewFakeTagStore()}
	srv.ContextBudget = 12000
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	if _, err := projects.Create(ctx, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	nodeID := "n1"
	if _, err := docs.Create(ctx, domain.Document{
		ID: "mem-1", OwnerID: "u1", NodeID: &nodeID, Type: domain.DocMemory,
		Path: "mem-1", Title: "Tailwind v4 gotchas", Body: "some memory body",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed memory doc: %v", err)
	}

	body, status := getWissenDocumentAs(t, srv, codec, "u1", "/wissen/mem-1")
	if status != http.StatusOK {
		t.Fatalf("GET /wissen/mem-1 status=%d body=%.400s", status, body)
	}
	for _, want := range []string{
		`hx-post="/wissen/mem-1/mode"`,
		`hx-target="#document-fragment"`,
		"auto", "immer", "nie",
		`aria-pressed="true"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("GET /wissen/mem-1 missing mode switcher %q in %.1200s", want, body)
		}
	}
}

// TestWebDocumentView_NieModeShowsHiddenLabel covers the "nie" display
// branch: a doc explicitly set to nie is never composed (StandingOf reports
// "absent"), but the docrail must still show "ausgeblendet (nie)" plus the
// switcher (now pressed on Nie) so the doc stays wiederherstellbar in place.
func TestWebDocumentView_NieModeShowsHiddenLabel(t *testing.T) {
	srv, codec, docs, projects := newWebWissenServer(t)
	srv.ComposeContext = usecase.ComposeContext{Nodes: projects, Docs: docs, Tags: testutil.NewFakeTagStore()}
	srv.ContextBudget = 12000
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	if _, err := projects.Create(ctx, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	nodeID := "n1"
	if _, err := docs.Create(ctx, domain.Document{
		ID: "mem-1", OwnerID: "u1", NodeID: &nodeID, Type: domain.DocMemory,
		Path: "mem-1", Title: "Hidden memory", Body: "b", ContextMode: domain.ContextModeNie,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed memory doc: %v", err)
	}

	body, status := getWissenDocumentAs(t, srv, codec, "u1", "/wissen/mem-1")
	if status != http.StatusOK {
		t.Fatalf("GET /wissen/mem-1 status=%d body=%.400s", status, body)
	}
	if !strings.Contains(body, "ausgeblendet (nie)") {
		t.Errorf("GET /wissen/mem-1 missing hidden label, got %.1200s", body)
	}
}

// TestWebDocMode_TogglesAndEmitsFragmentOnly is Codex-Fund #3's own mutation
// test for the doc-page round-trip (POST /wissen/{id}/mode): the mode
// toggles in the store, exactly one document.updated fires, and the response
// is ONLY the #document-fragment (never the full AppShell page).
func TestWebDocMode_TogglesAndEmitsFragmentOnly(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{
		ID: "d1", OwnerID: "u1", Type: domain.DocMemory, Path: "p/x",
		Title: "X", Body: "b", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	ch, cancel := srv.Bus.Subscribe("u1")
	defer cancel()

	body, status := postWissenMode(t, srv, codec, "u1", "d1", "immer")
	if status != http.StatusOK {
		t.Fatalf("POST /wissen/d1/mode status=%d body=%.400s", status, body)
	}
	if !strings.Contains(body, `id="document-fragment"`) {
		t.Errorf("mode response must contain #document-fragment: %.600s", body)
	}
	if strings.Contains(body, "<html") || strings.Contains(body, "<!DOCTYPE") {
		t.Errorf("mode response must be fragment-only, not a full page: %.600s", body)
	}
	d, err := docs.Get(ctx, "u1", "d1")
	if err != nil {
		t.Fatalf("get d1: %v", err)
	}
	if d.ContextMode != domain.ContextModeImmer {
		t.Errorf("d1.ContextMode = %q, want immer", d.ContextMode)
	}

	// Emitter.Emit always follows the primary event with an EventActivityLogged
	// (sse/emitter.go) — the mutation test asserts exactly ONE document.updated
	// (Codex-Fund #3), not zero total SSE traffic.
	updates := 0
	drain := true
	for drain {
		select {
		case ev := <-ch:
			if ev.Type == domain.EventDocumentUpdated {
				updates++
			}
		default:
			drain = false
		}
	}
	if updates != 1 {
		t.Errorf("want exactly 1 document.updated event, got %d", updates)
	}
}

// TestWebDocMode_OwnerScopedNoOp mirrors TestWebDocPin_OwnerScoped: a second
// tenant must not be able to mutate (or even discover) another owner's
// document via the mode round-trip.
func TestWebDocMode_OwnerScopedNoOp(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{
		ID: "secret", OwnerID: "u1", Type: domain.DocMemory, Path: "p/secret",
		Title: "Secret", Body: "shh", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	_, status := postWissenMode(t, srv, codec, "u2", "secret", "immer")
	if status != http.StatusNotFound {
		t.Fatalf("u2 POST /wissen/secret/mode status=%d, want 404 (owner-scoped)", status)
	}
	d, err := docs.Get(ctx, "u1", "secret")
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}
	if d.ContextMode == domain.ContextModeImmer {
		t.Error("foreign doc must not be mutated by another owner's mode POST")
	}
}

// TestWebDocMode_InvalidModeIsCleanNoOp covers the belt-and-suspenders check:
// an unknown mode string must degrade to a clean re-render (200, fragment),
// never a 500.
func TestWebDocMode_InvalidModeIsCleanNoOp(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{
		ID: "d1", OwnerID: "u1", Type: domain.DocMemory, Path: "p/x",
		Title: "X", Body: "b", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	body, status := postWissenMode(t, srv, codec, "u1", "d1", "bogus")
	if status != http.StatusOK {
		t.Fatalf("POST /wissen/d1/mode status=%d body=%.400s, want 200 no-op", status, body)
	}
	if !strings.Contains(body, `id="document-fragment"`) {
		t.Errorf("invalid-mode response must still be the fragment: %.600s", body)
	}
	d, err := docs.Get(ctx, "u1", "d1")
	if err != nil {
		t.Fatalf("get d1: %v", err)
	}
	// The fake store coalesces ContextMode to "auto" at Create (mirrors the
	// pgstore column default) — the no-op must leave that default untouched.
	if d.ContextMode != domain.ContextModeAuto {
		t.Errorf("d1.ContextMode = %q, want unchanged auto (no-op)", d.ContextMode)
	}
}

func postWissenMode(t *testing.T, s *Server, codec SessionCodec, userID, id, mode string) (string, int) {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("POST /wissen/{id}/mode", s.webAuth(http.HandlerFunc(s.handleWebDocMode)))
	cookieVal, err := codec.Issue(userID)
	if err != nil {
		t.Fatal(err)
	}
	form := strings.NewReader("mode=" + mode)
	req := httptest.NewRequest(http.MethodPost, "/wissen/"+id+"/mode", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookieVal})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Body.String(), rec.Code
}
