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
