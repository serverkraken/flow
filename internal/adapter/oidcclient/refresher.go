package oidcclient

import (
	"context"
	"net/http"
	"sync"

	"github.com/serverkraken/flow/internal/ports"
)

// StoreRefresher exchanges the keyring's refresh token for fresh tokens and
// persists the result. The token endpoint is resolved lazily from the
// flow-server's /api/v1/oidc/config and cached for the process lifetime.
type StoreRefresher struct {
	ServerURL  string
	ClientID   string
	Store      ports.TokenStore
	Slot       string
	HTTPClient *http.Client

	mu       sync.Mutex
	tokenURL string
}

// RefreshTokens performs one refresh-token exchange and stores the result.
// Returns ports.ErrTokenNotFound when no refresh token is available.
func (r *StoreRefresher) RefreshTokens(ctx context.Context) (ports.Tokens, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, err := r.Store.Get(r.Slot)
	if err != nil {
		return ports.Tokens{}, err
	}
	if cur.RefreshToken == "" {
		return ports.Tokens{}, ports.ErrTokenNotFound
	}
	if r.tokenURL == "" {
		_, tokenURL, rerr := ResolveEndpoints(ctx, r.ServerURL, r.HTTPClient)
		if rerr != nil {
			return ports.Tokens{}, rerr
		}
		r.tokenURL = tokenURL
	}
	fresh, err := Refresh(ctx, RefreshConfig{
		ClientID:     r.ClientID,
		TokenURL:     r.tokenURL,
		HTTPClient:   r.HTTPClient,
		RefreshToken: cur.RefreshToken,
	})
	if err != nil {
		return ports.Tokens{}, err
	}
	if err := r.Store.Put(r.Slot, fresh); err != nil {
		return ports.Tokens{}, err
	}
	return fresh, nil
}
