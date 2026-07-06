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
	// Lesesaal L3 (Task 5, fixed after review): the document view page itself
	// no longer carries a ConfirmDialog at all — Delete moved to the edit
	// page (editor.templ, edit mode only; see TestEditorEditModeHasDeleteConfirmDialog
	// in webui_editor_test.go) to match the Mockup (Z.688–695: only
	// Bearbeiten + Anpinnen) and the same L2 doctrine already applied to
	// nodes. So the whole document-fragment article can be checked directly
	// for Kristall remnants, no ConfirmDialog scoping needed anymore.
	fragStart := strings.Index(body, `id="document-fragment"`)
	articleEnd := strings.Index(body, `</article>`)
	if fragStart < 0 || articleEnd < 0 || articleEnd < fragStart {
		t.Fatalf("could not locate document-fragment markers in body: %.1200s", body)
	}
	fragment := body[fragStart:articleEnd]
	for _, gone := range []string{"glass", "bg-surface", "shadow-soft", "font-display", "data-dialog-open=\"del-"} {
		if strings.Contains(fragment, gone) {
			t.Errorf("Document fragment should not carry Kristall/delete remnant %q, got fragment:\n%s", gone, fragment)
		}
	}
	// Edit/pin actions (structure, hrefs, hx-attrs) must still be present.
	if !strings.Contains(body, `id="document-fragment"`) {
		t.Errorf("expected DocumentFragment structure intact, got:\n%s", body)
	}
	if strings.Contains(body, `hx-post="/wissen/target/delete"`) {
		t.Errorf("delete must no longer be reachable from the document view page, got:\n%s", body)
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
