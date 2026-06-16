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
	base string
	hc   *http.Client      // 15s timeout, for unary calls
	rt   http.RoundTripper // auth transport, reused for the no-timeout SSE client
}

// New builds a client that sends a fixed bearer token (CI / FLOW_TOKEN override).
func New(base, token string) *Client {
	return NewTransport(base, staticBearer{token})
}

// NewTransport builds a client whose auth (and refresh) is handled by rt.
func NewTransport(base string, rt http.RoundTripper) *Client {
	return &Client{base: base, rt: rt, hc: &http.Client{Timeout: 15 * time.Second, Transport: rt}}
}

// staticBearer injects a fixed bearer token on every request.
type staticBearer struct{ token string }

func (b staticBearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r2)
}

func (c *Client) Whoami(ctx context.Context) (domain.User, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/v1/me", nil)
	if err != nil {
		return domain.User{}, err
	}
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

func (c *Client) EditSession(ctx context.Context, id string, projectID *string, tag, note string, start time.Time, stop *time.Time) (domain.WorkSession, error) {
	var s domain.WorkSession
	err := c.do(ctx, http.MethodPatch, "/api/v1/sessions/"+id,
		map[string]any{"projectId": projectID, "tag": tag, "note": note, "start": start, "stop": stop}, &s)
	return s, err
}

func (c *Client) DeleteSession(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/sessions/"+id, nil, nil)
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
