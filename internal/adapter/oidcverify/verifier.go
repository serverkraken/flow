// Package oidcverify verifies Authentik/Dex-issued JWT access/ID tokens against
// a set of accepted audiences (the web client + the CLI client).
package oidcverify

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/serverkraken/flow/internal/ports"
)

type Verifier struct {
	v         *oidc.IDTokenVerifier
	audiences map[string]bool
}

// New builds a verifier from the issuer's discovery document. A token is
// accepted if at least one of its audiences is in the allowed set.
func New(ctx context.Context, issuer string, audiences []string) (*Verifier, error) {
	p, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidcverify: provider: %w", err)
	}
	allowed := make(map[string]bool, len(audiences))
	for _, a := range audiences {
		if a != "" {
			allowed[a] = true
		}
	}
	// SkipClientIDCheck: go-oidc only compares a single clientID; we do the
	// (multi-audience) aud check ourselves below.
	return &Verifier{
		v:         p.Verifier(&oidc.Config{SkipClientIDCheck: true}),
		audiences: allowed,
	}, nil
}

type claims struct {
	Sub               string   `json:"sub"`
	PreferredUsername string   `json:"preferred_username"`
	Email             string   `json:"email"`
	Name              string   `json:"name"`
	Groups            []string `json:"groups"`
}

// Verify checks the token's signature (via the issuer JWKS), issuer, expiry,
// and that at least one audience is in the allowed set, then extracts the
// flow Identity.
func (vr *Verifier) Verify(ctx context.Context, raw string) (ports.Identity, error) {
	tok, err := vr.v.Verify(ctx, raw)
	if err != nil {
		return ports.Identity{}, fmt.Errorf("oidcverify: verify: %w", err)
	}
	ok := false
	for _, a := range tok.Audience {
		if vr.audiences[a] {
			ok = true
			break
		}
	}
	if !ok {
		return ports.Identity{}, fmt.Errorf("oidcverify: audience %v not allowed", tok.Audience)
	}
	var c claims
	if err := tok.Claims(&c); err != nil {
		return ports.Identity{}, fmt.Errorf("oidcverify: claims: %w", err)
	}
	return ports.Identity{Subject: c.Sub, Username: c.PreferredUsername, Email: c.Email, Name: c.Name, Groups: c.Groups}, nil
}
