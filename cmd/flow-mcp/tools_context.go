package main

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

type getContextIn struct {
	Repo string `json:"repo,omitempty" jsonschema:"explicit node slug override; default = the current directory's resolved repo"`
	Cap  int    `json:"cap,omitempty"  jsonschema:"token budget override"`
}

// getContext handles flow_get_context: composes the cross-device start-context
// for the resolved (or caller-supplied) repo and returns the JSON payload.
// Resolution: when repo is empty, the already-resolved project slug is passed as
// q.Node (option B — reuses the cached resolved() state, avoids extra imports).
func (h *handlers) getContext(ctx context.Context, _ *mcp.CallToolRequest, in getContextIn) (*mcp.CallToolResult, any, error) {
	var out string
	err := h.mgr.Do(ctx, func(c *apiclient.Client) error {
		q := apiclient.ContextQuery{Cap: in.Cap}
		if in.Repo != "" {
			q.Node = in.Repo
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
	Repo string   `json:"repo,omitempty" jsonschema:"explicit node slug override; default = the current directory's resolved repo"`
	Body string   `json:"body"           jsonschema:"the activeContext markdown (where I was / what's open / next step)"`
	Tags []string `json:"tags,omitempty" jsonschema:"tags as a flat list; replaces the whole set"`
}

// setActiveContext handles flow_set_active_context: upserts the activeContext
// memory document for the resolved (or caller-supplied) repo.
// Resolution: same option-B approach as getContext.
func (h *handlers) setActiveContext(ctx context.Context, _ *mcp.CallToolRequest, in setActiveContextIn) (*mcp.CallToolResult, any, error) {
	err := h.mgr.Do(ctx, func(c *apiclient.Client) error {
		input := apiclient.SetActiveContextInput{Body: in.Body, Tags: in.Tags}
		if in.Repo != "" {
			input.Node = in.Repo
		} else if proj, matched := h.resolved(); matched {
			input.Node = proj.Slug
		}
		_, err := c.SetActiveContext(ctx, input)
		return err
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult("active-context updated"), nil, nil
}
