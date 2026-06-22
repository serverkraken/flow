package httpserver_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// TestWebDocView_EmbedBadge_FailedShowsRetry verifies that GET /docs/{id} for a
// dead-lettered (failed) document renders the embed-status chip and Retry button.
func TestWebDocView_EmbedBadge_FailedShowsRetry(t *testing.T) {
	srv, codec, docs := newWebDocsServer(t)
	ctx := context.Background()

	_, _ = docs.Create(ctx, domain.Document{
		ID:        "d1",
		OwnerID:   "u1",
		Type:      domain.DocFree,
		Path:      "p/x",
		Title:     "X",
		Body:      "b",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})
	// Record a dead-lettered failure so EmbedStatus returns EmbedFailed.
	_ = docs.RecordEmbedFailure(ctx, "d1", "u1", 5, time.Now(), true, "boom")

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	req, _ := http.NewRequest("GET", ts.URL+"/docs/d1", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	body := string(b)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /docs/d1 status=%d body=%.400s", res.StatusCode, body)
	}
	// The Retry control links to the reembed route.
	if !strings.Contains(body, "/docs/d1/reembed") {
		t.Fatalf("expected '/docs/d1/reembed' in body (Retry form), got: %.600s", body)
	}
	if !strings.Contains(body, "Retry") {
		t.Fatalf("expected 'Retry' button in body, got: %.600s", body)
	}
}

// TestWebDocReembed_HTMX verifies POST /docs/{id}/reembed with HX-Request returns
// 200 and the "embedding queued" fragment.
func TestWebDocReembed_HTMX(t *testing.T) {
	srv, codec, docs := newWebDocsServer(t)
	ctx := context.Background()

	_, _ = docs.Create(ctx, domain.Document{
		ID:        "d1",
		OwnerID:   "u1",
		Type:      domain.DocFree,
		Path:      "p/x",
		Title:     "X",
		Body:      "b",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})
	_ = docs.RecordEmbedFailure(ctx, "d1", "u1", 5, time.Now(), true, "boom")

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	req, _ := http.NewRequest("POST", ts.URL+"/docs/d1/reembed", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	req.Header.Set("HX-Request", "true")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	body := string(b)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /docs/d1/reembed with HX-Request status=%d body=%.400s", res.StatusCode, body)
	}
	if !strings.Contains(body, "embedding queued") {
		t.Fatalf("expected 'embedding queued' in HTMX fragment, got: %.400s", body)
	}
}

// TestWebDocReembed_NotFound verifies POST /docs/{unknown}/reembed returns 404.
func TestWebDocReembed_NotFound(t *testing.T) {
	srv, codec, _ := newWebDocsServer(t)

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	req, _ := http.NewRequest("POST", ts.URL+"/docs/no-such-id/reembed", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	req.Header.Set("HX-Request", "true")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("POST /docs/{unknown}/reembed want 404, got %d", res.StatusCode)
	}
}
