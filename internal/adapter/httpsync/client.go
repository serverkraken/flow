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

// PermanentError marks a server response as non-retryable: the request itself
// is malformed (400), forbidden (403), or rejected by validation (422). The
// worker's drain callback uses errors.As(err, *PermanentError{}) to decide
// whether to Ack the queue entry (drop it) instead of scheduling a retry.
// Plan F · Task 8 keeps the dead-letter mechanic out of scope: a permanent
// failure is logged and the entry is removed.
type PermanentError struct {
	Status int
	Body   string
}

func (e *PermanentError) Error() string {
	return fmt.Sprintf("server %d (permanent): %s", e.Status, e.Body)
}

// isRetryableStatus reports whether HTTP status `s` is transient (5xx, 408,
// 429) and should be retried via Backoff. All other 4xx (except the typed
// paths 401/404/409 which are handled separately by the client) are
// permanent and surface as *PermanentError.
func isRetryableStatus(s int) bool {
	if s >= 500 {
		return true
	}
	if s == http.StatusRequestTimeout || s == http.StatusTooManyRequests {
		return true
	}
	return false
}

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
// the caller should handle the status itself. 5xx and 408/429 stay as a
// plain fmt.Errorf so the worker treats them as transient (no
// *PermanentError wrap).
func handleCommonErrors(status int, body []byte) (bool, error) {
	switch {
	case status == http.StatusUnauthorized:
		return true, ErrUnauthorized
	case status >= 500:
		return true, fmt.Errorf("server %d: %s", status, string(body))
	}
	return false, nil
}

// unexpectedStatusError wraps a non-2xx status that fell through all the
// typed handlers (401/404/409 + 5xx). Retryable statuses (408/429) return a
// plain error; permanent statuses return *PermanentError.
func unexpectedStatusError(status int, body []byte) error {
	if isRetryableStatus(status) {
		return fmt.Errorf("server %d: %s", status, string(body))
	}
	return &PermanentError{Status: status, Body: string(body)}
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
		return 0, unexpectedStatusError(resp.StatusCode, body)
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
		return 0, unexpectedStatusError(resp.StatusCode, body)
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

// PullRepos fetches repos with version > since, up to limit rows.
func (c *Client) PullRepos(ctx context.Context, since int64, limit int) ([]domain.Repo, int64, bool, error) {
	url := fmt.Sprintf("%s/api/v1/repos?since=%d&limit=%d", c.base, since, limit)
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
	var out pullResponse[domain.Repo]
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, 0, false, err
	}
	return out.Items, out.HighWatermark, out.HasMore, nil
}

// PushRepo sends a single repo to the server (PUT /api/v1/repos/{id}).
// Returns *ConflictError wrapping ports.ErrRepoVersionConflict on 409.
func (c *Client) PushRepo(ctx context.Context, r domain.Repo, expectedVersion int64) (int64, error) {
	buf, err := json.Marshal(r)
	if err != nil {
		return 0, err
	}
	url := fmt.Sprintf("%s/api/v1/repos/%s", c.base, r.ID)
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
		return 0, &ConflictError{Sentinel: ports.ErrRepoVersionConflict, Current: raw.Current}
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return 0, unexpectedStatusError(resp.StatusCode, body)
	}
	var out struct {
		Version int64 `json:"version"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return 0, err
	}
	return out.Version, nil
}

// PullRepoNotes fetches repo-notes with version > since, up to limit rows.
func (c *Client) PullRepoNotes(ctx context.Context, since int64, limit int) ([]domain.RepoNote, int64, bool, error) {
	url := fmt.Sprintf("%s/api/v1/repo-notes?since=%d&limit=%d", c.base, since, limit)
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
	var out pullResponse[domain.RepoNote]
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, 0, false, err
	}
	return out.Items, out.HighWatermark, out.HasMore, nil
}

// PushRepoNote sends a single repo-note to the server
// (PUT /api/v1/repos/{repo_id}/note).
// Returns *ConflictError wrapping ports.ErrRepoNoteVersionConflict on 409.
func (c *Client) PushRepoNote(ctx context.Context, n domain.RepoNote, expectedVersion int64) (int64, error) {
	buf, err := json.Marshal(n)
	if err != nil {
		return 0, err
	}
	url := fmt.Sprintf("%s/api/v1/repos/%s/note", c.base, n.RepoID)
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
		return 0, &ConflictError{Sentinel: ports.ErrRepoNoteVersionConflict, Current: raw.Current}
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return 0, unexpectedStatusError(resp.StatusCode, body)
	}
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
// tag and note are persisted on the server's active row so Stop can carry
// them over to the finished Session even from a different device.
// Returns *ConflictError (wrapping ports.ErrActiveSessionConflict) on 409.
func (c *Client) StartActive(ctx context.Context, projectID, device string, expectedVersion int64, tag, note string) (domain.ActiveSession, error) {
	body, err := json.Marshal(struct {
		StartedOnDevice string `json:"started_on_device"`
		Tag             string `json:"tag"`
		Note            string `json:"note"`
	}{device, tag, note})
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
		return domain.ActiveSession{}, unexpectedStatusError(resp.StatusCode, respBody)
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
		return domain.Session{}, unexpectedStatusError(resp.StatusCode, respBody)
	}
	var out domain.Session
	if err := json.Unmarshal(respBody, &out); err != nil {
		return domain.Session{}, err
	}
	return out, nil
}
