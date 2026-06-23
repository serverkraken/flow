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

func TestGetProject(t *testing.T) {
	ctx := context.Background()
	ps := testutil.NewFakeProjectStore()
	now := time.Date(2026, 6, 23, 9, 0, 0, 0, time.UTC)
	p, _ := domain.NewProject("p1", "u1", "Flow", "flow", now)
	_, _ = ps.Create(ctx, p)

	uc := usecase.GetProject{Projects: ps}

	got, err := uc.Execute(ctx, "u1", "p1")
	if err != nil || got.Slug != "flow" {
		t.Fatalf("Execute: got %+v err=%v", got, err)
	}
	if _, err := uc.Execute(ctx, "u1", "missing"); !errors.Is(err, ports.ErrProjectNotFound) {
		t.Errorf("missing: want ErrProjectNotFound, got %v", err)
	}
	if _, err := uc.Execute(ctx, "other", "p1"); !errors.Is(err, ports.ErrProjectNotFound) {
		t.Errorf("foreign owner: want ErrProjectNotFound, got %v", err)
	}
}
