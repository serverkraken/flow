package httpserver_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// TestHandleTagTimes_EmptyRange exercises handleTagTimes (GET /api/v1/sessions/tag-times)
// with no sessions, verifying it returns 200 with an empty JSON array.
func TestHandleTagTimes_EmptyRange(t *testing.T) {
	srv, _ := newDocServer(t)
	ss := testutil.NewFakeSessionStore()
	srv.TagTimeReport = usecase.TagTimeReport{Sessions: ss}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	primeUser(t, ts.URL)

	res := doDoc(t, ts, "GET", "/api/v1/sessions/tag-times", "")
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d body=%.200s", res.StatusCode, b)
	}
	var arr []any
	if err := json.NewDecoder(res.Body).Decode(&arr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if arr == nil {
		t.Error("expected empty array, got nil")
	}
}

// TestHandleTagTimes_BadFromParam exercises the "bad from" error path.
func TestHandleTagTimes_BadFromParam(t *testing.T) {
	srv, _ := newDocServer(t)
	srv.TagTimeReport = usecase.TagTimeReport{Sessions: testutil.NewFakeSessionStore()}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	primeUser(t, ts.URL)

	res := doDoc(t, ts, "GET", "/api/v1/sessions/tag-times?from=not-a-date", "")
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}
}

// TestHandleTagTimes_BadToParam exercises the "bad to" error path.
func TestHandleTagTimes_BadToParam(t *testing.T) {
	srv, _ := newDocServer(t)
	srv.TagTimeReport = usecase.TagTimeReport{Sessions: testutil.NewFakeSessionStore()}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	primeUser(t, ts.URL)

	res := doDoc(t, ts, "GET", "/api/v1/sessions/tag-times?to=not-a-date", "")
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}
}

// TestHandleStripFrontmatter_DryRun exercises the handleStripFrontmatter handler.
func TestHandleStripFrontmatter_DryRun(t *testing.T) {
	srv, _ := newDocServer(t)
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)}
	srv.StripFrontmatter = usecase.StripFrontmatter{Docs: testutil.NewFakeDocumentStore(), Clock: clk}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	primeUser(t, ts.URL)

	res := doDoc(t, ts, "POST", "/api/v1/maintenance/strip-frontmatter?dry_run=true", "")
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d body=%.200s", res.StatusCode, b)
	}
}
