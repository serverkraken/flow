package apiclient

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/serverkraken/flow/internal/domain"
)

// ResolveProject calls GET /api/v1/projects/resolve and returns the matched
// project. If the server returns 404 (no binding found), ok is false and err
// is nil. Any other non-2xx status is returned as an error.
func (c *Client) ResolveProject(ctx context.Context, remoteSlug, machineID, cwd string) (domain.Project, bool, error) {
	path := "/api/v1/projects/resolve?slug=" + url.QueryEscape(remoteSlug) +
		"&machine=" + url.QueryEscape(machineID) +
		"&path=" + url.QueryEscape(cwd)
	var p domain.Project
	err := c.do(ctx, http.MethodGet, path, nil, &p)
	if err != nil {
		var ae *APIError
		if errors.As(err, &ae) && ae.StatusCode == http.StatusNotFound {
			return domain.Project{}, false, nil
		}
		return domain.Project{}, false, err
	}
	return p, true, nil
}

// BindRemote calls PUT /api/v1/projects/{projectID}/bindings with kind=remote.
func (c *Client) BindRemote(ctx context.Context, projectID, remoteSlug string) (domain.ProjectBinding, error) {
	var b domain.ProjectBinding
	err := c.do(ctx, http.MethodPut, "/api/v1/projects/"+projectID+"/bindings",
		map[string]any{"kind": "remote", "remoteSlug": remoteSlug}, &b)
	return b, err
}

// UnbindRemote calls DELETE /api/v1/projects/bindings?kind=remote&slug=<remoteSlug>.
func (c *Client) UnbindRemote(ctx context.Context, remoteSlug string) error {
	path := "/api/v1/projects/bindings?kind=remote&slug=" + url.QueryEscape(remoteSlug)
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

// BindPath calls PUT /api/v1/projects/{projectID}/bindings with kind=path.
func (c *Client) BindPath(ctx context.Context, projectID, machineID, machineLabel, path string) (domain.ProjectBinding, error) {
	var b domain.ProjectBinding
	err := c.do(ctx, http.MethodPut, "/api/v1/projects/"+projectID+"/bindings",
		map[string]any{"kind": "path", "machineId": machineID, "machineLabel": machineLabel, "path": path}, &b)
	return b, err
}

// UnbindPath calls DELETE /api/v1/projects/bindings?kind=path&machine=<machineID>&path=<path>.
func (c *Client) UnbindPath(ctx context.Context, machineID, path string) error {
	reqPath := "/api/v1/projects/bindings?kind=path&machine=" + url.QueryEscape(machineID) +
		"&path=" + url.QueryEscape(path)
	return c.do(ctx, http.MethodDelete, reqPath, nil, nil)
}

// ListBindings calls GET /api/v1/projects/bindings.
func (c *Client) ListBindings(ctx context.Context) ([]domain.ProjectBinding, error) {
	var out []domain.ProjectBinding
	err := c.do(ctx, http.MethodGet, "/api/v1/projects/bindings", nil, &out)
	return out, err
}
