package sqliteserver

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func serverTestRepo(t *testing.T, store *Store, userID, key string) domain.Repo {
	t.Helper()
	r, err := NewRepos(store).EnsureByCanonicalKey(userID, key, key)
	if err != nil {
		t.Fatalf("serverTestRepo: %v", err)
	}
	return r
}

func TestUnit_ServerRepoNotes_Upsert_InsertBumpsVersion(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "snote1")
	r := serverTestRepo(t, store, u.ID, "git:x/note1")
	notes := NewRepoNotes(store)

	got, err := notes.Upsert(domain.RepoNote{
		ID: uuid.NewString(), RepoID: r.ID, UserID: u.ID, Content: "hi",
	}, 0)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if got.Version == 0 {
		t.Error("Version should be non-zero after Upsert")
	}
}

func TestUnit_ServerRepoNotes_Upsert_UpdatePreservesIdentity(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "snote2")
	r := serverTestRepo(t, store, u.ID, "git:x/note2")
	notes := NewRepoNotes(store)

	id := uuid.NewString()
	first, err := notes.Upsert(domain.RepoNote{
		ID: id, RepoID: r.ID, UserID: u.ID, Content: "first",
	}, 0)
	if err != nil {
		t.Fatalf("first Upsert: %v", err)
	}
	second, err := notes.Upsert(domain.RepoNote{
		ID: id, RepoID: r.ID, UserID: u.ID, Content: "second",
	}, first.Version)
	if err != nil {
		t.Fatalf("second Upsert: %v", err)
	}
	if second.ID != id {
		t.Errorf("ID changed: %q", second.ID)
	}
	if second.Version <= first.Version {
		t.Errorf("version must bump on update: %d ≤ %d", second.Version, first.Version)
	}
}

func TestUnit_ServerRepoNotes_Upsert_StaleVersion_Conflict(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "snote3")
	r := serverTestRepo(t, store, u.ID, "git:x/note3")
	notes := NewRepoNotes(store)

	id := uuid.NewString()
	first, err := notes.Upsert(domain.RepoNote{
		ID: id, RepoID: r.ID, UserID: u.ID, Content: "first",
	}, 0)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	_, err = notes.Upsert(domain.RepoNote{
		ID: id, RepoID: r.ID, UserID: u.ID, Content: "stale-attempt",
	}, first.Version+99)
	if !errors.Is(err, ports.ErrRepoNoteVersionConflict) {
		t.Errorf("want ErrRepoNoteVersionConflict, got %v", err)
	}
}

func TestUnit_ServerRepoNotes_Upsert_BogusRepoID_Error(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "snote4")
	notes := NewRepoNotes(store)

	_, err := notes.Upsert(domain.RepoNote{
		ID: uuid.NewString(), RepoID: "nope", UserID: u.ID, Content: "x",
	}, 0)
	if err == nil {
		t.Error("expected error for nonexistent repo")
	}
}

func TestUnit_ServerRepoNotes_GetByRepo_NotFound(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "snote5")
	r := serverTestRepo(t, store, u.ID, "git:x/note5")
	notes := NewRepoNotes(store)

	_, err := notes.GetByRepo(u.ID, r.ID)
	if !errors.Is(err, ports.ErrRepoNoteNotFound) {
		t.Errorf("want ErrRepoNoteNotFound, got %v", err)
	}
}
