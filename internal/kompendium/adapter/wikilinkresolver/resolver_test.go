package wikilinkresolver_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/adapter/wikilinkresolver"
	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

func TestResolver_KnownTarget(t *testing.T) {
	t.Parallel()
	store := &fakeStore{
		notes: map[domain.ID]domain.Note{
			"daily/2026-04-25": mustNote(t, "daily/2026-04-25", "Tagesnotiz"),
		},
	}
	r := wikilinkresolver.New(store)

	uri, title, ok := r.Resolve("daily/2026-04-25")
	if !ok {
		t.Fatalf("Resolve(known) ok=false; want true")
	}
	if uri != "kompendium://note/daily/2026-04-25" {
		t.Errorf("URI got %q", uri)
	}
	if title != "Tagesnotiz" {
		t.Errorf("title got %q", title)
	}
}

func TestResolver_MissingTarget(t *testing.T) {
	t.Parallel()
	store := &fakeStore{notes: map[domain.ID]domain.Note{}}
	r := wikilinkresolver.New(store)

	_, _, ok := r.Resolve("daily/missing")
	if ok {
		t.Error("Resolve(missing) ok=true; want false")
	}
}

func TestResolver_MalformedID(t *testing.T) {
	t.Parallel()
	r := wikilinkresolver.New(&fakeStore{notes: map[domain.ID]domain.Note{}})

	// Empty target — domain.ParseID rejects empty strings.
	_, _, ok := r.Resolve("")
	if ok {
		t.Error("Resolve(empty) ok=true; want false")
	}
}

// TestResolver_StoreErrorBecomesBroken: a permission / IO error from
// the store must NOT bubble up as a panic or as ok=true. The
// renderer treats every non-ok target as a broken link, which is the
// right outcome for a transient read error too.
func TestResolver_StoreErrorBecomesBroken(t *testing.T) {
	t.Parallel()
	store := &fakeStore{forcedErr: errors.New("forced read error")}
	r := wikilinkresolver.New(store)

	_, _, ok := r.Resolve("daily/2026-04-25")
	if ok {
		t.Error("Resolve(io-error) ok=true; want false on read error")
	}
}

// TestResolver_TitleFallsBackToEmpty: a note whose frontmatter has no
// Title surfaces with title="". The renderer falls back to display
// the wikilink target verbatim — covered by the markdown renderer's
// own tests; here we only assert the resolver passes through the
// empty title.
func TestResolver_TitleFallsBackToEmpty(t *testing.T) {
	t.Parallel()
	store := &fakeStore{
		notes: map[domain.ID]domain.Note{
			"notes/no-title": mustNote(t, "notes/no-title", ""),
		},
	}
	r := wikilinkresolver.New(store)

	_, title, ok := r.Resolve("notes/no-title")
	if !ok {
		t.Fatalf("Resolve(no-title) ok=false; want true")
	}
	if title != "" {
		t.Errorf("title got %q, want empty", title)
	}
}

// --- fakes ---------------------------------------------------------------

type fakeStore struct {
	notes     map[domain.ID]domain.Note
	forcedErr error
}

func (f *fakeStore) Get(_ context.Context, id domain.ID) (domain.Note, error) {
	if f.forcedErr != nil {
		return domain.Note{}, f.forcedErr
	}
	n, ok := f.notes[id]
	if !ok {
		return domain.Note{}, ports.ErrNoteNotFound
	}
	return n, nil
}

// Remaining ports.NoteStore methods — unused by the resolver, present
// so fakeStore satisfies the interface.
func (f *fakeStore) Put(context.Context, domain.Note) error  { return nil }
func (f *fakeStore) Delete(context.Context, domain.ID) error { return nil }
func (f *fakeStore) Exists(context.Context, domain.ID) (bool, error) {
	return false, nil
}

func (f *fakeStore) List(context.Context, ports.ListFilter) ([]ports.NoteEntry, error) {
	return nil, nil
}
func (f *fakeStore) Path(domain.ID) string { return "" }
func (f *fakeStore) Root() string          { return "" }

func mustNote(t *testing.T, idStr, title string) domain.Note {
	t.Helper()
	id, err := domain.ParseID(idStr)
	if err != nil {
		t.Fatalf("ParseID(%q): %v", idStr, err)
	}
	fm := domain.Frontmatter{
		ID:    id.String(),
		Type:  domain.TypeDaily,
		Title: title,
	}
	n, err := domain.NewNote(id, fm, []byte("body"))
	if err != nil {
		t.Fatalf("NewNote: %v", err)
	}
	return n
}
