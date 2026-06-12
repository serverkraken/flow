package apistore_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/adapter/apistore"
	"github.com/serverkraken/flow/internal/kompendium/domain"
	kompports "github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/ports"
)

const testUser = "u1"

// makeNote builds a valid domain.Note for testing.
func makeNote(t *testing.T, id, title string, typ domain.NoteType, project string) domain.Note {
	t.Helper()
	parsed, err := domain.ParseID(id)
	if err != nil {
		t.Fatalf("ParseID(%q): %v", id, err)
	}
	fm := domain.Frontmatter{
		ID:      parsed.String(),
		Type:    typ,
		Title:   title,
		Project: project,
	}
	n, err := domain.NewNote(parsed, fm, []byte("body of "+id+"\n"))
	if err != nil {
		t.Fatalf("NewNote: %v", err)
	}
	return n
}

// TestRoundtrip verifies Put→Get→List(filter)→Delete.
func TestRoundtrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ds := testutil.NewFakeDocStore()
	s := apistore.New(ds, testUser)

	n := makeNote(t, "notes/hello", "Hello", domain.TypeFree, "")

	// Put (create)
	if err := s.Put(ctx, n); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Exists
	ok, err := s.Exists(ctx, n.ID)
	if err != nil || !ok {
		t.Errorf("Exists after Put: (%v, %v)", ok, err)
	}

	// Get
	got, err := s.Get(ctx, n.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != n.ID {
		t.Errorf("Get: ID = %q, want %q", got.ID, n.ID)
	}
	if got.Meta.Title != n.Meta.Title {
		t.Errorf("Get: Title = %q, want %q", got.Meta.Title, n.Meta.Title)
	}

	// List with type filter — should find the note
	entries, err := s.List(ctx, kompports.ListFilter{Type: domain.TypeFree})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("List: got %d entries, want 1", len(entries))
	}
	if entries[0].ID != n.ID {
		t.Errorf("List: entry ID = %q, want %q", entries[0].ID, n.ID)
	}

	// List with non-matching type filter — should return empty
	daily, err := s.List(ctx, kompports.ListFilter{Type: domain.TypeDaily})
	if err != nil {
		t.Fatalf("List daily: %v", err)
	}
	if len(daily) != 0 {
		t.Errorf("List daily filter: want 0, got %d", len(daily))
	}

	// Delete
	if err := s.Delete(ctx, n.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Get after delete — should be ErrNoteNotFound
	_, err = s.Get(ctx, n.ID)
	if !errors.Is(err, kompports.ErrNoteNotFound) {
		t.Errorf("Get after Delete: want ErrNoteNotFound, got %v", err)
	}

	// Exists after delete
	ok2, err := s.Exists(ctx, n.ID)
	if err != nil || ok2 {
		t.Errorf("Exists after Delete: (%v, %v)", ok2, err)
	}
}

// TestVersionConflict verifies that a Put conflict invalidates the corpus
// so the next operation re-reads from the server.
func TestVersionConflict(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ds := testutil.NewFakeDocStore()
	s := apistore.New(ds, testUser)

	n := makeNote(t, "notes/conflict", "Conflict", domain.TypeFree, "")

	// First Put succeeds.
	if err := s.Put(ctx, n); err != nil {
		t.Fatalf("initial Put: %v", err)
	}

	// Simulate a concurrent write by directly inserting a higher version into
	// the fake store. We do this by calling Put on the FakeDocStore directly.
	// The cache still holds version 1; the server now has version 2.
	// We achieve this by calling Put again on the underlying doc store
	// bypassing the apistore (which would bump version correctly). Instead,
	// we use the apistore's Put twice: second direct put on doc store to
	// advance the version past what the cache knows.
	body2 := "---\nid: notes/conflict\ntype: free\ntitle: Conflict v2\n---\nbody v2\n"
	existing, _ := ds.Get(testUser, "notes/conflict.md")
	_, err := ds.Put(testUser, "notes/conflict.md", body2, "", existing.Version)
	if err != nil {
		t.Fatalf("direct ds.Put to simulate concurrent write: %v", err)
	}

	// Now apistore cache has version 1 but server has version 2.
	// A Put through apistore with ifMatch=1 should get ErrDocumentVersionConflict.
	n2 := makeNote(t, "notes/conflict", "Conflict update", domain.TypeFree, "")
	err = s.Put(ctx, n2)
	if err == nil {
		t.Fatal("expected version conflict error")
	}
	if !errors.Is(err, ports.ErrDocumentVersionConflict) {
		t.Errorf("expected ErrDocumentVersionConflict, got: %v", err)
	}

	// After the conflict, corpus should be stale. Next Get should reload
	// and return the server's current version (v2 content).
	got, err := s.Get(ctx, n.ID)
	if err != nil {
		t.Fatalf("Get after conflict: %v", err)
	}
	if got.Meta.Title != "Conflict v2" {
		t.Errorf("Get after conflict reload: Title = %q, want %q", got.Meta.Title, "Conflict v2")
	}
}

// TestReposPrefixExcluded ensures that documents under the "repos/" path
// prefix are not visible in the corpus.
func TestReposPrefixExcluded(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ds := testutil.NewFakeDocStore()

	// Insert a repos/ document directly into the fake store.
	repoBody := "---\nid: repos/owner/proj\ntype: project\nproject: github.com/owner/proj\ntitle: Repo Note\n---\nbody\n"
	_, err := ds.Put(testUser, "repos/owner/proj.md", repoBody, "", 0)
	if err != nil {
		t.Fatalf("seed repos doc: %v", err)
	}

	// Insert a normal note too.
	noteBody := "---\nid: notes/visible\ntype: free\ntitle: Visible\n---\nbody\n"
	_, err = ds.Put(testUser, "notes/visible.md", noteBody, "", 0)
	if err != nil {
		t.Fatalf("seed normal doc: %v", err)
	}

	s := apistore.New(ds, testUser)
	entries, err := s.List(ctx, kompports.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, e := range entries {
		if e.ID == "repos/owner/proj" {
			t.Errorf("repos/ entry should not appear in List, but got %q", e.ID)
		}
	}
	if len(entries) != 1 {
		t.Errorf("List: want 1 entry (normal note), got %d", len(entries))
	}
}

// TestOfflineFallback verifies that when the second List call to the doc store
// fails, the store returns the cached corpus rather than an error.
func TestOfflineFallback(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ds := testutil.NewFakeDocStore()

	// Seed a note.
	noteBody := "---\nid: notes/cached\ntype: free\ntitle: Cached\n---\nbody\n"
	_, err := ds.Put(testUser, "notes/cached.md", noteBody, "", 0)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	s := apistore.New(ds, testUser)

	// First List: populates corpus.
	entries, err := s.List(ctx, kompports.ListFilter{})
	if err != nil {
		t.Fatalf("first List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("first List: want 1 entry, got %d", len(entries))
	}

	// Now make the next List on the doc store fail.
	ds.ListErr = errors.New("network down")

	// Invalidate to force a reload attempt.
	s.Invalidate()

	// Second List: doc store List fails, but we have a cached corpus → fallback.
	entries2, err := s.List(ctx, kompports.ListFilter{})
	if err != nil {
		t.Fatalf("fallback List: expected nil error, got %v", err)
	}
	if len(entries2) != 1 {
		t.Errorf("fallback List: want 1 cached entry, got %d", len(entries2))
	}
	if entries2[0].ID != domain.ID("notes/cached") {
		t.Errorf("fallback List: unexpected entry %q", entries2[0].ID)
	}
}

// TestListSortAndLimit verifies mtime-DESC sort and Limit trimming.
// FakeDocStore uses a monotonic counter for UpdatedAt so insert order (a, b, c)
// guarantees c has the newest timestamp without any time.Sleep.
func TestListSortAndLimit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ds := testutil.NewFakeDocStore()

	for _, id := range []string{"notes/a", "notes/b", "notes/c"} {
		body := "---\nid: " + id + "\ntype: free\ntitle: " + id + "\n---\nbody\n"
		_, err := ds.Put(testUser, id+".md", body, "", 0)
		if err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}

	s := apistore.New(ds, testUser)
	all, err := s.List(ctx, kompports.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("want 3 entries, got %d", len(all))
	}
	// Most recent (c) should come first.
	if all[0].ID != "notes/c" {
		t.Errorf("first entry: want notes/c, got %q", all[0].ID)
	}

	// Limit to 2.
	limited, err := s.List(ctx, kompports.ListFilter{Limit: 2})
	if err != nil {
		t.Fatalf("List limited: %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("Limit=2: got %d", len(limited))
	}
}
