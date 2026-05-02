package testutil

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// FakeIndexer is an in-memory ports.Indexer for use-case tests. Search runs a
// naive substring match on title and body — sufficient for verifying that a
// use case wires queries through correctly, not for benchmarking ranking.
//
// Set the *Err fields to force the corresponding method to return that error,
// which lets use-case tests cover index-failure branches.
type FakeIndexer struct {
	mu      sync.Mutex
	entries map[domain.ID]ports.IndexEntry

	UpsertErr    error
	DeleteErr    error
	SearchErr    error
	BacklinksErr error
	LinksFromErr error
	RebuildErr   error
}

// NewFakeIndexer returns an empty FakeIndexer.
func NewFakeIndexer() *FakeIndexer {
	return &FakeIndexer{entries: make(map[domain.ID]ports.IndexEntry)}
}

// Upsert implements ports.Indexer.
func (f *FakeIndexer) Upsert(_ context.Context, note domain.Note, mtime time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.UpsertErr != nil {
		return f.UpsertErr
	}
	f.entries[note.ID] = ports.IndexEntry{Note: note, Mtime: mtime}
	return nil
}

// Delete implements ports.Indexer.
func (f *FakeIndexer) Delete(_ context.Context, id domain.ID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.DeleteErr != nil {
		return f.DeleteErr
	}
	delete(f.entries, id)
	return nil
}

// Search implements ports.Indexer.
func (f *FakeIndexer) Search(_ context.Context, q domain.SearchQuery) ([]domain.SearchResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.SearchErr != nil {
		return nil, f.SearchErr
	}

	out := make([]domain.SearchResult, 0)
	for _, e := range f.entries {
		if !matchesFilter(e.Note, q) {
			continue
		}
		out = append(out, domain.SearchResult{
			ID:    e.Note.ID,
			Title: e.Note.Meta.Title,
			Score: 1.0,
		})
	}
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}

func matchesFilter(n domain.Note, q domain.SearchQuery) bool {
	if q.Type != "" && n.Meta.Type != q.Type {
		return false
	}
	if q.Project != "" && n.Meta.Project != q.Project {
		return false
	}
	if q.Text == "" {
		return true
	}
	if strings.Contains(n.Meta.Title, q.Text) {
		return true
	}
	return strings.Contains(string(n.Body), q.Text)
}

// BacklinksOf implements ports.Indexer.
func (f *FakeIndexer) BacklinksOf(_ context.Context, id domain.ID) ([]domain.LinkRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.BacklinksErr != nil {
		return nil, f.BacklinksErr
	}

	target := id.String()
	out := make([]domain.LinkRef, 0)
	for _, e := range f.entries {
		for _, l := range e.Note.Links() {
			if l.Target == target {
				out = append(out, domain.LinkRef{ID: e.Note.ID, Title: e.Note.Meta.Title})
				break
			}
		}
	}
	return out, nil
}

// LinksFrom implements ports.Indexer.
func (f *FakeIndexer) LinksFrom(_ context.Context, id domain.ID) ([]domain.LinkRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.LinksFromErr != nil {
		return nil, f.LinksFromErr
	}

	e, ok := f.entries[id]
	if !ok {
		return nil, nil
	}
	links := e.Note.Links()
	out := make([]domain.LinkRef, 0, len(links))
	for _, l := range links {
		title := ""
		if target, found := f.entries[domain.ID(l.Target)]; found {
			title = target.Note.Meta.Title
		}
		out = append(out, domain.LinkRef{ID: domain.ID(l.Target), Title: title})
	}
	return out, nil
}

// Rebuild implements ports.Indexer.
func (f *FakeIndexer) Rebuild(_ context.Context, all []ports.IndexEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.RebuildErr != nil {
		return f.RebuildErr
	}
	f.entries = make(map[domain.ID]ports.IndexEntry, len(all))
	for _, e := range all {
		f.entries[e.Note.ID] = e
	}
	return nil
}

var _ ports.Indexer = (*FakeIndexer)(nil)
