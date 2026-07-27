package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func seedSess(t *testing.T, ss *testutil.FakeSessionStore, id, owner string) {
	t.Helper()
	st := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	sp := st.Add(time.Hour)
	if _, err := ss.Create(context.Background(), domain.WorkSession{ID: id, OwnerID: owner, Start: st, Stop: &sp}); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func TestBulkAssignProject_AssignsOwnedSkipsForeign(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeNodeStore()
	seedSess(t, ss, "a", "u1")
	seedSess(t, ss, "b", "u1")
	seedSess(t, ss, "c", "u2") // foreign
	if _, err := ps.Create(ctx, domain.Node{ID: "p1", OwnerID: "u1", Name: "flow", Kind: domain.KindEngagement}); err != nil {
		t.Fatalf("seed proj: %v", err)
	}
	uc := usecase.BulkAssignNode{Sessions: ss, Nodes: ps}
	n, err := uc.Execute(ctx, "u1", []string{"a", "b", "c", "missing"}, "p1")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if n != 2 {
		t.Fatalf("updated = %d, want 2 (a,b; c foreign + missing skipped)", n)
	}
	got, _ := ss.Get(ctx, "u1", "a")
	if got.NodeID == nil || *got.NodeID != "p1" {
		t.Fatalf("a not assigned: %+v", got)
	}
	// foreign session untouched
	if c, _ := ss.Get(ctx, "u2", "c"); c.NodeID != nil {
		t.Fatalf("foreign c was mutated: %+v", c)
	}
}

func TestBulkAssignProject_EmptyIDs(t *testing.T) {
	uc := usecase.BulkAssignNode{Sessions: testutil.NewFakeSessionStore(), Nodes: testutil.NewFakeNodeStore()}
	if _, err := uc.Execute(context.Background(), "u1", nil, "p1"); !errors.Is(err, usecase.ErrNoSessions) {
		t.Fatalf("err = %v, want ErrNoSessions", err)
	}
}

func TestBulkAssignProject_RejectsUnboundedSelection(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeNodeStore()
	if _, err := ps.Create(ctx, domain.Node{ID: "p1", OwnerID: "u1", Name: "flow", Kind: domain.KindEngagement}); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 201)
	for i := range ids {
		ids[i] = fmt.Sprintf("session-%d", i)
	}

	if _, err := (usecase.BulkAssignNode{Sessions: ss, Nodes: ps}).Execute(ctx, "u1", ids, "p1"); err == nil {
		t.Fatal("unbounded bulk assignment must be rejected")
	}
}

func TestBulkAssignProject_ForeignProject(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeNodeStore()
	seedSess(t, ss, "a", "u1")
	if _, err := ps.Create(ctx, domain.Node{ID: "p2", OwnerID: "other", Name: "x"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	uc := usecase.BulkAssignNode{Sessions: ss, Nodes: ps}
	if _, err := uc.Execute(ctx, "u1", []string{"a"}, "p2"); !errors.Is(err, ports.ErrNodeNotFound) {
		t.Fatalf("err = %v, want ErrNodeNotFound", err)
	}
}

func TestBulkAssignProject_BranchRejected(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeNodeStore()
	seedSess(t, ss, "a", "u1")
	repoID := "repo1"
	if _, err := ps.Create(ctx, domain.Node{ID: repoID, OwnerID: "u1", Name: "myrepo", Kind: domain.KindRepo}); err != nil {
		t.Fatalf("seed repo: %v", err)
	}
	branchID := "branch1"
	if _, err := ps.Create(ctx, domain.Node{ID: branchID, OwnerID: "u1", Name: "feature", ParentID: &repoID, Kind: domain.KindBranch}); err != nil {
		t.Fatalf("seed branch: %v", err)
	}
	uc := usecase.BulkAssignNode{Sessions: ss, Nodes: ps}
	if _, err := uc.Execute(ctx, "u1", []string{"a"}, branchID); !errors.Is(err, domain.ErrInvalidNode) {
		t.Fatalf("err = %v, want domain.ErrInvalidNode", err)
	}
}
