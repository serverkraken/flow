package usecase

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// requireBookable verifies that nodeID (when set) names a bookable node
// (engagement, vorhaben or repo). A nil/empty nodeID is allowed (unbooked start).
func requireBookable(ctx context.Context, nodes ports.NodeStore, ownerID string, nodeID *string) error {
	if nodeID == nil || *nodeID == "" {
		return nil
	}
	if nodes == nil {
		return fmt.Errorf("worktime node validation: node store is not configured")
	}
	n, err := nodes.Get(ctx, ownerID, *nodeID)
	if err != nil {
		return err
	}
	if !domain.IsBookable(n.Kind) {
		return fmt.Errorf("%w: worktime books to a bookable node, got %s", domain.ErrInvalidNode, n.Kind)
	}
	return nil
}

// requireOwnedNode verifies that nodeID, when set, belongs to ownerID. Stores
// deliberately return ErrNodeNotFound for both missing and foreign nodes.
func requireOwnedNode(ctx context.Context, nodes ports.NodeStore, ownerID string, nodeID *string) error {
	if nodeID == nil || *nodeID == "" {
		return nil
	}
	if nodes == nil {
		return fmt.Errorf("document node validation: node store is not configured")
	}
	_, err := nodes.Get(ctx, ownerID, *nodeID)
	return err
}
