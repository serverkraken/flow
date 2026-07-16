package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

type getContextIn struct {
	Repo    string `json:"repo,omitempty" jsonschema:"explicit node id, slug, name, origin slug, or upstream Git remote; default = the current directory's resolved repo"`
	Cap     int    `json:"cap,omitempty"  jsonschema:"hard token budget override"`
	Profile string `json:"profile,omitempty" jsonschema:"context shape: handoff (resolution, active context, instructions), standard (plus selected memories), or full (plus curation diagnostics); default standard"`
}

// getContext handles flow_get_context: composes the cross-device start-context
// for the resolved (or caller-supplied) repo and returns the JSON payload.
// Resolution: when repo is empty, the already-resolved project slug is passed as
// q.Node (option B — reuses the cached resolved() state, avoids extra imports).
func (h *handlers) getContext(ctx context.Context, req *mcp.CallToolRequest, in getContextIn) (*mcp.CallToolResult, any, error) {
	var out string
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		q := apiclient.ContextQuery{Cap: in.Cap, Profile: in.Profile, Client: clientName(req)}
		if in.Repo != "" {
			node, err := h.lookupNode(ctx, in.Repo)
			if err != nil {
				return err
			}
			q.Node = node.Slug
		} else if proj, matched := h.resolved(); matched {
			q.Node = proj.Slug
		}
		cc, err := c.ComposeContext(ctx, q)
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(cc, "", "  ")
		out = string(b)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}

type setActiveContextIn struct {
	Repo string   `json:"repo,omitempty" jsonschema:"explicit node id, slug, name, origin slug, or upstream Git remote; default = the current directory's resolved repo"`
	Body string   `json:"body"           jsonschema:"the activeContext markdown (where I was / what's open / next step)"`
	Tags []string `json:"tags,omitempty" jsonschema:"tags as a flat list; replaces the whole set"`
}

// setActiveContext handles flow_set_active_context: upserts the activeContext
// memory document for the resolved (or caller-supplied) repo.
// Resolution: same option-B approach as getContext.
func (h *handlers) setActiveContext(ctx context.Context, req *mcp.CallToolRequest, in setActiveContextIn) (*mcp.CallToolResult, any, error) {
	var out *mcp.CallToolResult
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		input := apiclient.SetActiveContextInput{Body: in.Body, Tags: in.Tags}
		project := ""
		if in.Repo != "" {
			node, err := h.lookupNode(ctx, in.Repo)
			if err != nil {
				return err
			}
			input.Node = node.Slug
			project = node.Slug
		} else if proj, matched := h.resolved(); matched {
			input.Node = proj.Slug
			project = proj.Slug
		} else {
			return errGuard{err: fmt.Errorf("no project is bound to this directory; use flow_bind_project or pass repo explicitly")}
		}
		result, err := c.SetActiveContext(ctx, input)
		if err != nil {
			return err
		}
		out = structuredWriteResult("active_context_updated", result.ID, project, result.UpdatedAt, in.Body)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return out, nil, nil
}
