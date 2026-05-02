package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func TestSearchNotes_PassesQuery(t *testing.T) {
	t.Parallel()

	idx := testutil.NewFakeIndexer()
	_ = idx.Upsert(context.Background(), mustNote("daily/2026-04-25", domain.TypeDaily, ""), tm(1))
	_ = idx.Upsert(context.Background(), mustNote("notes/setup", domain.TypeFree, ""), tm(2))

	u := usecase.NewSearchNotes(idx)
	got, err := u.Execute(context.Background(), usecase.SearchNotesInput{
		Text: "title for",
		Type: domain.TypeDaily,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got) != 1 || got[0].ID != "daily/2026-04-25" {
		t.Errorf("filter not respected, got %+v", got)
	}
}

func TestSearchNotes_Empty(t *testing.T) {
	t.Parallel()
	u := usecase.NewSearchNotes(testutil.NewFakeIndexer())
	got, err := u.Execute(context.Background(), usecase.SearchNotesInput{Text: "anything"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty result, got %d", len(got))
	}
}

func TestSearchNotes_IndexerError(t *testing.T) {
	t.Parallel()
	forced := errors.New("indexer down")
	idx := testutil.NewFakeIndexer()
	idx.SearchErr = forced

	u := usecase.NewSearchNotes(idx)
	_, err := u.Execute(context.Background(), usecase.SearchNotesInput{Text: "q"})
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want %v", err, forced)
	}
}
