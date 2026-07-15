package usecase

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// SetNodeRate validates and stores (or clears) an engagement's per-hour rate.
// The database constraint permits rates only on engagements, so the use case
// enforces the same invariant before calling the store.
type SetNodeRate struct {
	Nodes ports.NodeStore
}

// Execute validates rate (when non-nil), then verifies the target is a
// engagement before delegating to the store.
// A nil rate clears any existing rate.
func (uc SetNodeRate) Execute(ctx context.Context, ownerID, nodeID string, rate *domain.Money) error {
	if rate != nil {
		if rate.Amount < 0 || len(rate.Currency) != 3 {
			return domain.ErrInvalidRate
		}
	}
	n, err := uc.Nodes.Get(ctx, ownerID, nodeID)
	if err != nil {
		return err // ErrNodeNotFound bubbles to a 404
	}
	if n.Kind != domain.KindEngagement {
		return fmt.Errorf("%w: only an engagement may carry a rate, got %s", domain.ErrInvalidNode, n.Kind)
	}
	return uc.Nodes.SetRate(ctx, ownerID, nodeID, rate)
}
