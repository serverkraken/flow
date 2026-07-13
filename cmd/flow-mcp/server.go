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
	proj    domain.Node
	matched bool

	// project-ref cache (2b), guarded by projMu. listProjects fetches via the
	// manager's current client so a rebuild is always reflected.
	projMu       sync.Mutex
	projects     []domain.Node
	projFetched  bool
	listProjects func(ctx context.Context) ([]domain.Node, error)
}

// newServerH builds the server + handlers and returns both. It also sets
// mgr.onAuth to this handlers' run-once post-auth init and h.srv to the server,
// and wires the project-ref fetch seam through the manager.
func newServerH(mgr *authManager) (*mcp.Server, *handlers) {
	h := &handlers{mgr: mgr}
	h.listProjects = func(ctx context.Context) ([]domain.Node, error) {
		c, err := mgr.client(ctx)
		if err != nil {
			return nil, err
		}
		return c.ListNodes(ctx)
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
		Description: "Create a Kompendium document in the current project by default. Tags are set via YAML frontmatter in the body. Type must be one of: daily, project, free, memory, instruction, skill, plan, spec, activecontext (agent: deprecated).",
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
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_bind_project",
		Description: "Bind the current working directory to a flow project so other tools auto-scope here. Pass project (existing id/slug/name) or create_name (to create one). Auto-detects a git-origin (remote) vs per-device (path) binding; override with kind.",
	}, h.bindProject)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_update_node",
		Description: "Update a node's metadata (name, description, color, glyph, icon, upstream, status) and/or rate — only the fields you pass change. Scoped to the current project by default; pass node to target another. This is how an agent maintains a project's description without the TUI.",
	}, h.updateNode)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_get_context",
		Description: "Compose the cross-device start-context (instructions + activeContext + memories) for the current repo, token-budgeted.",
	}, h.getContext)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_set_active_context",
		Description: "Upsert this repo's activeContext memory (where I was / what's open / next step).",
	}, h.setActiveContext)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_archive_doc",
		Description: "Archive a context doc (out of bootstrap + default lists/search, but findable + reversible) or un-archive it. Safe, reversible — use this to retire done/historical memories instead of deleting them.",
	}, h.archiveDoc)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_upload_artifact",
		Description: "Upload an artifact (image or downloadable file) onto a node. Scoped to the current project by default; pass node to target another. Images render inline via ![[slug]] in Kompendium docs; other MIME types are download links.",
	}, h.uploadArtifact)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_list_artifacts",
		Description: "List a node's artifact library (its own artifacts plus its ancestors', not its subtree). Scoped to the current project by default; pass node to target another.",
	}, h.listArtifacts)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_delete_artifact",
		Description: "Delete an artifact by slug from a node. Scoped to the current project by default; pass node to target another.",
	}, h.deleteArtifact)
	return s, h
}

// resolved returns the project + matched flag under the projMu lock, safe for
// concurrent access since onAuth may write these during a live session.
func (h *handlers) resolved() (domain.Node, bool) {
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

// refreshResolved re-resolves the cwd→project and re-registers the project's
// documents as resources, overwriting the resolved state under projMu. Run once
// by postAuthInit and again by flow_bind_project after a successful bind.
func (h *handlers) refreshResolved(ctx context.Context, c *apiclient.Client) {
	proj, matched := resolveProject(ctx, c, mcpLog())
	h.projMu.Lock()
	h.proj, h.matched = proj, matched
	h.projMu.Unlock()
	if err := h.registerResources(ctx, c); err != nil {
		mcpLog().Warn("could not register document resources", "err", err)
	}
}

// postAuthInit runs once on the first successful auth (mgr.onAuth).
func (h *handlers) postAuthInit(ctx context.Context, c *apiclient.Client) {
	h.refreshResolved(ctx, c)
}

// mcpLogger is the package-level stderr logger for the MCP server. stdout is
// owned by StdioTransport for the JSON-RPC stream.
var mcpLogger = slog.New(slog.NewTextHandler(os.Stderr, nil))

func mcpLog() *slog.Logger { return mcpLogger }

// clientName returns the actor name for attribution. The FLOW_ACTOR env var
// takes precedence (useful for testing or explicit override). Otherwise the
// MCP ClientInfo.Name reported by the connected client is used.
func clientName(req *mcp.CallToolRequest) string {
	if v := os.Getenv("FLOW_ACTOR"); v != "" {
		return v
	}
	if req == nil || req.Session == nil {
		return ""
	}
	ip := req.Session.InitializeParams()
	if ip == nil || ip.ClientInfo == nil {
		return ""
	}
	return ip.ClientInfo.Name
}

// do is a DRY wrapper around h.mgr.Do that applies WithActor from the calling
// tool's MCP request so every REST call carries X-Flow-Actor when the client
// is identifiable.
func (h *handlers) do(ctx context.Context, req *mcp.CallToolRequest, fn func(*apiclient.Client) error) error {
	name := clientName(req)
	return h.mgr.Do(ctx, func(c *apiclient.Client) error {
		if name != "" {
			c = c.WithActor(name)
		}
		return fn(c)
	})
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
