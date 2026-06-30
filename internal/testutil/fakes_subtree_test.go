package testutil_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
)

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
