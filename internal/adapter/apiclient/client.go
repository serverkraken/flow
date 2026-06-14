// Package apiclient is the client-side REST adapter used by the TUI/CLI.
package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

type Client struct {
	base  string
	token string
	hc    *http.Client
}

func New(base, token string) *Client {
	return &Client{base: base, token: token, hc: &http.Client{Timeout: 15 * time.Second}}
}

func (c *Client) Whoami(ctx context.Context) (domain.User, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/v1/me", nil)
	if err != nil {
		return domain.User{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := c.hc.Do(req)
	if err != nil {
		return domain.User{}, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return domain.User{}, fmt.Errorf("apiclient: /me status %d", res.StatusCode)
	}
	var u domain.User
	if err := json.NewDecoder(res.Body).Decode(&u); err != nil {
		return domain.User{}, fmt.Errorf("apiclient: decode: %w", err)
	}
	return u, nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode >= 300 {
		return fmt.Errorf("apiclient: %s %s: status %d", method, path, res.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(res.Body).Decode(out)
	}
	return nil
}

func (c *Client) StartSession(ctx context.Context, projectID *string, tag, note string) (domain.WorkSession, error) {
	var s domain.WorkSession
	err := c.do(ctx, http.MethodPost, "/api/v1/sessions",
		map[string]any{"projectId": projectID, "tag": tag, "note": note}, &s)
	return s, err
}

func (c *Client) StopSession(ctx context.Context, id, projectID string) (domain.WorkSession, error) {
	var s domain.WorkSession
	err := c.do(ctx, http.MethodPost, "/api/v1/sessions/"+id+"/stop",
		map[string]any{"projectId": projectID}, &s)
	return s, err
}

func (c *Client) ListSessions(ctx context.Context) ([]domain.WorkSession, error) {
	var out []domain.WorkSession
	err := c.do(ctx, http.MethodGet, "/api/v1/sessions", nil, &out)
	return out, err
}

func (c *Client) CreateProject(ctx context.Context, name string) (domain.Project, error) {
	var p domain.Project
	err := c.do(ctx, http.MethodPost, "/api/v1/projects", map[string]any{"name": name}, &p)
	return p, err
}

func (c *Client) ListProjects(ctx context.Context) ([]domain.Project, error) {
	var out []domain.Project
	err := c.do(ctx, http.MethodGet, "/api/v1/projects", nil, &out)
	return out, err
}
