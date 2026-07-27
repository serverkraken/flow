package usecase_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestNodeAncestors(t *testing.T) {
	t.Parallel()
	ns := testutil.NewFakeNodeStore()
	ctx := context.Background()
	p := func(s string) *string { return &s }
	_, _ = ns.Create(ctx, domain.Node{ID: "eng", OwnerID: "u", Kind: domain.KindEngagement})
	_, _ = ns.Create(ctx, domain.Node{ID: "repo", OwnerID: "u", Kind: domain.KindRepo, ParentID: p("eng")})

	chain, err := usecase.NodeAncestors{Nodes: ns}.Execute(ctx, "u", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(chain) != 2 || chain[0].ID != "repo" || chain[1].ID != "eng" {
		t.Fatalf("chain = %+v, want [repo eng] (leaf→root)", chain)
	}
}
