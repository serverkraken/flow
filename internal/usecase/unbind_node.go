package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// UnbindNode removes a project binding by kind-key.
type UnbindNode struct {
	Bindings ports.ProjectBindingStore
}

func (uc UnbindNode) Execute(ctx context.Context, ownerID string, k BindKey) error {
	switch k.Kind {
	case domain.BindingPath:
		return uc.Bindings.DeletePath(ctx, ownerID, k.MachineID, k.Path)
	default:
		return uc.Bindings.DeleteRemote(ctx, ownerID, k.RemoteSlug)
	}
}
