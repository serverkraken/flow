package httpserver_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestListNodes_ReturnsParentAndKind(t *testing.T) {
	do, ns, owner := newNodesSrv(t)
	seedNode(t, ns, owner, "eng1", domain.KindEngagement, nil)
	seedNode(t, ns, owner, "repo1", domain.KindRepo, ptr("eng1"))

	res := do("GET", "/api/v1/nodes", "")
	var raw []map[string]any
	_ = json.NewDecoder(res.Body).Decode(&raw)
	_ = res.Body.Close()
	var repo map[string]any
	for _, n := range raw {
		if n["id"] == "repo1" {
			repo = n
		}
	}
	if repo == nil || repo["kind"] != "repo" || repo["parentId"] != "eng1" {
		t.Fatalf("flat list missing kind/parentId: %+v", repo)
	}
}

func TestNodeAncestorsRoute(t *testing.T) {
	do, ns, owner := newNodesSrv(t)
	seedNode(t, ns, owner, "eng1", domain.KindEngagement, nil)
	seedNode(t, ns, owner, "repo1", domain.KindRepo, ptr("eng1"))

	res := do("GET", "/api/v1/nodes/repo1/ancestors", "")
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		t.Fatalf("ancestors status %d, want 200", res.StatusCode)
	}
	var chain []domain.Node
	_ = json.NewDecoder(res.Body).Decode(&chain)
	_ = res.Body.Close()
	if len(chain) != 2 || chain[0].ID != "repo1" || chain[1].ID != "eng1" {
		t.Fatalf("ancestors = %+v, want [repo1 eng1]", chain)
	}
}

func TestCreateNode_WithKindAndParent(t *testing.T) {
	do, ns, owner := newNodesSrv(t)
	seedNode(t, ns, owner, "eng1", domain.KindEngagement, nil)

	res := do("POST", "/api/v1/nodes", `{"name":"flow","kind":"repo","parentId":"eng1"}`)
	if res.StatusCode != http.StatusCreated {
		_ = res.Body.Close()
		t.Fatalf("create status %d, want 201", res.StatusCode)
	}
	var n domain.Node
	_ = json.NewDecoder(res.Body).Decode(&n)
	_ = res.Body.Close()
	if n.Kind != domain.KindRepo || n.ParentID == nil || *n.ParentID != "eng1" {
		t.Fatalf("created node = %+v", n)
	}
}

func TestCreateNode_InvalidKind400(t *testing.T) {
	do, _, _ := newNodesSrv(t)
	// repo with no parent → root-is-engagement violation → ErrInvalidNode.
	res := do("POST", "/api/v1/nodes", `{"name":"x","kind":"repo"}`)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", res.StatusCode)
	}
}

func TestResolveEngagement_Route(t *testing.T) {
	do, ns, owner := newNodesSrv(t)
	seedNode(t, ns, owner, "eng1", domain.KindEngagement, nil)
	seedNode(t, ns, owner, "repo1", domain.KindRepo, ptr("eng1"))
	// bind remote → repo, then resolve-engagement by that slug → eng1.
	_ = do("PUT", "/api/v1/nodes/repo1/bindings", `{"kind":"remote","remoteSlug":"github.com/sk/flow"}`).Body.Close()

	res := do("GET", "/api/v1/nodes/resolve-engagement?slug=github.com%2Fsk%2Fflow", "")
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		t.Fatalf("resolve-engagement status %d, want 200", res.StatusCode)
	}
	var n domain.Node
	_ = json.NewDecoder(res.Body).Decode(&n)
	_ = res.Body.Close()
	if n.ID != "eng1" {
		t.Fatalf("resolved engagement = %q, want eng1", n.ID)
	}
}
