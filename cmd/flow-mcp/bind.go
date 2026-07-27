package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// decideBindKind picks the binding kind. An explicit override wins ("remote"
// requires a git origin); otherwise a git origin → remote, else path.
func decideBindKind(kindOverride string, originOK bool) (string, error) {
	switch strings.TrimSpace(kindOverride) {
	case "remote":
		if !originOK {
			return "", errGuard{errors.New(`kind "remote" needs a git origin in this directory; use "path" or omit kind`)}
		}
		return "remote", nil
	case "path":
		return "path", nil
	case "":
		if originOK {
			return "remote", nil
		}
		return "path", nil
	default:
		return "", errGuard{fmt.Errorf(`invalid kind %q; use "remote" or "path", or omit to auto-detect`, kindOverride)}
	}
}

// bindNodeCore resolves the node reference and commits the already-resolved
// target as its binding. The create branch is gone: creating a node is
// flow_create_node's job (Spec §3), which keeps this function to one job.
func (h *handlers) bindNodeCore(ctx context.Context, c *apiclient.Client, nodeRef string, tgt bindTarget) (domain.Node, error) {
	ref := strings.TrimSpace(nodeRef)
	if ref == "" {
		return domain.Node{}, errGuard{errors.New(`"project" is required: pass an existing node's id, slug, or name (flow_list_projects shows the tree); to create a node use flow_create_node`)}
	}
	node, err := h.lookupNode(ctx, ref)
	if err != nil {
		return domain.Node{}, err
	}
	if err := bindTargetTo(ctx, c, node.ID, tgt); err != nil {
		return domain.Node{}, err
	}
	return node, nil
}

// bindTargetTo commits a resolved target as a binding on nodeID.
func bindTargetTo(ctx context.Context, c *apiclient.Client, nodeID string, tgt bindTarget) error {
	if tgt.Kind == "remote" {
		_, err := c.BindRemote(ctx, nodeID, tgt.RemoteSlug)
		return err
	}
	_, err := c.BindPath(ctx, nodeID, tgt.MachineID, tgt.MachineLabel, tgt.Path)
	return err
}

// unbindTarget deletes the binding a target addresses. Neither unbind call takes
// a node id (internal/adapter/apiclient/projectbindings.go:82,96): a binding is
// identified by its target alone, which is why flow_node_binding rejects a
// `node` argument for unbind (Spec §3).
func unbindTarget(ctx context.Context, c *apiclient.Client, tgt bindTarget) error {
	if tgt.Kind == "remote" {
		return c.UnbindRemote(ctx, tgt.RemoteSlug)
	}
	return c.UnbindPath(ctx, tgt.MachineID, tgt.Path)
}
