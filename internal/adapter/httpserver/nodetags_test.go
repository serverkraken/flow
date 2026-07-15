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

func TestNodeTags_SetThenGet(t *testing.T) {
	t.Parallel()
	srv, _ := newDocServer(t)
	tags := testutil.NewFakeTagStore()
	nodes := testutil.NewFakeNodeStore()
	if _, err := nodes.Create(context.Background(), domain.Node{ID: "n1", OwnerID: "id-1", Kind: domain.KindEngagement, Name: "Node", Slug: "node", Status: domain.NodeActive}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	srv.NodeTags = usecase.NodeTags{Nodes: nodes, Tags: tags}

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	primeUser(t, ts.URL)

	// PUT tags on node "n1"
	res := doDoc(t, ts, "PUT", "/api/v1/nodes/n1/tags", `{"tags":["infra","terraform"]}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT want 200, got %d", res.StatusCode)
	}
	_ = res.Body.Close()

	res = doDoc(t, ts, "GET", "/api/v1/nodes/n1/tags", "")
	defer func() { _ = res.Body.Close() }()
	var got []domain.Tag
	_ = json.NewDecoder(res.Body).Decode(&got)
	if len(got) != 2 {
		t.Fatalf("want 2 node tags, got %+v", got)
	}
}

func TestNodeTags_GetEmpty(t *testing.T) {
	t.Parallel()
	srv, _ := newDocServer(t)
	tags := testutil.NewFakeTagStore()
	nodes := testutil.NewFakeNodeStore()
	srv.NodeTags = usecase.NodeTags{Nodes: nodes, Tags: tags}

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	primeUser(t, ts.URL)

	res := doDoc(t, ts, "GET", "/api/v1/nodes/n-unknown/tags", "")
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("GET unknown want 404, got %d", res.StatusCode)
	}
}

func TestNodeTags_SetForeignNodeReturnsNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := newDocServer(t)
	tags := testutil.NewFakeTagStore()
	nodes := testutil.NewFakeNodeStore()
	if _, err := nodes.Create(context.Background(), domain.Node{ID: "foreign", OwnerID: "other", Kind: domain.KindEngagement, Name: "Foreign", Slug: "foreign", Status: domain.NodeActive}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	srv.NodeTags = usecase.NodeTags{Nodes: nodes, Tags: tags}

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	primeUser(t, ts.URL)

	res := doDoc(t, ts, "PUT", "/api/v1/nodes/foreign/tags", `{"tags":["secret"]}`)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("PUT foreign want 404, got %d", res.StatusCode)
	}
	got, err := tags.TagsFor(context.Background(), "id-1", domain.TaggableNode, "foreign")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("tags persisted for foreign node: %+v", got)
	}
}
