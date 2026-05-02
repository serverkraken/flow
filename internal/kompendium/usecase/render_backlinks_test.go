package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func TestRenderBacklinks_HappyPath(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	idx := testutil.NewFakeIndexer()

	target := mustNote("notes/setup", domain.TypeFree, "")
	src1 := mustNoteWithBody("daily/2026-04-25", domain.TypeDaily, "", "see [[notes/setup]] for context")
	src2 := mustNoteWithBody("daily/2026-04-22", domain.TypeDaily, "", "[[notes/setup]] also referenced here")

	store.Seed(target, tm(1))
	store.Seed(src1, tm(2))
	store.Seed(src2, tm(3))
	_ = idx.Upsert(context.Background(), target, tm(1))
	_ = idx.Upsert(context.Background(), src1, tm(2))
	_ = idx.Upsert(context.Background(), src2, tm(3))

	u := usecase.NewRenderBacklinks(store, idx)
	out, err := u.Execute(context.Background(), usecase.RenderBacklinksInput{NoteID: "notes/setup"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Note.ID != "notes/setup" {
		t.Errorf("Note got %q", out.Note.ID)
	}
	if len(out.Backlinks) != 2 {
		t.Fatalf("expected 2 backlinks, got %d", len(out.Backlinks))
	}
	for _, b := range out.Backlinks {
		if b.Title == "" {
			t.Errorf("backlink title should be populated: %+v", b)
		}
	}
}

func TestRenderBacklinks_NoteNotFound(t *testing.T) {
	t.Parallel()
	u := usecase.NewRenderBacklinks(testutil.NewFakeNoteStore(), testutil.NewFakeIndexer())
	_, err := u.Execute(context.Background(), usecase.RenderBacklinksInput{NoteID: "missing"})
	if !errors.Is(err, ports.ErrNoteNotFound) {
		t.Errorf("got %v, want ErrNoteNotFound", err)
	}
}

func TestRenderBacklinks_IndexError(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced backlink error")
	store := testutil.NewFakeNoteStore()
	store.Seed(mustNote("notes/setup", domain.TypeFree, ""), tm(1))
	idx := testutil.NewFakeIndexer()
	idx.BacklinksErr = forced

	u := usecase.NewRenderBacklinks(store, idx)
	_, err := u.Execute(context.Background(), usecase.RenderBacklinksInput{NoteID: "notes/setup"})
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want %v", err, forced)
	}
}

// TestRenderBacklinks_SurfacesIndexerTitles locks in the new contract:
// backlink titles come straight from the indexer's join, no per-link
// store lookup. A backlink whose source is missing from the store but
// present in the index still surfaces — the right place to fix that
// inconsistency is `kompendium index rebuild`, not by hiding the
// reference at read time.
func TestRenderBacklinks_SurfacesIndexerTitles(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	idx := testutil.NewFakeIndexer()

	target := mustNote("notes/setup", domain.TypeFree, "")
	src := mustNoteWithBody("daily/2026-04-25", domain.TypeDaily, "", "[[notes/setup]]")
	store.Seed(target, tm(1))
	// Note: src deliberately NOT seeded into the store. The old code
	// did a per-link Store.Get and dropped it. The new code resolves
	// title via the indexer's join and surfaces the entry.
	_ = idx.Upsert(context.Background(), src, tm(2))

	u := usecase.NewRenderBacklinks(store, idx)
	out, err := u.Execute(context.Background(), usecase.RenderBacklinksInput{NoteID: "notes/setup"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(out.Backlinks) != 1 {
		t.Fatalf("expected 1 backlink, got %d: %+v", len(out.Backlinks), out.Backlinks)
	}
	if out.Backlinks[0].ID != "daily/2026-04-25" {
		t.Errorf("unexpected backlink id: %+v", out.Backlinks[0])
	}
}

func mustNoteWithBody(id string, typ domain.NoteType, project, body string) domain.Note {
	fm := domain.Frontmatter{
		ID:      id,
		Type:    typ,
		Project: project,
		Title:   "title for " + id,
	}
	n, err := domain.NewNote(domain.ID(id), fm, []byte(body))
	if err != nil {
		panic(err)
	}
	return n
}
