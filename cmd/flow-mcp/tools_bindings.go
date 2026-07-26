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
	// Omitting both path and remote binds the flow-mcp PROCESS's own working
	// directory (bindNodeIn's documented contract). refreshResolved always
	// re-resolves that same cwd (resolve.go), so it is only this call shape
	// where re-resolving after the write can change what flow_project_context
	// reports — and only this call shape may say so. An explicit path or
	// remote addresses a directory that need not be (and typically isn't) the
	// process's cwd; claiming "flow_project_context now resolves here" for
	// that case would be false, and re-resolving cwd would just repeat the
	// document-resolution + resource reconciliation that already ran for it.
	boundCwd := strings.TrimSpace(in.Path) == "" && strings.TrimSpace(in.Remote) == ""
	var bound domain.Node
	derr := h.do(ctx, req, func(c *apiclient.Client) error {
		node, e := h.bindNodeCore(ctx, c, in.Project, tgt)
		if e != nil {
			return e
		}
		bound = node
		if boundCwd {
			h.refreshResolved(ctx, c)
		}
		return nil
	})
	if derr != nil {
		return h.resultErr(derr), nil, nil
	}
	msg := fmt.Sprintf("Bound %s to project %s (%s) via %s binding.",
		bindTargetLabel(tgt), bound.Name, bound.Slug, tgt.Kind)
	if boundCwd {
		msg += " flow_project_context now resolves here."
	}
	return textResult(msg), nil, nil
}

// nodeBindingActions is the action whitelist; every error message lists it.
var nodeBindingActions = []string{"bind", "unbind", "list", "resolve"}

// nodeBindingIn manages the bindings that map a directory or a git remote to a
// node. resolve is an action of this family rather than its own tool: it is the
// same question as list, asked from the other side (Spec §3).
type nodeBindingIn struct {
	Action string `json:"action" jsonschema:"bind, unbind, list, or resolve"`
	Node   string `json:"node,omitempty" jsonschema:"the node to bind to (id, slug, or name) — REQUIRED for bind, an optional filter for list, and REJECTED for unbind and resolve, which address a binding by its target alone"`
	Path   string `json:"path,omitempty" jsonschema:"an existing directory; ~ and relative paths resolve against the flow-mcp process. Mutually exclusive with remote; omit both to use the process's working directory. Ignored by action=list."`
	Remote string `json:"remote,omitempty" jsonschema:"a git clone URL or host/path slug; no local checkout needed. Mutually exclusive with path. Ignored by action=list."`
	Kind   string `json:"kind,omitempty" jsonschema:"binding kind override for bind and unbind only: 'remote' (git origin) or 'path' (this device); omit to auto-detect. Rejected for list and resolve, which report what the server already resolved."`
}

// validateNodeBinding checks the action and the node/action pairing, returning
// the trimmed action. unbind and resolve address a binding purely by its target:
// UnbindRemote, UnbindPath and ResolveNode take no node id at all
// (internal/adapter/apiclient/projectbindings.go:82,96,39), so a passed node
// would be silently ignored — which is why it is rejected instead.
func validateNodeBinding(in nodeBindingIn) (string, error) {
	action := strings.TrimSpace(in.Action)
	hasNode := strings.TrimSpace(in.Node) != ""
	switch action {
	case "bind":
		if !hasNode {
			return "", errGuard{errors.New(`action "bind" needs "node": the id, slug, or name of the node to bind the target to`)}
		}
	case "unbind", "resolve":
		if hasNode {
			return "", errGuard{fmt.Errorf(`action %q addresses a binding by its target only — drop "node" and pass "path" or "remote"`, action)}
		}
	case "list":
		// node is an optional client-side filter here.
	default:
		return "", errGuard{fmt.Errorf("invalid action %q; use one of: %s", action, strings.Join(nodeBindingActions, ", "))}
	}
	// kind steers WHICH binding is written or deleted, so it is meaningful only
	// for bind and unbind. resolve asks the server the same question every other
	// tool asks, and that chain is remote-first by definition
	// (domain.ResolveBinding, internal/domain/projectbinding.go:31-38): a remote
	// match wins over a path match. Accepting kind here would look like an
	// override and change nothing — so it is rejected rather than ignored.
	if strings.TrimSpace(in.Kind) != "" && (action == "resolve" || action == "list") {
		return "", errGuard{fmt.Errorf(`action %q does not take "kind": it reports what the server already resolved, and that chain always prefers a remote binding over a path binding`, action)}
	}
	return action, nil
}

// nodeBinding runs one of the four binding actions.
func (h *handlers) nodeBinding(ctx context.Context, req *mcp.CallToolRequest, in nodeBindingIn) (*mcp.CallToolResult, any, error) {
	action, err := validateNodeBinding(in)
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	// list never touches the filesystem: it reports what the server already
	// knows, so a stale or absent path argument must not make it fail.
	var tgt bindTarget
	if action != "list" {
		env, eerr := liveBindEnv()
		if eerr != nil {
			return h.resultErr(eerr), nil, nil
		}
		tgt, eerr = resolveBindTarget(bindTargetArgs{Path: in.Path, Remote: in.Remote, Kind: in.Kind}, env)
		if eerr != nil {
			return h.resultErr(eerr), nil, nil
		}
	}
	var out string
	derr := h.do(ctx, req, func(c *apiclient.Client) error {
		switch action {
		case "bind":
			node, lerr := h.lookupNode(ctx, strings.TrimSpace(in.Node))
			if lerr != nil {
				return lerr
			}
			if berr := bindTargetTo(ctx, c, node.ID, tgt); berr != nil {
				return berr
			}
			h.refreshResolved(ctx, c)
			out = fmt.Sprintf("Bound %s to %s %q (%s) via %s binding.",
				bindTargetLabel(tgt), node.Kind, node.Name, node.Slug, tgt.Kind)
		case "unbind":
			if uerr := unbindTarget(ctx, c, tgt); uerr != nil {
				return uerr
			}
			h.refreshResolved(ctx, c)
			out = fmt.Sprintf("Unbound %s (%s binding).", bindTargetLabel(tgt), tgt.Kind)
		case "list":
			rows, rerr := h.bindingRows(ctx, c, strings.TrimSpace(in.Node))
			if rerr != nil {
				return rerr
			}
			label := "for this owner across all devices"
			if ref := strings.TrimSpace(in.Node); ref != "" {
				label = "for node " + ref
			}
			out = formatBindingRows(rows, label)
		default: // resolve
			node, ok, rerr := c.ResolveNode(ctx, tgt.RemoteSlug, tgt.MachineID, tgt.Path)
			if rerr != nil {
				return rerr
			}
			if !ok {
				out = fmt.Sprintf(`Nothing is bound to %s. Bind it with flow_node_binding action="bind".`, bindTargetLabel(tgt))
				return nil
			}
			eng, engOK, eerr := c.ResolveEngagement(ctx, tgt.RemoteSlug, tgt.MachineID, tgt.Path)
			if eerr != nil {
				return eerr
			}
			out = formatResolvedTarget(bindTargetLabel(tgt), node, eng, engOK)
		}
		return nil
	})
	if derr != nil {
		return h.resultErr(derr), nil, nil
	}
	return textResult(out), nil, nil
}

// bindingRows fetches this owner's bindings and joins each to its node's
// identity. The optional nodeRef filter is applied client-side because
// ListBindings takes no filter parameters
// (internal/adapter/apiclient/projectbindings.go:103).
func (h *handlers) bindingRows(ctx context.Context, c *apiclient.Client, nodeRef string) ([]bindingRow, error) {
	bs, err := c.ListBindings(ctx)
	if err != nil {
		return nil, err
	}
	if nodeRef != "" {
		node, lerr := h.lookupNode(ctx, nodeRef)
		if lerr != nil {
			return nil, lerr
		}
		bs = bindingsForNode(bs, node.ID)
	}
	nodes, err := h.nodeList(ctx, false)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]domain.Node, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}
	rows := make([]bindingRow, 0, len(bs))
	for _, b := range bs {
		// A binding whose node is missing from the cache stays visible and
		// identifiable by its id rather than silently rendering blank.
		row := bindingRow{Binding: b, NodeName: "(unknown node)", NodeSlug: b.NodeID}
		if n, ok := byID[b.NodeID]; ok {
			row.NodeName, row.NodeSlug = n.Name, n.Slug
		}
		rows = append(rows, row)
	}
	return rows, nil
}
