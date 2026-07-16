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

func TestWebDocumentView_EmbedBadgeFailedShowsRetry(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{
		ID: "d1", OwnerID: "u1", Type: domain.DocFree, Path: "p/x",
		Title: "X", Body: "b", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	_ = docs.RecordEmbedFailure(ctx, "d1", "u1", docs.SnapshotHash("d1"), 5, time.Now(), true, "boom")

	body, status := getWissenDocument(t, srv, codec, "/wissen/d1")
	if status != http.StatusOK {
		t.Fatalf("GET /wissen/d1 status=%d body=%.400s", status, body)
	}
	if !strings.Contains(body, "/wissen/d1/reembed") {
		t.Fatalf("expected /wissen/d1/reembed retry form, got %.600s", body)
	}
	// Archive now has a ConfirmDialog; keep the old Lesesaal chrome assertion
	// scoped to the document content before that shared component.
	fragStart := strings.Index(body, `id="document-fragment"`)
	articleEnd := strings.Index(body, `</article>`)
	if fragStart < 0 || articleEnd < 0 || articleEnd < fragStart {
		t.Fatalf("could not locate document-fragment markers in body: %.1200s", body)
	}
	fragment := body[fragStart:articleEnd]
	if dialog := strings.Index(fragment, "<dialog"); dialog >= 0 {
		fragment = fragment[:dialog]
	}
	if !strings.Contains(fragment, `class="btn btn-q btn-s"`) {
		t.Errorf("expected reembed retry button to use the named .btn.btn-q.btn-s class, got fragment:\n%s", fragment)
	}
	for _, gone := range []string{"glass", "bg-surface"} {
		if strings.Contains(fragment, gone) {
			t.Errorf("document fragment should not use %q chrome, got fragment:\n%s", gone, fragment)
		}
	}
}

func TestWebDocumentReembedHTMX(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{
		ID: "d1", OwnerID: "u1", Type: domain.DocFree, Path: "p/x",
		Title: "X", Body: "b", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	_ = docs.RecordEmbedFailure(ctx, "d1", "u1", docs.SnapshotHash("d1"), 5, time.Now(), true, "boom")

	body, status := postWissenReembed(t, srv, codec, "d1")
	if status != http.StatusOK {
		t.Fatalf("POST /wissen/d1/reembed status=%d body=%.400s", status, body)
	}
	if !strings.Contains(body, "embed.pending") && !strings.Contains(body, "Einbettung geplant") {
		t.Fatalf("expected pending embed fragment, got %.400s", body)
	}
}

func TestWebDocumentReembedNotFound(t *testing.T) {
	srv, codec, _, _ := newWebWissenServer(t)
	_, status := postWissenReembed(t, srv, codec, "no-such-id")
	if status != http.StatusNotFound {
		t.Fatalf("POST /wissen/no-such-id/reembed status=%d, want 404", status)
	}
}

func postWissenReembed(t *testing.T, s *Server, codec SessionCodec, id string) (string, int) {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("POST /wissen/{id}/reembed", s.webAuth(http.HandlerFunc(s.handleWebDocReembed)))
	cookieVal, err := codec.Issue("u1")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/wissen/"+id+"/reembed", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookieVal})
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Body.String(), rec.Code
}
