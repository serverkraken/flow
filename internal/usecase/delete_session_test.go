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

func TestDeleteSession_ClearsTaggings(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	tags := testutil.NewFakeTagStore()
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	stop := start.Add(time.Hour)
	if _, err := ss.Create(ctx, domain.WorkSession{ID: "s1", OwnerID: "u1", Start: start, Stop: &stop}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := tags.SetTags(ctx, "u1", domain.TaggableWorkSession, "s1", []string{"meeting", "deep"}); err != nil {
		t.Fatalf("set tags: %v", err)
	}

	uc := usecase.DeleteSession{Sessions: ss, Tags: tags}
	if err := uc.Execute(ctx, "u1", "s1"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	got, err := tags.TagsFor(ctx, "u1", domain.TaggableWorkSession, "s1")
	if err != nil {
		t.Fatalf("tags for: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no taggings after delete, got %d: %+v", len(got), got)
	}
}
