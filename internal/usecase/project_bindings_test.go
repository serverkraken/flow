package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestBindProject_RemoteHappyAndUnknownProject(t *testing.T) {
	ps := testutil.NewFakeProjectStore()
	bs := testutil.NewFakeProjectBindingStore()
	clk := testutil.FakeClock{T: time.Unix(0, 0)}
	ids := &testutil.FakeIDGen{}

	p, _ := ps.Create(context.Background(), domain.Project{ID: "p1", OwnerID: "u", Slug: "flow"})

	uc := usecase.BindProject{Bindings: bs, Projects: ps, IDs: ids, Clock: clk}

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

func TestBindProject_PropagatesErrProjectNotFound(t *testing.T) {
	ps := testutil.NewFakeProjectStore()
	bs := testutil.NewFakeProjectBindingStore()
	clk := testutil.FakeClock{T: time.Now()}
	ids := &testutil.FakeIDGen{}

	uc := usecase.BindProject{Bindings: bs, Projects: ps, IDs: ids, Clock: clk}

	_, err := uc.Execute(context.Background(), "owner", "missing-id", usecase.BindKey{
		Kind:       domain.BindingRemote,
		RemoteSlug: "github.com/x/y",
	})
	if err != ports.ErrProjectNotFound {
		t.Fatalf("expected ErrProjectNotFound, got %v", err)
	}
}

func TestBindProject_UpsertCalledAndReturnsBinding(t *testing.T) {
	ps := testutil.NewFakeProjectStore()
	bs := testutil.NewFakeProjectBindingStore()
	clk := testutil.FakeClock{T: time.Unix(100, 0)}
	ids := &testutil.FakeIDGen{}

	p, _ := ps.Create(context.Background(), domain.Project{ID: "proj1", OwnerID: "alice", Slug: "myapp"})

	uc := usecase.BindProject{Bindings: bs, Projects: ps, IDs: ids, Clock: clk}
	b, err := uc.Execute(context.Background(), "alice", p.ID, usecase.BindKey{
		Kind:       domain.BindingRemote,
		RemoteSlug: "github.com/alice/myapp",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.ProjectID != "proj1" {
		t.Errorf("ProjectID = %q, want proj1", b.ProjectID)
	}
	if b.RemoteSlug != "github.com/alice/myapp" {
		t.Errorf("RemoteSlug = %q", b.RemoteSlug)
	}
	if b.CreatedAt != clk.T {
		t.Errorf("CreatedAt = %v, want %v", b.CreatedAt, clk.T)
	}

	// List via ListProjectBindings
	list := usecase.ListProjectBindings{Bindings: bs}
	bindings, err := list.Execute(context.Background(), "alice")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(bindings))
	}
}

func TestResolveProject_MatchingRemoteReturnsProject(t *testing.T) {
	ps := testutil.NewFakeProjectStore()
	bs := testutil.NewFakeProjectBindingStore()
	clk := testutil.FakeClock{T: time.Now()}
	ids := &testutil.FakeIDGen{}

	p, _ := ps.Create(context.Background(), domain.Project{ID: "p2", OwnerID: "bob", Slug: "svc"})

	binder := usecase.BindProject{Bindings: bs, Projects: ps, IDs: ids, Clock: clk}
	if _, err := binder.Execute(context.Background(), "bob", p.ID, usecase.BindKey{
		Kind:       domain.BindingRemote,
		RemoteSlug: "github.com/bob/svc",
	}); err != nil {
		t.Fatalf("bind: %v", err)
	}

	resolver := usecase.ResolveProject{Bindings: bs, Projects: ps}
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

func TestResolveProject_NoMatchReturnsFalse(t *testing.T) {
	ps := testutil.NewFakeProjectStore()
	bs := testutil.NewFakeProjectBindingStore()

	resolver := usecase.ResolveProject{Bindings: bs, Projects: ps}
	_, ok, err := resolver.Execute(context.Background(), "carol", "github.com/carol/nothing", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected no match")
	}
}

func TestUnbindProject_Remote(t *testing.T) {
	ps := testutil.NewFakeProjectStore()
	bs := testutil.NewFakeProjectBindingStore()
	clk := testutil.FakeClock{T: time.Now()}
	ids := &testutil.FakeIDGen{}

	p, _ := ps.Create(context.Background(), domain.Project{ID: "p3", OwnerID: "dave", Slug: "tool"})

	binder := usecase.BindProject{Bindings: bs, Projects: ps, IDs: ids, Clock: clk}
	if _, err := binder.Execute(context.Background(), "dave", p.ID, usecase.BindKey{
		Kind:       domain.BindingRemote,
		RemoteSlug: "github.com/dave/tool",
	}); err != nil {
		t.Fatalf("bind: %v", err)
	}

	unbinder := usecase.UnbindProject{Bindings: bs}
	if err := unbinder.Execute(context.Background(), "dave", usecase.BindKey{
		Kind:       domain.BindingRemote,
		RemoteSlug: "github.com/dave/tool",
	}); err != nil {
		t.Fatalf("unbind: %v", err)
	}

	// Verify it is gone.
	list := usecase.ListProjectBindings{Bindings: bs}
	bindings, _ := list.Execute(context.Background(), "dave")
	if len(bindings) != 0 {
		t.Fatalf("expected 0 bindings after unbind, got %d", len(bindings))
	}
}

func TestListProjectBindings_Empty(t *testing.T) {
	bs := testutil.NewFakeProjectBindingStore()
	uc := usecase.ListProjectBindings{Bindings: bs}
	bindings, err := uc.Execute(context.Background(), "nobody")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bindings) != 0 {
		t.Fatalf("expected 0, got %d", len(bindings))
	}
}
