package main

import "github.com/serverkraken/flow/internal/clientauth"

// newBootManager builds the authManager whose client is the shared clientauth
// builder (it re-reads the stored token on each build, so a fresh `flow login`
// is picked up without a reconnect). onAuth is wired by newServerH; auth is
// driven lazily by the first tool call (and an eager warm in main).
func newBootManager() *authManager {
	return newAuthManager(clientauth.Client, nil)
}
