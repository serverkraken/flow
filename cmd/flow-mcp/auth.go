package main

import (
	"errors"
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

// hasValidToken returns true iff the keyring slot holds a token whose
// Expiry is in the future (or zero — meaning the issuer doesn't expose
// expiry, in which case we optimistically trust the token until the
// server returns 401). A token-not-found result is the common case on
// a fresh install; we do NOT treat it as an error.
func hasValidToken(store ports.TokenStore, slot string) bool {
	t, err := store.Get(slot)
	if err != nil {
		if errors.Is(err, ports.ErrTokenNotFound) {
			return false
		}
		// Real keyring error (locked, IO failure). Be conservative and
		// require login rather than risk leaking the wrong error
		// surface to the MCP client.
		return false
	}
	if t.AccessToken == "" {
		return false
	}
	if t.Expiry.IsZero() {
		return true
	}
	return t.Expiry.After(time.Now())
}
