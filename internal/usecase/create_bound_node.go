package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// CreateBoundNodeInput is the atomic MCP create_name command.
type CreateBoundNodeInput struct {
	Node    CreateNodeInput
	Binding BindKey
}

type CreateBoundNodeResult struct {
	Node    domain.Node           `json:"node"`
	Binding domain.ProjectBinding `json:"binding"`
}

// CreateBoundNode creates one repo and its explicitly requested binding in a
// single store transaction.
type CreateBoundNode struct {
	Nodes     ports.NodeStore
	Aggregate ports.NodeBindingAggregateStore
	IDs       ports.IDGen
	Clock     ports.Clock
}

func (uc CreateBoundNode) Execute(ctx context.Context, ownerID string, in CreateBoundNodeInput) (CreateBoundNodeResult, error) {
	key, err := validateCreateBinding(in.Binding)
	if err != nil {
		return CreateBoundNodeResult{}, err
	}
	if uc.Aggregate == nil {
		return CreateBoundNodeResult{}, errors.New("create-bound node aggregate store is not configured")
	}
	builder := CreateNode{Nodes: uc.Nodes, IDs: uc.IDs, Clock: uc.Clock}
	n, changes, err := builder.prepare(ctx, ownerID, in.Node)
	if err != nil {
		return CreateBoundNodeResult{}, err
	}
	if n.Kind != domain.KindRepo {
		return CreateBoundNodeResult{}, fmt.Errorf("%w: bound node must be a repo", domain.ErrInvalidNode)
	}
	if key.Kind == domain.BindingRemote && n.OriginSlug == "" {
		n.OriginSlug = key.RemoteSlug
	}
	now := uc.Clock.Now()
	binding := domain.ProjectBinding{
		ID: uc.IDs.NewID(), OwnerID: ownerID, NodeID: n.ID, Kind: key.Kind,
		RemoteSlug: key.RemoteSlug, MachineID: key.MachineID,
		MachineLabel: key.MachineLabel, Path: key.Path,
		CreatedAt: now, UpdatedAt: now,
	}
	created, bound, err := uc.Aggregate.CreateBoundAggregate(ctx, n, changes, binding)
	if err != nil {
		return CreateBoundNodeResult{}, err
	}
	return CreateBoundNodeResult{Node: created, Binding: bound}, nil
}

func validateCreateBinding(k BindKey) (BindKey, error) {
	switch k.Kind {
	case domain.BindingRemote:
		raw := strings.TrimSpace(k.RemoteSlug)
		slug, ok := domain.NormalizeRemoteSlug(raw)
		if !ok {
			slug, ok = domain.NormalizeRemoteSlug("https://" + raw)
		}
		if !ok {
			return BindKey{}, fmt.Errorf("%w: invalid remote binding", ErrInvalidBindTarget)
		}
		k.RemoteSlug = slug
		k.MachineID, k.MachineLabel, k.Path = "", "", ""
	case domain.BindingPath:
		k.MachineID = strings.TrimSpace(k.MachineID)
		k.Path = strings.TrimSpace(k.Path)
		if k.MachineID == "" || k.Path == "" {
			return BindKey{}, fmt.Errorf("%w: path binding needs machine and path", ErrInvalidBindTarget)
		}
		k.RemoteSlug = ""
	default:
		return BindKey{}, fmt.Errorf("%w: unknown binding kind", ErrInvalidBindTarget)
	}
	return k, nil
}
