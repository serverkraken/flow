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
// Issuers is the set of OIDC issuer URLs flow-server trusts. Phase 1 has two:
// the browser auth-code provider (FLOW_OIDC_ISSUER, .../o/flow/) and the public
// CLI/MCP device-flow provider (FLOW_OIDC_CLI_ISSUER, .../o/flow-cli/). Authentik
// runs in per_provider issuer mode, so each Application mints tokens with a
// distinct `iss` AND signs them against its own JWKS — a single verifier bound
// to one issuer rejects the other before the audience check ever runs.
// NewProvider therefore stands up one verifier per issuer; Verify tries each in
// turn so a token from either provider validates against its own discovery doc.
//
// AcceptedClientIDs is the list of OIDC client_id values flow-server treats as a
// valid `aud` claim. A token must clear BOTH gates: its `iss` matches one of
// Issuers (signature included) AND its `aud` overlaps AcceptedClientIDs. The
// FLOW_ALLOWED_SUBS allowlist only filters AFTER a token validates, so
// issuer+audience are the load-bearing defence-in-depth.
type ProviderConfig struct {
	Issuers           []string
	AcceptedClientIDs []string
}

// Provider is the concrete AuthProvider. It holds one oidc.Provider + verifier
// per trusted issuer (deduplicated). providers[0] backs Endpoint() and
// DeviceAuthorizationURL(): Authentik shares the device/token endpoints across
// Applications, so the first issuer's discovery doc is representative and this
// matches the pre-split single-provider behaviour.
type Provider struct {
	verifiers []*oidc.IDTokenVerifier
	providers []*oidc.Provider
	accepted  []string
}

// NewProvider runs OIDC discovery for each (deduplicated) issuer — fetching its
// discovery document and JWKS — and builds one verifier per issuer. Each
// verifier intentionally skips the built-in single-client_id audience check;
// Verify re-applies a stricter multi-audience check so tokens from the browser
// AND CLI clients are both acceptable. Discovery failure on ANY issuer fails
// boot loudly rather than silently dropping a trusted provider.
func NewProvider(ctx context.Context, cfg ProviderConfig) (*Provider, error) {
	if len(cfg.AcceptedClientIDs) == 0 {
		return nil, fmt.Errorf("oidc provider: AcceptedClientIDs is empty")
	}
	issuers := dedupeIssuers(cfg.Issuers)
	if len(issuers) == 0 {
		return nil, fmt.Errorf("oidc provider: Issuers is empty")
	}

	providers := make([]*oidc.Provider, 0, len(issuers))
	verifiers := make([]*oidc.IDTokenVerifier, 0, len(issuers))
	for _, iss := range issuers {
		p, err := oidc.NewProvider(ctx, iss)
		if err != nil {
			return nil, fmt.Errorf("oidc discovery (%s): %w", iss, err)
		}
		// SkipClientIDCheck=true disables the verifier's built-in `aud ==
		// ClientID` assertion. We re-add a stricter version in Verify() that
		// matches against the whole accepted-list. ClientID must still be a
		// non-empty string for go-oidc to accept the config; the value is
		// unused with skip enabled.
		v := p.Verifier(&oidc.Config{
			ClientID:          cfg.AcceptedClientIDs[0],
			SkipClientIDCheck: true,
		})
		providers = append(providers, p)
		verifiers = append(verifiers, v)
	}
	return &Provider{
		verifiers: verifiers,
		providers: providers,
		accepted:  slices.Clone(cfg.AcceptedClientIDs),
	}, nil
}

// Endpoint exposes the OAuth2 endpoints discovered from the first issuer.
func (p *Provider) Endpoint() (authURL, tokenURL string) {
	e := p.providers[0].Endpoint()
	return e.AuthURL, e.TokenURL
}

// DeviceAuthorizationURL returns the discovered device-authorization endpoint
// (RFC 8628 extension to OIDC discovery) from the first issuer. May be empty if
// the IdP doesn't advertise it; clients should treat that as "device-flow not
// supported".
func (p *Provider) DeviceAuthorizationURL() string {
	var claims struct {
		DeviceAuth string `json:"device_authorization_endpoint"`
	}
	_ = p.providers[0].Claims(&claims)
	return claims.DeviceAuth
}

// Verify implements ports.AuthProvider. Tries each issuer's verifier in turn —
// each validates signature, issuer, and expiry against its own discovery doc —
// and the first to accept the token wins. It then enforces audience against the
// configured accepted-list (any single overlap suffices — JWT aud is a
// string-or-array, the verifier exposes it as a string slice). When every
// issuer rejects the token the last verifier error is returned.
func (p *Provider) Verify(ctx context.Context, raw string) (ports.Identity, error) {
	var tok *oidc.IDToken
	var lastErr error
	for _, v := range p.verifiers {
		t, err := v.Verify(ctx, raw)
		if err != nil {
			lastErr = err
			continue
		}
		tok = t
		break
	}
	if tok == nil {
		return ports.Identity{}, fmt.Errorf("verify: %w", lastErr)
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

// dedupeIssuers drops empty strings and duplicates while preserving order. The
// local dex stack points both the browser and CLI client at one issuer, so
// [issuer, issuer] collapses to a single verifier; Authentik's per_provider
// mode yields two distinct issuers that both survive.
func dedupeIssuers(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
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
