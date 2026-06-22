// Package apiclient is the client-side REST adapter used by the TUI/CLI.
package apiclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	return NewTransport(base, staticBearer{token: token})
}

// NewInsecure is New but skips TLS certificate verification. Use ONLY against
// the dev stack, whose flow-server presents a self-signed cert so the browser
// can negotiate HTTP/2 (FLOW_INSECURE_TLS=1). Never use against a real server.
func NewInsecure(base, token string) *Client {
	return NewTransport(base, staticBearer{token: token, base: InsecureBase()})
}

// NewTransport builds a client whose auth (and refresh) is handled by rt.
func NewTransport(base string, rt http.RoundTripper) *Client {
	return &Client{base: base, rt: rt, hc: &http.Client{Timeout: 15 * time.Second, Transport: rt}}
}

// InsecureBase returns an http.RoundTripper that does NOT verify server
// certificates — for the dev stack's self-signed flow-server only. Never use
// this in production.
func InsecureBase() http.RoundTripper {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // dev self-signed cert (FLOW_INSECURE_TLS)
	return t
}

// staticBearer injects a fixed bearer token on every request. base defaults to
// http.DefaultTransport when nil.
type staticBearer struct {
	token string
	base  http.RoundTripper
}

func (b staticBearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.Header.Set("Authorization", "Bearer "+b.token)
	base := b.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(r2)
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
		return &APIError{Method: method, Path: path, StatusCode: res.StatusCode}
	}
	if out != nil {
		return json.NewDecoder(res.Body).Decode(out)
	}
	return nil
}

// APIError is returned by do for any non-2xx response so callers can branch on
// the status (e.g. skip a 409 conflict). The message is unchanged, so existing
// `err != nil` callers are unaffected.
type APIError struct {
	Method, Path string
	StatusCode   int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("apiclient: %s %s: status %d", e.Method, e.Path, e.StatusCode)
}

// IsConflict reports whether err is an APIError with HTTP 409.
func IsConflict(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.StatusCode == http.StatusConflict
}

// IsUnauthorized reports whether err is (or wraps) an APIError with HTTP 401.
func IsUnauthorized(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.StatusCode == http.StatusUnauthorized
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

// ListSessionsSince returns sessions with start >= since.
func (c *Client) ListSessionsSince(ctx context.Context, since time.Time) ([]domain.WorkSession, error) {
	var out []domain.WorkSession
	path := "/api/v1/sessions?since=" + url.QueryEscape(since.Format(time.RFC3339))
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// AddSession backfills a complete past session with explicit start/stop.
func (c *Client) AddSession(ctx context.Context, projectID *string, start, stop time.Time, tag, note string) (domain.WorkSession, error) {
	var s domain.WorkSession
	err := c.do(ctx, http.MethodPost, "/api/v1/sessions",
		map[string]any{"projectId": projectID, "tag": tag, "note": note, "start": start, "stop": stop}, &s)
	return s, err
}

// ListSessionsRange returns sessions with since <= start < until.
func (c *Client) ListSessionsRange(ctx context.Context, since, until time.Time) ([]domain.WorkSession, error) {
	var out []domain.WorkSession
	path := "/api/v1/sessions?since=" + url.QueryEscape(since.Format(time.RFC3339)) +
		"&until=" + url.QueryEscape(until.Format(time.RFC3339))
	err := c.do(ctx, http.MethodGet, path, nil, &out)
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

func (c *Client) DeleteProject(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/projects/"+id, nil, nil)
}
