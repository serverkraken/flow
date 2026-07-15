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

var errInjectedSessionWrite = errors.New("injected session write failure")

type failingSessionWriter struct {
	ports.SessionWriter
	failCreate bool
	failTags   bool
}

func (w failingSessionWriter) Create(ctx context.Context, s domain.WorkSession) (domain.WorkSession, error) {
	if w.failCreate {
		return domain.WorkSession{}, errInjectedSessionWrite
	}
	return w.SessionWriter.Create(ctx, s)
}

func (w failingSessionWriter) SetTags(ctx context.Context, ownerID, sessionID string, tags []string) ([]string, error) {
	if w.failTags {
		return nil, errInjectedSessionWrite
	}
	return w.SessionWriter.SetTags(ctx, ownerID, sessionID, tags)
}

type failingSessionStore struct {
	*testutil.FakeSessionStore
	failCreate bool
	failTags   bool
}

func (s *failingSessionStore) WithinTransaction(ctx context.Context, fn func(ports.SessionWriter) error) error {
	return s.FakeSessionStore.WithinTransaction(ctx, func(w ports.SessionWriter) error {
		return fn(failingSessionWriter{SessionWriter: w, failCreate: s.failCreate, failTags: s.failTags})
	})
}

func TestStopSession_SplitWriteFailureRollsBackOriginalStop(t *testing.T) {
	ctx := context.Background()
	base := testutil.NewFakeSessionStore()
	store := &failingSessionStore{FakeSessionStore: base, failCreate: true}
	nodes := testutil.NewFakeNodeStore()
	_, _ = nodes.Create(ctx, domain.Node{ID: "eng", OwnerID: "u1", Kind: domain.KindEngagement, Name: "Work", Slug: "work", Status: domain.NodeActive})
	start := time.Date(2026, 7, 14, 22, 0, 0, 0, time.UTC)
	_, _ = base.Create(ctx, domain.WorkSession{ID: "run", OwnerID: "u1", Start: start, CreatedAt: start})

	uc := usecase.StopSession{
		Sessions: store,
		Nodes:    nodes,
		IDs:      &testutil.FakeIDGen{},
		Clock:    testutil.FakeClock{T: time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC)},
		Loc:      time.UTC,
	}
	nodeID := "eng"
	if _, err := uc.Execute(ctx, "u1", "run", &nodeID); !errors.Is(err, errInjectedSessionWrite) {
		t.Fatalf("stop error = %v, want injected failure", err)
	}
	got, err := base.Get(ctx, "u1", "run")
	if err != nil {
		t.Fatalf("get original: %v", err)
	}
	if got.Stop != nil {
		t.Fatalf("original session was partially stopped at %v", *got.Stop)
	}
	all, total, err := base.ListPage(ctx, "u1", 100, 0)
	if err != nil || total != 1 || len(all) != 1 {
		t.Fatalf("partial chunks persisted: total=%d sessions=%+v err=%v", total, all, err)
	}
}

func TestSwitchSession_StartFailureRollsBackStop(t *testing.T) {
	ctx := context.Background()
	base := testutil.NewFakeSessionStore()
	store := &failingSessionStore{FakeSessionStore: base, failCreate: true}
	nodes := testutil.NewFakeNodeStore()
	for _, id := range []string{"old", "next"} {
		_, _ = nodes.Create(ctx, domain.Node{ID: id, OwnerID: "u1", Kind: domain.KindRepo, Name: id, Slug: id, Status: domain.NodeActive})
	}
	start := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	old := "old"
	_, _ = base.Create(ctx, domain.WorkSession{ID: "run", OwnerID: "u1", NodeID: &old, Start: start, CreatedAt: start})

	uc := usecase.SwitchSession{
		Sessions: store,
		Nodes:    nodes,
		IDs:      &testutil.FakeIDGen{},
		Clock:    testutil.FakeClock{T: time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC)},
		Loc:      time.UTC,
	}
	next := "next"
	if _, _, _, err := uc.Execute(ctx, "u1", &next); !errors.Is(err, errInjectedSessionWrite) {
		t.Fatalf("switch error = %v, want injected failure", err)
	}
	got, running, err := base.Running(ctx, "u1")
	if err != nil || !running || got.ID != "run" {
		t.Fatalf("original timer not restored: got=%+v running=%v err=%v", got, running, err)
	}
}

func TestStartSession_TagFailureRollsBackSession(t *testing.T) {
	ctx := context.Background()
	base := testutil.NewFakeSessionStore()
	store := &failingSessionStore{FakeSessionStore: base, failTags: true}
	uc := usecase.StartSession{
		Sessions: store,
		IDs:      &testutil.FakeIDGen{},
		Clock:    testutil.FakeClock{T: time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC)},
	}
	if _, err := uc.Execute(ctx, "u1", nil, []string{"deep"}, ""); !errors.Is(err, errInjectedSessionWrite) {
		t.Fatalf("start error = %v, want injected failure", err)
	}
	if _, running, err := base.Running(ctx, "u1"); err != nil || running {
		t.Fatalf("session survived tag failure: running=%v err=%v", running, err)
	}
}

func TestEditSession_TagFailureRollsBackSessionUpdate(t *testing.T) {
	ctx := context.Background()
	base := testutil.NewFakeSessionStore()
	store := &failingSessionStore{FakeSessionStore: base, failTags: true}
	start := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	stop := start.Add(time.Hour)
	_, _ = base.Create(ctx, domain.WorkSession{ID: "s1", OwnerID: "u1", Note: "before", Start: start, Stop: &stop, CreatedAt: start})
	tags := []string{"deep"}
	uc := usecase.EditSession{Sessions: store, Clock: testutil.FakeClock{T: stop.Add(time.Hour)}, Loc: time.UTC}
	if _, err := uc.Execute(ctx, "u1", "s1", usecase.EditSessionInput{
		Tags: &tags, Note: "after", Start: start, Stop: &stop,
	}); !errors.Is(err, errInjectedSessionWrite) {
		t.Fatalf("edit error = %v, want injected failure", err)
	}
	got, err := base.Get(ctx, "u1", "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.Note != "before" {
		t.Fatalf("session update survived tag failure: %+v", got)
	}
}
