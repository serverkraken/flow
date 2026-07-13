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
func (h *handlers) projectContext(ctx context.Context, req *mcp.CallToolRequest, _ projectContextIn) (*mcp.CallToolResult, any, error) {
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
	err := h.do(ctx, req, func(c *apiclient.Client) error {
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
func (h *handlers) listProjectsTool(ctx context.Context, req *mcp.CallToolRequest, _ listProjectsIn) (*mcp.CallToolResult, any, error) {
	var ps []domain.Node
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		// Deliberately fetches fresh (not via the nodeList cache) so a just-created project is always visible before binding.
		got, e := c.ListNodes(ctx)
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

// bindNodeIn binds the current working directory to a project.
type bindNodeIn struct {
	Project      string `json:"project,omitempty" jsonschema:"an existing project to bind: id, slug, or name"`
	CreateName   string `json:"create_name,omitempty" jsonschema:"create a new repo with this name, then bind to it (requires create_parent)"`
	CreateParent string `json:"create_parent,omitempty" jsonschema:"the engagement or vorhaben (id, slug, or name) to nest the new repo under; required with create_name"`
	Kind         string `json:"kind,omitempty" jsonschema:"binding kind override: 'remote' (git origin) or 'path' (this directory); omit to auto-detect"`
}

// bindProject binds this directory to a project (remote-slug if a git origin is
// present, else a per-device path binding), creating the project first when
// create_name is given, then re-resolves so subsequent tools are scoped here.
func (h *handlers) bindProject(ctx context.Context, req *mcp.CallToolRequest, in bindNodeIn) (*mcp.CallToolResult, any, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return errorResult("cannot determine the working directory: " + err.Error()), nil, nil
	}
	originSlug, originOK, _ := gitremote.OriginSlug(cwd)
	machine, _ := clientmachine.Load() // best-effort; the path branch validates machine.ID
	var bound domain.Node
	var kind string
	derr := h.do(ctx, req, func(c *apiclient.Client) error {
		p, k, e := h.bindNodeCore(ctx, c, in, originSlug, originOK, machine, cwd)
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

// updateNodeIn holds node identification and partial metadata/rate fields to change.
type updateNodeIn struct {
	Node        string `json:"node,omitempty" jsonschema:"project slug, name, or id to update; omit to use the current directory's bound project"`
	Name        string `json:"name,omitempty" jsonschema:"new display name"`
	Description string `json:"description,omitempty" jsonschema:"new description (one-line subtitle)"`
	Color       string `json:"color,omitempty" jsonschema:"identity color name"`
	Glyph       string `json:"glyph,omitempty" jsonschema:"identity glyph"`
	Icon        string `json:"icon,omitempty" jsonschema:"identity icon"`
	Upstream    string `json:"upstream,omitempty" jsonschema:"git clone URL (repo only)"`
	Status      string `json:"status,omitempty" jsonschema:"active, paused or archived"`
	Slug        string `json:"slug,omitempty" jsonschema:"new slug — an identity change; rarely needed, changes how the node is addressed"`
	Rate        *int64 `json:"rate,omitempty" jsonschema:"per-hour rate in minor units (e.g. 8000 = 80.00)"`
	Currency    string `json:"currency,omitempty" jsonschema:"rate currency (default EUR)"`
	ClearRate   bool   `json:"clearRate,omitempty" jsonschema:"clear the rate instead of setting it"`
}

// updateNode applies a partial metadata update to a node (only the fields you
// pass change) and, optionally, a rate mutation. An empty string means "leave
// this field unchanged" — the MCP surface cannot clear a text field to empty
// (use the WebUI/TUI for that); rate can be cleared via clearRate.
func (h *handlers) updateNode(ctx context.Context, req *mcp.CallToolRequest, in updateNodeIn) (*mcp.CallToolResult, any, error) {
	if in.Status != "" && in.Status != "active" && in.Status != "paused" && in.Status != "archived" {
		return errorResult("status must be active, paused or archived"), nil, nil
	}
	if in.ClearRate && in.Rate != nil {
		return errorResult("rate and clearRate are mutually exclusive"), nil, nil
	}
	var out string
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		nodeID, label, err := h.artifactNode(ctx, in.Node)
		if err != nil {
			return err
		}
		var f apiclient.UpdateNodeFields
		if in.Name != "" {
			f.Name = &in.Name
		}
		if in.Slug != "" {
			f.Slug = &in.Slug
		}
		if in.Color != "" {
			f.Color = &in.Color
		}
		if in.Glyph != "" {
			f.Glyph = &in.Glyph
		}
		if in.Icon != "" {
			f.Icon = &in.Icon
		}
		if in.Description != "" {
			f.Description = &in.Description
		}
		if in.Upstream != "" {
			f.UpstreamGit = &in.Upstream
		}
		if in.Status != "" {
			f.Status = &in.Status
		}
		if f.Name != nil || f.Slug != nil || f.Color != nil || f.Glyph != nil ||
			f.Icon != nil || f.Description != nil || f.UpstreamGit != nil || f.Status != nil {
			if _, err := c.UpdateNode(ctx, nodeID, f); err != nil {
				return err
			}
		}
		switch {
		case in.ClearRate:
			if err := c.SetNodeRate(ctx, nodeID, nil, ""); err != nil {
				return err
			}
		case in.Rate != nil:
			cur := in.Currency
			if cur == "" {
				cur = "EUR"
			}
			if err := c.SetNodeRate(ctx, nodeID, in.Rate, cur); err != nil {
				return err
			}
		}
		out = fmt.Sprintf("Updated node %s.", label)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}
