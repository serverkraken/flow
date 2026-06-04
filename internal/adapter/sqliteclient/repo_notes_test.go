package sqliteclient

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func testRepo(t *testing.T, store *Store, userID, key string) domain.Repo {
	t.Helper()
	r, err := NewRepos(store).EnsureByCanonicalKey(userID, key, key)
	if err != nil {
		t.Fatalf("testRepo EnsureByCanonicalKey: %v", err)
	}
	return r
}

func TestUnit_RepoNotes_GetByRepo_NotFound(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	u := testUser(t, store, "note1")
	r := testRepo(t, store, u.ID, "git:example.com/note1")
	notes := NewRepoNotes(store)

	_, err := notes.GetByRepo(u.ID, r.ID)
	if !errors.Is(err, ports.ErrRepoNoteNotFound) {
		t.Errorf("want ErrRepoNoteNotFound, got %v", err)
	}
}

func TestUnit_RepoNotes_Upsert_InsertThenUpdate(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	u := testUser(t, store, "note2")
	r := testRepo(t, store, u.ID, "git:example.com/note2")
	notes := NewRepoNotes(store)

	now := time.Now().UTC().Truncate(time.Second)
	id := uuid.NewString()
	n := domain.RepoNote{
		ID: id, RepoID: r.ID, UserID: u.ID,
		Content: "first content", Version: 1, UpdatedAt: now,
	}
	if err := notes.Upsert(n); err != nil {
		t.Fatalf("Upsert insert: %v", err)
	}
	n.Content = "second content"
	n.Version = 5
	if err := notes.Upsert(n); err != nil {
		t.Fatalf("Upsert update: %v", err)
	}

	got, err := notes.GetByRepo(u.ID, r.ID)
	if err != nil {
		t.Fatalf("GetByRepo: %v", err)
	}
	if got.ID != id {
		t.Errorf("ID changed: %q vs %q", got.ID, id)
	}
	if got.Content != "second content" {
		t.Errorf("Content not updated: %q", got.Content)
	}
	if got.Version != 5 {
		t.Errorf("Version not updated: %d", got.Version)
	}
}

func TestUnit_RepoNotes_Delete_RemovesRow(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	u := testUser(t, store, "note3")
	r := testRepo(t, store, u.ID, "git:example.com/note3")
	notes := NewRepoNotes(store)

	id := uuid.NewString()
	if err := notes.Upsert(domain.RepoNote{
		ID: id, RepoID: r.ID, UserID: u.ID, Content: "x", Version: 1, UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := notes.Delete(u.ID, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := notes.GetByRepo(u.ID, r.ID)
	if !errors.Is(err, ports.ErrRepoNoteNotFound) {
		t.Errorf("want ErrRepoNoteNotFound after Delete, got %v", err)
	}
}

func TestUnit_RepoNotes_PullSince_ReturnsOnlyGreaterVersions(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	u := testUser(t, store, "note4")
	notes := NewRepoNotes(store)

	now := time.Now().UTC()
	for i, v := range []int64{2, 4, 6, 8} {
		r := testRepo(t, store, u.ID, "git:example.com/pull"+string(rune('a'+i)))
		n := domain.RepoNote{
			ID: uuid.NewString(), RepoID: r.ID, UserID: u.ID,
			Content: "n" + string(rune('a'+i)), Version: v, UpdatedAt: now,
		}
		if err := notes.Upsert(n); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	got, high, hasMore, err := notes.PullSince(u.ID, 4, 10)
	if err != nil {
		t.Fatalf("PullSince: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2 (versions 6 + 8)", len(got))
	}
	if high != 8 {
		t.Errorf("high = %d, want 8", high)
	}
	if hasMore {
		t.Error("hasMore = true, want false")
	}
}
