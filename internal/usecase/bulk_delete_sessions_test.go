package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestBulkDeleteSessions(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	seedSess(t, ss, "a", "u1")
	seedSess(t, ss, "b", "u1")
	seedSess(t, ss, "c", "u2")
	uc := usecase.BulkDeleteSessions{Sessions: ss}
	n, err := uc.Execute(ctx, "u1", []string{"a", "b", "c", "missing"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if n != 2 {
		t.Fatalf("deleted = %d, want 2", n)
	}
	if _, err := ss.Get(ctx, "u1", "a"); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Fatalf("a not deleted")
	}
	if _, err := ss.Get(ctx, "u2", "c"); err != nil {
		t.Fatalf("foreign c was deleted")
	}
	if _, err := uc.Execute(ctx, "u1", nil); !errors.Is(err, usecase.ErrNoSessions) {
		t.Fatalf("empty ids should be ErrNoSessions")
	}
}

func TestBulkDeleteSessions_TagFailureRollsBackDelete(t *testing.T) {
	ctx := context.Background()
	base := testutil.NewFakeSessionStore()
	store := &failingSessionStore{FakeSessionStore: base, failTags: true}
	seedSess(t, base, "a", "u1")
	uc := usecase.BulkDeleteSessions{Sessions: store}
	if n, err := uc.Execute(ctx, "u1", []string{"a"}); n != 0 || !errors.Is(err, errInjectedSessionWrite) {
		t.Fatalf("bulk delete=(%d,%v), want (0,injected tag failure)", n, err)
	}
	if _, err := base.Get(ctx, "u1", "a"); err != nil {
		t.Fatalf("session was deleted despite tag failure: %v", err)
	}
}

func TestBulkDeleteSessions_RejectsUnboundedSelection(t *testing.T) {
	ids := make([]string, 201)
	for i := range ids {
		ids[i] = fmt.Sprintf("session-%d", i)
	}
	uc := usecase.BulkDeleteSessions{Sessions: testutil.NewFakeSessionStore()}
	if _, err := uc.Execute(context.Background(), "u1", ids); err == nil {
		t.Fatal("unbounded bulk delete must be rejected")
	}
}
