package usecase_test

import (
	"context"
	"errors"
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
