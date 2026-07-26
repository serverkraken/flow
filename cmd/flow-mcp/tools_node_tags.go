package main

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

// setNodeTagsIn REPLACES a node's complete tag set — the same semantics
// flow_create_doc has for document tags. Tags has no `omitempty`, so the SDK's
// generated schema marks it required and rejects a call that omits the key
// before the handler ever runs. A Go slice field's generated schema also
// allows type "null", so a call that passes an explicit JSON null for tags is
// schema-valid and reaches the handler as a nil slice — indistinguishable
// there from [] except by the nil check below, which is what stops a model
// sending null from silently wiping a node's tags (Spec §3 flow_set_node_tags).
type setNodeTagsIn struct {
	Node string   `json:"node,omitempty" jsonschema:"the node whose tags to replace (id, slug, or name); omit to use the node bound to this directory"`
	Tags []string `json:"tags" jsonschema:"the COMPLETE tag set. This REPLACES the node's tags — every tag you omit is REMOVED. Pass [] to clear them."`
}

// setNodeTags replaces the tag set and reports the result.
func (h *handlers) setNodeTags(ctx context.Context, req *mcp.CallToolRequest, in setNodeTagsIn) (*mcp.CallToolResult, any, error) {
	if in.Tags == nil {
		return h.resultErr(errGuard{errors.New(`tags is required: pass the COMPLETE tag set (it REPLACES the node's tags), or [] to clear them`)}), nil, nil
	}
	var out string
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		ref, err := h.nodeTarget(ctx, in.Node)
		if err != nil {
			return err
		}
		tags, err := c.SetNodeTags(ctx, ref.ID, in.Tags)
		if err != nil {
			return err
		}
		out = formatNodeTags(ref, tags)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}
