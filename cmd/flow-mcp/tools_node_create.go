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

// nodeKindsForCreate is the kind whitelist. domain.KindBranch is deliberately
// absent: it is reserved without behavior (internal/domain/node.go:24), and
// there is no domain.NodeKinds() helper to defer to.
var nodeKindsForCreate = []string{"engagement", "vorhaben", "repo"}

// createNodeIn is the only way to create a node over MCP. There is no slug
// parameter — the server derives the slug from the name, like `flow node create`;
// renaming is flow_update_node's job. There is no icon parameter either, because
// apiclient.CreateNodeFields carries none (internal/adapter/apiclient/nodes.go:27);
// set it with flow_update_node right after.
type createNodeIn struct {
	Name               string `json:"name" jsonschema:"the node's display name; the server derives the slug from it"`
	Kind               string `json:"kind" jsonschema:"engagement (always a root), vorhaben, or repo"`
	Parent             string `json:"parent,omitempty" jsonschema:"the parent node (id, slug, or name) — REQUIRED for vorhaben and repo, and rejected for engagement, which is always a root. flow_list_projects shows the tree."`
	Description        string `json:"description,omitempty" jsonschema:"one-line subtitle"`
	Color              string `json:"color,omitempty" jsonschema:"identity color name"`
	Glyph              string `json:"glyph,omitempty" jsonschema:"identity glyph"`
	Upstream           string `json:"upstream,omitempty" jsonschema:"git clone URL; valid for kind=repo only"`
	CountsTowardTarget *bool  `json:"counts_toward_target,omitempty" jsonschema:"true = Work (counts toward the daily target), false = Privat (tracked only); omit to inherit from the ancestor chain"`
	BindPath           string `json:"bind_path,omitempty" jsonschema:"optional, kind=repo ONLY: bind this directory to the new node in the same atomic command. The directory must exist; ~ and relative paths resolve against the flow-mcp process, so \".\" is its working directory. Omit for a node without any binding; to bind an engagement or vorhaben, create it first and use flow_node_binding."`
}

// validateCreateNode is the parameter-only pre-check. It turns a bad
// kind/parent/upstream combination into a precise message instead of a bare
// server 400; the server stays the authority (domain.ValidParentKind). The
// parent's KIND can only be checked after the lookup, so that rule lives in the
// handler.
func validateCreateNode(in createNodeIn) error {
	if strings.TrimSpace(in.Name) == "" {
		return errGuard{errors.New("name is required")}
	}
	kind := strings.TrimSpace(in.Kind)
	hasParent := strings.TrimSpace(in.Parent) != ""
	switch domain.NodeKind(kind) {
	case domain.KindEngagement:
		if hasParent {
			return errGuard{errors.New(`an engagement is always a root: drop "parent", or use kind "vorhaben" or "repo" to nest something under it`)}
		}
	case domain.KindVorhaben, domain.KindRepo:
		if !hasParent {
			return errGuard{fmt.Errorf(`kind %q needs a "parent": the id, slug, or name of an engagement or vorhaben (flow_list_projects shows the tree)`, kind)}
		}
	case domain.KindBranch:
		return errGuard{fmt.Errorf(`kind "branch" is reserved and has no behavior yet; use one of: %s`, strings.Join(nodeKindsForCreate, ", "))}
	default:
		return errGuard{fmt.Errorf("invalid kind %q; use one of: %s", kind, strings.Join(nodeKindsForCreate, ", "))}
	}
	if strings.TrimSpace(in.Upstream) != "" && domain.NodeKind(kind) != domain.KindRepo {
		return errGuard{fmt.Errorf(`"upstream" is only valid for kind "repo", not %q`, kind)}
	}
	// bind_path is repo-only, and this guard is NOT cosmetic: the atomic usecase
	// rejects anything else outright ("bound node must be a repo",
	// internal/usecase/create_bound_node.go:46). Note the asymmetry with the
	// separate bind endpoint, which also allows a childless vorhaben
	// (internal/usecase/bind_node.go:64-75) — so a vorhaben is bound in a second
	// step with flow_node_binding, not atomically here.
	if strings.TrimSpace(in.BindPath) != "" && domain.NodeKind(kind) != domain.KindRepo {
		return errGuard{fmt.Errorf(`"bind_path" is only valid for kind "repo", not %q; create the %s first, then bind it with flow_node_binding (action="bind")`, kind, kind)}
	}
	return nil
}

// createNode creates a node — with bind_path through CreateBoundNode so node and
// binding stay ONE atomic REST command (Finding 56 of the 2026-07-15 review),
// otherwise through plain CreateNode. Afterwards the node cache is invalidated so
// the new node is addressable on the very next call, and with a binding the
// cwd→node resolution is refreshed too, exactly as bindProject does.
func (h *handlers) createNode(ctx context.Context, req *mcp.CallToolRequest, in createNodeIn) (*mcp.CallToolResult, any, error) {
	if err := validateCreateNode(in); err != nil {
		return h.resultErr(err), nil, nil
	}
	binding := strings.TrimSpace(in.BindPath) != ""
	var tgt bindTarget
	if binding {
		env, err := liveBindEnv()
		if err != nil {
			return h.resultErr(err), nil, nil
		}
		// bind_path goes through the same target resolution as flow_node_binding,
		// so a bad path fails here — before any node is created.
		tgt, err = resolveBindTarget(bindTargetArgs{Path: in.BindPath}, env)
		if err != nil {
			return h.resultErr(err), nil, nil
		}
	}
	var out string
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		kind := domain.NodeKind(strings.TrimSpace(in.Kind))
		var parentID *string
		if ref := strings.TrimSpace(in.Parent); ref != "" {
			parent, perr := h.lookupNode(ctx, ref)
			if perr != nil {
				return prefixGuard("parent", perr)
			}
			if !domain.ValidParentKind(kind, parent.Kind) {
				return errGuard{fmt.Errorf("a %s cannot hang under %s %q (%s): a parent must be an engagement or a vorhaben",
					kind, parent.Kind, parent.Name, parent.Slug)}
			}
			parentID = &parent.ID
		}
		fields := apiclient.CreateNodeFields{
			Name: strings.TrimSpace(in.Name), Kind: string(kind), ParentID: parentID,
			Color: in.Color, Glyph: in.Glyph, Description: in.Description,
			UpstreamGit: strings.TrimSpace(in.Upstream), CountsTowardTarget: in.CountsTowardTarget,
		}
		var node domain.Node
		if binding {
			result, cerr := c.CreateBoundNode(ctx, apiclient.CreateBoundNodeInput{
				Node: fields, Binding: bindingFieldsFor(tgt),
			})
			if cerr != nil {
				return cerr
			}
			node = result.Node
		} else {
			created, cerr := c.CreateNode(ctx, fields)
			if cerr != nil {
				return cerr
			}
			node = created
		}
		if _, lerr := h.nodeList(ctx, true); lerr != nil {
			mcpLog().Warn("could not refresh the node cache after create", "err", lerr)
		}
		out = fmt.Sprintf("Created %s %q (%s), id %s.", node.Kind, node.Name, node.Slug, node.ID)
		if binding {
			h.refreshResolved(ctx, c)
			out += fmt.Sprintf(" Bound %s to it via %s binding.", bindTargetLabel(tgt), tgt.Kind)
		}
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}
