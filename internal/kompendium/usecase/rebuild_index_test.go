package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func TestRebuildIndex_HappyPath(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	index := testutil.NewFakeIndexer()

	store.Seed(mustNoteFull("daily/2026-04-25", domain.TypeDaily, "", "2026-04-25"), time.Unix(2, 0))
	store.Seed(mustNoteFull("daily/2026-04-22", domain.TypeDaily, "", "2026-04-22"), time.Unix(1, 0))

	u := usecase.NewRebuildIndex(store, index)
	out, err := u.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Indexed != 2 {
		t.Errorf("Indexed got %d, want 2", out.Indexed)
	}
	if len(out.Errors) != 0 {
		t.Errorf("expected no errors, got %+v", out.Errors)
	}

	// Verify entries actually landed: a search for the seeded body matches.
	got, err := index.Search(context.Background(), domain.SearchQuery{Text: "body"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) == 0 {
		t.Errorf("expected the rebuild to populate the index, but search returned nothing")
	}
}

func TestRebuildIndex_ListError(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced list err")
	store := testutil.NewFakeNoteStore()
	store.ListErr = forced

	u := usecase.NewRebuildIndex(store, testutil.NewFakeIndexer())
	_, err := u.Execute(context.Background())
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want %v", err, forced)
	}
}

func TestRebuildIndex_RebuildError(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced rebuild err")
	store := testutil.NewFakeNoteStore()
	store.Seed(mustNoteFull("daily/2026-04-25", domain.TypeDaily, "", "2026-04-25"), time.Unix(1, 0))
	idx := testutil.NewFakeIndexer()
	idx.RebuildErr = forced

	u := usecase.NewRebuildIndex(store, idx)
	_, err := u.Execute(context.Background())
	if !errors.Is(err, forced) {
		t.Errorf("got %v", err)
	}
}

func TestRebuildIndex_GetErrorIsCollected(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	store.Seed(mustNoteFull("daily/2026-04-25", domain.TypeDaily, "", "2026-04-25"), time.Unix(1, 0))
	store.GetErr = errors.New("forced get err")

	u := usecase.NewRebuildIndex(store, testutil.NewFakeIndexer())
	out, err := u.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Indexed != 0 {
		t.Errorf("Indexed got %d, want 0 when every Get fails", out.Indexed)
	}
	if len(out.Errors) != 1 {
		t.Errorf("Errors got %+v, want 1", out.Errors)
	}
}
