package httpserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// machineDocServer wires the smallest real server that can serve
// POST /api/v1/documents for a machine token: the machine mapping, an owner
// already in the user store, and the create-document use case over in-memory
// fakes. It returns the server, the owner, and the document store so the test
// can read the PERSISTED row rather than only the response body.
func machineDocServer(t *testing.T) (*httpserver.Server, domain.User, *testutil.FakeDocumentStore) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	bus := sse.NewBus()
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	aggregate := testutil.NewFakeDocumentAggregateStore(docs, tags)
	nodes := testutil.NewFakeNodeStore()
	users := testutil.NewFakeUserStore()

	owner, err := domain.NewUser("owner-id", "owner-sub", "soenne", "s@x.de", "Soenne")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := users.UpsertBySub(context.Background(), owner); err != nil {
		t.Fatal(err)
	}

	srv := &httpserver.Server{
		Verifier: testutil.FakeVerifier{ID: ports.Identity{Subject: "machine-sub", Machine: true}},
		Users:    users,
		// Ensure is wired on purpose even though the machine path must never
		// reach it: leaving it nil would make "EnsureUser did not run" pass for
		// the wrong reason (a panic), instead of because delegation took over.
		Ensure: usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Machines: map[string]httpserver.MachineAccount{
			"machine-sub": {OwnerSub: "owner-sub", Label: "wartung-agent"},
		},
		Bus:            bus,
		Emitter:        sse.NewEmitter(bus, &fakeActivityStore{}, ids, clk),
		Clock:          clk,
		CreateDocument: usecase.CreateDocument{Docs: docs, Aggregate: aggregate, Nodes: nodes, Tags: tags, IDs: ids, Clock: clk},
		GetDocument:    usecase.GetDocument{Docs: docs},
	}
	return srv, owner, docs
}

// TestMachineTokenCreatesDocumentForOwnerEndToEnd is the design-spec §11
// end-to-end assertion: a machine token on POST /api/v1/documents answers 201,
// and the STORED document carries the delegated OWNER's owner_id plus machine
// provenance (updated_by_kind = agent, updated_by_ref = wartung-agent).
//
// Two adjacent tests already cover the halves compositionally — machineauth_test.go
// pins the context authMachineOK builds, and usecase/create_document_test.go
// pins that an agent context produces this provenance — but neither runs a
// machine token through the real router into a store. A change in the seam
// between them (a handler scoping by the token subject instead of the delegated
// user, or a route losing its authMachineOK wrapper) would leave both green
// while breaking exactly the delegation this feature exists for.
func TestMachineTokenCreatesDocumentForOwnerEndToEnd(t *testing.T) {
	srv, owner, docs := machineDocServer(t)

	body := `{"type":"free","path":"notes/runs/run-1","title":"Wartungslauf 2026-08-01","body":"all green"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer machine-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %q)", rec.Code, rec.Body.String())
	}

	var got domain.Document
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v (body %q)", err, rec.Body.String())
	}
	if got.ID == "" {
		t.Fatalf("response carries no document id: %q", rec.Body.String())
	}

	// Read the PERSISTED row scoped by the OWNER. A Get under the owner's id
	// succeeding is itself the delegation assertion: flow's stores are
	// owner-scoped, so a document written into any other tenant would not be
	// found here at all.
	stored, err := docs.Get(context.Background(), owner.ID, got.ID)
	if err != nil {
		t.Fatalf("document not found in the owner's tenant: %v", err)
	}
	if stored.OwnerID != owner.ID {
		t.Fatalf("owner_id = %q, want the delegated owner %q", stored.OwnerID, owner.ID)
	}
	if stored.UpdatedByKind != string(actor.Agent) {
		t.Fatalf("updated_by_kind = %q, want %q", stored.UpdatedByKind, actor.Agent)
	}
	if stored.UpdatedByRef != "wartung-agent" {
		t.Fatalf("updated_by_ref = %q, want %q", stored.UpdatedByRef, "wartung-agent")
	}
	// The owner's own display name in the ref would mean the audit trail credits
	// the human for a write the machine made — the failure mode §7 exists to
	// prevent, and one a Kind check alone would not catch.
	if stored.UpdatedByRef == owner.DisplayName {
		t.Fatalf("updated_by_ref credits the human owner %q, not the machine", owner.DisplayName)
	}

	// Still no second tenant: the machine subject must not have gained a user row.
	if _, err := srv.Users.GetBySub(context.Background(), "machine-sub"); err == nil {
		t.Fatal("a user record was created for the machine subject")
	}
}
