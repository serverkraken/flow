package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Backlinks returns the documents that link to a given document, after
// re-resolving each candidate through domain.ResolveWikilink so foreign-scope
// collisions never produce false references.
type Backlinks struct {
	Docs ports.DocumentStore
}

func (uc Backlinks) Execute(ctx context.Context, ownerID, docID string) ([]domain.BacklinkRef, error) {
	target, err := uc.Docs.Get(ctx, ownerID, docID)
	if err != nil {
		return nil, err
	}
	candidates, err := uc.Docs.Backlinks(ctx, ownerID, target.Path)
	if err != nil {
		return nil, err
	}
	all, err := uc.Docs.List(ctx, ownerID, nil)
	if err != nil {
		return nil, err
	}
	var out []domain.BacklinkRef
	for _, src := range candidates {
		if src.ID == target.ID {
			continue
		}
		if resolved, ok := domain.ResolveWikilink(src, target.Path, all); ok && resolved.ID == target.ID {
			out = append(out, domain.BacklinkRef{
				ID: src.ID, Path: src.Path, Title: src.Title, Type: src.Type,
			})
		}
	}
	return out, nil
}
