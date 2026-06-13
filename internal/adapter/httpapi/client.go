package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

// Sentinel errors the UI layers translate into banners/hints.
var (
	ErrNotConfigured = errors.New("httpapi: FLOW_SERVER_URL nicht gesetzt")
	ErrLoggedOut     = errors.New("httpapi: nicht angemeldet — flow login")
	ErrUnavailable   = errors.New("httpapi: server nicht erreichbar")
)

// TokenRefresher matches oidcclient.StoreRefresher.
type TokenRefresher interface {
	RefreshTokens(ctx context.Context) (ports.Tokens, error)
}

// Config holds the parameters for constructing a Client.
type Config struct {
	BaseURL   string           // "" => ErrNotConfigured auf jedem Call
	Tokens    ports.TokenStore // keyringadapter; Slot "tokens:"+BaseURL
	Slot      string
	Refresher TokenRefresher // optional (flow-mcp: nil)
	Version   string         // ldflags-Client-Version, "dev" wenn leer
	Device    string         // Hostname für X-Flow-Device
	HTTPC     *http.Client   // optional, default 15s Timeout
}

// Client is the bearer-authenticated HTTP client for the flow server API.
type Client struct {
	base      string
	tokens    ports.TokenStore
	slot      string
	refresher TokenRefresher
	version   string
	device    string
	httpc     *http.Client
	status    *Status // Task 2
}

// New constructs a Client from c. Nil HTTPC defaults to a 15 s timeout client;
// empty Version defaults to "dev"; empty Device defaults to os.Hostname().
func New(c Config) *Client {
	httpc := c.HTTPC
	if httpc == nil {
		httpc = &http.Client{Timeout: 15 * time.Second}
	}
	version := c.Version
	if version == "" {
		version = "dev"
	}
	device := c.Device
	if device == "" {
		device, _ = os.Hostname() //nolint:errcheck // hostname failure is unactionable; X-Flow-Device is best-effort
	}
	return &Client{
		base: c.BaseURL, tokens: c.Tokens, slot: c.Slot,
		refresher: c.Refresher, version: version, device: device,
		httpc: httpc, status: newStatus(),
	}
}

func (c *Client) bearer() (string, error) {
	t, err := c.tokens.Get(c.slot)
	if errors.Is(err, ports.ErrTokenNotFound) {
		return "", ErrLoggedOut
	}
	if err != nil {
		return "", err
	}
	return t.AccessToken, nil
}

// doJSON executes a request with Bearer auth and mandatory headers, performs
// one transparent token-refresh retry on 401, and maps the response status to
// port-level errors. out == nil discards the body; ifMatch >= 0 adds If-Match.
func (c *Client) doJSON(ctx context.Context, method, path string, body any, ifMatch int64, out any) error {
	if c.base == "" {
		return ErrNotConfigured
	}
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}
	mk := func() (*http.Request, error) {
		var rd io.Reader
		if payload != nil {
			rd = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
		if err != nil {
			return nil, err
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if ifMatch >= 0 {
			req.Header.Set("If-Match", fmt.Sprintf("%d", ifMatch))
		}
		req.Header.Set("X-Flow-Client-Version", c.version)
		req.Header.Set("X-Flow-Device", c.device)
		return req, nil
	}
	tok, err := c.bearer()
	if err != nil {
		c.status.setLoggedOut()
		return err
	}
	req, err := mk()
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := c.httpc.Do(req)
	if err != nil {
		c.status.setOffline()
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	resp, err = c.maybeRefresh(ctx, mk, resp)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) //nolint:errcheck // partial read is still useful for status mapping
	return c.processResponse(resp.StatusCode, raw, out)
}

// maybeRefresh performs a transparent token refresh when the initial response
// is 401 and a Refresher is configured, then retries the request once.
// On success it returns the refreshed response (original body already closed).
func (c *Client) maybeRefresh(ctx context.Context, mk func() (*http.Request, error), resp *http.Response) (*http.Response, error) {
	if resp.StatusCode != http.StatusUnauthorized || c.refresher == nil {
		return resp, nil
	}
	_ = resp.Body.Close()
	fresh, rerr := c.refresher.RefreshTokens(ctx)
	if rerr != nil {
		c.status.setLoggedOut()
		return nil, ErrLoggedOut
	}
	req, err := mk()
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+fresh.AccessToken)
	resp2, err := c.httpc.Do(req)
	if err != nil {
		c.status.setOffline()
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return resp2, nil
}

// processResponse maps an HTTP status code to a port-level error or
// unmarshals the body into out. Called by doJSON after all retries.
func (c *Client) processResponse(code int, raw []byte, out any) error {
	switch {
	case code == http.StatusUnauthorized:
		c.status.setLoggedOut()
		return ErrLoggedOut
	case code >= 500:
		c.status.setOffline()
		return fmt.Errorf("%w: server %d", ErrUnavailable, code)
	}
	c.status.setOnline(c.base)
	if code >= 400 {
		return &StatusError{Code: code, Body: raw}
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// StatusError transportiert 4xx-Antworten zu den Resource-Adaptern, die sie
// in ports-Sentinels übersetzen (404→NotFound, 409/412→Conflict, 403→Fehlertext).
type StatusError struct {
	Code int
	Body []byte
}

func (e *StatusError) Error() string { return fmt.Sprintf("httpapi: status %d: %s", e.Code, e.Body) }

func statusCode(err error) int {
	var se *StatusError
	if errors.As(err, &se) {
		return se.Code
	}
	return 0
}
