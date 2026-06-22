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
// via build), runs onAuth exactly once on the first successful auth, and on an
// auth error drops and rebuilds the client so a fresh `flow login` is picked up
// without an MCP reconnect.
type authManager struct {
	build  func(ctx context.Context) (*apiclient.Client, error)
	onAuth func(ctx context.Context, c *apiclient.Client)

	mu     sync.Mutex
	cur    *apiclient.Client
	inited bool // onAuth has run
}

func newAuthManager(build func(context.Context) (*apiclient.Client, error), onAuth func(context.Context, *apiclient.Client)) *authManager {
	return &authManager{build: build, onAuth: onAuth}
}

// client returns the current authenticated client, building it (which re-reads
// the stored token) when absent. On the first successful build it fires onAuth
// exactly once, outside the lock (onAuth must not call back into client). A
// build failure is normalized to errLoginRequired.
func (m *authManager) client(ctx context.Context) (*apiclient.Client, error) {
	m.mu.Lock()
	if m.cur != nil {
		c := m.cur
		m.mu.Unlock()
		return c, nil
	}
	c, err := m.build(ctx)
	if err != nil {
		m.mu.Unlock()
		return nil, errLoginRequired
	}
	m.cur = c
	fire := !m.inited
	m.inited = true
	m.mu.Unlock()

	if fire && m.onAuth != nil {
		m.onAuth(ctx, c)
	}
	return c, nil
}

// reset drops the cached client so the next client() call rebuilds from the
// store. inited is left set: onAuth is a once-per-process post-auth init, not
// re-run on every recovery.
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
