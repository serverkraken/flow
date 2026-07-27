package httpserver_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

// TestCreateNode_DuplicateSiblingSlug409 — POSTing a second node with a slug that
// another child of the same parent already uses returns 409, not 500.
func TestCreateNode_DuplicateSiblingSlug409(t *testing.T) {
	do, ns, owner := newNodesSrv(t)
	seedNode(t, ns, owner, "eng1", domain.KindEngagement, nil)

	res := do("POST", "/api/v1/nodes", `{"name":"API","slug":"api","kind":"vorhaben","parentId":"eng1"}`)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("first create status %d, want 201", res.StatusCode)
	}

	res = do("POST", "/api/v1/nodes", `{"name":"API","slug":"api","kind":"vorhaben","parentId":"eng1"}`)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate sibling slug status %d, want 409", res.StatusCode)
	}
}

// TestMoveNode_DuplicateSiblingSlug409 — moving a node into a parent that already
// has a child with the same slug returns 409.
func TestMoveNode_DuplicateSiblingSlug409(t *testing.T) {
	do, ns, owner := newNodesSrv(t)
	seedNode(t, ns, owner, "eng1", domain.KindEngagement, nil)
	seedNode(t, ns, owner, "vorA", domain.KindVorhaben, ptr("eng1"))
	seedNode(t, ns, owner, "vorB", domain.KindVorhaben, ptr("eng1"))
	// Two repos both slugged "dup", one under vorA, one under vorB.
	if _, err := ns.Create(context.Background(), domain.Node{
		ID: "r1", OwnerID: owner, Kind: domain.KindRepo, ParentID: ptr("vorA"),
		Name: "dup", Slug: "dup", Status: domain.NodeActive,
	}); err != nil {
		t.Fatalf("seed r1: %v", err)
	}
	if _, err := ns.Create(context.Background(), domain.Node{
		ID: "r2", OwnerID: owner, Kind: domain.KindRepo, ParentID: ptr("vorB"),
		Name: "dup", Slug: "dup", Status: domain.NodeActive,
	}); err != nil {
		t.Fatalf("seed r2: %v", err)
	}

	// Moving r2 under vorA collides with r1's slug.
	res := do("POST", "/api/v1/nodes/r2/move", `{"parentId":"vorA"}`)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("move into colliding-slug parent status %d, want 409", res.StatusCode)
	}
}
