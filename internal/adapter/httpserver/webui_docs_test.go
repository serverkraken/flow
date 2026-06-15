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
		GetDocument:    usecase.GetDocument{Docs: docs},
		ListDocuments:  usecase.ListDocuments{Docs: docs},
		UpdateDocument: usecase.UpdateDocument{Docs: docs, Clock: clk},
		DeleteDocument: usecase.DeleteDocument{Docs: docs},
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

// FIX 4: WebUI update must preserve existing tags (not wipe them).
func TestWebDocUpdate_PreservesTags(t *testing.T) {
	srv, codec, docs := newWebDocsServer(t)
	// Pre-seed a doc WITH tags set via the API.
	_, _ = docs.Create(context.Background(), domain.Document{
		ID: "tag-doc-1", OwnerID: "u1", Type: domain.DocFree,
		Path: "tagged/doc", Title: "Tagged Doc", Body: "original body",
		Tags:      []string{"go", "flow"},
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, _ := codec.Issue("u1")

	form := url.Values{
		"title": {"Updated Title"},
		"body":  {"updated body"},
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

	// Verify tags were NOT wiped.
	stored, err := docs.Get(context.Background(), "u1", "tag-doc-1")
	if err != nil {
		t.Fatalf("getting doc after update: %v", err)
	}
	if len(stored.Tags) != 2 {
		t.Errorf("want 2 preserved tags, got %d: %v", len(stored.Tags), stored.Tags)
	}
	// Verify title+body were updated.
	if stored.Title != "Updated Title" {
		t.Errorf("want title='Updated Title', got %q", stored.Title)
	}
}
