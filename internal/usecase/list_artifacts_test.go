package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// TestListArtifacts_AncestorChainNotSubtree seeds engagement→vorhaben→repo and
// puts one artifact on the engagement (ancestor of repo) and one on a sibling
// repo (not an ancestor). Listing from the leaf repo must see the ancestor's
// artifact but NOT the sibling's — reach is the ancestor chain, not the subtree.
func TestListArtifacts_AncestorChainNotSubtree(t *testing.T) {
	ctx := context.Background()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)}
	ns := testutil.NewFakeNodeStore()

	eng, err := domain.NewNode("eng", "u1", "acme", "acme", clk.Now())
	if err != nil {
		t.Fatal(err)
	}
	eng.Kind = domain.KindEngagement
	if _, err := ns.Create(ctx, eng); err != nil {
		t.Fatal(err)
	}

	vorID := "vor"
	vor, err := domain.NewNode(vorID, "u1", "project-x", "project-x", clk.Now())
	if err != nil {
		t.Fatal(err)
	}
	vor.Kind = domain.KindVorhaben
	vor.ParentID = &eng.ID
	if _, err := ns.Create(ctx, vor); err != nil {
		t.Fatal(err)
	}

	repo, err := domain.NewNode("repo-a", "u1", "flow", "flow", clk.Now())
	if err != nil {
		t.Fatal(err)
	}
	repo.Kind = domain.KindRepo
	repo.ParentID = &vor.ID
	if _, err := ns.Create(ctx, repo); err != nil {
		t.Fatal(err)
	}

	sibling, err := domain.NewNode("repo-b", "u1", "other", "other", clk.Now())
	if err != nil {
		t.Fatal(err)
	}
	sibling.Kind = domain.KindRepo
	sibling.ParentID = &vor.ID
	if _, err := ns.Create(ctx, sibling); err != nil {
		t.Fatal(err)
	}

	as := testutil.NewFakeArtifactStore()
	if err := as.Put(ctx, domain.Artifact{
		OwnerID: "u1", NodeID: eng.ID, Slug: "logo", Name: "Logo.png", Mime: "image/png",
	}); err != nil {
		t.Fatal(err)
	}
	if err := as.Put(ctx, domain.Artifact{
		OwnerID: "u1", NodeID: "repo-b", Slug: "notes", Name: "Notes.pdf", Mime: "application/pdf",
	}); err != nil {
		t.Fatal(err)
	}

	uc := usecase.ListArtifacts{Nodes: ns, Artifacts: as}
	got, err := uc.Execute(ctx, "u1", "repo-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 artifact reachable from repo-a's ancestor chain, got %d: %+v", len(got), got)
	}
	if got[0].Slug != "logo" {
		t.Errorf("slug = %q, want logo (from the ancestor engagement)", got[0].Slug)
	}
}

// --- Free (node-less) artifacts (free-artifacts Task 3) --------------------

// TestListArtifacts_FreeOnly covers Execute(owner, "") — the free-only
// branch, which must go straight to ListFree and skip Ancestors entirely.
func TestListArtifacts_FreeOnly(t *testing.T) {
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore() // no nodes at all — Ancestors must not be reached
	as := testutil.NewFakeArtifactStore()
	if err := as.Put(ctx, domain.Artifact{OwnerID: "u1", NodeID: "", Slug: "brand", Name: "Brand.png", Mime: "image/png"}); err != nil {
		t.Fatal(err)
	}
	// A node-scoped artifact for the same owner must never leak into the free-only result.
	if err := as.Put(ctx, domain.Artifact{OwnerID: "u1", NodeID: "some-node", Slug: "other", Name: "Other.png", Mime: "image/png"}); err != nil {
		t.Fatal(err)
	}

	uc := usecase.ListArtifacts{Nodes: ns, Artifacts: as}
	got, err := uc.Execute(ctx, "u1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Slug != "brand" {
		t.Fatalf("want only the free artifact 'brand', got %+v", got)
	}
}

// TestListArtifacts_NodePlusFree_BothAppearFreeLast covers Execute(owner,
// node) with both a node artifact and a free one: the list must contain
// BOTH, with the free artifact appended last (root-lowest priority, E1).
func TestListArtifacts_NodePlusFree_BothAppearFreeLast(t *testing.T) {
	ctx := context.Background()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)}
	ns := testutil.NewFakeNodeStore()
	n, err := domain.NewNode("n1", "u1", "flow", "flow", clk.Now())
	if err != nil {
		t.Fatal(err)
	}
	n.Kind = domain.KindEngagement
	if _, err := ns.Create(ctx, n); err != nil {
		t.Fatal(err)
	}

	as := testutil.NewFakeArtifactStore()
	if err := as.Put(ctx, domain.Artifact{OwnerID: "u1", NodeID: "n1", Slug: "logo", Name: "Node-Logo.png", Mime: "image/png"}); err != nil {
		t.Fatal(err)
	}
	if err := as.Put(ctx, domain.Artifact{OwnerID: "u1", NodeID: "", Slug: "brand", Name: "Brand.png", Mime: "image/png"}); err != nil {
		t.Fatal(err)
	}

	uc := usecase.ListArtifacts{Nodes: ns, Artifacts: as}
	got, err := uc.Execute(ctx, "u1", "n1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want both the node and the free artifact, got %d: %+v", len(got), got)
	}
	if got[len(got)-1].Slug != "brand" || got[len(got)-1].NodeID != "" {
		t.Fatalf("free artifact must be last, got %+v", got)
	}
}

// TestListArtifacts_BogusNode_ReturnsEmptyNotFreeLibrary is the KRITISCH
// codex #2 regression guard: Nodes.Ancestors returns (nil, nil) — not an
// error — for an unknown/foreign node id. Without the len(chain)==0 guard,
// an empty chain would fall through to appending ListFree(owner), leaking
// the caller's entire free library for a bogus/foreign node id. A valid node
// always has len(chain) >= 1 (it contains itself), so this must return nil.
func TestListArtifacts_BogusNode_ReturnsEmptyNotFreeLibrary(t *testing.T) {
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	as := testutil.NewFakeArtifactStore()
	if err := as.Put(ctx, domain.Artifact{OwnerID: "u1", NodeID: "", Slug: "brand", Name: "Brand.png", Mime: "image/png"}); err != nil {
		t.Fatal(err)
	}

	uc := usecase.ListArtifacts{Nodes: ns, Artifacts: as}
	got, err := uc.Execute(ctx, "u1", "bogus-or-foreign-id")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("bogus/foreign node must yield [], NOT the free library, got %+v", got)
	}
}
