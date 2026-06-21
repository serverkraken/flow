package main

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

const serverName = "flow-mcp"
const serverVersion = "0.1.0"

// handlers carries the dependencies every tool needs: the authenticated client,
// whether auth succeeded at boot, and the cwd-resolved project.
type handlers struct {
	client  *apiclient.Client
	authed  bool
	proj    domain.Project
	matched bool
}

// newServer builds the MCP server and registers the spine's tools. Kept
// dependency-injected (no global state, no I/O) so loopback tests can drive it.
func newServer(client *apiclient.Client, authed bool, proj domain.Project, matched bool) *mcp.Server {
	h := &handlers{client: client, authed: authed, proj: proj, matched: matched}
	s := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: serverVersion}, nil)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_project_context",
		Description: "Report which flow project the current working directory resolves to, and how many Kompendium documents are in scope. Call this first to orient.",
	}, h.projectContext)
	return s
}

// textResult wraps a plain-text success result.
func textResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

// errorResult wraps an actionable error result (IsError=true).
func errorResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

// loginRequired is the standard degraded-mode short-circuit.
func (h *handlers) loginRequired() *mcp.CallToolResult {
	return errorResult("Login required: run 'flow login' in a terminal on this device.")
}
