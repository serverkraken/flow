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

func newAddSession(ss *testutil.FakeSessionStore, ns *testutil.FakeNodeStore, now time.Time) usecase.AddSession {
	return usecase.AddSession{Sessions: ss, Nodes: ns, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: now}, Loc: time.UTC}
}

func TestAddSession_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
	seedEngagement(t, ns, "u1", "p1")
	now := time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC)
	uc := newAddSession(ss, ns, now)
	start := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	stop := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	pid := "p1"
	got, err := uc.Execute(ctx, "u1", &pid, start, stop, []string{"deep"}, "n")
	if err != nil {
		t.Fatalf("AddSession: %v", err)
	}
	if got.ID == "" || got.Stop == nil || !got.Stop.Equal(stop) || len(got.Tags) != 1 || got.Tags[0] != "deep" {
		t.Fatalf("AddSession result wrong: %+v", got)
	}
	if got.CreatedAt != start {
		t.Errorf("CreatedAt = %v, want start %v", got.CreatedAt, start)
	}
}

func TestAddSession_RepoAccepted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
	seedRepo(t, ns, "u1", "repo1")
	uc := newAddSession(ss, ns, time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC))
	start := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	stop := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	repo := "repo1"
	got, err := uc.Execute(ctx, "u1", &repo, start, stop, nil, "")
	if err != nil {
		t.Fatalf("add on repo: %v", err)
	}
	if got.NodeID == nil || *got.NodeID != "repo1" {
		t.Errorf("want NodeID repo1, got %v", got.NodeID)
	}
}

func TestAddSession_StopBeforeStart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	uc := newAddSession(testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore(), time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC))
	start := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	stop := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	if _, err := uc.Execute(ctx, "u1", nil, start, stop, nil, ""); !errors.Is(err, domain.ErrStopBeforeStart) {
		t.Fatalf("want ErrStopBeforeStart, got %v", err)
	}
}

func TestAddSession_Future(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	uc := newAddSession(testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore(), now)
	start := time.Date(2026, 6, 15, 11, 0, 0, 0, time.UTC)
	stop := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	if _, err := uc.Execute(ctx, "u1", nil, start, stop, nil, ""); !errors.Is(err, domain.ErrFutureSession) {
		t.Fatalf("want ErrFutureSession, got %v", err)
	}
}

func TestAddSession_CrossMidnight(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	uc := newAddSession(testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore(), now)
	start := time.Date(2026, 6, 15, 23, 0, 0, 0, time.UTC)
	stop := time.Date(2026, 6, 16, 1, 0, 0, 0, time.UTC)
	if _, err := uc.Execute(ctx, "u1", nil, start, stop, nil, ""); !errors.Is(err, domain.ErrInvalidSession) {
		t.Fatalf("want ErrInvalidSession (cross-midnight), got %v", err)
	}
}

func TestAddSession_UsesBusinessTimezoneForSameDayRule(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 7, 15, 21, 30, 0, 0, time.UTC) // 23:30 in Berlin
	stop := time.Date(2026, 7, 15, 22, 30, 0, 0, time.UTC)  // 00:30 next day in Berlin
	uc := newAddSession(testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore(), stop.Add(time.Hour))
	uc.Loc = loc
	if _, err := uc.Execute(ctx, "u1", nil, start, stop, nil, ""); !errors.Is(err, domain.ErrInvalidSession) {
		t.Fatalf("want business-day ErrInvalidSession, got %v", err)
	}
}

func TestAddSession_Overlap(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
	now := time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC)
	existingStop := time.Date(2026, 6, 15, 11, 0, 0, 0, time.UTC)
	if _, err := ss.Create(ctx, domain.WorkSession{
		ID: "x", OwnerID: "u1",
		Start: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC), Stop: &existingStop,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	uc := newAddSession(ss, ns, now)
	start := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	stop := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	if _, err := uc.Execute(ctx, "u1", nil, start, stop, nil, ""); !errors.Is(err, domain.ErrOverlap) {
		t.Fatalf("want ErrOverlap, got %v", err)
	}
}

func TestAddSession_OverlapWithRunningOutsideWindow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
	if _, err := ss.Create(ctx, domain.WorkSession{
		ID: "running", OwnerID: "u1",
		Start: time.Date(2026, 6, 12, 8, 0, 0, 0, time.UTC), Stop: nil,
	}); err != nil {
		t.Fatalf("seed running session: %v", err)
	}
	now := time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC)
	uc := newAddSession(ss, ns, now)
	start := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	stop := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	if _, err := uc.Execute(ctx, "u1", nil, start, stop, nil, ""); !errors.Is(err, domain.ErrOverlap) {
		t.Fatalf("want ErrOverlap (running session spans candidate), got %v", err)
	}
}
