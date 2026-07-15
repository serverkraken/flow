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
	aggregate := testutil.NewFakeDocumentAggregateStore(docs, tags)

	eng, _ := nodes.Create(ctx, domain.Node{ID: "E2", OwnerID: "id-1", Kind: domain.KindEngagement, Name: "Work", Slug: "work"})
	leaf, _ := nodes.Create(ctx, domain.Node{ID: "L2", OwnerID: "id-1", Kind: domain.KindRepo, Name: "myrepo", Slug: "myrepo", ParentID: &eng.ID, OriginSlug: "myrepo"})
	_ = binds.BindRemote(ctx, "id-1", "myrepo", leaf.ID)

	srv, _ := newDocServer(t)
	// Override the context store fakes to include our seeded binding+node.
	srv.SetActiveContext = usecase.SetActiveContext{
		Resolve: usecase.ResolveNode{Bindings: binds, Nodes: nodes},
		Nodes:   nodes, Docs: docs, Aggregate: aggregate, Tags: tags,
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

func TestHandlePutContextActive_AggregateFailureRollsBackAndEmitsNoEvent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	binds := testutil.NewFakeProjectBindingStore()
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	aggregate := testutil.NewFakeDocumentAggregateStore(docs, tags)
	aggregate.FailStage = "tags"

	eng, _ := nodes.Create(ctx, domain.Node{ID: "E-fail", OwnerID: "id-1", Kind: domain.KindEngagement, Name: "Work", Slug: "work-fail"})
	leaf, _ := nodes.Create(ctx, domain.Node{ID: "L-fail", OwnerID: "id-1", Kind: domain.KindRepo, Name: "repo-fail", Slug: "repo-fail", ParentID: &eng.ID, OriginSlug: "repo-fail"})
	_ = binds.BindRemote(ctx, "id-1", "repo-fail", leaf.ID)

	srv, bus := newDocServer(t)
	srv.SetActiveContext = usecase.SetActiveContext{
		Resolve: usecase.ResolveNode{Bindings: binds, Nodes: nodes},
		Nodes:   nodes, Docs: docs, Aggregate: aggregate, Tags: tags,
	}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	primeUser(t, ts.URL)
	ch, cancel := bus.Subscribe("id-1")
	defer cancel()

	res := doDoc(t, ts, http.MethodPut, "/api/v1/context/active", `{"remote":"repo-fail","body":"[[next]]","tags":["handoff"]}`)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status=%d, want 500", res.StatusCode)
	}
	if got, _ := docs.List(ctx, "id-1", nil); len(got) != 0 {
		t.Fatalf("partial active context survived: %+v", got)
	}
	assertNoDocumentEvent(t, ch)
}

func TestHandleReorderContext_StampsPrioritiesAndEmitsOneEvent(t *testing.T) {
	srv, bus := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	createA := doDoc(t, ts, "POST", "/api/v1/documents", `{"type":"free","path":"reorder-a","title":"A","body":""}`)
	var a domain.Document
	_ = json.NewDecoder(createA.Body).Decode(&a)
	_ = createA.Body.Close()

	createB := doDoc(t, ts, "POST", "/api/v1/documents", `{"type":"free","path":"reorder-b","title":"B","body":""}`)
	var b domain.Document
	_ = json.NewDecoder(createB.Body).Decode(&b)
	_ = createB.Body.Close()

	ch, cancel := bus.Subscribe("id-1")
	defer cancel()
	// Drain create events.
	for {
		select {
		case <-ch:
		default:
			goto drained
		}
	}
drained:

	body := `{"ids":["` + b.ID + `","` + a.ID + `"]}`
	res := doDoc(t, ts, "POST", "/api/v1/context/reorder", body)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", res.StatusCode)
	}

	getA := doDoc(t, ts, "GET", "/api/v1/documents/"+a.ID, "")
	var gotA domain.Document
	_ = json.NewDecoder(getA.Body).Decode(&gotA)
	_ = getA.Body.Close()
	if gotA.Priority != 1 {
		t.Errorf("a.Priority = %d, want 1", gotA.Priority)
	}

	getB := doDoc(t, ts, "GET", "/api/v1/documents/"+b.ID, "")
	var gotB domain.Document
	_ = json.NewDecoder(getB.Body).Decode(&gotB)
	_ = getB.Body.Close()
	if gotB.Priority != 2 {
		t.Errorf("b.Priority = %d, want 2", gotB.Priority)
	}

	// Exactly one document.updated event for the whole reorder batch (other
	// event types, e.g. activity.logged, may also be on the bus — only
	// document.updated occurrences are counted).
	updatedCount := 0
	for {
		select {
		case ev := <-ch:
			if ev.Type != domain.EventDocumentUpdated {
				continue
			}
			updatedCount++
			if n, ok := ev.Data["reordered"].(int); !ok || n != 2 {
				t.Errorf("want reordered=2, got %+v", ev.Data)
			}
		default:
			goto counted
		}
	}
counted:
	if updatedCount != 1 {
		t.Fatalf("want exactly one document.updated event, got %d", updatedCount)
	}
}

func TestHandleReorderContext_ForeignDocReturns404(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	primeUser(t, ts.URL)

	// Seed a document owned by a different user directly in the fake store.
	ctx := context.Background()
	other, err := srv.CreateDocument.Docs.Create(ctx, domain.Document{ID: "doc-other", OwnerID: "someone-else", Title: "Foreign", Path: "foreign.md"})
	if err != nil {
		t.Fatalf("seed foreign doc: %v", err)
	}

	res := doDoc(t, ts, "POST", "/api/v1/context/reorder", `{"ids":["`+other.ID+`"]}`)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", res.StatusCode)
	}
}
