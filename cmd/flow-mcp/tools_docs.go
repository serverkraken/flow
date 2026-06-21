package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/domain"
)

const defaultSearchLimit = 20

type searchDocsIn struct {
	Query   string   `json:"query" jsonschema:"the search query (hybrid keyword + semantic)"`
	Project string   `json:"project,omitempty" jsonschema:"project slug, name, or id to scope to; 'global' for all projects, 'none' for unassigned; omit to use the current directory's project"`
	Tags    []string `json:"tags,omitempty" jsonschema:"only documents carrying ALL of these tags"`
	Type    string   `json:"type,omitempty" jsonschema:"only this document type: daily, project, free, agent, memory, instruction, skill, or plan"`
	Limit   int      `json:"limit,omitempty" jsonschema:"maximum number of results (default 20)"`
}

func (h *handlers) searchDocs(ctx context.Context, _ *mcp.CallToolRequest, in searchDocsIn) (*mcp.CallToolResult, any, error) {
	if !h.authed {
		return h.loginRequired(), nil, nil
	}
	if strings.TrimSpace(in.Query) == "" {
		return errorResult("query is required"), nil, nil
	}
	typ, err := checkType(in.Type)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	sc, err := h.resolveScope(ctx, in.Project)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	hits, err := h.client.SearchScoped(ctx, in.Query, sc.projectID, in.Tags...)
	if err != nil {
		return errorResult(fmt.Sprintf("flow server error: %v", err)), nil, nil
	}
	if typ != "" {
		hits = filterHitsByType(hits, typ)
	}
	limit := in.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return textResult(formatSearchHits(hits, in.Query, sc)), nil, nil
}

type listDocsIn struct {
	Project string   `json:"project,omitempty" jsonschema:"project slug, name, or id to scope to; 'global' for all projects, 'none' for unassigned; omit to use the current directory's project"`
	Tags    []string `json:"tags,omitempty" jsonschema:"only documents carrying ALL of these tags"`
	Type    string   `json:"type,omitempty" jsonschema:"only this document type: daily, project, free, agent, memory, instruction, skill, or plan"`
}

func (h *handlers) listDocs(ctx context.Context, _ *mcp.CallToolRequest, in listDocsIn) (*mcp.CallToolResult, any, error) {
	if !h.authed {
		return h.loginRequired(), nil, nil
	}
	typ, err := checkType(in.Type)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	sc, err := h.resolveScope(ctx, in.Project)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	docs, err := h.client.ListDocumentsScoped(ctx, sc.projectID, in.Tags...)
	if err != nil {
		return errorResult(fmt.Sprintf("flow server error: %v", err)), nil, nil
	}
	if typ != "" {
		docs = filterDocsByType(docs, typ)
	}
	return textResult(formatDocList(docs, sc)), nil, nil
}

type getDocIn struct {
	ID   string `json:"id,omitempty" jsonschema:"the document id (pass exactly one of id or path)"`
	Path string `json:"path,omitempty" jsonschema:"the document path within the current project (pass exactly one of id or path)"`
}

func (h *handlers) getDoc(ctx context.Context, _ *mcp.CallToolRequest, in getDocIn) (*mcp.CallToolResult, any, error) {
	if !h.authed {
		return h.loginRequired(), nil, nil
	}
	sc, err := h.resolveScope(ctx, "") // path lookups use the cwd-resolved default scope
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	id, err := h.resolveDocRef(ctx, in.ID, in.Path, sc)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	d, err := h.client.GetDocument(ctx, id)
	if err != nil {
		return errorResult(fmt.Sprintf("flow server error: %v", err)), nil, nil
	}
	return textResult(formatDoc(d, h.projectName(ctx, d.ProjectID))), nil, nil
}

type listTagsIn struct {
	Project string `json:"project,omitempty" jsonschema:"project slug, name, or id to scope to; 'global' for all projects, 'none' for unassigned; omit to use the current directory's project"`
}

func (h *handlers) listTags(ctx context.Context, _ *mcp.CallToolRequest, in listTagsIn) (*mcp.CallToolResult, any, error) {
	if !h.authed {
		return h.loginRequired(), nil, nil
	}
	sc, err := h.resolveScope(ctx, in.Project)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	var tags []domain.TagCount
	if sc.projectID == nil { // global → the efficient owner-wide tag-count endpoint
		tags, err = h.client.Tags(ctx)
	} else { // scoped (a project, or "none") → aggregate over the scoped documents
		var docs []domain.Document
		docs, err = h.client.ListDocumentsScoped(ctx, sc.projectID)
		if err == nil {
			tags = domain.CollectTags(docs)
		}
	}
	if err != nil {
		return errorResult(fmt.Sprintf("flow server error: %v", err)), nil, nil
	}
	return textResult(formatTags(tags, sc)), nil, nil
}

type backlinksIn struct {
	ID   string `json:"id,omitempty" jsonschema:"the document id (pass exactly one of id or path)"`
	Path string `json:"path,omitempty" jsonschema:"the document path within the current project (pass exactly one of id or path)"`
}

func (h *handlers) backlinks(ctx context.Context, _ *mcp.CallToolRequest, in backlinksIn) (*mcp.CallToolResult, any, error) {
	if !h.authed {
		return h.loginRequired(), nil, nil
	}
	sc, err := h.resolveScope(ctx, "")
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	id, err := h.resolveDocRef(ctx, in.ID, in.Path, sc)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	refs, err := h.client.Backlinks(ctx, id)
	if err != nil {
		return errorResult(fmt.Sprintf("flow server error: %v", err)), nil, nil
	}
	ref := strings.TrimSpace(in.ID)
	if ref == "" {
		ref = strings.TrimSpace(in.Path)
	}
	return textResult(formatBacklinks(refs, ref)), nil, nil
}

// resolveDocRef turns a tool's id/path arguments into a document id. Exactly one
// of id or path must be set. A path is looked up within the given scope; a path
// matching zero or multiple documents is an actionable error (never a silent miss).
func (h *handlers) resolveDocRef(ctx context.Context, id, path string, sc scope) (string, error) {
	id, path = strings.TrimSpace(id), strings.TrimSpace(path)
	switch {
	case id != "" && path != "":
		return "", fmt.Errorf("pass either id or path, not both")
	case id != "":
		return id, nil
	case path == "":
		return "", fmt.Errorf("pass either id or path")
	}
	docs, err := h.client.ListDocumentsScoped(ctx, sc.projectID)
	if err != nil {
		return "", fmt.Errorf("flow server error: %v", err)
	}
	var matches []domain.Document
	for _, d := range docs {
		if d.Path == path {
			matches = append(matches, d)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no document at path %q %s", path, sc.label)
	case 1:
		return matches[0].ID, nil
	default:
		return "", fmt.Errorf("path %q matches %d documents %s; use id instead", path, len(matches), sc.label)
	}
}

func filterDocsByType(docs []domain.Document, t domain.DocumentType) []domain.Document {
	out := make([]domain.Document, 0, len(docs))
	for _, d := range docs {
		if d.Type == t {
			out = append(out, d)
		}
	}
	return out
}

func filterHitsByType(hits []domain.SearchHit, t domain.DocumentType) []domain.SearchHit {
	out := make([]domain.SearchHit, 0, len(hits))
	for _, h := range hits {
		if h.Type == t {
			out = append(out, h)
		}
	}
	return out
}
