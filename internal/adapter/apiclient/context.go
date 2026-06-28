package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/serverkraken/flow/internal/usecase"
)

// ContextQuery carries the resolution hints for GET /api/v1/context.
type ContextQuery struct {
	Remote, Machine, Path, Node string
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

// SetPinned calls POST /api/v1/documents/{id}/pin to pin or unpin a document.
func (c *Client) SetPinned(ctx context.Context, id string, pinned bool) error {
	return c.do(ctx, http.MethodPost, "/api/v1/documents/"+id+"/pin", map[string]bool{"pinned": pinned}, nil)
}
