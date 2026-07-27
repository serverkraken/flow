package usecase

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type SetContextMode struct {
	Docs     ports.DocumentStore
	Curation ports.DocumentCurationStore
	Clock    ports.Clock
}

// Execute validates the mode and sets it on the document (owner-scoped). An
// unknown mode is rejected before any store write (belt-and-suspenders with the
// DB CHECK constraint).
func (uc SetContextMode) Execute(ctx context.Context, ownerID, id string, mode domain.ContextMode) error {
	if !mode.Valid() {
		return fmt.Errorf("%w: bad context mode %q", domain.ErrInvalidDocument, mode)
	}
	if uc.Curation != nil && uc.Clock != nil {
		_, err := (BulkCurateDocuments{Docs: uc.Curation, Clock: uc.Clock}).Execute(ctx, ownerID, BulkCurateDocumentsInput{
			IDs: []string{id}, ContextMode: &mode,
		})
		return err
	}
	return uc.Docs.SetContextMode(ctx, ownerID, id, mode)
}
