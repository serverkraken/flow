package testutil_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
)

func TestFakeSessionStore_UpdateAndDelete(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	stop := start.Add(time.Hour)
	seed := domain.WorkSession{ID: "s1", OwnerID: "u1", Start: start, Stop: &stop}
	if _, err := ss.Create(ctx, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	pid := "p1"
	got, err := ss.Update(ctx, "u1", "s1", &pid, "deep", "note", start, &stop)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Tag != "deep" || got.Note != "note" || got.ProjectID == nil || *got.ProjectID != "p1" {
		t.Fatalf("update did not persist fields: %+v", got)
	}

	// non-existent id -> not found
	if _, err := ss.Update(ctx, "u1", "no-such-id", nil, "", "", start, &stop); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Fatalf("missing id update: want ErrSessionNotFound, got %v", err)
	}
	// foreign owner -> not found
	if _, err := ss.Update(ctx, "other", "s1", nil, "", "", start, &stop); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Fatalf("foreign update: want ErrSessionNotFound, got %v", err)
	}
	// delete foreign -> not found
	if err := ss.Delete(ctx, "other", "s1"); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Fatalf("foreign delete: want ErrSessionNotFound, got %v", err)
	}
	// delete owner -> ok, then gone
	if err := ss.Delete(ctx, "u1", "s1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := ss.Delete(ctx, "u1", "s1"); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Fatalf("double delete: want ErrSessionNotFound, got %v", err)
	}
}
