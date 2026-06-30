package httpserver_test

import (
	"context"
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
