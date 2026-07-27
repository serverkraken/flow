// Package oidcverify verifies Authentik/Dex-issued JWT tokens against a set of
// trusted issuers, each bound to the audiences accepted for that issuer.
package oidcverify

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/serverkraken/flow/internal/ports"
)

// IssuerAudiences binds one trusted OIDC issuer to the audiences (client_ids)
// accepted for tokens minted by that issuer.
type IssuerAudiences struct {
	Issuer    string
	Audiences []string
}

// issuerVerifier is one issuer's token verifier plus its accepted-audience set.
type issuerVerifier struct {
	v    *oidc.IDTokenVerifier
	auds map[string]bool
}

// Verifier validates tokens against several issuers. Authentik runs in
// per_provider issuer mode, so the browser provider and the CLI/device provider
// mint tokens with distinct `iss` values AND sign against distinct JWKS; a
// single-issuer verifier rejects the other before any audience check.
type Verifier struct {
	verifiers []issuerVerifier
}

// New runs OIDC discovery for each issuer (fetching its discovery doc + JWKS)
// and builds one verifier per issuer. Each verifier skips go-oidc's built-in
// single-client_id audience check; Verify re-applies a stricter PER-ISSUER
// audience check. Discovery failure on any issuer fails loudly.
func New(ctx context.Context, pairs []IssuerAudiences) (*Verifier, error) {
	if len(pairs) == 0 {
		return nil, fmt.Errorf("oidcverify: no issuer/audience pairs")
	}
	vs := make([]issuerVerifier, 0, len(pairs))
	for _, p := range pairs {
		if p.Issuer == "" {
			return nil, fmt.Errorf("oidcverify: empty issuer")
		}
		if len(p.Audiences) == 0 {
			return nil, fmt.Errorf("oidcverify: no audiences for issuer %s", p.Issuer)
		}
		prov, err := oidc.NewProvider(ctx, p.Issuer)
		if err != nil {
			return nil, fmt.Errorf("oidcverify: provider(%s): %w", p.Issuer, err)
		}
		auds := make(map[string]bool, len(p.Audiences))
		for _, a := range p.Audiences {
			if a != "" {
				auds[a] = true
			}
		}
		vs = append(vs, issuerVerifier{
			v:    prov.Verifier(&oidc.Config{SkipClientIDCheck: true}),
			auds: auds,
		})
	}
	return &Verifier{verifiers: vs}, nil
}

type claims struct {
	Sub               string   `json:"sub"`
	PreferredUsername string   `json:"preferred_username"`
	Email             string   `json:"email"`
	Name              string   `json:"name"`
	Groups            []string `json:"groups"`
}

// Verify tries each issuer's verifier; the first whose signature, `iss`, and
// `exp` check out OWNS the token, and that issuer's audience set must then
// contain at least one of the token's audiences (per-issuer tightness). If no
// issuer accepts the token, the last verifier error is returned.
func (vr *Verifier) Verify(ctx context.Context, raw string) (ports.Identity, error) {
	var lastErr error
	for _, iv := range vr.verifiers {
		tok, err := iv.v.Verify(ctx, raw)
		if err != nil {
			lastErr = err
			continue
		}
		ok := false
		for _, a := range tok.Audience {
			if iv.auds[a] {
				ok = true
				break
			}
		}
		if !ok {
			return ports.Identity{}, fmt.Errorf("oidcverify: audience %v not allowed for issuer %s", tok.Audience, tok.Issuer)
		}
		var c claims
		if err := tok.Claims(&c); err != nil {
			return ports.Identity{}, fmt.Errorf("oidcverify: claims: %w", err)
		}
		return ports.Identity{Subject: c.Sub, Username: c.PreferredUsername, Email: c.Email, Name: c.Name, Groups: c.Groups}, nil
	}
	return ports.Identity{}, fmt.Errorf("oidcverify: no trusted issuer accepted token: %w", lastErr)
}
