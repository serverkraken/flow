package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// GetNodeLogo returns a node's stored logo blob (the WebUI serving path).
type GetNodeLogo struct {
	Logos ports.NodeLogoStore
}

func (uc GetNodeLogo) Execute(ctx context.Context, ownerID, nodeID string) (domain.NodeLogo, error) {
	return uc.Logos.Get(ctx, ownerID, nodeID)
}
