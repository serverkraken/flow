package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"golang.org/x/oauth2"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/adapter/oidcdevice"
	"github.com/serverkraken/flow/internal/adapter/tokenstore"
	"github.com/serverkraken/flow/internal/clientconfig"
	"github.com/serverkraken/flow/internal/ports"
)

// persistingSource wraps a refreshing oauth2 source and writes refreshed
// tokens back to the store. It preserves the refresh token when a refresh
// response omits it (oauth2 already does this, but we guard the store too).
type persistingSource struct {
	base  oauth2.TokenSource
	store ports.TokenStore
	last  ports.Token
}

func (p *persistingSource) Token() (*oauth2.Token, error) {
	tok, err := p.base.Token()
	if err != nil {
		return nil, err
	}
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
// and wraps it in a refreshing, self-persisting source.
func clientFromStore(ctx context.Context) (*apiclient.Client, error) {
	cfg, err := clientconfig.Load(os.Getenv)
	if err != nil {
		return nil, err
	}
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
	flow, err := oidcdevice.New(ctx, cfg.OIDCIssuer, cfg.CliClientID)
	if err != nil {
		return nil, err
	}
	base := flow.TokenSource(ctx, &oauth2.Token{
		AccessToken:  loaded.AccessToken,
		RefreshToken: loaded.RefreshToken,
		Expiry:       loaded.Expiry,
	})
	src := &persistingSource{base: base, store: store, last: loaded}
	rt := &oauth2.Transport{Source: src, Base: http.DefaultTransport}
	return apiclient.NewTransport(cfg.ServerURL, rt), nil
}
