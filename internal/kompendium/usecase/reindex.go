package usecase

import (
	"context"
	"os"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// reindex is the shared best-effort upsert called by Create* / Open after
// the editor closes (the on-disk content may have changed) and by
// ImportLegacy after each migrated note. It is a no-op when index is nil
// — every use case carries Index as an optional dependency so tests can
// stay fakes-only.
//
// Errors are deliberately swallowed: a stale index is recoverable
// (`kompendium index rebuild`), a failed user-facing operation surfaced
// because of an index hiccup would be a bad trade.
//
// The mtime stamp comes from the file's Stat result rather than
// time.Now() — without that, opening a note that wasn't actually
// modified would still bump it to the top of the most-recent-first
// list, contradicting the field name and breaking deterministic tests.
// time.Now() is used only as a fallback when Stat fails (e.g. legacy
// import where the path doesn't exist yet).
func reindex(ctx context.Context, store ports.NoteStore, index ports.Indexer, id domain.ID) {
	if index == nil {
		return
	}
	note, err := store.Get(ctx, id)
	if err != nil {
		return
	}
	stamp := time.Now()
	if path := store.Path(id); path != "" {
		if st, statErr := os.Stat(path); statErr == nil {
			stamp = st.ModTime()
		}
	}
	_ = index.Upsert(ctx, note, stamp)
}
