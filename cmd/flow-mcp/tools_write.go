package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

// errGuard marks a guard/validation error whose message is meant for the user
// verbatim (not an auth or transport failure).
type errGuard struct{ err error }

func (e errGuard) Error() string { return e.err.Error() }

type createDocIn struct {
	Path    string `json:"path" jsonschema:"the document path (hierarchical slug, e.g. notes/architecture)"`
	Title   string `json:"title" jsonschema:"the document title"`
	Body    string `json:"body" jsonschema:"the markdown body; tags are set via YAML frontmatter in the body"`
	Type    string `json:"type" jsonschema:"the document type: daily, project, free, agent, memory, instruction, skill, or plan"`
	Project string `json:"project,omitempty" jsonschema:"project slug, name, or id to create in; 'global'/'none' for an unassigned document; omit to use the current directory's project"`
}

func (h *handlers) createDoc(ctx context.Context, _ *mcp.CallToolRequest, in createDocIn) (*mcp.CallToolResult, any, error) {
	typ, err := requireType(in.Type)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	if strings.TrimSpace(in.Path) == "" || strings.TrimSpace(in.Title) == "" {
		return errorResult("path and title are required"), nil, nil
	}
	var out string
	err = h.mgr.Do(ctx, func(c *apiclient.Client) error {
		sc, err := h.resolveScope(ctx, in.Project)
		if err != nil {
			return err
		}
		pid := sc.projectID
		if pid != nil && *pid == "none" { // "none"/"global" both mean unassigned for create
			pid = nil
		}
		d, err := c.CreateDocument(ctx, apiclient.CreateDocumentInput{
			Type: string(typ), ProjectID: pid, Path: in.Path, Title: in.Title, Body: in.Body,
		})
		if err != nil {
			return err
		}
		h.addResource(ctx, d)
		out = fmt.Sprintf("Created %s [%s] %s · %s.", d.Type, d.ID, d.Title, d.Path)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}

type updateDocIn struct {
	ID      string  `json:"id" jsonschema:"the document id to update"`
	Title   *string `json:"title,omitempty" jsonschema:"new title; omit to keep the current title"`
	Body    *string `json:"body,omitempty" jsonschema:"new markdown body; omit to keep the current body"`
	Confirm bool    `json:"confirm,omitempty" jsonschema:"required (true) to modify a human-owned note (daily/project/free)"`
}

func (h *handlers) updateDoc(ctx context.Context, _ *mcp.CallToolRequest, in updateDocIn) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.ID) == "" {
		return errorResult("id is required"), nil, nil
	}
	var out string
	err := h.mgr.Do(ctx, func(c *apiclient.Client) error {
		cur, err := c.GetDocument(ctx, in.ID)
		if err != nil {
			return err
		}
		if err := guardMutation(cur, in.Confirm); err != nil {
			return errGuard{err}
		}
		payload, err := mergeUpdate(cur, in.Title, in.Body)
		if err != nil {
			return errGuard{err}
		}
		d, err := c.UpdateDocument(ctx, in.ID, payload)
		if err != nil {
			return err
		}
		h.removeResource(d.ID)
		h.addResource(ctx, d)
		out = fmt.Sprintf("Updated %s [%s] %s · %s.", d.Type, d.ID, d.Title, d.Path)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}

type deleteDocIn struct {
	ID      string `json:"id" jsonschema:"the document id to delete"`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"required (true) to delete a human-owned note (daily/project/free)"`
}

func (h *handlers) deleteDoc(ctx context.Context, _ *mcp.CallToolRequest, in deleteDocIn) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.ID) == "" {
		return errorResult("id is required"), nil, nil
	}
	var out string
	err := h.mgr.Do(ctx, func(c *apiclient.Client) error {
		cur, err := c.GetDocument(ctx, in.ID)
		if err != nil {
			return err
		}
		if err := guardMutation(cur, in.Confirm); err != nil {
			return errGuard{err}
		}
		if err := c.DeleteDocument(ctx, in.ID); err != nil {
			return err
		}
		h.removeResource(cur.ID)
		out = fmt.Sprintf("Deleted [%s] %s.", cur.ID, cur.Title)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}
