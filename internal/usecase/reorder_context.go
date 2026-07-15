package usecase

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type ReorderContextDocs struct{ Docs ports.DocumentStore }

// Execute stamps a dense descending priority (first = highest) on the given
// documents in the given order. Owner-scoped; a write failure aborts (the
// client re-submits the full order — idempotent).
func (uc ReorderContextDocs) Execute(ctx context.Context, ownerID string, orderedIDs []string) error {
	seen := make(map[string]struct{}, len(orderedIDs))
	for _, id := range orderedIDs {
		if id == "" {
			return fmt.Errorf("%w: empty context document id", domain.ErrInvalidDocument)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("%w: duplicate context document id %q", domain.ErrInvalidDocument, id)
		}
		seen[id] = struct{}{}
	}
	return uc.Docs.ReorderPriorities(ctx, ownerID, orderedIDs)
}
