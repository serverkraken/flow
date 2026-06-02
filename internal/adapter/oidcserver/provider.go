package oidcserver

import (
	"context"
	"fmt"

	oidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/serverkraken/flow/internal/ports"
)

// ProviderConfig captures everything Verify needs.
type ProviderConfig struct {
	Issuer   string
	ClientID string // expected 'aud' value
}

// Provider is the concrete AuthProvider.
type Provider struct {
	verifier *oidc.IDTokenVerifier
	provider *oidc.Provider
}

// NewProvider initialises the underlying oidc.Provider (which fetches the
// discovery document and JWKS endpoint) and a verifier scoped to clientID.
func NewProvider(ctx context.Context, cfg ProviderConfig) (*Provider, error) {
	p, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery (%s): %w", cfg.Issuer, err)
	}
	v := p.Verifier(&oidc.Config{ClientID: cfg.ClientID})
	return &Provider{verifier: v, provider: p}, nil
}

// Endpoint exposes the OAuth2 endpoints discovered from the issuer.
func (p *Provider) Endpoint() (authURL, tokenURL string) {
	e := p.provider.Endpoint()
	return e.AuthURL, e.TokenURL
}

// DeviceAuthorizationURL returns the discovered device-authorization
// endpoint (RFC 8628 extension to OIDC discovery). May be empty if the IdP
// doesn't advertise it; clients should treat that as "device-flow not
// supported".
func (p *Provider) DeviceAuthorizationURL() string {
	var claims struct {
		DeviceAuth string `json:"device_authorization_endpoint"`
	}
	_ = p.provider.Claims(&claims)
	return claims.DeviceAuth
}

// Verify implements ports.AuthProvider.
func (p *Provider) Verify(ctx context.Context, raw string) (ports.Identity, error) {
	tok, err := p.verifier.Verify(ctx, raw)
	if err != nil {
		return ports.Identity{}, fmt.Errorf("verify: %w", err)
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := tok.Claims(&claims); err != nil {
		return ports.Identity{}, fmt.Errorf("claims: %w", err)
	}
	return ports.Identity{
		Sub:           tok.Subject,
		Email:         claims.Email,
		EmailVerified: claims.EmailVerified,
		Name:          claims.Name,
		IssuedAt:      tok.IssuedAt,
		ExpiresAt:     tok.Expiry,
	}, nil
}

// Compile-time assertion.
var _ ports.AuthProvider = (*Provider)(nil)
