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

func TestMoveNode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	now := time.Now()
	mk := func(id string, kind domain.NodeKind, parent *string) {
		n, _ := domain.NewNode(id, "o", id, id, now)
		n.Kind = kind
		n.ParentID = parent
		_, _ = nodes.Create(ctx, n)
	}
	mk("eng", domain.KindEngagement, nil)
	mk("eng2", domain.KindEngagement, nil)
	mk("vor", domain.KindVorhaben, sp("eng"))
	mk("repo", domain.KindRepo, sp("vor"))

	uc := usecase.MoveNode{Nodes: nodes}

	// valid: repo → eng2
	if got, err := uc.Execute(ctx, "o", "repo", sp("eng2")); err != nil || got.ParentID == nil || *got.ParentID != "eng2" {
		t.Fatalf("valid move: %+v err=%v", got, err)
	}
	// kind violation: vor under repo
	if _, err := uc.Execute(ctx, "o", "vor", sp("repo")); !errors.Is(err, domain.ErrInvalidNode) {
		t.Fatalf("kind violation: want ErrInvalidNode, got %v", err)
	}
	// cycle: eng under its own descendant vor
	if _, err := uc.Execute(ctx, "o", "eng", sp("vor")); !errors.Is(err, usecase.ErrNodeCycle) {
		t.Fatalf("cycle: want ErrNodeCycle, got %v", err)
	}
	// self-parent is a cycle
	if _, err := uc.Execute(ctx, "o", "vor", sp("vor")); !errors.Is(err, usecase.ErrNodeCycle) {
		t.Fatalf("self-parent: want ErrNodeCycle, got %v", err)
	}
	// move repo to root → only engagements may be roots
	if _, err := uc.Execute(ctx, "o", "repo", nil); !errors.Is(err, domain.ErrInvalidNode) {
		t.Fatalf("repo to root: want ErrInvalidNode, got %v", err)
	}
	// engagement to root is fine
	if _, err := uc.Execute(ctx, "o", "eng", nil); err != nil {
		t.Fatalf("eng to root: %v", err)
	}
}
