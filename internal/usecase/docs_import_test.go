package usecase_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// fakeDocStore is an in-memory DocumentStore for tests.
type fakeDocStore struct {
	docs map[string]ports.Document
}

func newFakeDocStore() *fakeDocStore {
	return &fakeDocStore{docs: make(map[string]ports.Document)}
}

func (s *fakeDocStore) Get(userID, path string) (ports.Document, error) {
	key := userID + ":" + path
	d, ok := s.docs[key]
	if !ok {
		return ports.Document{}, ports.ErrDocumentNotFound
	}
	return d, nil
}

func (s *fakeDocStore) GetByRepoKey(_, _ string) (ports.Document, error) {
	return ports.Document{}, ports.ErrDocumentNotFound
}

func (s *fakeDocStore) List(_, _, _ string, _ int) ([]ports.DocumentEntry, error) {
	return nil, nil
}

func (s *fakeDocStore) Put(userID, path, body, repoKey string, ifMatch int64) (ports.Document, error) {
	key := userID + ":" + path
	existing, exists := s.docs[key]
	if ifMatch == 0 && exists {
		return ports.Document{}, ports.ErrDocumentVersionConflict
	}
	if ifMatch != 0 && !exists {
		return ports.Document{}, ports.ErrDocumentVersionConflict
	}
	if ifMatch != 0 && exists && existing.Version != ifMatch {
		return ports.Document{}, ports.ErrDocumentVersionConflict
	}
	version := int64(1)
	if exists {
		version = existing.Version + 1
	}
	d := ports.Document{
		UserID:  userID,
		Path:    path,
		Body:    body,
		RepoKey: repoKey,
		Version: version,
	}
	s.docs[key] = d
	return d, nil
}

func (s *fakeDocStore) Delete(userID, path string) error {
	delete(s.docs, userID+":"+path)
	return nil
}

func TestDocsImport(t *testing.T) {
	dir := t.TempDir()
	// 2 markdown files
	must(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "b.md"), []byte("# B"), 0o644))
	// 1 non-markdown file (should be skipped)
	must(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("text"), 0o644))
	// hidden dir — should be skipped entirely
	must(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".git", "x.md"), []byte("hidden"), 0o644))

	store := newFakeDocStore()
	uc := &usecase.DocsImport{Docs: store, UserID: "u1"}

	// First run: both .md files created, txt skipped, .git subtree skipped
	res, err := uc.Run(dir, nil)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if res.Created != 2 {
		t.Errorf("created: want 2, got %d", res.Created)
	}
	if res.Skipped != 1 {
		t.Errorf("skipped: want 1 (txt), got %d", res.Skipped)
	}
	if res.Updated != 0 || res.Unchanged != 0 {
		t.Errorf("unexpected updated=%d unchanged=%d", res.Updated, res.Unchanged)
	}

	// Second run (unchanged): both files unchanged
	res, err = uc.Run(dir, nil)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if res.Unchanged != 2 {
		t.Errorf("unchanged: want 2, got %d", res.Unchanged)
	}
	if res.Created != 0 || res.Updated != 0 {
		t.Errorf("unexpected created=%d updated=%d", res.Created, res.Updated)
	}

	// Modify a.md, third run: 1 updated, 1 unchanged
	must(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A modified"), 0o644))
	res, err = uc.Run(dir, nil)
	if err != nil {
		t.Fatalf("third run: %v", err)
	}
	if res.Updated != 1 {
		t.Errorf("updated: want 1, got %d", res.Updated)
	}
	if res.Unchanged != 1 {
		t.Errorf("unchanged: want 1, got %d", res.Unchanged)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
