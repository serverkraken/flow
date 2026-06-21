package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// BindKey carries the kind-specific fields for a project binding operation.
// Path fields are populated through even when Kind == BindingRemote so that
// a later path-tier slice can reuse the same type without breaking callers.
type BindKey struct {
	Kind                           domain.BindingKind
	RemoteSlug, MachineID, MachineLabel, Path string
}

// BindProject creates or replaces the project binding described by key.
// It validates that the project exists first.
type BindProject struct {
	Bindings ports.ProjectBindingStore
	Projects ports.ProjectStore
	IDs      ports.IDGen
	Clock    ports.Clock
}

func (uc BindProject) Execute(ctx context.Context, ownerID, projectID string, k BindKey) (domain.ProjectBinding, error) {
	if _, err := uc.Projects.Get(ctx, ownerID, projectID); err != nil {
		return domain.ProjectBinding{}, err
	}
	now := uc.Clock.Now()
	b := domain.ProjectBinding{
		ID:           uc.IDs.NewID(),
		OwnerID:      ownerID,
		ProjectID:    projectID,
		Kind:         k.Kind,
		RemoteSlug:   k.RemoteSlug,
		MachineID:    k.MachineID,
		MachineLabel: k.MachineLabel,
		Path:         k.Path,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	return uc.Bindings.Upsert(ctx, b)
}
