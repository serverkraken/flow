package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ResolveProject resolves a project from the binding registry using the
// remote slug, machine ID, and current working directory.
type ResolveProject struct {
	Bindings ports.ProjectBindingStore
	Projects ports.ProjectStore
}

func (uc ResolveProject) Execute(ctx context.Context, ownerID, remoteSlug, machineID, cwd string) (domain.Project, bool, error) {
	bs, err := uc.Bindings.List(ctx, ownerID)
	if err != nil {
		return domain.Project{}, false, err
	}
	b, ok := domain.ResolveBinding(bs, remoteSlug, machineID, cwd)
	if !ok {
		return domain.Project{}, false, nil
	}
	p, err := uc.Projects.Get(ctx, ownerID, b.ProjectID)
	if err != nil {
		return domain.Project{}, false, err
	}
	return p, true, nil
}
