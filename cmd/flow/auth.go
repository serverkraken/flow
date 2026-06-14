package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"

	"golang.org/x/oauth2"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/adapter/oidcdevice"
	"github.com/serverkraken/flow/internal/adapter/tokenstore"
	"github.com/serverkraken/flow/internal/clientconfig"
	"github.com/serverkraken/flow/internal/ports"
)

// lazyDeviceSource hands out the stored access token while it is still valid,
// and only builds the refreshing device-flow source — which performs OIDC
// discovery and therefore needs FLOW_OIDC_ISSUER — once the token has actually
// expired. This lets `flow whoami`/`worktime` run from a valid cached token
// without the issuer set and without a discovery round-trip per invocation.
type lazyDeviceSource struct {
	ctx context.Context
	cfg clientconfig.Config

	mu        sync.Mutex
	last      ports.Token
	refresher oauth2.TokenSource
}

func (s *lazyDeviceSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refresher == nil {
		cached := &oauth2.Token{
			AccessToken:  s.last.AccessToken,
			RefreshToken: s.last.RefreshToken,
			Expiry:       s.last.Expiry,
		}
		if cached.Valid() {
			return cached, nil // fast path: no issuer, no discovery round-trip
		}
		if s.cfg.OIDCIssuer == "" {
			return nil, fmt.Errorf("access token expired and FLOW_OIDC_ISSUER is not set — run `flow login` (or set FLOW_OIDC_ISSUER)")
		}
		flow, err := oidcdevice.New(s.ctx, s.cfg.OIDCIssuer, s.cfg.CliClientID)
		if err != nil {
			return nil, err
		}
		s.refresher = flow.TokenSource(s.ctx, cached)
	}
	return s.refresher.Token()
}

// persistingSource wraps a refreshing oauth2 source and writes refreshed
// tokens back to the store. It preserves the refresh token when a refresh
// response omits it (oauth2 already does this, but we guard the store too).
type persistingSource struct {
	base  oauth2.TokenSource
	store ports.TokenStore

	mu   sync.Mutex
	last ports.Token
}

func (p *persistingSource) Token() (*oauth2.Token, error) {
	tok, err := p.base.Token()
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	refresh := tok.RefreshToken
	if refresh == "" {
		refresh = p.last.RefreshToken
	}
	if tok.AccessToken != p.last.AccessToken {
		next := ports.Token{AccessToken: tok.AccessToken, RefreshToken: refresh, Expiry: tok.Expiry}
		if err := p.store.Save(next); err != nil {
			return nil, fmt.Errorf("flow: persist token: %w", err)
		}
		p.last = next
	}
	return tok, nil
}

// clientFromStore builds an authenticated apiclient. FLOW_TOKEN (if set) wins
// as a static, non-refreshing bearer (CI). Otherwise it loads the stored token
// and wraps it in a lazily-refreshing, self-persisting source: a valid cached
// token is used as-is (no issuer required), and only an expired token triggers
// discovery + refresh (which needs FLOW_OIDC_ISSUER).
func clientFromStore(ctx context.Context) (*apiclient.Client, error) {
	cfg := clientconfig.Load(os.Getenv)
	if t := os.Getenv("FLOW_TOKEN"); t != "" {
		return apiclient.New(cfg.ServerURL, t), nil
	}
	store := tokenstore.Open()
	loaded, ok, err := store.Load()
	if err != nil {
		return nil, err
	}
	if !ok || loaded.AccessToken == "" {
		return nil, fmt.Errorf("not logged in — run `flow login`")
	}
	base := &lazyDeviceSource{ctx: ctx, cfg: cfg, last: loaded}
	src := &persistingSource{base: base, store: store, last: loaded}
	rt := &oauth2.Transport{Source: src, Base: http.DefaultTransport}
	return apiclient.NewTransport(cfg.ServerURL, rt), nil
}
