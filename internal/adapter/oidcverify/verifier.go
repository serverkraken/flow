// Package oidcverify verifies Authentik-issued JWT access/ID tokens.
package oidcverify

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/serverkraken/flow/internal/ports"
)

type Verifier struct{ v *oidc.IDTokenVerifier }

// New builds a verifier from the issuer's discovery document.
func New(ctx context.Context, issuer, clientID string) (*Verifier, error) {
	p, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidcverify: provider: %w", err)
	}
	return &Verifier{v: p.Verifier(&oidc.Config{ClientID: clientID})}, nil
}

type claims struct {
	Sub               string   `json:"sub"`
	PreferredUsername string   `json:"preferred_username"`
	Email             string   `json:"email"`
	Name              string   `json:"name"`
	Groups            []string `json:"groups"`
}

func (vr *Verifier) Verify(ctx context.Context, raw string) (ports.Identity, error) {
	tok, err := vr.v.Verify(ctx, raw)
	if err != nil {
		return ports.Identity{}, fmt.Errorf("oidcverify: verify: %w", err)
	}
	var c claims
	if err := tok.Claims(&c); err != nil {
		return ports.Identity{}, fmt.Errorf("oidcverify: claims: %w", err)
	}
	return ports.Identity{Subject: c.Sub, Username: c.PreferredUsername, Email: c.Email, Name: c.Name, Groups: c.Groups}, nil
}
