package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
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

	resourceRefreshMu sync.Mutex
	resourceMu        sync.Mutex
	resources         map[string]string // document id -> descriptor fingerprint
}

// newServerH builds the server + handlers and returns both. It also sets
// mgr.onAuth to this handlers' per-client post-auth reconciliation and h.srv,
// and wires the project-ref fetch seam through the manager.
func newServerH(mgr *authManager) (*mcp.Server, *handlers) {
	h := &handlers{mgr: mgr, resources: make(map[string]string)}
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
		Description: "Create a Kompendium document in the current project by default. Tags are a flat list; daily documents accept date=YYYY-MM-DD and derive their path. Type must be one of: daily, project, free, memory, instruction, skill, plan, spec, activecontext (agent: deprecated).",
	}, h.createDoc)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_update_doc",
		Description: "CAS-safe partial update of a document's title, body, and/or tags. Returns id, canonical project, version, updatedAt, and hash. Modifying a human-owned note requires confirm=true.",
	}, h.updateDoc)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_patch_doc",
		Description: "CAS-safe Markdown mutation without loading a large document into model context: replace_section, append_section, or set_checkbox (optionally changing checkbox label/status atomically). Returns id, canonical project, version, updatedAt, and hash.",
	}, h.patchDoc)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_move_doc",
		Description: "Atomically reclassify a document's type, project, path, and date. Daily paths are derived from date. Modifying a human-owned note requires confirm=true.",
	}, h.moveDoc)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_delete_doc",
		Description: "Delete a document by id. Deleting a human-owned note (daily/project/free) requires confirm=true.",
	}, h.deleteDoc)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_list_projects",
		Description: "List the complete flow node hierarchy as an indented tree — kind glyph, name, slug, kind, status and id per line, two spaces per level. Use this to find an existing node before binding, and to pick a valid parent for flow_create_node (an engagement is always a root; a vorhaben or repo needs an engagement or vorhaben as parent).",
	}, h.listProjectsTool)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_bind_project",
		Description: "Bind a directory or a git remote to an EXISTING flow node so other tools auto-scope there. Pass project (id/slug/name) plus optionally path (a directory that must exist; ~ and relative paths resolve against the flow-mcp process) or remote (a clone URL or host/path slug, no local checkout needed); omit both to bind the process's working directory. Auto-detects a git-origin (remote) vs per-device (path) binding; override with kind. A target can only be bound to ONE node: binding a target that is already bound MOVES it to the new node — check with flow_node_binding action=resolve first. To create a node use flow_create_node; for unbind/list/resolve use flow_node_binding.",
	}, h.bindProject)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_update_node",
		Description: "Update a node's metadata (name, description, color, glyph, icon, upstream, status) and/or rate — only the fields you pass change. Scoped to the current project by default; pass node to target another. This is how an agent maintains a project's description without the TUI.",
	}, h.updateNode)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_get_context",
		Description: "Compose the cross-device start-context for the current repo with a hard token cap. Profiles: handoff, standard (default), full.",
	}, h.getContext)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_context_inventory",
		Description: "Inspect every context candidate for the current repo as metadata only, including its included, dropped, hidden, or always standing and the reason. Use before curating.",
	}, h.contextInventory)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_curate_context",
		Description: "Apply exactly one checked context action to a document: set mode (auto/immer/nie), pin/unpin, or archive/un-archive. Returns the resulting standing and budget.",
	}, h.curateContext)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_reorder_context",
		Description: "Atomically reorder the complete ranked context set for the current repo. Requires every currently ranked document id exactly once.",
	}, h.reorderContext)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_set_active_context",
		Description: "Upsert this repo's activeContext memory (where I was / what's open / next step). Fails closed when the repo cannot be resolved and returns id, canonical project, version, updatedAt, and hash.",
	}, h.setActiveContext)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_refresh_resources",
		Description: "Reconcile the complete MCP resource URI set with the currently resolved project's live documents, removing stale URIs and adding external changes.",
	}, h.refreshResourcesTool)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_archive_doc",
		Description: "Archive any document (out of bootstrap + default lists/search, but reversible) or un-archive it. Human-owned notes require confirm=true.",
	}, h.archiveDoc)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_list_archived_docs",
		Description: "List archived documents as metadata only, scoped to the current project by default and optionally filtered by project or type.",
	}, h.listArchivedDocs)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_upload_artifact",
		Description: "Upload an artifact (image or downloadable file) onto a node. Pass exactly one source: path reads a local file from the flow-mcp process (relative paths use its working directory) and is preferred for local files; base64 accepts inline encoded content. In path mode name defaults to the basename and MIME is guessed from the extension; both can be overridden. In base64 mode name and mime are required. Scoped to the current project by default; pass node to target another. Images render inline via ![[slug]] in Kompendium docs; other MIME types are download links.",
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
		return structuredErrorResult("invalid_request", g.Error(), http.StatusBadRequest, false, "")
	}
	if errors.Is(err, errLoginRequired) {
		return h.loginRequired()
	}
	var apiErr *apiclient.APIError
	if errors.As(err, &apiErr) {
		code := "flow_server_error"
		retryable := apiErr.StatusCode == http.StatusTooManyRequests || apiErr.StatusCode >= 500
		switch apiErr.StatusCode {
		case http.StatusConflict:
			code, retryable = "document_conflict", true
		case http.StatusNotFound:
			code = "not_found"
		case http.StatusUnauthorized:
			code = "authentication_required"
		}
		message := apiErr.Message
		conflictVersion := ""
		var problem struct {
			Code            string `json:"code"`
			Message         string `json:"message"`
			Retryable       *bool  `json:"retryable"`
			ConflictVersion string `json:"conflictVersion"`
		}
		if json.Unmarshal([]byte(apiErr.Message), &problem) == nil {
			if problem.Code != "" {
				code = problem.Code
			}
			if problem.Message != "" {
				message = problem.Message
			}
			if problem.Retryable != nil {
				retryable = *problem.Retryable
			}
			conflictVersion = problem.ConflictVersion
		}
		if message == "" {
			message = http.StatusText(apiErr.StatusCode)
		}
		return structuredErrorResult(code, message, apiErr.StatusCode, retryable, conflictVersion)
	}
	return structuredErrorResult("flow_server_error", "flow server error: "+err.Error(), http.StatusInternalServerError, true, "")
}

// refreshResolved re-resolves the cwd→project and fully reconciles the project's
// resources, overwriting the resolved state under projMu. It runs after every
// authenticated client build and after flow_bind_project.
func (h *handlers) refreshResolved(ctx context.Context, c *apiclient.Client) {
	h.resourceRefreshMu.Lock()
	defer h.resourceRefreshMu.Unlock()
	proj, matched := resolveProject(ctx, c, mcpLog())
	h.projMu.Lock()
	h.proj, h.matched = proj, matched
	// Drop the node-ref cache with it. refreshResolved runs after EVERY
	// authenticated client build, and a rebuild can carry a different identity —
	// a surviving cache would let one owner resolve or even see another owner's
	// slugs (lookupNode lists them in its miss message). The cost is one extra
	// ListNodes on the next lookup; the alternative is a cross-tenant leak.
	h.projects, h.projFetched = nil, false
	h.projMu.Unlock()
	if _, err := h.reconcileResourcesLocked(ctx, c); err != nil {
		mcpLog().Warn("could not register document resources", "err", err)
	}
}

// postAuthInit runs after every successful authenticated-client build.
func (h *handlers) postAuthInit(ctx context.Context, c *apiclient.Client) {
	h.refreshResolved(ctx, c)
}

// mcpLogger is the package-level stderr logger for the MCP server. stdout is
// owned by StdioTransport for the JSON-RPC stream.
var mcpLogger = slog.New(slog.NewTextHandler(os.Stderr, nil))

func mcpLog() *slog.Logger { return mcpLogger }

// clientName identifies the MCP surface for client-specific context selection.
// It is presentation metadata only and is never forwarded as audit provenance.
// FLOW_ACTOR remains as a backwards-compatible local override.
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

// do is the shared authenticated REST-client wrapper. MCP ClientInfo is not a
// credential, so it must not affect actor provenance.
func (h *handlers) do(ctx context.Context, _ *mcp.CallToolRequest, fn func(*apiclient.Client) error) error {
	return h.mgr.Do(ctx, fn)
}

// textResult wraps a plain-text success result.
func textResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

func structuredResult(out any) *mcp.CallToolResult {
	b, _ := json.Marshal(out)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}, StructuredContent: out}
}

// errorResult wraps an actionable error result (IsError=true).
func errorResult(s string) *mcp.CallToolResult {
	return structuredErrorResult("invalid_request", s, http.StatusBadRequest, false, "")
}

type toolErrorResult struct {
	Code            string `json:"code"`
	Message         string `json:"message"`
	HTTPStatus      int    `json:"httpStatus"`
	Retryable       bool   `json:"retryable"`
	ConflictVersion string `json:"conflictVersion,omitempty"`
}

func structuredErrorResult(code, message string, status int, retryable bool, conflictVersion string) *mcp.CallToolResult {
	out := toolErrorResult{Code: code, Message: message, HTTPStatus: status, Retryable: retryable, ConflictVersion: conflictVersion}
	b, _ := json.Marshal(out)
	return &mcp.CallToolResult{
		IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}, StructuredContent: out,
	}
}

// loginRequired is the standard degraded-mode short-circuit.
func (h *handlers) loginRequired() *mcp.CallToolResult {
	return structuredErrorResult("authentication_required", "Login required: run 'flow login' in a terminal on this device.", http.StatusUnauthorized, false, "")
}
