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

type bindingsSrv struct {
	ts      *httptest.Server
	ps      *testutil.FakeNodeStore
	bs      *testutil.FakeProjectBindingStore
	bound   *fakeBoundNodeAggregate
	emitter *captureEmitter
	ids     *testutil.FakeIDGen
	clk     testutil.FakeClock
	do      func(method, path, body string) *http.Response
}

type fakeBoundNodeAggregate struct {
	nodes    *testutil.FakeNodeStore
	bindings *testutil.FakeProjectBindingStore
	err      error
}

func (s *fakeBoundNodeAggregate) CreateBoundAggregate(ctx context.Context, n domain.Node, _ ports.NodeAggregateChanges, binding domain.ProjectBinding) (domain.Node, domain.ProjectBinding, error) {
	if s.err != nil {
		return domain.Node{}, domain.ProjectBinding{}, s.err
	}
	created, err := s.nodes.Create(ctx, n)
	if err != nil {
		return domain.Node{}, domain.ProjectBinding{}, err
	}
	bound, err := s.bindings.Upsert(ctx, binding)
	return created, bound, err
}

func newBindingsSrv(t *testing.T) (*httptest.Server, func(method, path, body string) *http.Response) {
	t.Helper()
	s := newBindingsSrvFull(t)
	return s.ts, s.do
}

func newBindingsSrvFull(t *testing.T) *bindingsSrv {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	ps := testutil.NewFakeNodeStore()
	bs := testutil.NewFakeProjectBindingStore()
	bound := &fakeBoundNodeAggregate{nodes: ps, bindings: bs}
	users := testutil.NewFakeUserStore()

	bus := sse.NewBus()
	srv := &httpserver.Server{
		Verifier:         testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:           usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:              bus,
		Emitter:          &captureEmitter{},
		Clock:            clk,
		CreateNode:       usecase.CreateNode{Nodes: ps, IDs: ids, Clock: clk},
		CreateBoundNode:  usecase.CreateBoundNode{Nodes: ps, Aggregate: bound, IDs: ids, Clock: clk},
		ListNodes:        usecase.ListNodes{Nodes: ps},
		BindNode:         usecase.BindNode{Bindings: bs, Nodes: ps, IDs: ids, Clock: clk},
		UnbindNode:       usecase.UnbindNode{Bindings: bs},
		ResolveNode:      usecase.ResolveNode{Bindings: bs, Nodes: ps},
		ListNodeBindings: usecase.ListNodeBindings{Bindings: bs},
	}

	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	do := func(method, path, body string) *http.Response {
		req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer x")
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	return &bindingsSrv{
		ts: ts, ps: ps, bs: bs, bound: bound, emitter: srv.Emitter.(*captureEmitter),
		ids: ids, clk: clk, do: do,
	}
}

func TestProjectBindings_CreateBoundNodeCommitsBeforeEmitting(t *testing.T) {
	srv := newBindingsSrvFull(t)
	ctx := t.Context()
	res := srv.do("GET", "/api/v1/nodes", "")
	_ = res.Body.Close()
	parentID := srv.ids.NewID()
	_, err := srv.ps.Create(ctx, domain.Node{
		ID: parentID, OwnerID: "id-1", Name: "Work", Slug: "work",
		Kind: domain.KindEngagement, Status: domain.NodeActive,
	})
	if err != nil {
		t.Fatal(err)
	}

	res = srv.do("POST", "/api/v1/nodes/create-bound", `{
		"node":{"name":"Flow","kind":"repo","parentId":"`+parentID+`"},
		"binding":{"kind":"remote","remoteSlug":"git@github.com:serverkraken/flow.git"}
	}`)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusCreated {
		body := make([]byte, 512)
		n, _ := res.Body.Read(body)
		t.Fatalf("create-bound: status %d body=%s", res.StatusCode, body[:n])
	}
	var result usecase.CreateBoundNodeResult
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Node.OriginSlug != "github.com/serverkraken/flow" || result.Binding.NodeID != result.Node.ID {
		t.Fatalf("result=%+v", result)
	}
	stored, err := srv.ps.Get(ctx, "id-1", result.Node.ID)
	if err != nil || stored.ID != result.Node.ID {
		t.Fatalf("stored node=%+v err=%v", stored, err)
	}
	bindings, err := srv.bs.List(ctx, "id-1")
	if err != nil || len(bindings) != 1 || bindings[0].NodeID != result.Node.ID {
		t.Fatalf("stored bindings=%+v err=%v", bindings, err)
	}
	if srv.emitter.count() != 1 {
		t.Fatalf("events=%d, want 1 after commit", srv.emitter.count())
	}
}

func TestProjectBindings_CreateBoundNodeFailureDoesNotEmitOrLeaveNode(t *testing.T) {
	srv := newBindingsSrvFull(t)
	ctx := t.Context()
	res := srv.do("GET", "/api/v1/nodes", "")
	_ = res.Body.Close()
	parentID := srv.ids.NewID()
	_, _ = srv.ps.Create(ctx, domain.Node{
		ID: parentID, OwnerID: "id-1", Name: "Work", Slug: "work",
		Kind: domain.KindEngagement, Status: domain.NodeActive,
	})
	srv.bound.err = errors.New("binding write failed")

	res = srv.do("POST", "/api/v1/nodes/create-bound", `{
		"node":{"name":"Flow","kind":"repo","parentId":"`+parentID+`"},
		"binding":{"kind":"remote","remoteSlug":"github.com/serverkraken/flow"}
	}`)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("create-bound failure: status %d", res.StatusCode)
	}
	if srv.emitter.count() != 0 {
		t.Fatalf("events=%d after failed commit", srv.emitter.count())
	}
	nodes, err := srv.ps.List(ctx, "id-1")
	if err != nil || len(nodes) != 1 || nodes[0].ID != parentID {
		t.Fatalf("partial node survived: %+v err=%v", nodes, err)
	}
	bindings, err := srv.bs.List(ctx, "id-1")
	if err != nil || len(bindings) != 0 {
		t.Fatalf("partial binding survived: %+v err=%v", bindings, err)
	}
}

// TestProjectBindings_BindAndResolveAndList is the primary TDD scenario:
// PUT a remote binding → resolve by slug → list all bindings.
func TestProjectBindings_BindAndResolveAndList(t *testing.T) {
	srv := newBindingsSrvFull(t)
	do := srv.do

	// Trigger first request to create the user ("sub-1" → user.ID = "id-1" via FakeIDGen).
	// Then create a KindRepo node owned by that user (remote binding requires KindRepo).
	ctx := t.Context()
	_ = do("GET", "/api/v1/nodes", "") // seeds the user; user.ID = first ids.NewID()
	ownerID := "id-1"
	engID := srv.ids.NewID()
	repoID := srv.ids.NewID()
	engParent := (*string)(nil)
	_, _ = srv.ps.Create(ctx, domain.Node{ID: engID, OwnerID: ownerID, Name: "Privat", Slug: "privat", Kind: domain.KindEngagement, ParentID: engParent, Status: domain.NodeActive})
	_, _ = srv.ps.Create(ctx, domain.Node{ID: repoID, OwnerID: ownerID, Name: "Flow", Slug: "flow", Kind: domain.KindRepo, ParentID: &engID, Status: domain.NodeActive})
	proj := domain.Node{ID: repoID}

	// PUT a remote binding for the project.
	bindBody := `{"kind":"remote","remoteSlug":"github.com/serverkraken/flow"}`
	res := do("PUT", "/api/v1/nodes/"+proj.ID+"/bindings", bindBody)
	if res.StatusCode != http.StatusOK {
		body := make([]byte, 512)
		n, _ := res.Body.Read(body)
		t.Fatalf("bind: status %d body=%s", res.StatusCode, body[:n])
	}
	var binding domain.ProjectBinding
	_ = json.NewDecoder(res.Body).Decode(&binding)
	_ = res.Body.Close()
	if binding.RemoteSlug != "github.com/serverkraken/flow" {
		t.Fatalf("binding RemoteSlug = %q", binding.RemoteSlug)
	}
	if binding.NodeID != proj.ID {
		t.Fatalf("binding NodeID = %q, want %q", binding.NodeID, proj.ID)
	}

	// GET /resolve?slug=<remote> → 200 with the project.
	res = do("GET", "/api/v1/nodes/resolve?slug=github.com%2Fserverkraken%2Fflow", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("resolve: status %d", res.StatusCode)
	}
	var resolved domain.Node
	_ = json.NewDecoder(res.Body).Decode(&resolved)
	_ = res.Body.Close()
	if resolved.ID != proj.ID {
		t.Fatalf("resolved project ID = %q, want %q", resolved.ID, proj.ID)
	}

	// GET /api/v1/nodes/bindings → 200, lists the binding.
	res = do("GET", "/api/v1/nodes/bindings", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list bindings: status %d", res.StatusCode)
	}
	var all []domain.ProjectBinding
	_ = json.NewDecoder(res.Body).Decode(&all)
	_ = res.Body.Close()
	if len(all) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(all))
	}
	if all[0].RemoteSlug != "github.com/serverkraken/flow" {
		t.Fatalf("listed binding RemoteSlug = %q", all[0].RemoteSlug)
	}
}

func TestProjectBindings_ResolveByUpstreamGitWithoutBinding(t *testing.T) {
	srv := newBindingsSrvFull(t)
	ctx := t.Context()

	// Seed the authenticated user, then a repo with only its canonical upstream.
	res := srv.do("GET", "/api/v1/nodes", "")
	_ = res.Body.Close()
	ownerID := "id-1"
	engID := srv.ids.NewID()
	repoID := srv.ids.NewID()
	_, _ = srv.ps.Create(ctx, domain.Node{
		ID: engID, OwnerID: ownerID, Name: "Privat", Slug: "privat",
		Kind: domain.KindEngagement, Status: domain.NodeActive,
	})
	_, _ = srv.ps.Create(ctx, domain.Node{
		ID: repoID, OwnerID: ownerID, Name: "Flow", Slug: "flow",
		Kind: domain.KindRepo, ParentID: &engID, Status: domain.NodeActive,
		UpstreamGit: "git@github.com:serverkraken/flow.git",
	})

	res = srv.do("GET", "/api/v1/nodes/resolve?slug=github.com%2Fserverkraken%2Fflow", "")
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		t.Fatalf("resolve upstreamGit: status %d", res.StatusCode)
	}
	var resolved domain.Node
	if err := json.NewDecoder(res.Body).Decode(&resolved); err != nil {
		_ = res.Body.Close()
		t.Fatalf("decode resolved node: %v", err)
	}
	_ = res.Body.Close()
	if resolved.ID != repoID {
		t.Fatalf("resolved node ID = %q, want %q", resolved.ID, repoID)
	}
}

// TestProjectBindings_ResolveUnknown ensures 404 when no binding matches.
func TestProjectBindings_ResolveUnknown(t *testing.T) {
	_, do := newBindingsSrv(t)

	res := do("GET", "/api/v1/nodes/resolve?slug=github.com%2Funknown%2Frepo", "")
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("resolve unknown: want 404, got %d", res.StatusCode)
	}
	_ = res.Body.Close()
}

// TestProjectBindings_BindUnknownProject ensures 404 when project doesn't exist.
func TestProjectBindings_BindUnknownProject(t *testing.T) {
	_, do := newBindingsSrv(t)

	res := do("PUT", "/api/v1/nodes/no-such-id/bindings", `{"kind":"remote","remoteSlug":"github.com/x/y"}`)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("bind unknown project: want 404, got %d", res.StatusCode)
	}
	_ = res.Body.Close()
}

// TestProjectBindings_BindInvalidBody ensures 400 on malformed JSON.
func TestProjectBindings_BindInvalidBody(t *testing.T) {
	_, do := newBindingsSrv(t)

	// First create a project so we know the {id} is valid.
	res := do("POST", "/api/v1/nodes", `{"name":"X","kind":"engagement"}`)
	var proj domain.Node
	_ = json.NewDecoder(res.Body).Decode(&proj)
	_ = res.Body.Close()

	res = do("PUT", "/api/v1/nodes/"+proj.ID+"/bindings", `not-json`)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad body: want 400, got %d", res.StatusCode)
	}
	_ = res.Body.Close()
}

// TestProjectBindings_ListByProject verifies GET /api/v1/nodes/{id}/bindings.
func TestProjectBindings_ListByProject(t *testing.T) {
	srv := newBindingsSrvFull(t)
	do := srv.do

	// Trigger first request to create the user then seed a KindRepo node for binding.
	ctx := t.Context()
	_ = do("GET", "/api/v1/nodes", "")
	ownerID := "id-1"
	engID := srv.ids.NewID()
	repoID := srv.ids.NewID()
	_, _ = srv.ps.Create(ctx, domain.Node{ID: engID, OwnerID: ownerID, Name: "Privat", Slug: "privat", Kind: domain.KindEngagement, Status: domain.NodeActive})
	_, _ = srv.ps.Create(ctx, domain.Node{ID: repoID, OwnerID: ownerID, Name: "Kompendium", Slug: "kompendium", Kind: domain.KindRepo, ParentID: &engID, Status: domain.NodeActive})
	proj := domain.Node{ID: repoID}

	res := do("PUT", "/api/v1/nodes/"+proj.ID+"/bindings", `{"kind":"remote","remoteSlug":"github.com/sk/kompendium"}`)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("bind: %d", res.StatusCode)
	}

	res = do("GET", "/api/v1/nodes/"+proj.ID+"/bindings", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list by project: %d", res.StatusCode)
	}
	var bs []domain.ProjectBinding
	_ = json.NewDecoder(res.Body).Decode(&bs)
	_ = res.Body.Close()
	if len(bs) != 1 || bs[0].NodeID != proj.ID {
		t.Fatalf("list by project: got %+v", bs)
	}
}

// TestProjectBindings_DeleteUnbind verifies DELETE /api/v1/nodes/bindings.
func TestProjectBindings_DeleteUnbind(t *testing.T) {
	srv := newBindingsSrvFull(t)
	do := srv.do

	// Trigger first request to create the user then seed a KindRepo node for binding.
	ctx := t.Context()
	_ = do("GET", "/api/v1/nodes", "") // seeds the user; user.ID = first ids.NewID() = "id-1"
	ownerID := "id-1"
	engID := srv.ids.NewID()
	repoID := srv.ids.NewID()
	_, _ = srv.ps.Create(ctx, domain.Node{ID: engID, OwnerID: ownerID, Name: "Privat", Slug: "privat", Kind: domain.KindEngagement, Status: domain.NodeActive})
	_, _ = srv.ps.Create(ctx, domain.Node{ID: repoID, OwnerID: ownerID, Name: "Tool", Slug: "tool", Kind: domain.KindRepo, ParentID: &engID, Status: domain.NodeActive})

	// PUT a remote binding — must succeed now that the node is KindRepo.
	res := do("PUT", "/api/v1/nodes/"+repoID+"/bindings", `{"kind":"remote","remoteSlug":"github.com/x/tool"}`)
	if res.StatusCode != http.StatusOK {
		body := make([]byte, 512)
		n, _ := res.Body.Read(body)
		t.Fatalf("bind: want 200, got %d body=%s", res.StatusCode, body[:n])
	}
	_ = res.Body.Close()

	// Delete the binding.
	res = do("DELETE", "/api/v1/nodes/bindings?kind=remote&slug=github.com%2Fx%2Ftool", "")
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete binding: want 204, got %d", res.StatusCode)
	}
	_ = res.Body.Close()

	// Verify it's gone: list should be empty.
	res = do("GET", "/api/v1/nodes/bindings", "")
	var all []domain.ProjectBinding
	_ = json.NewDecoder(res.Body).Decode(&all)
	_ = res.Body.Close()
	if len(all) != 0 {
		t.Fatalf("expected 0 bindings after delete, got %d", len(all))
	}
}

// TestBindNode_InvalidTargetKind400 verifies that binding onto a non-repo/leaf node returns 400.
func TestBindNode_InvalidTargetKind400(t *testing.T) {
	do, ns, owner := newNodesSrv(t)
	seedNode(t, ns, owner, "eng1", domain.KindEngagement, nil) // remote bind onto an engagement → invalid (must be repo)
	res := do("PUT", "/api/v1/nodes/eng1/bindings", `{"kind":"remote","remoteSlug":"github.com/x/y"}`)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid bind target status %d, want 400", res.StatusCode)
	}
}

// TestProjectBindings_RouteNotShadowed verifies GET /nodes/resolve hits the
// resolve handler and not a {id} wildcard handler.
func TestProjectBindings_RouteNotShadowed(t *testing.T) {
	_, do := newBindingsSrv(t)

	// Without any bindings, resolve must return 404 (not 405 or 400 from a {id} route).
	res := do("GET", "/api/v1/nodes/resolve?slug=x", "")
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("route-shadow check: want 404, got %d", res.StatusCode)
	}
	_ = res.Body.Close()
}

func TestBindNode_EmitsNodeUpdated(t *testing.T) {
	s := newBindingsSrvFull(t)
	// newBindingsSrvFull authenticates via usecase.EnsureUser backed by the
	// shared FakeIDGen; the authenticated user's ID is the first ID that
	// generator hands out ("id-1"), the same convention every other test in
	// this file relies on (see TestProjectBindings_CreateBoundNodeCommitsBeforeEmitting).
	if _, err := s.ps.Create(context.Background(), domain.Node{
		ID: "n1", OwnerID: "id-1", Name: "Flow", Slug: "flow", Kind: domain.KindRepo,
	}); err != nil {
		t.Fatalf("seed node: %v", err)
	}

	res := s.do(http.MethodPut, "/api/v1/nodes/n1/bindings",
		`{"kind":"remote","remoteSlug":"github.com/serverkraken/flow"}`)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}

	events := s.emitter.all()
	if len(events) != 1 {
		t.Fatalf("emitted %d event(s), want exactly 1: %+v", len(events), events)
	}
	if events[0].Type != domain.EventNodeUpdated {
		t.Errorf("event type = %q, want %q", events[0].Type, domain.EventNodeUpdated)
	}
	if events[0].UserID != "id-1" {
		t.Errorf("event UserID = %q, want %q", events[0].UserID, "id-1")
	}
	if got := events[0].Data["id"]; got != "n1" {
		t.Errorf("event Data[id] = %v, want %q", got, "n1")
	}
}

func TestUnbindNode_EmitsNodeUpdated(t *testing.T) {
	s := newBindingsSrvFull(t)
	if _, err := s.ps.Create(context.Background(), domain.Node{
		ID: "n1", OwnerID: "id-1", Name: "Flow", Slug: "flow", Kind: domain.KindRepo,
	}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	bindRes := s.do(http.MethodPut, "/api/v1/nodes/n1/bindings",
		`{"kind":"remote","remoteSlug":"github.com/serverkraken/flow"}`)
	_ = bindRes.Body.Close()

	res := s.do(http.MethodDelete,
		"/api/v1/nodes/bindings?kind=remote&slug=github.com/serverkraken/flow", "")
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", res.StatusCode)
	}

	events := s.emitter.all()
	if len(events) != 2 {
		t.Fatalf("emitted %d event(s), want 2 (bind + unbind): %+v", len(events), events)
	}
	// The unbind handler is addressed by binding target, not by node — UnbindNode.
	// Execute returns only an error (internal/usecase/unbind_node.go), so the node
	// id is genuinely unavailable here. Consumers trigger on the event TYPE and
	// refetch, so an id-less node.updated still drives the live update.
	if events[1].Type != domain.EventNodeUpdated {
		t.Errorf("unbind event type = %q, want %q", events[1].Type, domain.EventNodeUpdated)
	}
	if events[1].UserID != "id-1" {
		t.Errorf("unbind event UserID = %q, want %q", events[1].UserID, "id-1")
	}
}
