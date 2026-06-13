package oidcserver

import (
	"github.com/serverkraken/flow/internal/ports"
)

// SubAllowlist is the Phase-1 AccessChecker — a finite set of OIDC 'sub'
// values that are permitted to use the server. Phase 2 will swap this for a
// User-table lookup once self-service registration is in scope.
//
// An empty list rejects everyone (fail-closed).
type SubAllowlist struct {
	set map[string]struct{}
}

// NewSubAllowlist builds a SubAllowlist from a slice of OIDC subject values.
// Empty strings are silently ignored; an empty slice rejects everyone.
func NewSubAllowlist(subs []string) *SubAllowlist {
	m := make(map[string]struct{}, len(subs))
	for _, s := range subs {
		if s == "" {
			continue
		}
		m[s] = struct{}{}
	}
	return &SubAllowlist{set: m}
}

// Allow reports whether the OIDC subject in id is on the allowlist.
func (a *SubAllowlist) Allow(id ports.Identity) bool {
	_, ok := a.set[id.Sub]
	return ok
}

// Compile-time assertion.
var _ ports.AccessChecker = (*SubAllowlist)(nil)
