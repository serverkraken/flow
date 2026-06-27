package usecase

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// requireEngagement verifies that nodeID (when non-nil and non-empty) names an
// existing node of kind engagement owned by ownerID. Worktime is booked at the
// engagement level (D3); a nil/empty nodeID is allowed (unbooked). A missing or
// foreign node surfaces the store's ErrNodeNotFound; a non-engagement kind
// yields ErrInvalidNode.
func requireEngagement(ctx context.Context, nodes ports.NodeStore, ownerID string, nodeID *string) error {
	if nodeID == nil || *nodeID == "" {
		return nil
	}
	n, err := nodes.Get(ctx, ownerID, *nodeID)
	if err != nil {
		return err
	}
	if n.Kind != domain.KindEngagement {
		return fmt.Errorf("%w: worktime books to an engagement, got %s", domain.ErrInvalidNode, n.Kind)
	}
	return nil
}
