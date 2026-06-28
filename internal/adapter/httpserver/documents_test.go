package httpserver_test

import (
	"context"
	"encoding/json"
	"errors"
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
	tags := testutil.NewFakeTagStore()

	nodes := testutil.NewFakeNodeStore()
	binds := testutil.NewFakeProjectBindingStore()

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
		Verifier:          testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:            usecase.EnsureUser{Users: testutil.NewFakeUserStore(), IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:               bus,
		Clock:             clk,
		Stats:             stats,
		CreateDocument:    usecase.CreateDocument{Docs: docs, Tags: tags, IDs: ids, Clock: clk},
		ImportDocument:    usecase.ImportDocument{Docs: docs, Tags: tags, IDs: ids, Clock: clk},
		GetDocument:       usecase.GetDocument{Docs: docs},
		ListDocuments:     usecase.ListDocuments{Docs: docs},
		UpdateDocument:    usecase.UpdateDocument{Docs: docs, Tags: tags, Clock: clk},
		DeleteDocument:    usecase.DeleteDocument{Docs: docs, Tags: tags},
		BacklinksDocument: usecase.Backlinks{Docs: docs},
		ListTags:          usecase.ListTags{Tags: tags},
		SearchDocuments:   usecase.SearchDocuments{Docs: docs},
		SetPinned:         usecase.SetPinned{Docs: docs},
		// Session usecases wired with the shared FakeTagStore so session
		// multi-tags round-trip through the taggings junction (B2 D1).
		StartSession: usecase.StartSession{Sessions: sessions, IDs: ids, Clock: clk, Tags: tags},
		AddSession:   usecase.AddSession{Sessions: sessions, IDs: ids, Clock: clk, Tags: tags},
		EditSession:  usecase.EditSession{Sessions: sessions, Tags: tags},
		ComposeContext: usecase.ComposeContext{
			Resolve: usecase.ResolveNode{Bindings: binds, Nodes: nodes},
			Nodes:   nodes, Docs: docs, Tags: tags,
		},
		SetActiveContext: usecase.SetActiveContext{
			Resolve: usecase.ResolveNode{Bindings: binds, Nodes: nodes},
			Nodes:   nodes, Docs: docs, Tags: tags,
		},
		ContextBudget: 6000,
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


func TestHandleListDocuments_TagFilter(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	// doc "a": tags [go, tui] via explicit tags param
	resA := doDoc(t, ts, "POST", "/api/v1/documents",
		`{"type":"free","path":"tag-filter-a","title":"A","body":"some content","tags":["go","tui"]}`)
	_ = resA.Body.Close()
	if resA.StatusCode != http.StatusCreated {
		t.Fatalf("create A: want 201, got %d", resA.StatusCode)
	}

	// doc "b": tags [go] only via explicit tags param
	resB := doDoc(t, ts, "POST", "/api/v1/documents",
		`{"type":"free","path":"tag-filter-b","title":"B","body":"other content","tags":["go"]}`)
	_ = resB.Body.Close()
	if resB.StatusCode != http.StatusCreated {
		t.Fatalf("create B: want 201, got %d", resB.StatusCode)
	}

	// GET ?tag=go&tag=tui — AND filter: only doc A matches
	res := doDoc(t, ts, "GET", "/api/v1/documents?tag=go&tag=tui", "")
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", res.StatusCode)
	}
	var list []domain.Document
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("want exactly 1 doc, got %d: %+v", len(list), list)
	}
	if list[0].Path != "tag-filter-a" {
		t.Errorf("want doc with path tag-filter-a, got %q", list[0].Path)
	}
}

func TestHandleListTags(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	// doc "a": tags [go, tui] via explicit tags param
	resA := doDoc(t, ts, "POST", "/api/v1/documents",
		`{"type":"free","path":"tags-list-a","title":"A","body":"some content","tags":["go","tui"]}`)
	_ = resA.Body.Close()
	if resA.StatusCode != http.StatusCreated {
		t.Fatalf("create A: want 201, got %d", resA.StatusCode)
	}

	// doc "b": tags [go] via explicit tags param
	resB := doDoc(t, ts, "POST", "/api/v1/documents",
		`{"type":"free","path":"tags-list-b","title":"B","body":"other content","tags":["go"]}`)
	_ = resB.Body.Close()
	if resB.StatusCode != http.StatusCreated {
		t.Fatalf("create B: want 201, got %d", resB.StatusCode)
	}

	// GET /api/v1/documents/tags
	res := doDoc(t, ts, "GET", "/api/v1/documents/tags", "")
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", res.StatusCode)
	}
	var tags []domain.TagCount
	if err := json.NewDecoder(res.Body).Decode(&tags); err != nil {
		t.Fatalf("decode tags: %v", err)
	}
	if len(tags) == 0 {
		t.Fatal("want at least 1 tag, got empty slice")
	}
	// "go" should appear with count 2 (both docs), and be first (highest count)
	if tags[0].Tag != "go" {
		t.Errorf("want first tag %q, got %q", "go", tags[0].Tag)
	}
	if tags[0].Count != 2 {
		t.Errorf("want count 2 for tag %q, got %d", "go", tags[0].Count)
	}
}

// failingDocStore is a ports.DocumentStore stub whose configured methods always
// return an error. All other methods delegate to a FakeDocumentStore so the
// server can still boot.
type failingDocStore struct {
	*testutil.FakeDocumentStore
	listErr      error
	searchErr    error
	createErr    error
	getErr       error
	backlinksErr error
}

func (s *failingDocStore) List(_ context.Context, _ string, _ *string, _ ...string) ([]domain.Document, error) {
	return nil, s.listErr
}

func (s *failingDocStore) Search(_ context.Context, _, _ string, _ *string, _ []string) ([]domain.SearchHit, error) {
	return nil, s.searchErr
}

func (s *failingDocStore) Create(_ context.Context, d domain.Document) (domain.Document, error) {
	if s.createErr != nil {
		return domain.Document{}, s.createErr
	}
	return s.FakeDocumentStore.Create(context.Background(), d)
}

func (s *failingDocStore) Get(_ context.Context, ownerID, id string) (domain.Document, error) {
	if s.getErr != nil {
		return domain.Document{}, s.getErr
	}
	return s.FakeDocumentStore.Get(context.Background(), ownerID, id)
}

func (s *failingDocStore) Backlinks(_ context.Context, ownerID, targetPath string) ([]domain.Document, error) {
	if s.backlinksErr != nil {
		return nil, s.backlinksErr
	}
	return s.FakeDocumentStore.Backlinks(context.Background(), ownerID, targetPath)
}

// errTagStore embeds FakeTagStore and overrides ListTags to always fail.
type errTagStore struct {
	*testutil.FakeTagStore
	err error
}

func (s *errTagStore) ListTags(_ context.Context, _ string, _ domain.TagScope) ([]domain.TagCount, error) {
	return nil, s.err
}

func newFailingDocServer(t *testing.T) *httptest.Server {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	bus := sse.NewBus()
	failing := &failingDocStore{
		FakeDocumentStore: testutil.NewFakeDocumentStore(),
		listErr:           errors.New("db down"),
		searchErr:         errors.New("search db down"),
		createErr:         errors.New("create db down"),
		getErr:            errors.New("get db down"),
		backlinksErr:      errors.New("backlinks db down"),
	}
	sessions := testutil.NewFakeSessionStore()
	settings := testutil.NewFakeUserSettingsStore()
	dayOffs := testutil.NewFakeDayOffStore()
	listDayOffs := usecase.ListDayOffs{Store: dayOffs, Settings: settings, Loc: time.UTC}
	stats := usecase.StatsComputer{
		Sessions: sessions, Settings: settings, DayOffs: listDayOffs, Clock: clk, Loc: time.UTC,
	}
	srv := &httpserver.Server{
		Verifier:          testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:            usecase.EnsureUser{Users: testutil.NewFakeUserStore(), IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:               bus,
		Clock:             clk,
		Stats:             stats,
		CreateDocument:    usecase.CreateDocument{Docs: failing, IDs: ids, Clock: clk},
		ImportDocument:    usecase.ImportDocument{Docs: failing, IDs: ids, Clock: clk},
		GetDocument:       usecase.GetDocument{Docs: failing},
		ListDocuments:     usecase.ListDocuments{Docs: failing},
		UpdateDocument:    usecase.UpdateDocument{Docs: failing, Clock: clk},
		DeleteDocument:    usecase.DeleteDocument{Docs: failing},
		BacklinksDocument: usecase.Backlinks{Docs: failing},
		ListTags:          usecase.ListTags{Tags: testutil.NewFakeTagStore()},
		SearchDocuments:   usecase.SearchDocuments{Docs: failing},
	}
	return httptest.NewServer(srv.Routes())
}

func TestHandleListDocuments_StoreError(t *testing.T) {
	ts := newFailingDocServer(t)
	defer ts.Close()
	primeUser(t, ts.URL)
	res := doDoc(t, ts, "GET", "/api/v1/documents", "")
	_ = res.Body.Close()
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500 on store error, got %d", res.StatusCode)
	}
}

func TestHandleListDocuments_SearchError(t *testing.T) {
	ts := newFailingDocServer(t)
	defer ts.Close()
	primeUser(t, ts.URL)
	// ?q= triggers SearchDocuments; failing store returns error → 500
	res := doDoc(t, ts, "GET", "/api/v1/documents?q=anything", "")
	_ = res.Body.Close()
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500 on search store error, got %d", res.StatusCode)
	}
}

func TestHandleCreateDocument_StoreError(t *testing.T) {
	ts := newFailingDocServer(t)
	defer ts.Close()
	primeUser(t, ts.URL)
	// Valid payload but store.Create returns a generic error → 500
	body := `{"type":"free","path":"err-path","title":"T","body":"B"}`
	res := doDoc(t, ts, "POST", "/api/v1/documents", body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500 on store error, got %d", res.StatusCode)
	}
}

func TestHandleDocumentBacklinks_StoreError(t *testing.T) {
	ts := newFailingDocServer(t)
	defer ts.Close()
	primeUser(t, ts.URL)
	// store.Get returns a generic error (not ErrDocumentNotFound) → 500
	res := doDoc(t, ts, "GET", "/api/v1/documents/any-id/backlinks", "")
	_ = res.Body.Close()
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500 on store error, got %d", res.StatusCode)
	}
}

func TestHandleListTags_StoreError(t *testing.T) {
	srv, _ := newDocServer(t)
	srv.ListTags = usecase.ListTags{Tags: &errTagStore{
		FakeTagStore: testutil.NewFakeTagStore(),
		err:          errors.New("tag store down"),
	}}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	primeUser(t, ts.URL)
	res := doDoc(t, ts, "GET", "/api/v1/documents/tags", "")
	_ = res.Body.Close()
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500 on store error, got %d", res.StatusCode)
	}
}

func TestHandleListDocuments_SearchQuery(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	// doc "a": title contains "Kompendium" — query "kompend" should match
	resA := doDoc(t, ts, "POST", "/api/v1/documents",
		`{"type":"free","path":"a","title":"Kompendium","body":"x"}`)
	_ = resA.Body.Close()
	if resA.StatusCode != http.StatusCreated {
		t.Fatalf("create A: want 201, got %d", resA.StatusCode)
	}

	// doc "b": no match
	resB := doDoc(t, ts, "POST", "/api/v1/documents",
		`{"type":"free","path":"b","title":"Other","body":"y"}`)
	_ = resB.Body.Close()
	if resB.StatusCode != http.StatusCreated {
		t.Fatalf("create B: want 201, got %d", resB.StatusCode)
	}

	res := doDoc(t, ts, "GET", "/api/v1/documents?q=kompend", "")
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var hits []domain.SearchHit
	if err := json.NewDecoder(res.Body).Decode(&hits); err != nil {
		t.Fatalf("decode hits: %v", err)
	}
	if len(hits) != 1 || hits[0].Path != "a" {
		t.Fatalf("got %#v, want [a]", hits)
	}
	if !strings.Contains(hits[0].Snippet, domain.HighlightStart) {
		t.Fatalf("missing snippet markers: %q", hits[0].Snippet)
	}
}

func TestHandleListDocuments_NoQueryUnchanged(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	resA := doDoc(t, ts, "POST", "/api/v1/documents",
		`{"type":"free","path":"noq-a","title":"A","body":""}`)
	_ = resA.Body.Close()
	if resA.StatusCode != http.StatusCreated {
		t.Fatalf("create A: want 201, got %d", resA.StatusCode)
	}

	res := doDoc(t, ts, "GET", "/api/v1/documents", "")
	defer func() { _ = res.Body.Close() }()

	var list []domain.Document
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 || list[0].Path != "noq-a" {
		t.Fatalf("plain list broke: %#v", list)
	}
}

func TestImportDocument_HappyDailyHistorical(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	body := `{"type":"daily","path":"daily/2026-04-28","title":"2026-04-28","body":"# 2026-04-28\n","date":"2026-04-28T00:00:00Z"}`
	res := doDoc(t, ts, "POST", "/api/v1/documents/import", body)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", res.StatusCode)
	}
}

func TestImportDocument_BadType(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	res := doDoc(t, ts, "POST", "/api/v1/documents/import", `{"type":"bogus","path":"x","title":"T","body":"B"}`)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
}

func TestImportDocument_DuplicatePath(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	body := `{"type":"free","path":"notes/dup","title":"T","body":"B"}`
	res1 := doDoc(t, ts, "POST", "/api/v1/documents/import", body)
	_ = res1.Body.Close()
	res := doDoc(t, ts, "POST", "/api/v1/documents/import", body)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", res.StatusCode)
	}
}

func TestBacklinksEndpoint(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	destRes := doDoc(t, ts, "POST", "/api/v1/documents", `{"type":"free","path":"dest","title":"Dest","body":""}`)
	defer func() { _ = destRes.Body.Close() }()
	var dest domain.Document
	_ = json.NewDecoder(destRes.Body).Decode(&dest)

	doDoc(t, ts, "POST", "/api/v1/documents", `{"type":"free","path":"src","title":"Src","body":"[[dest]]"}`)

	res := doDoc(t, ts, "GET", "/api/v1/documents/"+dest.ID+"/backlinks", "")
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var refs []domain.BacklinkRef
	_ = json.NewDecoder(res.Body).Decode(&refs)
	if len(refs) != 1 || refs[0].Path != "src" {
		t.Fatalf("backlinks = %v, want [src]", refs)
	}

	res404 := doDoc(t, ts, "GET", "/api/v1/documents/nope/backlinks", "")
	_ = res404.Body.Close()
	if res404.StatusCode != 404 {
		t.Fatalf("missing doc status = %d, want 404", res404.StatusCode)
	}
}

func TestHandleCreateDocument_TagsParam(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	res := doDoc(t, ts, "POST", "/api/v1/documents",
		`{"type":"free","path":"tp","title":"T","body":"pure","tags":["go","tui"]}`)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("want 201, got %d", res.StatusCode)
	}
	var doc domain.Document
	_ = json.NewDecoder(res.Body).Decode(&doc)
	if len(doc.Tags) != 2 {
		t.Fatalf("want 2 tags, got %+v", doc.Tags)
	}
}

func TestHandlePinDocument_HappyPath(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	createRes := doDoc(t, ts, "POST", "/api/v1/documents", `{"type":"free","path":"pin-test","title":"PinMe","body":""}`)
	defer func() { _ = createRes.Body.Close() }()
	var created domain.Document
	_ = json.NewDecoder(createRes.Body).Decode(&created)

	res := doDoc(t, ts, "POST", "/api/v1/documents/"+created.ID+"/pin", `{"pinned":true}`)
	_ = res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("want 204, got %d", res.StatusCode)
	}

	// Verify the document is pinned.
	getRes := doDoc(t, ts, "GET", "/api/v1/documents/"+created.ID, "")
	defer func() { _ = getRes.Body.Close() }()
	var doc domain.Document
	_ = json.NewDecoder(getRes.Body).Decode(&doc)
	if !doc.Pinned {
		t.Errorf("want pinned=true, got %v", doc.Pinned)
	}
}

func TestHandlePinDocument_NotFound(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	res := doDoc(t, ts, "POST", "/api/v1/documents/no-such-id/pin", `{"pinned":true}`)
	_ = res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", res.StatusCode)
	}
}

func TestHandlePinDocument_PublishesSSE(t *testing.T) {
	srv, bus := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	// Create a document to pin.
	createRes := doDoc(t, ts, "POST", "/api/v1/documents", `{"type":"free","path":"pin-sse-test","title":"SSEPin","body":""}`)
	defer func() { _ = createRes.Body.Close() }()
	var created domain.Document
	_ = json.NewDecoder(createRes.Body).Decode(&created)

	// Subscribe to bus events for user "id-1" (FakeIDGen produces "id-1").
	ch, cancel := bus.Subscribe("id-1")
	defer cancel()

	// Drain any events from the create above.
	for {
		select {
		case <-ch:
		default:
			goto drained
		}
	}
drained:

	// Issue the pin request.
	res := doDoc(t, ts, "POST", "/api/v1/documents/"+created.ID+"/pin", `{"pinned":true}`)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("want 204, got %d", res.StatusCode)
	}

	// Assert document.updated event was published.
	select {
	case ev := <-ch:
		if ev.Type != domain.EventDocumentUpdated {
			t.Errorf("want event type %q, got %q", domain.EventDocumentUpdated, ev.Type)
		}
		if id, _ := ev.Data["id"].(string); id != created.ID {
			t.Errorf("want event id %q, got %q", created.ID, id)
		}
	default:
		t.Error("want document.updated SSE event after pin, got none")
	}
}
