package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/ports"
)

type ReorderContextDocs struct{ Docs ports.DocumentStore }

// Execute stamps a dense descending priority (first = highest) on the given
// documents in the given order. Owner-scoped; a write failure aborts (the
// client re-submits the full order — idempotent).
func (uc ReorderContextDocs) Execute(ctx context.Context, ownerID string, orderedIDs []string) error {
	n := len(orderedIDs)
	for i, id := range orderedIDs {
		if err := uc.Docs.SetPriority(ctx, ownerID, id, n-i); err != nil {
			return err
		}
	}
	return nil
}
