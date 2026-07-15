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

func TestDeleteSession(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	stop := start.Add(time.Hour)
	if _, err := ss.Create(ctx, domain.WorkSession{ID: "s1", OwnerID: "u1", Start: start, Stop: &stop}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	uc := usecase.DeleteSession{Sessions: ss}

	if err := uc.Execute(ctx, "u1", "s1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := uc.Execute(ctx, "u1", "s1"); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Fatalf("want ErrSessionNotFound, got %v", err)
	}
}

func TestDeleteSession_TagFailureRollsBackDelete(t *testing.T) {
	ctx := context.Background()
	base := testutil.NewFakeSessionStore()
	store := &failingSessionStore{FakeSessionStore: base, failTags: true}
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	stop := start.Add(time.Hour)
	if _, err := base.Create(ctx, domain.WorkSession{ID: "s1", OwnerID: "u1", Start: start, Stop: &stop}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	uc := usecase.DeleteSession{Sessions: store}
	if err := uc.Execute(ctx, "u1", "s1"); !errors.Is(err, errInjectedSessionWrite) {
		t.Fatalf("delete error=%v, want injected tag failure", err)
	}
	if _, err := base.Get(ctx, "u1", "s1"); err != nil {
		t.Fatalf("session was deleted despite tag failure: %v", err)
	}
}
