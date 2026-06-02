package ports

import (
	"context"
	"time"
)

// Identity is the resolved OIDC identity after a successful token verification.
// Fields mirror the standard OIDC ID-Token claims that flow cares about.
type Identity struct {
	Sub           string
	Email         string
	EmailVerified bool
	Name          string
	IssuedAt      time.Time
	ExpiresAt     time.Time
}

// AuthProvider verifies an OIDC ID-Token (or access token, when audience-bound)
// and returns the corresponding Identity. Implementations cache JWKS keys.
type AuthProvider interface {
	Verify(ctx context.Context, rawToken string) (Identity, error)
}

// AccessChecker decides whether a verified Identity is allowed to use the
// server. Phase 1: allowlist of OIDC 'sub' values. Phase 2 will swap to a
// database-backed User table.
type AccessChecker interface {
	Allow(id Identity) bool
}

// Tokens holds the OAuth2 token bundle a client persists locally after login.
type Tokens struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	Expiry       time.Time
}

// TokenStore persists Tokens locally (typically in the OS-Keychain) so a CLI
// user logs in once and subsequent commands reuse the token. SlotName lets a
// single physical store (Keychain) hold tokens for different flow instances
// (e.g. dev vs prod servers).
type TokenStore interface {
	Get(slotName string) (Tokens, error)
	Put(slotName string, t Tokens) error
	Delete(slotName string) error
}

// ErrTokenNotFound is returned by TokenStore.Get when no tokens exist for the
// slot. Callers use this to distinguish "not logged in" from real errors.
var ErrTokenNotFound = errSentinel("flow: token not found")

// BrowserSessionStore persists browser-session state (post-OIDC-login). Phase 1
// uses signed/encrypted cookies via gorilla/securecookie so we have no
// server-side store yet; the interface lets us swap to Redis or DB later.
//
// Named BrowserSessionStore to avoid collision with ports.SessionStore which
// governs the worktime TSV log.
type BrowserSessionStore interface {
	Encode(name string, value any) (string, error)
	Decode(name, raw string, out any) error
}

type errSentinel string

func (e errSentinel) Error() string { return string(e) }
