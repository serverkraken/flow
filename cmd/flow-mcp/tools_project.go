package main

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/clientmachine"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/gitremote"
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
		return textResult("No flow project is bound to this directory. Bind it with flow_bind_project, or set FLOW_PROJECT."), nil, nil
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
	msg := fmt.Sprintf("Project: %s (%s) — status %s — %d document(s) in scope.", proj.Name, proj.Slug, proj.Status, count)
	if proj.UpstreamGit != "" {
		msg += " Upstream: " + proj.UpstreamGit + "."
	}
	msg += " Resolved for this working directory."
	return textResult(msg), nil, nil
}

// listProjectsIn has no parameters.
type listProjectsIn struct{}

// listProjectsTool lists all projects (id/name/slug) so the model can pick an
// existing one before binding instead of duplicate-creating.
func (h *handlers) listProjectsTool(ctx context.Context, _ *mcp.CallToolRequest, _ listProjectsIn) (*mcp.CallToolResult, any, error) {
	var ps []domain.Project
	err := h.mgr.Do(ctx, func(c *apiclient.Client) error {
		// Deliberately fetches fresh (not via the projectList cache) so a just-created project is always visible before binding.
		got, e := c.ListProjects(ctx)
		if e != nil {
			return e
		}
		ps = got
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(formatProjects(ps)), nil, nil
}

// bindProjectIn binds the current working directory to a project.
type bindProjectIn struct {
	Project    string `json:"project,omitempty" jsonschema:"an existing project to bind: id, slug, or name"`
	CreateName string `json:"create_name,omitempty" jsonschema:"create a new project with this name, then bind to it"`
	Kind       string `json:"kind,omitempty" jsonschema:"binding kind override: 'remote' (git origin) or 'path' (this directory); omit to auto-detect"`
}

// bindProject binds this directory to a project (remote-slug if a git origin is
// present, else a per-device path binding), creating the project first when
// create_name is given, then re-resolves so subsequent tools are scoped here.
func (h *handlers) bindProject(ctx context.Context, _ *mcp.CallToolRequest, in bindProjectIn) (*mcp.CallToolResult, any, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return errorResult("cannot determine the working directory: " + err.Error()), nil, nil
	}
	originSlug, originOK, _ := gitremote.OriginSlug(cwd)
	machine, _ := clientmachine.Load() // best-effort; the path branch validates machine.ID
	var bound domain.Project
	var kind string
	derr := h.mgr.Do(ctx, func(c *apiclient.Client) error {
		p, k, e := h.bindProjectCore(ctx, c, in, originSlug, originOK, machine, cwd)
		if e != nil {
			return e
		}
		bound, kind = p, k
		h.refreshResolved(ctx, c)
		return nil
	})
	if derr != nil {
		return h.resultErr(derr), nil, nil
	}
	msg := fmt.Sprintf("Bound this directory to project %s (%s) via %s binding. flow_project_context now resolves here.", bound.Name, bound.Slug, kind)
	return textResult(msg), nil, nil
}
