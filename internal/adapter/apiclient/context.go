package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// ContextQuery carries the resolution hints for GET /api/v1/context.
type ContextQuery struct {
	Remote, Machine, Path, Node string
	Client                      string
	Profile                     string
	Cap                         int
}

func (q ContextQuery) values() url.Values {
	v := url.Values{}
	set := func(k, s string) {
		if s != "" {
			v.Set(k, s)
		}
	}
	set("remote", q.Remote)
	set("machine", q.Machine)
	set("path", q.Path)
	set("node", q.Node)
	set("client", q.Client)
	set("profile", q.Profile)
	if q.Cap > 0 {
		v.Set("cap", strconv.Itoa(q.Cap))
	}
	return v
}

// ComposeContext calls GET /api/v1/context and returns the composed context.
func (c *Client) ComposeContext(ctx context.Context, in ContextQuery) (usecase.ComposedContext, error) {
	var out usecase.ComposedContext
	err := c.do(ctx, http.MethodGet, "/api/v1/context?"+in.values().Encode(), nil, &out)
	return out, err
}

// SetActiveContextInput is the request body for PUT /api/v1/context/active.
type SetActiveContextInput struct {
	Remote  string   `json:"remote,omitempty"`
	Machine string   `json:"machine,omitempty"`
	Path    string   `json:"path,omitempty"`
	Node    string   `json:"node,omitempty"`
	Title   string   `json:"title,omitempty"`
	Body    string   `json:"body"`
	Tags    []string `json:"tags,omitempty"`
}

// SetActiveContextResult is the response body from PUT /api/v1/context/active.
type SetActiveContextResult struct {
	ID        string `json:"id"`
	UpdatedAt string `json:"updatedAt"`
}

// SetActiveContext calls PUT /api/v1/context/active to upsert the active-context
// memory doc at the resolved leaf node.
func (c *Client) SetActiveContext(ctx context.Context, in SetActiveContextInput) (SetActiveContextResult, error) {
	var out SetActiveContextResult
	err := c.do(ctx, http.MethodPut, "/api/v1/context/active", in, &out)
	return out, err
}

// ReorderContext calls POST /api/v1/context/reorder to stamp a dense
// descending priority on the given documents in the given order.
func (c *Client) ReorderContext(ctx context.Context, orderedIDs []string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/context/reorder", map[string][]string{"ids": orderedIDs}, nil)
}

// SetPinned calls POST /api/v1/documents/{id}/pin to pin or unpin a document.
func (c *Client) SetPinned(ctx context.Context, id string, pinned bool) error {
	return c.do(ctx, http.MethodPost, "/api/v1/documents/"+id+"/pin", map[string]bool{"pinned": pinned}, nil)
}

// SetContextMode calls POST /api/v1/documents/{id}/context-mode to set a
// document's agent-context membership mode (auto/immer/nie).
func (c *Client) SetContextMode(ctx context.Context, id string, mode domain.ContextMode) error {
	return c.do(ctx, http.MethodPost, "/api/v1/documents/"+id+"/context-mode", map[string]string{"mode": string(mode)}, nil)
}

// SetArchived calls POST /api/v1/documents/{id}/archive to archive or un-archive a document.
func (c *Client) SetArchived(ctx context.Context, id string, archived bool) error {
	return c.do(ctx, http.MethodPost, "/api/v1/documents/"+id+"/archive", map[string]bool{"archived": archived}, nil)
}

// RedesignDocTypes triggers the server-side maintenance op that rewrites legacy
// `agent` docs to spec/plan with slim paths. dryRun audits without mutating.
func (c *Client) RedesignDocTypes(ctx context.Context, dryRun bool) (domain.RedesignReport, error) {
	path := "/api/v1/maintenance/redesign-doctypes"
	if dryRun {
		path += "?dry_run=true"
	}
	var out domain.RedesignReport
	err := c.do(ctx, http.MethodPost, path, nil, &out)
	return out, err
}

// UpsertByPathInput mirrors the by-path upsert payload.
type UpsertByPathInput struct {
	Type     string   `json:"type"`
	NodeID   *string  `json:"projectId,omitempty"`
	Path     string   `json:"path"`
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	Tags     []string `json:"tags,omitempty"`
	Pinned   bool     `json:"pinned"`
	Archived bool     `json:"archived"`
}

// UpsertByPathResult is the response body from PUT /api/v1/documents/by-path.
type UpsertByPathResult struct {
	ID        string    `json:"id"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// UpsertDocumentByPath inserts or updates a document at (node, path) idempotently.
func (c *Client) UpsertDocumentByPath(ctx context.Context, in UpsertByPathInput) (UpsertByPathResult, error) {
	var out UpsertByPathResult
	err := c.do(ctx, http.MethodPut, "/api/v1/documents/by-path", in, &out)
	return out, err
}
