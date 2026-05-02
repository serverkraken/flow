package sqliteindex_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

func TestUpsert_AndSearchByText(t *testing.T) {
	t.Parallel()
	idx := newIdx(t)

	if err := idx.Upsert(context.Background(),
		makeNote(t, "daily/2026-04-25", "kompendium architecture body"), unix(2)); err != nil {
		t.Fatal(err)
	}
	if err := idx.Upsert(context.Background(),
		makeNoteWithType(t, "notes/setup", domain.TypeFree, "", "no match here"), unix(1)); err != nil {
		t.Fatal(err)
	}

	got, err := idx.Search(context.Background(), domain.SearchQuery{Text: "kompendium"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "daily/2026-04-25" {
		t.Errorf("got %+v, want one result for 'kompendium' on daily/2026-04-25", got)
	}
}

func TestUpsert_ReplacesExistingRow(t *testing.T) {
	t.Parallel()
	idx := newIdx(t)
	ctx := context.Background()

	if err := idx.Upsert(ctx, makeNote(t, "daily/2026-04-25", "first version body"), unix(1)); err != nil {
		t.Fatal(err)
	}
	if err := idx.Upsert(ctx, makeNote(t, "daily/2026-04-25", "second version body"), unix(2)); err != nil {
		t.Fatal(err)
	}

	got, err := idx.Search(ctx, domain.SearchQuery{Text: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("first version should have been replaced, got %+v", got)
	}
	got, err = idx.Search(ctx, domain.SearchQuery{Text: "second"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("second version not found, got %+v", got)
	}
}

func TestUpsert_DedupesDuplicateLinks(t *testing.T) {
	t.Parallel()
	idx := newIdx(t)
	ctx := context.Background()

	src := makeNoteAtID(t, "daily/2026-04-25", "[[notes/setup]] [[notes/setup]] [[notes/setup|setup]]")
	_ = idx.Upsert(ctx, src, unix(1))

	links, err := idx.LinksFrom(ctx, "daily/2026-04-25")
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].ID != "notes/setup" {
		t.Errorf("links not deduplicated: %+v", links)
	}
}

func TestDelete(t *testing.T) {
	t.Parallel()
	idx := newIdx(t)
	ctx := context.Background()

	_ = idx.Upsert(ctx, makeNote(t, "daily/2026-04-25", "alpha"), unix(1))
	if err := idx.Delete(ctx, "daily/2026-04-25"); err != nil {
		t.Fatal(err)
	}

	got, err := idx.Search(ctx, domain.SearchQuery{Text: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected no results after Delete, got %+v", got)
	}
}

func TestDelete_MissingIsNoOp(t *testing.T) {
	t.Parallel()
	idx := newIdx(t)
	if err := idx.Delete(context.Background(), "nonexistent/id"); err != nil {
		t.Errorf("Delete on missing id should be a no-op, got %v", err)
	}
}

func TestRebuild(t *testing.T) {
	t.Parallel()
	idx := newIdx(t)
	ctx := context.Background()

	_ = idx.Upsert(ctx, makeNote(t, "daily/2026-04-22", "stale entry"), unix(1))

	all := []ports.IndexEntry{
		{Note: makeNote(t, "daily/2026-04-25", "fresh body"), Mtime: unix(10)},
		{Note: makeNote(t, "notes/setup", "another fresh"), Mtime: unix(11)},
	}
	if err := idx.Rebuild(ctx, all); err != nil {
		t.Fatal(err)
	}

	got, err := idx.Search(ctx, domain.SearchQuery{Text: "stale"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("Rebuild left old entry behind: %+v", got)
	}
	got, err = idx.Search(ctx, domain.SearchQuery{Text: "fresh"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 fresh entries, got %d", len(got))
	}
}
