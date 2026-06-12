// Package apistore implements kompports.NoteStore over the server-side
// ports.DocumentStore HTTP API. Notes are stored as "id.md" paths in the
// document namespace; repos/ paths are excluded from the corpus.
//
// A full-corpus cache is maintained so list/get operations do not make a
// round-trip per note. The cache is invalidated on version conflict (Put)
// and can be invalidated externally via Invalidate (e.g. after an SSE
// event signals a remote change).
package apistore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	kompports "github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/ports"
)

// Store implements kompports.NoteStore by delegating to a ports.DocumentStore.
// An in-memory corpus cache avoids a Get-per-note on every List call.
type Store struct {
	docs   ports.DocumentStore
	userID string

	mu      sync.Mutex
	corpus  map[string]corpusEntry // keyed by path ("id.md")
	loaded  bool
	stale   bool
	loading bool // true while a corpus reload is in progress
}

// corpusEntry pairs a cached body with the server version it was fetched at.
type corpusEntry struct {
	doc ports.Document
}

// New returns a Store backed by docs for the given userID.
func New(docs ports.DocumentStore, userID string) *Store {
	return &Store{
		docs:   docs,
		userID: userID,
		corpus: make(map[string]corpusEntry),
	}
}

// Invalidate marks the corpus as stale so the next operation reloads it from
// the server. Safe to call concurrently (e.g. from an SSE goroutine).
func (s *Store) Invalidate() {
	s.mu.Lock()
	s.stale = true
	s.mu.Unlock()
}

// ensure loads or reloads the corpus when necessary. It holds the mutex only
// for map reads/writes — network calls happen outside the lock to avoid
// blocking other goroutines. Concurrent callers that arrive while a reload is
// already in progress return immediately; the in-flight load will update the
// corpus for everyone.
func (s *Store) ensure(ctx context.Context) error {
	// Fast path: already loaded and not stale, or a reload is already in flight.
	s.mu.Lock()
	if s.loaded && !s.stale {
		s.mu.Unlock()
		return nil
	}
	if s.loading {
		s.mu.Unlock()
		return nil
	}
	// Snapshot old corpus for version comparison (used below without lock).
	old := make(map[string]corpusEntry, len(s.corpus))
	for k, v := range s.corpus {
		old[k] = v
	}
	s.loading = true
	s.mu.Unlock()

	// Fetch the entry list from the server (no lock).
	entries, err := s.docs.List(s.userID, "", "", 0)
	if err != nil {
		// On transient failure, keep using the old corpus if we have one.
		s.mu.Lock()
		s.loading = false
		if s.loaded {
			s.stale = false // suppress retry until next Invalidate
			s.mu.Unlock()
			slog.Warn("apistore: list failed, serving stale corpus", "error", err)
			return nil
		}
		s.mu.Unlock()
		return fmt.Errorf("apistore: list documents: %w", err)
	}

	// Determine which paths need a body fetch.
	type fetchWork struct {
		path      string
		version   int64
		updatedAt time.Time
	}
	var toFetch []fetchWork
	reuse := make(map[string]corpusEntry)

	for _, e := range entries {
		if strings.HasPrefix(e.Path, "repos/") {
			continue
		}
		if !strings.HasSuffix(e.Path, ".md") {
			continue
		}
		if prev, ok := old[e.Path]; ok && prev.doc.Version == e.Version {
			reuse[e.Path] = prev
		} else {
			toFetch = append(toFetch, fetchWork{path: e.Path, version: e.Version, updatedAt: e.UpdatedAt})
		}
	}

	// Fetch new/changed bodies (no lock).
	fetched := make(map[string]corpusEntry, len(toFetch))
	for _, w := range toFetch {
		doc, err := s.docs.Get(s.userID, w.path)
		if err != nil {
			if errors.Is(err, ports.ErrDocumentNotFound) {
				// Deleted between List and Get — skip silently.
				continue
			}
			s.mu.Lock()
			s.loading = false
			s.mu.Unlock()
			return fmt.Errorf("apistore: get %q: %w", w.path, err)
		}
		fetched[w.path] = corpusEntry{doc: doc}
	}

	// Merge and commit under the lock.
	s.mu.Lock()
	newCorpus := make(map[string]corpusEntry, len(reuse)+len(fetched))
	for k, v := range reuse {
		newCorpus[k] = v
	}
	for k, v := range fetched {
		newCorpus[k] = v
	}
	s.corpus = newCorpus
	s.loaded = true
	s.stale = false
	s.loading = false
	s.mu.Unlock()
	return nil
}

// docPath returns the document path for a note ID ("id.md").
func docPath(id domain.ID) string {
	return string(id) + ".md"
}

// idFromPath strips the ".md" suffix to yield a domain.ID.
func idFromPath(p string) (domain.ID, error) {
	s := strings.TrimSuffix(p, ".md")
	return domain.ParseID(s)
}

// noteFromDoc parses a document body into a domain.Note.
func noteFromDoc(id domain.ID, doc ports.Document) (domain.Note, error) {
	fm, body, err := domain.ParseFrontmatter([]byte(doc.Body))
	if err != nil {
		return domain.Note{}, fmt.Errorf("parse frontmatter of %q: %w", doc.Path, err)
	}
	note, err := domain.NewNote(id, fm, body)
	if err != nil {
		return domain.Note{}, fmt.Errorf("validate %q: %w", doc.Path, err)
	}
	return note, nil
}

// renderNote serializes a domain.Note back to markdown with frontmatter.
func renderNote(note domain.Note) string {
	return string(note.Meta.Serialize(note.Body))
}

// entryFromDoc builds a NoteEntry from a cached corpus entry.
func entryFromDoc(id domain.ID, e corpusEntry) (kompports.NoteEntry, bool) {
	fm, _, err := domain.ParseFrontmatter([]byte(e.doc.Body))
	if err != nil {
		return kompports.NoteEntry{}, false
	}
	if err := fm.Validate(); err != nil {
		return kompports.NoteEntry{}, false
	}
	return kompports.NoteEntry{
		ID:    id,
		Meta:  fm,
		Mtime: e.doc.UpdatedAt,
	}, true
}

// matchesFilter reports whether entry matches the filter criteria.
func matchesFilter(entry kompports.NoteEntry, filter kompports.ListFilter) bool {
	if filter.Type != "" && entry.Meta.Type != filter.Type {
		return false
	}
	if filter.Project != "" && entry.Meta.Project != filter.Project {
		return false
	}
	return true
}

// sortEntries sorts entries by mtime descending (most recent first).
func sortEntries(entries []kompports.NoteEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Mtime.After(entries[j].Mtime)
	})
}

// Get implements kompports.NoteStore.
func (s *Store) Get(ctx context.Context, id domain.ID) (domain.Note, error) {
	if err := s.ensure(ctx); err != nil {
		return domain.Note{}, err
	}
	p := docPath(id)
	s.mu.Lock()
	e, ok := s.corpus[p]
	s.mu.Unlock()
	if !ok {
		return domain.Note{}, kompports.ErrNoteNotFound
	}
	return noteFromDoc(id, e.doc)
}

// Exists implements kompports.NoteStore.
func (s *Store) Exists(ctx context.Context, id domain.ID) (bool, error) {
	if err := s.ensure(ctx); err != nil {
		return false, err
	}
	p := docPath(id)
	s.mu.Lock()
	_, ok := s.corpus[p]
	s.mu.Unlock()
	return ok, nil
}

// Put implements kompports.NoteStore. It uses If-Match semantics: version 0
// signals a create, otherwise the current cached version is passed.
// ifMatch 0 is used for cache-miss entries (treat as create); callers may
// receive ErrDocumentVersionConflict if a concurrent create already landed on
// the server. On ErrDocumentVersionConflict the corpus is invalidated so the
// next read sees fresh state.
func (s *Store) Put(ctx context.Context, note domain.Note) error {
	if err := s.ensure(ctx); err != nil {
		return err
	}

	p := docPath(note.ID)
	s.mu.Lock()
	e, exists := s.corpus[p]
	s.mu.Unlock()

	var ifMatch int64
	if exists {
		ifMatch = e.doc.Version
	}

	body := renderNote(note)
	doc, err := s.docs.Put(s.userID, p, body, "", ifMatch)
	if err != nil {
		if errors.Is(err, ports.ErrDocumentVersionConflict) {
			s.Invalidate()
		}
		return fmt.Errorf("apistore: put %q: %w", p, err)
	}

	// Update cache with the new document.
	s.mu.Lock()
	s.corpus[p] = corpusEntry{doc: doc}
	s.mu.Unlock()
	return nil
}

// Delete implements kompports.NoteStore.
func (s *Store) Delete(ctx context.Context, id domain.ID) error {
	if err := s.ensure(ctx); err != nil {
		return err
	}
	p := docPath(id)

	s.mu.Lock()
	_, ok := s.corpus[p]
	s.mu.Unlock()
	if !ok {
		return kompports.ErrNoteNotFound
	}

	if err := s.docs.Delete(s.userID, p); err != nil {
		return fmt.Errorf("apistore: delete %q: %w", p, err)
	}

	s.mu.Lock()
	delete(s.corpus, p)
	s.mu.Unlock()
	return nil
}

// List implements kompports.NoteStore.
func (s *Store) List(ctx context.Context, filter kompports.ListFilter) ([]kompports.NoteEntry, error) {
	if err := s.ensure(ctx); err != nil {
		return nil, err
	}

	s.mu.Lock()
	snapshot := make(map[string]corpusEntry, len(s.corpus))
	for k, v := range s.corpus {
		snapshot[k] = v
	}
	s.mu.Unlock()

	var out []kompports.NoteEntry
	for p, e := range snapshot {
		id, err := idFromPath(p)
		if err != nil {
			continue
		}
		entry, ok := entryFromDoc(id, e)
		if !ok {
			continue
		}
		if !matchesFilter(entry, filter) {
			continue
		}
		out = append(out, entry)
	}

	sortEntries(out)
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

var _ kompports.NoteStore = (*Store)(nil)
