// Package apiclient is the client-side REST adapter used by the TUI/CLI.
package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
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
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return domain.User{}, fmt.Errorf("apiclient: /me status %d", res.StatusCode)
	}
	var u domain.User
	if err := json.NewDecoder(res.Body).Decode(&u); err != nil {
		return domain.User{}, fmt.Errorf("apiclient: decode: %w", err)
	}
	return u, nil
}
