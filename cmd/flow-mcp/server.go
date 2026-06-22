package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

const serverName = "flow-mcp"
const serverVersion = "0.1.0"

// handlers carries the dependencies every tool needs: the authManager (which
// owns the authenticated client and recovery lifecycle), the cwd-resolved
// project (written once by onAuth under projMu), and a lazily-fetched
// project-ref cache used to resolve explicit `project` arguments.
type handlers struct {
	mgr *authManager
	srv *mcp.Server

	// resolved-project state, written once by onAuth under projMu.
	proj    domain.Project
	matched bool

	// project-ref cache (2b), guarded by projMu. listProjects fetches via the
	// manager's current client so a rebuild is always reflected.
	projMu       sync.Mutex
	projects     []domain.Project
	projFetched  bool
	listProjects func(ctx context.Context) ([]domain.Project, error)
}

// newServerH builds the server + handlers and returns both. It also sets
// mgr.onAuth to this handlers' run-once post-auth init and h.srv to the server,
// and wires the project-ref fetch seam through the manager.
func newServerH(mgr *authManager) (*mcp.Server, *handlers) {
	h := &handlers{mgr: mgr}
	h.listProjects = func(ctx context.Context) ([]domain.Project, error) {
		c, err := mgr.client(ctx)
		if err != nil {
			return nil, err
		}
		return c.ListProjects(ctx)
	}
	mgr.onAuth = h.postAuthInit
	s := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: serverVersion}, nil)
	h.srv = s
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_project_context",
		Description: "Report which flow project the current working directory resolves to, and how many Kompendium documents are in scope. Call this first to orient.",
	}, h.projectContext)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_search_docs",
		Description: "Search the Kompendium (hybrid keyword + semantic). Scoped to the current project by default; pass project='global' to search everything. Returns matching documents with snippets.",
	}, h.searchDocs)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_list_docs",
		Description: "List Kompendium documents (metadata only) in the current project by default. Filter by project, tags, or type.",
	}, h.listDocs)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_get_doc",
		Description: "Fetch one document's full content by id, or by path within the current project.",
	}, h.getDoc)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_list_tags",
		Description: "List tag counts for filtering — across the current project by default, or project='global' for all.",
	}, h.listTags)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_backlinks",
		Description: "List documents that link (via wikilinks) to a given document, by id or path. Navigates the memory graph.",
	}, h.backlinks)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_create_doc",
		Description: "Create a Kompendium document in the current project by default. Tags are set via YAML frontmatter in the body. Type must be one of: daily, project, free, agent, memory, instruction, skill, plan.",
	}, h.createDoc)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_update_doc",
		Description: "Update a document's title and/or body by id (partial: omit a field to keep it). Modifying a human-owned note (daily/project/free) requires confirm=true.",
	}, h.updateDoc)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_delete_doc",
		Description: "Delete a document by id. Deleting a human-owned note (daily/project/free) requires confirm=true.",
	}, h.deleteDoc)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_list_projects",
		Description: "List all flow projects (id, name, slug). Use this to find an existing project before flow_bind_project, to avoid creating a duplicate.",
	}, h.listProjectsTool)
	return s, h
}

// resolved returns the project + matched flag under the projMu lock, safe for
// concurrent access since onAuth may write these during a live session.
func (h *handlers) resolved() (domain.Project, bool) {
	h.projMu.Lock()
	defer h.projMu.Unlock()
	return h.proj, h.matched
}

// resultErr maps a backend error to a tool result: errGuard errors render
// verbatim; errLoginRequired → the standard login-required message; anything
// else → a generic actionable error.
func (h *handlers) resultErr(err error) *mcp.CallToolResult {
	var g errGuard
	if errors.As(err, &g) {
		return errorResult(g.Error())
	}
	if errors.Is(err, errLoginRequired) {
		return h.loginRequired()
	}
	return errorResult("flow server error: " + err.Error())
}

// postAuthInit runs once on the first successful auth (mgr.onAuth): resolve the
// cwd→project, then register the project's documents as resources.
func (h *handlers) postAuthInit(ctx context.Context, c *apiclient.Client) {
	proj, matched := resolveProject(ctx, c, mcpLog())
	h.projMu.Lock()
	h.proj, h.matched = proj, matched
	h.projMu.Unlock()
	if err := h.registerResources(ctx, c); err != nil {
		mcpLog().Warn("could not register document resources", "err", err)
	}
}

// mcpLog returns a stderr logger for use by the MCP server. stdout is owned
// by StdioTransport for the JSON-RPC stream.
func mcpLog() *slog.Logger { return slog.New(slog.NewTextHandler(os.Stderr, nil)) }

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
