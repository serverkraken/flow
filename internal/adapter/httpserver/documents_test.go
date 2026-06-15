package httpserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// newDocServer builds a Server wired with the 5 document usecases using
// in-memory fakes. Returns the server and the bus so tests can assert events.
// Stats is minimally wired so primeUser (which hits /api/v1/today) doesn't panic.
func newDocServer(t *testing.T) (*httpserver.Server, *sse.Bus) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	bus := sse.NewBus()
	docs := testutil.NewFakeDocumentStore()

	sessions := testutil.NewFakeSessionStore()
	settings := testutil.NewFakeUserSettingsStore()
	dayOffs := testutil.NewFakeDayOffStore()
	listDayOffs := usecase.ListDayOffs{Store: dayOffs, Settings: settings, Loc: time.UTC}
	stats := usecase.StatsComputer{
		Sessions: sessions,
		Settings: settings,
		DayOffs:  listDayOffs,
		Clock:    clk,
		Loc:      time.UTC,
	}

	srv := &httpserver.Server{
		Verifier:       testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:         usecase.EnsureUser{Users: testutil.NewFakeUserStore(), IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:            bus,
		Clock:          clk,
		Stats:          stats,
		CreateDocument: usecase.CreateDocument{Docs: docs, IDs: ids, Clock: clk},
		GetDocument:    usecase.GetDocument{Docs: docs},
		ListDocuments:  usecase.ListDocuments{Docs: docs},
		UpdateDocument: usecase.UpdateDocument{Docs: docs, Clock: clk},
		DeleteDocument: usecase.DeleteDocument{Docs: docs},
	}
	return srv, bus
}

// doDoc issues an authenticated JSON request to the document API.
func doDoc(t *testing.T, ts *httptest.Server, method, path, body string) *http.Response {
	t.Helper()
	var bodyReader *strings.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	} else {
		bodyReader = strings.NewReader("")
	}
	req, _ := http.NewRequest(method, ts.URL+path, bodyReader)
	req.Header.Set("Authorization", "Bearer x")
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestHandleCreateDocument_HappyPath(t *testing.T) {
	srv, bus := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	// Prime EnsureUser so user "id-1" is created.
	primeUser(t, ts.URL)

	// Subscribe to document.created events for user id-1.
	ch, cancel := bus.Subscribe("id-1")
	defer cancel()

	body := `{"type":"free","path":"my-first-note","title":"Hello","body":"World"}`
	res := doDoc(t, ts, "POST", "/api/v1/documents", body)
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusCreated {
		t.Fatalf("want 201, got %d", res.StatusCode)
	}

	var doc domain.Document
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if doc.ID == "" {
		t.Error("want non-empty id in response")
	}
	if doc.Path != "my-first-note" {
		t.Errorf("want path my-first-note, got %q", doc.Path)
	}

	// Assert document.created event was published.
	select {
	case ev := <-ch:
		if ev.Type != domain.EventDocumentCreated {
			t.Errorf("want event type %q, got %q", domain.EventDocumentCreated, ev.Type)
		}
	default:
		t.Error("want document.created event, got none")
	}
}

func TestHandleCreateDocument_BadType(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	body := `{"type":"bogus","path":"some-path","title":"T","body":"B"}`
	res := doDoc(t, ts, "POST", "/api/v1/documents", body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid type, got %d", res.StatusCode)
	}
}

func TestHandleCreateDocument_DuplicatePath(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	body := `{"type":"free","path":"notes/intro","title":"Intro","body":""}`
	res := doDoc(t, ts, "POST", "/api/v1/documents", body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("first create: want 201, got %d", res.StatusCode)
	}

	// Second create with same path → 409.
	res2 := doDoc(t, ts, "POST", "/api/v1/documents", body)
	_ = res2.Body.Close()
	if res2.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate path: want 409, got %d", res2.StatusCode)
	}
}

func TestHandleListDocuments(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	// Create one document first.
	doDoc(t, ts, "POST", "/api/v1/documents", `{"type":"free","path":"list-test","title":"T","body":"B"}`)

	res := doDoc(t, ts, "GET", "/api/v1/documents", "")
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", res.StatusCode)
	}

	var list []domain.Document
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) == 0 {
		t.Error("want at least 1 document in list")
	}
}

func TestHandleGetDocument_HappyPath(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	// Create document and extract its ID.
	createRes := doDoc(t, ts, "POST", "/api/v1/documents", `{"type":"free","path":"get-test","title":"GetMe","body":""}`)
	defer func() { _ = createRes.Body.Close() }()
	var created domain.Document
	_ = json.NewDecoder(createRes.Body).Decode(&created)

	res := doDoc(t, ts, "GET", "/api/v1/documents/"+created.ID, "")
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", res.StatusCode)
	}
	var got domain.Document
	_ = json.NewDecoder(res.Body).Decode(&got)
	if got.ID != created.ID {
		t.Errorf("want id %q, got %q", created.ID, got.ID)
	}
}

func TestHandleGetDocument_NotFound(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	res := doDoc(t, ts, "GET", "/api/v1/documents/no-such-id", "")
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", res.StatusCode)
	}
}

func TestHandleUpdateDocument_HappyPath(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	createRes := doDoc(t, ts, "POST", "/api/v1/documents", `{"type":"free","path":"update-test","title":"Old","body":"old body"}`)
	defer func() { _ = createRes.Body.Close() }()
	var created domain.Document
	_ = json.NewDecoder(createRes.Body).Decode(&created)

	putBody := `{"title":"New Title","body":"new body","tags":["go","test"]}`
	res := doDoc(t, ts, "PUT", "/api/v1/documents/"+created.ID, putBody)
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", res.StatusCode)
	}
	var updated domain.Document
	_ = json.NewDecoder(res.Body).Decode(&updated)
	if updated.Title != "New Title" {
		t.Errorf("want title %q, got %q", "New Title", updated.Title)
	}
	if updated.Body != "new body" {
		t.Errorf("want body %q, got %q", "new body", updated.Body)
	}
}

func TestHandleUpdateDocument_NotFound(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	res := doDoc(t, ts, "PUT", "/api/v1/documents/no-such-id", `{"title":"X","body":"Y"}`)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", res.StatusCode)
	}
}

func TestHandleDeleteDocument_HappyPath(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	createRes := doDoc(t, ts, "POST", "/api/v1/documents", `{"type":"free","path":"delete-test","title":"Del","body":""}`)
	defer func() { _ = createRes.Body.Close() }()
	var created domain.Document
	_ = json.NewDecoder(createRes.Body).Decode(&created)

	res := doDoc(t, ts, "DELETE", "/api/v1/documents/"+created.ID, "")
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("want 204, got %d", res.StatusCode)
	}

	// Verify the document is gone.
	getRes := doDoc(t, ts, "GET", "/api/v1/documents/"+created.ID, "")
	_ = getRes.Body.Close()
	if getRes.StatusCode != http.StatusNotFound {
		t.Fatalf("after delete: want 404, got %d", getRes.StatusCode)
	}
}

func TestHandleDeleteDocument_NotFound(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	res := doDoc(t, ts, "DELETE", "/api/v1/documents/no-such-id", "")
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", res.StatusCode)
	}
}
