package testutil

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// FakeNoteStore is an in-memory ports.NoteStore for use-case tests.
//
// Set the *Err fields to force the corresponding method to return that error
// instead of running its normal logic — this lets use-case tests cover
// store-failure branches without spinning up a real adapter.
type FakeNoteStore struct {
	mu    sync.Mutex
	notes map[domain.ID]storedNote

	GetErr    error
	PutErr    error
	DeleteErr error
	ExistsErr error
	ListErr   error
}

type storedNote struct {
	note  domain.Note
	mtime time.Time
}

// NewFakeNoteStore returns an empty FakeNoteStore.
func NewFakeNoteStore() *FakeNoteStore {
	return &FakeNoteStore{notes: make(map[domain.ID]storedNote)}
}

// Path implements ports.NoteStore. The fake returns a synthetic path under
// "/fake-notebook/" so use-case tests can assert that the editor was handed
// a path derived from the note's ID without depending on a real filesystem.
func (f *FakeNoteStore) Path(id domain.ID) string {
	return "/fake-notebook/" + id.Path()
}

// Root implements ports.NoteStore. Returns the synthetic notebook root used
// by the fake's Path resolver so use-case tests stay self-consistent.
func (f *FakeNoteStore) Root() string {
	return "/fake-notebook"
}

// Seed inserts a pre-built note with explicit mtime, useful for arranging
// list-order or backlink-aware tests.
func (f *FakeNoteStore) Seed(note domain.Note, mtime time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notes[note.ID] = storedNote{note: note, mtime: mtime}
}

// Get implements ports.NoteStore.
func (f *FakeNoteStore) Get(_ context.Context, id domain.ID) (domain.Note, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.GetErr != nil {
		return domain.Note{}, f.GetErr
	}
	s, ok := f.notes[id]
	if !ok {
		return domain.Note{}, ports.ErrNoteNotFound
	}
	return s.note, nil
}

// Put implements ports.NoteStore. Inserts or overwrites; mtime advances on
// every Put, simulating an OS that updates mtime on write.
func (f *FakeNoteStore) Put(_ context.Context, n domain.Note) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.PutErr != nil {
		return f.PutErr
	}
	f.notes[n.ID] = storedNote{note: n, mtime: time.Now()}
	return nil
}

// Delete implements ports.NoteStore.
func (f *FakeNoteStore) Delete(_ context.Context, id domain.ID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.DeleteErr != nil {
		return f.DeleteErr
	}
	if _, ok := f.notes[id]; !ok {
		return ports.ErrNoteNotFound
	}
	delete(f.notes, id)
	return nil
}

// Exists implements ports.NoteStore.
func (f *FakeNoteStore) Exists(_ context.Context, id domain.ID) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ExistsErr != nil {
		return false, f.ExistsErr
	}
	_, ok := f.notes[id]
	return ok, nil
}

// List implements ports.NoteStore. Results are sorted by mtime descending so
// "most recent" tests are deterministic.
func (f *FakeNoteStore) List(_ context.Context, filter ports.ListFilter) ([]ports.NoteEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ListErr != nil {
		return nil, f.ListErr
	}

	out := make([]ports.NoteEntry, 0, len(f.notes))
	for id, s := range f.notes {
		if filter.Type != "" && s.note.Meta.Type != filter.Type {
			continue
		}
		if filter.Project != "" && s.note.Meta.Project != filter.Project {
			continue
		}
		out = append(out, ports.NoteEntry{ID: id, Meta: s.note.Meta, Mtime: s.mtime})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Mtime.After(out[j].Mtime)
	})
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

var _ ports.NoteStore = (*FakeNoteStore)(nil)
