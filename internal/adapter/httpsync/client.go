package httpsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ErrUnauthorized is returned when the token store has no token for the slot,
// or when the server responds with 401. The caller should trigger a re-login.
var ErrUnauthorized = errors.New("flow: unauthorized — login required")

// ConflictError is returned on 409 responses. It wraps the appropriate
// sentinel (ports.ErrSessionVersionConflict, ports.ErrProjectVersionConflict,
// or ports.ErrActiveSessionConflict) so errors.Is works against those
// sentinels, while still carrying the raw server "current" JSON for callers
// that need to inspect the conflicting row.
type ConflictError struct {
	// Sentinel is one of ports.Err*VersionConflict / ports.ErrActiveSessionConflict.
	Sentinel error
	// Current holds the raw JSON value of the "current" field from the 409 body.
	Current json.RawMessage
}

func (e *ConflictError) Error() string { return e.Sentinel.Error() }
func (e *ConflictError) Unwrap() error { return e.Sentinel }

// Client issues typed HTTP requests to flow-server's /api/v1 endpoints.
// All methods read a bearer token from tokens.Get(slot); a missing token
// returns ErrUnauthorized rather than hitting the server.
type Client struct {
	base   string
	tokens ports.TokenStore
	slot   string
	httpc  *http.Client
}

// NewClient constructs a Client. base is the server root URL (no trailing slash),
// e.g. "https://flow.example.com". slot is the TokenStore slot name.
func NewClient(base string, tokens ports.TokenStore, slot string) *Client {
	return &Client{
		base:   base,
		tokens: tokens,
		slot:   slot,
		httpc:  &http.Client{Timeout: 30 * time.Second},
	}
}

// bearer retrieves the access token for the configured slot.
// Returns ErrUnauthorized when no token is stored.
func (c *Client) bearer() (string, error) {
	t, err := c.tokens.Get(c.slot)
	if errors.Is(err, ports.ErrTokenNotFound) {
		return "", ErrUnauthorized
	}
	if err != nil {
		return "", err
	}
	return t.AccessToken, nil
}

// do executes req, sets the Authorization header, and returns the response.
func (c *Client) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	token, err := c.bearer()
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req = req.WithContext(ctx)
	return c.httpc.Do(req)
}

// readBody reads and closes the response body.
func readBody(r *http.Response) ([]byte, error) {
	defer func() { _ = r.Body.Close() }()
	return io.ReadAll(r.Body)
}

// handleCommonErrors maps 401 and 5xx into typed errors; returns false when
// the caller should handle the status itself.
func handleCommonErrors(status int, body []byte) (bool, error) {
	switch {
	case status == http.StatusUnauthorized:
		return true, ErrUnauthorized
	case status >= 500:
		return true, fmt.Errorf("server %d: %s", status, string(body))
	}
	return false, nil
}

// pullResponse is the shared shape for GET /api/v1/{sessions,projects}.
type pullResponse[T any] struct {
	Items         []T   `json:"items"`
	HighWatermark int64 `json:"high_watermark"`
	HasMore       bool  `json:"has_more"`
}

// activeListResponse is the shape for GET /api/v1/active?since=N.
type activeListResponse struct {
	Items         []domain.ActiveSession `json:"items"`
	HighWatermark int64                  `json:"high_watermark"`
}

// PullSessions fetches sessions with version > since, up to limit rows.
// Returns (items, highWatermark, hasMore, err).
func (c *Client) PullSessions(ctx context.Context, since int64, limit int) ([]domain.Session, int64, bool, error) {
	url := fmt.Sprintf("%s/api/v1/sessions?since=%d&limit=%d", c.base, since, limit)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, false, err
	}
	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, 0, false, err
	}
	body, err := readBody(resp)
	if err != nil {
		return nil, 0, false, err
	}
	if handled, herr := handleCommonErrors(resp.StatusCode, body); handled {
		return nil, 0, false, herr
	}
	if resp.StatusCode != http.StatusOK {
		return nil, 0, false, fmt.Errorf("server %d: %s", resp.StatusCode, string(body))
	}
	var out pullResponse[domain.Session]
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, 0, false, err
	}
	return out.Items, out.HighWatermark, out.HasMore, nil
}

// PushSession sends a single session to the server (PUT /api/v1/sessions/{id}).
// expectedVersion is 0 for new rows. Returns the server-assigned version on success.
// Returns *ConflictError (wrapping ports.ErrSessionVersionConflict) on 409.
func (c *Client) PushSession(ctx context.Context, s domain.Session, expectedVersion int64) (int64, error) {
	buf, err := json.Marshal(s)
	if err != nil {
		return 0, err
	}
	url := fmt.Sprintf("%s/api/v1/sessions/%s", c.base, s.ID)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(buf))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", strconv.FormatInt(expectedVersion, 10))
	resp, err := c.do(ctx, req)
	if err != nil {
		return 0, err
	}
	body, err := readBody(resp)
	if err != nil {
		return 0, err
	}
	if handled, herr := handleCommonErrors(resp.StatusCode, body); handled {
		return 0, herr
	}
	if resp.StatusCode == http.StatusConflict {
		var raw struct {
			Current json.RawMessage `json:"current"`
		}
		_ = json.Unmarshal(body, &raw)
		return 0, &ConflictError{Sentinel: ports.ErrSessionVersionConflict, Current: raw.Current}
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return 0, fmt.Errorf("server %d: %s", resp.StatusCode, string(body))
	}
	// Server returns the full domain.Session row; extract Version.
	var out domain.Session
	if err := json.Unmarshal(body, &out); err != nil {
		return 0, err
	}
	return out.Version, nil
}

// PullProjects fetches projects with version > since, up to limit rows.
func (c *Client) PullProjects(ctx context.Context, since int64, limit int) ([]domain.Project, int64, bool, error) {
	url := fmt.Sprintf("%s/api/v1/projects?since=%d&limit=%d", c.base, since, limit)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, false, err
	}
	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, 0, false, err
	}
	body, err := readBody(resp)
	if err != nil {
		return nil, 0, false, err
	}
	if handled, herr := handleCommonErrors(resp.StatusCode, body); handled {
		return nil, 0, false, herr
	}
	if resp.StatusCode != http.StatusOK {
		return nil, 0, false, fmt.Errorf("server %d: %s", resp.StatusCode, string(body))
	}
	var out pullResponse[domain.Project]
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, 0, false, err
	}
	return out.Items, out.HighWatermark, out.HasMore, nil
}

// PushProject sends a single project to the server (PUT /api/v1/projects/{id}).
// Returns the server-assigned version on success.
// Returns *ConflictError (wrapping ports.ErrProjectVersionConflict) on 409.
func (c *Client) PushProject(ctx context.Context, p domain.Project, expectedVersion int64) (int64, error) {
	buf, err := json.Marshal(p)
	if err != nil {
		return 0, err
	}
	url := fmt.Sprintf("%s/api/v1/projects/%s", c.base, p.ID)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(buf))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", strconv.FormatInt(expectedVersion, 10))
	resp, err := c.do(ctx, req)
	if err != nil {
		return 0, err
	}
	body, err := readBody(resp)
	if err != nil {
		return 0, err
	}
	if handled, herr := handleCommonErrors(resp.StatusCode, body); handled {
		return 0, herr
	}
	if resp.StatusCode == http.StatusConflict {
		var raw struct {
			Current json.RawMessage `json:"current"`
		}
		_ = json.Unmarshal(body, &raw)
		return 0, &ConflictError{Sentinel: ports.ErrProjectVersionConflict, Current: raw.Current}
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return 0, fmt.Errorf("server %d: %s", resp.StatusCode, string(body))
	}
	// Server returns {"id": "...", "version": N}.
	var out struct {
		Version int64 `json:"version"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return 0, err
	}
	return out.Version, nil
}

// PullActive fetches active sessions. When since > 0, uses the ?since=N form
// which returns {"items": [...], "high_watermark": N}. When since == 0, omits
// the query param and returns all active sessions for the user.
func (c *Client) PullActive(ctx context.Context, since int64) ([]domain.ActiveSession, int64, error) {
	var url string
	if since > 0 {
		url = fmt.Sprintf("%s/api/v1/active?since=%d", c.base, since)
	} else {
		url = fmt.Sprintf("%s/api/v1/active", c.base)
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, 0, err
	}
	body, err := readBody(resp)
	if err != nil {
		return nil, 0, err
	}
	if handled, herr := handleCommonErrors(resp.StatusCode, body); handled {
		return nil, 0, herr
	}
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("server %d: %s", resp.StatusCode, string(body))
	}
	var out activeListResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, 0, err
	}
	return out.Items, out.HighWatermark, nil
}

// StartActive starts tracking on the server (POST /api/v1/active/{project_id}/start).
// expectedVersion is 0 when no active session is expected to exist.
// Returns *ConflictError (wrapping ports.ErrActiveSessionConflict) on 409.
func (c *Client) StartActive(ctx context.Context, projectID, device string, expectedVersion int64) (domain.ActiveSession, error) {
	body, err := json.Marshal(struct {
		StartedOnDevice string `json:"started_on_device"`
	}{device})
	if err != nil {
		return domain.ActiveSession{}, err
	}
	url := fmt.Sprintf("%s/api/v1/active/%s/start", c.base, projectID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return domain.ActiveSession{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", strconv.FormatInt(expectedVersion, 10))
	resp, err := c.do(ctx, req)
	if err != nil {
		return domain.ActiveSession{}, err
	}
	respBody, err := readBody(resp)
	if err != nil {
		return domain.ActiveSession{}, err
	}
	if handled, herr := handleCommonErrors(resp.StatusCode, respBody); handled {
		return domain.ActiveSession{}, herr
	}
	if resp.StatusCode == http.StatusConflict {
		var raw struct {
			Current json.RawMessage `json:"current"`
		}
		_ = json.Unmarshal(respBody, &raw)
		return domain.ActiveSession{}, &ConflictError{Sentinel: ports.ErrActiveSessionConflict, Current: raw.Current}
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return domain.ActiveSession{}, fmt.Errorf("server %d: %s", resp.StatusCode, string(respBody))
	}
	var out domain.ActiveSession
	if err := json.Unmarshal(respBody, &out); err != nil {
		return domain.ActiveSession{}, err
	}
	return out, nil
}

// StopActive stops tracking on the server (DELETE /api/v1/active/{project_id}).
// Returns the completed domain.Session on success.
// Returns ports.ErrActiveSessionNotFound on 404.
// Returns *ConflictError (wrapping ports.ErrActiveSessionConflict) on 409.
func (c *Client) StopActive(ctx context.Context, projectID string, expectedVersion int64, tag, note string) (domain.Session, error) {
	body, err := json.Marshal(struct {
		Tag  string `json:"tag"`
		Note string `json:"note"`
	}{tag, note})
	if err != nil {
		return domain.Session{}, err
	}
	url := fmt.Sprintf("%s/api/v1/active/%s", c.base, projectID)
	req, err := http.NewRequest(http.MethodDelete, url, bytes.NewReader(body))
	if err != nil {
		return domain.Session{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", strconv.FormatInt(expectedVersion, 10))
	resp, err := c.do(ctx, req)
	if err != nil {
		return domain.Session{}, err
	}
	respBody, err := readBody(resp)
	if err != nil {
		return domain.Session{}, err
	}
	if handled, herr := handleCommonErrors(resp.StatusCode, respBody); handled {
		return domain.Session{}, herr
	}
	if resp.StatusCode == http.StatusNotFound {
		return domain.Session{}, ports.ErrActiveSessionNotFound
	}
	if resp.StatusCode == http.StatusConflict {
		var raw struct {
			Current json.RawMessage `json:"current"`
		}
		_ = json.Unmarshal(respBody, &raw)
		return domain.Session{}, &ConflictError{Sentinel: ports.ErrActiveSessionConflict, Current: raw.Current}
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return domain.Session{}, fmt.Errorf("server %d: %s", resp.StatusCode, string(respBody))
	}
	var out domain.Session
	if err := json.Unmarshal(respBody, &out); err != nil {
		return domain.Session{}, err
	}
	return out, nil
}
