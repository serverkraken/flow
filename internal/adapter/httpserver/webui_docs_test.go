package httpserver_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/adapter/websession"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// newWebDocsServer wires the docs web handlers behind cookie auth, with a
// pre-seeded user "u1" whose session cookie the test forges via the codec.
// It also returns the FakeDocumentStore so tests can inspect stored state.
func newWebDocsServer(t *testing.T) (*httpserver.Server, *websession.Codec, *testutil.FakeDocumentStore) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)}
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x", "Martin")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour)
	bus := sse.NewBus()
	docs := testutil.NewFakeDocumentStore()

	srv := &httpserver.Server{
		Ensure:  usecase.EnsureUser{Users: users, IDs: &testutil.FakeIDGen{}, Allow: func(ports.Identity) bool { return true }},
		Bus:     bus,
		Clock:   clk,
		Users:   users,
		Session: codec,
		CreateDocument: usecase.CreateDocument{
			Docs:  docs,
			IDs:   &testutil.FakeIDGen{},
			Clock: clk,
		},
		GetDocument:       usecase.GetDocument{Docs: docs},
		ListDocuments:     usecase.ListDocuments{Docs: docs},
		UpdateDocument:    usecase.UpdateDocument{Docs: docs, Clock: clk},
		DeleteDocument:    usecase.DeleteDocument{Docs: docs},
		BacklinksDocument: usecase.Backlinks{Docs: docs},
		ListTags:          usecase.ListTags{Docs: docs},
	}
	return srv, codec, docs
}

func TestWebDocsHome(t *testing.T) {
	srv, codec, docs := newWebDocsServer(t)
	// Pre-seed a document so the list is non-empty.
	_, _ = docs.Create(context.Background(), domain.Document{
		ID:        "doc-1",
		OwnerID:   "u1",
		Type:      domain.DocFree,
		Path:      "my-note/intro",
		Title:     "My Intro Doc",
		Body:      "hello",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	req, _ := http.NewRequest("GET", ts.URL+"/docs", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	body := string(b)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /docs status=%d body=%.200s", res.StatusCode, body)
	}
	if !strings.Contains(body, "flow · docs") {
		t.Fatalf("expected 'flow · docs' in body, got: %.200s", body)
	}
	if !strings.Contains(body, "my-note/intro") {
		t.Fatalf("expected doc path 'my-note/intro' in body, got: %.300s", body)
	}
	if !strings.Contains(body, "My Intro Doc") {
		t.Fatalf("expected doc title 'My Intro Doc' in body, got: %.300s", body)
	}
}

// TestWebDocCreate_PublishesEvent locks in the live-sync contract: a document
// created through the WebUI form must publish document.created on the bus so the
// TUI (and other WebUI tabs) refresh live. Regression guard for the bug where
// only the REST handler published while the WebUI handler stayed silent.
func TestWebDocCreate_PublishesEvent(t *testing.T) {
	srv, codec, _ := newWebDocsServer(t)
	ch, cancel := srv.Bus.Subscribe("u1")
	defer cancel()

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	form := url.Values{"type": {"free"}, "path": {"live/sync"}, "title": {"Live"}, "body": {"x"}}
	req, _ := http.NewRequest("POST", ts.URL+"/docs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	// Don't follow the 303 redirect; we only care that the event fired.
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("want 303, got %d", res.StatusCode)
	}

	select {
	case ev := <-ch:
		if ev.Type != domain.EventDocumentCreated {
			t.Fatalf("want document.created, got %q", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("no document.created event published by WebUI create handler")
	}
}

func TestWebDocView(t *testing.T) {
	srv, codec, docs := newWebDocsServer(t)
	_, _ = docs.Create(context.Background(), domain.Document{
		ID:        "doc-view-1",
		OwnerID:   "u1",
		Type:      domain.DocFree,
		Path:      "view-test",
		Title:     "View Test",
		Body:      "# Hello World",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	req, _ := http.NewRequest("GET", ts.URL+"/docs/doc-view-1", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	body := string(b)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /docs/{id} status=%d body=%.200s", res.StatusCode, body)
	}
	// Markdown "# Hello World" should render as <h1
	if !strings.Contains(body, "<h1") {
		t.Fatalf("expected rendered <h1> from markdown, got: %.300s", body)
	}
	if !strings.Contains(body, "View Test") {
		t.Fatalf("expected doc title 'View Test' in body, got: %.200s", body)
	}
}

func TestWebDocNew(t *testing.T) {
	srv, codec, _ := newWebDocsServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	req, _ := http.NewRequest("GET", ts.URL+"/docs/new", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	body := string(b)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /docs/new status=%d body=%.200s", res.StatusCode, body)
	}
	if !strings.Contains(body, `name="path"`) {
		t.Fatalf("expected name=\"path\" form field in body, got: %.300s", body)
	}
	if !strings.Contains(body, "<textarea") {
		t.Fatalf("expected <textarea> form field in body, got: %.300s", body)
	}
}

func TestWebDocCreate(t *testing.T) {
	srv, codec, docs := newWebDocsServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	form := url.Values{
		"type":  {"free"},
		"path":  {"test-create/note"},
		"title": {"Created Note"},
		"body":  {"some content"},
	}.Encode()

	req, _ := http.NewRequest("POST", ts.URL+"/docs", strings.NewReader(form))
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()

	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /docs want 303, got %d", res.StatusCode)
	}

	// Verify doc was created in store.
	list, _ := docs.List(context.Background(), "u1")
	if len(list) == 0 {
		t.Fatal("expected a doc in store after POST /docs")
	}
	var found bool
	for _, d := range list {
		if d.Path == "test-create/note" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("doc with path 'test-create/note' not found in store after create")
	}
}

func TestWebDocView_NotFound(t *testing.T) {
	srv, codec, _ := newWebDocsServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	req, _ := http.NewRequest("GET", ts.URL+"/docs/no-such-id", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /docs/{unknownid} want 404, got %d", res.StatusCode)
	}
}

// FIX 1: list page must carry SSE live-refresh attributes.
func TestWebDocsHome_SSEAttrs(t *testing.T) {
	srv, codec, docs := newWebDocsServer(t)
	_, _ = docs.Create(context.Background(), domain.Document{
		ID: "sse-doc-1", OwnerID: "u1", Type: domain.DocFree,
		Path: "sse-test", Title: "SSE Doc", Body: "hi",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	req, _ := http.NewRequest("GET", ts.URL+"/docs", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	body := string(b)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /docs status=%d body=%.200s", res.StatusCode, body)
	}
	if !strings.Contains(body, `sse-connect="/api/v1/events"`) {
		t.Fatalf("expected sse-connect attr in /docs page, got: %.400s", body)
	}
	if !strings.Contains(body, `hx-get="/ui/docs/list"`) {
		t.Fatalf("expected hx-get=/ui/docs/list attr in /docs page, got: %.400s", body)
	}
	if !strings.Contains(body, "document.created") {
		t.Fatalf("expected document.created SSE trigger in /docs page, got: %.400s", body)
	}
}

// FIX 1: GET /ui/docs/list must return fragment (no <!DOCTYPE), but contain doc rows.
func TestWebDocsList_Fragment(t *testing.T) {
	srv, codec, docs := newWebDocsServer(t)
	_, _ = docs.Create(context.Background(), domain.Document{
		ID: "frag-doc-1", OwnerID: "u1", Type: domain.DocFree,
		Path: "frag-test", Title: "Frag Doc", Body: "body",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	req, _ := http.NewRequest("GET", ts.URL+"/ui/docs/list", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	body := string(b)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /ui/docs/list status=%d body=%.200s", res.StatusCode, body)
	}
	if strings.Contains(body, "<!DOCTYPE") {
		t.Fatalf("fragment must NOT contain <!DOCTYPE, but got full page: %.200s", body)
	}
	if !strings.Contains(body, "frag-test") {
		t.Fatalf("expected doc path 'frag-test' in fragment, got: %.300s", body)
	}
	if !strings.Contains(body, "Frag Doc") {
		t.Fatalf("expected doc title 'Frag Doc' in fragment, got: %.300s", body)
	}
}

// FIX 2: edit form must render path and type as disabled (read-only).
func TestWebDocEdit_ReadOnlyFields(t *testing.T) {
	srv, codec, docs := newWebDocsServer(t)
	_, _ = docs.Create(context.Background(), domain.Document{
		ID: "edit-doc-1", OwnerID: "u1", Type: domain.DocFree,
		Path: "edit-test/path", Title: "Edit Me", Body: "content",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	req, _ := http.NewRequest("GET", ts.URL+"/docs/edit-doc-1/edit", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	body := string(b)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /docs/{id}/edit status=%d body=%.200s", res.StatusCode, body)
	}
	// path input must be disabled in edit mode.
	if !strings.Contains(body, "disabled") {
		t.Fatalf("expected disabled attribute on path/type fields in edit form, got: %.400s", body)
	}
	// title and body must remain editable (no disabled near them).
	if !strings.Contains(body, `name="title"`) {
		t.Fatalf("expected name=title input in edit form, got: %.300s", body)
	}
	if !strings.Contains(body, "<textarea") {
		t.Fatalf("expected textarea (body) in edit form, got: %.300s", body)
	}
}

// FIX 2: new form must NOT have disabled attributes on path/type.
func TestWebDocNew_EditableFields(t *testing.T) {
	srv, codec, _ := newWebDocsServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	req, _ := http.NewRequest("GET", ts.URL+"/docs/new", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	body := string(b)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /docs/new status=%d body=%.200s", res.StatusCode, body)
	}
	if strings.Contains(body, "disabled") {
		t.Fatalf("new form must NOT have disabled fields, got: %.400s", body)
	}
	// type should be a <select> in create mode.
	if !strings.Contains(body, "<select") {
		t.Fatalf("expected <select> for type in new form, got: %.300s", body)
	}
}

// FIX 3: duplicate path (409) must re-render form WITH the submitted values.
func TestWebDocCreate_DuplicatePath_PreservesFormValues(t *testing.T) {
	srv, codec, docs := newWebDocsServer(t)
	// Pre-seed a doc to cause a collision.
	_, _ = docs.Create(context.Background(), domain.Document{
		ID: "existing-doc", OwnerID: "u1", Type: domain.DocFree,
		Path: "collision/path", Title: "Existing",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	form := url.Values{
		"type":  {"free"},
		"path":  {"collision/path"},
		"title": {"My Duplicate Title"},
		"body":  {"my body content"},
	}.Encode()

	req, _ := http.NewRequest("POST", ts.URL+"/docs", strings.NewReader(form))
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	body := string(b)

	if res.StatusCode != http.StatusConflict {
		t.Fatalf("POST /docs with dup path want 409, got %d body=%.200s", res.StatusCode, body)
	}
	// Form must carry back submitted values.
	if !strings.Contains(body, "collision/path") {
		t.Fatalf("expected submitted path 'collision/path' in error re-render, got: %.400s", body)
	}
	if !strings.Contains(body, "My Duplicate Title") {
		t.Fatalf("expected submitted title in error re-render, got: %.400s", body)
	}
	// Must show the error message.
	if !strings.Contains(body, "already exists") {
		t.Fatalf("expected 'already exists' error text in error re-render, got: %.300s", body)
	}
}

// TestWebDocDelete covers handleWebDocDelete: POST /docs/{id}/delete → 303 /docs.
func TestWebDocDelete(t *testing.T) {
	srv, codec, docs := newWebDocsServer(t)
	_, _ = docs.Create(context.Background(), domain.Document{
		ID: "del-doc-1", OwnerID: "u1", Type: domain.DocFree,
		Path: "delete-me", Title: "Delete Me", Body: "bye",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	req, _ := http.NewRequest("POST", ts.URL+"/docs/del-doc-1/delete", strings.NewReader(""))
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()

	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /docs/{id}/delete want 303, got %d", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); loc != "/docs" {
		t.Fatalf("POST /docs/{id}/delete Location want /docs, got %q", loc)
	}

	// Verify doc is gone from store.
	list, _ := docs.List(context.Background(), "u1")
	for _, d := range list {
		if d.ID == "del-doc-1" {
			t.Fatal("doc should be deleted from store, but it still exists")
		}
	}
}

// TestWebDocDelete_NotFound covers the 404 branch of handleWebDocDelete.
func TestWebDocDelete_NotFound(t *testing.T) {
	srv, codec, _ := newWebDocsServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	req, _ := http.NewRequest("POST", ts.URL+"/docs/no-such-id/delete", strings.NewReader(""))
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("POST /docs/{unknown}/delete want 404, got %d", res.StatusCode)
	}
}

// TestWebDocUpdate_TitleAndBody covers handleWebDocUpdate: plain title+body update.
func TestWebDocUpdate_TitleAndBody(t *testing.T) {
	srv, codec, docs := newWebDocsServer(t)
	_, _ = docs.Create(context.Background(), domain.Document{
		ID: "upd-doc-1", OwnerID: "u1", Type: domain.DocFree,
		Path: "update-me", Title: "Old Title", Body: "old body",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	form := url.Values{
		"title": {"New Title"},
		"body":  {"new body"},
	}.Encode()

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	req, _ := http.NewRequest("POST", ts.URL+"/docs/upd-doc-1", strings.NewReader(form))
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()

	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /docs/{id} want 303, got %d", res.StatusCode)
	}

	stored, err := docs.Get(context.Background(), "u1", "upd-doc-1")
	if err != nil {
		t.Fatalf("getting updated doc: %v", err)
	}
	if stored.Title != "New Title" {
		t.Errorf("want title='New Title', got %q", stored.Title)
	}
	if stored.Body != "new body" {
		t.Errorf("want body='new body', got %q", stored.Body)
	}
}

// TestWebDocUpdate_NotFound covers the 404 branch of handleWebDocUpdate.
func TestWebDocUpdate_NotFound(t *testing.T) {
	srv, codec, _ := newWebDocsServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	form := url.Values{"title": {"x"}, "body": {"y"}}.Encode()
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	req, _ := http.NewRequest("POST", ts.URL+"/docs/no-such-id", strings.NewReader(form))
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("POST /docs/{unknown} want 404, got %d", res.StatusCode)
	}
}

// TestWebDocEdit_NotFound covers the 404 branch of handleWebDocEdit.
func TestWebDocEdit_NotFound(t *testing.T) {
	srv, codec, _ := newWebDocsServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	req, _ := http.NewRequest("GET", ts.URL+"/docs/no-such-id/edit", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /docs/{unknown}/edit want 404, got %d", res.StatusCode)
	}
}

// TestWebDocCreate_InvalidDocument covers the 400 (ErrInvalidDocument) branch.
func TestWebDocCreate_InvalidDocument(t *testing.T) {
	srv, codec, _ := newWebDocsServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	// Send an invalid type value to trigger ErrInvalidDocument.
	form := url.Values{
		"type":  {"invalid-type"},
		"path":  {"bad path with spaces"},
		"title": {"Bad"},
		"body":  {""},
	}.Encode()

	req, _ := http.NewRequest("POST", ts.URL+"/docs", strings.NewReader(form))
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	// Accept either 400 or 409 — just must not be 303.
	if res.StatusCode == http.StatusSeeOther {
		t.Fatalf("invalid doc POST should not redirect 303, got %d body=%.200s", res.StatusCode, b)
	}
}

// Tags are derived from YAML frontmatter in the submitted body; pre-existing
// store tags are replaced, not preserved.
func TestWebDocUpdate_TagsDerivedFromFrontmatter(t *testing.T) {
	srv, codec, docs := newWebDocsServer(t)
	// Pre-seed a doc with stale tags (will be replaced by frontmatter derivation).
	_, _ = docs.Create(context.Background(), domain.Document{
		ID: "tag-doc-1", OwnerID: "u1", Type: domain.DocFree,
		Path: "tagged/doc", Title: "Tagged Doc", Body: "original body",
		Tags:      []string{"old-tag"},
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	// Submit body with frontmatter — tags come from here, not preserved from store.
	form := url.Values{
		"title": {"Updated Title"},
		"body":  {"---\ntags: [go, flow]\n---\nupdated body"},
	}.Encode()

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	req, _ := http.NewRequest("POST", ts.URL+"/docs/tag-doc-1", strings.NewReader(form))
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()

	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /docs/{id} want 303, got %d", res.StatusCode)
	}

	// Verify tags are derived from the submitted frontmatter (not from old store value).
	stored, err := docs.Get(context.Background(), "u1", "tag-doc-1")
	if err != nil {
		t.Fatalf("getting doc after update: %v", err)
	}
	if len(stored.Tags) != 2 {
		t.Errorf("want 2 frontmatter-derived tags, got %d: %v", len(stored.Tags), stored.Tags)
	}
	// Verify title+body were updated.
	if stored.Title != "Updated Title" {
		t.Errorf("want title='Updated Title', got %q", stored.Title)
	}
}

// TestWebDocsList_FilterBarAndChips verifies that:
//   - GET /docs shows tag chips for all tags when documents have tags.
//   - GET /ui/docs/list?tag=go&tag=tui filters to only docs that carry BOTH tags.
func TestWebDocsList_FilterBarAndChips(t *testing.T) {
	srv, codec, docs := newWebDocsServer(t)

	// Seed doc A with tags [go, tui].
	_, _ = docs.Create(context.Background(), domain.Document{
		ID: "tag-a", OwnerID: "u1", Type: domain.DocFree,
		Path: "doc-a", Title: "Doc Alpha",
		Tags:      []string{"go", "tui"},
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	// Seed doc B with tags [web].
	_, _ = docs.Create(context.Background(), domain.Document{
		ID: "tag-b", OwnerID: "u1", Type: domain.DocFree,
		Path: "doc-b", Title: "Doc Beta",
		Tags:      []string{"web"},
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")
	cookie := &http.Cookie{Name: "flow_session", Value: cookieVal}

	authedGet := func(path string) string {
		t.Helper()
		req, _ := http.NewRequest("GET", ts.URL+path, nil)
		req.AddCookie(cookie)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		b, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%.300s", path, res.StatusCode, b)
		}
		return string(b)
	}

	// GET /docs — filter bar should list all tags (#go, #tui, #web).
	body := authedGet("/docs")
	if !strings.Contains(body, "#go") {
		t.Errorf("GET /docs: expected '#go' chip in filter bar, got: %.400s", body)
	}
	if !strings.Contains(body, "#web") {
		t.Errorf("GET /docs: expected '#web' chip in filter bar, got: %.400s", body)
	}

	// GET /ui/docs/list?tag=go&tag=tui — AND filter: should include Doc Alpha, not Doc Beta.
	filtered := authedGet("/ui/docs/list?tag=go&tag=tui")
	if !strings.Contains(filtered, "Doc Alpha") {
		t.Errorf("GET /ui/docs/list?tag=go&tag=tui: expected 'Doc Alpha' in body, got: %.400s", filtered)
	}
	if strings.Contains(filtered, "Doc Beta") {
		t.Errorf("GET /ui/docs/list?tag=go&tag=tui: 'Doc Beta' should be filtered out, got: %.400s", filtered)
	}
}

// TestWebDocView_WikilinksAndBacklinks verifies:
//   - GET /docs/{destID} contains "Referenced by" and the src title when src links to dest.
//   - GET /docs/{srcID} contains a rendered wikilink anchor pointing to /docs/{destID}.
func TestWebDocView_WikilinksAndBacklinks(t *testing.T) {
	srv, codec, store := newWebDocsServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")
	cookieHdr := &http.Cookie{Name: "flow_session", Value: cookieVal}

	// Create dest doc (empty body — no outgoing links).
	postDoc := func(path, title, body string) {
		t.Helper()
		form := url.Values{"type": {"free"}, "path": {path}, "title": {title}, "body": {body}}
		req, _ := http.NewRequest("POST", ts.URL+"/docs", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(cookieHdr)
		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST /docs: %v", err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusSeeOther {
			t.Fatalf("POST /docs want 303, got %d", res.StatusCode)
		}
	}

	postDoc("wikilink-dest", "Dest Doc", "")
	postDoc("wikilink-src", "Src Doc", "go to [[wikilink-dest]]")

	// Resolve IDs by path via the store.
	list, err := store.List(context.Background(), "u1")
	if err != nil {
		t.Fatalf("store.List: %v", err)
	}
	idByPath := map[string]string{}
	for _, d := range list {
		idByPath[d.Path] = d.ID
	}
	destID, ok := idByPath["wikilink-dest"]
	if !ok {
		t.Fatal("dest doc not found in store")
	}
	srcID, ok := idByPath["wikilink-src"]
	if !ok {
		t.Fatal("src doc not found in store")
	}

	getBody := func(path string) string {
		t.Helper()
		req, _ := http.NewRequest("GET", ts.URL+path, nil)
		req.AddCookie(cookieHdr)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		b, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%.300s", path, res.StatusCode, b)
		}
		return string(b)
	}

	// Dest page should show the backlink from Src.
	destBody := getBody("/docs/" + destID)
	if !strings.Contains(destBody, "Referenced by") {
		t.Errorf("dest doc page: expected 'Referenced by' section, got: %.400s", destBody)
	}
	if !strings.Contains(destBody, "Src Doc") {
		t.Errorf("dest doc page: expected src title 'Src Doc' in backlinks, got: %.400s", destBody)
	}

	// Src page should contain a rendered wikilink anchor pointing to dest.
	srcBody := getBody("/docs/" + srcID)
	wantHref := `href="/docs/` + destID + `"`
	if !strings.Contains(srcBody, wantHref) {
		t.Errorf("src doc page: expected wikilink anchor %q, got: %.400s", wantHref, srcBody)
	}
}
