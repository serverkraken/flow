package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ListProjectBindings returns all project bindings for the owner.
type ListProjectBindings struct {
	Bindings ports.ProjectBindingStore
}

func (uc ListProjectBindings) Execute(ctx context.Context, ownerID string) ([]domain.ProjectBinding, error) {
	return uc.Bindings.List(ctx, ownerID)
}
