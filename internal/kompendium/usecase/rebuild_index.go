package usecase

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// RebuildIndex walks every note in the store and replaces the FTS5 index
// from scratch. The escape hatch when auto-reindex hasn't been keeping up
// — runs after `import-legacy` to surface migrated notes in
// `kompendium search`, or when files have been edited outside kompendium.
type RebuildIndex struct {
	Store ports.NoteStore
	Index ports.Indexer
}

// NewRebuildIndex wires the use case with its required ports.
func NewRebuildIndex(store ports.NoteStore, index ports.Indexer) *RebuildIndex {
	return &RebuildIndex{Store: store, Index: index}
}

// RebuildIndexOutput reports how many notes were re-indexed and which note
// reads failed (the rebuild continues past per-note errors so the index
// reflects as much as could be loaded).
type RebuildIndexOutput struct {
	Indexed int
	Errors  []RebuildIssue
}

// RebuildIssue names a note ID that could not be loaded during the rebuild.
type RebuildIssue struct {
	NoteID string
	Detail string
}

// Execute lists every note, loads its body, and replaces the index.
func (u *RebuildIndex) Execute(ctx context.Context) (RebuildIndexOutput, error) {
	entries, err := u.Store.List(ctx, ports.ListFilter{})
	if err != nil {
		return RebuildIndexOutput{}, fmt.Errorf("list: %w", err)
	}

	out := RebuildIndexOutput{}
	items := make([]ports.IndexEntry, 0, len(entries))
	for _, e := range entries {
		note, err := u.Store.Get(ctx, e.ID)
		if err != nil {
			out.Errors = append(out.Errors, RebuildIssue{NoteID: e.ID.String(), Detail: err.Error()})
			continue
		}
		items = append(items, ports.IndexEntry{Note: note, Mtime: e.Mtime})
	}

	if err := u.Index.Rebuild(ctx, items); err != nil {
		return RebuildIndexOutput{}, fmt.Errorf("rebuild: %w", err)
	}
	out.Indexed = len(items)
	return out, nil
}
