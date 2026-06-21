package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// projectContextIn has no parameters.
type projectContextIn struct{}

// projectContext reports the resolved project and its in-scope document count.
// Out is `any` (no output schema) — the result is concise plain text per the
// design spec.
func (h *handlers) projectContext(ctx context.Context, _ *mcp.CallToolRequest, _ projectContextIn) (*mcp.CallToolResult, any, error) {
	if !h.authed {
		return h.loginRequired(), nil, nil
	}
	if !h.matched {
		return textResult("No flow project is bound to this directory. Set FLOW_PROJECT, or bind it (flow_bind_project, coming in a later version)."), nil, nil
	}
	docs, err := h.client.ListDocumentsScoped(ctx, &h.proj.ID)
	if err != nil {
		return errorResult(fmt.Sprintf("flow server error: %v", err)), nil, nil
	}
	msg := fmt.Sprintf("Project: %s (%s) — %d document(s) in scope. Resolved for this working directory.", h.proj.Name, h.proj.Slug, len(docs))
	return textResult(msg), nil, nil
}
