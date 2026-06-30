package testutil_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
)

func TestFakeNodeStore_Subtree_OwnerIsolation(t *testing.T) {
	ns := testutil.NewFakeNodeStore()
	ctx := context.Background()

	eng := "eng"
	vor := "vor"
	for _, n := range []domain.Node{
		{ID: "eng", OwnerID: "u1", ParentID: nil, Kind: domain.KindEngagement, Name: "eng", Slug: "eng", Status: domain.NodeActive},
		{ID: "vor", OwnerID: "u1", ParentID: &eng, Kind: domain.KindVorhaben, Name: "vor", Slug: "vor", Status: domain.NodeActive},
		{ID: "repo", OwnerID: "u1", ParentID: &vor, Kind: domain.KindRepo, Name: "repo", Slug: "repo", Status: domain.NodeActive},
	} {
		if _, err := ns.Create(ctx, n); err != nil {
			t.Fatalf("seed %s: %v", n.ID, err)
		}
	}

	// Intruder: owned by u2 but ParentID points at u1's "eng" node.
	if _, err := ns.Create(ctx, domain.Node{
		ID: "intruder", OwnerID: "u2", ParentID: &eng,
		Kind: domain.KindRepo, Name: "intruder", Slug: "intruder", Status: domain.NodeActive,
	}); err != nil {
		t.Fatalf("seed intruder: %v", err)
	}

	// u1's subtree must NOT include the intruder owned by u2.
	got, err := ns.Subtree(ctx, "u1", "eng")
	if err != nil {
		t.Fatalf("Subtree(u1, eng): %v", err)
	}
	for _, n := range got {
		if n.ID == "intruder" {
			t.Fatalf("Subtree(u1, eng) must not include intruder owned by u2")
		}
	}
	if len(got) != 3 {
		t.Fatalf("Subtree(u1, eng): want 3 nodes, got %d", len(got))
	}

	// u2 does not own "eng" → Subtree must return empty/nil.
	got2, err := ns.Subtree(ctx, "u2", "eng")
	if err != nil {
		t.Fatalf("Subtree(u2, eng): %v", err)
	}
	if len(got2) != 0 {
		t.Fatalf("Subtree(u2, eng): want empty (u2 doesn't own eng), got %v", got2)
	}
}

func TestFakeNodeStore_Subtree(t *testing.T) {
	ns := testutil.NewFakeNodeStore()
	ctx := context.Background()
	mk := func(id string, parent *string, k domain.NodeKind) {
		if _, err := ns.Create(ctx, domain.Node{ID: id, OwnerID: "u1", ParentID: parent, Kind: k, Name: id, Slug: id, Status: domain.NodeActive}); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	eng := "eng"
	vor := "vor"
	mk("eng", nil, domain.KindEngagement)
	mk("vor", &eng, domain.KindVorhaben)
	mk("repo", &vor, domain.KindRepo)
	mk("other", nil, domain.KindEngagement)
	got, err := ns.Subtree(ctx, "u1", "eng")
	if err != nil {
		t.Fatalf("subtree: %v", err)
	}
	ids := map[string]bool{}
	for _, n := range got {
		ids[n.ID] = true
	}
	if len(ids) != 3 || !ids["eng"] || !ids["vor"] || !ids["repo"] || ids["other"] {
		t.Fatalf("want {eng,vor,repo}, got %v", ids)
	}
	leaf, _ := ns.Subtree(ctx, "u1", "repo")
	if len(leaf) != 1 || leaf[0].ID != "repo" {
		t.Fatalf("want just repo, got %v", leaf)
	}
}
