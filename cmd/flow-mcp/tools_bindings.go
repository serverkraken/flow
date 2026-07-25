package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// bindNodeIn binds a directory or a git remote to an EXISTING node. The old
// create_name/create_parent branch is gone — creating is flow_create_node's job.
// Omitting both path and remote keeps the historical behaviour: bind the
// flow-mcp process's working directory (Spec §3 flow_bind_project).
type bindNodeIn struct {
	Project string `json:"project,omitempty" jsonschema:"the existing node to bind to: id, slug, or name. Create a new node with flow_create_node."`
	Path    string `json:"path,omitempty" jsonschema:"an existing directory to bind; ~ and relative paths resolve against the flow-mcp process. Mutually exclusive with remote; omit both to bind the process's working directory."`
	Remote  string `json:"remote,omitempty" jsonschema:"a git clone URL or host/path slug to bind; no local checkout needed. Mutually exclusive with path."`
	Kind    string `json:"kind,omitempty" jsonschema:"binding kind override: 'remote' (git origin) or 'path' (this device); omit to auto-detect"`
}

// bindProject binds the resolved target to an existing node, then re-resolves so
// subsequent tools are scoped there.
func (h *handlers) bindProject(ctx context.Context, req *mcp.CallToolRequest, in bindNodeIn) (*mcp.CallToolResult, any, error) {
	env, err := liveBindEnv()
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	tgt, err := resolveBindTarget(bindTargetArgs{Path: in.Path, Remote: in.Remote, Kind: in.Kind}, env)
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	var bound domain.Node
	derr := h.do(ctx, req, func(c *apiclient.Client) error {
		node, e := h.bindNodeCore(ctx, c, in.Project, tgt)
		if e != nil {
			return e
		}
		bound = node
		h.refreshResolved(ctx, c)
		return nil
	})
	if derr != nil {
		return h.resultErr(derr), nil, nil
	}
	return textResult(fmt.Sprintf("Bound %s to project %s (%s) via %s binding. flow_project_context now resolves here.",
		bindTargetLabel(tgt), bound.Name, bound.Slug, tgt.Kind)), nil, nil
}
