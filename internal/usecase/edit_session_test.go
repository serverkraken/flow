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

func TestEditSession(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	stop := start.Add(2 * time.Hour)
	if _, err := ss.Create(ctx, domain.WorkSession{ID: "s1", OwnerID: "u1", Start: start, Stop: &stop}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	uc := usecase.EditSession{Sessions: ss}

	newStop := start.Add(3 * time.Hour)
	got, err := uc.Execute(ctx, "u1", "s1", usecase.EditSessionInput{Tag: "deep", Note: "n", Start: start, Stop: &newStop})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if got.Tag != "deep" || got.Stop == nil || !got.Stop.Equal(newStop) {
		t.Fatalf("edit not applied: %+v", got)
	}

	// stop <= start -> ErrStopBeforeStart
	bad := start.Add(-time.Minute)
	if _, err := uc.Execute(ctx, "u1", "s1", usecase.EditSessionInput{Start: start, Stop: &bad}); !errors.Is(err, domain.ErrStopBeforeStart) {
		t.Fatalf("want ErrStopBeforeStart, got %v", err)
	}
	// foreign owner -> not found (store-enforced)
	if _, err := uc.Execute(ctx, "other", "s1", usecase.EditSessionInput{Start: start, Stop: &newStop}); err == nil {
		t.Fatal("foreign edit should fail")
	}
}
