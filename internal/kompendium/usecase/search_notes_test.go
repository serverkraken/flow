package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

const searchTestUser = "u1"

// seedSearchDoc puts a raw markdown document with frontmatter into the fake
// doc store so SearchNotes can discover it via DocumentStore.List.
func seedSearchDoc(t *testing.T, ds *testutil.FakeDocStore, id string) {
	t.Helper()
	body := "---\nid: " + id + "\ntype: free\ntitle: title for " + id + "\n---\nbody of " + id + "\n"
	_, err := ds.Put(searchTestUser, id+".md", body, "", 0)
	if err != nil {
		t.Fatalf("seedSearchDoc(%q): %v", id, err)
	}
}

func TestSearchNotes_ReturnsMappedEntries(t *testing.T) {
	t.Parallel()

	ds := testutil.NewFakeDocStore()
	seedSearchDoc(t, ds, "daily/2026-04-25")
	seedSearchDoc(t, ds, "notes/setup")

	u := usecase.NewSearchNotes(ds, searchTestUser)
	got, err := u.Execute(context.Background(), usecase.SearchNotesInput{
		Text: "setup",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// FakeDocStore ignores query text — both entries are returned and mapped.
	if len(got) != 2 {
		t.Errorf("expected 2 results, got %d: %+v", len(got), got)
	}
	for _, r := range got {
		if r.ID == "" {
			t.Errorf("result with empty ID: %+v", r)
		}
	}
}

func TestSearchNotes_Empty(t *testing.T) {
	t.Parallel()
	ds := testutil.NewFakeDocStore()
	u := usecase.NewSearchNotes(ds, searchTestUser)
	got, err := u.Execute(context.Background(), usecase.SearchNotesInput{Text: "anything"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty result, got %d", len(got))
	}
}

func TestSearchNotes_StoreError(t *testing.T) {
	t.Parallel()
	forced := errors.New("store down")
	ds := testutil.NewFakeDocStore()
	ds.ListErr = forced

	u := usecase.NewSearchNotes(ds, searchTestUser)
	_, err := u.Execute(context.Background(), usecase.SearchNotesInput{Text: "q"})
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want %v", err, forced)
	}
}

func TestSearchNotes_SkipsNonMarkdown(t *testing.T) {
	t.Parallel()

	ds := testutil.NewFakeDocStore()
	// Put a .md note and a repos/ document (no .md suffix used for ID, but
	// FakeDocStore uses Path verbatim — seed with a non-.md path to simulate).
	// Note: FakeDocStore.List returns docs keyed by userID:path; store a note
	// with a non-.md path to verify the .md filter.
	_, err := ds.Put(searchTestUser, "notes/readme", "body without frontmatter", "", 0)
	if err != nil {
		t.Fatalf("seed non-md: %v", err)
	}
	seedSearchDoc(t, ds, "notes/real")

	u := usecase.NewSearchNotes(ds, searchTestUser)
	got, err := u.Execute(context.Background(), usecase.SearchNotesInput{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Only notes/real.md should appear; notes/readme has no .md suffix.
	for _, r := range got {
		if r.ID == "notes/readme" {
			t.Errorf("non-.md path leaked into results: %+v", r)
		}
	}
}
