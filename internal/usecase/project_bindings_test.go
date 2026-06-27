package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestBindNode_RemoteHappyAndUnknownNode(t *testing.T) {
	ps := testutil.NewFakeNodeStore()
	bs := testutil.NewFakeProjectBindingStore()
	clk := testutil.FakeClock{T: time.Unix(0, 0)}
	ids := &testutil.FakeIDGen{}

	p, _ := ps.Create(context.Background(), domain.Node{ID: "p1", OwnerID: "u", Slug: "flow", Kind: domain.KindRepo})

	uc := usecase.BindNode{Bindings: bs, Nodes: ps, IDs: ids, Clock: clk}

	b, err := uc.Execute(context.Background(), "u", p.ID, usecase.BindKey{
		Kind:       domain.BindingRemote,
		RemoteSlug: "github.com/a/flow",
	})
	if err != nil || b.RemoteSlug != "github.com/a/flow" {
		t.Fatalf("happy path: %+v %v", b, err)
	}

	_, err = uc.Execute(context.Background(), "u", "nope", usecase.BindKey{
		Kind:       domain.BindingRemote,
		RemoteSlug: "x",
	})
	if err == nil {
		t.Fatal("unknown project must error")
	}
}

func TestBindNode_PropagatesErrNodeNotFound(t *testing.T) {
	ps := testutil.NewFakeNodeStore()
	bs := testutil.NewFakeProjectBindingStore()
	clk := testutil.FakeClock{T: time.Now()}
	ids := &testutil.FakeIDGen{}

	uc := usecase.BindNode{Bindings: bs, Nodes: ps, IDs: ids, Clock: clk}

	_, err := uc.Execute(context.Background(), "owner", "missing-id", usecase.BindKey{
		Kind:       domain.BindingRemote,
		RemoteSlug: "github.com/x/y",
	})
	if err != ports.ErrNodeNotFound {
		t.Fatalf("expected ErrNodeNotFound, got %v", err)
	}
}

func TestBindNode_UpsertCalledAndReturnsBinding(t *testing.T) {
	ps := testutil.NewFakeNodeStore()
	bs := testutil.NewFakeProjectBindingStore()
	clk := testutil.FakeClock{T: time.Unix(100, 0)}
	ids := &testutil.FakeIDGen{}

	p, _ := ps.Create(context.Background(), domain.Node{ID: "proj1", OwnerID: "alice", Slug: "myapp", Kind: domain.KindRepo})

	uc := usecase.BindNode{Bindings: bs, Nodes: ps, IDs: ids, Clock: clk}
	b, err := uc.Execute(context.Background(), "alice", p.ID, usecase.BindKey{
		Kind:       domain.BindingRemote,
		RemoteSlug: "github.com/alice/myapp",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.NodeID != "proj1" {
		t.Errorf("NodeID = %q, want proj1", b.NodeID)
	}
	if b.RemoteSlug != "github.com/alice/myapp" {
		t.Errorf("RemoteSlug = %q", b.RemoteSlug)
	}
	if b.CreatedAt != clk.T {
		t.Errorf("CreatedAt = %v, want %v", b.CreatedAt, clk.T)
	}

	// List via ListNodeBindings
	list := usecase.ListNodeBindings{Bindings: bs}
	bindings, err := list.Execute(context.Background(), "alice")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(bindings))
	}
}

func TestResolveNode_MatchingRemoteReturnsNode(t *testing.T) {
	ps := testutil.NewFakeNodeStore()
	bs := testutil.NewFakeProjectBindingStore()
	clk := testutil.FakeClock{T: time.Now()}
	ids := &testutil.FakeIDGen{}

	p, _ := ps.Create(context.Background(), domain.Node{ID: "p2", OwnerID: "bob", Slug: "svc", Kind: domain.KindRepo})

	binder := usecase.BindNode{Bindings: bs, Nodes: ps, IDs: ids, Clock: clk}
	if _, err := binder.Execute(context.Background(), "bob", p.ID, usecase.BindKey{
		Kind:       domain.BindingRemote,
		RemoteSlug: "github.com/bob/svc",
	}); err != nil {
		t.Fatalf("bind: %v", err)
	}

	resolver := usecase.ResolveNode{Bindings: bs, Nodes: ps}
	got, ok, err := resolver.Execute(context.Background(), "bob", "github.com/bob/svc", "", "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !ok {
		t.Fatal("expected match, got no match")
	}
	if got.ID != "p2" {
		t.Errorf("got project %q, want p2", got.ID)
	}
}

func TestResolveNode_NoMatchReturnsFalse(t *testing.T) {
	ps := testutil.NewFakeNodeStore()
	bs := testutil.NewFakeProjectBindingStore()

	resolver := usecase.ResolveNode{Bindings: bs, Nodes: ps}
	_, ok, err := resolver.Execute(context.Background(), "carol", "github.com/carol/nothing", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected no match")
	}
}

func TestUnbindNode_Remote(t *testing.T) {
	ps := testutil.NewFakeNodeStore()
	bs := testutil.NewFakeProjectBindingStore()
	clk := testutil.FakeClock{T: time.Now()}
	ids := &testutil.FakeIDGen{}

	p, _ := ps.Create(context.Background(), domain.Node{ID: "p3", OwnerID: "dave", Slug: "tool", Kind: domain.KindRepo})

	binder := usecase.BindNode{Bindings: bs, Nodes: ps, IDs: ids, Clock: clk}
	if _, err := binder.Execute(context.Background(), "dave", p.ID, usecase.BindKey{
		Kind:       domain.BindingRemote,
		RemoteSlug: "github.com/dave/tool",
	}); err != nil {
		t.Fatalf("bind: %v", err)
	}

	unbinder := usecase.UnbindNode{Bindings: bs}
	if err := unbinder.Execute(context.Background(), "dave", usecase.BindKey{
		Kind:       domain.BindingRemote,
		RemoteSlug: "github.com/dave/tool",
	}); err != nil {
		t.Fatalf("unbind: %v", err)
	}

	// Verify it is gone.
	list := usecase.ListNodeBindings{Bindings: bs}
	bindings, _ := list.Execute(context.Background(), "dave")
	if len(bindings) != 0 {
		t.Fatalf("expected 0 bindings after unbind, got %d", len(bindings))
	}
}

func TestListNodeBindings_Empty(t *testing.T) {
	bs := testutil.NewFakeProjectBindingStore()
	uc := usecase.ListNodeBindings{Bindings: bs}
	bindings, err := uc.Execute(context.Background(), "nobody")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bindings) != 0 {
		t.Fatalf("expected 0, got %d", len(bindings))
	}
}

func TestBindNode_TargetKind(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	binds := testutil.NewFakeProjectBindingStore()
	now := time.Now()
	mk := func(id string, kind domain.NodeKind, parent *string) {
		n, _ := domain.NewNode(id, "o", id, id, now)
		n.Kind = kind
		n.ParentID = parent
		_, _ = nodes.Create(ctx, n)
	}
	mk("eng", domain.KindEngagement, nil)
	mk("repo", domain.KindRepo, sp("eng"))
	mk("leafvor", domain.KindVorhaben, sp("eng"))
	mk("parentvor", domain.KindVorhaben, sp("eng"))
	mk("child", domain.KindRepo, sp("parentvor"))

	uc := usecase.BindNode{Bindings: binds, Nodes: nodes, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: now}}
	remote := usecase.BindKey{Kind: domain.BindingRemote, RemoteSlug: "github.com/o/r"}
	path := usecase.BindKey{Kind: domain.BindingPath, MachineID: "m", Path: "/p"}

	if _, err := uc.Execute(ctx, "o", "repo", remote); err != nil {
		t.Fatalf("remote→repo ok: %v", err)
	}
	if _, err := uc.Execute(ctx, "o", "eng", remote); !errors.Is(err, usecase.ErrInvalidBindTarget) {
		t.Fatalf("remote→engagement: want ErrInvalidBindTarget, got %v", err)
	}
	if _, err := uc.Execute(ctx, "o", "repo", path); err != nil {
		t.Fatalf("path→repo ok: %v", err)
	}
	if _, err := uc.Execute(ctx, "o", "leafvor", path); err != nil {
		t.Fatalf("path→leaf vorhaben ok: %v", err)
	}
	if _, err := uc.Execute(ctx, "o", "parentvor", path); !errors.Is(err, usecase.ErrInvalidBindTarget) {
		t.Fatalf("path→non-leaf vorhaben: want ErrInvalidBindTarget, got %v", err)
	}
}

// TestUnbindProject_Path covers the BindingPath branch of UnbindNode.Execute.
func TestUnbindNode_Path(t *testing.T) {
	ps := testutil.NewFakeNodeStore()
	bs := testutil.NewFakeProjectBindingStore()
	clk := testutil.FakeClock{T: time.Now()}
	ids := &testutil.FakeIDGen{}

	p, _ := ps.Create(context.Background(), domain.Node{ID: "p4", OwnerID: "eve", Slug: "myapp", Kind: domain.KindRepo})

	binder := usecase.BindNode{Bindings: bs, Nodes: ps, IDs: ids, Clock: clk}
	if _, err := binder.Execute(context.Background(), "eve", p.ID, usecase.BindKey{
		Kind:      domain.BindingPath,
		MachineID: "mac-2",
		Path:      "/home/eve/myapp",
	}); err != nil {
		t.Fatalf("bind: %v", err)
	}

	unbinder := usecase.UnbindNode{Bindings: bs}
	if err := unbinder.Execute(context.Background(), "eve", usecase.BindKey{
		Kind:      domain.BindingPath,
		MachineID: "mac-2",
		Path:      "/home/eve/myapp",
	}); err != nil {
		t.Fatalf("unbind path: %v", err)
	}

	// Verify the binding is gone.
	list := usecase.ListNodeBindings{Bindings: bs}
	bindings, _ := list.Execute(context.Background(), "eve")
	if len(bindings) != 0 {
		t.Fatalf("expected 0 bindings after unbind path, got %d", len(bindings))
	}
}
