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

type createBoundAggregateStub struct {
	called  int
	node    domain.Node
	changes ports.NodeAggregateChanges
	binding domain.ProjectBinding
	err     error
}

func (s *createBoundAggregateStub) CreateBoundAggregate(_ context.Context, n domain.Node, changes ports.NodeAggregateChanges, binding domain.ProjectBinding) (domain.Node, domain.ProjectBinding, error) {
	s.called++
	s.node, s.changes, s.binding = n, changes, binding
	if s.err != nil {
		return domain.Node{}, domain.ProjectBinding{}, s.err
	}
	return n, binding, nil
}

func TestCreateBoundNode_PreparesRemoteNodeAndBindingForOneAggregateWrite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	parent, err := domain.NewNode("parent", "owner", "Work", "work", now)
	if err != nil {
		t.Fatal(err)
	}
	parent.Kind = domain.KindEngagement
	if _, err := nodes.Create(ctx, parent); err != nil {
		t.Fatal(err)
	}
	aggregate := &createBoundAggregateStub{}
	uc := usecase.CreateBoundNode{
		Nodes: nodes, Aggregate: aggregate, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: now},
	}

	result, err := uc.Execute(ctx, "owner", usecase.CreateBoundNodeInput{
		Node:    usecase.CreateNodeInput{Name: "Flow", Kind: domain.KindRepo, ParentID: &parent.ID},
		Binding: usecase.BindKey{Kind: domain.BindingRemote, RemoteSlug: "git@github.com:serverkraken/flow.git"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if aggregate.called != 1 {
		t.Fatalf("aggregate writes = %d, want 1", aggregate.called)
	}
	if result.Node.ID == "" || result.Node.ParentID == nil || *result.Node.ParentID != parent.ID {
		t.Fatalf("node = %+v", result.Node)
	}
	if result.Node.OriginSlug != "github.com/serverkraken/flow" {
		t.Fatalf("origin slug = %q", result.Node.OriginSlug)
	}
	if result.Binding.NodeID != result.Node.ID || result.Binding.OwnerID != "owner" || result.Binding.RemoteSlug != result.Node.OriginSlug {
		t.Fatalf("binding = %+v, node = %+v", result.Binding, result.Node)
	}
	if _, err := nodes.Get(ctx, "owner", result.Node.ID); !errors.Is(err, ports.ErrNodeNotFound) {
		t.Fatalf("usecase performed a separate node write: %v", err)
	}
}

func TestCreateBoundNode_RejectsInvalidBindingBeforeAggregateWrite(t *testing.T) {
	t.Parallel()
	aggregate := &createBoundAggregateStub{}
	uc := usecase.CreateBoundNode{
		Nodes: testutil.NewFakeNodeStore(), Aggregate: aggregate,
		IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: time.Now()},
	}
	_, err := uc.Execute(context.Background(), "owner", usecase.CreateBoundNodeInput{
		Node:    usecase.CreateNodeInput{Name: "Flow", Kind: domain.KindRepo},
		Binding: usecase.BindKey{Kind: domain.BindingPath, Path: "/work/flow"},
	})
	if !errors.Is(err, usecase.ErrInvalidBindTarget) {
		t.Fatalf("want ErrInvalidBindTarget, got %v", err)
	}
	if aggregate.called != 0 {
		t.Fatalf("aggregate called %d times", aggregate.called)
	}
}

func TestCreateBoundNode_PropagatesAggregateFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	parent, _ := domain.NewNode("parent", "owner", "Work", "work", now)
	parent.Kind = domain.KindEngagement
	_, _ = nodes.Create(ctx, parent)
	wantErr := errors.New("binding write failed")
	aggregate := &createBoundAggregateStub{err: wantErr}
	uc := usecase.CreateBoundNode{
		Nodes: nodes, Aggregate: aggregate, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: now},
	}

	_, err := uc.Execute(ctx, "owner", usecase.CreateBoundNodeInput{
		Node:    usecase.CreateNodeInput{Name: "Flow", Kind: domain.KindRepo, ParentID: &parent.ID},
		Binding: usecase.BindKey{Kind: domain.BindingRemote, RemoteSlug: "github.com/serverkraken/flow"},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("want aggregate failure, got %v", err)
	}
	if aggregate.called != 1 {
		t.Fatalf("aggregate writes = %d, want 1", aggregate.called)
	}
}
