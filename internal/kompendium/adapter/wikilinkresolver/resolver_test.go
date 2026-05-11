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

// TestResolver_TransientStoreErrorNotCached guards review finding T4:
// the resolver intentionally swallows non-NotFound errors and does NOT
// poison its cache with them. This invariant is load-bearing — the
// renderer runs on every WindowSizeMsg and search keystroke; once a
// transient SQL hiccup clears, the next render must get a fresh chance
// instead of permanently displaying a broken link.
func TestResolver_TransientStoreErrorNotCached(t *testing.T) {
	t.Parallel()
	knownNote := mustNote(t, "daily/2026-04-25", "Tagesnotiz")
	store := &fakeStore{
		notes:     map[domain.ID]domain.Note{"daily/2026-04-25": knownNote},
		forcedErr: errors.New("transient read error"),
	}
	r := wikilinkresolver.New(store)

	// First call hits the forced error → ok=false, must NOT cache.
	if _, _, ok := r.Resolve("daily/2026-04-25"); ok {
		t.Fatal("first Resolve must surface the forced error as ok=false")
	}

	// Clear the error: simulating the underlying issue resolving.
	store.forcedErr = nil

	// Second call must now succeed — proves the failure was not cached.
	uri, title, ok := r.Resolve("daily/2026-04-25")
	if !ok {
		t.Fatalf("second Resolve ok=false; the transient error poisoned the cache")
	}
	if uri == "" || title != "Tagesnotiz" {
		t.Errorf("post-recovery Resolve returned uri=%q title=%q", uri, title)
	}
}

// TestResolver_NotFoundIsCached locks down the complementary contract:
// ports.ErrNoteNotFound *is* a permanent answer ("the note doesn't
// exist") and must be cached so a 50-backlink note with one missing
// target doesn't re-Get on every render.
func TestResolver_NotFoundIsCached(t *testing.T) {
	t.Parallel()
	store := &fakeStore{notes: map[domain.ID]domain.Note{}}
	r := wikilinkresolver.New(store)

	// Two consecutive Resolves of the same missing target.
	_, _, ok1 := r.Resolve("daily/2026-04-25")
	_, _, ok2 := r.Resolve("daily/2026-04-25")
	if ok1 || ok2 {
		t.Fatal("missing target must be ok=false both times")
	}
	// Now the note appears — but the cache should still report missing
	// (Invalidate would be required to refresh).
	store.notes["daily/2026-04-25"] = mustNote(t, "daily/2026-04-25", "title")
	if _, _, ok := r.Resolve("daily/2026-04-25"); ok {
		t.Error("NotFound result was not cached — got ok=true after note appeared without Invalidate")
	}
	// After Invalidate, the new note should resolve.
	r.Invalidate("daily/2026-04-25")
	if _, _, ok := r.Resolve("daily/2026-04-25"); !ok {
		t.Error("post-Invalidate Resolve should pick up the new note")
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
