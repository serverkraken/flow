package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestResolveEngagement(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	binds := testutil.NewFakeProjectBindingStore()
	now := time.Now()
	eng, _ := domain.NewNode("eng", "o", "Privat", "privat", now)
	eng.Kind = domain.KindEngagement
	_, _ = nodes.Create(ctx, eng)
	repo, _ := domain.NewNode("repo", "o", "flow", "flow", now)
	repo.Kind = domain.KindRepo
	repo.ParentID = sp("eng")
	_, _ = nodes.Create(ctx, repo)
	_, _ = binds.Upsert(ctx, domain.ProjectBinding{ID: "b", OwnerID: "o", NodeID: "repo", Kind: domain.BindingRemote, RemoteSlug: "github.com/serverkraken/flow", CreatedAt: now, UpdatedAt: now})

	uc := usecase.ResolveEngagement{
		Resolve: usecase.ResolveNode{Bindings: binds, Nodes: nodes},
		Nodes:   nodes,
	}
	got, ok, err := uc.Execute(ctx, "o", "github.com/serverkraken/flow", "m", "/x")
	if err != nil || !ok || got.ID != "eng" {
		t.Fatalf("got %+v ok=%v err=%v", got, ok, err)
	}
	// Unresolved context → ok=false, no error.
	if _, ok, err := uc.Execute(ctx, "o", "github.com/none/none", "m", "/x"); ok || err != nil {
		t.Fatalf("unresolved: ok=%v err=%v", ok, err)
	}
}

func sp(s string) *string { return &s }
