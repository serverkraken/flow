// Package oidcdevice runs the OAuth2 Device Authorization Grant (RFC 8628) for
// the CLI/TUI, using go-oidc for discovery and x/oauth2 for the flow itself.
package oidcdevice

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Flow is a configured device-flow client for one issuer + public client.
type Flow struct{ cfg oauth2.Config }

// New discovers the issuer endpoints and builds the device-flow config.
// go-oidc's Endpoint() omits the device endpoint, so it is read from the raw
// discovery document and set explicitly.
func New(ctx context.Context, issuer, clientID string) (*Flow, error) {
	p, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidcdevice: provider: %w", err)
	}
	var extra struct {
		DeviceAuthURL string `json:"device_authorization_endpoint"`
	}
	if err := p.Claims(&extra); err != nil {
		return nil, fmt.Errorf("oidcdevice: discovery claims: %w", err)
	}
	if extra.DeviceAuthURL == "" {
		return nil, fmt.Errorf("oidcdevice: issuer %q advertises no device_authorization_endpoint", issuer)
	}
	return &Flow{cfg: oauth2.Config{
		ClientID: clientID, // public client: no secret
		Endpoint: oauth2.Endpoint{
			AuthURL:       p.Endpoint().AuthURL,
			TokenURL:      p.Endpoint().TokenURL,
			DeviceAuthURL: extra.DeviceAuthURL,
		},
		Scopes: []string{oidc.ScopeOpenID, "profile", "email", "offline_access"},
	}}, nil
}

// Start requests a device + user code from the issuer.
func (f *Flow) Start(ctx context.Context) (*oauth2.DeviceAuthResponse, error) {
	da, err := f.cfg.DeviceAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("oidcdevice: device auth: %w", err)
	}
	return da, nil
}

// Poll blocks until the user approves (or the code expires / is denied),
// honouring the server interval and slow_down responses internally.
func (f *Flow) Poll(ctx context.Context, da *oauth2.DeviceAuthResponse) (*oauth2.Token, error) {
	tok, err := f.cfg.DeviceAccessToken(ctx, da)
	if err != nil {
		return nil, fmt.Errorf("oidcdevice: poll: %w", err)
	}
	return tok, nil
}

// TokenSource returns a refreshing source seeded with a stored token.
func (f *Flow) TokenSource(ctx context.Context, t *oauth2.Token) oauth2.TokenSource {
	return f.cfg.TokenSource(ctx, t)
}
