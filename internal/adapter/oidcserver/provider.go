package oidcserver

import (
	"context"
	"fmt"
	"slices"

	oidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/serverkraken/flow/internal/ports"
)

// ProviderConfig captures everything Verify needs.
//
// AcceptedClientIDs is the list of OIDC client_id values flow-server treats
// as a valid `aud` claim on incoming bearer tokens. Phase 1 has two: the
// confidential browser auth-code client (FLOW_OIDC_CLIENT_ID) and the
// public CLI/MCP device-flow client (FLOW_OIDC_CLI_CLIENT_ID). A token
// issued for either is accepted; tokens for any other audience are
// rejected — the audience check is the load-bearing defence-in-depth on
// top of issuer + signature, since FLOW_ALLOWED_SUBS only filters AFTER a
// token validates.
type ProviderConfig struct {
	Issuer            string
	AcceptedClientIDs []string
}

// Provider is the concrete AuthProvider.
type Provider struct {
	verifier *oidc.IDTokenVerifier
	provider *oidc.Provider
	accepted []string
}

// NewProvider initialises the underlying oidc.Provider (which fetches the
// discovery document and JWKS endpoint) and a verifier that intentionally
// skips the built-in single-client_id audience check — Verify enforces
// the multi-audience policy explicitly so we can accept tokens from the
// browser AND CLI clients without standing up two separate verifiers.
func NewProvider(ctx context.Context, cfg ProviderConfig) (*Provider, error) {
	if len(cfg.AcceptedClientIDs) == 0 {
		return nil, fmt.Errorf("oidc provider: AcceptedClientIDs is empty")
	}
	p, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery (%s): %w", cfg.Issuer, err)
	}
	// SkipClientIDCheck=true disables the verifier's built-in `aud == ClientID`
	// assertion. We re-add a stricter version in Verify() that matches against
	// the whole accepted-list. ClientID must still be set to a non-empty
	// string for go-oidc to accept the config; the value is unused with skip
	// enabled.
	v := p.Verifier(&oidc.Config{
		ClientID:          cfg.AcceptedClientIDs[0],
		SkipClientIDCheck: true,
	})
	return &Provider{
		verifier: v,
		provider: p,
		accepted: slices.Clone(cfg.AcceptedClientIDs),
	}, nil
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

// Verify implements ports.AuthProvider. Validates signature, issuer, and
// expiry via go-oidc, then enforces audience against the configured
// accepted-list (any single overlap suffices — JWT aud is a string-or-array,
// the verifier exposes it as a string slice).
func (p *Provider) Verify(ctx context.Context, raw string) (ports.Identity, error) {
	tok, err := p.verifier.Verify(ctx, raw)
	if err != nil {
		return ports.Identity{}, fmt.Errorf("verify: %w", err)
	}
	if !audienceAccepted(tok.Audience, p.accepted) {
		return ports.Identity{}, fmt.Errorf("verify: audience %v not in accepted set %v", tok.Audience, p.accepted)
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

// audienceAccepted returns true when tokenAud contains at least one value
// present in accepted. Empty tokenAud (well-formed JWTs always have an aud
// claim, but defensive) is treated as rejected.
func audienceAccepted(tokenAud, accepted []string) bool {
	for _, a := range tokenAud {
		if slices.Contains(accepted, a) {
			return true
		}
	}
	return false
}

// Compile-time assertion.
var _ ports.AuthProvider = (*Provider)(nil)
