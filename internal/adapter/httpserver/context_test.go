package httpserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestHandleGetContext_UnresolvedReturns200Global(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	primeUser(t, ts.URL)

	res := doDoc(t, ts, "GET", "/api/v1/context?remote=does-not-exist", "")
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("unresolved context must be 200, got %d", res.StatusCode)
	}
	var cc usecase.ComposedContext
	if err := json.NewDecoder(res.Body).Decode(&cc); err != nil {
		t.Fatal(err)
	}
	if !cc.Resolution.Unresolved {
		t.Errorf("want Unresolved=true for unknown repo")
	}
}

func TestHandlePutContextActive_UnboundReturns409(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	primeUser(t, ts.URL)

	// remote slug "unbound-repo" has no binding → must return 409
	res := doDoc(t, ts, "PUT", "/api/v1/context/active", `{"remote":"unbound-repo","body":"some state"}`)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("want 409 for unbound repo, got %d", res.StatusCode)
	}
}

func TestHandlePutContextActive_HappyPath(t *testing.T) {
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	binds := testutil.NewFakeProjectBindingStore()
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()

	eng, _ := nodes.Create(ctx, domain.Node{ID: "E2", OwnerID: "id-1", Kind: domain.KindEngagement, Name: "Work", Slug: "work"})
	leaf, _ := nodes.Create(ctx, domain.Node{ID: "L2", OwnerID: "id-1", Kind: domain.KindRepo, Name: "myrepo", Slug: "myrepo", ParentID: &eng.ID, OriginSlug: "myrepo"})
	_ = binds.BindRemote(ctx, "id-1", "myrepo", leaf.ID)

	srv, _ := newDocServer(t)
	// Override the context store fakes to include our seeded binding+node.
	srv.SetActiveContext = usecase.SetActiveContext{
		Resolve: usecase.ResolveNode{Bindings: binds, Nodes: nodes},
		Nodes:   nodes, Docs: docs, Tags: tags,
	}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	primeUser(t, ts.URL)

	res := doDoc(t, ts, "PUT", "/api/v1/context/active", `{"remote":"myrepo","title":"Where I was","body":"working on the B2 task"}`)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", res.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if id, ok := out["id"].(string); !ok || id == "" {
		t.Fatalf("want non-empty id in response, got %+v", out)
	}
}
