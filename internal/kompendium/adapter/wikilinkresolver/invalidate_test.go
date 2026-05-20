package wikilinkresolver_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/adapter/wikilinkresolver"
	"github.com/serverkraken/flow/internal/kompendium/domain"
)

func TestResolver_InvalidateAll_ClearsCache(t *testing.T) {
	t.Parallel()
	store := &fakeStore{
		notes: map[domain.ID]domain.Note{
			"daily/2026-04-25": mustNote(t, "daily/2026-04-25", "Tagesnotiz"),
		},
	}
	r := wikilinkresolver.New(store)
	// Prime the cache.
	if _, _, ok := r.Resolve("daily/2026-04-25"); !ok {
		t.Fatalf("seed Resolve should succeed")
	}
	// InvalidateAll wipes everything.
	r.InvalidateAll()
	// Resolve must still work via the store fallback after invalidation.
	if _, _, ok := r.Resolve("daily/2026-04-25"); !ok {
		t.Errorf("after InvalidateAll Resolve should refetch via store, got ok=false")
	}
}
