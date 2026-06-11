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

type Config struct {
	BaseURL   string           // "" => ErrNotConfigured auf jedem Call
	Tokens    ports.TokenStore // keyringadapter; Slot "tokens:"+BaseURL
	Slot      string
	Refresher TokenRefresher // optional (flow-mcp: nil)
	Version   string         // ldflags-Client-Version, "dev" wenn leer
	Device    string         // Hostname für X-Flow-Device
	HTTPC     *http.Client   // optional, default 15s Timeout
}

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
		device, _ = os.Hostname()
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

// doJSON führt einen Request aus: Bearer + Pflicht-Header, EIN transparenter
// Retry nach Token-Refresh bei 401 (Muster aus httpsync), Status-Mapping.
// out == nil ⇒ Body wird verworfen. ifMatch >= 0 ⇒ If-Match-Header.
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
	if resp.StatusCode == http.StatusUnauthorized && c.refresher != nil {
		_ = resp.Body.Close()
		fresh, rerr := c.refresher.RefreshTokens(ctx)
		if rerr != nil {
			c.status.setLoggedOut()
			return ErrLoggedOut
		}
		if req, err = mk(); err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+fresh.AccessToken)
		if resp, err = c.httpc.Do(req); err != nil {
			c.status.setOffline()
			return fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		c.status.setLoggedOut()
		return ErrLoggedOut
	case resp.StatusCode >= 500:
		c.status.setOffline()
		return fmt.Errorf("%w: server %d", ErrUnavailable, resp.StatusCode)
	}
	c.status.setOnline(c.base)
	if resp.StatusCode >= 400 {
		return &StatusError{Code: resp.StatusCode, Body: raw}
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

// statusStub — replaced by status.go in Task 2.
type Status struct{}

func newStatus() *Status             { return &Status{} }
func (s *Status) setOnline(_ string) {}
func (s *Status) setOffline()        {}
func (s *Status) setLoggedOut()      {}
