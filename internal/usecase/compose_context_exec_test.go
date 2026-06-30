package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestComposeContext_Execute_ResolvesChainAndGates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	binds := testutil.NewFakeProjectBindingStore()

	// hierarchy: engagement E ← repo L; bind remote slug "flow" → L.
	eng, _ := nodes.Create(ctx, domain.Node{ID: "E", OwnerID: "u1", Kind: domain.KindEngagement, Name: "Privat", Slug: "privat"})
	leaf, _ := nodes.Create(ctx, domain.Node{ID: "L", OwnerID: "u1", Kind: domain.KindRepo, Name: "flow", Slug: "flow", ParentID: &eng.ID, OriginSlug: "flow"})
	// FakeProjectBindingStore has no BindRemote helper; use Upsert directly.
	_, _ = binds.Upsert(ctx, domain.ProjectBinding{Kind: domain.BindingRemote, OwnerID: "u1", RemoteSlug: "flow", NodeID: leaf.ID})

	t0 := time.Now()
	_, _ = docs.Create(ctx, domain.Document{ID: "ac", OwnerID: "u1", NodeID: &leaf.ID, Type: domain.DocActiveContext, Path: usecase.ActiveContextPath, Body: "where", UpdatedAt: t0})
	_, _ = docs.Create(ctx, domain.Document{ID: "gmem", OwnerID: "u1", NodeID: nil, Type: domain.DocMemory, Path: "g", Body: "global", UpdatedAt: t0})
	// tag both the leaf node and the global memory with "go" so the D7 gate lets gmem cross.
	_, _ = tags.SetTags(ctx, "u1", domain.TaggableNode, leaf.ID, []string{"go"})
	_, _ = tags.SetTags(ctx, "u1", domain.TaggableDocument, "gmem", []string{"go"})

	uc := usecase.ComposeContext{
		Resolve: usecase.ResolveNode{Bindings: binds, Nodes: nodes},
		Nodes:   nodes, Docs: docs, Tags: tags,
	}
	got, err := uc.Execute(ctx, "u1", usecase.ContextResolveInput{RemoteSlug: "flow"}, 100000)
	if err != nil {
		t.Fatal(err)
	}
	if got.Resolution.Unresolved {
		t.Fatalf("should resolve via remote binding")
	}
	if got.ActiveContext == nil || got.ActiveContext.ID != "ac" {
		t.Errorf("activeContext missing: %+v", got.ActiveContext)
	}
	if len(got.Memories["global"]) != 1 || got.Memories["global"][0].ID != "gmem" {
		t.Errorf("D7 tag-gate should admit gmem: %+v", got.Memories["global"])
	}
}

func TestComposeContext_Execute_UnresolvedGivesGlobalOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	binds := testutil.NewFakeProjectBindingStore()
	_, _ = docs.Create(ctx, domain.Document{ID: "gi", OwnerID: "u1", NodeID: nil, Type: domain.DocInstruction, Path: "claude", Body: "rule"})

	uc := usecase.ComposeContext{Resolve: usecase.ResolveNode{Bindings: binds, Nodes: nodes}, Nodes: nodes, Docs: docs, Tags: tags}
	got, err := uc.Execute(ctx, "u1", usecase.ContextResolveInput{RemoteSlug: "unknown", Cwd: "/tmp/x"}, 100000)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Resolution.Unresolved {
		t.Fatalf("unknown repo must be Unresolved")
	}
	if len(got.Instructions) != 1 {
		t.Errorf("global instruction should still load when unresolved: %+v", got.Instructions)
	}
}

func TestComposeContext_ExcludesArchived(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	binds := testutil.NewFakeProjectBindingStore()

	// hierarchy: engagement E ← repo L; bind remote slug "flow" → L.
	eng, _ := nodes.Create(ctx, domain.Node{ID: "E", OwnerID: "u1", Kind: domain.KindEngagement, Name: "Privat", Slug: "privat"})
	leaf, _ := nodes.Create(ctx, domain.Node{ID: "L", OwnerID: "u1", Kind: domain.KindRepo, Name: "flow", Slug: "flow", ParentID: &eng.ID, OriginSlug: "flow"})
	_, _ = binds.Upsert(ctx, domain.ProjectBinding{Kind: domain.BindingRemote, OwnerID: "u1", RemoteSlug: "flow", NodeID: leaf.ID})

	t0 := time.Now()
	// Non-archived memory in leaf node → must appear.
	_, _ = docs.Create(ctx, domain.Document{ID: "live", OwnerID: "u1", NodeID: &leaf.ID, Type: domain.DocMemory, Path: "live", Body: "Live", UpdatedAt: t0})
	// Archived memory in leaf node → must NOT appear even though node is in chain.
	_, _ = docs.Create(ctx, domain.Document{ID: "arch", OwnerID: "u1", NodeID: &leaf.ID, Type: domain.DocMemory, Path: "old", Body: "Old", UpdatedAt: t0})
	_ = docs.SetArchived(ctx, "u1", "arch", true)

	uc := usecase.ComposeContext{
		Resolve: usecase.ResolveNode{Bindings: binds, Nodes: nodes},
		Nodes:   nodes, Docs: docs, Tags: tags,
	}
	got, err := uc.Execute(ctx, "u1", usecase.ContextResolveInput{RemoteSlug: "flow"}, 100000)
	if err != nil {
		t.Fatal(err)
	}
	for _, mems := range got.Memories {
		for _, m := range mems {
			if m.ID == "arch" {
				t.Fatalf("archived memory must not appear in compose output")
			}
		}
	}
	// non-archived memory must be present.
	found := false
	for _, mems := range got.Memories {
		for _, m := range mems {
			if m.ID == "live" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("non-archived memory must appear in compose output, got %+v", got.Memories)
	}
}

// TestComposeContext_Execute_NodeOverride exercises the resolveLeaf NodeOverride
// branch which resolves the leaf node by slug directly without using the binding
// registry.
func TestComposeContext_Execute_NodeOverride(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	binds := testutil.NewFakeProjectBindingStore()

	leaf, _ := nodes.Create(ctx, domain.Node{ID: "L2", OwnerID: "u1", Kind: domain.KindRepo, Name: "alpha", Slug: "alpha"})
	_, _ = docs.Create(ctx, domain.Document{ID: "ac2", OwnerID: "u1", NodeID: &leaf.ID, Type: domain.DocActiveContext, Path: usecase.ActiveContextPath, Body: "alpha ctx"})

	uc := usecase.ComposeContext{
		Resolve: usecase.ResolveNode{Bindings: binds, Nodes: nodes},
		Nodes:   nodes, Docs: docs, Tags: tags,
	}
	// Use NodeOverride to resolve the leaf by slug.
	got, err := uc.Execute(ctx, "u1", usecase.ContextResolveInput{NodeOverride: "alpha"}, 100000)
	if err != nil {
		t.Fatal(err)
	}
	if got.Resolution.Unresolved {
		t.Fatalf("should resolve via NodeOverride")
	}
	if got.ActiveContext == nil || got.ActiveContext.ID != "ac2" {
		t.Errorf("activeContext missing with NodeOverride: %+v", got.ActiveContext)
	}
}

// TestComposeContext_Execute_NodeOverrideNotFound exercises the resolveLeaf path
// when NodeOverride slug matches no node (should resolve as Unresolved).
func TestComposeContext_Execute_NodeOverrideNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	binds := testutil.NewFakeProjectBindingStore()

	uc := usecase.ComposeContext{
		Resolve: usecase.ResolveNode{Bindings: binds, Nodes: nodes},
		Nodes:   nodes, Docs: docs, Tags: tags,
	}
	got, err := uc.Execute(ctx, "u1", usecase.ContextResolveInput{NodeOverride: "nonexistent"}, 100000)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Resolution.Unresolved {
		t.Fatalf("should be unresolved when NodeOverride slug not found")
	}
}
