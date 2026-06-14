// Package oidcauth implements the OIDC authorization-code flow for the WebUI.
package oidcauth

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/serverkraken/flow/internal/ports"
)

// Authenticator drives the auth-code flow against Authentik and verifies the
// returned id_token.
type Authenticator struct {
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
}

func New(ctx context.Context, issuer, clientID, clientSecret, redirectURL string) (*Authenticator, error) {
	p, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidcauth: provider: %w", err)
	}
	return &Authenticator{
		oauth: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     p.Endpoint(),
			RedirectURL:  redirectURL,
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		},
		verifier: p.Verifier(&oidc.Config{ClientID: clientID}),
	}, nil
}

func (a *Authenticator) AuthCodeURL(state string) string { return a.oauth.AuthCodeURL(state) }

func (a *Authenticator) Exchange(ctx context.Context, code string) (ports.Identity, error) {
	tok, err := a.oauth.Exchange(ctx, code)
	if err != nil {
		return ports.Identity{}, fmt.Errorf("oidcauth: exchange: %w", err)
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok {
		return ports.Identity{}, fmt.Errorf("oidcauth: no id_token in token response")
	}
	idt, err := a.verifier.Verify(ctx, raw)
	if err != nil {
		return ports.Identity{}, fmt.Errorf("oidcauth: verify id_token: %w", err)
	}
	var c struct {
		Sub               string   `json:"sub"`
		PreferredUsername string   `json:"preferred_username"`
		Email             string   `json:"email"`
		Name              string   `json:"name"`
		Groups            []string `json:"groups"`
	}
	if err := idt.Claims(&c); err != nil {
		return ports.Identity{}, fmt.Errorf("oidcauth: claims: %w", err)
	}
	return ports.Identity{Subject: c.Sub, Username: c.PreferredUsername, Email: c.Email, Name: c.Name, Groups: c.Groups}, nil
}
