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

func TestListNotes_Empty(t *testing.T) {
	t.Parallel()
	u := usecase.NewListNotes(testutil.NewFakeNoteStore())

	got, err := u.Execute(context.Background(), usecase.ListNotesInput{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty result, got %d", len(got))
	}
}

func TestListNotes_TierOrdering(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	// Three tiers, two entries each, with distinct mtimes so order within a
	// tier is deterministic.
	store.Seed(makeProject("projects/github.com/foo/bar/2026-04-22", "github.com/foo/bar"), tm(1))
	store.Seed(makeProject("projects/github.com/foo/bar/2026-04-25", "github.com/foo/bar"), tm(2))
	store.Seed(makeDaily("daily/2026-04-22"), tm(3))
	store.Seed(makeDaily("daily/2026-04-25"), tm(4))
	store.Seed(makeFree("notes/setup"), tm(5))
	store.Seed(makeProject("projects/github.com/other/baz/2026-04-25", "github.com/other/baz"), tm(6))

	u := usecase.NewListNotes(store)
	got, err := u.Execute(context.Background(), usecase.ListNotesInput{
		CurrentRepo: domain.CanonicalURL("github.com/foo/bar"),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	wantOrder := []domain.ID{
		// Tier 0: current-repo project notes, newest first.
		"projects/github.com/foo/bar/2026-04-25",
		"projects/github.com/foo/bar/2026-04-22",
		// Tier 1: all daily notes, newest first.
		"daily/2026-04-25",
		"daily/2026-04-22",
		// Tier 2: rest, newest first.
		"projects/github.com/other/baz/2026-04-25",
		"notes/setup",
	}
	if len(got) != len(wantOrder) {
		t.Fatalf("got %d entries, want %d", len(got), len(wantOrder))
	}
	for i, want := range wantOrder {
		if got[i].ID != want {
			t.Errorf("position %d: got %q, want %q", i, got[i].ID, want)
		}
	}
}

func TestListNotes_NoCurrentRepo(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	store.Seed(makeProject("projects/p/2026-04-25", "p"), tm(2))
	store.Seed(makeDaily("daily/2026-04-25"), tm(1))

	u := usecase.NewListNotes(store)
	got, err := u.Execute(context.Background(), usecase.ListNotesInput{})
	if err != nil {
		t.Fatal(err)
	}
	// Without CurrentRepo, all projects fall to tier 2; daily wins tier 1.
	if got[0].ID != "daily/2026-04-25" {
		t.Errorf("expected daily first when no CurrentRepo, got %q", got[0].ID)
	}
}

func TestListNotes_FilterPassesThrough(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	store.Seed(makeDaily("daily/2026-04-25"), tm(1))
	store.Seed(makeProject("projects/p/2026-04-25", "p"), tm(2))

	u := usecase.NewListNotes(store)
	got, err := u.Execute(context.Background(), usecase.ListNotesInput{Type: domain.TypeDaily})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Meta.Type != domain.TypeDaily {
		t.Errorf("type filter not honoured, got %+v", got)
	}
}

func TestListNotes_LimitAfterTiering(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	store.Seed(makeProject("projects/p/2026-04-22", "p"), tm(1))
	store.Seed(makeDaily("daily/2026-04-25"), tm(2))
	store.Seed(makeProject("projects/p/2026-04-25", "p"), tm(3))

	u := usecase.NewListNotes(store)
	got, err := u.Execute(context.Background(), usecase.ListNotesInput{
		CurrentRepo: domain.CanonicalURL("p"),
		Limit:       2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries (limit), got %d", len(got))
	}
	// First two should both be tier-0 project notes for "p", newest first.
	if got[0].ID != "projects/p/2026-04-25" || got[1].ID != "projects/p/2026-04-22" {
		t.Errorf("tier-aware limit broken, got %+v", got)
	}
}

func TestListNotes_StoreError(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced store error")
	store := testutil.NewFakeNoteStore()
	store.ListErr = forced

	u := usecase.NewListNotes(store)
	_, err := u.Execute(context.Background(), usecase.ListNotesInput{})
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want wrapped %v", err, forced)
	}
}

// --- helpers ----------------------------------------------------------------

func tm(s int64) time.Time { return time.Unix(s, 0) }

func makeDaily(id string) domain.Note {
	return mustNote(domain.ID(id), domain.TypeDaily, "")
}

func makeProject(id, project string) domain.Note {
	return mustNote(domain.ID(id), domain.TypeProject, project)
}

func makeFree(id string) domain.Note {
	return mustNote(domain.ID(id), domain.TypeFree, "")
}

func mustNote(id domain.ID, typ domain.NoteType, project string) domain.Note {
	fm := domain.Frontmatter{
		ID:      id.String(),
		Type:    typ,
		Project: project,
		Title:   "title for " + id.String(),
	}
	n, err := domain.NewNote(id, fm, []byte("body of "+id.String()+"\n"))
	if err != nil {
		panic(err)
	}
	return n
}
