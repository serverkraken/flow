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
	n, err := nodes.Get(ctx, ownerID, *nodeID)
	if err != nil {
		return err
	}
	if !domain.IsBookable(n.Kind) {
		return fmt.Errorf("%w: worktime books to a bookable node, got %s", domain.ErrInvalidNode, n.Kind)
	}
	return nil
}
