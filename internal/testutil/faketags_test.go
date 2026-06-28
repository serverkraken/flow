package testutil_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
)

func TestFakeTagStore_SetThenFilterAnd(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := testutil.NewFakeTagStore()
	if _, err := ts.SetTags(ctx, "u1", domain.TaggableDocument, "d1", []string{"Go", "tui"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.SetTags(ctx, "u1", domain.TaggableDocument, "d2", []string{"go"}); err != nil {
		t.Fatal(err)
	}
	ids, err := ts.FilterIDs(ctx, "u1", domain.TaggableDocument, []string{"go", "tui"}, domain.TagMatchAll)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "d1" {
		t.Fatalf("AND filter want [d1], got %v", ids)
	}
}

func TestFakeTagStore_SetReplacesAndHydrates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := testutil.NewFakeTagStore()
	_, _ = ts.SetTags(ctx, "u1", domain.TaggableWorkSession, "s1", []string{"deep"})
	_, _ = ts.SetTags(ctx, "u1", domain.TaggableWorkSession, "s1", []string{"meeting", "deep"})
	got, err := ts.TagsFor(ctx, "u1", domain.TaggableWorkSession, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 tags after replace, got %d: %+v", len(got), got)
	}
}

func TestFakeTagStore_DisplayFirstWriteWins(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := testutil.NewFakeTagStore()
	_, _ = ts.SetTags(ctx, "u1", domain.TaggableDocument, "d1", []string{"Go"})
	_, _ = ts.SetTags(ctx, "u1", domain.TaggableDocument, "d2", []string{"go"}) // later, lowercased
	got, _ := ts.TagsFor(ctx, "u1", domain.TaggableDocument, "d2")
	if len(got) != 1 || got[0].Display != "Go" {
		t.Fatalf("display should be first-seen raw 'Go', got %+v", got)
	}
}
