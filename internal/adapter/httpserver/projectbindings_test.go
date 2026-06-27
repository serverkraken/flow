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

type bindingsSrv struct {
	ts  *httptest.Server
	ps  *testutil.FakeNodeStore
	ids *testutil.FakeIDGen
	clk testutil.FakeClock
	do  func(method, path, body string) *http.Response
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
	users := testutil.NewFakeUserStore()

	srv := &httpserver.Server{
		Verifier:         testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:           usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:              sse.NewBus(),
		Clock:            clk,
		CreateNode:       usecase.CreateNode{Nodes: ps, IDs: ids, Clock: clk},
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
	return &bindingsSrv{ts: ts, ps: ps, ids: ids, clk: clk, do: do}
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
	res := do("POST", "/api/v1/nodes", `{"name":"X"}`)
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
	_, do := newBindingsSrv(t)

	// Create project + bind.
	res := do("POST", "/api/v1/nodes", `{"name":"Tool"}`)
	var proj domain.Node
	_ = json.NewDecoder(res.Body).Decode(&proj)
	_ = res.Body.Close()

	res = do("PUT", "/api/v1/nodes/"+proj.ID+"/bindings", `{"kind":"remote","remoteSlug":"github.com/x/tool"}`)
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
