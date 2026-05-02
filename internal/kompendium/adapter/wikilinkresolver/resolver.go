package wikilinkresolver

import (
	"context"
	"errors"

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
type Resolver struct {
	store ports.NoteStore
}

// New returns a Resolver backed by store. The zero-value Resolver is
// not usable — store must be non-nil.
func New(store ports.NoteStore) *Resolver { return &Resolver{store: store} }

// Resolve implements ports.WikilinkResolver. Returns the kompendium
// URI and the target's frontmatter title for valid notes; ok=false
// for malformed IDs and missing / unreadable notes.
func (r *Resolver) Resolve(target string) (uri, title string, ok bool) {
	id, err := domain.ParseID(target)
	if err != nil {
		return "", "", false
	}
	note, err := r.store.Get(context.Background(), id)
	if err != nil {
		if errors.Is(err, ports.ErrNoteNotFound) {
			return "", "", false
		}
		return "", "", false
	}
	return uriScheme + id.String(), note.Meta.Title, true
}

var _ flowports.WikilinkResolver = (*Resolver)(nil)
