package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestWebWissenDocumentView(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	_, _ = docs.Create(ctx, domain.Document{
		ID:      "target",
		OwnerID: "u1",
		Type:    domain.DocFree,
		Path:    "target/path",
		Title:   "Target Doc",
		Body: strings.Join([]string{
			"> [!NOTE]",
			"> important",
			"",
			"| A | B |",
			"|---|---|",
			"| 1 | 2 |",
			"",
			"```go",
			"package main",
			"```",
			"",
			"See [[source/path]].",
		}, "\n"),
		Tags:      []string{"alpha"},
		CreatedAt: now,
		UpdatedAt: now,
	})
	_, _ = docs.Create(ctx, domain.Document{
		ID:        "source",
		OwnerID:   "u1",
		Type:      domain.DocFree,
		Path:      "source/path",
		Title:     "Source Link",
		Body:      "Links [[target/path]].",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err := docs.ReplaceLinks(ctx, "source", "u1", []string{"target/path"}); err != nil {
		t.Fatal(err)
	}

	body, status := getWissenDocumentAs(t, srv, codec, "u1", "/wissen/target")
	if status != http.StatusOK {
		t.Fatalf("GET /wissen/target status=%d body=%.400s", status, body)
	}
	for _, want := range []string{
		"<table",
		"callout-",
		`class="chroma"`,
		`class="spine"`,
		`class="prov"`,
		`href="/wissen/target/bearbeiten"`,
		`hx-post="/wissen/target/pin"`,
		"Source Link", // inline wikilink resolution inside the prose body
		`href="/wissen/source"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("GET /wissen/target missing %q in %.1200s", want, body)
		}
	}
	// Lesesaal L3 (Task 5): no Kristall chrome left on the swapped fragment.
	// Scope the negative check to everything BEFORE the shared ConfirmDialog
	// markup — that shared component's BtnDanger button legitimately still
	// carries its own "shadow-soft" (button.templ, app-wide, out of scope).
	fragStart := strings.Index(body, `id="document-fragment"`)
	dialogStart := strings.Index(body, `<dialog id="del-target"`)
	if fragStart < 0 || dialogStart < 0 || dialogStart < fragStart {
		t.Fatalf("could not locate document-fragment/ConfirmDialog markers in body: %.1200s", body)
	}
	swappedBlock := body[fragStart:dialogStart]
	for _, gone := range []string{"glass", "bg-surface", "shadow-soft", "font-display"} {
		if strings.Contains(swappedBlock, gone) {
			t.Errorf("Dokument meta/action chrome should not carry Kristall %q, got swapped block:\n%s", gone, swappedBlock)
		}
	}
	// Also verify the read/prose/docrail region (after ConfirmDialog closes)
	// stays Kristall-free.
	dialogEnd := strings.Index(body, `</dialog>`)
	articleEnd := strings.Index(body, `</article>`)
	if dialogEnd < 0 || articleEnd < 0 || articleEnd < dialogEnd {
		t.Fatalf("could not locate </dialog> or </article> markers in body: %.1200s", body)
	}
	treeBlock := body[dialogEnd:articleEnd]
	for _, gone := range []string{"glass", "bg-surface", "shadow-soft"} {
		if strings.Contains(treeBlock, gone) {
			t.Errorf("Read/prose/docrail should not carry Kristall %q, got block:\n%s", gone, treeBlock)
		}
	}
	// Edit/delete/pin actions (structure, hrefs, hx-attrs) must still be
	// present after the restyle — Delete stays reachable from this page
	// (Bestand ConfirmDialog), only its chrome moved to named .btn classes.
	if !strings.Contains(body, `data-dialog-open="del-target"`) {
		t.Errorf("expected delete confirm-dialog trigger to survive the restyle, got:\n%s", body)
	}
	if !strings.Contains(body, `id="document-fragment"`) {
		t.Errorf("expected DocumentFragment structure intact, got:\n%s", body)
	}
	if !strings.Contains(body, `hx-post="/wissen/target/delete"`) {
		t.Errorf("expected delete ConfirmDialog hx-post intact, got:\n%s", body)
	}
}

// TestWebWissenDocumentView_OwnerScoped covers the Task 5 owner-scope
// negative test explicitly required by the brief: a second tenant (u2) must
// never be able to load u1's document via the web document-view route.
func TestWebWissenDocumentView_OwnerScoped(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{
		ID: "secret", OwnerID: "u1", Type: domain.DocFree, Path: "p/secret",
		Title: "Secret", Body: "shh", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	body, status := getWissenDocumentAs(t, srv, codec, "u2", "/wissen/secret")
	if status != http.StatusNotFound {
		t.Fatalf("u2 GET /wissen/secret status=%d, want 404 (owner-scoped), body=%.400s", status, body)
	}
}

func getWissenDocument(t *testing.T, s *Server, codec SessionCodec, target string) (string, int) {
	t.Helper()
	return getWissenDocumentAs(t, s, codec, "u1", target)
}

func getWissenDocumentAs(t *testing.T, s *Server, codec SessionCodec, userID, target string) (string, int) {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("GET /wissen/{id}", s.webAuth(http.HandlerFunc(s.handleWebDocumentView)))
	cookieVal, err := codec.Issue(userID)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookieVal})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Body.String(), rec.Code
}
