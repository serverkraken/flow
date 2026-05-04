package wikilinkresolver

import (
	"context"
	"errors"
	"sync"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
	flowports "github.com/serverkraken/flow/internal/ports"
)

// uriScheme is the URI prefix the resolver returns for valid targets.
// kompendium's editor launcher recognises this scheme and routes the
// click back into `flow kompendium open <id>` (post-K4) or into a
// note-store lookup directly.
const uriScheme = "kompendium://note/"

// Resolver implements ports.WikilinkResolver by parsing the target as
// a kompendium note ID and fetching the note from the store. The
// store's Get returns ports.ErrNoteNotFound for unknown IDs; any
// other error also surfaces as ok=false so a broken read never poisons
// the rendered output.
//
// Resolve is invoked by the markdown renderer once per [[wikilink]]
// per render; the markdown renderer itself runs on every WindowSizeMsg
// and on every search keystroke. Without a cache a 50-backlink note
// would do ~50 ReadFile + ParseFrontmatter calls per resize. cache
// keeps the per-id result so the steady-state cost is one map lookup;
// Invalidate lets long-lived callers (browse) drop entries after
// editor.Edit so a renamed title resolves fresh on the next render.
type Resolver struct {
	store ports.NoteStore

	mu    sync.RWMutex
	cache map[string]cachedResolve
}

type cachedResolve struct {
	uri   string
	title string
	ok    bool
}

// New returns a Resolver backed by store. The zero-value Resolver is
// not usable — store must be non-nil.
func New(store ports.NoteStore) *Resolver {
	return &Resolver{store: store, cache: map[string]cachedResolve{}}
}

// Resolve implements ports.WikilinkResolver. Returns the kompendium
// URI and the target's frontmatter title for valid notes; ok=false
// for malformed IDs and missing / unreadable notes.
func (r *Resolver) Resolve(target string) (uri, title string, ok bool) {
	r.mu.RLock()
	if cached, hit := r.cache[target]; hit {
		r.mu.RUnlock()
		return cached.uri, cached.title, cached.ok
	}
	r.mu.RUnlock()

	id, err := domain.ParseID(target)
	if err != nil {
		r.remember(target, cachedResolve{})
		return "", "", false
	}
	note, err := r.store.Get(context.Background(), id)
	if err != nil {
		if errors.Is(err, ports.ErrNoteNotFound) {
			r.remember(target, cachedResolve{})
			return "", "", false
		}
		// Don't poison the cache with non-NotFound errors — the next
		// render gets a fresh chance once the underlying issue clears.
		return "", "", false
	}
	out := cachedResolve{uri: uriScheme + id.String(), title: note.Meta.Title, ok: true}
	r.remember(target, out)
	return out.uri, out.title, out.ok
}

func (r *Resolver) remember(target string, v cachedResolve) {
	r.mu.Lock()
	r.cache[target] = v
	r.mu.Unlock()
}

// Invalidate drops the cached entry for target so the next Resolve
// re-reads from the store. Callers that just edited / deleted the
// underlying note (browse screen after editor.Edit) should call this.
// Invalidating an unknown target is a no-op.
func (r *Resolver) Invalidate(target string) {
	r.mu.Lock()
	delete(r.cache, target)
	r.mu.Unlock()
}

// InvalidateAll drops every cached entry. Used after operations that
// can rename or move many notes at once (rebuild, import-legacy,
// snapshot import).
func (r *Resolver) InvalidateAll() {
	r.mu.Lock()
	r.cache = map[string]cachedResolve{}
	r.mu.Unlock()
}

var _ flowports.WikilinkResolver = (*Resolver)(nil)
