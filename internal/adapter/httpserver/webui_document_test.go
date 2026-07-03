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

	body, status := getWissenDocument(t, srv, codec, "/wissen/target")
	if status != http.StatusOK {
		t.Fatalf("GET /wissen/target status=%d body=%.400s", status, body)
	}
	for _, want := range []string{
		"<table",
		"callout-",
		`class="chroma"`,
		"Zurück zu",
		"Frei",
		`href="/wissen/frei"`,
		`href="/wissen/target/bearbeiten"`,
		"Source Link",
		`href="/wissen/source"`,
		"glass", // Kristall glass chrome (node pill / tag pills / edit / delete)
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("GET /wissen/target missing %q in %.1200s", want, body)
		}
	}
	// The Dokument-owned meta/action chrome (node pill, tag pills, edit/delete
	// actions) must be glass, not the old hand-rolled bg-surface/border-line.
	// Scope the negative check to the swapped block only: from the fragment
	// root up to the (untouched, shared) ConfirmDialog markup — that shared
	// component still legitimately renders "border border-line bg-surface"
	// and is explicitly out of scope for this task.
	fragStart := strings.Index(body, `id="document-fragment"`)
	dialogStart := strings.Index(body, `<dialog id="del-target"`)
	if fragStart < 0 || dialogStart < 0 || dialogStart < fragStart {
		t.Fatalf("could not locate document-fragment/ConfirmDialog markers in body: %.1200s", body)
	}
	swappedBlock := body[fragStart:dialogStart]
	if strings.Contains(swappedBlock, "bg-surface") {
		t.Errorf("Dokument meta/action chrome should use glass, not bg-surface, got swapped block:\n%s", swappedBlock)
	}
	// Node pill, tag pill, edit/delete actions, and the reembed retry chrome
	// (structure, hrefs, hx-attrs) must still be present after the restyle.
	if !strings.Contains(body, `data-dialog-open="del-target"`) {
		t.Errorf("expected delete confirm-dialog trigger to survive the glass swap, got:\n%s", body)
	}
	if !strings.Contains(body, "hover:text-danger") {
		t.Errorf("expected delete button to keep hover:text-danger, got:\n%s", body)
	}
	if !strings.Contains(body, `id="document-fragment"`) {
		t.Errorf("expected DocumentFragment structure intact, got:\n%s", body)
	}
	if !strings.Contains(body, `hx-post="/wissen/target/delete"`) {
		t.Errorf("expected delete ConfirmDialog hx-post intact, got:\n%s", body)
	}
}

func getWissenDocument(t *testing.T, s *Server, codec SessionCodec, target string) (string, int) {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("GET /wissen/{id}", s.webAuth(http.HandlerFunc(s.handleWebDocumentView)))
	cookieVal, err := codec.Issue("u1")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookieVal})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Body.String(), rec.Code
}
