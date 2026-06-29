package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestStopSession_RepoAccepted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
	seedRepo(t, ns, "u1", "repo1")
	start := time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)
	if _, err := ss.Create(ctx, domain.WorkSession{ID: "s1", OwnerID: "u1", Start: start}); err != nil {
		t.Fatalf("seed running: %v", err)
	}
	uc := usecase.StopSession{
		Sessions: ss, Nodes: ns, IDs: &testutil.FakeIDGen{},
		Clock: testutil.FakeClock{T: start.Add(time.Hour)}, Loc: time.UTC,
	}
	repo := "repo1"
	got, err := uc.Execute(ctx, "u1", "s1", &repo)
	if err != nil {
		t.Fatalf("stop on repo: %v", err)
	}
	if got.NodeID == nil || *got.NodeID != "repo1" {
		t.Errorf("want NodeID repo1, got %v", got.NodeID)
	}
}
