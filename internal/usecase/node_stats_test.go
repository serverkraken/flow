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

func TestNodeStats_UnknownNode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	ss := fakeSessionStore{}
	c := usecase.StatsComputer{
		Sessions: ss,
		Clock:    fixedClock{t: time.Now()},
		Loc:      time.UTC,
		Nodes:    ns,
	}
	_, err := c.NodeStats(ctx, "u1", "ghost")
	if !errors.Is(err, ports.ErrNodeNotFound) {
		t.Fatalf("want ErrNodeNotFound, got %v", err)
	}
}

func TestNodeStats_RollsUpSubtree(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	eng := "eng"
	_, _ = ns.Create(ctx, domain.Node{ID: "eng", OwnerID: "u1", Kind: domain.KindEngagement, Name: "eng", Slug: "eng", Status: domain.NodeActive})
	_, _ = ns.Create(ctx, domain.Node{ID: "repo", OwnerID: "u1", ParentID: &eng, Kind: domain.KindRepo, Name: "repo", Slug: "repo", Status: domain.NodeActive})
	_, _ = ns.Create(ctx, domain.Node{ID: "other", OwnerID: "u1", Kind: domain.KindEngagement, Name: "other", Slug: "other", Status: domain.NodeActive})

	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC) // Wednesday

	repo := "repo"
	other := "other"
	stop1 := time.Date(2026, 6, 17, 11, 0, 0, 0, time.UTC)
	stop2 := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	stop3 := time.Date(2026, 6, 17, 13, 0, 0, 0, time.UTC)

	ss := fakeSessionStore{list: []domain.WorkSession{
		// repo session 2026-06-17 09:00–11:00 (2h, this week+month)
		{ID: "s1", OwnerID: "u1", NodeID: &repo, Start: time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC), Stop: &stop1},
		// repo session 2026-06-03 09:00–10:00 (1h, this month, not this week)
		{ID: "s2", OwnerID: "u1", NodeID: &repo, Start: time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC), Stop: &stop2},
		// other session 2026-06-17 09:00–13:00 (4h, must NOT count)
		{ID: "s3", OwnerID: "u1", NodeID: &other, Start: time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC), Stop: &stop3},
	}}

	c := usecase.StatsComputer{
		Sessions: ss,
		Clock:    fixedClock{t: now},
		Loc:      time.UTC,
		Nodes:    ns,
	}

	r, err := c.NodeStats(ctx, "u1", "eng")
	if err != nil {
		t.Fatalf("nodestats: %v", err)
	}
	if r.Total != 3*time.Hour {
		t.Errorf("Total = %v, want 3h", r.Total)
	}
	if r.Week != 2*time.Hour {
		t.Errorf("Week = %v, want 2h", r.Week)
	}
	if r.Month != 3*time.Hour {
		t.Errorf("Month = %v, want 3h", r.Month)
	}
}
