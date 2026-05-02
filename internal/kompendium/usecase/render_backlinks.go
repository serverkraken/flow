package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// RenderBacklinks fetches a note plus every note that links to it, so the
// read view can show "referenced by" without persisting backlinks into the
// note files (cross-link model C — see CLAUDE.md section 9).
type RenderBacklinks struct {
	Store ports.NoteStore
	Index ports.Indexer
}

// NewRenderBacklinks returns a RenderBacklinks using the given store and
// indexer.
func NewRenderBacklinks(store ports.NoteStore, index ports.Indexer) *RenderBacklinks {
	return &RenderBacklinks{Store: store, Index: index}
}

// RenderBacklinksInput configures one Execute call.
type RenderBacklinksInput struct {
	NoteID domain.ID
}

// BacklinkRef is a small projection used by RenderBacklinks. It carries the
// minimum needed to render a "referenced by" line without loading note
// bodies.
type BacklinkRef struct {
	ID    domain.ID
	Title string
}

// RenderBacklinksOutput bundles the requested note with its resolved
// backlink references.
type RenderBacklinksOutput struct {
	Note      domain.Note
	Backlinks []BacklinkRef
}

// Execute fetches the note and reads its backlinks straight from the
// indexer — the indexer already joins title in, so the previous
// per-link store.Get N+1 is gone. Backlinks pointing at deleted notes
// (empty title) stay in the result so the read view can surface them
// as "broken backlink"; the per-link store fetch was the only reason
// we filtered them out before.
func (u *RenderBacklinks) Execute(ctx context.Context, in RenderBacklinksInput) (RenderBacklinksOutput, error) {
	note, err := u.Store.Get(ctx, in.NoteID)
	if err != nil {
		return RenderBacklinksOutput{}, err
	}

	links, err := u.Index.BacklinksOf(ctx, in.NoteID)
	if err != nil {
		return RenderBacklinksOutput{}, err
	}

	refs := make([]BacklinkRef, 0, len(links))
	for _, l := range links {
		refs = append(refs, BacklinkRef{ID: l.ID, Title: l.Title})
	}

	return RenderBacklinksOutput{Note: note, Backlinks: refs}, nil
}
