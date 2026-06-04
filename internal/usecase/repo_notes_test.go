package usecase_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// ---- fakes ----

type fakeRepoStore struct {
	mu     sync.Mutex
	rows   map[string]domain.Repo // keyed by ID
	byKey  map[string]string      // userID|key → repoID
	nextID int
}

func newFakeRepoStore() *fakeRepoStore {
	return &fakeRepoStore{rows: map[string]domain.Repo{}, byKey: map[string]string{}}
}

func (f *fakeRepoStore) EnsureByCanonicalKey(userID, key, displayName string) (domain.Repo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id, ok := f.byKey[userID+"|"+key]; ok {
		return f.rows[id], nil
	}
	f.nextID++
	id := "fake-repo-" + string(rune('a'-1+f.nextID))
	r := domain.Repo{
		ID: id, UserID: userID, CanonicalKey: key, DisplayName: displayName,
	}
	f.rows[id] = r
	f.byKey[userID+"|"+key] = id
	return r, nil
}

func (f *fakeRepoStore) GetByID(userID, id string) (domain.Repo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.rows[id]; ok && r.UserID == userID {
		return r, nil
	}
	return domain.Repo{}, ports.ErrRepoNotFound
}

func (f *fakeRepoStore) Upsert(r domain.Repo) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[r.ID] = r
	f.byKey[r.UserID+"|"+r.CanonicalKey] = r.ID
	return nil
}

func (f *fakeRepoStore) PullSince(userID string, since int64, _ int) ([]domain.Repo, int64, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Repo
	for _, r := range f.rows {
		if r.UserID == userID && r.Version > since {
			out = append(out, r)
		}
	}
	// no sort needed for this test — caller only checks length
	high := since
	if len(out) > 0 {
		high = out[len(out)-1].Version
	}
	return out, high, false, nil
}

type fakeRepoNoteStore struct {
	mu     sync.Mutex
	byID   map[string]domain.RepoNote
	byRepo map[string]string // userID|repoID → noteID
}

func newFakeRepoNoteStore() *fakeRepoNoteStore {
	return &fakeRepoNoteStore{
		byID: map[string]domain.RepoNote{}, byRepo: map[string]string{},
	}
}

func (f *fakeRepoNoteStore) GetByRepo(userID, repoID string) (domain.RepoNote, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id, ok := f.byRepo[userID+"|"+repoID]; ok {
		return f.byID[id], nil
	}
	return domain.RepoNote{}, ports.ErrRepoNoteNotFound
}

func (f *fakeRepoNoteStore) Upsert(n domain.RepoNote) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byID[n.ID] = n
	f.byRepo[n.UserID+"|"+n.RepoID] = n.ID
	return nil
}

func (f *fakeRepoNoteStore) Delete(userID, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n, ok := f.byID[id]; ok && n.UserID == userID {
		delete(f.byID, id)
		delete(f.byRepo, n.UserID+"|"+n.RepoID)
	}
	return nil
}

func (f *fakeRepoNoteStore) PullSince(userID string, since int64, _ int) ([]domain.RepoNote, int64, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.RepoNote
	for _, n := range f.byID {
		if n.UserID == userID && n.Version > since {
			out = append(out, n)
		}
	}
	return out, since, false, nil
}

// ---- tests ----

func TestRepoNotes_GetForPwd_AutoCreatesRepo(t *testing.T) {
	t.Parallel()
	repos := newFakeRepoStore()
	notes := newFakeRepoNoteStore()
	queue := &fakeWriteQueue{}
	uc := usecase.NewRepoNotes(repos, notes, queue, fakeResolverPkg{url: "git@github.com:foo/bar.git", ok: true})

	note, repo, err := uc.GetForPwd("u1", "/some/local/path")
	if err != nil {
		t.Fatalf("GetForPwd: %v", err)
	}
	if repo.CanonicalKey != "git:github.com/foo/bar" {
		t.Errorf("CanonicalKey: %q", repo.CanonicalKey)
	}
	if repo.ID == "" {
		t.Error("expected repo to be created")
	}
	if note.ID != "" {
		t.Errorf("expected zero-value note for new repo, got %+v", note)
	}
}

func TestRepoNotes_Save_FirstWriteGeneratesID(t *testing.T) {
	t.Parallel()
	repos := newFakeRepoStore()
	notes := newFakeRepoNoteStore()
	queue := &fakeWriteQueue{}
	uc := usecase.NewRepoNotes(repos, notes, queue, fakeResolverPkg{url: "git@host:o/r.git", ok: true})

	n, err := uc.Save("u1", "/pwd", "hello")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if n.ID == "" {
		t.Error("ID should be generated")
	}
	if n.Content != "hello" {
		t.Errorf("Content: %q", n.Content)
	}
	if len(queue.entries) != 1 {
		t.Fatalf("queue entries: got %d, want 1", len(queue.entries))
	}
	if queue.entries[0].Resource != "repo_notes" {
		t.Errorf("resource: %q", queue.entries[0].Resource)
	}
}

func TestRepoNotes_Save_PreservesIDOnUpdate(t *testing.T) {
	t.Parallel()
	repos := newFakeRepoStore()
	notes := newFakeRepoNoteStore()
	queue := &fakeWriteQueue{}
	uc := usecase.NewRepoNotes(repos, notes, queue, fakeResolverPkg{url: "git@host:o/r.git", ok: true})

	first, _ := uc.Save("u1", "/pwd", "v1")
	second, _ := uc.Save("u1", "/pwd", "v2")

	if first.ID != second.ID {
		t.Errorf("ID changed on update: %q → %q", first.ID, second.ID)
	}
	if second.Content != "v2" {
		t.Errorf("Content not updated: %q", second.Content)
	}
}

func TestRepoNotes_Save_FailsBubbleUp(t *testing.T) {
	t.Parallel()
	repos := newFakeRepoStore()
	notes := &erroringNoteStore{err: errors.New("disk full"), fakeRepoNoteStore: newFakeRepoNoteStore()}
	queue := &fakeWriteQueue{}
	uc := usecase.NewRepoNotes(repos, notes, queue, fakeResolverPkg{ok: false})

	_, err := uc.Save("u1", "/pwd", "x")
	if err == nil {
		t.Error("expected error to bubble up")
	}
}

// erroringNoteStore wraps fakeRepoNoteStore so we can inject errors on Upsert.
type erroringNoteStore struct {
	*fakeRepoNoteStore
	err error
}

func (e *erroringNoteStore) Upsert(_ domain.RepoNote) error { return e.err }

// fakeResolverPkg replicates the resolver fake in canonical_key_test.go but for
// the *_test package boundary. The real ports.RemoteResolver is unexported in
// usecase but the test sits in usecase_test, so we re-declare a fake here.
type fakeResolverPkg struct {
	url string
	ok  bool
}

func (f fakeResolverPkg) RemoteURL(_ string) (string, bool) { return f.url, f.ok }
