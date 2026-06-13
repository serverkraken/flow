package testutil

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	kompports "github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/ports"
)

// FakeDocStore is an in-memory ports.DocumentStore for tests.
//
// Set ListErr to make the next List call return that error (cleared after use).
type FakeDocStore struct {
	mu      sync.Mutex
	docs    map[string]ports.Document // key: "userID:path"
	version int64
	counter int64 // monotonic tick for stable UpdatedAt ordering

	// ListErr, if non-nil, is returned by the next List call and then cleared.
	ListErr error
}

// NewFakeDocStore returns an empty FakeDocStore.
func NewFakeDocStore() *FakeDocStore {
	return &FakeDocStore{docs: make(map[string]ports.Document)}
}

func (s *FakeDocStore) key(userID, path string) string { return userID + ":" + path }

// Get implements ports.DocumentStore.
func (s *FakeDocStore) Get(userID, path string) (ports.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.docs[s.key(userID, path)]
	if !ok {
		return ports.Document{}, ports.ErrDocumentNotFound
	}
	return d, nil
}

// GetByRepoKey implements ports.DocumentStore.
func (s *FakeDocStore) GetByRepoKey(_, _ string) (ports.Document, error) {
	return ports.Document{}, ports.ErrDocumentNotFound
}

// List implements ports.DocumentStore.
func (s *FakeDocStore) List(userID, prefix, _ string, _ int) ([]ports.DocumentEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ListErr != nil {
		err := s.ListErr
		s.ListErr = nil
		return nil, err
	}
	var out []ports.DocumentEntry
	pfx := userID + ":"
	for k, d := range s.docs {
		if len(k) <= len(pfx) || k[:len(pfx)] != pfx {
			continue
		}
		if prefix != "" && !hasPrefix(d.Path, prefix) {
			continue
		}
		out = append(out, ports.DocumentEntry{
			Path:      d.Path,
			RepoKey:   d.RepoKey,
			Version:   d.Version,
			UpdatedAt: d.UpdatedAt,
		})
	}
	return out, nil
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// Put implements ports.DocumentStore with If-Match semantics.
func (s *FakeDocStore) Put(userID, path, body, repoKey string, ifMatch int64) (ports.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := s.key(userID, path)
	existing, exists := s.docs[k]
	// create-only (ifMatch==0): conflict if already exists
	if ifMatch == 0 && exists {
		return ports.Document{}, ports.ErrDocumentVersionConflict
	}
	// update (ifMatch!=0): must exist and version must match
	if ifMatch != 0 && !exists {
		return ports.Document{}, ports.ErrDocumentVersionConflict
	}
	if ifMatch != 0 && exists && existing.Version != ifMatch {
		return ports.Document{}, ports.ErrDocumentVersionConflict
	}
	s.version++
	v := s.version
	tick := atomic.AddInt64(&s.counter, int64(time.Millisecond))
	d := ports.Document{
		UserID:    userID,
		Path:      path,
		Body:      body,
		RepoKey:   repoKey,
		Version:   v,
		UpdatedAt: time.Unix(0, tick),
	}
	s.docs[k] = d
	return d, nil
}

// Delete implements ports.DocumentStore.
func (s *FakeDocStore) Delete(userID, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.docs, s.key(userID, path))
	return nil
}

// ListRaw implements kompports.NoteSearcher. It delegates to List, ignoring
// query and limit (consistent with the fake's FTS-ignorant List behaviour).
func (s *FakeDocStore) ListRaw(_ context.Context, userID, prefix, _ string, _ int) ([]kompports.RawSearchEntry, error) {
	entries, err := s.List(userID, prefix, "", 0)
	if err != nil {
		return nil, err
	}
	out := make([]kompports.RawSearchEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, kompports.RawSearchEntry{Path: e.Path})
	}
	return out, nil
}

var (
	_ ports.DocumentStore    = (*FakeDocStore)(nil)
	_ kompports.NoteSearcher = (*FakeDocStore)(nil)
)
