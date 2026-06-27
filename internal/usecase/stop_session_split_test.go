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

// TestStopSession_SplitsAcrossMidnight is the #9 behaviour: stopping a timer
// that started yesterday produces one booked session per calendar day, all on
// the same project.
func TestStopSession_SplitsAcrossMidnight(t *testing.T) {
	ctx := context.Background()
	loc := time.UTC
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeNodeStore()
	ids := &testutil.FakeIDGen{}
	now := time.Date(2026, 6, 24, 14, 0, 0, 0, loc)
	clk := testutil.FakeClock{T: now}
	if _, err := ps.Create(ctx, domain.Node{ID: "p1", OwnerID: "u1", Kind: domain.KindEngagement, Name: "flow", Status: domain.NodeActive}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	// running since yesterday 18:51
	start := time.Date(2026, 6, 23, 18, 51, 0, 0, loc)
	if _, err := ss.Create(ctx, domain.WorkSession{ID: "run", OwnerID: "u1", Start: start, Tag: "deep"}); err != nil {
		t.Fatalf("seed running: %v", err)
	}

	uc := usecase.StopSession{Sessions: ss, Nodes: ps, IDs: ids, Clock: clk, Loc: loc}
	pid := "p1"
	if _, err := uc.Execute(ctx, "u1", "run", &pid); err != nil {
		t.Fatalf("stop: %v", err)
	}

	all, _, err := ss.ListPage(ctx, "u1", 100, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("want 2 sessions after split, got %d: %+v", len(all), all)
	}
	mid := time.Date(2026, 6, 24, 0, 0, 0, 0, loc)
	for _, s := range all {
		if s.Stop == nil {
			t.Errorf("session %s still running after stop", s.ID)
		}
		if s.NodeID == nil || *s.NodeID != "p1" {
			t.Errorf("session %s not booked to p1: %+v", s.ID, s.NodeID)
		}
		if s.Tag != "deep" {
			t.Errorf("session %s lost tag: %q", s.ID, s.Tag)
		}
		// each chunk stays within one calendar day
		if s.Start.Before(mid) && s.Stop.After(mid) {
			t.Errorf("session %s spans midnight: %v..%v", s.ID, s.Start, *s.Stop)
		}
	}
}

// TestStopSession_SameDayNoSplit confirms a normal same-day stop is unchanged.
func TestStopSession_SameDayNoSplit(t *testing.T) {
	ctx := context.Background()
	loc := time.UTC
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeNodeStore()
	ids := &testutil.FakeIDGen{}
	clk := testutil.FakeClock{T: time.Date(2026, 6, 24, 12, 0, 0, 0, loc)}
	_, _ = ps.Create(ctx, domain.Node{ID: "p1", OwnerID: "u1", Kind: domain.KindEngagement, Name: "flow", Status: domain.NodeActive})
	_, _ = ss.Create(ctx, domain.WorkSession{ID: "run", OwnerID: "u1", Start: time.Date(2026, 6, 24, 9, 0, 0, 0, loc)})
	uc := usecase.StopSession{Sessions: ss, Nodes: ps, IDs: ids, Clock: clk, Loc: loc}
	pid := "p1"
	if _, err := uc.Execute(ctx, "u1", "run", &pid); err != nil {
		t.Fatalf("stop: %v", err)
	}
	all, _, _ := ss.ListPage(ctx, "u1", 100, 0)
	if len(all) != 1 {
		t.Fatalf("same-day stop should not split, got %d sessions", len(all))
	}
}

// TestStopSession_RepoRejected ensures that booking a session to a non-engagement
// node (a KindRepo here) returns domain.ErrInvalidNode.
func TestStopSession_RepoRejected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	loc := time.UTC
	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
	seedRepo(t, ns, "u1", "repo1")
	_, _ = ss.Create(ctx, domain.WorkSession{ID: "run", OwnerID: "u1", Start: time.Date(2026, 6, 24, 9, 0, 0, 0, loc)})
	uc := usecase.StopSession{Sessions: ss, Nodes: ns, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: time.Date(2026, 6, 24, 12, 0, 0, 0, loc)}, Loc: loc}
	repo := "repo1"
	if _, err := uc.Execute(ctx, "u1", "run", &repo); !errors.Is(err, domain.ErrInvalidNode) {
		t.Fatalf("want ErrInvalidNode booking a repo, got %v", err)
	}
}
