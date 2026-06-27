package apiclient

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/serverkraken/flow/internal/domain"
)

// ResolveNode calls GET /api/v1/nodes/resolve and returns the matched
// project. If the server returns 404 (no binding found), ok is false and err
// is nil. Any other non-2xx status is returned as an error.
func (c *Client) ResolveNode(ctx context.Context, remoteSlug, machineID, cwd string) (domain.Node, bool, error) {
	path := "/api/v1/nodes/resolve?slug=" + url.QueryEscape(remoteSlug) +
		"&machine=" + url.QueryEscape(machineID) +
		"&path=" + url.QueryEscape(cwd)
	var p domain.Node
	err := c.do(ctx, http.MethodGet, path, nil, &p)
	if err != nil {
		var ae *APIError
		if errors.As(err, &ae) && ae.StatusCode == http.StatusNotFound {
			return domain.Node{}, false, nil
		}
		return domain.Node{}, false, err
	}
	return p, true, nil
}

// ResolveEngagement calls GET /api/v1/nodes/resolve-engagement and returns the
// engagement for the resolved repo. 404 → ok=false, err=nil.
func (c *Client) ResolveEngagement(ctx context.Context, remoteSlug, machineID, cwd string) (domain.Node, bool, error) {
	path := "/api/v1/nodes/resolve-engagement?slug=" + url.QueryEscape(remoteSlug) +
		"&machine=" + url.QueryEscape(machineID) +
		"&path=" + url.QueryEscape(cwd)
	var n domain.Node
	err := c.do(ctx, http.MethodGet, path, nil, &n)
	if err != nil {
		var ae *APIError
		if errors.As(err, &ae) && ae.StatusCode == http.StatusNotFound {
			return domain.Node{}, false, nil
		}
		return domain.Node{}, false, err
	}
	return n, true, nil
}

// BindRemote calls PUT /api/v1/nodes/{nodeID}/bindings with kind=remote.
func (c *Client) BindRemote(ctx context.Context, nodeID, remoteSlug string) (domain.ProjectBinding, error) {
	var b domain.ProjectBinding
	err := c.do(ctx, http.MethodPut, "/api/v1/nodes/"+nodeID+"/bindings",
		map[string]any{"kind": "remote", "remoteSlug": remoteSlug}, &b)
	return b, err
}

// UnbindRemote calls DELETE /api/v1/nodes/bindings?kind=remote&slug=<remoteSlug>.
func (c *Client) UnbindRemote(ctx context.Context, remoteSlug string) error {
	path := "/api/v1/nodes/bindings?kind=remote&slug=" + url.QueryEscape(remoteSlug)
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

// BindPath calls PUT /api/v1/nodes/{nodeID}/bindings with kind=path.
func (c *Client) BindPath(ctx context.Context, nodeID, machineID, machineLabel, path string) (domain.ProjectBinding, error) {
	var b domain.ProjectBinding
	err := c.do(ctx, http.MethodPut, "/api/v1/nodes/"+nodeID+"/bindings",
		map[string]any{"kind": "path", "machineId": machineID, "machineLabel": machineLabel, "path": path}, &b)
	return b, err
}

// UnbindPath calls DELETE /api/v1/nodes/bindings?kind=path&machine=<machineID>&path=<path>.
func (c *Client) UnbindPath(ctx context.Context, machineID, path string) error {
	reqPath := "/api/v1/nodes/bindings?kind=path&machine=" + url.QueryEscape(machineID) +
		"&path=" + url.QueryEscape(path)
	return c.do(ctx, http.MethodDelete, reqPath, nil, nil)
}

// ListBindings calls GET /api/v1/nodes/bindings.
func (c *Client) ListBindings(ctx context.Context) ([]domain.ProjectBinding, error) {
	var out []domain.ProjectBinding
	err := c.do(ctx, http.MethodGet, "/api/v1/nodes/bindings", nil, &out)
	return out, err
}
