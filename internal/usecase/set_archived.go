package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/ports"
)

// SetArchived archives or un-archives a document.
type SetArchived struct {
	Docs     ports.DocumentStore
	Curation ports.DocumentCurationStore
	Clock    ports.Clock
}

func (uc SetArchived) Execute(ctx context.Context, ownerID, id string, archived bool) error {
	if uc.Curation != nil && uc.Clock != nil {
		_, err := (BulkCurateDocuments{Docs: uc.Curation, Clock: uc.Clock}).Execute(ctx, ownerID, BulkCurateDocumentsInput{
			IDs: []string{id}, Archived: &archived,
		})
		return err
	}
	return uc.Docs.SetArchived(ctx, ownerID, id, archived)
}
