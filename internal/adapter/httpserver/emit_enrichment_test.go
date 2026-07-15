package httpserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// recordingActivityStore records every Append call so tests can assert on the
// stored activity entries.
type recordingActivityStore struct {
	mu    sync.Mutex
	items []domain.ActivityEntry
}

func (r *recordingActivityStore) Append(_ context.Context, e domain.ActivityEntry) error {
	r.mu.Lock()
	r.items = append(r.items, e)
	r.mu.Unlock()
	return nil
}

func (r *recordingActivityStore) ListPage(_ context.Context, _ string, _ []string, _ *string, _, _ int) ([]domain.ActivityEntry, int, error) {
	return nil, 0, nil
}

func (r *recordingActivityStore) DistinctActors(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (r *recordingActivityStore) snapshot() []domain.ActivityEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]domain.ActivityEntry, len(r.items))
	copy(cp, r.items)
	return cp
}

var _ ports.ActivityStore = (*recordingActivityStore)(nil)

// TestCreateDocumentRecordsActivity verifies that a successful POST /api/v1/documents
// results in an activity entry with Kind="document.created", Label=title, ActorKind="human".
func TestCreateDocumentRecordsActivity(t *testing.T) {
	clk := testutil.FakeClock{T: time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	bus := sse.NewBus()
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	actStore := &recordingActivityStore{}
	emitter := sse.NewEmitter(bus, actStore, ids, clk)

	srv := &httpserver.Server{
		Verifier:       testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:         usecase.EnsureUser{Users: testutil.NewFakeUserStore(), IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:            bus,
		Emitter:        emitter,
		Clock:          clk,
		CreateDocument: usecase.CreateDocument{Docs: docs, Tags: tags, IDs: ids, Clock: clk},
		GetDocument:    usecase.GetDocument{Docs: docs},
	}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	// First request primes EnsureUser (user id = "id-1" via FakeIDGen).
	body := `{"type":"free","path":"notes/hello","title":"Hello World","body":"content"}`
	res := doDoc(t, ts, "POST", "/api/v1/documents", body)
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != 201 {
		t.Fatalf("want 201, got %d", res.StatusCode)
	}

	entries := actStore.snapshot()
	if len(entries) == 0 {
		t.Fatal("want at least 1 activity entry, got 0")
	}
	e := entries[0]
	if e.Kind != "document.created" {
		t.Errorf("Kind: want %q, got %q", "document.created", e.Kind)
	}
	if e.Label == nil || *e.Label != "Hello World" {
		t.Errorf("Label: want %q, got %v", "Hello World", e.Label)
	}
	if e.ActorKind != "human" {
		t.Errorf("ActorKind: want %q, got %q", "human", e.ActorKind)
	}
}

func TestSetActiveContextRecordsEffectiveDefaultTitle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 15, 14, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	bus := sse.NewBus()
	actStore := &recordingActivityStore{}
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	nodes := testutil.NewFakeNodeStore()
	bindings := testutil.NewFakeProjectBindingStore()
	eng, _ := nodes.Create(ctx, domain.Node{ID: "eng", OwnerID: "id-1", Kind: domain.KindEngagement, Name: "Work", Slug: "work"})
	repo, _ := nodes.Create(ctx, domain.Node{ID: "repo", OwnerID: "id-1", Kind: domain.KindRepo, Name: "Flow", Slug: "flow", ParentID: &eng.ID})
	if err := bindings.BindRemote(ctx, "id-1", "github-com-serverkraken-flow", repo.ID); err != nil {
		t.Fatal(err)
	}
	srv := &httpserver.Server{
		Verifier: testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:   usecase.EnsureUser{Users: testutil.NewFakeUserStore(), IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:      bus,
		Emitter:  sse.NewEmitter(bus, actStore, ids, clk),
		SetActiveContext: usecase.SetActiveContext{
			Resolve: usecase.ResolveNode{Bindings: bindings, Nodes: nodes},
			Nodes:   nodes, Docs: docs, Tags: tags,
		},
		GetDocument: usecase.GetDocument{Docs: docs},
	}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	res := doDoc(t, ts, http.MethodPut, "/api/v1/context/active", `{"remote":"github-com-serverkraken-flow","body":"state"}`)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	entries := actStore.snapshot()
	if len(entries) != 1 {
		t.Fatalf("activity entries = %d, want 1", len(entries))
	}
	if entries[0].Label == nil || *entries[0].Label != "Active Context" {
		t.Fatalf("activity label = %v, want Active Context", entries[0].Label)
	}
}

func TestPinDocumentRecordsDocumentTitle(t *testing.T) {
	t.Parallel()
	srv, bus := newDocServer(t)
	actStore := &recordingActivityStore{}
	srv.Emitter = sse.NewEmitter(bus, actStore, &testutil.FakeIDGen{}, testutil.FakeClock{T: time.Date(2026, 7, 15, 14, 0, 0, 0, time.UTC)})
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	createdRes := doDoc(t, ts, http.MethodPost, "/api/v1/documents", `{"type":"free","path":"activity-pin","title":"Pinned title","body":""}`)
	var doc domain.Document
	if err := json.NewDecoder(createdRes.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	_ = createdRes.Body.Close()
	before := len(actStore.snapshot())
	pinRes := doDoc(t, ts, http.MethodPost, "/api/v1/documents/"+doc.ID+"/pin", `{"pinned":true}`)
	defer func() { _ = pinRes.Body.Close() }()
	if pinRes.StatusCode != http.StatusNoContent {
		t.Fatalf("pin status = %d, want 204", pinRes.StatusCode)
	}
	entries := actStore.snapshot()
	if len(entries) != before+1 {
		t.Fatalf("activity entries after pin = %d, want %d", len(entries), before+1)
	}
	last := entries[len(entries)-1]
	if last.Label == nil || *last.Label != doc.Title {
		t.Fatalf("pin activity label = %v, want %q", last.Label, doc.Title)
	}
}

// TestCreateNodeRecordsActivity verifies that a successful POST /api/v1/nodes
// results in an activity entry with Kind="node.created", Label=node name, ActorKind="human".
func TestCreateNodeRecordsActivity(t *testing.T) {
	clk := testutil.FakeClock{T: time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	bus := sse.NewBus()
	nodes := testutil.NewFakeNodeStore()
	actStore := &recordingActivityStore{}
	emitter := sse.NewEmitter(bus, actStore, ids, clk)

	srv := &httpserver.Server{
		Verifier:   testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:     usecase.EnsureUser{Users: testutil.NewFakeUserStore(), IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:        bus,
		Emitter:    emitter,
		Clock:      clk,
		CreateNode: usecase.CreateNode{Nodes: nodes, IDs: ids, Clock: clk},
		ListNodes:  usecase.ListNodes{Nodes: nodes},
	}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	res := doDoc(t, ts, "POST", "/api/v1/nodes", `{"name":"MyEngagement","kind":"engagement"}`)
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != 201 {
		t.Fatalf("want 201, got %d", res.StatusCode)
	}

	entries := actStore.snapshot()
	if len(entries) == 0 {
		t.Fatal("want at least 1 activity entry, got 0")
	}
	e := entries[0]
	if e.Kind != "node.created" {
		t.Errorf("Kind: want %q, got %q", "node.created", e.Kind)
	}
	if e.Label == nil || *e.Label != "MyEngagement" {
		t.Errorf("Label: want %q, got %v", "MyEngagement", e.Label)
	}
	if e.ActorKind != "human" {
		t.Errorf("ActorKind: want %q, got %q", "human", e.ActorKind)
	}
}
