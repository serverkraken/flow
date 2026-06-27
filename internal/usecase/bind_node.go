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

// BindNode creates or replaces the project binding described by key.
// It validates that the project exists first.
type BindNode struct {
	Bindings ports.ProjectBindingStore
	Nodes	ports.NodeStore
	IDs      ports.IDGen
	Clock    ports.Clock
}

func (uc BindNode) Execute(ctx context.Context, ownerID, nodeID string, k BindKey) (domain.ProjectBinding, error) {
	if _, err := uc.Nodes.Get(ctx, ownerID, nodeID); err != nil {
		return domain.ProjectBinding{}, err
	}
	now := uc.Clock.Now()
	b := domain.ProjectBinding{
		ID:           uc.IDs.NewID(),
		OwnerID:      ownerID,
		NodeID:    nodeID,
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
