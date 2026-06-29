package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestSetActiveContext_UpsertsAtLeaf(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	binds := testutil.NewFakeProjectBindingStore()
	eng, _ := nodes.Create(ctx, domain.Node{ID: "E", OwnerID: "u1", Kind: domain.KindEngagement, Name: "Privat", Slug: "privat"})
	leaf, _ := nodes.Create(ctx, domain.Node{ID: "L", OwnerID: "u1", Kind: domain.KindRepo, Name: "flow", Slug: "flow", ParentID: &eng.ID, OriginSlug: "flow"})
	_ = binds.BindRemote(ctx, "u1", "flow", leaf.ID)

	uc := usecase.SetActiveContext{Resolve: usecase.ResolveNode{Bindings: binds, Nodes: nodes}, Nodes: nodes, Docs: docs, Tags: tags}
	id, _, err := uc.Execute(ctx, "u1", usecase.ContextResolveInput{RemoteSlug: "flow"}, "", "where I was", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := docs.Get(ctx, "u1", id)
	if got.Path != usecase.ActiveContextPath || got.NodeID == nil || *got.NodeID != "L" || got.Type != domain.DocActiveContext {
		t.Fatalf("activeContext not written at leaf as activecontext: %+v", got)
	}
	// idempotent: a second flush reuses the same row.
	id2, _, _ := uc.Execute(ctx, "u1", usecase.ContextResolveInput{RemoteSlug: "flow"}, "", "v2", nil)
	if id2 != id {
		t.Fatalf("flush must reuse the row: %q vs %q", id, id2)
	}
}

func TestSetActiveContext_UnresolvedErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	uc := usecase.SetActiveContext{Resolve: usecase.ResolveNode{Bindings: testutil.NewFakeProjectBindingStore(), Nodes: nodes}, Nodes: nodes, Docs: testutil.NewFakeDocumentStore(), Tags: testutil.NewFakeTagStore()}
	_, _, err := uc.Execute(ctx, "u1", usecase.ContextResolveInput{RemoteSlug: "nope", Cwd: "/x"}, "", "body", nil)
	if !errors.Is(err, usecase.ErrContextUnresolved) {
		t.Fatalf("want ErrContextUnresolved, got %v", err)
	}
}
