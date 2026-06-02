package oidcclient

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

// TokensConfig wires Tokens to its dependencies.
type TokensConfig struct {
	Store         ports.TokenStore
	SlotName      string
	Refresh       func(context.Context, string) (ports.Tokens, error) // optional, no-op if nil
	RefreshLeeway time.Duration                                       // refresh this much before expiry
}

// Tokens is the high-level "give me a usable access token" API. Hides the
// TokenStore + Refresh sequence from callers (TUI/MCP).
type Tokens struct {
	cfg TokensConfig
}

// NewTokens constructs a Tokens facade wired to the given store and refresh
// function. If RefreshLeeway is zero it defaults to 60 s.
func NewTokens(c TokensConfig) *Tokens {
	if c.RefreshLeeway == 0 {
		c.RefreshLeeway = 60 * time.Second
	}
	return &Tokens{cfg: c}
}

// Current returns a valid access token, refreshing if needed.
func (t *Tokens) Current(ctx context.Context) (ports.Tokens, error) {
	cur, err := t.cfg.Store.Get(t.cfg.SlotName)
	if err != nil {
		return ports.Tokens{}, err
	}
	if time.Until(cur.Expiry) > t.cfg.RefreshLeeway {
		return cur, nil
	}
	if t.cfg.Refresh == nil || cur.RefreshToken == "" {
		return cur, nil // best effort
	}
	fresh, err := t.cfg.Refresh(ctx, cur.RefreshToken)
	if err != nil {
		return ports.Tokens{}, err
	}
	if err := t.cfg.Store.Put(t.cfg.SlotName, fresh); err != nil {
		return ports.Tokens{}, err
	}
	return fresh, nil
}

// Save persists a fresh token bundle.
func (t *Tokens) Save(tok ports.Tokens) error {
	return t.cfg.Store.Put(t.cfg.SlotName, tok)
}

// Delete clears the slot (used by `flow logout`).
func (t *Tokens) Delete() error { return t.cfg.Store.Delete(t.cfg.SlotName) }
