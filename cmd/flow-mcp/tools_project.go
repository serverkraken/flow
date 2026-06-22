package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

// projectContextIn has no parameters.
type projectContextIn struct{}

// projectContext reports the resolved project and its in-scope document count.
// Out is `any` (no output schema) — the result is concise plain text per the
// design spec.
func (h *handlers) projectContext(ctx context.Context, _ *mcp.CallToolRequest, _ projectContextIn) (*mcp.CallToolResult, any, error) {
	proj, matched := h.resolved()
	if !matched {
		// Either unauthed (no project resolved yet) or genuinely unbound. Probe
		// auth so a logged-out caller gets the actionable login message.
		if _, err := h.mgr.client(ctx); err != nil {
			return h.resultErr(err), nil, nil
		}
		return textResult("No flow project is bound to this directory. Set FLOW_PROJECT, or bind it (flow_bind_project, coming in a later version)."), nil, nil
	}
	var count int
	err := h.mgr.Do(ctx, func(c *apiclient.Client) error {
		docs, err := c.ListDocumentsScoped(ctx, &proj.ID)
		if err != nil {
			return err
		}
		count = len(docs)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	msg := fmt.Sprintf("Project: %s (%s) — %d document(s) in scope. Resolved for this working directory.", proj.Name, proj.Slug, count)
	return textResult(msg), nil, nil
}
