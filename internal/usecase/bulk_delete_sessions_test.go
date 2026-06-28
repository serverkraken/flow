package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
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

func TestBulkDeleteSessions_ClearsTaggings(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	tags := testutil.NewFakeTagStore()
	seedSess(t, ss, "a", "u1")
	seedSess(t, ss, "b", "u1")
	if _, err := tags.SetTags(ctx, "u1", domain.TaggableWorkSession, "a", []string{"alpha"}); err != nil {
		t.Fatalf("set tags a: %v", err)
	}
	if _, err := tags.SetTags(ctx, "u1", domain.TaggableWorkSession, "b", []string{"beta"}); err != nil {
		t.Fatalf("set tags b: %v", err)
	}

	uc := usecase.BulkDeleteSessions{Sessions: ss, Tags: tags}
	if _, err := uc.Execute(ctx, "u1", []string{"a", "b"}); err != nil {
		t.Fatalf("execute: %v", err)
	}

	for _, id := range []string{"a", "b"} {
		got, err := tags.TagsFor(ctx, "u1", domain.TaggableWorkSession, id)
		if err != nil {
			t.Fatalf("tags for %s: %v", id, err)
		}
		if len(got) != 0 {
			t.Fatalf("expected no taggings for %s after bulk delete, got %d: %+v", id, len(got), got)
		}
	}
}
