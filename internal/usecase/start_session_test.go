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

func newStartSession(ss *testutil.FakeSessionStore, ns *testutil.FakeNodeStore, now time.Time) usecase.StartSession {
	return usecase.StartSession{Sessions: ss, Nodes: ns, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: now}}
}

// seedEngagement / seedRepo are shared by the worktime use-case tests.
func seedEngagement(t *testing.T, ns *testutil.FakeNodeStore, ownerID, id string) {
	t.Helper()
	if _, err := ns.Create(context.Background(), domain.Node{
		ID: id, OwnerID: ownerID, Kind: domain.KindEngagement,
		Name: id, Slug: id, Status: domain.NodeActive,
	}); err != nil {
		t.Fatalf("seed engagement: %v", err)
	}
}

func seedRepo(t *testing.T, ns *testutil.FakeNodeStore, ownerID, id string) {
	t.Helper()
	parent := "eng-root"
	if _, err := ns.Create(context.Background(), domain.Node{
		ID: id, OwnerID: ownerID, ParentID: &parent, Kind: domain.KindRepo,
		Name: id, Slug: id, Status: domain.NodeActive,
	}); err != nil {
		t.Fatalf("seed repo: %v", err)
	}
}

func TestStartSession_NilNodeStartsUnbooked(t *testing.T) {
	t.Parallel()
	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
	uc := newStartSession(ss, ns, time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC))
	got, err := uc.Execute(context.Background(), "u1", nil, []string{"deep"}, "n")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if got.NodeID != nil {
		t.Errorf("want nil NodeID, got %v", *got.NodeID)
	}
}

func TestStartSession_EngagementAccepted(t *testing.T) {
	t.Parallel()
	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
	seedEngagement(t, ns, "u1", "eng1")
	uc := newStartSession(ss, ns, time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC))
	eng := "eng1"
	got, err := uc.Execute(context.Background(), "u1", &eng, nil, "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if got.NodeID == nil || *got.NodeID != "eng1" {
		t.Errorf("want NodeID eng1, got %v", got.NodeID)
	}
}

func TestStartSession_RepoRejected(t *testing.T) {
	t.Parallel()
	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
	seedRepo(t, ns, "u1", "repo1")
	uc := newStartSession(ss, ns, time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC))
	repo := "repo1"
	if _, err := uc.Execute(context.Background(), "u1", &repo, nil, ""); !errors.Is(err, domain.ErrInvalidNode) {
		t.Fatalf("want ErrInvalidNode for repo node, got %v", err)
	}
}

func TestStartSession_MissingNodeRejected(t *testing.T) {
	t.Parallel()
	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
	uc := newStartSession(ss, ns, time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC))
	ghost := "ghost"
	if _, err := uc.Execute(context.Background(), "u1", &ghost, nil, ""); !errors.Is(err, ports.ErrNodeNotFound) {
		t.Fatalf("want ErrNodeNotFound, got %v", err)
	}
}
