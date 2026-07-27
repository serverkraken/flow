package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// moveNodeIn reparents a node. Exactly one of parent / to_root is required:
// JSON cannot distinguish an omitted string from an empty one, so the CLI's
// `--parent ""` + fl.Changed pattern (cmd/flow/node_subcommands.go:341) is not
// expressible over MCP — hence the separate boolean (Spec §3).
type moveNodeIn struct {
	Node   string `json:"node" jsonschema:"the node to reparent (id, slug, or name)"`
	Parent string `json:"parent,omitempty" jsonschema:"the new parent (id, slug, or name) — an engagement or vorhaben. Pass exactly one of parent or to_root."`
	ToRoot bool   `json:"to_root,omitempty" jsonschema:"true makes the node a root; only an engagement may be a root. Pass exactly one of parent or to_root."`
}

// validateMoveNode enforces exactly one destination.
func validateMoveNode(in moveNodeIn) error {
	if strings.TrimSpace(in.Node) == "" {
		return errGuard{errors.New("node is required: the id, slug, or name of the node to reparent")}
	}
	hasParent := strings.TrimSpace(in.Parent) != ""
	if hasParent == in.ToRoot {
		return errGuard{errors.New(`pass exactly one destination: "parent" (an engagement or vorhaben id/slug/name) or to_root=true (make it a root engagement)`)}
	}
	return nil
}

// moveNode reparents a node. The kind rules are pre-checked client-side for a
// precise message; cycle-freeness is the server's job (it answers 409, which
// h.resultErr surfaces verbatim).
func (h *handlers) moveNode(ctx context.Context, req *mcp.CallToolRequest, in moveNodeIn) (*mcp.CallToolResult, any, error) {
	if err := validateMoveNode(in); err != nil {
		return h.resultErr(err), nil, nil
	}
	var out string
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		node, err := h.lookupNode(ctx, strings.TrimSpace(in.Node))
		if err != nil {
			return err
		}
		var parentID *string
		dest := "root"
		if ref := strings.TrimSpace(in.Parent); ref != "" {
			parent, perr := h.lookupNode(ctx, ref)
			if perr != nil {
				return prefixGuard("parent", perr)
			}
			if !domain.ValidParentKind(node.Kind, parent.Kind) {
				return errGuard{fmt.Errorf("a %s cannot hang under %s %q (%s): a parent must be an engagement or a vorhaben",
					node.Kind, parent.Kind, parent.Name, parent.Slug)}
			}
			parentID = &parent.ID
			dest = fmt.Sprintf("%s %q (%s)", parent.Kind, parent.Name, parent.Slug)
		} else if node.Kind != domain.KindEngagement {
			// ValidParentKind's default case makes an engagement the only legal
			// root (internal/domain/node.go:107).
			return errGuard{fmt.Errorf("only an engagement may be a root; %s %q (%s) needs a parent",
				node.Kind, node.Name, node.Slug)}
		}
		moved, err := c.MoveNode(ctx, node.ID, parentID)
		if err != nil {
			return err
		}
		if _, lerr := h.nodeList(ctx, true); lerr != nil {
			mcpLog().Warn("could not refresh the node cache after move", "err", lerr)
		}
		out = fmt.Sprintf("Moved %s %q (%s) to %s.", moved.Kind, moved.Name, moved.Slug, dest)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}

// deleteNodeIn reports the consequences of deleting a node, and deletes only
// with confirm=true. `node` is deliberately required: silently deleting whatever
// this directory happens to resolve to is too dangerous to default into.
type deleteNodeIn struct {
	Node    string `json:"node" jsonschema:"the node to delete (id, slug, or name)"`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"true actually deletes; omit or false only reports what deletion would destroy"`
}

// deleteImpact is everything the dry run learned about a node's deletion.
type deleteImpact struct {
	Node         domain.Node
	Children     []domain.Node
	ProjectDocs  []domain.Document
	OwnArtifacts int // ListArtifacts filtered to NodeID == Node.ID
	HasLogo      bool
	Rollup       apiclient.NodeRollup
}

// blocked reports whether the database would refuse this deletion outright.
func (d deleteImpact) blocked() bool {
	return len(d.Children) > 0 || len(d.ProjectDocs) > 0
}

// deleteImpactOf gathers the dry run from owner-scoped endpoints only.
//
// The artifact count is filtered to this node on purpose: ListArtifacts returns
// the node's own artifacts PLUS its whole ancestor chain PLUS the owner's free
// (node-less) library (internal/usecase/list_artifacts.go:21-51). Unfiltered, the
// report would threaten artifacts that deletion never touches — including the
// owner's entire free library.
func (h *handlers) deleteImpactOf(ctx context.Context, c *apiclient.Client, nodeID string) (deleteImpact, error) {
	node, err := c.GetNode(ctx, nodeID) // authoritative, and the only source of LogoRef
	if err != nil {
		return deleteImpact{}, err
	}
	impact := deleteImpact{Node: node, HasLogo: node.LogoRef != ""}

	nodes, err := h.nodeList(ctx, true) // refresh: a just-created child must be seen
	if err != nil {
		return deleteImpact{}, err
	}
	for _, n := range nodes {
		if n.ParentID != nil && *n.ParentID == node.ID {
			impact.Children = append(impact.Children, n)
		}
	}

	arts, err := c.ListArtifacts(ctx, node.ID)
	if err != nil {
		return deleteImpact{}, err
	}
	for _, a := range arts {
		if a.NodeID == node.ID {
			impact.OwnArtifacts++
		}
	}

	docs, err := c.ListDocumentsScoped(ctx, &node.ID)
	if err != nil {
		return deleteImpact{}, err
	}
	for _, d := range docs {
		if d.Type == domain.DocProject {
			impact.ProjectDocs = append(impact.ProjectDocs, d)
		}
	}

	rollup, err := c.NodeStats(ctx, node.ID)
	if err != nil {
		return deleteImpact{}, err
	}
	impact.Rollup = rollup
	return impact, nil
}

// deleteNode reports first, deletes only on confirm. The client-side report is
// advisory: the server stays the authority, and its 409 reaches the model
// verbatim through h.resultErr.
func (h *handlers) deleteNode(ctx context.Context, req *mcp.CallToolRequest, in deleteNodeIn) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Node) == "" {
		return h.resultErr(errGuard{errors.New("node is required: the id, slug, or name of the node to delete")}), nil, nil
	}
	var out string
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		ref, err := h.lookupNode(ctx, strings.TrimSpace(in.Node))
		if err != nil {
			return err
		}
		impact, err := h.deleteImpactOf(ctx, c, ref.ID)
		if err != nil {
			return err
		}
		if !in.Confirm {
			out = formatDeleteImpact(impact)
			return nil
		}
		if err := c.DeleteNode(ctx, impact.Node.ID); err != nil {
			return err
		}
		if _, lerr := h.nodeList(ctx, true); lerr != nil {
			mcpLog().Warn("could not refresh the node cache after delete", "err", lerr)
		}
		out = fmt.Sprintf("Deleted %s %q (%s).", impact.Node.Kind, impact.Node.Name, impact.Node.Slug)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}
