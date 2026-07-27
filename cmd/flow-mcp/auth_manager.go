package main

import (
	"context"
	"errors"
	"sync"

	"golang.org/x/oauth2"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/clientauth"
)

// errLoginRequired is the sentinel a tool maps to the "run flow login" result.
var errLoginRequired = errors.New("login required")

// authManager owns the authenticated apiclient and the auth lifecycle for the
// long-running MCP process. It builds the client lazily (re-reading the keyring
// via build), runs onAuth after every successful client build, and on an
// auth error drops and rebuilds the client so a fresh `flow login` is picked up
// without an MCP reconnect.
type authManager struct {
	// base is the process-lifetime context the client is BUILT against. The
	// built apiclient's oauth2 token source bakes its context in at
	// construction and reuses it for every refresh (clientauth.lazyDeviceSource),
	// so building against a per-request context would leave the cached refresher
	// holding a canceled context the moment that request ends → every later
	// refresh fails "oidcdevice: context canceled", which is not an auth error,
	// so the wedged client is never rebuilt (no reconnect recovers it in-process).
	base   context.Context
	build  func(ctx context.Context) (*apiclient.Client, error)
	onAuth func(ctx context.Context, c *apiclient.Client)

	mu  sync.Mutex
	cur *apiclient.Client
}

func newAuthManager(build func(context.Context) (*apiclient.Client, error), onAuth func(context.Context, *apiclient.Client)) *authManager {
	return &authManager{base: context.Background(), build: build, onAuth: onAuth}
}

// client returns the current authenticated client, building it (which re-reads
// the stored token) when absent. After every successful build it fires onAuth
// outside the lock (onAuth must not call back into client). A
// build failure is normalized to errLoginRequired.
func (m *authManager) client(ctx context.Context) (*apiclient.Client, error) {
	// Fail fast if the triggering request is already canceled — but build the
	// client itself against m.base, never ctx (see the base field doc): the
	// token source must outlive this request.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	if m.cur != nil {
		c := m.cur
		m.mu.Unlock()
		return c, nil
	}
	c, err := m.build(m.base)
	if err != nil {
		m.mu.Unlock()
		return nil, errLoginRequired
	}
	m.cur = c
	m.mu.Unlock()

	if m.onAuth != nil {
		m.onAuth(m.base, c)
	}
	return c, nil
}

// reset drops the cached client so the next client() call rebuilds from the
// store and re-runs post-auth reconciliation for the new identity.
func (m *authManager) reset() {
	m.mu.Lock()
	m.cur = nil
	m.mu.Unlock()
}

// Do runs fn with the current client. On an auth error it resets, rebuilds, and
// retries fn exactly once; a persistent auth failure (or no usable token) is
// returned as errLoginRequired. Non-auth errors are returned unchanged and never
// retried.
func (m *authManager) Do(ctx context.Context, fn func(c *apiclient.Client) error) error {
	c, err := m.client(ctx)
	if err != nil {
		return err // already errLoginRequired
	}
	if err := fn(c); err == nil {
		return nil
	} else if !isAuthError(err) {
		return err
	}
	// Auth error: drop the (stale) client, rebuild from the store, retry once.
	m.reset()
	c, err = m.client(ctx)
	if err != nil {
		return err // errLoginRequired
	}
	if err := fn(c); err != nil {
		if isAuthError(err) {
			return errLoginRequired
		}
		return err
	}
	return nil
}

// isAuthError reports whether err means "the credential is bad" (so a rebuild
// from the store might help) rather than a transport/server failure. It matches
// an HTTP 401, the not-logged-in sentinel, and an OAuth refresh failure.
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	if apiclient.IsUnauthorized(err) {
		return true
	}
	if errors.Is(err, clientauth.ErrNotLoggedIn) {
		return true
	}
	var re *oauth2.RetrieveError
	return errors.As(err, &re)
}
