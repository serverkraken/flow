package usecase

import (
	"context"
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
// time.Now() is used as the mtime stamp. Previously the store's on-disk
// path was stat'd for the exact mtime, but NoteStore no longer exposes
// Path() (the API store has no local filesystem path). For the legacy
// fsstore the difference is negligible — the stamp is used only for
// ordering in the search result list, not for correctness.
func reindex(ctx context.Context, store ports.NoteStore, index ports.Indexer, id domain.ID) {
	if index == nil {
		return
	}
	note, err := store.Get(ctx, id)
	if err != nil {
		return
	}
	_ = index.Upsert(ctx, note, time.Now())
}
