package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// getNodeIn shows one node in full.
type getNodeIn struct {
	Node string `json:"node,omitempty" jsonschema:"the node to show (id, slug, or name); omit to use the node bound to this directory"`
}

// bindingsForNode narrows an owner-wide binding list to one node. ListBindings
// takes no filter parameters (internal/adapter/apiclient/projectbindings.go:103),
// so the filtering is client-side — and it MUST happen, because the list spans
// every device of this owner.
func bindingsForNode(bs []domain.ProjectBinding, nodeID string) []domain.ProjectBinding {
	var out []domain.ProjectBinding
	for _, b := range bs {
		if b.NodeID == nodeID {
			out = append(out, b)
		}
	}
	return out
}

// getNode is the MCP counterpart to `flow node show`: detail, ancestor chain,
// node tags, this node's bindings and the worktime rollup. The node reference is
// re-read with GetNode so Description, LogoRef and Status are authoritative even
// when the omitted-node branch used the auth-time snapshot.
func (h *handlers) getNode(ctx context.Context, req *mcp.CallToolRequest, in getNodeIn) (*mcp.CallToolResult, any, error) {
	var out string
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		ref, err := h.nodeTarget(ctx, in.Node)
		if err != nil {
			return err
		}
		node, err := c.GetNode(ctx, ref.ID)
		if err != nil {
			return err
		}
		chain, err := c.Ancestors(ctx, node.ID)
		if err != nil {
			return err
		}
		tags, err := c.NodeTags(ctx, node.ID)
		if err != nil {
			return err
		}
		allBindings, err := c.ListBindings(ctx)
		if err != nil {
			return err
		}
		rollup, err := c.NodeStats(ctx, node.ID)
		if err != nil {
			return err
		}
		out = formatNodeDetail(nodeDetail{
			Node: node, Chain: chain, Tags: tags,
			Bindings: bindingsForNode(allBindings, node.ID), Rollup: rollup,
		})
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}
