package usecase

import (
	"context"
	"errors"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ErrInvalidBindTarget is returned when a binding points at a node whose kind is
// not permitted for that binding kind (remote→repo; path→repo|leaf-vorhaben).
var ErrInvalidBindTarget = errors.New("usecase: invalid binding target kind")

// BindKey carries the kind-specific fields for a project binding operation.
// Path fields are populated through even when Kind == BindingRemote so that
// a later path-tier slice can reuse the same type without breaking callers.
type BindKey struct {
	Kind                                    domain.BindingKind
	RemoteSlug, MachineID, MachineLabel, Path string
}

// BindNode creates or replaces the project binding described by key.
// It validates that the node exists and that the binding kind is appropriate
// for the node's kind (remote→repo; path→repo or leaf vorhaben).
type BindNode struct {
	Bindings ports.ProjectBindingStore
	Nodes    ports.NodeStore
	IDs      ports.IDGen
	Clock    ports.Clock
}

func (uc BindNode) Execute(ctx context.Context, ownerID, nodeID string, k BindKey) (domain.ProjectBinding, error) {
	node, err := uc.Nodes.Get(ctx, ownerID, nodeID)
	if err != nil {
		return domain.ProjectBinding{}, err
	}
	if err := uc.validateTarget(ctx, ownerID, node, k.Kind); err != nil {
		return domain.ProjectBinding{}, err
	}
	now := uc.Clock.Now()
	b := domain.ProjectBinding{
		ID:           uc.IDs.NewID(),
		OwnerID:      ownerID,
		NodeID:       nodeID,
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

// validateTarget enforces: remote→repo; path→repo or a leaf (childless) vorhaben.
func (uc BindNode) validateTarget(ctx context.Context, ownerID string, node domain.Node, kind domain.BindingKind) error {
	switch kind {
	case domain.BindingRemote:
		if node.Kind != domain.KindRepo {
			return ErrInvalidBindTarget
		}
	case domain.BindingPath:
		if node.Kind == domain.KindRepo {
			return nil
		}
		if node.Kind == domain.KindVorhaben {
			children, err := uc.Nodes.Children(ctx, ownerID, &node.ID)
			if err != nil {
				return err
			}
			if len(children) == 0 {
				return nil
			}
		}
		return ErrInvalidBindTarget
	}
	return nil
}
