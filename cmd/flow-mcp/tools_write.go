package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// errGuard marks a guard/validation error whose message is meant for the user
// verbatim (not an auth or transport failure).
type errGuard struct{ err error }

func (e errGuard) Error() string { return e.err.Error() }

type createDocIn struct {
	Path    string   `json:"path" jsonschema:"the document path (hierarchical slug, e.g. notes/architecture)"`
	Date    string   `json:"date,omitempty" jsonschema:"daily date in YYYY-MM-DD; omit to use today"`
	Title   string   `json:"title" jsonschema:"the document title"`
	Body    string   `json:"body" jsonschema:"the markdown body"`
	Type    string   `json:"type" jsonschema:"the document type: daily, project, free, memory, instruction, skill, plan, spec, or activecontext (agent: deprecated)"`
	Project string   `json:"project,omitempty" jsonschema:"project slug, name, or id to create in; 'global'/'none' for an unassigned document; omit to use the current directory's project"`
	Tags    []string `json:"tags,omitempty" jsonschema:"tags as a flat list; replaces the whole set. Body is pure content — do NOT put tags in YAML frontmatter."`
}

func (h *handlers) createDoc(ctx context.Context, req *mcp.CallToolRequest, in createDocIn) (*mcp.CallToolResult, any, error) {
	typ, err := requireType(in.Type)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	if strings.TrimSpace(in.Title) == "" || (typ != domain.DocDaily && strings.TrimSpace(in.Path) == "") {
		return errorResult("title and a path for non-daily documents are required"), nil, nil
	}
	var date *time.Time
	if strings.TrimSpace(in.Date) != "" {
		if typ != domain.DocDaily {
			return errorResult("date is only valid for daily documents"), nil, nil
		}
		parsed, parseErr := time.Parse("2006-01-02", strings.TrimSpace(in.Date))
		if parseErr != nil {
			return errorResult("date must use YYYY-MM-DD"), nil, nil
		}
		date = &parsed
	}
	var out string
	err = h.do(ctx, req, func(c *apiclient.Client) error {
		sc, err := h.resolveScope(ctx, in.Project)
		if err != nil {
			return err
		}
		pid := sc.nodeID
		if pid != nil && *pid == "none" { // "none"/"global" both mean unassigned for create
			pid = nil
		}
		d, err := c.CreateDocument(ctx, apiclient.CreateDocumentInput{
			Type: string(typ), NodeID: pid, Path: in.Path, Date: date, Title: in.Title, Body: in.Body, Tags: in.Tags,
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
	ID      string    `json:"id" jsonschema:"the document id to update"`
	Title   *string   `json:"title,omitempty" jsonschema:"new title; omit to keep the current title"`
	Body    *string   `json:"body,omitempty" jsonschema:"new markdown body; omit to keep the current body"`
	Tags    *[]string `json:"tags,omitempty" jsonschema:"replace the whole tag set; omit to leave unchanged; [] to clear"`
	Confirm bool      `json:"confirm,omitempty" jsonschema:"required (true) to modify a human-owned note (daily/project/free)"`
}

func (h *handlers) updateDoc(ctx context.Context, req *mcp.CallToolRequest, in updateDocIn) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.ID) == "" {
		return errorResult("id is required"), nil, nil
	}
	var out string
	err := h.do(ctx, req, func(c *apiclient.Client) error {
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
		payload.Tags = in.Tags
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

type moveDocIn struct {
	ID      string `json:"id" jsonschema:"the document id to reclassify"`
	Type    string `json:"type" jsonschema:"the complete destination type"`
	Project string `json:"project,omitempty" jsonschema:"destination project slug, name, or id; 'global'/'none' for unassigned; omit to use the current directory's project"`
	Path    string `json:"path,omitempty" jsonschema:"destination path; required except for daily documents, whose path is derived from date"`
	Date    string `json:"date,omitempty" jsonschema:"daily date in YYYY-MM-DD; required for type=daily and ignored for no other type"`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"required (true) to reclassify a human-owned note (daily/project/free)"`
}

func (h *handlers) moveDoc(ctx context.Context, req *mcp.CallToolRequest, in moveDocIn) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.ID) == "" {
		return errorResult("id is required"), nil, nil
	}
	typ, err := requireType(in.Type)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	if typ != domain.DocDaily && strings.TrimSpace(in.Path) == "" {
		return errorResult("path is required for non-daily documents"), nil, nil
	}
	var date *time.Time
	if typ == domain.DocDaily {
		parsed, parseErr := time.Parse("2006-01-02", strings.TrimSpace(in.Date))
		if parseErr != nil {
			return errorResult("date is required for daily documents and must use YYYY-MM-DD"), nil, nil
		}
		date = &parsed
	} else if strings.TrimSpace(in.Date) != "" {
		return errorResult("date is only valid for daily documents"), nil, nil
	}

	var out string
	err = h.do(ctx, req, func(c *apiclient.Client) error {
		cur, err := c.GetDocument(ctx, in.ID)
		if err != nil {
			return err
		}
		if err := guardMutation(cur, in.Confirm); err != nil {
			return errGuard{err}
		}
		sc, err := h.resolveScope(ctx, in.Project)
		if err != nil {
			return err
		}
		nodeID := sc.nodeID
		if nodeID != nil && *nodeID == "none" {
			nodeID = nil
		}
		d, err := c.MoveDocument(ctx, in.ID, apiclient.MoveDocumentInput{
			Type: string(typ), NodeID: nodeID, Path: in.Path, Date: date,
		})
		if err != nil {
			return err
		}
		h.removeResource(cur.ID)
		h.addResource(ctx, d)
		out = fmt.Sprintf("Moved %s [%s] %s · %s.", d.Type, d.ID, d.Title, d.Path)
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

func (h *handlers) deleteDoc(ctx context.Context, req *mcp.CallToolRequest, in deleteDocIn) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.ID) == "" {
		return errorResult("id is required"), nil, nil
	}
	var out string
	err := h.do(ctx, req, func(c *apiclient.Client) error {
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

type archiveDocIn struct {
	ID       string `json:"id" jsonschema:"the document id to archive or un-archive"`
	Archived *bool  `json:"archived,omitempty" jsonschema:"true (default) to archive — out of bootstrap + default lists/search but still findable; false to un-archive"`
}

func (h *handlers) archiveDoc(ctx context.Context, req *mcp.CallToolRequest, in archiveDocIn) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.ID) == "" {
		return errorResult("id is required"), nil, nil
	}
	archived := true
	if in.Archived != nil {
		archived = *in.Archived
	}
	var out string
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		cur, err := c.GetDocument(ctx, in.ID)
		if err != nil {
			return err
		}
		if err := c.SetArchived(ctx, in.ID, archived); err != nil {
			return err
		}
		if archived {
			h.removeResource(cur.ID)
			out = fmt.Sprintf("Archived [%s] %s.", cur.ID, cur.Title)
		} else {
			h.addResource(ctx, cur)
			out = fmt.Sprintf("Un-archived [%s] %s.", cur.ID, cur.Title)
		}
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}
