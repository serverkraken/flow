package nodetree

import (
	"context"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// TreeAPI is the read+mutate surface the tree root needs: list for the tree,
// delete + move for the in-route dialogs. *apiclient.Client satisfies it.
type TreeAPI interface {
	ListNodes(ctx context.Context) ([]domain.Node, error)
	DeleteNode(ctx context.Context, id string) error
	MoveNode(ctx context.Context, id string, parentID *string) (domain.Node, error)
}

var _ TreeAPI = (*apiclient.Client)(nil)
