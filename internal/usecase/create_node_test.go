package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func ptrBool(b bool) *bool { return &b }

func TestCreateNode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	uc := usecase.CreateNode{Nodes: nodes, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: time.Now()}}

	// root must be engagement
	eng, err := uc.Execute(ctx, "o", usecase.CreateNodeInput{Name: "Privat", Kind: domain.KindEngagement})
	if err != nil || eng.Kind != domain.KindEngagement || eng.Slug != "privat" {
		t.Fatalf("engagement: %+v err=%v", eng, err)
	}
	if _, err := uc.Execute(ctx, "o", usecase.CreateNodeInput{Name: "X", Kind: domain.KindRepo}); !errors.Is(err, domain.ErrInvalidNode) {
		t.Fatalf("rootless repo: want ErrInvalidNode, got %v", err)
	}
	// repo under engagement ok
	repo, err := uc.Execute(ctx, "o", usecase.CreateNodeInput{Name: "flow", Kind: domain.KindRepo, ParentID: &eng.ID})
	if err != nil || repo.ParentID == nil || *repo.ParentID != eng.ID {
		t.Fatalf("repo: %+v err=%v", repo, err)
	}
	// repo under repo rejected
	if _, err := uc.Execute(ctx, "o", usecase.CreateNodeInput{Name: "b", Kind: domain.KindRepo, ParentID: &repo.ID}); !errors.Is(err, domain.ErrInvalidNode) {
		t.Fatalf("repo under repo: want ErrInvalidNode, got %v", err)
	}
	// unknown parent → ErrNodeNotFound
	bad := "nope"
	if _, err := uc.Execute(ctx, "o", usecase.CreateNodeInput{Name: "x", Kind: domain.KindRepo, ParentID: &bad}); err == nil {
		t.Fatal("unknown parent must error")
	}
}

func TestCreateNode_CountsTowardTarget(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	uc := usecase.CreateNode{Nodes: nodes, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: time.Now()}}

	// Explicit false overrides the NewNode default of nil (inherit).
	eng, err := uc.Execute(ctx, "o", usecase.CreateNodeInput{
		Name: "Privat", Kind: domain.KindEngagement,
		CountsTowardTarget: ptrBool(false),
	})
	if err != nil {
		t.Fatalf("create with false: %v", err)
	}
	if eng.CountsTowardTarget == nil || *eng.CountsTowardTarget {
		t.Fatalf("countsTowardTarget: want *false, got %v", eng.CountsTowardTarget)
	}

	// Omitted (nil) → NewNode default nil (inherit) is preserved.
	eng2, err := uc.Execute(ctx, "o", usecase.CreateNodeInput{Name: "Work", Kind: domain.KindEngagement})
	if err != nil {
		t.Fatalf("create default: %v", err)
	}
	if eng2.CountsTowardTarget != nil {
		t.Fatalf("countsTowardTarget omitted: want nil (inherit), got %v", *eng2.CountsTowardTarget)
	}
}

func TestCreateNode_ValidatesAndPersistsUpstreamGit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	uc := usecase.CreateNode{Nodes: nodes, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: time.Now()}}

	eng, err := uc.Execute(ctx, "o", usecase.CreateNodeInput{Name: "Work", Kind: domain.KindEngagement})
	if err != nil {
		t.Fatalf("create engagement: %v", err)
	}

	if _, err := uc.Execute(ctx, "o", usecase.CreateNodeInput{
		Name:        "invalid",
		Kind:        domain.KindRepo,
		ParentID:    &eng.ID,
		UpstreamGit: "not-a-git-remote",
	}); !errors.Is(err, domain.ErrInvalidUpstream) {
		t.Fatalf("invalid upstream: want ErrInvalidUpstream, got %v", err)
	}

	all, err := nodes.List(ctx, "o")
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("invalid create wrote a node: got %d nodes, want 1", len(all))
	}

	repo, err := uc.Execute(ctx, "o", usecase.CreateNodeInput{
		Name:        "flow",
		Kind:        domain.KindRepo,
		ParentID:    &eng.ID,
		UpstreamGit: "git@github.com:serverkraken/flow.git",
	})
	if err != nil {
		t.Fatalf("valid upstream: %v", err)
	}
	if repo.UpstreamGit != "git@github.com:serverkraken/flow.git" {
		t.Fatalf("upstreamGit = %q", repo.UpstreamGit)
	}
	if repo.OriginSlug != "github.com/serverkraken/flow" {
		t.Fatalf("originSlug = %q, want normalized upstream", repo.OriginSlug)
	}
}
