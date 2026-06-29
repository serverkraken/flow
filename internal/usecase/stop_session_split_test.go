package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// TestStopSession_SplitsAcrossMidnight is the #9 behaviour: stopping a timer
// that started yesterday produces one booked session per calendar day, all on
// the same project. The tag-loss fix (B2 D2) is verified by asserting via
// FakeTagStore.TagsFor — not via s.Tags on the session struct, which is a
// false-green in fakes (Create stores Tags but real pgstore does not).
func TestStopSession_SplitsAcrossMidnight(t *testing.T) {
	ctx := context.Background()
	loc := time.UTC
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeNodeStore()
	ts := testutil.NewFakeTagStore()
	ids := &testutil.FakeIDGen{}
	now := time.Date(2026, 6, 24, 14, 0, 0, 0, loc)
	clk := testutil.FakeClock{T: now}
	if _, err := ps.Create(ctx, domain.Node{ID: "p1", OwnerID: "u1", Kind: domain.KindEngagement, Name: "flow", Status: domain.NodeActive}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	// running since yesterday 18:51
	start := time.Date(2026, 6, 23, 18, 51, 0, 0, loc)
	if _, err := ss.Create(ctx, domain.WorkSession{ID: "run", OwnerID: "u1", Start: start, Tags: []string{"deep"}}); err != nil {
		t.Fatalf("seed running: %v", err)
	}
	// Seed the original session's tags in the tag store (mirrors what pgstore does
	// after SetTags on creation). StopSession reads cur.Tags from the session struct
	// returned by Sessions.Get (FakeSessionStore copies the Tags field directly).
	if _, err := ts.SetTags(ctx, "u1", domain.TaggableWorkSession, "run", []string{"deep"}); err != nil {
		t.Fatalf("seed tags: %v", err)
	}

	uc := usecase.StopSession{Sessions: ss, Nodes: ps, IDs: ids, Clock: clk, Loc: loc, Tags: ts}
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
		// Assert via FakeTagStore that EACH chunk carries the tag (real pgstore path).
		sessionTags, terr := ts.TagsFor(ctx, "u1", domain.TaggableWorkSession, s.ID)
		if terr != nil {
			t.Fatalf("TagsFor(%s): %v", s.ID, terr)
		}
		if len(sessionTags) != 1 || sessionTags[0].Slug != "deep" {
			t.Errorf("session %s lost tags: got %v", s.ID, sessionTags)
		}
		// each chunk stays within one calendar day
		if s.Stop != nil && s.Start.Before(mid) && s.Stop.After(mid) {
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