package oidcclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

// RefreshConfig holds the parameters for a refresh-token exchange.
type RefreshConfig struct {
	ClientID     string
	ClientSecret string
	TokenURL     string
	HTTPClient   *http.Client
	RefreshToken string
}

// Refresh exchanges a refresh token for a fresh access (+ optionally new
// refresh) token. Authentik rotates refresh tokens by default — we always
// persist whatever comes back; some IdPs don't rotate, in which case we
// keep the input refresh token.
func Refresh(ctx context.Context, c RefreshConfig) (ports.Tokens, error) {
	if c.HTTPClient == nil {
		c.HTTPClient = http.DefaultClient
	}

	body := url.Values{}
	body.Set("grant_type", "refresh_token")
	body.Set("refresh_token", c.RefreshToken)
	body.Set("client_id", c.ClientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.TokenURL,
		strings.NewReader(body.Encode()))
	if err != nil {
		return ports.Tokens{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if c.ClientSecret != "" {
		req.SetBasicAuth(c.ClientID, c.ClientSecret)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return ports.Tokens{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return ports.Tokens{}, fmt.Errorf("refresh: status %d: %s", resp.StatusCode, string(b))
	}

	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return ports.Tokens{}, err
	}
	if raw.RefreshToken == "" {
		raw.RefreshToken = c.RefreshToken // some IdPs don't rotate
	}
	return ports.Tokens{
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		IDToken:      raw.IDToken,
		Expiry:       time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second),
	}, nil
}
