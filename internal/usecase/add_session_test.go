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

func newAddSession(ss *testutil.FakeSessionStore, now time.Time) usecase.AddSession {
	return usecase.AddSession{Sessions: ss, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: now}}
}

func TestAddSession_HappyPath(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	now := time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC)
	uc := newAddSession(ss, now)
	start := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	stop := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	pid := "p1"
	got, err := uc.Execute(ctx, "u1", &pid, start, stop, "deep", "n")
	if err != nil {
		t.Fatalf("AddSession: %v", err)
	}
	if got.ID == "" || got.Stop == nil || !got.Stop.Equal(stop) || got.Tag != "deep" {
		t.Fatalf("AddSession result wrong: %+v", got)
	}
	if got.CreatedAt != start {
		t.Errorf("CreatedAt = %v, want start %v", got.CreatedAt, start)
	}
}

func TestAddSession_StopBeforeStart(t *testing.T) {
	ctx := context.Background()
	uc := newAddSession(testutil.NewFakeSessionStore(), time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC))
	start := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	stop := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	if _, err := uc.Execute(ctx, "u1", nil, start, stop, "", ""); !errors.Is(err, domain.ErrStopBeforeStart) {
		t.Fatalf("want ErrStopBeforeStart, got %v", err)
	}
}

func TestAddSession_Future(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	uc := newAddSession(testutil.NewFakeSessionStore(), now)
	start := time.Date(2026, 6, 15, 11, 0, 0, 0, time.UTC) // after now
	stop := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	if _, err := uc.Execute(ctx, "u1", nil, start, stop, "", ""); !errors.Is(err, domain.ErrFutureSession) {
		t.Fatalf("want ErrFutureSession, got %v", err)
	}
}

func TestAddSession_CrossMidnight(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	uc := newAddSession(testutil.NewFakeSessionStore(), now)
	start := time.Date(2026, 6, 15, 23, 0, 0, 0, time.UTC)
	stop := time.Date(2026, 6, 16, 1, 0, 0, 0, time.UTC) // next day
	if _, err := uc.Execute(ctx, "u1", nil, start, stop, "", ""); !errors.Is(err, domain.ErrInvalidSession) {
		t.Fatalf("want ErrInvalidSession (cross-midnight), got %v", err)
	}
}

func TestAddSession_Overlap(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	now := time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC)
	existingStop := time.Date(2026, 6, 15, 11, 0, 0, 0, time.UTC)
	if _, err := ss.Create(ctx, domain.WorkSession{
		ID: "x", OwnerID: "u1",
		Start: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC), Stop: &existingStop,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	uc := newAddSession(ss, now)
	start := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC) // overlaps 09:00–11:00
	stop := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	if _, err := uc.Execute(ctx, "u1", nil, start, stop, "", ""); !errors.Is(err, domain.ErrOverlap) {
		t.Fatalf("want ErrOverlap, got %v", err)
	}
}
