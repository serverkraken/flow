package usecase_test

import (
	"context"
	"errors"
	"strings"
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

// TestSetActiveContext_NodeOverrideNotFound exercises the path where NodeOverride
// resolves by slug scan but the slug doesn't exist, returning ErrContextUnresolved.
func TestSetActiveContext_NodeOverrideNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	// No nodes created; NodeOverride slug will not match.
	uc := usecase.SetActiveContext{
		Resolve: usecase.ResolveNode{Bindings: testutil.NewFakeProjectBindingStore(), Nodes: nodes},
		Nodes:   nodes, Docs: testutil.NewFakeDocumentStore(), Tags: testutil.NewFakeTagStore(),
	}
	_, _, err := uc.Execute(ctx, "u1", usecase.ContextResolveInput{NodeOverride: "nonexistent"}, "", "body", nil)
	if !errors.Is(err, usecase.ErrContextUnresolved) {
		t.Fatalf("want ErrContextUnresolved, got %v", err)
	}
}

// TestSetActiveContext_NodeOverride exercises the path where NodeOverride
// finds a node by slug scan and upserts the active context there.
func TestSetActiveContext_NodeOverride(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	_, _ = nodes.Create(ctx, domain.Node{ID: "N1", OwnerID: "u1", Kind: domain.KindRepo, Name: "alpha", Slug: "alpha"})
	uc := usecase.SetActiveContext{
		Resolve: usecase.ResolveNode{Bindings: testutil.NewFakeProjectBindingStore(), Nodes: nodes},
		Nodes:   nodes, Docs: docs, Tags: tags,
	}
	id, _, err := uc.Execute(ctx, "u1", usecase.ContextResolveInput{NodeOverride: "alpha"}, "", "body", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := docs.Get(ctx, "u1", id)
	if got.NodeID == nil || *got.NodeID != "N1" {
		t.Fatalf("expected doc at node N1, got %+v", got)
	}
}

func TestSetActiveContext_NodeOverrideRejectsAmbiguousSlug(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	workID, privateID := "work", "private"
	for _, n := range []domain.Node{
		{ID: "work", OwnerID: "u1", Kind: domain.KindEngagement, Name: "Work", Slug: "work"},
		{ID: "private", OwnerID: "u1", Kind: domain.KindEngagement, Name: "Private", Slug: "private"},
		{ID: "work-api", OwnerID: "u1", Kind: domain.KindRepo, ParentID: &workID, Name: "API", Slug: "api"},
		{ID: "private-api", OwnerID: "u1", Kind: domain.KindRepo, ParentID: &privateID, Name: "API", Slug: "api"},
	} {
		if _, err := nodes.Create(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	uc := usecase.SetActiveContext{Nodes: nodes, Docs: testutil.NewFakeDocumentStore(), Tags: testutil.NewFakeTagStore()}
	_, _, err := uc.Execute(ctx, "u1", usecase.ContextResolveInput{NodeOverride: "api"}, "", "must not write", nil)
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous NodeOverride error = %v, want fail-closed ambiguity", err)
	}
}
