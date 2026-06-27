package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ListNodeBindings returns all project bindings for the owner.
type ListNodeBindings struct {
	Bindings ports.ProjectBindingStore
}

func (uc ListNodeBindings) Execute(ctx context.Context, ownerID string) ([]domain.ProjectBinding, error) {
	return uc.Bindings.List(ctx, ownerID)
}

// ExecuteByProject returns bindings scoped to a single project.
func (uc ListNodeBindings) ExecuteByProject(ctx context.Context, ownerID, nodeID string) ([]domain.ProjectBinding, error) {
	return uc.Bindings.ListByProject(ctx, ownerID, nodeID)
}
